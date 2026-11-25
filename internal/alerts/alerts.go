package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/database"
	"github.com/dandisetiawanco/InfraOS/internal/incidents"
	"github.com/dandisetiawanco/InfraOS/internal/security"
	"github.com/dandisetiawanco/InfraOS/internal/sse"
	"github.com/google/uuid"
)

type Rule struct {
	ID              string  `json:"id"`
	OrgID           string  `json:"organization_id"`
	EnvironmentID   string  `json:"environment_id,omitempty"`
	Name            string  `json:"name"`
	RuleType        string  `json:"rule_type"`
	TargetRef       string  `json:"target_ref"`
	Threshold       float64 `json:"threshold"`
	DurationSeconds int     `json:"duration_seconds"`
	CooldownSeconds int     `json:"cooldown_seconds"`
	Severity        string  `json:"severity"`
	AutoIncident    bool    `json:"auto_incident"`
	WebhookURL      string  `json:"webhook_url,omitempty"`
	Enabled         bool    `json:"enabled"`
	State           string  `json:"state"`
	PendingSince    string  `json:"pending_since,omitempty"`
	LastFiredAt     string  `json:"last_fired_at,omitempty"`
	LastResolvedAt  string  `json:"last_resolved_at,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type AlertEvent struct {
	ID          string  `json:"id"`
	OrgID       string  `json:"organization_id"`
	RuleID      string  `json:"rule_id"`
	RuleName    string  `json:"rule_name,omitempty"`
	HostID      string  `json:"host_id,omitempty"`
	ServiceID   string  `json:"service_id,omitempty"`
	State       string  `json:"state"`
	Severity    string  `json:"severity"`
	Summary     string  `json:"summary"`
	MetricValue float64 `json:"metric_value"`
	Threshold   float64 `json:"threshold"`
	Fingerprint string  `json:"fingerprint"`
	IncidentID  string  `json:"incident_id,omitempty"`
	TriggeredAt string  `json:"triggered_at"`
	ResolvedAt  string  `json:"resolved_at,omitempty"`
	IsSynthetic bool    `json:"is_synthetic"`
}

type Notification struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Severity  string `json:"severity"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Link      string `json:"link"`
	ReadAt    string `json:"read_at,omitempty"`
	CreatedAt string `json:"created_at"`
}

type CreateRuleInput struct {
	EnvironmentID   string
	Name            string
	RuleType        string
	TargetRef       string
	Threshold       float64
	DurationSeconds int
	CooldownSeconds int
	Severity        string
	AutoIncident    bool
	WebhookURL      string
}

type Engine struct {
	db        *database.DB
	ssrf      *security.SSRFValidator
	incidents *incidents.Service
	broker    *sse.Broker
}

func NewEngine(db *database.DB, ssrf *security.SSRFValidator, incSvc *incidents.Service, broker *sse.Broker) *Engine {
	return &Engine{db: db, ssrf: ssrf, incidents: incSvc, broker: broker}
}

