package api_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/alerts"
	"github.com/dandisetiawanco/InfraOS/internal/api"
	"github.com/dandisetiawanco/InfraOS/internal/audit"
	"github.com/dandisetiawanco/InfraOS/internal/auth"
	"github.com/dandisetiawanco/InfraOS/internal/config"
	"github.com/dandisetiawanco/InfraOS/internal/containers"
	"github.com/dandisetiawanco/InfraOS/internal/database"
	"github.com/dandisetiawanco/InfraOS/internal/deployments"
	"github.com/dandisetiawanco/InfraOS/internal/hosts"
	"github.com/dandisetiawanco/InfraOS/internal/incidents"
	"github.com/dandisetiawanco/InfraOS/internal/integrations"
	"github.com/dandisetiawanco/InfraOS/internal/intelligence"
	"github.com/dandisetiawanco/InfraOS/internal/logs"
	"github.com/dandisetiawanco/InfraOS/internal/monitoring"
	"github.com/dandisetiawanco/InfraOS/internal/security"
	"github.com/dandisetiawanco/InfraOS/internal/sse"
	"github.com/dandisetiawanco/InfraOS/internal/topology"
	"github.com/dandisetiawanco/InfraOS/pkg/contracts"
	"github.com/dandisetiawanco/InfraOS/pkg/telemetry"
)

type testHarness struct {
	server    *httptest.Server
	db        *database.DB
	alertsEng *alerts.Engine
	monSched  *monitoring.Scheduler
	depSvc    *deployments.Service
	tmpDir    string
}

func newTestHarness(t *testing.T, localAllowlist []string) *testHarness {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	ctx := context.Background()

	db, err := database.Open(ctx, "sqlite", dbPath, "")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{
		Profile:                  "lightweight",
		DBDriver:                 "sqlite",
		SQLitePath:               dbPath,
		SessionTTL:               time.Hour,
		LoginRateLimitPerMin:     20,
		MonitoringLocalAllowlist: localAllowlist,
		MonitoringMaxConcurrency: 4,
		ExposeAPIDocs:            true,
		BackupDir:                filepath.Join(tmpDir, "backups"),
		SecretEncryptionKey:      "test-secret-key",
		AllowedOrigins:           []string{"http://localhost:3000"},
	}

	metrics := telemetry.New()
	broker := sse.NewBroker()
	auditRec := audit.NewRecorder(db)
	ssrf := security.NewSSRFValidator(localAllowlist)
	authSvc := auth.NewService(db, auditRec, cfg.SessionTTL, cfg.LoginRateLimitPerMin)
	logSvc := logs.NewService(db, broker)
	incSvc := incidents.NewService(db, auditRec, broker)
	alertEng := alerts.NewEngine(db, ssrf, incSvc, broker)
	hostSvc := hosts.NewService(db, auditRec, broker, metrics)
	monSched := monitoring.NewScheduler(db, ssrf, broker, metrics, logger, 4)
	cntSvc := containers.NewService(db, auditRec, false, "/nonexistent/docker.sock", false, nil)
	// Synchronous deployment execution (nil queue) keeps integration tests deterministic.
	depSvc := deployments.NewService(db, nil, ssrf, auditRec, logSvc, broker)
	intelSvc := intelligence.NewAnalyzer(db, "disabled", "", "", "")
	integSvc := integrations.NewService(db, auditRec, logSvc, cfg.SecretEncryptionKey)
	topoSvc := topology.NewService(db)

	srv := api.NewServer(api.Dependencies{
		Config: cfg, DB: db, Logger: logger, Metrics: metrics, Broker: broker, Audit: auditRec,
		Auth: authSvc, Hosts: hostSvc, Monitoring: monSched, Containers: cntSvc, Deployments: depSvc,
		Logs: logSvc, Alerts: alertEng, Incidents: incSvc, Intelligence: intelSvc, Integrations: integSvc,
		Topology: topoSvc, SSRF: ssrf,
	})
	ts := httptest.NewServer(srv.Router())
	t.Cleanup(func() {
		ts.Close()
		_ = db.Close()
	})
	return &testHarness{server: ts, db: db, alertsEng: alertEng, monSched: monSched, depSvc: depSvc, tmpDir: tmpDir}
}

func (h *testHarness) req(t *testing.T, method, path, bearer string, body any, extraHeaders map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, h.server.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var parsed map[string]any
	_ = json.Unmarshal(raw, &parsed)
	return resp, parsed
}

