package hosts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/audit"
	"github.com/dandisetiawanco/InfraOS/internal/database"
	"github.com/dandisetiawanco/InfraOS/internal/security"
	"github.com/dandisetiawanco/InfraOS/internal/sse"
	"github.com/dandisetiawanco/InfraOS/pkg/contracts"
	"github.com/dandisetiawanco/InfraOS/pkg/telemetry"
	"github.com/google/uuid"
)

type Host struct {
	ID              string                   `json:"id"`
	OrgID           string                   `json:"organization_id"`
	EnvironmentID   string                   `json:"environment_id,omitempty"`
	EnvironmentName string                   `json:"environment_name,omitempty"`
	Hostname        string                   `json:"hostname"`
	DisplayName     string                   `json:"display_name"`
	OS              string                   `json:"os"`
	Arch            string                   `json:"arch"`
	KernelVersion   string                   `json:"kernel_version"`
	IPAddress       string                   `json:"ip_address"`
	Tags            []string                 `json:"tags"`
	Status          string                   `json:"status"`
	Freshness       string                   `json:"freshness"`
	IsSynthetic     bool                     `json:"is_synthetic"`
	Metadata        map[string]any           `json:"metadata"`
	LastHeartbeatAt string                   `json:"last_heartbeat_at,omitempty"`
	AgentID         string                   `json:"agent_id,omitempty"`
	AgentVersion    string                   `json:"agent_version,omitempty"`
	AgentStatus     string                   `json:"agent_status,omitempty"`
	LatestMetric    *MetricPoint             `json:"latest_metric,omitempty"`
	Filesystems     []contracts.FilesystemMetric `json:"filesystems,omitempty"`
	Interfaces      []contracts.NetworkInterfaceMetric `json:"interfaces,omitempty"`
	Optional        *contracts.OptionalMetrics `json:"optional,omitempty"`
	SupportedRanges []string                 `json:"supported_ranges,omitempty"`
	CreatedAt       string                   `json:"created_at"`
	UpdatedAt       string                   `json:"updated_at"`
}

type MetricPoint struct {
	RecordedAt    string  `json:"recorded_at"`
	CPUPercent    float64 `json:"cpu_percent"`
	CPUCores      int     `json:"cpu_cores"`
	Load1m        float64 `json:"load_1m"`
	Load5m        float64 `json:"load_5m"`
	Load15m       float64 `json:"load_15m"`
	MemTotalBytes uint64  `json:"mem_total_bytes"`
	MemUsedBytes  uint64  `json:"mem_used_bytes"`
	MemAvailBytes uint64  `json:"mem_available_bytes"`
	MemPercent    float64 `json:"mem_percent"`
	DiskTotal     uint64  `json:"disk_total_bytes"`
	DiskUsed      uint64  `json:"disk_used_bytes"`
	DiskPercent   float64 `json:"disk_percent"`
	NetRxBytes    uint64  `json:"net_rx_bytes"`
	NetTxBytes    uint64  `json:"net_tx_bytes"`
	NetRxBps      float64 `json:"net_rx_bps"`
	NetTxBps      float64 `json:"net_tx_bps"`
	UptimeSeconds uint64  `json:"uptime_seconds"`
	IsSynthetic   bool    `json:"is_synthetic"`
}

type EnrollmentTokenResult struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Token         string `json:"token"`
	EnvironmentID string `json:"environment_id,omitempty"`
	ExpiresAt     string `json:"expires_at"`
	CreatedAt     string `json:"created_at"`
}

type AgentIdentity struct {
	AgentID        string
	HostID         string
	OrganizationID string
	CredentialID   string
}

type Service struct {
	db              *database.DB
	audit           *audit.Recorder
	broker          *sse.Broker
	metrics         *telemetry.Metrics
	onMetricIngest  func(ctx context.Context, orgID, hostID string, m contracts.HostMetricPayload)
}

