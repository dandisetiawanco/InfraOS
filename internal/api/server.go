package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/alerts"
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
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
)

type Server struct {
	cfg          config.Config
	db           *database.DB
	logger       *slog.Logger
	metrics      *telemetry.Metrics
	broker       *sse.Broker
	audit        *audit.Recorder
	auth         *auth.Service
	hosts        *hosts.Service
	monitoring   *monitoring.Scheduler
	containers   *containers.Service
	deployments  *deployments.Service
	logs         *logs.Service
	alerts       *alerts.Engine
	incidents    *incidents.Service
	intelligence *intelligence.Analyzer
	integrations *integrations.Service
	topology     *topology.Service
	ssrf         *security.SSRFValidator
}

type Dependencies struct {
	Config       config.Config
	DB           *database.DB
	Logger       *slog.Logger
	Metrics      *telemetry.Metrics
	Broker       *sse.Broker
	Audit        *audit.Recorder
	Auth         *auth.Service
	Hosts        *hosts.Service
	Monitoring   *monitoring.Scheduler
	Containers   *containers.Service
	Deployments  *deployments.Service
	Logs         *logs.Service
	Alerts       *alerts.Engine
	Incidents    *incidents.Service
	Intelligence *intelligence.Analyzer
	Integrations *integrations.Service
	Topology     *topology.Service
	SSRF         *security.SSRFValidator
}

func NewServer(deps Dependencies) *Server {
	return &Server{
		cfg:          deps.Config,
		db:           deps.DB,
		logger:       deps.Logger,
		metrics:      deps.Metrics,
		broker:       deps.Broker,
		audit:        deps.Audit,
		auth:         deps.Auth,
		hosts:        deps.Hosts,
		monitoring:   deps.Monitoring,
		containers:   deps.Containers,
		deployments:  deps.Deployments,
		logs:         deps.Logs,
		alerts:       deps.Alerts,
		incidents:    deps.Incidents,
		intelligence: deps.Intelligence,
		integrations: deps.Integrations,
		topology:     deps.Topology,
		ssrf:         deps.SSRF,
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(s.requestIDMiddleware)
	r.Use(s.securityHeadersMiddleware)
	r.Use(s.corsMiddleware)
	r.Use(s.observabilityMiddleware)

	// Health & Observability
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok", "profile": s.cfg.Profile, "timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.db.Ping(r.Context()); err != nil {
			writeError(w, r, http.StatusServiceUnavailable, "database_unavailable", "Database readiness check failed.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ready", "driver": s.db.Driver, "timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})
	r.Handle("/metrics", s.metrics.Handler())

	if s.cfg.ExposeAPIDocs {
		r.Get("/api/docs", s.handleAPIDocsHTML)
		r.Get("/api/v1/openapi.json", s.handleOpenAPISpec)
	}

	r.Route("/api/v1", func(v1 chi.Router) {
		// Public auth endpoints
		v1.Post("/auth/register", s.handleRegister)
		v1.Post("/auth/login", s.handleLogin)
		v1.Post("/auth/demo", s.handleDemoLaunch)

		// Dedicated agent authentication endpoints (separate from user sessions)
		v1.Post("/agents/enroll", s.handleAgentEnroll)
		v1.Post("/agents/metrics", s.handleAgentMetrics)

		// Signed GitHub webhook endpoint
		v1.Post("/webhooks/github/{orgId}", s.handleGitHubWebhook)

		// Authenticated workspace API
		v1.Group(func(authed chi.Router) {
			authed.Use(s.requireSession)
			authed.Use(s.csrfMiddleware)

			authed.Get("/auth/me", s.handleMe)
			authed.Post("/auth/logout", s.handleLogout)
			authed.Post("/auth/switch-org", s.handleSwitchOrg)

			authed.Get("/overview", s.handleOverview)
			authed.Get("/insights", s.handleInsights)
			authed.Get("/search", s.handleGlobalSearch)
			authed.Get("/events/stream", s.handleSSEStream)

			// Projects, Environments, Team
			authed.Get("/projects", s.handleListProjects)
			authed.With(s.requirePerm(auth.PermManageHosts)).Post("/projects", s.handleCreateProject)
			authed.Get("/environments", s.handleListEnvironments)
			authed.With(s.requirePerm(auth.PermManageHosts)).Post("/environments", s.handleCreateEnvironment)
			authed.Get("/team", s.handleListTeam)
			authed.With(s.requirePerm(auth.PermManageTeam)).Post("/team", s.handleInviteMember)
			authed.With(s.requirePerm(auth.PermManageTeam)).Patch("/team/{userId}", s.handleUpdateMemberRole)

			// Hosts & Agent Lifecycle
			authed.Get("/hosts", s.handleListHosts)
			authed.Get("/hosts/{id}", s.handleGetHost)
			authed.With(s.requirePerm(auth.PermManageHosts)).Patch("/hosts/{id}", s.handleUpdateHost)
			authed.Get("/hosts/{id}/metrics", s.handleHostMetrics)
			authed.With(s.requirePerm(auth.PermManageHosts)).Post("/agents/enrollment-tokens", s.handleCreateEnrollmentToken)
			authed.With(s.requirePerm(auth.PermManageHosts)).Post("/agents/{id}/rotate", s.handleRotateAgent)
			authed.With(s.requirePerm(auth.PermManageHosts)).Post("/agents/{id}/revoke", s.handleRevokeAgent)

			// Services & Monitoring
			authed.Get("/services", s.handleListServices)
			authed.With(s.requirePerm(auth.PermOperateServices)).Post("/services", s.handleCreateService)
			authed.Get("/services/{id}", s.handleGetService)
			authed.With(s.requirePerm(auth.PermOperateServices)).Delete("/services/{id}", s.handleDeleteService)
			authed.Get("/services/{id}/checks", s.handleListServiceChecks)
			authed.With(s.requirePerm(auth.PermOperateServices)).Post("/services/{id}/check-now", s.handleCheckServiceNow)

			// Containers
			authed.Get("/containers", s.handleListContainers)
			authed.Get("/containers/{id}", s.handleGetContainer)
			authed.Get("/containers/{id}/logs", s.handleContainerLogs)
			authed.With(s.requirePerm(auth.PermManageContainers)).Post("/containers/{id}/action", s.handleContainerAction)

			// Deployments
			authed.Get("/deployment-targets", s.handleListDeploymentTargets)
			authed.With(s.requirePerm(auth.PermOperateDeployments)).Post("/deployment-targets", s.handleCreateDeploymentTarget)
			authed.Get("/deployments", s.handleListDeployments)
			authed.With(s.requirePerm(auth.PermOperateDeployments)).Post("/deployments", s.handleTriggerDeployment)
			authed.Get("/deployments/{id}", s.handleGetDeployment)
			authed.With(s.requirePerm(auth.PermOperateDeployments)).Post("/deployments/{id}/rollback", s.handleRollbackDeployment)

			// Logs
			authed.Get("/logs", s.handleQueryLogs)
			authed.With(s.requirePerm(auth.PermOperateLogs)).Post("/logs", s.handleIngestLog)
			authed.Get("/logs/export", s.handleExportLogs)

			// Alerts & Notifications
			authed.Get("/alerts/rules", s.handleListAlertRules)
			authed.With(s.requirePerm(auth.PermManageAlerts)).Post("/alerts/rules", s.handleCreateAlertRule)
			authed.With(s.requirePerm(auth.PermManageAlerts)).Delete("/alerts/rules/{id}", s.handleDeleteAlertRule)
			authed.Get("/alerts/events", s.handleListAlertEvents)
			authed.Get("/notifications", s.handleListNotifications)
			authed.Post("/notifications/read-all", s.handleMarkNotificationsRead)

			// Incidents
			authed.Get("/incidents", s.handleListIncidents)
			authed.With(s.requirePerm(auth.PermOperateIncidents)).Post("/incidents", s.handleCreateIncident)
			authed.Get("/incidents/{id}", s.handleGetIncident)
			authed.With(s.requirePerm(auth.PermOperateIncidents)).Patch("/incidents/{id}", s.handleUpdateIncident)
			authed.With(s.requirePerm(auth.PermOperateIncidents)).Post("/incidents/{id}/timeline", s.handleAddIncidentTimeline)
			authed.With(s.requirePerm(auth.PermOperateIncidents)).Post("/incidents/{id}/summarize", s.handleSummarizeIncident)

			// Topology
			authed.Get("/topology", s.handleTopology)
			authed.With(s.requirePerm(auth.PermOperateServices)).Post("/topology/links", s.handleAddTopologyLink)

			// Integrations
			authed.Get("/integrations", s.handleListIntegrations)
			authed.With(s.requirePerm(auth.PermManageIntegrations)).Put("/integrations/{provider}", s.handleUpdateIntegration)
			authed.Get("/integrations/github/overview", s.handleGitHubOverview)

			// Audit & Backups
			authed.Get("/audit", s.handleListAudit)
			authed.Get("/backups", s.handleListBackups)
			authed.With(s.requirePerm(auth.PermManageBackups)).Post("/backups", s.handleCreateBackup)
		})
	})

	return r
}

type reqIDKeyType string

const reqIDKey reqIDKeyType = "req_id"

func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), reqIDKey, id)))
	})
}

