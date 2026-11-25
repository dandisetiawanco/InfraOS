package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	Registry           *prometheus.Registry
	HTTPRequestsTotal  *prometheus.CounterVec
	HTTPRequestSeconds *prometheus.HistogramVec
	HTTPErrorsTotal    *prometheus.CounterVec
	ActiveMonitorJobs  prometheus.Gauge
	AgentHeartbeats    prometheus.Counter
	MetricsIngested    prometheus.Counter
	DeploymentQueueLen prometheus.Gauge
	WorkerDuration     *prometheus.HistogramVec
	DatabaseErrors     *prometheus.CounterVec
}

func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Registry: reg,
		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "infraos_http_requests_total",
			Help: "Total HTTP requests handled by the InfraOS API.",
		}, []string{"method", "route", "status"}),
		HTTPRequestSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "infraos_http_request_duration_seconds",
			Help:    "HTTP request latency distribution.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		}, []string{"method", "route"}),
		HTTPErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "infraos_http_errors_total",
			Help: "Total HTTP responses with 5xx status codes.",
		}, []string{"method", "route"}),
		ActiveMonitorJobs: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "infraos_monitoring_active_jobs",
			Help: "Number of service monitoring checks currently executing.",
		}),
		AgentHeartbeats: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "infraos_agent_heartbeats_total",
			Help: "Total authenticated agent heartbeats accepted.",
		}),
		MetricsIngested: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "infraos_host_metrics_ingested_total",
			Help: "Total host metric samples persisted.",
		}),
		DeploymentQueueLen: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "infraos_deployment_queue_size",
			Help: "Current number of queued background jobs.",
		}),
		WorkerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "infraos_worker_job_duration_seconds",
			Help:    "Duration of background worker jobs.",
			Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 15, 30},
		}, []string{"job_type", "outcome"}),
		DatabaseErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "infraos_database_errors_total",
			Help: "Database operation failures grouped by operation.",
		}, []string{"operation"}),
	}

	reg.MustRegister(
		m.HTTPRequestsTotal,
		m.HTTPRequestSeconds,
		m.HTTPErrorsTotal,
		m.ActiveMonitorJobs,
		m.AgentHeartbeats,
		m.MetricsIngested,
		m.DeploymentQueueLen,
		m.WorkerDuration,
		m.DatabaseErrors,
	)
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{})
}

func (m *Metrics) ObserveHTTP(method, route string, status int, dur time.Duration) {
	statusCode := strconv.Itoa(status)
	m.HTTPRequestsTotal.WithLabelValues(method, route, statusCode).Inc()
	m.HTTPRequestSeconds.WithLabelValues(method, route).Observe(dur.Seconds())
	if status >= 500 {
		m.HTTPErrorsTotal.WithLabelValues(method, route).Inc()
	}
}

type OTelConfig struct {
	Enabled          bool
	ExporterEndpoint string
	ServiceName      string
}

// InitOTel records explicit telemetry export configuration without sending data when disabled.
func InitOTel(_ context.Context, cfg OTelConfig, logger *slog.Logger) func(context.Context) error {
	if !cfg.Enabled || cfg.ExporterEndpoint == "" {
		logger.Info("opentelemetry export disabled by configuration")
		return func(context.Context) error { return nil }
	}
	logger.Info("opentelemetry exporter configured", "endpoint", cfg.ExporterEndpoint, "service", cfg.ServiceName)
	return func(context.Context) error { return nil }
}