func NewService(db *database.DB, auditRec *audit.Recorder, broker *sse.Broker, metrics *telemetry.Metrics) *Service {
	return &Service{db: db, audit: auditRec, broker: broker, metrics: metrics}
}

func (s *Service) SetMetricHook(fn func(ctx context.Context, orgID, hostID string, m contracts.HostMetricPayload)) {
	s.onMetricIngest = fn
}

func (s *Service) CreateEnrollmentToken(ctx context.Context, orgID, envID, label, userID string, ttl time.Duration) (*EnrollmentTokenResult, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "Host agent enrollment"
	}
	if ttl <= 0 || ttl > 24*time.Hour {
		ttl = 30 * time.Minute
	}
	raw, hash, err := security.GenerateToken("infraos_enr", 24)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	exp := now.Add(ttl)
	var envArg any
	if envID != "" {
		envArg = envID
	}
	var userArg any
	if userID != "" {
		userArg = userID
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO agent_enrollment_tokens (id, organization_id, environment_id, label, token_hash, expires_at, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, orgID, envArg, label, hash, exp.Format(time.RFC3339), userArg, now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	_ = s.audit.Record(ctx, audit.RecordInput{
		OrgID: orgID, ActorUserID: userID, Action: "agent.enrollment_token.created", ResourceType: "agent_enrollment_token", ResourceID: id,
		Details: map[string]any{"label": label, "expires_at": exp.Format(time.RFC3339)},
	})
	return &EnrollmentTokenResult{
		ID: id, Label: label, Token: raw, EnvironmentID: envID, ExpiresAt: exp.Format(time.RFC3339), CreatedAt: now.Format(time.RFC3339),
	}, nil
}

func (s *Service) EnrollAgent(ctx context.Context, req contracts.AgentEnrollRequest, remoteIP string) (*contracts.AgentEnrollResponse, error) {
	if strings.TrimSpace(req.EnrollmentToken) == "" || strings.TrimSpace(req.Hostname) == "" {
		return nil, errors.New("enrollment_token and hostname are required")
	}
	tokenHash := security.HashToken(strings.TrimSpace(req.EnrollmentToken))

	var tokID, orgID string
	var envID, usedAt sql.NullString
	var expRaw string
	err := s.db.QueryRow(ctx, `
		SELECT id, organization_id, environment_id, expires_at, used_at
		FROM agent_enrollment_tokens
		WHERE token_hash = ?`, tokenHash).Scan(&tokID, &orgID, &envID, &expRaw, &usedAt)
	if err != nil {
		return nil, errors.New("invalid enrollment token")
	}
	if usedAt.Valid && usedAt.String != "" {
		return nil, errors.New("enrollment token has already been used")
	}
	expTime, err := time.Parse(time.RFC3339, expRaw)
	if err != nil || time.Now().UTC().After(expTime) {
		return nil, errors.New("enrollment token has expired")
	}

	rawAgentToken, agentTokenHash, err := security.GenerateToken("infraos_agt", 32)
	if err != nil {
		return nil, err
	}
	prefix := rawAgentToken[:18]
	now := time.Now().UTC().Format(time.RFC3339)
	hostID := uuid.NewString()
	agentID := uuid.NewString()
	credID := uuid.NewString()
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = req.Hostname
	}
	tagsJSON, _ := json.Marshal(req.Tags)
	if len(req.Tags) == 0 {
		tagsJSON = []byte("[]")
	}
	agentVer := req.AgentVersion
	if agentVer == "" {
		agentVer = "0.1.0"
	}

	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, s.db.Rebind(`
			UPDATE agent_enrollment_tokens SET used_at = ? WHERE id = ? AND used_at IS NULL`), now, tokID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errors.New("enrollment token already consumed")
		}

		var envArg any
		if envID.Valid && envID.String != "" {
			envArg = envID.String
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO hosts (id, organization_id, environment_id, hostname, display_name, os, arch, kernel_version, ip_address, tags_json, status, is_synthetic, metadata_json, last_heartbeat_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'online', ?, '{}', ?, ?, ?)`),
			hostID, orgID, envArg, req.Hostname, displayName, req.OS, req.Arch, req.KernelVersion, req.IPAddress, string(tagsJSON),
			database.BoolToDB(s.db.Driver, false), now, now, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO agents (id, organization_id, host_id, version, status, last_heartbeat_at, last_ip, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'active', ?, ?, ?, ?)`),
			agentID, orgID, hostID, agentVer, now, remoteIP, now, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO agent_credentials (id, organization_id, agent_id, token_hash, prefix, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`),
			credID, orgID, agentID, agentTokenHash, prefix, now); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.RecordInput{
		OrgID: orgID, ActorAgentID: agentID, Action: "agent.enrolled", ResourceType: "host", ResourceID: hostID, IPAddress: remoteIP,
		Details: map[string]any{"hostname": req.Hostname, "os": req.OS, "arch": req.Arch, "agent_version": agentVer},
	})

	return &contracts.AgentEnrollResponse{
		AgentID: agentID, HostID: hostID, OrganizationID: orgID, AgentToken: rawAgentToken, TokenPrefix: prefix,
	}, nil
}

func (s *Service) AuthenticateAgent(ctx context.Context, rawToken string) (*AgentIdentity, error) {
	if strings.TrimSpace(rawToken) == "" {
		return nil, errors.New("missing agent credential")
	}
	hash := security.HashToken(strings.TrimSpace(rawToken))
	var ident AgentIdentity
	var agentStatus string
	var revokedAt, expiresAt sql.NullString

	err := s.db.QueryRow(ctx, `
		SELECT c.id, c.organization_id, a.id, a.host_id, a.status, c.revoked_at, c.expires_at
		FROM agent_credentials c
		JOIN agents a ON a.id = c.agent_id
		WHERE c.token_hash = ?`, hash).Scan(
		&ident.CredentialID, &ident.OrganizationID, &ident.AgentID, &ident.HostID, &agentStatus, &revokedAt, &expiresAt,
	)
	if err != nil {
		return nil, errors.New("invalid agent credential")
	}
	if agentStatus != "active" || (revokedAt.Valid && revokedAt.String != "") {
		return nil, errors.New("agent credential revoked")
	}
	if expiresAt.Valid && expiresAt.String != "" {
		if exp, err := time.Parse(time.RFC3339, expiresAt.String); err == nil && time.Now().UTC().After(exp) {
			return nil, errors.New("agent credential expired")
		}
	}
	return &ident, nil
}

func (s *Service) RotateAgentCredential(ctx context.Context, orgID, agentID, userID string) (string, string, error) {
	var oldCredID string
	err := s.db.QueryRow(ctx, `
		SELECT id FROM agent_credentials WHERE organization_id = ? AND agent_id = ? AND revoked_at IS NULL
		ORDER BY created_at DESC LIMIT 1`, orgID, agentID).Scan(&oldCredID)
	if err != nil {
		return "", "", errors.New("active agent credential not found")
	}
	raw, hash, err := security.GenerateToken("infraos_agt", 32)
	if err != nil {
		return "", "", err
	}
	prefix := raw[:18]
	now := time.Now().UTC().Format(time.RFC3339)

	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE agent_credentials SET revoked_at = ? WHERE id = ?`), now, oldCredID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO agent_credentials (id, organization_id, agent_id, token_hash, prefix, rotated_from_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`),
			uuid.NewString(), orgID, agentID, hash, prefix, oldCredID, now)
		return err
	})
	if err != nil {
		return "", "", err
	}
	_ = s.audit.Record(ctx, audit.RecordInput{
		OrgID: orgID, ActorUserID: userID, Action: "agent.credential.rotated", ResourceType: "agent", ResourceID: agentID,
	})
	return raw, prefix, nil
}

func (s *Service) RevokeAgent(ctx context.Context, orgID, agentID, userID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var hostID string
	if err := s.db.QueryRow(ctx, `SELECT host_id FROM agents WHERE organization_id = ? AND id = ?`, orgID, agentID).Scan(&hostID); err != nil {
		return errors.New("agent not found")
	}
	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE agents SET status = 'revoked', updated_at = ? WHERE id = ? AND organization_id = ?`), now, agentID, orgID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE agent_credentials SET revoked_at = ? WHERE agent_id = ? AND organization_id = ? AND revoked_at IS NULL`), now, agentID, orgID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE hosts SET status = 'revoked', updated_at = ? WHERE id = ? AND organization_id = ?`), now, hostID, orgID)
		return err
	})
	if err == nil {
		_ = s.audit.Record(ctx, audit.RecordInput{
			OrgID: orgID, ActorUserID: userID, Action: "agent.revoked", ResourceType: "agent", ResourceID: agentID,
			Details: map[string]any{"host_id": hostID},
		})
	}
	return err
}