func requestID(r *http.Request) string {
	if v, ok := r.Context().Value(reqIDKey).(string); ok {
		return v
	}
	return ""
}

func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(s.cfg.AllowedOrigins))
	for _, o := range s.cfg.AllowedOrigins {
		allowed[strings.TrimRight(o, "/")] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimRight(r.Header.Get("Origin"), "/")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token, X-Request-ID, X-Idempotency-Key")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Vary", "Origin")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) observabilityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		route := r.URL.Path
		if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePattern() != "" {
			route = rctx.RoutePattern()
		}
		dur := time.Since(start)
		if s.metrics != nil && route != "/metrics" {
			s.metrics.ObserveHTTP(r.Method, route, sw.status, dur)
		}
		if route != "/healthz" && route != "/metrics" {
			s.logger.Info("http_request",
				"request_id", requestID(r),
				"method", r.Method,
				"route", route,
				"status", sw.status,
				"duration_ms", dur.Milliseconds(),
				"remote_ip", clientIP(r),
			)
		}
	})
}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawToken := ""
		if c, err := r.Cookie(auth.SessionCookieName); err == nil && c.Value != "" {
			rawToken = c.Value
		} else if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			rawToken = strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		}
		sess, err := s.auth.AuthenticateToken(r.Context(), rawToken)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication required.")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithSession(r.Context(), sess)))
	})
}

