package integrations

import (
	"context"
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
	"github.com/dandisetiawanco/InfraOS/internal/security"
	"github.com/google/uuid"
)

type Setting struct {
	ID             string         `json:"id"`
	Provider       string         `json:"provider"`
	Status         string         `json:"status"`
	Config         map[string]any `json:"config"`
	SecretMasked   string         `json:"secret_masked,omitempty"`
	LastVerifiedAt string         `json:"last_verified_at,omitempty"`
	UpdatedAt      string         `json:"updated_at"`
}

type GitHubCommit struct {
	SHA     string `json:"sha"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	URL     string `json:"url"`
}

type GitHubWorkflowRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Branch     string `json:"branch"`
	CommitSHA  string `json:"commit_sha"`
	CreatedAt  string `json:"created_at"`
	HTMLURL    string `json:"html_url"`
}

type GitHubOverview struct {
	Connected    bool                `json:"connected"`
	Repository   string              `json:"repository,omitempty"`
	Message      string              `json:"message"`
	Commits      []GitHubCommit      `json:"commits"`
	WorkflowRuns []GitHubWorkflowRun `json:"workflow_runs"`
}

type Service struct {
	db        *database.DB
	audit     *audit.Recorder
	logs      *logs.Service
	secretKey string
	client    *http.Client
}

func NewService(db *database.DB, auditRec *audit.Recorder, logSvc *logs.Service, secretKey string) *Service {
	return &Service{
		db:        db,
		audit:     auditRec,
		logs:      logSvc,
		secretKey: secretKey,
		client:    &http.Client{Timeout: 8 * time.Second},
	}
}

func (s *Service) List(ctx context.Context, orgID string) ([]Setting, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, provider, status, config_json, secret_masked, COALESCE(last_verified_at, ''), updated_at
		FROM integration_settings WHERE organization_id = ?
		ORDER BY provider ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byProvider := make(map[string]Setting)
	for rows.Next() {
		var st Setting
		var cfgRaw string
		if err := rows.Scan(&st.ID, &st.Provider, &st.Status, &cfgRaw, &st.SecretMasked, &st.LastVerifiedAt, &st.UpdatedAt); err != nil {
			return nil, err
		}
		st.Config = map[string]any{}
		_ = json.Unmarshal([]byte(cfgRaw), &st.Config)
		byProvider[st.Provider] = st
	}

	providers := []string{"github", "docker", "prometheus", "otel", "ai"}
	out := make([]Setting, 0, len(providers))
	for _, p := range providers {
		if existing, ok := byProvider[p]; ok {
			out = append(out, existing)
		} else {
			out = append(out, Setting{Provider: p, Status: "disconnected", Config: map[string]any{}})
		}
	}
	return out, nil
}

func (s *Service) Upsert(ctx context.Context, orgID, provider, userID string, cfg map[string]any, secret string, isDemo bool) (*Setting, error) {
	if isDemo {
		return nil, errors.New("external integrations cannot be configured in demonstration mode")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "github", "docker", "prometheus", "otel", "ai":
	default:
		return nil, errors.New("unsupported integration provider")
	}

	if cfg == nil {
		cfg = map[string]any{}
	}
	cfgJSON, _ := json.Marshal(cfg)
	now := time.Now().UTC().Format(time.RFC3339)

	var masked, encrypted string
	if strings.TrimSpace(secret) != "" {
		masked = security.MaskSecret(secret)
		enc, err := security.EncryptSecret(s.secretKey, strings.TrimSpace(secret))
		if err != nil {
			return nil, err
		}
		encrypted = enc
	} else {
		// Preserve existing stored secret if caller updated non-secret fields.
		_ = s.db.QueryRow(ctx, `SELECT secret_masked, secret_encrypted FROM integration_settings WHERE organization_id = ? AND provider = ?`, orgID, provider).Scan(&masked, &encrypted)
	}

	status := "connected"
	if provider == "github" && strings.TrimSpace(fmt.Sprint(cfg["repository"])) == "" {
		status = "disconnected"
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO integration_settings (id, organization_id, provider, status, config_json, secret_masked, secret_encrypted, last_verified_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(organization_id, provider) DO UPDATE SET
			status = excluded.status, config_json = excluded.config_json,
			secret_masked = excluded.secret_masked, secret_encrypted = excluded.secret_encrypted,
			updated_at = excluded.updated_at`,
		uuid.NewString(), orgID, provider, status, string(cfgJSON), masked, encrypted, now, now)
	if err != nil {
		return nil, err
	}

	_ = s.audit.Record(ctx, audit.RecordInput{
		OrgID: orgID, ActorUserID: userID, Action: "integration.updated", ResourceType: "integration", ResourceID: provider,
		Details: map[string]any{"provider": provider, "status": status},
	})
	return &Setting{Provider: provider, Status: status, Config: cfg, SecretMasked: masked, LastVerifiedAt: now, UpdatedAt: now}, nil
}