func (e *Engine) CreateRule(ctx context.Context, orgID string, isDemo bool, in CreateRuleInput) (*Rule, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.RuleType = strings.ToLower(strings.TrimSpace(in.RuleType))
	if in.Name == "" {
		return nil, errors.New("alert rule name is required")
	}
	switch in.RuleType {
	case "high_cpu", "high_memory", "low_disk", "agent_offline", "service_down", "high_latency", "tls_expiring", "deployment_failed":
	default:
		return nil, errors.New("invalid alert rule_type")
	}
	if in.TargetRef == "" {
		in.TargetRef = "*"
	}
	if in.DurationSeconds < 0 {
		in.DurationSeconds = 0
	}
	if in.CooldownSeconds < 30 {
		in.CooldownSeconds = 300
	}
	sev := strings.ToLower(strings.TrimSpace(in.Severity))
	switch sev {
	case "info", "warning", "critical":
	default:
		sev = "warning"
	}
	if strings.TrimSpace(in.WebhookURL) != "" {
		if _, _, err := e.ssrf.ValidateHTTPURL(ctx, in.WebhookURL, isDemo); err != nil {
			return nil, fmt.Errorf("webhook_url rejected by destination policy: %w", err)
		}
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	var envArg any
	if in.EnvironmentID != "" {
		envArg = in.EnvironmentID
	}

	_, err := e.db.Exec(ctx, `
		INSERT INTO alert_rules (id, organization_id, environment_id, name, rule_type, target_ref, threshold, duration_seconds, cooldown_seconds, severity, auto_incident, webhook_url, enabled, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ok', ?, ?)`,
		id, orgID, envArg, in.Name, in.RuleType, in.TargetRef, in.Threshold, in.DurationSeconds, in.CooldownSeconds,
		sev, database.BoolToDB(e.db.Driver, in.AutoIncident), strings.TrimSpace(in.WebhookURL), database.BoolToDB(e.db.Driver, true), now, now,
	)
	if err != nil {
		return nil, err
	}
	return e.GetRule(ctx, orgID, id)
}

func (e *Engine) ListRules(ctx context.Context, orgID string) ([]Rule, error) {
	rows, err := e.db.Query(ctx, `
		SELECT id, organization_id, COALESCE(environment_id, ''), name, rule_type, target_ref, threshold,
		       duration_seconds, cooldown_seconds, severity, CASE WHEN auto_incident THEN 1 ELSE 0 END,
		       webhook_url, CASE WHEN enabled THEN 1 ELSE 0 END, state,
		       COALESCE(pending_since, ''), COALESCE(last_fired_at, ''), COALESCE(last_resolved_at, ''),
		       created_at, updated_at
		FROM alert_rules
		WHERE organization_id = ?
		ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Rule, 0)
	for rows.Next() {
		var r Rule
		var autoInt, enInt int
		if err := rows.Scan(
			&r.ID, &r.OrgID, &r.EnvironmentID, &r.Name, &r.RuleType, &r.TargetRef, &r.Threshold,
			&r.DurationSeconds, &r.CooldownSeconds, &r.Severity, &autoInt, &r.WebhookURL, &enInt,
			&r.State, &r.PendingSince, &r.LastFiredAt, &r.LastResolvedAt, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		r.AutoIncident = autoInt == 1
		r.Enabled = enInt == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (e *Engine) GetRule(ctx context.Context, orgID, ruleID string) (*Rule, error) {
	list, err := e.ListRules(ctx, orgID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == ruleID {
			item := list[i]
			return &item, nil
		}
	}
	return nil, errors.New("alert rule not found")
}

func (e *Engine) DeleteRule(ctx context.Context, orgID, ruleID string) error {
	res, err := e.db.Exec(ctx, `DELETE FROM alert_rules WHERE id = ? AND organization_id = ?`, ruleID, orgID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("alert rule not found")
	}
	return nil
}

func (e *Engine) ListEvents(ctx context.Context, orgID string, limit int) ([]AlertEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := e.db.Query(ctx, `
		SELECT a.id, a.organization_id, a.rule_id, COALESCE(r.name, ''), COALESCE(a.host_id, ''),
		       COALESCE(a.service_id, ''), a.state, a.severity, a.summary, a.metric_value, a.threshold,
		       a.fingerprint, COALESCE(a.incident_id, ''), a.triggered_at, COALESCE(a.resolved_at, ''),
		       CASE WHEN a.is_synthetic THEN 1 ELSE 0 END
		FROM alert_events a
		LEFT JOIN alert_rules r ON r.id = a.rule_id
		WHERE a.organization_id = ?
		ORDER BY a.triggered_at DESC
		LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]AlertEvent, 0)
	for rows.Next() {
		var ev AlertEvent
		var synthInt int
		if err := rows.Scan(
			&ev.ID, &ev.OrgID, &ev.RuleID, &ev.RuleName, &ev.HostID, &ev.ServiceID,
			&ev.State, &ev.Severity, &ev.Summary, &ev.MetricValue, &ev.Threshold,
			&ev.Fingerprint, &ev.IncidentID, &ev.TriggeredAt, &ev.ResolvedAt, &synthInt,
		); err != nil {
			return nil, err
		}
		ev.IsSynthetic = synthInt == 1
		out = append(out, ev)
	}
	return out, rows.Err()
}