func (s *Server) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		// Bearer token clients do not use browser cookies for auth.
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			next.ServeHTTP(w, r)
			return
		}
		sess, ok := auth.FromContext(r.Context())
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication required.")
			return
		}
		hdr := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
		if hdr == "" || hdr != sess.CSRFToken {
			writeError(w, r, http.StatusForbidden, "csrf_failed", "Missing or invalid CSRF token.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requirePerm(perm auth.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := auth.FromContext(r.Context())
			if !ok {
				writeError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication required.")
				return
			}
			if !auth.HasPermission(sess.Org.Role, perm) {
				writeError(w, r, http.StatusForbidden, "forbidden", fmt.Sprintf("Role %q does not have %q permission.", sess.Org.Role, perm))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Auth handlers

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name             string `json:"name"`
		Email            string `json:"email"`
		Password         string `json:"password"`
		OrganizationName string `json:"organization_name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	sess, rawToken, err := s.auth.Register(r.Context(), auth.RegisterInput{
		Name: body.Name, Email: body.Email, Password: body.Password, OrganizationName: body.OrganizationName,
		IPAddress: clientIP(r), UserAgent: r.UserAgent(),
	})
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "registration_failed", err.Error())
		return
	}
	auth.SetSessionCookies(w, rawToken, sess.CSRFToken, sess.ExpiresAt, s.cfg.CookieSecure)
	orgs, _ := s.auth.ListUserOrganizations(r.Context(), sess.User.ID)
	writeEnvelope(w, http.StatusCreated, map[string]any{
		"user": sess.User, "organization": sess.Org, "organizations": orgs, "csrf_token": sess.CSRFToken, "token": rawToken,
	}, nil)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	sess, rawToken, err := s.auth.Login(r.Context(), body.Email, body.Password, clientIP(r), r.UserAgent())
	if err != nil {
		code := http.StatusUnauthorized
		if strings.Contains(err.Error(), "too many") {
			code = http.StatusTooManyRequests
		}
		writeError(w, r, code, "login_failed", err.Error())
		return
	}
	auth.SetSessionCookies(w, rawToken, sess.CSRFToken, sess.ExpiresAt, s.cfg.CookieSecure)
	orgs, _ := s.auth.ListUserOrganizations(r.Context(), sess.User.ID)
	writeEnvelope(w, http.StatusOK, map[string]any{
		"user": sess.User, "organization": sess.Org, "organizations": orgs, "csrf_token": sess.CSRFToken, "token": rawToken,
	}, nil)
}

func (s *Server) handleDemoLaunch(w http.ResponseWriter, r *http.Request) {
	sess, rawToken, err := s.CreateIsolatedDemoSession(r.Context(), clientIP(r), r.UserAgent())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "demo_init_failed", err.Error())
		return
	}
	auth.SetSessionCookies(w, rawToken, sess.CSRFToken, sess.ExpiresAt, s.cfg.CookieSecure)
	orgs, _ := s.auth.ListUserOrganizations(r.Context(), sess.User.ID)
	writeEnvelope(w, http.StatusCreated, map[string]any{
		"user": sess.User, "organization": sess.Org, "organizations": orgs, "csrf_token": sess.CSRFToken, "token": rawToken, "is_demo": true,
	}, nil)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	orgs, _ := s.auth.ListUserOrganizations(r.Context(), sess.User.ID)
	writeEnvelope(w, http.StatusOK, map[string]any{
		"user": sess.User, "organization": sess.Org, "organizations": orgs, "csrf_token": sess.CSRFToken,
		"profile": s.cfg.Profile, "db_driver": s.db.Driver, "ai_enabled": s.intelligence.AIEnabled(),
	}, nil)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	_ = s.auth.RevokeSession(r.Context(), sess.SessionID, sess.Org.ID, sess.User.ID, clientIP(r))
	auth.ClearSessionCookies(w, s.cfg.CookieSecure)
	writeEnvelope(w, http.StatusOK, map[string]any{"revoked": true}, nil)
}

func (s *Server) handleSwitchOrg(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		OrganizationID string `json:"organization_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	org, err := s.auth.SwitchOrganization(r.Context(), sess.SessionID, sess.User.ID, body.OrganizationID)
	if err != nil {
		writeError(w, r, http.StatusForbidden, "switch_org_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]any{"organization": org}, nil)
}

// Overview, Insights, Search, SSE

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	envID := r.URL.Query().Get("environment_id")
	hostList, _ := s.hosts.ListHosts(r.Context(), sess.Org.ID, "", envID)
	svcList, _ := s.monitoring.ListServices(r.Context(), sess.Org.ID, envID)
	depList, _ := s.deployments.ListDeployments(r.Context(), sess.Org.ID, envID, 8)
	incList, _ := s.incidents.List(r.Context(), sess.Org.ID, "")
	alertEvents, _ := s.alerts.ListEvents(r.Context(), sess.Org.ID, 10)
	insights, _ := s.intelligence.Analyze(r.Context(), sess.Org.ID)

	var onlineHosts, offlineHosts, staleHosts int
	var sumCPU, sumMem, sumDisk, sumRxBps, sumTxBps float64
	var metricHosts int
	dataSource := "unavailable"
	latestAt := ""

	for _, h := range hostList {
		switch h.Status {
		case "online":
			onlineHosts++
		case "stale":
			staleHosts++
		default:
			offlineHosts++
		}
		if h.LatestMetric != nil {
			metricHosts++
			sumCPU += h.LatestMetric.CPUPercent
			sumMem += h.LatestMetric.MemPercent
			sumDisk += h.LatestMetric.DiskPercent
			sumRxBps += h.LatestMetric.NetRxBps
			sumTxBps += h.LatestMetric.NetTxBps
			if h.LatestMetric.RecordedAt > latestAt {
				latestAt = h.LatestMetric.RecordedAt
			}
			if h.IsSynthetic {
				dataSource = "synthetic"
			} else if h.Freshness == "live" {
				dataSource = "live"
			} else if dataSource == "unavailable" {
				dataSource = h.Freshness
			}
		}
	}

	var healthySvcs, degradedSvcs, downSvcs int
	var availSum float64
	var availCount int
	for _, sv := range svcList {
		switch sv.Status {
		case "healthy":
			healthySvcs++
		case "degraded":
			degradedSvcs++
		case "down":
			downSvcs++
		}
		if sv.UptimePercent != nil {
			availSum += *sv.UptimePercent
			availCount++
		}
	}
	var avgAvail *float64
	if availCount > 0 {
		v := float64(int(availSum/float64(availCount)*100)) / 100
		avgAvail = &v
	}

	openIncidents := 0
	for _, inc := range incList {
		if inc.Status != "resolved" {
			openIncidents++
		}
	}

	var avgCPU, avgMem, avgDisk *float64
	if metricHosts > 0 {
		c := float64(int(sumCPU/float64(metricHosts)*10)) / 10
		m := float64(int(sumMem/float64(metricHosts)*10)) / 10
		d := float64(int(sumDisk/float64(metricHosts)*10)) / 10
		avgCPU, avgMem, avgDisk = &c, &m, &d
	}

	writeEnvelope(w, http.StatusOK, map[string]any{
		"is_demo":     sess.Org.IsDemo,
		"data_source": dataSource,
		"latest_at":   latestAt,
		"summary": map[string]any{
			"hosts_total":              len(hostList),
			"hosts_online":             onlineHosts,
			"hosts_stale":              staleHosts,
			"hosts_offline":            offlineHosts,
			"services_total":           len(svcList),
			"services_healthy":         healthySvcs,
			"services_degraded":        degradedSvcs,
			"services_down":            downSvcs,
			"service_availability_pct": avgAvail,
			"open_incidents":           openIncidents,
			"avg_cpu_percent":          avgCPU,
			"avg_mem_percent":          avgMem,
			"avg_disk_percent":         avgDisk,
			"total_net_rx_bps":         sumRxBps,
			"total_net_tx_bps":         sumTxBps,
		},
		"hosts":              hostList,
		"services":           svcList,
		"recent_deployments": depList,
		"open_incident_list": incList,
		"recent_alerts":      alertEvents,
		"insights":           insights,
	}, nil)
}

func (s *Server) handleInsights(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	list, err := s.intelligence.Analyze(r.Context(), sess.Org.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "insights_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, list, nil)
}

func (s *Server) handleGlobalSearch(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	hostsList, _ := s.hosts.ListHosts(r.Context(), sess.Org.ID, q, "")
	svcs, _ := s.monitoring.ListServices(r.Context(), sess.Org.ID, "")
	deps, _ := s.deployments.ListDeployments(r.Context(), sess.Org.ID, "", 20)
	incs, _ := s.incidents.List(r.Context(), sess.Org.ID, "")

	type resultItem struct {
		Type     string `json:"type"`
		ID       string `json:"id"`
		Title    string `json:"title"`
		Subtitle string `json:"subtitle"`
		Href     string `json:"href"`
	}
	results := make([]resultItem, 0)
	for _, h := range hostsList {
		results = append(results, resultItem{Type: "host", ID: h.ID, Title: h.DisplayName, Subtitle: h.Hostname + " · " + h.Status, Href: "/workspace/hosts/" + h.ID})
	}
	for _, sv := range svcs {
		if q == "" || strings.Contains(strings.ToLower(sv.Name), q) || strings.Contains(strings.ToLower(sv.Target), q) {
			results = append(results, resultItem{Type: "service", ID: sv.ID, Title: sv.Name, Subtitle: sv.CheckType + " · " + sv.Status, Href: "/workspace/services/" + sv.ID})
		}
	}
	for _, d := range deps {
		if q == "" || strings.Contains(strings.ToLower(d.Version), q) || strings.Contains(strings.ToLower(d.TargetName), q) {
			results = append(results, resultItem{Type: "deployment", ID: d.ID, Title: d.TargetName + " " + d.Version, Subtitle: d.Status, Href: "/workspace/deployments/" + d.ID})
		}
	}
	for _, inc := range incs {
		if q == "" || strings.Contains(strings.ToLower(inc.Title), q) {
			results = append(results, resultItem{Type: "incident", ID: inc.ID, Title: inc.Title, Subtitle: inc.Severity + " · " + inc.Status, Href: "/workspace/incidents/" + inc.ID})
		}
	}
	if len(results) > 25 {
		results = results[:25]
	}
	writeEnvelope(w, http.StatusOK, results, nil)
}

func (s *Server) handleSSEStream(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, r, http.StatusInternalServerError, "streaming_unsupported", "Streaming unsupported.")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsubscribe := s.broker.Subscribe(sess.Org.ID)
	defer unsubscribe()

	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"organization_id\":%q,\"timestamp\":%q}\n\n", sess.Org.ID, time.Now().UTC().Format(time.RFC3339))
	flusher.Flush()

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprintf(w, ": heartbeat %s\n\n", time.Now().UTC().Format(time.RFC3339))
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			raw, err := ev.EncodeJSON()
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, raw)
			flusher.Flush()
		}
	}
}

