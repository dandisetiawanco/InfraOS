package logs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/database"
	"github.com/dandisetiawanco/InfraOS/internal/security"
	"github.com/dandisetiawanco/InfraOS/internal/sse"
	"github.com/google/uuid"
)

type Entry struct {
	ID            int64          `json:"id"`
	OrgID         string         `json:"organization_id"`
	SourceID      string         `json:"source_id,omitempty"`
	HostID        string         `json:"host_id,omitempty"`
	HostName      string         `json:"host_name,omitempty"`
	ServiceID     string         `json:"service_id,omitempty"`
	ServiceName   string         `json:"service_name,omitempty"`
	EnvironmentID string         `json:"environment_id,omitempty"`
	Timestamp     string         `json:"timestamp"`
	Severity      string         `json:"severity"`
	SourceName    string         `json:"source_name"`
	Message       string         `json:"message"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Fields        map[string]any `json:"fields"`
	IsSynthetic   bool           `json:"is_synthetic"`
}

type IngestInput struct {
	OrgID         string
	SourceID      string
	HostID        string
	ServiceID     string
	EnvironmentID string
	Severity      string
	SourceName    string
	Message       string
	CorrelationID string
	Fields        map[string]any
	IsSynthetic   bool
	Timestamp     time.Time
}

type Filter struct {
	Search        string
	Severity      string
	Source        string
	HostID        string
	ServiceID     string
	EnvironmentID string
	Range         string
	Limit         int
	Offset        int
}

type Service struct {
	db     *database.DB
	broker *sse.Broker
}

func NewService(db *database.DB, broker *sse.Broker) *Service {
	return &Service{db: db, broker: broker}
}

func (s *Service) Ingest(ctx context.Context, in IngestInput) (*Entry, error) {
	if in.OrgID == "" || strings.TrimSpace(in.Message) == "" {
		return nil, errors.New("organization_id and message are required")
	}
	sev := strings.ToLower(strings.TrimSpace(in.Severity))
	switch sev {
	case "debug", "info", "warn", "error", "fatal":
	default:
		sev = "info"
	}
	src := strings.TrimSpace(in.SourceName)
	if src == "" {
		src = "application"
	}
	ts := in.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	tsStr := ts.UTC().Format(time.RFC3339)

	// Redact credentials before persistence to prevent secret leakage in log queries or exports.
	redactedMsg := security.RedactCredentials(in.Message)
	if len(redactedMsg) > 4096 {
		redactedMsg = redactedMsg[:4096] + "...[truncated]"
	}

	fieldsJSON := "{}"
	if in.Fields != nil {
		if raw, err := json.Marshal(in.Fields); err == nil {
			fieldsJSON = security.RedactCredentials(string(raw))
		}
	}

	var srcArg, hostArg, svcArg, envArg any
	if in.SourceID != "" {
		srcArg = in.SourceID
	}
	if in.HostID != "" {
		hostArg = in.HostID
	}
	if in.ServiceID != "" {
		svcArg = in.ServiceID
	}
	if in.EnvironmentID != "" {
		envArg = in.EnvironmentID
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO log_entries (organization_id, source_id, host_id, service_id, environment_id, timestamp, severity, source_name, message, correlation_id, fields_json, is_synthetic)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.OrgID, srcArg, hostArg, svcArg, envArg, tsStr, sev, src, redactedMsg, in.CorrelationID, fieldsJSON, database.BoolToDB(s.db.Driver, in.IsSynthetic),
	)
	if err != nil {
		return nil, err
	}

	entry := &Entry{
		OrgID:         in.OrgID,
		SourceID:      in.SourceID,
		HostID:        in.HostID,
		ServiceID:     in.ServiceID,
		EnvironmentID: in.EnvironmentID,
		Timestamp:     tsStr,
		Severity:      sev,
		SourceName:    src,
		Message:       redactedMsg,
		CorrelationID: in.CorrelationID,
		Fields:        map[string]any{},
		IsSynthetic:   in.IsSynthetic,
	}
	_ = json.Unmarshal([]byte(fieldsJSON), &entry.Fields)

	if s.broker != nil && !in.IsSynthetic {
		s.broker.Publish(in.OrgID, "log.entry", entry)
	}
	return entry, nil
}

func (s *Service) Query(ctx context.Context, orgID string, f Filter) ([]Entry, int, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	where := "WHERE l.organization_id = ?"
	args := []any{orgID}
	if f.Severity != "" {
		where += " AND l.severity = ?"
		args = append(args, strings.ToLower(f.Severity))
	}
	if f.Source != "" {
		where += " AND l.source_name = ?"
		args = append(args, f.Source)
	}
	if f.HostID != "" {
		where += " AND l.host_id = ?"
		args = append(args, f.HostID)
	}
	if f.ServiceID != "" {
		where += " AND l.service_id = ?"
		args = append(args, f.ServiceID)
	}
	if f.EnvironmentID != "" {
		where += " AND l.environment_id = ?"
		args = append(args, f.EnvironmentID)
	}
	if q := strings.TrimSpace(f.Search); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		where += " AND (LOWER(l.message) LIKE ? OR LOWER(l.source_name) LIKE ? OR LOWER(l.correlation_id) LIKE ?)"
		args = append(args, like, like, like)
	}
	if cutoff := rangeCutoff(f.Range); !cutoff.IsZero() {
		where += " AND l.timestamp >= ?"
		args = append(args, cutoff.Format(time.RFC3339))
	}

	var total int
	if err := s.db.QueryRow(ctx, "SELECT COUNT(*) FROM log_entries l "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT l.id, l.organization_id, COALESCE(l.source_id, ''), COALESCE(l.host_id, ''), COALESCE(h.hostname, ''),
		       COALESCE(l.service_id, ''), COALESCE(sv.name, ''), COALESCE(l.environment_id, ''),
		       l.timestamp, l.severity, l.source_name, l.message, l.correlation_id, l.fields_json,
		       CASE WHEN l.is_synthetic THEN 1 ELSE 0 END
		FROM log_entries l
		LEFT JOIN hosts h ON h.id = l.host_id
		LEFT JOIN services sv ON sv.id = l.service_id
		` + where + `
		ORDER BY l.timestamp DESC, l.id DESC
		LIMIT ? OFFSET ?`
	queryArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	rows, err := s.db.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries := make([]Entry, 0)
	for rows.Next() {
		var e Entry
		var fieldsRaw string
		var synthInt int
		if err := rows.Scan(
			&e.ID, &e.OrgID, &e.SourceID, &e.HostID, &e.HostName,
			&e.ServiceID, &e.ServiceName, &e.EnvironmentID,
			&e.Timestamp, &e.Severity, &e.SourceName, &e.Message, &e.CorrelationID, &fieldsRaw, &synthInt,
		); err != nil {
			return nil, 0, err
		}
		e.IsSynthetic = synthInt == 1
		e.Fields = map[string]any{}
		_ = json.Unmarshal([]byte(fieldsRaw), &e.Fields)
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

func (s *Service) ExportCSV(ctx context.Context, orgID string, f Filter, w io.Writer) error {
	f.Limit = 200
	entries, _, err := s.Query(ctx, orgID, f)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"timestamp", "severity", "source", "host", "service", "correlation_id", "message", "is_synthetic"})
	for _, e := range entries {
		synth := "false"
		if e.IsSynthetic {
			synth = "true"
		}
		_ = cw.Write([]string{e.Timestamp, e.Severity, e.SourceName, e.HostName, e.ServiceName, e.CorrelationID, e.Message, synth})
	}
	cw.Flush()
	return cw.Error()
}

func (s *Service) EnsureDefaultSource(ctx context.Context, orgID, name, sourceType string) string {
	var id string
	err := s.db.QueryRow(ctx, `SELECT id FROM log_sources WHERE organization_id = ? AND name = ?`, orgID, name).Scan(&id)
	if err == nil {
		return id
	}
	id = uuid.NewString()
	_, _ = s.db.Exec(ctx, `
		INSERT INTO log_sources (id, organization_id, name, source_type, retention_days, enabled, created_at)
		VALUES (?, ?, ?, ?, 14, ?, ?)`,
		id, orgID, name, sourceType, database.BoolToDB(s.db.Driver, true), time.Now().UTC().Format(time.RFC3339))
	return id
}

func rangeCutoff(r string) time.Time {
	now := time.Now().UTC()
	switch r {
	case "15m":
		return now.Add(-15 * time.Minute)
	case "1h":
		return now.Add(-time.Hour)
	case "24h":
		return now.Add(-24 * time.Hour)
	case "7d":
		return now.Add(-7 * 24 * time.Hour)
	default:
		return time.Time{}
	}
}
