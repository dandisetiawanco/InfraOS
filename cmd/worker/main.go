package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/alerts"
	"github.com/dandisetiawanco/InfraOS/internal/audit"
	"github.com/dandisetiawanco/InfraOS/internal/config"
	"github.com/dandisetiawanco/InfraOS/internal/database"
	"github.com/dandisetiawanco/InfraOS/internal/incidents"
	"github.com/dandisetiawanco/InfraOS/internal/logs"
	"github.com/dandisetiawanco/InfraOS/internal/monitoring"
	"github.com/dandisetiawanco/InfraOS/internal/security"
	"github.com/dandisetiawanco/InfraOS/pkg/telemetry"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid worker configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.DBDriver, cfg.SQLitePath, cfg.PostgresDSN)
	if err != nil {
		logger.Error("worker database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		logger.Error("worker database migration failed", "error", err)
		os.Exit(1)
	}

	metrics := telemetry.New()
	ssrf := security.NewSSRFValidator(cfg.MonitoringLocalAllowlist)
	auditRec := audit.NewRecorder(db)
	logSvc := logs.NewService(db, nil)
	incSvc := incidents.NewService(db, auditRec, nil)
	alertEng := alerts.NewEngine(db, ssrf, incSvc, nil)
	monSched := monitoring.NewScheduler(db, ssrf, nil, metrics, logger, cfg.MonitoringMaxConcurrency)

	monSched.SetCheckHook(func(cCtx context.Context, svc monitoring.ServiceRecord, res monitoring.CheckResult) {
		sev := "info"
		msg := fmt.Sprintf("Worker check %s (%s) succeeded in %dms", svc.Name, svc.CheckType, res.LatencyMS)
		if res.Status != "up" {
			sev = "error"
			msg = fmt.Sprintf("Worker check %s (%s) failed: %s", svc.Name, svc.CheckType, res.FailureReason)
		}
		_, _ = logSvc.Ingest(cCtx, logs.IngestInput{
			OrgID: svc.OrgID, ServiceID: svc.ID, EnvironmentID: svc.EnvironmentID,
			Severity: sev, SourceName: "infraos-worker", Message: msg,
		})
		_ = alertEng.EvaluateObservation(cCtx, svc.OrgID, "service_down", svc.ID, svc.HostID, svc.ID, msg, float64(svc.ConsecutiveFailures), svc.Status == "down")
	})

	logger.Info("infraos standalone worker started", "profile", cfg.Profile, "db_driver", cfg.DBDriver)
	monSched.Start(ctx)

	retentionTicker := time.NewTicker(6 * time.Hour)
	defer retentionTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("infraos worker shutting down")
			monSched.Wait()
			return
		case <-retentionTicker.C:
			deleted, err := db.PruneRetention(ctx, cfg.MetricsRetentionDays, cfg.LogsRetentionDays, cfg.ChecksRetentionDays)
			if err != nil {
				logger.Warn("worker retention prune failed", "error", err)
			} else if deleted > 0 {
				logger.Info("worker pruned expired records", "count", deleted)
			}
		}
	}
}