// Projects, Environments, Team

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	rows, err := s.db.Query(r.Context(), `SELECT id, name, slug, description, repository_url, created_at FROM projects WHERE organization_id = ? ORDER BY created_at ASC`, sess.Org.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, slug, desc, repo, created string
		if rows.Scan(&id, &name, &slug, &desc, &repo, &created) == nil {
			out = append(out, map[string]any{"id": id, "name": name, "slug": slug, "description": desc, "repository_url": repo, "created_at": created})
		}
	}
	writeEnvelope(w, http.StatusOK, out, nil)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		RepositoryURL string `json:"repository_url"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "Project name is required.")
		return
	}
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	slug := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(body.Name), " ", "-")) + "-" + id[:4]
	_, err := s.db.Exec(r.Context(), `
		INSERT INTO projects (id, organization_id, name, slug, description, repository_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, sess.Org.ID, strings.TrimSpace(body.Name), slug, body.Description, body.RepositoryURL, now, now)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "create_project_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusCreated, map[string]any{"id": id, "name": body.Name, "slug": slug, "description": body.Description}, nil)
}

func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	rows, err := s.db.Query(r.Context(), `
		SELECT e.id, e.project_id, COALESCE(p.name, ''), e.name, e.slug, e.tier, e.description, e.created_at
		FROM environments e
		LEFT JOIN projects p ON p.id = e.project_id
		WHERE e.organization_id = ?
		ORDER BY e.created_at ASC`, sess.Org.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "query_failed", err.Error())
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, projID, projName, name, slug, tier, desc, created string
		if rows.Scan(&id, &projID, &projName, &name, &slug, &tier, &desc, &created) == nil {
			out = append(out, map[string]any{
				"id": id, "project_id": projID, "project_name": projName, "name": name, "slug": slug, "tier": tier, "description": desc, "created_at": created,
			})
		}
	}
	writeEnvelope(w, http.StatusOK, out, nil)
}

func (s *Server) handleCreateEnvironment(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		ProjectID   string `json:"project_id"`
		Name        string `json:"name"`
		Tier        string `json:"tier"`
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "Environment name is required.")
		return
	}
	if body.ProjectID == "" {
		_ = s.db.QueryRow(r.Context(), `SELECT id FROM projects WHERE organization_id = ? ORDER BY created_at ASC LIMIT 1`, sess.Org.ID).Scan(&body.ProjectID)
	}
	tier := strings.ToLower(strings.TrimSpace(body.Tier))
	switch tier {
	case "production", "staging", "development":
	default:
		tier = "development"
	}
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	slug := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(body.Name), " ", "-")) + "-" + id[:4]
	_, err := s.db.Exec(r.Context(), `
		INSERT INTO environments (id, organization_id, project_id, name, slug, tier, description, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, sess.Org.ID, body.ProjectID, strings.TrimSpace(body.Name), slug, tier, body.Description, now)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "create_environment_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusCreated, map[string]any{"id": id, "project_id": body.ProjectID, "name": body.Name, "slug": slug, "tier": tier}, nil)
}

func (s *Server) handleListTeam(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	members, err := s.auth.ListMembers(r.Context(), sess.Org.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "team_query_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, members, nil)
}

func (s *Server) handleInviteMember(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	member, err := s.auth.AddOrInviteMember(r.Context(), sess.Org.ID, body.Email, body.Name, body.Password, auth.Role(body.Role))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invite_failed", err.Error())
		return
	}
	_ = s.audit.Record(r.Context(), audit.RecordInput{
		OrgID: sess.Org.ID, ActorUserID: sess.User.ID, Action: "team.member.added", ResourceType: "user", ResourceID: member.UserID, IPAddress: clientIP(r),
		Details: map[string]any{"email": member.Email, "role": member.Role},
	})
	writeEnvelope(w, http.StatusCreated, member, nil)
}

func (s *Server) handleUpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		Role string `json:"role"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	targetID := chi.URLParam(r, "userId")
	if err := s.auth.UpdateMemberRole(r.Context(), sess.Org.ID, targetID, auth.Role(body.Role)); err != nil {
		writeError(w, r, http.StatusBadRequest, "role_update_failed", err.Error())
		return
	}
	_ = s.audit.Record(r.Context(), audit.RecordInput{
		OrgID: sess.Org.ID, ActorUserID: sess.User.ID, Action: "team.role.updated", ResourceType: "user", ResourceID: targetID, IPAddress: clientIP(r),
		Details: map[string]any{"role": body.Role},
	})
	writeEnvelope(w, http.StatusOK, map[string]any{"updated": true}, nil)
}