func TestAuthRBACAndOrganizationIsolation(t *testing.T) {
	h := newTestHarness(t, nil)

	// Register Org A owner
	resp, dataA := h.req(t, "POST", "/api/v1/auth/register", "", map[string]any{
		"name": "Alice Owner", "email": "alice@org-a.local", "password": "Password123!", "organization_name": "Org Alpha",
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register org A failed: %d %v", resp.StatusCode, dataA)
	}
	tokenA := dataA["data"].(map[string]any)["token"].(string)

	// Register Org B owner
	resp, dataB := h.req(t, "POST", "/api/v1/auth/register", "", map[string]any{
		"name": "Bob Owner", "email": "bob@org-b.local", "password": "Password123!", "organization_name": "Org Beta",
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register org B failed: %d", resp.StatusCode)
	}
	tokenB := dataB["data"].(map[string]any)["token"].(string)

	// Org A adds a Viewer member
	resp, _ = h.req(t, "POST", "/api/v1/team", "Bearer "+tokenA, map[string]any{
		"email": "viewer@org-a.local", "name": "Victor Viewer", "password": "Password123!", "role": "viewer",
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("invite viewer failed: %d", resp.StatusCode)
	}

	// Viewer logs in
	resp, viewerData := h.req(t, "POST", "/api/v1/auth/login", "", map[string]any{
		"email": "viewer@org-a.local", "password": "Password123!",
	}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("viewer login failed: %d", resp.StatusCode)
	}
	viewerToken := viewerData["data"].(map[string]any)["token"].(string)

	// Viewer must be blocked from mutating infrastructure (RBAC enforcement server-side).
	resp, _ = h.req(t, "POST", "/api/v1/agents/enrollment-tokens", "Bearer "+viewerToken, map[string]any{"label": "unauthorized"}, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer creating enrollment token, got %d", resp.StatusCode)
	}
	resp, _ = h.req(t, "POST", "/api/v1/incidents", "Bearer "+viewerToken, map[string]any{"title": "test"}, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for viewer creating incident, got %d", resp.StatusCode)
	}

	// Org A creates an incident; Org B must NOT be able to read it.
	resp, incData := h.req(t, "POST", "/api/v1/incidents", "Bearer "+tokenA, map[string]any{
		"title": "Org A Private Incident", "severity": "sev2",
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("org A create incident failed: %d", resp.StatusCode)
	}
	incID := incData["data"].(map[string]any)["id"].(string)

	resp, _ = h.req(t, "GET", "/api/v1/incidents/"+incID, "Bearer "+tokenB, nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-org incident access, got %d", resp.StatusCode)
	}

	// Logout revokes session immediately.
	resp, _ = h.req(t, "POST", "/api/v1/auth/logout", "Bearer "+viewerToken, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout failed: %d", resp.StatusCode)
	}
	resp, _ = h.req(t, "GET", "/api/v1/auth/me", "Bearer "+viewerToken, nil, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after session revocation, got %d", resp.StatusCode)
	}
}

func TestAgentEnrollmentMetricsIngestionAndRevocation(t *testing.T) {
	h := newTestHarness(t, nil)
	_, reg := h.req(t, "POST", "/api/v1/auth/register", "", map[string]any{
		"name": "Ops Admin", "email": "ops@infraos.local", "password": "Password123!", "organization_name": "Ops Org",
	}, nil)
	token := reg["data"].(map[string]any)["token"].(string)

	// Issue one-time enrollment token
	resp, tokRes := h.req(t, "POST", "/api/v1/agents/enrollment-tokens", "Bearer "+token, map[string]any{"label": "mac-dev"}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("enrollment token failed: %d", resp.StatusCode)
	}
	enrollToken := tokRes["data"].(map[string]any)["token"].(string)

	// Enroll agent
	resp, enrollRes := h.req(t, "POST", "/api/v1/agents/enroll", "", contracts.AgentEnrollRequest{
		EnrollmentToken: enrollToken, Hostname: "macbook-dev", OS: "darwin", Arch: "amd64", IPAddress: "127.0.0.1", AgentVersion: "0.1.0",
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("agent enroll failed: %d %v", resp.StatusCode, enrollRes)
	}
	enrollData := enrollRes["data"].(map[string]any)
	agentToken := enrollData["agent_token"].(string)
	agentID := enrollData["agent_id"].(string)
	hostID := enrollData["host_id"].(string)

	// Reusing one-time enrollment token must fail
	resp, _ = h.req(t, "POST", "/api/v1/agents/enroll", "", contracts.AgentEnrollRequest{
		EnrollmentToken: enrollToken, Hostname: "attacker-host", OS: "linux", Arch: "amd64",
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 on enrollment token reuse, got %d", resp.StatusCode)
	}

	// Ingest metrics with Agent auth
	resp, _ = h.req(t, "POST", "/api/v1/agents/metrics", "Agent "+agentToken, contracts.HostMetricPayload{
		RecordedAt: time.Now().UTC(), Hostname: "macbook-dev", OS: "darwin", Arch: "amd64", AgentVersion: "0.1.0",
		CPUPercent: 27.5, CPUCores: 8, MemTotalBytes: 8589934592, MemUsedBytes: 4294967296, MemAvailBytes: 4294967296, MemPercent: 50.0,
		DiskTotalBytes: 250000000000, DiskUsedBytes: 100000000000, DiskPercent: 40.0,
	}, nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("metrics ingest failed: %d", resp.StatusCode)
	}

	// Verify host detail reflects ingested metric
	resp, hostRes := h.req(t, "GET", "/api/v1/hosts/"+hostID, "Bearer "+token, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get host failed: %d", resp.StatusCode)
	}
	hostObj := hostRes["data"].(map[string]any)
	if hostObj["freshness"] != "live" {
		t.Fatalf("expected live freshness, got %v", hostObj["freshness"])
	}

	// Revoke agent -> subsequent metrics ingestion must fail
	resp, _ = h.req(t, "POST", "/api/v1/agents/"+agentID+"/revoke", "Bearer "+token, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revoke agent failed: %d", resp.StatusCode)
	}
	resp, _ = h.req(t, "POST", "/api/v1/agents/metrics", "Agent "+agentToken, contracts.HostMetricPayload{
		Hostname: "macbook-dev", CPUPercent: 10, CPUCores: 8,
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for revoked agent, got %d", resp.StatusCode)
	}
}

func TestServiceMonitoringAndSSRFProtection(t *testing.T) {
	probeTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer probeTarget.Close()

	h := newTestHarness(t, []string{probeTarget.URL})
	_, reg := h.req(t, "POST", "/api/v1/auth/register", "", map[string]any{
		"name": "SRE", "email": "sre@infraos.local", "password": "Password123!", "organization_name": "SRE Org",
	}, nil)
	token := reg["data"].(map[string]any)["token"].(string)

	// Blocked SSRF metadata endpoint
	resp, _ := h.req(t, "POST", "/api/v1/services", "Bearer "+token, map[string]any{
		"name": "Metadata Probe", "check_type": "http", "target": "http://169.254.169.254/latest/meta-data/",
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected SSRF target to be rejected with 400, got %d", resp.StatusCode)
	}

	// Allowed local test service
	resp, svcRes := h.req(t, "POST", "/api/v1/services", "Bearer "+token, map[string]any{
		"name": "Local Probe", "check_type": "http", "target": probeTarget.URL + "/healthz", "expected_status": 200,
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create service failed: %d %v", resp.StatusCode, svcRes)
	}
	svcID := svcRes["data"].(map[string]any)["id"].(string)
	if svcRes["data"].(map[string]any)["uptime_percent"] != nil {
		t.Fatal("uptime must be nil before any check observations occur")
	}

	// Execute real check
	resp, chkRes := h.req(t, "POST", "/api/v1/services/"+svcID+"/check-now", "Bearer "+token, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("check-now failed: %d %v", resp.StatusCode, chkRes)
	}
	if chkRes["data"].(map[string]any)["status"] != "up" {
		t.Fatalf("expected check status up, got %v", chkRes["data"])
	}

	// Uptime should now be 100 based on 1 recorded observation
	_, getRes := h.req(t, "GET", "/api/v1/services/"+svcID, "Bearer "+token, nil, nil)
	svcDetail := getRes["data"].(map[string]any)["service"].(map[string]any)
	if svcDetail["uptime_percent"].(float64) != 100 {
		t.Fatalf("expected 100%% observed uptime, got %v", svcDetail["uptime_percent"])
	}
}

func TestDeploymentsIdempotencyLockingAndRollback(t *testing.T) {
	healthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthSrv.Close()

	h := newTestHarness(t, []string{healthSrv.URL})
	_, reg := h.req(t, "POST", "/api/v1/auth/register", "", map[string]any{
		"name": "Deployer", "email": "dep@infraos.local", "password": "Password123!", "organization_name": "Dep Org",
	}, nil)
	token := reg["data"].(map[string]any)["token"].(string)

	// Create target
	resp, tRes := h.req(t, "POST", "/api/v1/deployment-targets", "Bearer "+token, map[string]any{
		"name": "Sample Service", "adapter": "local_http", "health_url": healthSrv.URL + "/healthz",
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create target failed: %d %v", resp.StatusCode, tRes)
	}
	targetID := tRes["data"].(map[string]any)["id"].(string)

	// Release v1.0.0 with idempotency key
	resp, d1Res := h.req(t, "POST", "/api/v1/deployments", "Bearer "+token, map[string]any{
		"target_id": targetID, "version": "v1.0.0", "idempotency_key": "rel-100",
	}, nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("deploy v1.0.0 failed: %d %v", resp.StatusCode, d1Res)
	}
	d1 := d1Res["data"].(map[string]any)
	if d1["status"] != "succeeded" {
		t.Fatalf("expected v1.0.0 succeeded, got %v", d1["status"])
	}

	// Duplicate idempotency key returns same deployment ID
	_, d1Dup := h.req(t, "POST", "/api/v1/deployments", "Bearer "+token, map[string]any{
		"target_id": targetID, "version": "v1.0.0", "idempotency_key": "rel-100",
	}, nil)
	if d1Dup["data"].(map[string]any)["id"] != d1["id"] {
		t.Fatal("idempotency key did not deduplicate deployment")
	}

	// Release v1.1.0 then roll back to v1.0.0
	_, d2Res := h.req(t, "POST", "/api/v1/deployments", "Bearer "+token, map[string]any{
		"target_id": targetID, "version": "v1.1.0",
	}, nil)
	d2ID := d2Res["data"].(map[string]any)["id"].(string)

	resp, rbRes := h.req(t, "POST", "/api/v1/deployments/"+d2ID+"/rollback", "Bearer "+token, nil, nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("rollback failed: %d %v", resp.StatusCode, rbRes)
	}
	rb := rbRes["data"].(map[string]any)
	if rb["status"] != "rolled_back" || rb["version"] != "v1.0.0" {
		t.Fatalf("expected rolled_back to v1.0.0, got status=%v version=%v", rb["status"], rb["version"])
	}

	// Simulate active target lock -> concurrent deploy must return 409 Conflict
	_, _ = h.db.Exec(context.Background(), `UPDATE deployment_targets SET locked_by_deployment_id = 'lock-test' WHERE id = ?`, targetID)
	resp, _ = h.req(t, "POST", "/api/v1/deployments", "Bearer "+token, map[string]any{
		"target_id": targetID, "version": "v1.2.0",
	}, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 Conflict when target is locked, got %d", resp.StatusCode)
	}
}

func TestAlertStateTransitionsWebhookAndBackupRetention(t *testing.T) {
	h := newTestHarness(t, nil)
	_, reg := h.req(t, "POST", "/api/v1/auth/register", "", map[string]any{
		"name": "Alert Admin", "email": "alert@infraos.local", "password": "Password123!", "organization_name": "Alert Org",
	}, nil)
	token := reg["data"].(map[string]any)["token"].(string)
	orgID := reg["data"].(map[string]any)["organization"].(map[string]any)["id"].(string)

	// Create alert rule with auto_incident=true and duration=0
	resp, ruleRes := h.req(t, "POST", "/api/v1/alerts/rules", "Bearer "+token, map[string]any{
		"name": "CPU Spike", "rule_type": "high_cpu", "threshold": 80, "duration_seconds": 0, "cooldown_seconds": 300, "severity": "critical", "auto_incident": true,
	}, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create alert rule failed: %d %v", resp.StatusCode, ruleRes)
	}

	// Trigger firing -> creates alert event + auto incident
	if err := h.alertsEng.EvaluateObservation(context.Background(), orgID, "high_cpu", "host-1", "", "", "CPU at 91%", 91, false); err != nil {
		t.Fatalf("evaluate firing failed: %v", err)
	}
	_, evRes := h.req(t, "GET", "/api/v1/alerts/events", "Bearer "+token, nil, nil)
	events := evRes["data"].([]any)
	if len(events) != 1 || events[0].(map[string]any)["state"] != "firing" {
		t.Fatalf("expected 1 firing alert event, got %v", events)
	}
	_, incRes := h.req(t, "GET", "/api/v1/incidents", "Bearer "+token, nil, nil)
	if len(incRes["data"].([]any)) != 1 {
		t.Fatalf("expected 1 auto-created incident, got %d", len(incRes["data"].([]any)))
	}

	// Recovery resolves alert
	if err := h.alertsEng.EvaluateObservation(context.Background(), orgID, "high_cpu", "host-1", "", "", "CPU normalized 30%", 30, false); err != nil {
		t.Fatalf("evaluate recovery failed: %v", err)
	}
	_, evRes2 := h.req(t, "GET", "/api/v1/alerts/events", "Bearer "+token, nil, nil)
	if evRes2["data"].([]any)[0].(map[string]any)["state"] != "resolved" {
		t.Fatal("expected alert event to resolve upon recovery")
	}

	// GitHub webhook HMAC verification + duplicate delivery deduplication
	_, _ = h.req(t, "PUT", "/api/v1/integrations/github", "Bearer "+token, map[string]any{
		"config": map[string]any{"repository": "dandisetiawanco/InfraOS", "webhook_secret": "hook_secret_123"},
	}, nil)
	payload := []byte(`{"workflow_run":{"name":"CI"}}`)
	mac := hmac.New(sha256.New, []byte("hook_secret_123"))
	mac.Write(payload)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	rawReq, _ := http.NewRequest("POST", h.server.URL+"/api/v1/webhooks/github/"+orgID, bytes.NewReader(payload))
	rawReq.Header.Set("X-GitHub-Delivery", "deliv-001")
	rawReq.Header.Set("X-GitHub-Event", "workflow_run")
	rawReq.Header.Set("X-Hub-Signature-256", sig)
	wResp, err := http.DefaultClient.Do(rawReq)
	if err != nil || wResp.StatusCode != http.StatusOK {
		t.Fatalf("valid github webhook failed: err=%v status=%d", err, wResp.StatusCode)
	}
	wResp.Body.Close()

	// Duplicate delivery returns duplicate=true
	rawReq2, _ := http.NewRequest("POST", h.server.URL+"/api/v1/webhooks/github/"+orgID, bytes.NewReader(payload))
	rawReq2.Header.Set("X-GitHub-Delivery", "deliv-001")
	rawReq2.Header.Set("X-GitHub-Event", "workflow_run")
	rawReq2.Header.Set("X-Hub-Signature-256", sig)
	wResp2, _ := http.DefaultClient.Do(rawReq2)
	var dupBody map[string]any
	_ = json.NewDecoder(wResp2.Body).Decode(&dupBody)
	wResp2.Body.Close()
	if dupBody["data"].(map[string]any)["duplicate"] != true {
		t.Fatalf("expected duplicate=true on repeated delivery ID, got %v", dupBody)
	}

	// Invalid signature must be rejected
	badReq, _ := http.NewRequest("POST", h.server.URL+"/api/v1/webhooks/github/"+orgID, bytes.NewReader(payload))
	badReq.Header.Set("X-GitHub-Delivery", "deliv-002")
	badReq.Header.Set("X-GitHub-Event", "workflow_run")
	badReq.Header.Set("X-Hub-Signature-256", "sha256=badbad")
	badResp, _ := http.DefaultClient.Do(badReq)
	badResp.Body.Close()
	if badResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 on invalid webhook signature, got %d", badResp.StatusCode)
	}

	// SQLite VACUUM INTO backup verification
	resp, bRes := h.req(t, "POST", "/api/v1/backups", "Bearer "+token, nil, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("sqlite backup failed: %d %v", resp.StatusCode, bRes)
	}
	if bRes["data"].(map[string]any)["sha256"] == "" {
		t.Fatal("expected backup sha256 checksum")
	}

	// Public Demo Isolation: demo user cannot enroll hosts or run external probes
	resp, demoRes := h.req(t, "POST", "/api/v1/auth/demo", "", nil, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("demo launch failed: %d", resp.StatusCode)
	}
	demoToken := demoRes["data"].(map[string]any)["token"].(string)
	resp, _ = h.req(t, "POST", "/api/v1/services", "Bearer "+demoToken, map[string]any{
		"name": "External Attack", "check_type": "http", "target": "https://example.com",
	}, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected demo service probe creation to be blocked, got %d", resp.StatusCode)
	}
	resp, cntRes := h.req(t, "GET", "/api/v1/containers", "Bearer "+demoToken, nil, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(cntRes["data"].(map[string]any)["integration"].(map[string]any)["mode"].(string), "synthetic") {
		t.Fatalf("expected synthetic_demo container mode for demo org, got %v", cntRes)
	}
}
