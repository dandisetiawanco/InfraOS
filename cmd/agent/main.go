package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dandisetiawanco/InfraOS/pkg/collector"
	"github.com/dandisetiawanco/InfraOS/pkg/contracts"
)

const agentVersion = "0.1.0"

type storedCredentials struct {
	AgentID        string `json:"agent_id"`
	HostID         string `json:"host_id"`
	OrganizationID string `json:"organization_id"`
	AgentToken     string `json:"agent_token"`
	APIURL         string `json:"api_url"`
	EnrolledAt     string `json:"enrolled_at"`
}

func main() {
	apiURL := flag.String("api-url", envOr("INFRAOS_AGENT_API_URL", "http://127.0.0.1:8080"), "InfraOS API base URL")
	enrollToken := flag.String("enroll-token", os.Getenv("INFRAOS_AGENT_ENROLL_TOKEN"), "One-time enrollment token")
	credsPath := flag.String("credentials-file", envOr("INFRAOS_AGENT_CREDENTIALS_PATH", "./.infraos/agent-credentials.json"), "Path to agent credential file")
	intervalSec := flag.Int("interval", envInt("INFRAOS_AGENT_INTERVAL_SECONDS", 10), "Metric collection interval in seconds")
	displayName := flag.String("display-name", os.Getenv("INFRAOS_AGENT_DISPLAY_NAME"), "Optional friendly host display name")
	tagsCSV := flag.String("tags", envOr("INFRAOS_AGENT_TAGS", "local,workstation"), "Comma-separated host tags")
	once := flag.Bool("once", false, "Collect and transmit one sample then exit")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if *intervalSec < 2 || *intervalSec > 300 {
		logger.Error("invalid sampling interval; must be between 2 and 300 seconds", "interval", *intervalSec)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	baseURL := strings.TrimRight(*apiURL, "/")
	httpClient := &http.Client{Timeout: 10 * time.Second}
	sysCollector := collector.NewSystemCollector(agentVersion)

	creds, err := loadCredentials(*credsPath)
	if err != nil {
		if strings.TrimSpace(*enrollToken) == "" {
			logger.Error("no stored credentials found and no --enroll-token provided", "credentials_file", *credsPath)
			os.Exit(1)
		}
		sample, err := sysCollector.Collect(ctx)
		if err != nil {
			logger.Error("initial system inspection failed", "error", err)
			os.Exit(1)
		}
		var tags []string
		for _, t := range strings.Split(*tagsCSV, ",") {
			if trimmed := strings.TrimSpace(t); trimmed != "" {
				tags = append(tags, trimmed)
			}
		}
		creds, err = enroll(ctx, httpClient, baseURL, contracts.AgentEnrollRequest{
			EnrollmentToken: strings.TrimSpace(*enrollToken),
			Hostname:        sample.Hostname,
			DisplayName:     strings.TrimSpace(*displayName),
			OS:              sample.OS,
			Arch:            sample.Arch,
			KernelVersion:   sample.KernelVersion,
			IPAddress:       sample.IPAddress,
			AgentVersion:    agentVersion,
			Tags:            tags,
		})
		if err != nil {
			logger.Error("agent enrollment failed", "error", err)
			os.Exit(1)
		}
		creds.APIURL = baseURL
		if err := saveCredentials(*credsPath, creds); err != nil {
			logger.Error("failed to save agent credentials", "error", err)
			os.Exit(1)
		}
		logger.Info("agent enrolled", "agent_id", creds.AgentID, "host_id", creds.HostID, "os", runtime.GOOS, "arch", runtime.GOARCH)
	}

	// Bounded retry buffer caps memory use when the API is temporarily unreachable.
	const maxQueue = 60
	var retryQueue []contracts.HostMetricPayload
	attempt := 0

	sendBatch := func() bool {
		for len(retryQueue) > 0 {
			if err := postMetric(ctx, httpClient, baseURL, creds.AgentToken, retryQueue[0]); err != nil {
				logger.Warn("failed to transmit host metrics", "error", err, "buffered", len(retryQueue))
				return false
			}
			retryQueue = retryQueue[1:]
		}
		return true
	}

	collectAndSend := func() error {
		sample, err := sysCollector.Collect(ctx)
		if err != nil {
			return err
		}
		if len(retryQueue) >= maxQueue {
			retryQueue = retryQueue[1:]
		}
		retryQueue = append(retryQueue, sample)
		if sendBatch() {
			attempt = 0
			logger.Info("metrics transmitted",
				"host_id", creds.HostID,
				"cpu_percent", sample.CPUPercent,
				"mem_percent", sample.MemPercent,
				"disk_percent", sample.DiskPercent,
				"net_rx_bps", sample.NetRxBps,
				"net_tx_bps", sample.NetTxBps,
			)
			return nil
		}
		attempt++
		return errors.New("delivery deferred to retry buffer")
	}

	if err := collectAndSend(); err != nil && *once {
		logger.Error("single-shot collection failed", "error", err)
		os.Exit(1)
	}
	if *once {
		return
	}

	interval := time.Duration(*intervalSec) * time.Second
	logger.Info("agent running", "host_id", creds.HostID, "api_url", baseURL, "interval_seconds", *intervalSec)

	for {
		wait := interval
		if attempt > 0 {
			wait = backoffWithJitter(interval, attempt)
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			logger.Info("agent shutting down cleanly")
			return
		case <-timer.C:
			_ = collectAndSend()
		}
	}
}

func enroll(ctx context.Context, client *http.Client, baseURL string, req contracts.AgentEnrollRequest) (storedCredentials, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return storedCredentials{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/agents/enroll", bytes.NewReader(body))
	if err != nil {
		return storedCredentials{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return storedCredentials{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return storedCredentials{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var envelope struct {
		Data contracts.AgentEnrollResponse `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return storedCredentials{}, err
	}
	if envelope.Data.AgentToken == "" {
		return storedCredentials{}, errors.New("enrollment response missing agent token")
	}
	return storedCredentials{
		AgentID:        envelope.Data.AgentID,
		HostID:         envelope.Data.HostID,
		OrganizationID: envelope.Data.OrganizationID,
		AgentToken:     envelope.Data.AgentToken,
		EnrolledAt:     time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func postMetric(ctx context.Context, client *http.Client, baseURL, token string, sample contracts.HostMetricPayload) error {
	body, err := json.Marshal(sample)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/agents/metrics", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Agent "+token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func loadCredentials(path string) (storedCredentials, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return storedCredentials{}, err
	}
	var creds storedCredentials
	if err := json.Unmarshal(raw, &creds); err != nil {
		return storedCredentials{}, err
	}
	if creds.AgentToken == "" || creds.HostID == "" {
		return storedCredentials{}, errors.New("incomplete credentials file")
	}
	return creds, nil
}

func saveCredentials(path string, creds storedCredentials) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func backoffWithJitter(base time.Duration, attempt int) time.Duration {
	if attempt > 5 {
		attempt = 5
	}
	delay := base * (1 << (attempt - 1))
	if delay > 60*time.Second {
		delay = 60 * time.Second
	}
	jitter := time.Duration(rand.Int64N(int64(delay/4 + 1)))
	return delay + jitter
}

func envOr(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}

func envInt(k string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