// Agent & Host handlers

func (s *Server) handleCreateEnrollmentToken(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	if sess.Org.IsDemo {
		writeError(w, r, http.StatusForbidden, "demo_restricted", "Real host enrollment is disabled in public demonstration mode.")
		return
	}
	var body struct {
		Label         string `json:"label"`
		EnvironmentID string `json:"environment_id"`
		TTLMinutes    int    `json:"ttl_minutes"`
	}
	_ = decodeJSON(r, &body)
	ttl := time.Duration(body.TTLMinutes) * time.Minute
	res, err := s.hosts.CreateEnrollmentToken(r.Context(), sess.Org.ID, body.EnvironmentID, body.Label, sess.User.ID, ttl)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "token_create_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusCreated, res, nil)
}

func (s *Server) handleAgentEnroll(w http.ResponseWriter, r *http.Request) {
	var req contracts.AgentEnrollRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	res, err := s.hosts.EnrollAgent(r.Context(), req, clientIP(r))
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "enrollment_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusCreated, res, nil)
}

func (s *Server) handleAgentMetrics(w http.ResponseWriter, r *http.Request) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authHeader, "Agent ") {
		writeError(w, r, http.StatusUnauthorized, "agent_auth_required", "Expected Authorization: Agent <token>.")
		return
	}
	rawToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Agent "))
	ident, err := s.hosts.AuthenticateAgent(r.Context(), rawToken)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "agent_auth_failed", err.Error())
		return
	}
	var payload contracts.HostMetricPayload
	if err := decodeJSON(r, &payload); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if err := s.hosts.IngestMetrics(r.Context(), ident, payload, clientIP(r)); err != nil {
		writeError(w, r, http.StatusInternalServerError, "ingest_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusAccepted, map[string]any{"accepted": true, "host_id": ident.HostID}, nil)
}

func (s *Server) handleRotateAgent(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	agentID := chi.URLParam(r, "id")
	raw, prefix, err := s.hosts.RotateAgentCredential(r.Context(), sess.Org.ID, agentID, sess.User.ID)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "rotate_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]any{"agent_token": raw, "token_prefix": prefix}, nil)
}

func (s *Server) handleRevokeAgent(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	agentID := chi.URLParam(r, "id")
	if err := s.hosts.RevokeAgent(r.Context(), sess.Org.ID, agentID, sess.User.ID); err != nil {
		writeError(w, r, http.StatusBadRequest, "revoke_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]any{"revoked": true}, nil)
}

func (s *Server) handleListHosts(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	list, err := s.hosts.ListHosts(r.Context(), sess.Org.ID, r.URL.Query().Get("q"), r.URL.Query().Get("environment_id"))
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "hosts_query_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, list, nil)
}

func (s *Server) handleGetHost(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	host, err := s.hosts.GetHost(r.Context(), sess.Org.ID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "host_not_found", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, host, nil)
}

func (s *Server) handleUpdateHost(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		DisplayName   string   `json:"display_name"`
		EnvironmentID string   `json:"environment_id"`
		Tags          []string `json:"tags"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := s.hosts.UpdateHost(r.Context(), sess.Org.ID, chi.URLParam(r, "id"), body.DisplayName, body.EnvironmentID, body.Tags); err != nil {
		writeError(w, r, http.StatusBadRequest, "host_update_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]any{"updated": true}, nil)
}

func (s *Server) handleHostMetrics(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	pts, rng, err := s.hosts.GetHostMetrics(r.Context(), sess.Org.ID, chi.URLParam(r, "id"), r.URL.Query().Get("range"))
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "metrics_query_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]any{"range": rng, "points": pts}, nil)
}

// Service Monitoring handlers

func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	list, err := s.monitoring.ListServices(r.Context(), sess.Org.ID, r.URL.Query().Get("environment_id"))
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "services_query_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, list, nil)
}

func (s *Server) handleCreateService(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		Name              string `json:"name"`
		CheckType         string `json:"check_type"`
		Target            string `json:"target"`
		ProjectID         string `json:"project_id"`
		EnvironmentID     string `json:"environment_id"`
		HostID            string `json:"host_id"`
		IntervalSeconds   int    `json:"interval_seconds"`
		TimeoutMS         int    `json:"timeout_ms"`
		ExpectedStatus    int    `json:"expected_status"`
		FailureThreshold  int    `json:"failure_threshold"`
		RecoveryThreshold int    `json:"recovery_threshold"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	svc, err := s.monitoring.CreateService(r.Context(), sess.Org.ID, sess.Org.IsDemo, monitoring.CreateServiceInput{
		Name: body.Name, CheckType: body.CheckType, Target: body.Target, ProjectID: body.ProjectID,
		EnvironmentID: body.EnvironmentID, HostID: body.HostID, IntervalSeconds: body.IntervalSeconds,
		TimeoutMS: body.TimeoutMS, ExpectedStatus: body.ExpectedStatus,
		FailureThreshold: body.FailureThreshold, RecoveryThreshold: body.RecoveryThreshold,
	})
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "service_create_failed", err.Error())
		return
	}
	_ = s.audit.Record(r.Context(), audit.RecordInput{
		OrgID: sess.Org.ID, ActorUserID: sess.User.ID, Action: "service.created", ResourceType: "service", ResourceID: svc.ID, IPAddress: clientIP(r),
		Details: map[string]any{"name": svc.Name, "type": svc.CheckType, "target": svc.Target},
	})
	writeEnvelope(w, http.StatusCreated, svc, nil)
}

func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	svc, err := s.monitoring.GetService(r.Context(), sess.Org.ID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "service_not_found", err.Error())
		return
	}
	checks, _ := s.monitoring.ListChecks(r.Context(), sess.Org.ID, svc.ID, 60)
	writeEnvelope(w, http.StatusOK, map[string]any{"service": svc, "checks": checks}, nil)
}