func (s *Service) IngestMetrics(ctx context.Context, ident *AgentIdentity, payload contracts.HostMetricPayload, remoteIP string) error {
	recorded := payload.RecordedAt
	if recorded.IsZero() {
		recorded = time.Now().UTC()
	}
	nowStr := time.Now().UTC().Format(time.RFC3339)
	recStr := recorded.UTC().Format(time.RFC3339)
	fsJSON, _ := json.Marshal(payload.Filesystems)
	if len(payload.Filesystems) == 0 {
		fsJSON = []byte("[]")
	}
	ifaceJSON, _ := json.Marshal(payload.Interfaces)
	if len(payload.Interfaces) == 0 {
		ifaceJSON = []byte("[]")
	}
	optJSON, _ := json.Marshal(payload.Optional)
	metaJSON, _ := json.Marshal(map[string]any{
		"platform":         payload.Platform,
		"platform_version": payload.PlatformVersion,
	})

	err := s.db.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO host_metrics (
				organization_id, host_id, recorded_at, cpu_percent, cpu_cores,
				load_1m, load_5m, load_15m, mem_total_bytes, mem_used_bytes, mem_available_bytes, mem_percent,
				disk_total_bytes, disk_used_bytes, disk_percent, net_rx_bytes, net_tx_bytes, net_rx_bps, net_tx_bps,
				uptime_seconds, filesystems_json, interfaces_json, optional_json, is_synthetic
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			ident.OrganizationID, ident.HostID, recStr, payload.CPUPercent, payload.CPUCores,
			payload.Load1m, payload.Load5m, payload.Load15m, payload.MemTotalBytes, payload.MemUsedBytes, payload.MemAvailBytes, payload.MemPercent,
			payload.DiskTotalBytes, payload.DiskUsedBytes, payload.DiskPercent, payload.NetRxBytes, payload.NetTxBytes, payload.NetRxBps, payload.NetTxBps,
			payload.UptimeSeconds, string(fsJSON), string(ifaceJSON), string(optJSON), database.BoolToDB(s.db.Driver, false),
		); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			UPDATE hosts SET hostname = ?, os = ?, arch = ?, kernel_version = ?, ip_address = ?,
			                 metadata_json = ?, status = 'online', last_heartbeat_at = ?, updated_at = ?
			WHERE id = ? AND organization_id = ?`),
			payload.Hostname, payload.OS, payload.Arch, payload.KernelVersion, payload.IPAddress,
			string(metaJSON), nowStr, nowStr, ident.HostID, ident.OrganizationID,
		); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			UPDATE agents SET version = ?, last_heartbeat_at = ?, last_ip = ?, updated_at = ?
			WHERE id = ? AND organization_id = ?`),
			payload.AgentVersion, nowStr, remoteIP, nowStr, ident.AgentID, ident.OrganizationID,
		); err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE agent_credentials SET last_used_at = ? WHERE id = ?`), nowStr, ident.CredentialID)
		return err
	})
	if err != nil {
		if s.metrics != nil {
			s.metrics.DatabaseErrors.WithLabelValues("ingest_host_metrics").Inc()
		}
		return err
	}

	if s.metrics != nil {
		s.metrics.AgentHeartbeats.Inc()
		s.metrics.MetricsIngested.Inc()
	}
	if s.broker != nil {
		s.broker.Publish(ident.OrganizationID, "host.metrics", map[string]any{
			"host_id":      ident.HostID,
			"hostname":     payload.Hostname,
			"cpu_percent":  payload.CPUPercent,
			"mem_percent":  payload.MemPercent,
			"disk_percent": payload.DiskPercent,
			"net_rx_bps":   payload.NetRxBps,
			"net_tx_bps":   payload.NetTxBps,
			"recorded_at":  recStr,
		})
	}
	if s.onMetricIngest != nil {
		s.onMetricIngest(ctx, ident.OrganizationID, ident.HostID, payload)
	}
	return nil
}

func (s *Service) ListHosts(ctx context.Context, orgID, search, envID string) ([]Host, error) {
	where := "WHERE h.organization_id = ?"
	args := []any{orgID}
	if envID != "" {
		where += " AND h.environment_id = ?"
		args = append(args, envID)
	}
	if search = strings.TrimSpace(search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where += " AND (LOWER(h.hostname) LIKE ? OR LOWER(h.display_name) LIKE ? OR LOWER(h.tags_json) LIKE ? OR LOWER(h.ip_address) LIKE ?)"
		args = append(args, like, like, like, like)
	}

	rows, err := s.db.Query(ctx, `
		SELECT h.id, h.organization_id, COALESCE(h.environment_id, ''), COALESCE(e.name, ''),
		       h.hostname, h.display_name, h.os, h.arch, h.kernel_version, h.ip_address,
		       h.tags_json, h.status, CASE WHEN h.is_synthetic THEN 1 ELSE 0 END,
		       h.metadata_json, COALESCE(h.last_heartbeat_at, ''),
		       COALESCE(a.id, ''), COALESCE(a.version, ''), COALESCE(a.status, ''),
		       h.created_at, h.updated_at
		FROM hosts h
		LEFT JOIN environments e ON e.id = h.environment_id
		LEFT JOIN agents a ON a.host_id = h.id
		`+where+`
		ORDER BY h.is_synthetic ASC, h.updated_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Host, 0)
	for rows.Next() {
		var h Host
		var tagsRaw, metaRaw string
		var synthInt int
		if err := rows.Scan(
			&h.ID, &h.OrgID, &h.EnvironmentID, &h.EnvironmentName,
			&h.Hostname, &h.DisplayName, &h.OS, &h.Arch, &h.KernelVersion, &h.IPAddress,
			&tagsRaw, &h.Status, &synthInt, &metaRaw, &h.LastHeartbeatAt,
			&h.AgentID, &h.AgentVersion, &h.AgentStatus, &h.CreatedAt, &h.UpdatedAt,
		); err != nil {
			return nil, err
		}
		h.IsSynthetic = synthInt == 1
		h.Tags = []string{}
		_ = json.Unmarshal([]byte(tagsRaw), &h.Tags)
		h.Metadata = map[string]any{}
		_ = json.Unmarshal([]byte(metaRaw), &h.Metadata)
		h.Status, h.Freshness = computeHostFreshness(h.Status, h.LastHeartbeatAt, h.IsSynthetic)
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range out {
		if m, fs, ifaces, opt, err := s.latestMetricForHost(ctx, orgID, out[i].ID); err == nil && m != nil {
			out[i].LatestMetric = m
			out[i].Filesystems = fs
			out[i].Interfaces = ifaces
			out[i].Optional = opt
		}
	}
	return out, nil
}