func (s *Service) FetchGitHubOverview(ctx context.Context, orgID string, isDemo bool) (*GitHubOverview, error) {
	if isDemo {
		return &GitHubOverview{
			Connected:    false,
			Message:      "GitHub integration is disconnected in demonstration mode. Connect a repository in an authenticated workspace to inspect commits and workflow runs.",
			Commits:      []GitHubCommit{},
			WorkflowRuns: []GitHubWorkflowRun{},
		}, nil
	}
	var status, cfgRaw, encSecret string
	err := s.db.QueryRow(ctx, `SELECT status, config_json, secret_encrypted FROM integration_settings WHERE organization_id = ? AND provider = 'github'`, orgID).
		Scan(&status, &cfgRaw, &encSecret)
	if err != nil || status == "disconnected" {
		return &GitHubOverview{
			Connected:    false,
			Message:      "GitHub repository not configured. Add an owner/repo and optional read-only token in Integrations.",
			Commits:      []GitHubCommit{},
			WorkflowRuns: []GitHubWorkflowRun{},
		}, nil
	}

	var cfg map[string]any
	_ = json.Unmarshal([]byte(cfgRaw), &cfg)
	repo := strings.TrimSpace(fmt.Sprint(cfg["repository"]))
	if repo == "" || !strings.Contains(repo, "/") {
		return &GitHubOverview{
			Connected:    false,
			Message:      "Repository must be configured in owner/name format.",
			Commits:      []GitHubCommit{},
			WorkflowRuns: []GitHubWorkflowRun{},
		}, nil
	}

	token, _ := security.DecryptSecret(s.secretKey, encSecret)
	commits, commitErr := s.fetchGitHubCommits(ctx, repo, token)
	runs, runErr := s.fetchGitHubWorkflowRuns(ctx, repo, token)
	if commitErr != nil && runErr != nil {
		return &GitHubOverview{
			Connected:    false,
			Repository:   repo,
			Message:      fmt.Sprintf("Unable to reach GitHub API for %s: %v", repo, commitErr),
			Commits:      []GitHubCommit{},
			WorkflowRuns: []GitHubWorkflowRun{},
		}, nil
	}

	return &GitHubOverview{
		Connected:    true,
		Repository:   repo,
		Message:      "Connected to GitHub API (read-only repository inspection).",
		Commits:      commits,
		WorkflowRuns: runs,
	}, nil
}

