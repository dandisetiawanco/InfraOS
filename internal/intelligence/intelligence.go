package intelligence

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/database"
	"github.com/dandisetiawanco/InfraOS/internal/security"
)

type Insight struct {
	ID              string   `json:"id"`
	Category        string   `json:"category"`
	Severity        string   `json:"severity"`
	Title           string   `json:"title"`
	Detail          string   `json:"detail"`
	CorrelationOnly bool     `json:"correlation_only"`
	ResourceType    string   `json:"resource_type"`
	ResourceID      string   `json:"resource_id"`
	Evidence        []string `json:"evidence"`
	DetectedAt      string   `json:"detected_at"`
}

type AISummaryRequest struct {
	Title       string
	Description string
	Severity    string
	Timeline    []string
}

type AISummaryResponse struct {
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	Summary         string   `json:"summary"`
	SuggestedChecks []string `json:"suggested_checks"`
	AdvisoryNote    string   `json:"advisory_note"`
}

type Analyzer struct {
	db         *database.DB
	aiProvider string
	aiEndpoint string
	aiAPIKey   string
	aiModel    string
	httpClient *http.Client
}

func NewAnalyzer(db *database.DB, aiProvider, aiEndpoint, aiAPIKey, aiModel string) *Analyzer {
	return &Analyzer{
		db:         db,
		aiProvider: strings.ToLower(strings.TrimSpace(aiProvider)),
		aiEndpoint: strings.TrimSpace(aiEndpoint),
		aiAPIKey:   strings.TrimSpace(aiAPIKey),
		aiModel:    strings.TrimSpace(aiModel),
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (a *Analyzer) AIEnabled() bool {
	return a.aiProvider != "" && a.aiProvider != "disabled"
}

// Analyze generates deterministic operational insights from real stored telemetry without external AI.
func (a *Analyzer) Analyze(ctx context.Context, orgID string) ([]Insight, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	insights := make([]Insight, 0)

	// 1. Sustained memory pressure & disk capacity threshold across hosts.
	type hostRow struct {
		id, hostname, status, lastHB string
		synthInt                     int
	}
	var hostItems []hostRow
	hostRows, err := a.db.Query(ctx, `
		SELECT id, hostname, status, COALESCE(last_heartbeat_at, ''), CASE WHEN is_synthetic THEN 1 ELSE 0 END
		FROM hosts WHERE organization_id = ?`, orgID)
	if err == nil {
		for hostRows.Next() {
			var h hostRow
			if err := hostRows.Scan(&h.id, &h.hostname, &h.status, &h.lastHB, &h.synthInt); err == nil {
				hostItems = append(hostItems, h)
			}
		}
		_ = hostRows.Close()
	}

	for _, h := range hostItems {
		hostID, hostname, status, lastHB, synthInt := h.id, h.hostname, h.status, h.lastHB, h.synthInt

		// Missing agent heartbeats
		if synthInt == 0 && status != "revoked" && lastHB != "" {
			if hbTime, err := time.Parse(time.RFC3339, lastHB); err == nil && now.Sub(hbTime) > 90*time.Second {
				insights = append(insights, Insight{
					ID:           "stale-hb-" + hostID,
					Category:     "agent_heartbeat",
					Severity:     "warning",
					Title:        fmt.Sprintf("Missing agent heartbeats from %s", hostname),
					Detail:       fmt.Sprintf("Last heartbeat was received %s ago.", now.Sub(hbTime).Round(time.Second)),
					ResourceType: "host",
					ResourceID:   hostID,
					Evidence:     []string{"Last heartbeat: " + lastHB},
					DetectedAt:   nowStr,
				})
			}
		}

		// Inspect last 5 metric samples for sustained memory pressure and disk capacity
		mRows, err := a.db.Query(ctx, `
			SELECT mem_percent, disk_percent, cpu_percent
			FROM host_metrics WHERE organization_id = ? AND host_id = ?
			ORDER BY id DESC LIMIT 5`, orgID, hostID)
		if err != nil {
			continue
		}
		var highMemCount int
		var latestDisk, latestMem float64
		samples := 0
		for mRows.Next() {
			var memPct, diskPct, cpuPct float64
			if err := mRows.Scan(&memPct, &diskPct, &cpuPct); err == nil {
				if samples == 0 {
					latestDisk = diskPct
					latestMem = memPct
				}
				if memPct >= 80 {
					highMemCount++
				}
				samples++
			}
		}
		_ = mRows.Close()

		if samples >= 3 && highMemCount == samples {
			insights = append(insights, Insight{
				ID:           "mem-pressure-" + hostID,
				Category:     "memory_pressure",
				Severity:     "warning",
				Title:        fmt.Sprintf("Sustained memory pressure on %s", hostname),
				Detail:       fmt.Sprintf("Memory utilization remained above 80%% across the last %d samples (latest %.1f%%).", samples, latestMem),
				ResourceType: "host",
				ResourceID:   hostID,
				Evidence:     []string{fmt.Sprintf("Consecutive samples >= 80%%: %d", highMemCount), fmt.Sprintf("Latest memory usage: %.1f%%", latestMem)},
				DetectedAt:   nowStr,
			})
		}
		if samples > 0 && latestDisk >= 80 {
			sev := "warning"
			if latestDisk >= 90 {
				sev = "critical"
			}
			insights = append(insights, Insight{
				ID:           "disk-cap-" + hostID,
				Category:     "disk_capacity",
				Severity:     sev,
				Title:        fmt.Sprintf("Disk capacity approaching limit on %s", hostname),
				Detail:       fmt.Sprintf("Primary filesystem usage is %.1f%%.", latestDisk),
				ResourceType: "host",
				ResourceID:   hostID,
				Evidence:     []string{fmt.Sprintf("Observed disk usage: %.1f%%", latestDisk)},
				DetectedAt:   nowStr,
			})
		}
	}

	// 2. Service repeated failures, latency increase, and deployment correlation (distinct from root cause).
	type svcRow struct {
		id, name, status, envID string
		consecFail              int
	}
	var svcItems []svcRow
	svcRows, err := a.db.Query(ctx, `
		SELECT id, name, status, consecutive_failures, COALESCE(environment_id, '')
		FROM services WHERE organization_id = ?`, orgID)
	if err == nil {
		for svcRows.Next() {
			var sv svcRow
			if err := svcRows.Scan(&sv.id, &sv.name, &sv.status, &sv.consecFail, &sv.envID); err == nil {
				svcItems = append(svcItems, sv)
			}
		}
		_ = svcRows.Close()
	}

	for _, sv := range svcItems {
		svcID, svcName, status, consecFail, envID := sv.id, sv.name, sv.status, sv.consecFail, sv.envID

		if consecFail >= 2 || status == "down" {
			insights = append(insights, Insight{
				ID:           "svc-fail-" + svcID,
				Category:     "service_failures",
				Severity:     "critical",
				Title:        fmt.Sprintf("Repeated check failures on %s", svcName),
				Detail:       fmt.Sprintf("Service status is %s with %d consecutive failed checks.", status, consecFail),
				ResourceType: "service",
				ResourceID:   svcID,
				Evidence:     []string{fmt.Sprintf("Consecutive failures: %d", consecFail)},
				DetectedAt:   nowStr,
			})

			// Check for recent deployment in the same environment within 60 minutes.
			if envID != "" {
				var depID, depVer, depTime string
				err := a.db.QueryRow(ctx, `
					SELECT id, version, created_at FROM deployments
					WHERE organization_id = ? AND environment_id = ? AND created_at >= ?
					ORDER BY created_at DESC LIMIT 1`,
					orgID, envID, now.Add(-60*time.Minute).Format(time.RFC3339)).Scan(&depID, &depVer, &depTime)
				if err == nil {
					insights = append(insights, Insight{
						ID:              "dep-corr-" + svcID,
						Category:        "deployment_correlation",
						Severity:        "warning",
						Title:           fmt.Sprintf("Degradation on %s followed release %s", svcName, depVer),
						Detail:          "Temporal correlation observed between a recent deployment and service degradation. Verify logs and rollout changes before concluding root cause.",
						CorrelationOnly: true,
						ResourceType:    "deployment",
						ResourceID:      depID,
						Evidence:        []string{"Deployment version: " + depVer, "Deployed at: " + depTime, "Correlation does not confirm root cause."},
						DetectedAt:      nowStr,
					})
				}
			}
		}

		// Latency trend over recent checks
		cRows, err := a.db.Query(ctx, `
			SELECT latency_ms FROM service_checks
			WHERE organization_id = ? AND service_id = ? AND status = 'up'
			ORDER BY id DESC LIMIT 10`, orgID, svcID)
		if err == nil {
			var vals []int
			for cRows.Next() {
				var ms int
				if err := cRows.Scan(&ms); err == nil {
					vals = append(vals, ms)
				}
			}
			_ = cRows.Close()
			if len(vals) >= 6 {
				recentAvg := avgInts(vals[:3])
				baselineAvg := avgInts(vals[3:])
				if baselineAvg >= 20 && recentAvg > baselineAvg*1.5 && recentAvg-baselineAvg >= 40 {
					insights = append(insights, Insight{
						ID:           "latency-trend-" + svcID,
						Category:     "latency_trend",
						Severity:     "warning",
						Title:        fmt.Sprintf("Increasing response latency on %s", svcName),
						Detail:       fmt.Sprintf("Recent average latency (%.0f ms) is above the prior baseline (%.0f ms).", recentAvg, baselineAvg),
						ResourceType: "service",
						ResourceID:   svcID,
						Evidence:     []string{fmt.Sprintf("Recent 3-check average: %.0f ms", recentAvg), fmt.Sprintf("Prior baseline average: %.0f ms", baselineAvg)},
						DetectedAt:   nowStr,
					})
				}
			}
		}
	}

	return insights, nil
}

// SummarizeIncident invokes an optional configured AI provider with redacted incident evidence.
func (a *Analyzer) SummarizeIncident(ctx context.Context, in AISummaryRequest) (*AISummaryResponse, error) {
	if !a.AIEnabled() {
		return nil, errors.New("external AI summarization is disabled by default; deterministic operational insights remain active")
	}

	redactedDesc := security.RedactCredentials(in.Description)
	redactedTimeline := make([]string, 0, len(in.Timeline))
	for _, line := range in.Timeline {
		if len(redactedTimeline) >= 12 {
			break
		}
		redactedTimeline = append(redactedTimeline, security.RedactCredentials(line))
	}

	prompt := fmt.Sprintf(
		"Summarize this infrastructure incident objectively in 3 sentences and provide 3 safe read-only investigation steps. Do not assume root cause beyond evidence.\nTitle: %s\nSeverity: %s\nDescription: %s\nTimeline:\n- %s",
		security.RedactCredentials(in.Title),
		in.Severity,
		redactedDesc,
		strings.Join(redactedTimeline, "\n- "),
	)

	var summaryText string
	var err error
	model := a.aiModel

	switch a.aiProvider {
	case "openai":
		if model == "" {
			model = "gpt-4o-mini"
		}
		endpoint := a.aiEndpoint
		if endpoint == "" {
			endpoint = "https://api.openai.com/v1/chat/completions"
		}
		summaryText, err = a.callOpenAICompatible(ctx, endpoint, model, prompt)
	case "ollama":
		if model == "" {
			model = "llama3.2"
		}
		endpoint := a.aiEndpoint
		if endpoint == "" {
			endpoint = "http://127.0.0.1:11434/api/generate"
		}
		summaryText, err = a.callOllama(ctx, endpoint, model, prompt)
	case "gemini":
		if model == "" {
			model = "gemini-2.0-flash"
		}
		summaryText, err = a.callGemini(ctx, model, prompt)
	default:
		return nil, fmt.Errorf("unsupported AI provider %q", a.aiProvider)
	}
	if err != nil {
		return nil, err
	}

	return &AISummaryResponse{
		Provider:        a.aiProvider,
		Model:           model,
		Summary:         strings.TrimSpace(summaryText),
		SuggestedChecks: []string{"Inspect correlated service checks and logs around incident start.", "Compare recent configuration or deployment changes without assuming causation.", "Confirm resource pressure metrics on affected hosts."},
		AdvisoryNote:    "AI output is advisory only and cannot execute commands or mutate infrastructure.",
	}, nil
}

func (a *Analyzer) callOpenAICompatible(ctx context.Context, endpoint, model, prompt string) (string, error) {
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "You summarize operational incidents conservatively based only on provided evidence."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.aiAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.aiAPIKey)
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("AI provider returned HTTP %d", resp.StatusCode)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Choices) == 0 {
		return "", errors.New("unexpected response from AI provider")
	}
	return parsed.Choices[0].Message.Content, nil
}

func (a *Analyzer) callOllama(ctx context.Context, endpoint, model, prompt string) (string, error) {
	raw, _ := json.Marshal(map[string]any{"model": model, "prompt": prompt, "stream": false})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var parsed struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(&parsed); err != nil {
		return "", err
	}
	return parsed.Response, nil
}

func (a *Analyzer) callGemini(ctx context.Context, model, prompt string) (string, error) {
	if a.aiAPIKey == "" {
		return "", errors.New("INFRAOS_AI_API_KEY is required for Gemini provider")
	}
	endpoint := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, a.aiAPIKey)
	raw, _ := json.Marshal(map[string]any{
		"contents": []map[string]any{
			{"parts": []map[string]string{{"text": prompt}}},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(&parsed); err != nil || len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("unexpected Gemini response")
	}
	return parsed.Candidates[0].Content.Parts[0].Text, nil
}

func avgInts(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0
	for _, v := range vals {
		sum += v
	}
	return float64(sum) / float64(len(vals))
}