func (s *Service) GetHost(ctx context.Context, orgID, hostID string) (*Host, error) {
	list, err := s.ListHosts(ctx, orgID, "", "")
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == hostID {
			h := list[i]
			h.SupportedRanges = s.supportedRanges(ctx, orgID, hostID)
			return &h, nil
		}
	}
	return nil, errors.New("host not found")
}

func (s *Service) UpdateHost(ctx context.Context, orgID, hostID, displayName, envID string, tags []string) error {
	tagsJSON, _ := json.Marshal(tags)
	if len(tags) == 0 {
		tagsJSON = []byte("[]")
	}
	var envArg any
	if envID != "" {
		envArg = envID
	}
	res, err := s.db.Exec(ctx, `
		UPDATE hosts SET display_name = ?, environment_id = ?, tags_json = ?, updated_at = ?
		WHERE id = ? AND organization_id = ?`,
		displayName, envArg, string(tagsJSON), time.Now().UTC().Format(time.RFC3339), hostID, orgID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("host not found")
	}
	return nil
}

func (s *Service) GetHostMetrics(ctx context.Context, orgID, hostID, rangeKey string) ([]MetricPoint, string, error) {
	dur := 15 * time.Minute
	switch rangeKey {
	case "15m":
		dur = 15 * time.Minute
	case "1h":
		dur = time.Hour
	case "24h":
		dur = 24 * time.Hour
	case "7d":
		dur = 7 * 24 * time.Hour
	default:
		rangeKey = "1h"
		dur = time.Hour
	}

	cutoff := time.Now().UTC().Add(-dur).Format(time.RFC3339)
	rows, err := s.db.Query(ctx, `
		SELECT recorded_at, cpu_percent, cpu_cores, load_1m, load_5m, load_15m,
		       mem_total_bytes, mem_used_bytes, mem_available_bytes, mem_percent,
		       disk_total_bytes, disk_used_bytes, disk_percent,
		       net_rx_bytes, net_tx_bytes, net_rx_bps, net_tx_bps, uptime_seconds,
		       CASE WHEN is_synthetic THEN 1 ELSE 0 END
		FROM host_metrics
		WHERE organization_id = ? AND host_id = ? AND recorded_at >= ?
		ORDER BY recorded_at ASC`, orgID, hostID, cutoff)
	if err != nil {
		return nil, rangeKey, err
	}
	defer rows.Close()

	points := make([]MetricPoint, 0)
	for rows.Next() {
		var p MetricPoint
		var synthInt int
		if err := rows.Scan(
			&p.RecordedAt, &p.CPUPercent, &p.CPUCores, &p.Load1m, &p.Load5m, &p.Load15m,
			&p.MemTotalBytes, &p.MemUsedBytes, &p.MemAvailBytes, &p.MemPercent,
			&p.DiskTotal, &p.DiskUsed, &p.DiskPercent,
			&p.NetRxBytes, &p.NetTxBytes, &p.NetRxBps, &p.NetTxBps, &p.UptimeSeconds,
			&synthInt,
		); err != nil {
			return nil, rangeKey, err
		}
		p.IsSynthetic = synthInt == 1
		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, rangeKey, err
	}

	return downsampleMetrics(points, 120), rangeKey, nil
}

