package incidents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/audit"
	"github.com/dandisetiawanco/InfraOS/internal/database"
	"github.com/dandisetiawanco/InfraOS/internal/sse"
	"github.com/google/uuid"
)

type Incident struct {
	ID                 string          `json:"id"`
	OrgID              string          `json:"organization_id"`
	EnvironmentID      string          `json:"environment_id,omitempty"`
	EnvironmentName    string          `json:"environment_name,omitempty"`
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	Severity           string          `json:"severity"`
	Status             string          `json:"status"`
	AffectedServiceIDs []string        `json:"affected_service_ids"`
	AssignedUserID     string          `json:"assigned_user_id,omitempty"`
	AssignedEmail      string          `json:"assigned_email,omitempty"`
	RootCauseNotes     string          `json:"root_cause_notes"`
	ResolutionSummary  string          `json:"resolution_summary"`
	PostmortemMarkdown string          `json:"postmortem_markdown"`
	DedupeKey          string          `json:"dedupe_key,omitempty"`
	IsSynthetic        bool            `json:"is_synthetic"`
	OpenedAt           string          `json:"opened_at"`
	ResolvedAt         string          `json:"resolved_at,omitempty"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
	Timeline           []TimelineEntry `json:"timeline,omitempty"`
}

type TimelineEntry struct {
	ID          int64          `json:"id"`
	IncidentID  string         `json:"incident_id"`
	EventType   string         `json:"event_type"`
	AuthorID    string         `json:"author_id,omitempty"`
	AuthorEmail string         `json:"author_email,omitempty"`
	Message     string         `json:"message"`
	Metadata    map[string]any `json:"metadata"`
	CreatedAt   string         `json:"created_at"`
}

type CreateInput struct {
	EnvironmentID      string
	Title              string
	Description        string
	Severity           string
	AffectedServiceIDs []string
	AssignedUserID     string
	DedupeKey          string
	AuthorID           string
	IsSynthetic        bool
}

type UpdateInput struct {
	Status             string
	Severity           string
	AssignedUserID     string
	RootCauseNotes     string
	ResolutionSummary  string
	PostmortemMarkdown string
	AuthorID           string
}

type Service struct {
	db     *database.DB
	audit  *audit.Recorder
	broker *sse.Broker
}

func NewService(db *database.DB, auditRec *audit.Recorder, broker *sse.Broker) *Service {
	return &Service{db: db, audit: auditRec, broker: broker}
}

func (s *Service) Create(ctx context.Context, orgID string, in CreateInput) (*Incident, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return nil, errors.New("incident title is required")
	}
	sev := strings.ToLower(strings.TrimSpace(in.Severity))
	switch sev {
	case "sev1", "sev2", "sev3", "sev4":
	default:
		sev = "sev3"
	}

	// Deduplicate open incidents created from automated alert rules.
	if in.DedupeKey != "" {
		var existingID string
		err := s.db.QueryRow(ctx, `
			SELECT id FROM incidents
			WHERE organization_id = ? AND dedupe_key = ? AND status != 'resolved'
			ORDER BY opened_at DESC LIMIT 1`, orgID, in.DedupeKey).Scan(&existingID)
		if err == nil {
			return s.Get(ctx, orgID, existingID)
		}
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	svcsJSON, _ := json.Marshal(in.AffectedServiceIDs)
	if len(in.AffectedServiceIDs) == 0 {
		svcsJSON = []byte("[]")
	}
	var envArg, assignArg, authorArg any
	if in.EnvironmentID != "" {
		envArg = in.EnvironmentID
	}
	if in.AssignedUserID != "" {
		assignArg = in.AssignedUserID
	}
	if in.AuthorID != "" {
		authorArg = in.AuthorID
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO incidents (id, organization_id, environment_id, title, description, severity, status, affected_service_ids_json, assigned_user_id, dedupe_key, is_synthetic, opened_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 'open', ?, ?, ?, ?, ?, ?, ?)`,
		id, orgID, envArg, in.Title, in.Description, sev, string(svcsJSON), assignArg, in.DedupeKey,
		database.BoolToDB(s.db.Driver, in.IsSynthetic), now, now, now,
	)
	if err != nil {
		return nil, err
	}

	_, _ = s.db.Exec(ctx, `
		INSERT INTO incident_timelines (organization_id, incident_id, event_type, author_id, message, metadata_json, created_at)
		VALUES (?, ?, 'status_change', ?, ?, '{}', ?)`,
		orgID, id, authorArg, "Incident opened with severity "+strings.ToUpper(sev), now)

	_ = s.audit.Record(ctx, audit.RecordInput{
		OrgID: orgID, ActorUserID: in.AuthorID, Action: "incident.created", ResourceType: "incident", ResourceID: id,
		Details: map[string]any{"title": in.Title, "severity": sev},
	})
	if s.broker != nil && !in.IsSynthetic {
		s.broker.Publish(orgID, "incident.updated", map[string]any{"incident_id": id, "status": "open", "title": in.Title})
	}
	return s.Get(ctx, orgID, id)
}

