package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/database"
)

type Event struct {
	ID           int64          `json:"id"`
	OrgID        string         `json:"organization_id"`
	ActorUserID  string         `json:"actor_user_id,omitempty"`
	ActorEmail   string         `json:"actor_email,omitempty"`
	ActorAgentID string         `json:"actor_agent_id,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Status       string         `json:"status"`
	IPAddress    string         `json:"ip_address"`
	RequestID    string         `json:"request_id"`
	Details      map[string]any `json:"details"`
	CreatedAt    string         `json:"created_at"`
}

type Recorder struct {
	db *database.DB
}

func NewRecorder(db *database.DB) *Recorder {
	return &Recorder{db: db}
}

type RecordInput struct {
	OrgID        string
	ActorUserID  string
	ActorAgentID string
	Action       string
	ResourceType string
	ResourceID   string
	Status       string
	IPAddress    string
	RequestID    string
	Details      map[string]any
}

func (r *Recorder) Record(ctx context.Context, in RecordInput) error {
	if in.OrgID == "" {
		return nil
	}
	if in.Status == "" {
		in.Status = "success"
	}
	detailsBytes := []byte("{}")
	if in.Details != nil {
		if b, err := json.Marshal(in.Details); err == nil {
			detailsBytes = b
		}
	}
	var userID, agentID any
	if in.ActorUserID != "" {
		userID = in.ActorUserID
	}
	if in.ActorAgentID != "" {
		agentID = in.ActorAgentID
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO audit_events (organization_id, actor_user_id, actor_agent_id, action, resource_type, resource_id, status, ip_address, request_id, details_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.OrgID, userID, agentID, in.Action, in.ResourceType, in.ResourceID, in.Status, in.IPAddress, in.RequestID, string(detailsBytes), time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func (r *Recorder) List(ctx context.Context, orgID, action, resourceType string, limit, offset int) ([]Event, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	where := "WHERE a.organization_id = ?"
	args := []any{orgID}
	if action != "" {
		where += " AND a.action = ?"
		args = append(args, action)
	}
	if resourceType != "" {
		where += " AND a.resource_type = ?"
		args = append(args, resourceType)
	}

	var total int
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM audit_events a "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT a.id, a.organization_id, COALESCE(a.actor_user_id, ''), COALESCE(u.email, ''), COALESCE(a.actor_agent_id, ''),
		       a.action, a.resource_type, a.resource_id, a.status, a.ip_address, a.request_id, a.details_json, a.created_at
		FROM audit_events a
		LEFT JOIN users u ON u.id = a.actor_user_id
		` + where + `
		ORDER BY a.id DESC
		LIMIT ? OFFSET ?`
	queryArgs := append(append([]any{}, args...), limit, offset)
	rows, err := r.db.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	events := make([]Event, 0)
	for rows.Next() {
		var ev Event
		var detailsRaw string
		if err := rows.Scan(&ev.ID, &ev.OrgID, &ev.ActorUserID, &ev.ActorEmail, &ev.ActorAgentID,
			&ev.Action, &ev.ResourceType, &ev.ResourceID, &ev.Status, &ev.IPAddress, &ev.RequestID, &detailsRaw, &ev.CreatedAt); err != nil {
			return nil, 0, err
		}
		ev.Details = map[string]any{}
		_ = json.Unmarshal([]byte(detailsRaw), &ev.Details)
		events = append(events, ev)
	}
	return events, total, rows.Err()
}