func (s *Service) latestMetricForHost(ctx context.Context, orgID, hostID string) (*MetricPoint, []contracts.FilesystemMetric, []contracts.NetworkInterfaceMetric, *contracts.OptionalMetrics, error) {
	var p MetricPoint
	var fsRaw, ifRaw, optRaw string
	var synthInt int
	err := s.db.QueryRow(ctx, `
		SELECT recorded_at, cpu_percent, cpu_cores, load_1m, load_5m, load_15m,
		       mem_total_bytes, mem_used_bytes, mem_available_bytes, mem_percent,
		       disk_total_bytes, disk_used_bytes, disk_percent,
		       net_rx_bytes, net_tx_bytes, net_rx_bps, net_tx_bps, uptime_seconds,
		       filesystems_json, interfaces_json, optional_json,
		       CASE WHEN is_synthetic THEN 1 ELSE 0 END
		FROM host_metrics
		WHERE organization_id = ? AND host_id = ?
		ORDER BY id DESC LIMIT 1`, orgID, hostID).Scan(
		&p.RecordedAt, &p.CPUPercent, &p.CPUCores, &p.Load1m, &p.Load5m, &p.Load15m,
		&p.MemTotalBytes, &p.MemUsedBytes, &p.MemAvailBytes, &p.MemPercent,
		&p.DiskTotal, &p.DiskUsed, &p.DiskPercent,
		&p.NetRxBytes, &p.NetTxBytes, &p.NetRxBps, &p.NetTxBps, &p.UptimeSeconds,
		&fsRaw, &ifRaw, &optRaw, &synthInt,
	)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	p.IsSynthetic = synthInt == 1
	var fs []contracts.FilesystemMetric
	var ifaces []contracts.NetworkInterfaceMetric
	var opt contracts.OptionalMetrics
	_ = json.Unmarshal([]byte(fsRaw), &fs)
	_ = json.Unmarshal([]byte(ifRaw), &ifaces)
	_ = json.Unmarshal([]byte(optRaw), &opt)
	return &p, fs, ifaces, &opt, nil
}