func (s *Server) handleDeleteService(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	id := chi.URLParam(r, "id")
	if err := s.monitoring.DeleteService(r.Context(), sess.Org.ID, id); err != nil {
		writeError(w, r, http.StatusNotFound, "service_delete_failed", err.Error())
		return
	}
	_ = s.audit.Record(r.Context(), audit.RecordInput{
		OrgID: sess.Org.ID, ActorUserID: sess.User.ID, Action: "service.deleted", ResourceType: "service", ResourceID: id, IPAddress: clientIP(r),
	})
	writeEnvelope(w, http.StatusOK, map[string]any{"deleted": true}, nil)
}

func (s *Server) handleListServiceChecks(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	checks, err := s.monitoring.ListChecks(r.Context(), sess.Org.ID, chi.URLParam(r, "id"), limit)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "checks_query_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, checks, nil)
}

func (s *Server) handleCheckServiceNow(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	res, err := s.monitoring.RunCheckNow(r.Context(), sess.Org.ID, chi.URLParam(r, "id"), sess.Org.IsDemo)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "check_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, res, nil)
}

// Containers handlers

func (s *Server) handleListContainers(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	res, err := s.containers.List(r.Context(), sess.Org.ID, sess.Org.IsDemo)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "containers_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, res, nil)
}

func (s *Server) handleGetContainer(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	cnt, st, err := s.containers.Get(r.Context(), sess.Org.ID, chi.URLParam(r, "id"), sess.Org.IsDemo)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "container_not_found", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]any{"integration": st, "container": cnt}, nil)
}

func (s *Server) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	lines, err := s.containers.Logs(r.Context(), sess.Org.ID, chi.URLParam(r, "id"), sess.Org.IsDemo)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "container_logs_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]any{"lines": lines}, nil)
}

func (s *Server) handleContainerAction(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		Action    string `json:"action"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := s.containers.PerformAction(r.Context(), sess.Org.ID, chi.URLParam(r, "id"), body.Action, sess.User.ID, clientIP(r), body.Confirmed, sess.Org.IsDemo); err != nil {
		writeError(w, r, http.StatusForbidden, "container_action_denied", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]any{"ok": true, "action": body.Action}, nil)
}

// Deployment handlers

func (s *Server) handleListDeploymentTargets(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	targets, err := s.deployments.ListTargets(r.Context(), sess.Org.ID, r.URL.Query().Get("environment_id"))
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "targets_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, targets, nil)
}

func (s *Server) handleCreateDeploymentTarget(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		Name          string `json:"name"`
		ProjectID     string `json:"project_id"`
		EnvironmentID string `json:"environment_id"`
		HostID        string `json:"host_id"`
		Adapter       string `json:"adapter"`
		HealthURL     string `json:"health_url"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	t, err := s.deployments.CreateTarget(r.Context(), sess.Org.ID, sess.Org.IsDemo, deployments.CreateTargetInput{
		Name: body.Name, ProjectID: body.ProjectID, EnvironmentID: body.EnvironmentID,
		HostID: body.HostID, Adapter: body.Adapter, HealthURL: body.HealthURL,
	})
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "target_create_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusCreated, t, nil)
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := s.deployments.ListDeployments(r.Context(), sess.Org.ID, r.URL.Query().Get("environment_id"), limit)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "deployments_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, list, nil)
}

func (s *Server) handleTriggerDeployment(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		TargetID       string `json:"target_id"`
		Version        string `json:"version"`
		CommitSHA      string `json:"commit_sha"`
		CommitMessage  string `json:"commit_message"`
		Branch         string `json:"branch"`
		ImageRef       string `json:"image_ref"`
		IdempotencyKey string `json:"idempotency_key"`
		SimulateFail   bool   `json:"simulate_fail"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	idemp := body.IdempotencyKey
	if idemp == "" {
		idemp = strings.TrimSpace(r.Header.Get("X-Idempotency-Key"))
	}
	dep, err := s.deployments.Trigger(r.Context(), sess.Org.ID, deployments.TriggerInput{
		TargetID: body.TargetID, Version: body.Version, CommitSHA: body.CommitSHA, CommitMessage: body.CommitMessage,
		Branch: body.Branch, ImageRef: body.ImageRef, IdempotencyKey: idemp, SimulateFail: body.SimulateFail,
		TriggeredBy: sess.User.ID, IPAddress: clientIP(r), IsDemo: sess.Org.IsDemo,
	})
	if err != nil {
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "locked") {
			code = http.StatusConflict
		}
		writeError(w, r, code, "deployment_trigger_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusAccepted, dep, nil)
}

func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	dep, err := s.deployments.GetDeployment(r.Context(), sess.Org.ID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "deployment_not_found", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, dep, nil)
}

func (s *Server) handleRollbackDeployment(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	dep, err := s.deployments.Rollback(r.Context(), sess.Org.ID, chi.URLParam(r, "id"), sess.User.ID, clientIP(r), sess.Org.IsDemo)
	if err != nil {
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "locked") {
			code = http.StatusConflict
		}
		writeError(w, r, code, "rollback_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusAccepted, dep, nil)
}

// Logs handlers

func (s *Server) handleQueryLogs(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	entries, total, err := s.logs.Query(r.Context(), sess.Org.ID, logs.Filter{
		Search: q.Get("q"), Severity: q.Get("severity"), Source: q.Get("source"),
		HostID: q.Get("host_id"), ServiceID: q.Get("service_id"), EnvironmentID: q.Get("environment_id"),
		Range: q.Get("range"), Limit: limit, Offset: offset,
	})
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "logs_query_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, entries, map[string]any{"total": total, "limit": limit, "offset": offset})
}

func (s *Server) handleIngestLog(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		Severity      string         `json:"severity"`
		SourceName    string         `json:"source_name"`
		HostID        string         `json:"host_id"`
		ServiceID     string         `json:"service_id"`
		EnvironmentID string         `json:"environment_id"`
		Message       string         `json:"message"`
		CorrelationID string         `json:"correlation_id"`
		Fields        map[string]any `json:"fields"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	entry, err := s.logs.Ingest(r.Context(), logs.IngestInput{
		OrgID: sess.Org.ID, Severity: body.Severity, SourceName: body.SourceName,
		HostID: body.HostID, ServiceID: body.ServiceID, EnvironmentID: body.EnvironmentID,
		Message: body.Message, CorrelationID: body.CorrelationID, Fields: body.Fields, IsSynthetic: sess.Org.IsDemo,
	})
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "log_ingest_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusCreated, entry, nil)
}

