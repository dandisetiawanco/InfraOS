package monitoring

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/database"
	"github.com/dandisetiawanco/InfraOS/internal/security"
	"github.com/dandisetiawanco/InfraOS/internal/sse"
	"github.com/dandisetiawanco/InfraOS/pkg/telemetry"
	"github.com/google/uuid"
)

type ServiceRecord struct {
	ID                   string   `json:"id"`
	OrgID                string   `json:"organization_id"`
	ProjectID            string   `json:"project_id,omitempty"`
	EnvironmentID        string   `json:"environment_id,omitempty"`
	EnvironmentName      string   `json:"environment_name,omitempty"`
	HostID               string   `json:"host_id,omitempty"`
	Name                 string   `json:"name"`
	CheckType            string   `json:"check_type"`
	Target               string   `json:"target"`
	IntervalSeconds      int      `json:"interval_seconds"`
	TimeoutMS            int      `json:"timeout_ms"`
	ExpectedStatus       int      `json:"expected_status"`
	FailureThreshold     int      `json:"failure_threshold"`
	RecoveryThreshold    int      `json:"recovery_threshold"`
	Enabled              bool     `json:"enabled"`
	Status               string   `json:"status"`
	ConsecutiveFailures  int      `json:"consecutive_failures"`
	ConsecutiveSuccesses int      `json:"consecutive_successes"`
	LastCheckedAt        string   `json:"last_checked_at,omitempty"`
	LastLatencyMS        int      `json:"last_latency_ms"`
	LastError            string   `json:"last_error,omitempty"`
	TLSExpiresAt         string   `json:"tls_expires_at,omitempty"`
	UptimePercent        *float64 `json:"uptime_percent"`
	ObservationsCount    int      `json:"observations_count"`
	IsSynthetic          bool     `json:"is_synthetic"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}

type CheckResult struct {
	ID               int64    `json:"id"`
	ServiceID        string   `json:"service_id"`
	CheckedAt        string   `json:"checked_at"`
	Status           string   `json:"status"`
	StatusCode       int      `json:"status_code"`
	LatencyMS        int      `json:"latency_ms"`
	FailureReason    string   `json:"failure_reason,omitempty"`
	TLSDaysRemaining *int     `json:"tls_days_remaining,omitempty"`
	ResolvedIPs      []string `json:"resolved_ips,omitempty"`
	IsSynthetic      bool     `json:"is_synthetic"`
}

type CreateServiceInput struct {
	Name              string
	CheckType         string
	Target            string
	ProjectID         string
	EnvironmentID     string
	HostID            string
	IntervalSeconds   int
	TimeoutMS         int
	ExpectedStatus    int
	FailureThreshold  int
	RecoveryThreshold int
}

type CheckHook func(ctx context.Context, svc ServiceRecord, result CheckResult)

type Scheduler struct {
	db       *database.DB
	ssrf     *security.SSRFValidator
	broker   *sse.Broker
	metrics  *telemetry.Metrics
	logger   *slog.Logger
	sem      chan struct{}
	mu       sync.Mutex
	inFlight map[string]bool
	onCheck  CheckHook
	wg       sync.WaitGroup
}

func NewScheduler(db *database.DB, ssrf *security.SSRFValidator, broker *sse.Broker, metrics *telemetry.Metrics, logger *slog.Logger, maxConcurrency int) *Scheduler {
	if maxConcurrency <= 0 {
		maxConcurrency = 16
	}
	return &Scheduler{
		db:       db,
		ssrf:     ssrf,
		broker:   broker,
		metrics:  metrics,
		logger:   logger,
		sem:      make(chan struct{}, maxConcurrency),
		inFlight: make(map[string]bool),
	}
}

func (s *Scheduler) SetCheckHook(fn CheckHook) {
	s.onCheck = fn
}

func (s *Scheduler) ValidateTarget(ctx context.Context, checkType, target string, isDemo bool) error {
	checkType = strings.ToLower(strings.TrimSpace(checkType))
	target = strings.TrimSpace(target)
	switch checkType {
	case "http", "https":
		_, _, err := s.ssrf.ValidateHTTPURL(ctx, target, isDemo)
		return err
	case "tcp", "tls":
		_, _, err := s.ssrf.ValidateHostPort(ctx, target, isDemo)
		return err
	case "dns":
		if isDemo {
			return errors.New("external DNS probes are disabled in demonstration mode")
		}
		if target == "" || strings.ContainsAny(target, " /:\\") {
			return errors.New("dns target must be a valid hostname")
		}
		_, err := s.ssrf.ResolveAndValidateHost(ctx, target, false)
		return err
	default:
		return fmt.Errorf("unsupported check type %q", checkType)
	}
}

func (s *Scheduler) CreateService(ctx context.Context, orgID string, isDemo bool, in CreateServiceInput) (*ServiceRecord, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.CheckType = strings.ToLower(strings.TrimSpace(in.CheckType))
	in.Target = strings.TrimSpace(in.Target)
	if in.Name == "" || in.Target == "" {
		return nil, errors.New("service name and target are required")
	}
	if err := s.ValidateTarget(ctx, in.CheckType, in.Target, isDemo); err != nil {
		return nil, err
	}
	if in.IntervalSeconds < 10 {
		in.IntervalSeconds = 30
	}
	if in.IntervalSeconds > 3600 {
		in.IntervalSeconds = 3600
	}
	if in.TimeoutMS < 500 || in.TimeoutMS > 30000 {
		in.TimeoutMS = 5000
	}
	if in.ExpectedStatus == 0 {
		in.ExpectedStatus = 200
	}
	if in.FailureThreshold < 1 {
		in.FailureThreshold = 2
	}
	if in.RecoveryThreshold < 1 {
		in.RecoveryThreshold = 2
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	var projArg, envArg, hostArg any
	if in.ProjectID != "" {
		projArg = in.ProjectID
	}
	if in.EnvironmentID != "" {
		envArg = in.EnvironmentID
	}
	if in.HostID != "" {
		hostArg = in.HostID
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO services (
			id, organization_id, project_id, environment_id, host_id, name, check_type, target,
			interval_seconds, timeout_ms, expected_status, failure_threshold, recovery_threshold,
			enabled, status, is_synthetic, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'unknown', ?, ?, ?)`,
		id, orgID, projArg, envArg, hostArg, in.Name, in.CheckType, in.Target,
		in.IntervalSeconds, in.TimeoutMS, in.ExpectedStatus, in.FailureThreshold, in.RecoveryThreshold,
		database.BoolToDB(s.db.Driver, true), database.BoolToDB(s.db.Driver, false), now, now,
	)
	if err != nil {
		return nil, err
	}
	return s.GetService(ctx, orgID, id)
}