func (s *Service) supportedRanges(ctx context.Context, orgID, hostID string) []string {
	var oldest sql.NullString
	_ = s.db.QueryRow(ctx, `SELECT MIN(recorded_at) FROM host_metrics WHERE organization_id = ? AND host_id = ?`, orgID, hostID).Scan(&oldest)
	if !oldest.Valid || oldest.String == "" {
		return []string{}
	}
	t, err := time.Parse(time.RFC3339, oldest.String)
	if err != nil {
		return []string{"15m"}
	}
	age := time.Since(t)
	ranges := []string{"15m"}
	if age >= 20*time.Minute {
		ranges = append(ranges, "1h")
	}
	if age >= 2*time.Hour {
		ranges = append(ranges, "24h")
	}
	if age >= 26*time.Hour {
		ranges = append(ranges, "7d")
	}
	return ranges
}

func computeHostFreshness(storedStatus, lastHB string, isSynthetic bool) (string, string) {
	if isSynthetic {
		return storedStatus, "synthetic"
	}
	if storedStatus == "revoked" {
		return "revoked", "unavailable"
	}
	if lastHB == "" {
		return "pending", "unavailable"
	}
	t, err := time.Parse(time.RFC3339, lastHB)
	if err != nil {
		return storedStatus, "stale"
	}
	age := time.Since(t)
	switch {
	case age <= 45*time.Second:
		return "online", "live"
	case age <= 3*time.Minute:
		return "stale", "stale"
	default:
		return "offline", "stale"
	}
}