func (s *Server) handleExportLogs(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	q := r.URL.Query()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="infraos-logs.csv"`)
	_ = s.logs.ExportCSV(r.Context(), sess.Org.ID, logs.Filter{
		Search: q.Get("q"), Severity: q.Get("severity"), Source: q.Get("source"), Range: q.Get("range"),
	}, w)
}

// Alerts & Notifications handlers

func (s *Server) handleListAlertRules(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	rules, err := s.alerts.ListRules(r.Context(), sess.Org.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "alert_rules_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, rules, nil)
}

func (s *Server) handleCreateAlertRule(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		EnvironmentID   string  `json:"environment_id"`
		Name            string  `json:"name"`
		RuleType        string  `json:"rule_type"`
		TargetRef       string  `json:"target_ref"`
		Threshold       float64 `json:"threshold"`
		DurationSeconds int     `json:"duration_seconds"`
		CooldownSeconds int     `json:"cooldown_seconds"`
		Severity        string  `json:"severity"`
		AutoIncident    bool    `json:"auto_incident"`
		WebhookURL      string  `json:"webhook_url"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	rule, err := s.alerts.CreateRule(r.Context(), sess.Org.ID, sess.Org.IsDemo, alerts.CreateRuleInput{
		EnvironmentID: body.EnvironmentID, Name: body.Name, RuleType: body.RuleType, TargetRef: body.TargetRef,
		Threshold: body.Threshold, DurationSeconds: body.DurationSeconds, CooldownSeconds: body.CooldownSeconds,
		Severity: body.Severity, AutoIncident: body.AutoIncident, WebhookURL: body.WebhookURL,
	})
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "alert_rule_create_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusCreated, rule, nil)
}

func (s *Server) handleDeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	if err := s.alerts.DeleteRule(r.Context(), sess.Org.ID, chi.URLParam(r, "id")); err != nil {
		writeError(w, r, http.StatusNotFound, "alert_rule_delete_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]any{"deleted": true}, nil)
}

func (s *Server) handleListAlertEvents(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := s.alerts.ListEvents(r.Context(), sess.Org.ID, limit)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "alert_events_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, events, nil)
}

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	list, err := s.alerts.ListNotifications(r.Context(), sess.Org.ID, 30)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "notifications_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, list, nil)
}

func (s *Server) handleMarkNotificationsRead(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	_ = s.alerts.MarkNotificationsRead(r.Context(), sess.Org.ID)
	writeEnvelope(w, http.StatusOK, map[string]any{"read": true}, nil)
}

// Incident handlers

func (s *Server) handleListIncidents(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	list, err := s.incidents.List(r.Context(), sess.Org.ID, r.URL.Query().Get("status"))
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "incidents_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, list, nil)
}

func (s *Server) handleCreateIncident(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		EnvironmentID      string   `json:"environment_id"`
		Title              string   `json:"title"`
		Description        string   `json:"description"`
		Severity           string   `json:"severity"`
		AffectedServiceIDs []string `json:"affected_service_ids"`
		AssignedUserID     string   `json:"assigned_user_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	inc, err := s.incidents.Create(r.Context(), sess.Org.ID, incidents.CreateInput{
		EnvironmentID: body.EnvironmentID, Title: body.Title, Description: body.Description,
		Severity: body.Severity, AffectedServiceIDs: body.AffectedServiceIDs,
		AssignedUserID: body.AssignedUserID, AuthorID: sess.User.ID, IsSynthetic: sess.Org.IsDemo,
	})
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "incident_create_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusCreated, inc, nil)
}

func (s *Server) handleGetIncident(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	inc, err := s.incidents.Get(r.Context(), sess.Org.ID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "incident_not_found", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, inc, nil)
}

func (s *Server) handleUpdateIncident(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		Status             string `json:"status"`
		Severity           string `json:"severity"`
		AssignedUserID     string `json:"assigned_user_id"`
		RootCauseNotes     string `json:"root_cause_notes"`
		ResolutionSummary  string `json:"resolution_summary"`
		PostmortemMarkdown string `json:"postmortem_markdown"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	inc, err := s.incidents.Update(r.Context(), sess.Org.ID, chi.URLParam(r, "id"), incidents.UpdateInput{
		Status: body.Status, Severity: body.Severity, AssignedUserID: body.AssignedUserID,
		RootCauseNotes: body.RootCauseNotes, ResolutionSummary: body.ResolutionSummary,
		PostmortemMarkdown: body.PostmortemMarkdown, AuthorID: sess.User.ID,
	})
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "incident_update_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, inc, nil)
}