func (s *Scheduler) DeleteService(ctx context.Context, orgID, serviceID string) error {
	res, err := s.db.Exec(ctx, `DELETE FROM services WHERE id = ? AND organization_id = ?`, serviceID, orgID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("service not found")
	}
	return nil
}

func (s *Scheduler) ListServices(ctx context.Context, orgID, envID string) ([]ServiceRecord, error) {
	where := "WHERE sv.organization_id = ?"
	args := []any{orgID}
	if envID != "" {
		where += " AND sv.environment_id = ?"
		args = append(args, envID)
	}
	rows, err := s.db.Query(ctx, `
		SELECT sv.id, sv.organization_id, COALESCE(sv.project_id, ''), COALESCE(sv.environment_id, ''),
		       COALESCE(e.name, ''), COALESCE(sv.host_id, ''), sv.name, sv.check_type, sv.target,
		       sv.interval_seconds, sv.timeout_ms, sv.expected_status, sv.failure_threshold, sv.recovery_threshold,
		       CASE WHEN sv.enabled THEN 1 ELSE 0 END, sv.status, sv.consecutive_failures, sv.consecutive_successes,
		       COALESCE(sv.last_checked_at, ''), sv.last_latency_ms, sv.last_error, COALESCE(sv.tls_expires_at, ''),
		       CASE WHEN sv.is_synthetic THEN 1 ELSE 0 END, sv.created_at, sv.updated_at
		FROM services sv
		LEFT JOIN environments e ON e.id = sv.environment_id
		`+where+`
		ORDER BY sv.created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ServiceRecord, 0)
	for rows.Next() {
		var r ServiceRecord
		var enInt, synthInt int
		if err := rows.Scan(
			&r.ID, &r.OrgID, &r.ProjectID, &r.EnvironmentID, &r.EnvironmentName, &r.HostID,
			&r.Name, &r.CheckType, &r.Target, &r.IntervalSeconds, &r.TimeoutMS, &r.ExpectedStatus,
			&r.FailureThreshold, &r.RecoveryThreshold, &enInt, &r.Status,
			&r.ConsecutiveFailures, &r.ConsecutiveSuccesses, &r.LastCheckedAt, &r.LastLatencyMS,
			&r.LastError, &r.TLSExpiresAt, &synthInt, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		r.Enabled = enInt == 1
		r.IsSynthetic = synthInt == 1
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		uptime, count := s.computeObservedUptime(ctx, orgID, out[i].ID)
		out[i].UptimePercent = uptime
		out[i].ObservationsCount = count
	}
	return out, nil
}

func (s *Scheduler) GetService(ctx context.Context, orgID, serviceID string) (*ServiceRecord, error) {
	list, err := s.ListServices(ctx, orgID, "")
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == serviceID {
			item := list[i]
			return &item, nil
		}
	}
	return nil, errors.New("service not found")
}

func (s *Scheduler) ListChecks(ctx context.Context, orgID, serviceID string, limit int) ([]CheckResult, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, service_id, checked_at, status, status_code, latency_ms, failure_reason,
		       tls_days_remaining, resolved_ips_json, CASE WHEN is_synthetic THEN 1 ELSE 0 END
		FROM service_checks
		WHERE organization_id = ? AND service_id = ?
		ORDER BY id DESC LIMIT ?`, orgID, serviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]CheckResult, 0)
	for rows.Next() {
		var c CheckResult
		var tlsDays sql.NullInt64
		var ipsRaw string
		var synthInt int
		if err := rows.Scan(&c.ID, &c.ServiceID, &c.CheckedAt, &c.Status, &c.StatusCode, &c.LatencyMS, &c.FailureReason, &tlsDays, &ipsRaw, &synthInt); err != nil {
			return nil, err
		}
		if tlsDays.Valid {
			v := int(tlsDays.Int64)
			c.TLSDaysRemaining = &v
		}
		_ = json.Unmarshal([]byte(ipsRaw), &c.ResolvedIPs)
		c.IsSynthetic = synthInt == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Scheduler) computeObservedUptime(ctx context.Context, orgID, serviceID string) (*float64, int) {
	var total, up int
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN status = 'up' THEN 1 ELSE 0 END), 0)
		FROM service_checks
		WHERE organization_id = ? AND service_id = ?`, orgID, serviceID).Scan(&total, &up)
	if err != nil || total == 0 {
		return nil, 0
	}
	pct := float64(int(float64(up)/float64(total)*10000)) / 100
	return &pct, total
}

func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		s.sweep(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sweep(ctx)
			}
		}
	}()
}

func (s *Scheduler) Wait() {
	s.wg.Wait()
}

func (s *Scheduler) sweep(ctx context.Context) {
	rows, err := s.db.Query(ctx, `
		SELECT sv.id, sv.organization_id, sv.name, sv.check_type, sv.target,
		       sv.interval_seconds, sv.timeout_ms, sv.expected_status,
		       sv.failure_threshold, sv.recovery_threshold,
		       sv.status, sv.consecutive_failures, sv.consecutive_successes,
		       COALESCE(sv.last_checked_at, '')
		FROM services sv
		JOIN organizations o ON o.id = sv.organization_id
		WHERE sv.enabled = ? AND sv.is_synthetic = ? AND o.is_demo = ?`,
		database.BoolToDB(s.db.Driver, true), database.BoolToDB(s.db.Driver, false), database.BoolToDB(s.db.Driver, false),
	)
	if err != nil {
		s.logger.Error("monitoring sweep query failed", "error", err)
		return
	}
	defer rows.Close()

	var due []ServiceRecord
	now := time.Now().UTC()
	for rows.Next() {
		var sv ServiceRecord
		if err := rows.Scan(
			&sv.ID, &sv.OrgID, &sv.Name, &sv.CheckType, &sv.Target,
			&sv.IntervalSeconds, &sv.TimeoutMS, &sv.ExpectedStatus,
			&sv.FailureThreshold, &sv.RecoveryThreshold,
			&sv.Status, &sv.ConsecutiveFailures, &sv.ConsecutiveSuccesses, &sv.LastCheckedAt,
		); err != nil {
			continue
		}
		if sv.LastCheckedAt != "" {
			if last, err := time.Parse(time.RFC3339, sv.LastCheckedAt); err == nil {
				if now.Sub(last) < time.Duration(sv.IntervalSeconds)*time.Second {
					continue
				}
			}
		}
		due = append(due, sv)
	}

	for _, svc := range due {
		s.mu.Lock()
		if s.inFlight[svc.ID] {
			s.mu.Unlock()
			continue
		}
		s.inFlight[svc.ID] = true
		s.mu.Unlock()

		s.wg.Add(1)
		go func(item ServiceRecord) {
			defer s.wg.Done()
			defer func() {
				s.mu.Lock()
				delete(s.inFlight, item.ID)
				s.mu.Unlock()
			}()

			select {
			case s.sem <- struct{}{}:
				defer func() { <-s.sem }()
			case <-ctx.Done():
				return
			}
			if s.metrics != nil {
				s.metrics.ActiveMonitorJobs.Inc()
				defer s.metrics.ActiveMonitorJobs.Dec()
			}
			_, _ = s.RunCheckNow(ctx, item.OrgID, item.ID, false)
		}(svc)
	}
}

func (s *Scheduler) RunCheckNow(ctx context.Context, orgID, serviceID string, isDemo bool) (*CheckResult, error) {
	if isDemo {
		return nil, errors.New("live external checks are disabled in demonstration mode")
	}
	svc, err := s.GetService(ctx, orgID, serviceID)
	if err != nil {
		return nil, err
	}
	if svc.IsSynthetic {
		return nil, errors.New("cannot run live network checks on synthetic service")
	}

	result, tlsExpiresAt := s.executeProbe(ctx, *svc)
	nowStr := time.Now().UTC().Format(time.RFC3339)
	result.CheckedAt = nowStr
	result.ServiceID = svc.ID

	ipsJSON, _ := json.Marshal(result.ResolvedIPs)
	if len(result.ResolvedIPs) == 0 {
		ipsJSON = []byte("[]")
	}
	var tlsDaysArg any
	if result.TLSDaysRemaining != nil {
		tlsDaysArg = *result.TLSDaysRemaining
	}

	newStatus := svc.Status
	consecFail := svc.ConsecutiveFailures
	consecSucc := svc.ConsecutiveSuccesses
	if result.Status == "up" {
		consecFail = 0
		consecSucc++
		if svc.Status == "unknown" || consecSucc >= svc.RecoveryThreshold {
			newStatus = "healthy"
		} else {
			newStatus = "degraded"
		}
	} else {
		consecSucc = 0
		consecFail++
		if consecFail >= svc.FailureThreshold {
			newStatus = "down"
		} else {
			newStatus = "degraded"
		}
	}

	var tlsExpArg any
	if tlsExpiresAt != "" {
		tlsExpArg = tlsExpiresAt
	}

	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO service_checks (organization_id, service_id, checked_at, status, status_code, latency_ms, failure_reason, tls_days_remaining, resolved_ips_json, is_synthetic)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			orgID, svc.ID, nowStr, result.Status, result.StatusCode, result.LatencyMS, result.FailureReason,
			tlsDaysArg, string(ipsJSON), database.BoolToDB(s.db.Driver, false),
		); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, s.db.Rebind(`
			UPDATE services SET status = ?, consecutive_failures = ?, consecutive_successes = ?,
			                    last_checked_at = ?, last_latency_ms = ?, last_error = ?,
			                    tls_expires_at = COALESCE(?, tls_expires_at), updated_at = ?
			WHERE id = ? AND organization_id = ?`),
			newStatus, consecFail, consecSucc, nowStr, result.LatencyMS, result.FailureReason, tlsExpArg, nowStr, svc.ID, orgID,
		)
		return err
	})
	if err != nil {
		return nil, err
	}

	svc.Status = newStatus
	svc.LastLatencyMS = result.LatencyMS
	svc.LastError = result.FailureReason
	svc.LastCheckedAt = nowStr

	if s.broker != nil {
		s.broker.Publish(orgID, "service.check", map[string]any{
			"service_id": svc.ID,
			"name":       svc.Name,
			"status":     newStatus,
			"latency_ms": result.LatencyMS,
			"checked_at": nowStr,
		})
	}
	if s.onCheck != nil {
		s.onCheck(ctx, *svc, result)
	}
	return &result, nil
}

func (s *Scheduler) executeProbe(ctx context.Context, svc ServiceRecord) (CheckResult, string) {
	timeout := time.Duration(svc.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	switch svc.CheckType {
	case "http", "https":
		_, ips, err := s.ssrf.ValidateHTTPURL(probeCtx, svc.Target, false)
		if err != nil {
			return CheckResult{Status: "down", LatencyMS: int(time.Since(start).Milliseconds()), FailureReason: err.Error()}, ""
		}
		client := s.ssrf.NewHTTPClient(timeout, false)
		req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, svc.Target, nil)
		if err != nil {
			return CheckResult{Status: "down", LatencyMS: int(time.Since(start).Milliseconds()), FailureReason: err.Error()}, ""
		}
		req.Header.Set("User-Agent", "InfraOS-Monitor/0.1")
		resp, err := client.Do(req)
		latency := int(time.Since(start).Milliseconds())
		resolvedStrs := ipsToStrings(ips)
		if err != nil {
			return CheckResult{Status: "down", LatencyMS: latency, FailureReason: err.Error(), ResolvedIPs: resolvedStrs}, ""
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))

		var tlsDays *int
		var tlsExp string
		if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
			exp := resp.TLS.PeerCertificates[0].NotAfter.UTC()
			days := int(time.Until(exp).Hours() / 24)
			tlsDays = &days
			tlsExp = exp.Format(time.RFC3339)
		}

		if resp.StatusCode != svc.ExpectedStatus {
			return CheckResult{
				Status:           "down",
				StatusCode:       resp.StatusCode,
				LatencyMS:        latency,
				FailureReason:    fmt.Sprintf("unexpected HTTP status %d (expected %d)", resp.StatusCode, svc.ExpectedStatus),
				TLSDaysRemaining: tlsDays,
				ResolvedIPs:      resolvedStrs,
			}, tlsExp
		}
		return CheckResult{
			Status:           "up",
			StatusCode:       resp.StatusCode,
			LatencyMS:        latency,
			TLSDaysRemaining: tlsDays,
			ResolvedIPs:      resolvedStrs,
		}, tlsExp

	case "tcp":
		hostPort, ips, err := s.ssrf.ValidateHostPort(probeCtx, svc.Target, false)
		if err != nil {
			return CheckResult{Status: "down", LatencyMS: int(time.Since(start).Milliseconds()), FailureReason: err.Error()}, ""
		}
		_, port, _ := net.SplitHostPort(hostPort)
		dialer := &net.Dialer{Timeout: timeout}
		conn, err := dialer.DialContext(probeCtx, "tcp", net.JoinHostPort(ips[0].String(), port))
		latency := int(time.Since(start).Milliseconds())
		if err != nil {
			return CheckResult{Status: "down", LatencyMS: latency, FailureReason: err.Error(), ResolvedIPs: ipsToStrings(ips)}, ""
		}
		_ = conn.Close()
		return CheckResult{Status: "up", LatencyMS: latency, ResolvedIPs: ipsToStrings(ips)}, ""

	case "dns":
		ips, err := s.ssrf.ResolveAndValidateHost(probeCtx, svc.Target, false)
		latency := int(time.Since(start).Milliseconds())
		if err != nil {
			return CheckResult{Status: "down", LatencyMS: latency, FailureReason: err.Error()}, ""
		}
		return CheckResult{Status: "up", LatencyMS: latency, ResolvedIPs: ipsToStrings(ips)}, ""

	case "tls":
		hostPort, ips, err := s.ssrf.ValidateHostPort(probeCtx, svc.Target, false)
		if err != nil {
			return CheckResult{Status: "down", LatencyMS: int(time.Since(start).Milliseconds()), FailureReason: err.Error()}, ""
		}
		host, port, _ := net.SplitHostPort(hostPort)
		dialer := &tls.Dialer{
			NetDialer: &net.Dialer{Timeout: timeout},
			Config:    &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12},
		}
		conn, err := dialer.DialContext(probeCtx, "tcp", net.JoinHostPort(ips[0].String(), port))
		latency := int(time.Since(start).Milliseconds())
		if err != nil {
			return CheckResult{Status: "down", LatencyMS: latency, FailureReason: err.Error(), ResolvedIPs: ipsToStrings(ips)}, ""
		}
		defer conn.Close()
		tlsConn := conn.(*tls.Conn)
		certs := tlsConn.ConnectionState().PeerCertificates
		if len(certs) == 0 {
			return CheckResult{Status: "down", LatencyMS: latency, FailureReason: "no peer TLS certificate presented"}, ""
		}
		exp := certs[0].NotAfter.UTC()
		days := int(time.Until(exp).Hours() / 24)
		status := "up"
		reason := ""
		if days < 0 {
			status = "down"
			reason = "TLS certificate expired"
		}
		return CheckResult{
			Status:           status,
			LatencyMS:        latency,
			FailureReason:    reason,
			TLSDaysRemaining: &days,
			ResolvedIPs:      ipsToStrings(ips),
		}, exp.Format(time.RFC3339)

	default:
		return CheckResult{Status: "down", FailureReason: "unsupported check type"}, ""
	}
}

func ipsToStrings(ips []net.IP) []string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	return out
}