func (s *Service) Update(ctx context.Context, orgID, incidentID string, in UpdateInput) (*Incident, error) {
	curr, err := s.Get(ctx, orgID, incidentID)
	if err != nil {
		return nil, err
	}
	status := curr.Status
	if in.Status != "" {
		switch strings.ToLower(in.Status) {
		case "open", "investigating", "identified", "monitoring", "resolved":
			status = strings.ToLower(in.Status)
		default:
			return nil, errors.New("invalid incident status")
		}
	}
	sev := curr.Severity
	if in.Severity != "" {
		switch strings.ToLower(in.Severity) {
		case "sev1", "sev2", "sev3", "sev4":
			sev = strings.ToLower(in.Severity)
		}
	}
	assigned := curr.AssignedUserID
	if in.AssignedUserID != "" {
		assigned = in.AssignedUserID
	}
	rootCause := curr.RootCauseNotes
	if in.RootCauseNotes != "" {
		rootCause = in.RootCauseNotes
	}
	resolution := curr.ResolutionSummary
	if in.ResolutionSummary != "" {
		resolution = in.ResolutionSummary
	}
	postmortem := curr.PostmortemMarkdown
	if in.PostmortemMarkdown != "" {
		postmortem = in.PostmortemMarkdown
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var resolvedArg, assignArg, authorArg any
	if status == "resolved" {
		if curr.ResolvedAt != "" {
			resolvedArg = curr.ResolvedAt
		} else {
			resolvedArg = now
		}
	}
	if assigned != "" {
		assignArg = assigned
	}
	if in.AuthorID != "" {
		authorArg = in.AuthorID
	}

	_, err = s.db.Exec(ctx, `
		UPDATE incidents
		SET status = ?, severity = ?, assigned_user_id = ?, root_cause_notes = ?, resolution_summary = ?,
		    postmortem_markdown = ?, resolved_at = ?, updated_at = ?
		WHERE id = ? AND organization_id = ?`,
		status, sev, assignArg, rootCause, resolution, postmortem, resolvedArg, now, incidentID, orgID)
	if err != nil {
		return nil, err
	}

	if status != curr.Status {
		_, _ = s.db.Exec(ctx, `
			INSERT INTO incident_timelines (organization_id, incident_id, event_type, author_id, message, metadata_json, created_at)
			VALUES (?, ?, 'status_change', ?, ?, '{}', ?)`,
			orgID, incidentID, authorArg, "Status changed from "+curr.Status+" to "+status, now)
	}
	if in.PostmortemMarkdown != "" && in.PostmortemMarkdown != curr.PostmortemMarkdown {
		_, _ = s.db.Exec(ctx, `
			INSERT INTO incident_timelines (organization_id, incident_id, event_type, author_id, message, metadata_json, created_at)
			VALUES (?, ?, 'postmortem', ?, 'Postmortem documentation updated.', '{}', ?)`,
			orgID, incidentID, authorArg, now)
	}

	_ = s.audit.Record(ctx, audit.RecordInput{
		OrgID: orgID, ActorUserID: in.AuthorID, Action: "incident.updated", ResourceType: "incident", ResourceID: incidentID,
		Details: map[string]any{"status": status, "severity": sev},
	})
	if s.broker != nil {
		s.broker.Publish(orgID, "incident.updated", map[string]any{"incident_id": incidentID, "status": status})
	}
	return s.Get(ctx, orgID, incidentID)
}

func (s *Service) AddTimelineEvent(ctx context.Context, orgID, incidentID, eventType, authorID, message string, metadata map[string]any) error {
	message = strings.TrimSpace(message)
	if message == "" {
		return errors.New("timeline message is required")
	}
	switch eventType {
	case "status_change", "note", "alert_linked", "deployment_linked", "ai_summary", "postmortem":
	default:
		eventType = "note"
	}
	metaJSON := "{}"
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			metaJSON = string(b)
		}
	}
	var authorArg any
	if authorID != "" {
		authorArg = authorID
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO incident_timelines (organization_id, incident_id, event_type, author_id, message, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		orgID, incidentID, eventType, authorArg, message, metaJSON, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Service) List(ctx context.Context, orgID, statusFilter string) ([]Incident, error) {
	where := "WHERE i.organization_id = ?"
	args := []any{orgID}
	if statusFilter != "" {
		where += " AND i.status = ?"
		args = append(args, strings.ToLower(statusFilter))
	}
	rows, err := s.db.Query(ctx, `
		SELECT i.id, i.organization_id, COALESCE(i.environment_id, ''), COALESCE(e.name, ''),
		       i.title, i.description, i.severity, i.status, i.affected_service_ids_json,
		       COALESCE(i.assigned_user_id, ''), COALESCE(u.email, ''),
		       i.root_cause_notes, i.resolution_summary, i.postmortem_markdown, i.dedupe_key,
		       CASE WHEN i.is_synthetic THEN 1 ELSE 0 END, i.opened_at, COALESCE(i.resolved_at, ''),
		       i.created_at, i.updated_at
		FROM incidents i
		LEFT JOIN environments e ON e.id = i.environment_id
		LEFT JOIN users u ON u.id = i.assigned_user_id
		`+where+`
		ORDER BY CASE WHEN i.status = 'resolved' THEN 1 ELSE 0 END ASC, i.opened_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Incident, 0)
	for rows.Next() {
		var inc Incident
		var svcsRaw string
		var synthInt int
		if err := rows.Scan(
			&inc.ID, &inc.OrgID, &inc.EnvironmentID, &inc.EnvironmentName,
			&inc.Title, &inc.Description, &inc.Severity, &inc.Status, &svcsRaw,
			&inc.AssignedUserID, &inc.AssignedEmail, &inc.RootCauseNotes, &inc.ResolutionSummary,
			&inc.PostmortemMarkdown, &inc.DedupeKey, &synthInt, &inc.OpenedAt, &inc.ResolvedAt,
			&inc.CreatedAt, &inc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		inc.IsSynthetic = synthInt == 1
		inc.AffectedServiceIDs = []string{}
		_ = json.Unmarshal([]byte(svcsRaw), &inc.AffectedServiceIDs)
		out = append(out, inc)
	}
	return out, rows.Err()
}

func (s *Service) Get(ctx context.Context, orgID, incidentID string) (*Incident, error) {
	list, err := s.List(ctx, orgID, "")
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == incidentID {
			inc := list[i]
			rows, err := s.db.Query(ctx, `
				SELECT t.id, t.incident_id, t.event_type, COALESCE(t.author_id, ''), COALESCE(u.email, ''),
				       t.message, t.metadata_json, t.created_at
				FROM incident_timelines t
				LEFT JOIN users u ON u.id = t.author_id
				WHERE t.organization_id = ? AND t.incident_id = ?
				ORDER BY t.id ASC`, orgID, incidentID)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var te TimelineEntry
					var metaRaw string
					if err := rows.Scan(&te.ID, &te.IncidentID, &te.EventType, &te.AuthorID, &te.AuthorEmail, &te.Message, &metaRaw, &te.CreatedAt); err == nil {
						te.Metadata = map[string]any{}
						_ = json.Unmarshal([]byte(metaRaw), &te.Metadata)
						inc.Timeline = append(inc.Timeline, te)
					}
				}
			}
			return &inc, nil
		}
	}
	return nil, errors.New("incident not found")
}
