package deployments

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/audit"
	"github.com/dandisetiawanco/InfraOS/internal/database"
	"github.com/dandisetiawanco/InfraOS/internal/logs"
	"github.com/dandisetiawanco/InfraOS/internal/queue"
	"github.com/dandisetiawanco/InfraOS/internal/security"
	"github.com/dandisetiawanco/InfraOS/internal/sse"
	"github.com/google/uuid"
)

type Target struct {
	ID                   string `json:"id"`
	OrgID                string `json:"organization_id"`
	ProjectID            string `json:"project_id,omitempty"`
	EnvironmentID        string `json:"environment_id,omitempty"`
	EnvironmentName      string `json:"environment_name,omitempty"`
	HostID               string `json:"host_id,omitempty"`
	Name                 string `json:"name"`
	Adapter              string `json:"adapter"`
	HealthURL            string `json:"health_url,omitempty"`
	CurrentVersion       string `json:"current_version,omitempty"`
	CurrentArtifactID    string `json:"current_artifact_id,omitempty"`
	LockedByDeploymentID string `json:"locked_by_deployment_id,omitempty"`
	IsSynthetic          bool   `json:"is_synthetic"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

type Deployment struct {
	ID                 string  `json:"id"`
	OrgID              string  `json:"organization_id"`
	TargetID           string  `json:"target_id"`
	TargetName         string  `json:"target_name,omitempty"`
	Adapter            string  `json:"adapter,omitempty"`
	ArtifactID         string  `json:"artifact_id,omitempty"`
	PreviousArtifactID string  `json:"previous_artifact_id,omitempty"`
	EnvironmentID      string  `json:"environment_id,omitempty"`
	EnvironmentName    string  `json:"environment_name,omitempty"`
	IdempotencyKey     string  `json:"idempotency_key,omitempty"`
	Version            string  `json:"version"`
	CommitSHA          string  `json:"commit_sha"`
	CommitMessage      string  `json:"commit_message"`
	Branch             string  `json:"branch"`
	Status             string  `json:"status"`
	TriggerType        string  `json:"trigger_type"`
	IsSimulation       bool    `json:"is_simulation"`
	TriggeredBy        string  `json:"triggered_by,omitempty"`
	TriggeredByEmail   string  `json:"triggered_by_email,omitempty"`
	StartedAt          string  `json:"started_at,omitempty"`
	FinishedAt         string  `json:"finished_at,omitempty"`
	ErrorMessage       string  `json:"error_message,omitempty"`
	CreatedAt          string  `json:"created_at"`
	Events             []Event `json:"events,omitempty"`
}

type Event struct {
	ID           int64          `json:"id"`
	DeploymentID string         `json:"deployment_id"`
	State        string         `json:"state"`
	Level        string         `json:"level"`
	Message      string         `json:"message"`
	Details      map[string]any `json:"details"`
	CreatedAt    string         `json:"created_at"`
}

type CreateTargetInput struct {
	Name          string
	ProjectID     string
	EnvironmentID string
	HostID        string
	Adapter       string
	HealthURL     string
}

type TriggerInput struct {
	TargetID       string
	Version        string
	CommitSHA      string
	CommitMessage  string
	Branch         string
	ImageRef       string
	IdempotencyKey string
	SimulateFail   bool
	TriggeredBy    string
	IPAddress      string
	IsDemo         bool
}

type Service struct {
	db       *database.DB
	queue    queue.Queue
	ssrf     *security.SSRFValidator
	audit    *audit.Recorder
	logs     *logs.Service
	broker   *sse.Broker
	onFailed func(ctx context.Context, dep Deployment)
}

func NewService(db *database.DB, q queue.Queue, ssrf *security.SSRFValidator, auditRec *audit.Recorder, logSvc *logs.Service, broker *sse.Broker) *Service {
	return &Service{db: db, queue: q, ssrf: ssrf, audit: auditRec, logs: logSvc, broker: broker}
}

func (s *Service) SetFailureHook(fn func(ctx context.Context, dep Deployment)) {
	s.onFailed = fn
}

func (s *Service) CreateTarget(ctx context.Context, orgID string, isDemo bool, in CreateTargetInput) (*Target, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Adapter = strings.ToLower(strings.TrimSpace(in.Adapter))
	if in.Name == "" {
		return nil, errors.New("target name is required")
	}
	if isDemo {
		in.Adapter = "simulation"
	}
	switch in.Adapter {
	case "local_http", "simulation", "docker_compose":
	default:
		return nil, errors.New("adapter must be local_http, simulation, or docker_compose")
	}
	if in.HealthURL != "" && !isDemo {
		if _, _, err := s.ssrf.ValidateHTTPURL(ctx, in.HealthURL, isDemo); err != nil {
			return nil, fmt.Errorf("invalid health_url: %w", err)
		}
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	var projArg, envArg, hostArg any
	if in.ProjectID != "" {
		projArg = in.ProjectID
	}
	if in.EnvironmentID != "" {
		envArg = in.EnvironmentID
	}
	if in.HostID != "" {
		hostArg = in.HostID
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO deployment_targets (id, organization_id, project_id, environment_id, host_id, name, adapter, health_url, is_synthetic, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, orgID, projArg, envArg, hostArg, in.Name, in.Adapter, in.HealthURL, database.BoolToDB(s.db.Driver, isDemo), now, now,
	)
	if err != nil {
		return nil, err
	}
	return s.GetTarget(ctx, orgID, id)
}

func (s *Service) ListTargets(ctx context.Context, orgID, envID string) ([]Target, error) {
	where := "WHERE t.organization_id = ?"
	args := []any{orgID}
	if envID != "" {
		where += " AND t.environment_id = ?"
		args = append(args, envID)
	}
	rows, err := s.db.Query(ctx, `
		SELECT t.id, t.organization_id, COALESCE(t.project_id, ''), COALESCE(t.environment_id, ''), COALESCE(e.name, ''),
		       COALESCE(t.host_id, ''), t.name, t.adapter, t.health_url, t.current_version,
		       COALESCE(t.current_artifact_id, ''), COALESCE(t.locked_by_deployment_id, ''),
		       CASE WHEN t.is_synthetic THEN 1 ELSE 0 END, t.created_at, t.updated_at
		FROM deployment_targets t
		LEFT JOIN environments e ON e.id = t.environment_id
		`+where+`
		ORDER BY t.created_at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Target, 0)
	for rows.Next() {
		var t Target
		var synthInt int
		if err := rows.Scan(
			&t.ID, &t.OrgID, &t.ProjectID, &t.EnvironmentID, &t.EnvironmentName, &t.HostID,
			&t.Name, &t.Adapter, &t.HealthURL, &t.CurrentVersion, &t.CurrentArtifactID, &t.LockedByDeploymentID,
			&synthInt, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		t.IsSynthetic = synthInt == 1
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Service) GetTarget(ctx context.Context, orgID, targetID string) (*Target, error) {
	list, err := s.ListTargets(ctx, orgID, "")
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == targetID {
			item := list[i]
			return &item, nil
		}
	}
	return nil, errors.New("deployment target not found")
}