func (s *Server) handleAddIncidentTimeline(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		EventType string `json:"event_type"`
		Message   string `json:"message"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := s.incidents.AddTimelineEvent(r.Context(), sess.Org.ID, chi.URLParam(r, "id"), body.EventType, sess.User.ID, body.Message, nil); err != nil {
		writeError(w, r, http.StatusBadRequest, "timeline_add_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusCreated, map[string]any{"added": true}, nil)
}

func (s *Server) handleSummarizeIncident(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	inc, err := s.incidents.Get(r.Context(), sess.Org.ID, chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, r, http.StatusNotFound, "incident_not_found", err.Error())
		return
	}
	lines := make([]string, 0, len(inc.Timeline))
	for _, te := range inc.Timeline {
		lines = append(lines, te.CreatedAt+" "+te.Message)
	}
	res, err := s.intelligence.SummarizeIncident(r.Context(), intelligence.AISummaryRequest{
		Title: inc.Title, Description: inc.Description, Severity: inc.Severity, Timeline: lines,
	})
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "ai_unavailable", err.Error())
		return
	}
	_ = s.incidents.AddTimelineEvent(r.Context(), sess.Org.ID, inc.ID, "ai_summary", sess.User.ID, res.Summary, map[string]any{"provider": res.Provider, "model": res.Model})
	writeEnvelope(w, http.StatusOK, res, nil)
}

// Topology, Integrations, Audit, Backups

func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	g, err := s.topology.BuildGraph(r.Context(), sess.Org.ID, r.URL.Query().Get("environment_id"))
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "topology_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, g, nil)
}

func (s *Server) handleAddTopologyLink(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		EnvironmentID string `json:"environment_id"`
		SourceType    string `json:"source_type"`
		SourceID      string `json:"source_id"`
		TargetType    string `json:"target_type"`
		TargetID      string `json:"target_id"`
		Label         string `json:"label"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := s.topology.AddManualLink(r.Context(), sess.Org.ID, body.EnvironmentID, body.SourceType, body.SourceID, body.TargetType, body.TargetID, body.Label); err != nil {
		writeError(w, r, http.StatusBadRequest, "link_create_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusCreated, map[string]any{"created": true}, nil)
}

func (s *Server) handleListIntegrations(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	list, err := s.integrations.List(r.Context(), sess.Org.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "integrations_failed", err.Error())
		return
	}
	dockerStatus := s.containers.Status(r.Context(), sess.Org.IsDemo)
	writeEnvelope(w, http.StatusOK, map[string]any{
		"settings":   list,
		"docker":     dockerStatus,
		"ai_enabled": s.intelligence.AIEnabled(),
		"otel":       map[string]any{"enabled": s.cfg.OTELEnabled, "endpoint": s.cfg.OTELExporterEndpoint},
	}, nil)
}

func (s *Server) handleUpdateIntegration(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	var body struct {
		Config map[string]any `json:"config"`
		Secret string         `json:"secret"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	st, err := s.integrations.Upsert(r.Context(), sess.Org.ID, chi.URLParam(r, "provider"), sess.User.ID, body.Config, body.Secret, sess.Org.IsDemo)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "integration_update_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, st, nil)
}

func (s *Server) handleGitHubOverview(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	res, err := s.integrations.FetchGitHubOverview(r.Context(), sess.Org.ID, sess.Org.IsDemo)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "github_fetch_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, res, nil)
}

func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgId")
	body, err := io.ReadAll(io.LimitReader(r.Body, 512*1024))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "invalid_body", "Could not read webhook body.")
		return
	}
	dup, err := s.integrations.ProcessGitHubWebhook(
		r.Context(),
		orgID,
		r.Header.Get("X-GitHub-Delivery"),
		r.Header.Get("X-GitHub-Event"),
		r.Header.Get("X-Hub-Signature-256"),
		body,
	)
	if err != nil {
		writeError(w, r, http.StatusUnauthorized, "webhook_rejected", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, map[string]any{"accepted": true, "duplicate": dup}, nil)
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	events, total, err := s.audit.List(r.Context(), sess.Org.ID, q.Get("action"), q.Get("resource_type"), limit, offset)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "audit_failed", err.Error())
		return
	}
	writeEnvelope(w, http.StatusOK, events, map[string]any{"total": total})
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	rows, err := s.db.Query(r.Context(), `
		SELECT id, driver, file_path, size_bytes, sha256, status, notes, created_at
		FROM backup_records WHERE organization_id = ? ORDER BY created_at DESC LIMIT 30`, sess.Org.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "backups_failed", err.Error())
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, driver, fp, sum, status, notes, created string
		var size int64
		if rows.Scan(&id, &driver, &fp, &size, &sum, &status, &notes, &created) == nil {
			out = append(out, map[string]any{
				"id": id, "driver": driver, "file_path": filepath.Base(fp), "size_bytes": size,
				"sha256": sum, "status": status, "notes": notes, "created_at": created,
			})
		}
	}
	writeEnvelope(w, http.StatusOK, out, nil)
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	sess, _ := auth.FromContext(r.Context())
	if sess.Org.IsDemo {
		writeError(w, r, http.StatusForbidden, "demo_restricted", "Database backups are disabled in demonstration mode.")
		return
	}
	if s.db.Driver != "sqlite" {
		writeError(w, r, http.StatusBadRequest, "postgres_script_required", "Use scripts/backup-postgres.sh (pg_dump) for consistent PostgreSQL backups.")
		return
	}
	filename := fmt.Sprintf("infraos-sqlite-%s.db", time.Now().UTC().Format("20060102-150405"))
	dest := filepath.Join(s.cfg.BackupDir, filename)
	size, sum, err := s.db.BackupSQLite(r.Context(), dest)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "backup_failed", err.Error())
		return
	}
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.Exec(r.Context(), `
		INSERT INTO backup_records (id, organization_id, driver, file_path, size_bytes, sha256, status, notes, created_by, created_at)
		VALUES (?, ?, 'sqlite', ?, ?, ?, 'verified', 'Consistent snapshot created via SQLite VACUUM INTO.', ?, ?)`,
		id, sess.Org.ID, dest, size, sum, sess.User.ID, now)
	_ = s.audit.Record(r.Context(), audit.RecordInput{
		OrgID: sess.Org.ID, ActorUserID: sess.User.ID, Action: "backup.created", ResourceType: "backup", ResourceID: id, IPAddress: clientIP(r),
		Details: map[string]any{"filename": filename, "size_bytes": size, "sha256": sum},
	})
	writeEnvelope(w, http.StatusCreated, map[string]any{
		"id": id, "driver": "sqlite", "file_path": filename, "size_bytes": size, "sha256": sum, "status": "verified", "created_at": now,
	}, nil)
}

// Helpers

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("request body is empty")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeEnvelope(w http.ResponseWriter, status int, data any, meta any) {
	payload := map[string]any{"data": data}
	if meta != nil {
		payload["meta"] = meta
	}
	writeJSON(w, status, payload)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":       code,
			"message":    message,
			"request_id": requestID(r),
		},
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