// EvaluateObservation transitions matching alert rules across ok -> pending -> firing -> resolved.
func (e *Engine) EvaluateObservation(ctx context.Context, orgID, ruleType, targetID, hostID, serviceID, summary string, value float64, breached bool) error {
	rules, err := e.ListRules(ctx, orgID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	for _, rule := range rules {
		if !rule.Enabled || rule.RuleType != ruleType {
			continue
		}
		if rule.TargetRef != "*" && rule.TargetRef != targetID {
			continue
		}
		isBreached := breached
		if !isBreached {
			switch ruleType {
			case "high_cpu", "high_memory", "high_latency":
				isBreached = value >= rule.Threshold
			case "low_disk":
				// Threshold represents minimum free % or maximum used % (>= threshold triggers).
				isBreached = value >= rule.Threshold
			case "tls_expiring":
				isBreached = value <= rule.Threshold
			}
		}

		fp := fmt.Sprintf("%s:%s:%s", rule.ID, ruleType, targetID)
		if isBreached {
			if rule.State == "ok" {
				if rule.DurationSeconds > 0 {
					_, _ = e.db.Exec(ctx, `UPDATE alert_rules SET state = 'pending', pending_since = ?, updated_at = ? WHERE id = ?`, nowStr, nowStr, rule.ID)
					continue
				}
			} else if rule.State == "pending" && rule.PendingSince != "" {
				if ps, err := time.Parse(time.RFC3339, rule.PendingSince); err == nil {
					if now.Sub(ps) < time.Duration(rule.DurationSeconds)*time.Second {
						continue
					}
				}
			}

			// Enforce cooldown to prevent alert storms.
			if rule.State == "firing" && rule.LastFiredAt != "" {
				if lf, err := time.Parse(time.RFC3339, rule.LastFiredAt); err == nil {
					if now.Sub(lf) < time.Duration(rule.CooldownSeconds)*time.Second {
						continue
					}
				}
			}

			var incidentID string
			if rule.AutoIncident && e.incidents != nil {
				incSev := "sev3"
				if rule.Severity == "critical" {
					incSev = "sev2"
				}
				var svcs []string
				if serviceID != "" {
					svcs = []string{serviceID}
				}
				if inc, err := e.incidents.Create(ctx, orgID, incidents.CreateInput{
					EnvironmentID:      rule.EnvironmentID,
					Title:              rule.Name + ": " + summary,
					Description:        fmt.Sprintf("Automated alert rule %q breached threshold %.2f with observed value %.2f.", rule.Name, rule.Threshold, value),
					Severity:           incSev,
					AffectedServiceIDs: svcs,
					DedupeKey:          fp,
				}); err == nil && inc != nil {
					incidentID = inc.ID
				}
			}

			evID := uuid.NewString()
			var hostArg, svcArg, incArg any
			if hostID != "" {
				hostArg = hostID
			}
			if serviceID != "" {
				svcArg = serviceID
			}
			if incidentID != "" {
				incArg = incidentID
			}

			_, _ = e.db.Exec(ctx, `
				INSERT INTO alert_events (id, organization_id, rule_id, host_id, service_id, state, severity, summary, metric_value, threshold, fingerprint, incident_id, triggered_at, is_synthetic)
				VALUES (?, ?, ?, ?, ?, 'firing', ?, ?, ?, ?, ?, ?, ?, ?)`,
				evID, orgID, rule.ID, hostArg, svcArg, rule.Severity, summary, value, rule.Threshold, fp, incArg, nowStr, database.BoolToDB(e.db.Driver, false))
			_, _ = e.db.Exec(ctx, `UPDATE alert_rules SET state = 'firing', last_fired_at = ?, pending_since = NULL, updated_at = ? WHERE id = ?`, nowStr, nowStr, rule.ID)

			e.createNotification(ctx, orgID, "alert_firing", rule.Severity, rule.Name, summary, "/workspace/alerts")
			if rule.WebhookURL != "" {
				go e.sendWebhook(rule.WebhookURL, map[string]any{
					"event": "alert.firing", "rule": rule.Name, "severity": rule.Severity, "summary": summary, "value": value, "threshold": rule.Threshold, "timestamp": nowStr,
				})
			}
			if e.broker != nil {
				e.broker.Publish(orgID, "alert.event", map[string]any{"event_id": evID, "rule_id": rule.ID, "state": "firing", "summary": summary})
			}
		} else if rule.State == "firing" || rule.State == "pending" {
			_, _ = e.db.Exec(ctx, `UPDATE alert_rules SET state = 'ok', pending_since = NULL, last_resolved_at = ?, updated_at = ? WHERE id = ?`, nowStr, nowStr, rule.ID)
			if rule.State == "firing" {
				_, _ = e.db.Exec(ctx, `
					UPDATE alert_events SET state = 'resolved', resolved_at = ?
					WHERE organization_id = ? AND rule_id = ? AND fingerprint = ? AND state = 'firing'`,
					nowStr, orgID, rule.ID, fp)
				e.createNotification(ctx, orgID, "alert_resolved", "info", "Recovered: "+rule.Name, summary, "/workspace/alerts")
				if e.broker != nil {
					e.broker.Publish(orgID, "alert.event", map[string]any{"rule_id": rule.ID, "state": "resolved", "summary": summary})
				}
			}
		}
	}
	return nil
}

func (e *Engine) createNotification(ctx context.Context, orgID, kind, severity, title, body, link string) {
	_, _ = e.db.Exec(ctx, `
		INSERT INTO notifications (id, organization_id, kind, severity, title, body, link, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), orgID, kind, severity, title, body, link, time.Now().UTC().Format(time.RFC3339))
}

func (e *Engine) ListNotifications(ctx context.Context, orgID string, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 50 {
		limit = 25
	}
	rows, err := e.db.Query(ctx, `
		SELECT id, kind, severity, title, body, link, COALESCE(read_at, ''), created_at
		FROM notifications WHERE organization_id = ?
		ORDER BY created_at DESC LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Notification, 0)
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Kind, &n.Severity, &n.Title, &n.Body, &n.Link, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (e *Engine) MarkNotificationsRead(ctx context.Context, orgID string) error {
	_, err := e.db.Exec(ctx, `UPDATE notifications SET read_at = ? WHERE organization_id = ? AND read_at IS NULL`, time.Now().UTC().Format(time.RFC3339), orgID)
	return err
}

func (e *Engine) sendWebhook(rawURL string, payload map[string]any) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := e.ssrf.ValidateHTTPURL(ctx, rawURL, false); err != nil {
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	client := e.ssrf.NewHTTPClient(5*time.Second, false)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "InfraOS-Alerts/0.1")
	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}