func (s *Service) fetchGitHubCommits(ctx context.Context, repo, token string) ([]GitHubCommit, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/commits?per_page=8", repo), nil)
	if err != nil {
		return []GitHubCommit{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "InfraOS/0.1")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return []GitHubCommit{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return []GitHubCommit{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var raw []struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
				Date string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256*1024)).Decode(&raw); err != nil {
		return []GitHubCommit{}, err
	}
	out := make([]GitHubCommit, 0, len(raw))
	for _, c := range raw {
		sha := c.SHA
		if len(sha) > 10 {
			sha = sha[:10]
		}
		out = append(out, GitHubCommit{
			SHA:     sha,
			Message: strings.Split(c.Commit.Message, "\n")[0],
			Author:  c.Commit.Author.Name,
			Date:    c.Commit.Author.Date,
			URL:     c.HTMLURL,
		})
	}
	return out, nil
}

func (s *Service) fetchGitHubWorkflowRuns(ctx context.Context, repo, token string) ([]GitHubWorkflowRun, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/actions/runs?per_page=8", repo), nil)
	if err != nil {
		return []GitHubWorkflowRun{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "InfraOS/0.1")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return []GitHubWorkflowRun{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return []GitHubWorkflowRun{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var raw struct {
		WorkflowRuns []struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HeadBranch string `json:"head_branch"`
			HeadSHA    string `json:"head_sha"`
			CreatedAt  string `json:"created_at"`
			HTMLURL    string `json:"html_url"`
		} `json:"workflow_runs"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 256*1024)).Decode(&raw); err != nil {
		return []GitHubWorkflowRun{}, err
	}
	out := make([]GitHubWorkflowRun, 0, len(raw.WorkflowRuns))
	for _, r := range raw.WorkflowRuns {
		sha := r.HeadSHA
		if len(sha) > 10 {
			sha = sha[:10]
		}
		out = append(out, GitHubWorkflowRun{
			ID:         r.ID,
			Name:       r.Name,
			Status:     r.Status,
			Conclusion: r.Conclusion,
			Branch:     r.HeadBranch,
			CommitSHA:  sha,
			CreatedAt:  r.CreatedAt,
			HTMLURL:    r.HTMLURL,
		})
	}
	return out, nil
}

// ProcessGitHubWebhook verifies HMAC-SHA256 signatures and deduplicates deliveries by X-GitHub-Delivery.
func (s *Service) ProcessGitHubWebhook(ctx context.Context, orgID, deliveryID, eventType, sigHeader string, body []byte) (bool, error) {
	if deliveryID == "" {
		return false, errors.New("missing X-GitHub-Delivery header")
	}
	var cfgRaw string
	err := s.db.QueryRow(ctx, `SELECT config_json FROM integration_settings WHERE organization_id = ? AND provider = 'github'`, orgID).Scan(&cfgRaw)
	if err != nil {
		return false, errors.New("github integration is not configured for this organization")
	}
	var cfg map[string]any
	_ = json.Unmarshal([]byte(cfgRaw), &cfg)
	webhookSecret := strings.TrimSpace(fmt.Sprint(cfg["webhook_secret"]))
	if webhookSecret == "" {
		return false, errors.New("github webhook_secret is not configured")
	}
	if !security.VerifyGitHubSignature(webhookSecret, sigHeader, body) {
		return false, errors.New("invalid GitHub webhook signature")
	}

	var existing string
	if err := s.db.QueryRow(ctx, `SELECT id FROM webhook_deliveries WHERE provider = 'github' AND delivery_id = ?`, deliveryID).Scan(&existing); err == nil {
		// Duplicate delivery safely ignored.
		return true, nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	summary := map[string]any{"event": eventType, "bytes": len(body)}
	sumJSON, _ := json.Marshal(summary)
	_, err = s.db.Exec(ctx, `
		INSERT INTO webhook_deliveries (id, organization_id, provider, delivery_id, event_type, status, payload_summary_json, received_at)
		VALUES (?, ?, 'github', ?, ?, 'processed', ?, ?)`,
		uuid.NewString(), orgID, deliveryID, eventType, string(sumJSON), now)
	if err != nil {
		return false, err
	}

	if s.logs != nil {
		_, _ = s.logs.Ingest(ctx, logs.IngestInput{
			OrgID: orgID, Severity: "info", SourceName: "github-webhook",
			Message: fmt.Sprintf("Verified GitHub %s webhook delivery %s", eventType, deliveryID), CorrelationID: deliveryID,
		})
	}
	return false, nil
}
