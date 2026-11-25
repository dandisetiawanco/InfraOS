package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/auth"
	"github.com/dandisetiawanco/InfraOS/internal/database"
	"github.com/dandisetiawanco/InfraOS/internal/security"
	"github.com/google/uuid"
)

// CreateIsolatedDemoSession provisions a dedicated synthetic demonstration organization and session.
func (s *Server) CreateIsolatedDemoSession(ctx context.Context, ipAddr, userAgent string) (*auth.SessionContext, string, error) {
	shortID := uuid.NewString()[:8]
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	userID := uuid.NewString()
	orgID := uuid.NewString()
	projectID := uuid.NewString()
	envProdID := uuid.NewString()
	envStgID := uuid.NewString()
	envDevID := uuid.NewString()

	pwHash, err := security.HashPassword(uuid.NewString() + "Demo1!")
	if err != nil {
		return nil, "", err
	}

	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		email := fmt.Sprintf("demo-%s@demo.infraos.local", shortID)
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`),
			userID, email, "Demo Operator", pwHash, nowStr, nowStr); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO organizations (id, name, slug, is_demo, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`),
			orgID, "InfraOS Sandbox (Synthetic Demo)", "demo-"+shortID, database.BoolToDB(s.db.Driver, true), nowStr, nowStr); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO organization_memberships (id, organization_id, user_id, role, created_at)
			VALUES (?, ?, ?, 'operator', ?)`),
			uuid.NewString(), orgID, userID, nowStr); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO projects (id, organization_id, name, slug, description, repository_url, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
			projectID, orgID, "Edge Platform (Synthetic)", "edge-platform-demo",
			"Synthetic demonstration project showcasing multi-environment operations.",
			"https://github.com/dandisetiawanco/InfraOS", nowStr, nowStr); err != nil {
			return err
		}

		envs := []struct{ id, name, slug, tier, desc string }{
			{envProdID, "Production (Demo)", "production", "production", "Synthetic production topology."},
			{envStgID, "Staging (Demo)", "staging", "staging", "Synthetic staging verification cluster."},
			{envDevID, "Development (Demo)", "development", "development", "Synthetic development workstation tier."},
		}
		for _, e := range envs {
			if _, err := tx.ExecContext(ctx, s.db.Rebind(`
				INSERT INTO environments (id, organization_id, project_id, name, slug, tier, description, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
				e.id, orgID, projectID, e.name, e.slug, e.tier, e.desc, nowStr); err != nil {
				return err
			}
		}

		// Synthetic Hosts & Agents
		hostAPI := uuid.NewString()
		hostWorker := uuid.NewString()
		hostDB := uuid.NewString()
		hostStg := uuid.NewString()

		demoHosts := []struct {
			id, envID, hostname, display, osName, arch, kernel, ip, status string
			tags                                                           []string
			cpuBase, memBase, diskBase                                     float64
			cores                                                          int
			memGB, diskGB                                                  uint64
		}{
			{hostAPI, envProdID, "edge-api-01.demo.internal", "edge-api-01 (Demo)", "linux", "amd64", "6.8.0-45-generic", "10.24.0.11", "online", []string{"demo", "api", "eu-west"}, 42, 64, 58, 4, 16, 160},
			{hostWorker, envProdID, "worker-pool-02.demo.internal", "worker-pool-02 (Demo)", "linux", "amd64", "6.8.0-45-generic", "10.24.0.14", "online", []string{"demo", "worker", "queue"}, 68, 84, 71, 8, 32, 240},
			{hostDB, envProdID, "pg-primary-01.demo.internal", "pg-primary-01 (Demo)", "linux", "arm64", "6.8.0-45-generic", "10.24.1.10", "online", []string{"demo", "postgres", "primary"}, 31, 76, 82, 8, 32, 500},
			{hostStg, envStgID, "stg-app-01.demo.internal", "stg-app-01 (Demo)", "linux", "amd64", "6.8.0-45-generic", "10.25.0.20", "stale", []string{"demo", "staging"}, 18, 48, 44, 2, 8, 80},
		}

		for idx, h := range demoHosts {
			tagsJSON, _ := json.Marshal(h.tags)
			hbTime := now.Add(-12 * time.Second).Format(time.RFC3339)
			if h.status == "stale" {
				hbTime = now.Add(-4 * time.Minute).Format(time.RFC3339)
			}
			if _, err := tx.ExecContext(ctx, s.db.Rebind(`
				INSERT INTO hosts (id, organization_id, environment_id, hostname, display_name, os, arch, kernel_version, ip_address, tags_json, status, is_synthetic, metadata_json, last_heartbeat_at, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '{"source":"synthetic_demo","platform":"ubuntu","platform_version":"24.04"}', ?, ?, ?)`),
				h.id, orgID, h.envID, h.hostname, h.display, h.osName, h.arch, h.kernel, h.ip, string(tagsJSON), h.status, database.BoolToDB(s.db.Driver, true), hbTime, nowStr, nowStr); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, s.db.Rebind(`
				INSERT INTO agents (id, organization_id, host_id, version, status, last_heartbeat_at, last_ip, created_at, updated_at)
				VALUES (?, ?, ?, '0.1.0', 'active', ?, ?, ?, ?)`),
				uuid.NewString(), orgID, h.id, hbTime, h.ip, nowStr, nowStr); err != nil {
				return err
			}

			// Generate 36 deterministic historical points spanning 3 hours (no randomness).
			fsJSON, _ := json.Marshal([]map[string]any{
				{"mount_point": "/", "device": "/dev/nvme0n1p1", "fs_type": "ext4", "total_bytes": h.diskGB * 1024 * 1024 * 1024, "used_bytes": uint64(float64(h.diskGB) * (h.diskBase / 100.0) * 1024 * 1024 * 1024), "free_bytes": uint64(float64(h.diskGB) * (1 - h.diskBase/100.0) * 1024 * 1024 * 1024), "used_percent": h.diskBase},
			})
			ifJSON, _ := json.Marshal([]map[string]any{
				{"name": "eth0", "addresses": []string{h.ip + "/24"}, "rx_bytes": uint64(820000000 + idx*150000000), "tx_bytes": uint64(640000000 + idx*120000000), "rx_bps": 420000.0, "tx_bps": 310000.0},
			})
			for p := 35; p >= 0; p-- {
				ts := now.Add(-time.Duration(p*5) * time.Minute).Format(time.RFC3339)
				wave := math.Sin(float64(p+idx)*0.45) * 6.5
				cpuVal := math.Max(5, math.Min(96, h.cpuBase+wave))
				memVal := math.Max(15, math.Min(95, h.memBase+wave*0.45))
				memTotal := h.memGB * 1024 * 1024 * 1024
				memUsed := uint64(float64(memTotal) * (memVal / 100.0))
				diskTotal := h.diskGB * 1024 * 1024 * 1024
				diskUsed := uint64(float64(diskTotal) * (h.diskBase / 100.0))
				rxBps := 280000 + math.Abs(wave)*45000
				txBps := 210000 + math.Abs(wave)*32000
				if _, err := tx.ExecContext(ctx, s.db.Rebind(`
					INSERT INTO host_metrics (
						organization_id, host_id, recorded_at, cpu_percent, cpu_cores, load_1m, load_5m, load_15m,
						mem_total_bytes, mem_used_bytes, mem_available_bytes, mem_percent,
						disk_total_bytes, disk_used_bytes, disk_percent, net_rx_bytes, net_tx_bytes, net_rx_bps, net_tx_bps,
						uptime_seconds, filesystems_json, interfaces_json, optional_json, is_synthetic
					) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '{}', ?)`),
					orgID, h.id, ts, math.Round(cpuVal*100)/100, h.cores, math.Round(cpuVal/35*100)/100, math.Round(cpuVal/38*100)/100, math.Round(cpuVal/42*100)/100,
					memTotal, memUsed, memTotal-memUsed, math.Round(memVal*100)/100,
					diskTotal, diskUsed, h.diskBase, uint64(820000000+(36-p)*900000), uint64(640000000+(36-p)*700000), rxBps, txBps,
					uint64(1420000+(36-p)*300), string(fsJSON), string(ifJSON), database.BoolToDB(s.db.Driver, true),
				); err != nil {
					return err
				}
			}
		}

		// Synthetic Services & Checks
		svcGateway := uuid.NewString()
		svcWorkerHTTP := uuid.NewString()
		svcTLS := uuid.NewString()
		svcDNS := uuid.NewString()

		demoServices := []struct {
			id, envID, hostID, name, checkType, target, status, lastErr string
			latency, fails, succs                                       int
			tlsDays                                                     int
		}{
			{svcGateway, envProdID, hostAPI, "Edge API Gateway (Demo)", "https", "https://api.demo.infraos.internal/healthz", "healthy", "", 34, 0, 24, 62},
			{svcWorkerHTTP, envProdID, hostWorker, "Job Queue Processor (Demo)", "http", "http://worker.demo.infraos.internal:8085/ready", "degraded", "elevated latency > 400ms", 468, 1, 0, 0},
			{svcTLS, envProdID, hostAPI, "Public Edge TLS Certificate (Demo)", "tls", "edge.demo.infraos.internal:443", "healthy", "", 22, 0, 18, 14},
			{svcDNS, envStgID, hostStg, "Staging DNS Resolver (Demo)", "dns", "staging.demo.infraos.internal", "down", "simulated upstream NXDOMAIN timeout", 0, 3, 0, 0},
		}

		for _, sv := range demoServices {
			var tlsExpArg any
			if sv.tlsDays > 0 {
				tlsExpArg = now.AddDate(0, 0, sv.tlsDays).Format(time.RFC3339)
			}
			if _, err := tx.ExecContext(ctx, s.db.Rebind(`
				INSERT INTO services (
					id, organization_id, project_id, environment_id, host_id, name, check_type, target,
					interval_seconds, timeout_ms, expected_status, failure_threshold, recovery_threshold,
					enabled, status, consecutive_failures, consecutive_successes, last_checked_at,
					last_latency_ms, last_error, tls_expires_at, is_synthetic, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 30, 5000, 200, 2, 2, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
				sv.id, orgID, projectID, sv.envID, sv.hostID, sv.name, sv.checkType, sv.target,
				database.BoolToDB(s.db.Driver, true), sv.status, sv.fails, sv.succs, now.Add(-18*time.Second).Format(time.RFC3339),
				sv.latency, sv.lastErr, tlsExpArg, database.BoolToDB(s.db.Driver, true), nowStr, nowStr); err != nil {
				return err
			}

			for c := 23; c >= 0; c-- {
				cTime := now.Add(-time.Duration(c*4) * time.Minute).Format(time.RFC3339)
				cStatus := "up"
				code := 200
				lat := sv.latency
				reason := ""
				if sv.id == svcWorkerHTTP && c < 4 {
					lat = 360 + (4-c)*35
				} else if sv.id == svcWorkerHTTP {
					lat = 82 + (c%4)*7
				} else if sv.id == svcGateway {
					lat = 29 + (c%5)*4
				} else if sv.id == svcDNS && c < 3 {
					cStatus = "down"
					code = 0
					lat = 0
					reason = sv.lastErr
				} else if sv.id == svcDNS {
					lat = 14 + (c%3)*3
				}
				var tlsArg any
				if sv.tlsDays > 0 {
					tlsArg = sv.tlsDays
				}
				if _, err := tx.ExecContext(ctx, s.db.Rebind(`
					INSERT INTO service_checks (organization_id, service_id, checked_at, status, status_code, latency_ms, failure_reason, tls_days_remaining, resolved_ips_json, is_synthetic)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, '["10.24.0.11"]', ?)`),
					orgID, sv.id, cTime, cStatus, code, lat, reason, tlsArg, database.BoolToDB(s.db.Driver, true)); err != nil {
					return err
				}
			}
		}

		// Synthetic Containers
		demoContainers := []struct {
			extID, hostID, envID, name, image, state, health string
			cpu                                              float64
			memMB, limitMB                                   uint64
			restarts                                         int
		}{
			{"demo-cnt-nginx-01", hostAPI, envProdID, "infraos-edge-proxy", "nginx:1.27-alpine", "running", "healthy", 3.4, 96, 512, 0},
			{"demo-cnt-api-01", hostAPI, envProdID, "edge-api-server", "ghcr.io/demo/edge-api:v2.14.2", "running", "healthy", 24.8, 384, 1024, 0},
			{"demo-cnt-worker-01", hostWorker, envProdID, "queue-worker", "ghcr.io/demo/worker:v2.14.2", "running", "healthy", 48.2, 742, 1024, 2},
			{"demo-cnt-pg-01", hostDB, envProdID, "postgres-primary", "postgres:16-alpine", "running", "healthy", 19.6, 1840, 4096, 0},
		}
		for _, cnt := range demoContainers {
			if _, err := tx.ExecContext(ctx, s.db.Rebind(`
				INSERT INTO containers (id, organization_id, host_id, environment_id, external_id, name, image, state, health_status, cpu_percent, mem_usage_bytes, mem_limit_bytes, networks_json, restart_count, container_created_at, allow_mutations, is_synthetic, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '["infraos-net"]', ?, ?, ?, ?, ?)`),
				uuid.NewString(), orgID, cnt.hostID, cnt.envID, cnt.extID, cnt.name, cnt.image, cnt.state, cnt.health,
				cnt.cpu, cnt.memMB*1024*1024, cnt.limitMB*1024*1024, cnt.restarts, now.Add(-96*time.Hour).Format(time.RFC3339),
				database.BoolToDB(s.db.Driver, false), database.BoolToDB(s.db.Driver, true), nowStr); err != nil {
				return err
			}
		}

		// Synthetic Deployment Targets, Artifacts, and Deployments
		targetProd := uuid.NewString()
		targetStg := uuid.NewString()
		artPrev := uuid.NewString()
		artCurr := uuid.NewString()
		depPrev := uuid.NewString()
		depCurr := uuid.NewString()

		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO deployment_targets (id, organization_id, project_id, environment_id, host_id, name, adapter, health_url, current_version, current_artifact_id, is_synthetic, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 'Edge API Production (Simulation)', 'simulation', 'https://api.demo.infraos.internal/healthz', 'v2.14.2', ?, ?, ?, ?)`),
			targetProd, orgID, projectID, envProdID, hostAPI, artCurr, database.BoolToDB(s.db.Driver, true), nowStr, nowStr); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO deployment_targets (id, organization_id, project_id, environment_id, host_id, name, adapter, health_url, current_version, is_synthetic, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 'Staging Preview App (Simulation)', 'simulation', 'https://stg.demo.infraos.internal/healthz', 'v2.15.0-rc1', ?, ?, ?)`),
			targetStg, orgID, projectID, envStgID, hostStg, database.BoolToDB(s.db.Driver, true), nowStr, nowStr); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO deployment_artifacts (id, organization_id, target_id, version, commit_sha, commit_message, branch, image_ref, checksum, build_metadata_json, created_at)
			VALUES (?, ?, ?, 'v2.14.1', '9a4f2c1b8e', 'Tighten HTTP client timeout defaults', 'main', 'ghcr.io/demo/edge-api:v2.14.1', 'sha256:9a4f2c1b8e7710a2', '{}', ?)`),
			artPrev, orgID, targetProd, now.Add(-6*time.Hour).Format(time.RFC3339)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO deployment_artifacts (id, organization_id, target_id, version, commit_sha, commit_message, branch, image_ref, checksum, build_metadata_json, created_at)
			VALUES (?, ?, ?, 'v2.14.2', 'c7d31e50a4', 'Add SSE reconnect jitter and worker pool metrics', 'main', 'ghcr.io/demo/edge-api:v2.14.2', 'sha256:c7d31e50a499b120', '{}', ?)`),
			artCurr, orgID, targetProd, now.Add(-38*time.Minute).Format(time.RFC3339)); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO deployments (id, organization_id, target_id, artifact_id, environment_id, version, commit_sha, commit_message, branch, status, trigger_type, is_simulation, triggered_by, started_at, finished_at, created_at)
			VALUES (?, ?, ?, ?, ?, 'v2.14.1', '9a4f2c1b8e', 'Tighten HTTP client timeout defaults', 'main', 'succeeded', 'demo', ?, ?, ?, ?, ?)`),
			depPrev, orgID, targetProd, artPrev, envProdID, database.BoolToDB(s.db.Driver, true), userID,
			now.Add(-6*time.Hour).Format(time.RFC3339), now.Add(-5*time.Hour-58*time.Minute).Format(time.RFC3339), now.Add(-6*time.Hour).Format(time.RFC3339)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO deployments (id, organization_id, target_id, artifact_id, previous_artifact_id, environment_id, version, commit_sha, commit_message, branch, status, trigger_type, is_simulation, triggered_by, started_at, finished_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 'v2.14.2', 'c7d31e50a4', 'Add SSE reconnect jitter and worker pool metrics', 'main', 'succeeded', 'demo', ?, ?, ?, ?, ?)`),
			depCurr, orgID, targetProd, artCurr, artPrev, envProdID, database.BoolToDB(s.db.Driver, true), userID,
			now.Add(-38*time.Minute).Format(time.RFC3339), now.Add(-37*time.Minute).Format(time.RFC3339), now.Add(-38*time.Minute).Format(time.RFC3339)); err != nil {
			return err
		}
		for _, ev := range []struct{ state, msg string }{
			{"queued", "Simulation deployment v2.14.2 queued for Edge API Production."},
			{"preparing", "Validating artifact digest sha256:c7d31e50a499b120."},
			{"deploying", "Rolling update applied in simulation mode."},
			{"verifying", "Synthetic health verification passed (200 OK)."},
			{"succeeded", "Simulation deployment v2.14.2 completed."},
		} {
			_, _ = tx.ExecContext(ctx, s.db.Rebind(`
				INSERT INTO deployment_events (organization_id, deployment_id, state, level, message, details_json, created_at)
				VALUES (?, ?, ?, 'info', ?, '{}', ?)`),
				orgID, depCurr, ev.state, ev.msg, now.Add(-37*time.Minute).Format(time.RFC3339))
		}

		// Synthetic Alert Rules, Events & Incident
		ruleMem := uuid.NewString()
		ruleDNS := uuid.NewString()
		incID := uuid.NewString()

		_, _ = tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO alert_rules (id, organization_id, environment_id, name, rule_type, target_ref, threshold, duration_seconds, cooldown_seconds, severity, auto_incident, enabled, state, last_fired_at, created_at, updated_at)
			VALUES (?, ?, ?, 'High Worker Memory Utilization', 'high_memory', '*', 80, 60, 300, 'warning', ?, ?, 'firing', ?, ?, ?)`),
			ruleMem, orgID, envProdID, database.BoolToDB(s.db.Driver, false), database.BoolToDB(s.db.Driver, true), now.Add(-14*time.Minute).Format(time.RFC3339), nowStr, nowStr)
		_, _ = tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO alert_rules (id, organization_id, environment_id, name, rule_type, target_ref, threshold, duration_seconds, cooldown_seconds, severity, auto_incident, enabled, state, last_fired_at, created_at, updated_at)
			VALUES (?, ?, ?, 'Staging DNS Availability', 'service_down', ?, 1, 30, 300, 'critical', ?, ?, 'firing', ?, ?, ?)`),
			ruleDNS, orgID, envStgID, svcDNS, database.BoolToDB(s.db.Driver, true), database.BoolToDB(s.db.Driver, true), now.Add(-9*time.Minute).Format(time.RFC3339), nowStr, nowStr)

		_, _ = tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO incidents (id, organization_id, environment_id, title, description, severity, status, affected_service_ids_json, assigned_user_id, root_cause_notes, is_synthetic, opened_at, created_at, updated_at)
			VALUES (?, ?, ?, 'Staging DNS Resolver check timeouts (Synthetic Demo)', 'Automated monitoring observed three consecutive failed DNS checks on staging.demo.infraos.internal.', 'sev2', 'investigating', ?, ?, 'Checking synthetic upstream resolver configuration.', ?, ?, ?, ?)`),
			incID, orgID, envStgID, fmt.Sprintf(`["%s"]`, svcDNS), userID, database.BoolToDB(s.db.Driver, true), now.Add(-9*time.Minute).Format(time.RFC3339), nowStr, nowStr)

		_, _ = tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO incident_timelines (organization_id, incident_id, event_type, author_id, message, metadata_json, created_at)
			VALUES (?, ?, 'status_change', ?, 'Incident opened automatically from Staging DNS Availability alert rule.', '{}', ?)`),
			orgID, incID, userID, now.Add(-9*time.Minute).Format(time.RFC3339))
		_, _ = tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO incident_timelines (organization_id, incident_id, event_type, author_id, message, metadata_json, created_at)
			VALUES (?, ?, 'note', ?, 'Correlated with recent staging deployment window; checking whether resolver config changed.', '{}', ?)`),
			orgID, incID, userID, now.Add(-5*time.Minute).Format(time.RFC3339))

		_, _ = tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO alert_events (id, organization_id, rule_id, host_id, state, severity, summary, metric_value, threshold, fingerprint, triggered_at, is_synthetic)
			VALUES (?, ?, ?, ?, 'firing', 'warning', 'worker-pool-02 (Demo) memory usage sustained above 80%', 84.2, 80, 'demo:mem:worker-02', ?, ?)`),
			uuid.NewString(), orgID, ruleMem, hostWorker, now.Add(-14*time.Minute).Format(time.RFC3339), database.BoolToDB(s.db.Driver, true))
		_, _ = tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO alert_events (id, organization_id, rule_id, service_id, state, severity, summary, metric_value, threshold, fingerprint, incident_id, triggered_at, is_synthetic)
			VALUES (?, ?, ?, ?, 'firing', 'critical', 'Staging DNS Resolver (Demo) failed 3 consecutive checks', 3, 1, 'demo:dns:stg', ?, ?, ?)`),
			uuid.NewString(), orgID, ruleDNS, svcDNS, incID, now.Add(-9*time.Minute).Format(time.RFC3339), database.BoolToDB(s.db.Driver, true))

		// Synthetic Structured Logs
		demoLogs := []struct {
			sev, src, msg, corr, hostID, svcID, envID string
			minsAgo                                   int
		}{
			{"info", "edge-api", "Accepted 142 requests/s on /v1/health with p95 latency 34ms", "req-demo-101", hostAPI, svcGateway, envProdID, 2},
			{"warn", "queue-worker", "Heap allocation crossed 80% watermark on worker-pool-02 (Demo)", "wrk-demo-204", hostWorker, svcWorkerHTTP, envProdID, 6},
			{"error", "monitoring", "DNS lookup failed for staging.demo.infraos.internal: simulated NXDOMAIN timeout", "chk-demo-309", hostStg, svcDNS, envStgID, 9},
			{"info", "deployment-engine", "Simulation release v2.14.2 verified and promoted on Edge API Production", depCurr, hostAPI, svcGateway, envProdID, 37},
			{"info", "agent", "Heartbeat persisted from pg-primary-01.demo.internal (disk 82.0%)", "hb-demo-412", hostDB, "", envProdID, 12},
		}
		for _, dl := range demoLogs {
			var hArg, sArg any
			if dl.hostID != "" {
				hArg = dl.hostID
			}
			if dl.svcID != "" {
				sArg = dl.svcID
			}
			_, _ = tx.ExecContext(ctx, s.db.Rebind(`
				INSERT INTO log_entries (organization_id, host_id, service_id, environment_id, timestamp, severity, source_name, message, correlation_id, fields_json, is_synthetic)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '{"demo":true}', ?)`),
				orgID, hArg, sArg, dl.envID, now.Add(-time.Duration(dl.minsAgo)*time.Minute).Format(time.RFC3339),
				dl.sev, dl.src, dl.msg, dl.corr, database.BoolToDB(s.db.Driver, true))
		}

		// Manual Topology Link (explicitly marked manual)
		_, _ = tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO topology_links (id, organization_id, environment_id, source_type, source_id, target_type, target_id, label, is_manual, created_at)
			VALUES (?, ?, ?, 'service', ?, 'host', ?, 'reads_from (manual)', ?, ?)`),
			uuid.NewString(), orgID, envProdID, svcGateway, hostDB, database.BoolToDB(s.db.Driver, true), nowStr)

		// Notification
		_, _ = tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO notifications (id, organization_id, kind, severity, title, body, link, created_at)
			VALUES (?, ?, 'demo_notice', 'info', 'Isolated synthetic sandbox active', 'All hosts, metrics, containers, and incidents in this session are synthetic demonstration data.', '/workspace', ?)`),
			uuid.NewString(), orgID, nowStr)

		return nil
	})
	if err != nil {
		return nil, "", err
	}

	return s.auth.CreateSessionForUser(ctx, userID, orgID, ipAddr, userAgent)
}
