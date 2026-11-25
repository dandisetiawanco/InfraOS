package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
	"github.com/dandisetiawanco/InfraOS/internal/queue"
	"github.com/dandisetiawanco/InfraOS/internal/security"
	"github.com/dandisetiawanco/InfraOS/internal/sse"
	"github.com/dandisetiawanco/InfraOS/internal/topology"
	"github.com/dandisetiawanco/InfraOS/pkg/contracts"
	"github.com/dandisetiawanco/InfraOS/pkg/telemetry"
)

func main() {
	seedDemoOnly := flag.Bool("seed-demo-only", false, "Initialize database, seed a synthetic demo workspace, and exit")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.DBDriver, cfg.SQLitePath, cfg.PostgresDSN)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}

	metrics := telemetry.New()
	otelShutdown := telemetry.InitOTel(ctx, telemetry.OTelConfig{
		Enabled:          cfg.OTELEnabled,
		ExporterEndpoint: cfg.OTELExporterEndpoint,
		ServiceName:      "infraos-api",
	}, logger)
	defer func() { _ = otelShutdown(context.Background()) }()

	broker := sse.NewBroker()
	auditRec := audit.NewRecorder(db)
	ssrfValidator := security.NewSSRFValidator(cfg.MonitoringLocalAllowlist)
	jobQueue := queue.NewInProcess(128, 4, logger, metrics)

	authSvc := auth.NewService(db, auditRec, cfg.SessionTTL, cfg.LoginRateLimitPerMin)
	logSvc := logs.NewService(db, broker)
	incSvc := incidents.NewService(db, auditRec, broker)
	alertEng := alerts.NewEngine(db, ssrfValidator, incSvc, broker)
	hostSvc := hosts.NewService(db, auditRec, broker, metrics)
	monSched := monitoring.NewScheduler(db, ssrfValidator, broker, metrics, logger, cfg.MonitoringMaxConcurrency)
	cntSvc := containers.NewService(db, auditRec, cfg.DockerEnabled, cfg.DockerSocket, cfg.DockerAllowMutations, cfg.DockerMutationAllowlist)
	depSvc := deployments.NewService(db, jobQueue, ssrfValidator, auditRec, logSvc, broker)
	intelSvc := intelligence.NewAnalyzer(db, cfg.AIProvider, cfg.AIEndpoint, cfg.AIAPIKey, cfg.AIModel)
	integSvc := integrations.NewService(db, auditRec, logSvc, cfg.SecretEncryptionKey)
	topoSvc := topology.NewService(db)

	// Connect metric ingestion, service checks, and deployment failures to alert rules and structured logs.
	hostSvc.SetMetricHook(func(hCtx context.Context, orgID, hostID string, m contracts.HostMetricPayload) {
		_ = alertEng.EvaluateObservation(hCtx, orgID, "high_cpu", hostID, hostID, "", fmt.Sprintf("%s CPU at %.1f%%", m.Hostname, m.CPUPercent), m.CPUPercent, false)
		_ = alertEng.EvaluateObservation(hCtx, orgID, "high_memory", hostID, hostID, "", fmt.Sprintf("%s memory at %.1f%%", m.Hostname, m.MemPercent), m.MemPercent, false)
		_ = alertEng.EvaluateObservation(hCtx, orgID, "low_disk", hostID, hostID, "", fmt.Sprintf("%s disk at %.1f%%", m.Hostname, m.DiskPercent), m.DiskPercent, false)
	})
	monSched.SetCheckHook(func(cCtx context.Context, svc monitoring.ServiceRecord, res monitoring.CheckResult) {
		sev := "info"
		msg := fmt.Sprintf("Service check %s (%s) succeeded in %dms", svc.Name, svc.CheckType, res.LatencyMS)
		if res.Status != "up" {
			sev = "error"
			msg = fmt.Sprintf("Service check %s (%s) failed: %s", svc.Name, svc.CheckType, res.FailureReason)
		}
		_, _ = logSvc.Ingest(cCtx, logs.IngestInput{
			OrgID: svc.OrgID, ServiceID: svc.ID, EnvironmentID: svc.EnvironmentID,
			Severity: sev, SourceName: "monitoring-scheduler", Message: msg,
		})
		_ = alertEng.EvaluateObservation(cCtx, svc.OrgID, "service_down", svc.ID, svc.HostID, svc.ID, msg, float64(svc.ConsecutiveFailures), svc.Status == "down")
		if res.Status == "up" {
			_ = alertEng.EvaluateObservation(cCtx, svc.OrgID, "high_latency", svc.ID, svc.HostID, svc.ID, fmt.Sprintf("%s latency %dms", svc.Name, res.LatencyMS), float64(res.LatencyMS), false)
		}
		if res.TLSDaysRemaining != nil {
			_ = alertEng.EvaluateObservation(cCtx, svc.OrgID, "tls_expiring", svc.ID, svc.HostID, svc.ID, fmt.Sprintf("%s TLS certificate expires in %d days", svc.Name, *res.TLSDaysRemaining), float64(*res.TLSDaysRemaining), false)
		}
	})
	depSvc.SetFailureHook(func(dCtx context.Context, dep deployments.Deployment) {
		_ = alertEng.EvaluateObservation(dCtx, dep.OrgID, "deployment_failed", dep.TargetID, "", "", fmt.Sprintf("Deployment %s failed on %s: %s", dep.Version, dep.TargetName, dep.ErrorMessage), 1, true)
	})

	srv := api.NewServer(api.Dependencies{
		Config: cfg, DB: db, Logger: logger, Metrics: metrics, Broker: broker, Audit: auditRec,
		Auth: authSvc, Hosts: hostSvc, Monitoring: monSched, Containers: cntSvc, Deployments: depSvc,
		Logs: logSvc, Alerts: alertEng, Incidents: incSvc, Intelligence: intelSvc, Integrations: integSvc,
		Topology: topoSvc, SSRF: ssrfValidator,
	})

	if *seedDemoOnly {
		sess, _, err := srv.CreateIsolatedDemoSession(ctx, "127.0.0.1", "infraos-seed-cli")
		if err != nil {
			logger.Error("demo seed failed", "error", err)
			os.Exit(1)
		}
		logger.Info("synthetic demo workspace seeded", "organization_id", sess.Org.ID, "slug", sess.Org.Slug)
		return
	}

	jobQueue.Start(ctx)
	if cfg.EmbeddedWorker {
		monSched.Start(ctx)
		go runRetentionLoop(ctx, db, cfg, logger)
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("infraos api listening", "addr", cfg.Addr(), "profile", cfg.Profile, "db_driver", cfg.DBDriver)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		logger.Error("http server error", "error", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	_ = jobQueue.Shutdown(shutdownCtx)
	monSched.Wait()
	logger.Info("infraos api stopped cleanly")
}

func runRetentionLoop(ctx context.Context, db *database.DB, cfg config.Config, logger *slog.Logger) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := db.PruneRetention(ctx, cfg.MetricsRetentionDays, cfg.LogsRetentionDays, cfg.ChecksRetentionDays)
			if err != nil {
				logger.Warn("retention pruning failed", "error", err)
			} else if deleted > 0 {
				logger.Info("retention pruning completed", "rows_deleted", deleted)
			}
		}
	}
}