func (s *Service) Trigger(ctx context.Context, orgID string, in TriggerInput) (*Deployment, error) {
	in.Version = strings.TrimSpace(in.Version)
	if in.TargetID == "" || in.Version == "" {
		return nil, errors.New("target_id and version are required")
	}

	// Honor idempotency key to prevent duplicate releases on retries.
	if in.IdempotencyKey != "" {
		var existingID string
		err := s.db.QueryRow(ctx, `SELECT id FROM deployments WHERE organization_id = ? AND idempotency_key = ?`, orgID, in.IdempotencyKey).Scan(&existingID)
		if err == nil {
			return s.GetDeployment(ctx, orgID, existingID)
		}
	}

	target, err := s.GetTarget(ctx, orgID, in.TargetID)
	if err != nil {
		return nil, err
	}
	if in.IsDemo && target.Adapter != "simulation" {
		return nil, errors.New("demonstration sessions may only execute clearly labeled simulation deployments")
	}

	isSim := in.IsDemo || target.Adapter == "simulation"
	triggerType := "manual"
	if in.IsDemo {
		triggerType = "demo"
	}
	if in.Branch == "" {
		in.Branch = "main"
	}
	if in.CommitSHA == "" {
		sum := sha256.Sum256([]byte(in.Version + time.Now().UTC().String()))
		in.CommitSHA = hex.EncodeToString(sum[:])[:12]
	}

	depID := uuid.NewString()
	artID := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)

	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		// Atomic per-target lock prevents overlapping deployments to the same target.
		res, err := tx.ExecContext(ctx, s.db.Rebind(`
			UPDATE deployment_targets SET locked_by_deployment_id = ?, locked_at = ?, updated_at = ?
			WHERE id = ? AND organization_id = ? AND locked_by_deployment_id IS NULL`),
			depID, now, now, target.ID, orgID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errors.New("deployment target is currently locked by another active deployment")
		}

		checksum := sha256.Sum256([]byte(target.ID + ":" + in.Version + ":" + in.CommitSHA))
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO deployment_artifacts (id, organization_id, target_id, version, commit_sha, commit_message, branch, image_ref, checksum, build_metadata_json, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '{}', ?)`),
			artID, orgID, target.ID, in.Version, in.CommitSHA, in.CommitMessage, in.Branch, in.ImageRef, hex.EncodeToString(checksum[:16]), now); err != nil {
			return err
		}

		var prevArtArg, envArg, idempArg, userArg any
		if target.CurrentArtifactID != "" {
			prevArtArg = target.CurrentArtifactID
		}
		if target.EnvironmentID != "" {
			envArg = target.EnvironmentID
		}
		if in.IdempotencyKey != "" {
			idempArg = in.IdempotencyKey
		}
		if in.TriggeredBy != "" {
			userArg = in.TriggeredBy
		}

		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO deployments (id, organization_id, target_id, artifact_id, previous_artifact_id, environment_id, idempotency_key, version, commit_sha, commit_message, branch, status, trigger_type, is_simulation, triggered_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'queued', ?, ?, ?, ?)`),
			depID, orgID, target.ID, artID, prevArtArg, envArg, idempArg, in.Version, in.CommitSHA, in.CommitMessage, in.Branch, triggerType, database.BoolToDB(s.db.Driver, isSim), userArg, now); err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO deployment_events (organization_id, deployment_id, state, level, message, details_json, created_at)
			VALUES (?, ?, 'queued', 'info', ?, '{}', ?)`),
			orgID, depID, fmt.Sprintf("Deployment %s queued for target %s (adapter: %s)", in.Version, target.Name, target.Adapter), now)
		return err
	})
	if err != nil {
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.RecordInput{
		OrgID: orgID, ActorUserID: in.TriggeredBy, Action: "deployment.triggered", ResourceType: "deployment", ResourceID: depID, IPAddress: in.IPAddress,
		Details: map[string]any{"target_id": target.ID, "version": in.Version, "adapter": target.Adapter, "is_simulation": isSim},
	})

	if s.queue != nil {
		_ = s.queue.Enqueue(queue.Job{
			Type: "deployment",
			Run: func(runCtx context.Context) error {
				return s.executeDeployment(runCtx, orgID, depID, target, artID, in.Version, false, in.SimulateFail)
			},
		})
	} else {
		_ = s.executeDeployment(ctx, orgID, depID, target, artID, in.Version, false, in.SimulateFail)
	}

	return s.GetDeployment(ctx, orgID, depID)
}

func (s *Service) Rollback(ctx context.Context, orgID, deploymentID, userID, ipAddr string, isDemo bool) (*Deployment, error) {
	orig, err := s.GetDeployment(ctx, orgID, deploymentID)
	if err != nil {
		return nil, err
	}
	target, err := s.GetTarget(ctx, orgID, orig.TargetID)
	if err != nil {
		return nil, err
	}
	if isDemo && target.Adapter != "simulation" {
		return nil, errors.New("demonstration sessions cannot roll back real infrastructure targets")
	}

	rollbackArtifactID := orig.PreviousArtifactID
	if rollbackArtifactID == "" {
		// Fall back to the most recent earlier artifact for this target.
		_ = s.db.QueryRow(ctx, `
			SELECT id FROM deployment_artifacts
			WHERE organization_id = ? AND target_id = ? AND id != ?
			ORDER BY created_at DESC LIMIT 1`, orgID, target.ID, orig.ArtifactID).Scan(&rollbackArtifactID)
	}
	if rollbackArtifactID == "" {
		return nil, errors.New("no verified previous release artifact is available to roll back to")
	}

	var prevVersion, prevSHA, prevBranch string
	if err := s.db.QueryRow(ctx, `SELECT version, commit_sha, branch FROM deployment_artifacts WHERE id = ? AND organization_id = ?`, rollbackArtifactID, orgID).
		Scan(&prevVersion, &prevSHA, &prevBranch); err != nil {
		return nil, errors.New("previous artifact record not found")
	}

	depID := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	isSim := isDemo || target.Adapter == "simulation"

	err = s.db.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, s.db.Rebind(`
			UPDATE deployment_targets SET locked_by_deployment_id = ?, locked_at = ?, updated_at = ?
			WHERE id = ? AND organization_id = ? AND locked_by_deployment_id IS NULL`),
			depID, now, now, target.ID, orgID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return errors.New("deployment target is currently locked by another active deployment")
		}
		var envArg, userArg any
		if target.EnvironmentID != "" {
			envArg = target.EnvironmentID
		}
		if userID != "" {
			userArg = userID
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO deployments (id, organization_id, target_id, artifact_id, previous_artifact_id, environment_id, version, commit_sha, commit_message, branch, status, trigger_type, is_simulation, triggered_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'rolling_back', 'rollback', ?, ?, ?)`),
			depID, orgID, target.ID, rollbackArtifactID, target.CurrentArtifactID, envArg, prevVersion, prevSHA,
			fmt.Sprintf("Rollback to %s from %s", prevVersion, orig.Version), prevBranch, database.BoolToDB(s.db.Driver, isSim), userArg, now); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO deployment_events (organization_id, deployment_id, state, level, message, details_json, created_at)
			VALUES (?, ?, 'rolling_back', 'warn', ?, '{}', ?)`),
			orgID, depID, fmt.Sprintf("Rollback initiated to verified artifact %s (%s)", prevVersion, prevSHA), now)
		return err
	})
	if err != nil {
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.RecordInput{
		OrgID: orgID, ActorUserID: userID, Action: "deployment.rollback", ResourceType: "deployment", ResourceID: depID, IPAddress: ipAddr,
		Details: map[string]any{"target_id": target.ID, "restored_version": prevVersion, "from_deployment_id": orig.ID},
	})

	if s.queue != nil {
		_ = s.queue.Enqueue(queue.Job{
			Type: "deployment_rollback",
			Run: func(runCtx context.Context) error {
				return s.executeDeployment(runCtx, orgID, depID, target, rollbackArtifactID, prevVersion, true, false)
			},
		})
	} else {
		_ = s.executeDeployment(ctx, orgID, depID, target, rollbackArtifactID, prevVersion, true, false)
	}

	return s.GetDeployment(ctx, orgID, depID)
}

func (s *Service) executeDeployment(ctx context.Context, orgID, depID string, target *Target, artifactID, version string, isRollback, simulateFail bool) error {
	runCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	defer func() {
		_, _ = s.db.Exec(context.Background(), `
			UPDATE deployment_targets SET locked_by_deployment_id = NULL, locked_at = NULL, updated_at = ?
			WHERE id = ? AND organization_id = ? AND locked_by_deployment_id = ?`,
			time.Now().UTC().Format(time.RFC3339), target.ID, orgID, depID)
	}()

	startNow := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.Exec(runCtx, `UPDATE deployments SET started_at = ? WHERE id = ? AND organization_id = ?`, startNow, depID, orgID)

	steps := []struct {
		state   string
		message string
	}{
		{"preparing", fmt.Sprintf("Preparing release %s for %s", version, target.Name)},
		{"building", fmt.Sprintf("Validating artifact checksum and build manifest for %s", version)},
		{"deploying", fmt.Sprintf("Applying release %s via %s adapter", version, target.Adapter)},
		{"verifying", fmt.Sprintf("Running post-deployment health verification for %s", version)},
	}
	if isRollback {
		steps[0].message = fmt.Sprintf("Preparing rollback to previous artifact %s", version)
		steps[1].message = fmt.Sprintf("Retrieving verified previous artifact %s", version)
		steps[2].message = fmt.Sprintf("Restoring release %s on %s", version, target.Name)
		steps[3].message = fmt.Sprintf("Verifying restored release %s health", version)
	}

	stepDelay := 120 * time.Millisecond
	for _, st := range steps {
		if runCtx.Err() != nil {
			s.finishDeployment(orgID, depID, target, "", "", "cancelled", "deployment timed out or cancelled")
			return runCtx.Err()
		}
		s.transition(runCtx, orgID, depID, st.state, "info", st.message)
		select {
		case <-runCtx.Done():
			s.finishDeployment(orgID, depID, target, "", "", "cancelled", "deployment cancelled")
			return runCtx.Err()
		case <-time.After(stepDelay):
		}

		if st.state == "verifying" {
			if simulateFail {
				s.finishDeployment(orgID, depID, target, "", "", "failed", "Simulated verification failure triggered for testing.")
				return errors.New("simulated verification failure")
			}
			if target.Adapter == "local_http" && target.HealthURL != "" {
				if err := s.verifyHTTPHealth(runCtx, target.HealthURL); err != nil {
					s.finishDeployment(orgID, depID, target, "", "", "failed", fmt.Sprintf("Health check failed for %s: %v", target.HealthURL, err))
					return err
				}
				s.transition(runCtx, orgID, depID, "verifying", "info", fmt.Sprintf("Verified HTTP 200 response from %s", target.HealthURL))
			} else if target.Adapter == "simulation" {
				s.transition(runCtx, orgID, depID, "verifying", "info", "Simulation verification completed (no external host modified).")
			}
		}
	}

	finalState := "succeeded"
	if isRollback {
		finalState = "rolled_back"
	}
	s.finishDeployment(orgID, depID, target, artifactID, version, finalState, "")
	return nil
}

func (s *Service) verifyHTTPHealth(ctx context.Context, rawURL string) error {
	if _, _, err := s.ssrf.ValidateHTTPURL(ctx, rawURL, false); err != nil {
		return err
	}
	client := s.ssrf.NewHTTPClient(5*time.Second, false)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 16*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected HTTP status %d", resp.StatusCode)
	}
	return nil
}

func (s *Service) transition(ctx context.Context, orgID, depID, state, level, msg string) {
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.Exec(ctx, `UPDATE deployments SET status = ? WHERE id = ? AND organization_id = ?`, state, depID, orgID)
	_, _ = s.db.Exec(ctx, `
		INSERT INTO deployment_events (organization_id, deployment_id, state, level, message, details_json, created_at)
		VALUES (?, ?, ?, ?, ?, '{}', ?)`, orgID, depID, state, level, msg, now)
	if s.logs != nil {
		_, _ = s.logs.Ingest(ctx, logs.IngestInput{
			OrgID: orgID, Severity: level, SourceName: "deployment-engine", Message: msg, CorrelationID: depID,
		})
	}
	if s.broker != nil {
		s.broker.Publish(orgID, "deployment.state", map[string]any{
			"deployment_id": depID, "state": state, "message": msg, "timestamp": now,
		})
	}
}

func (s *Service) finishDeployment(orgID, depID string, target *Target, artifactID, version, finalState, errMsg string) {
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.Exec(ctx, `
		UPDATE deployments SET status = ?, finished_at = ?, error_message = ?
		WHERE id = ? AND organization_id = ?`, finalState, now, errMsg, depID, orgID)

	level := "info"
	msg := fmt.Sprintf("Deployment completed with status %s", finalState)
	if finalState == "succeeded" || finalState == "rolled_back" {
		_, _ = s.db.Exec(ctx, `
			UPDATE deployment_targets SET current_version = ?, current_artifact_id = ?, updated_at = ?
			WHERE id = ? AND organization_id = ?`, version, artifactID, now, target.ID, orgID)
	} else {
		level = "error"
		msg = fmt.Sprintf("Deployment %s: %s", finalState, errMsg)
	}

	_, _ = s.db.Exec(ctx, `
		INSERT INTO deployment_events (organization_id, deployment_id, state, level, message, details_json, created_at)
		VALUES (?, ?, ?, ?, ?, '{}', ?)`, orgID, depID, finalState, level, msg, now)

	if s.logs != nil {
		_, _ = s.logs.Ingest(ctx, logs.IngestInput{
			OrgID: orgID, EnvironmentID: target.EnvironmentID, Severity: level, SourceName: "deployment-engine", Message: msg, CorrelationID: depID,
		})
	}
	if s.broker != nil {
		s.broker.Publish(orgID, "deployment.state", map[string]any{
			"deployment_id": depID, "state": finalState, "message": msg, "timestamp": now,
		})
	}
	if finalState == "failed" && s.onFailed != nil {
		if dep, err := s.GetDeployment(ctx, orgID, depID); err == nil {
			s.onFailed(ctx, *dep)
		}
	}
}

func (s *Service) ListDeployments(ctx context.Context, orgID, envID string, limit int) ([]Deployment, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	where := "WHERE d.organization_id = ?"
	args := []any{orgID}
	if envID != "" {
		where += " AND d.environment_id = ?"
		args = append(args, envID)
	}
	rows, err := s.db.Query(ctx, `
		SELECT d.id, d.organization_id, d.target_id, COALESCE(t.name, ''), COALESCE(t.adapter, ''),
		       COALESCE(d.artifact_id, ''), COALESCE(d.previous_artifact_id, ''),
		       COALESCE(d.environment_id, ''), COALESCE(e.name, ''), COALESCE(d.idempotency_key, ''),
		       d.version, d.commit_sha, d.commit_message, d.branch, d.status, d.trigger_type,
		       CASE WHEN d.is_simulation THEN 1 ELSE 0 END, COALESCE(d.triggered_by, ''), COALESCE(u.email, ''),
		       COALESCE(d.started_at, ''), COALESCE(d.finished_at, ''), d.error_message, d.created_at
		FROM deployments d
		LEFT JOIN deployment_targets t ON t.id = d.target_id
		LEFT JOIN environments e ON e.id = d.environment_id
		LEFT JOIN users u ON u.id = d.triggered_by
		`+where+`
		ORDER BY d.created_at DESC
		LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Deployment, 0)
	for rows.Next() {
		var d Deployment
		var simInt int
		if err := rows.Scan(
			&d.ID, &d.OrgID, &d.TargetID, &d.TargetName, &d.Adapter,
			&d.ArtifactID, &d.PreviousArtifactID, &d.EnvironmentID, &d.EnvironmentName, &d.IdempotencyKey,
			&d.Version, &d.CommitSHA, &d.CommitMessage, &d.Branch, &d.Status, &d.TriggerType,
			&simInt, &d.TriggeredBy, &d.TriggeredByEmail, &d.StartedAt, &d.FinishedAt, &d.ErrorMessage, &d.CreatedAt,
		); err != nil {
			return nil, err
		}
		d.IsSimulation = simInt == 1
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Service) GetDeployment(ctx context.Context, orgID, deploymentID string) (*Deployment, error) {
	list, err := s.ListDeployments(ctx, orgID, "", 100)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == deploymentID {
			dep := list[i]
			evRows, err := s.db.Query(ctx, `
				SELECT id, deployment_id, state, level, message, details_json, created_at
				FROM deployment_events WHERE organization_id = ? AND deployment_id = ?
				ORDER BY id ASC`, orgID, deploymentID)
			if err == nil {
				defer evRows.Close()
				for evRows.Next() {
					var ev Event
					var raw string
					if err := evRows.Scan(&ev.ID, &ev.DeploymentID, &ev.State, &ev.Level, &ev.Message, &raw, &ev.CreatedAt); err == nil {
						ev.Details = map[string]any{}
						_ = json.Unmarshal([]byte(raw), &ev.Details)
						dep.Events = append(dep.Events, ev)
					}
				}
			}
			return &dep, nil
		}
	}
	return nil, errors.New("deployment not found")
}