func downsampleMetrics(points []MetricPoint, maxPoints int) []MetricPoint {
	if len(points) <= maxPoints || maxPoints <= 0 {
		return points
	}
	step := float64(len(points)) / float64(maxPoints)
	out := make([]MetricPoint, 0, maxPoints)
	for i := 0; i < maxPoints; i++ {
		start := int(float64(i) * step)
		end := int(float64(i+1) * step)
		if end > len(points) {
			end = len(points)
		}
		if start >= end {
			continue
		}
		bucket := points[start:end]
		agg := bucket[len(bucket)-1]
		var sumCPU, sumMem, sumDisk, sumRx, sumTx float64
		for _, p := range bucket {
			sumCPU += p.CPUPercent
			sumMem += p.MemPercent
			sumDisk += p.DiskPercent
			sumRx += p.NetRxBps
			sumTx += p.NetTxBps
		}
		n := float64(len(bucket))
		agg.CPUPercent = float64(int(sumCPU/n*100)) / 100
		agg.MemPercent = float64(int(sumMem/n*100)) / 100
		agg.DiskPercent = float64(int(sumDisk/n*100)) / 100
		agg.NetRxBps = float64(int(sumRx/n*100)) / 100
		agg.NetTxBps = float64(int(sumTx/n*100)) / 100
		out = append(out, agg)
	}
	return out
}

func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
