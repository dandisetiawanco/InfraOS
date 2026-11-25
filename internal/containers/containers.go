package containers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/audit"
	"github.com/dandisetiawanco/InfraOS/internal/database"
	"github.com/dandisetiawanco/InfraOS/internal/security"
	"github.com/google/uuid"
)

type Container struct {
	ID                 string   `json:"id"`
	OrgID              string   `json:"organization_id"`
	HostID             string   `json:"host_id,omitempty"`
	EnvironmentID      string   `json:"environment_id,omitempty"`
	ExternalID         string   `json:"external_id"`
	Name               string   `json:"name"`
	Image              string   `json:"image"`
	State              string   `json:"state"`
	HealthStatus       string   `json:"health_status"`
	CPUPercent         float64  `json:"cpu_percent"`
	MemUsageBytes      uint64   `json:"mem_usage_bytes"`
	MemLimitBytes      uint64   `json:"mem_limit_bytes"`
	Networks           []string `json:"networks"`
	RestartCount       int      `json:"restart_count"`
	ContainerCreatedAt string   `json:"container_created_at,omitempty"`
	AllowMutations     bool     `json:"allow_mutations"`
	IsSynthetic        bool     `json:"is_synthetic"`
	UpdatedAt          string   `json:"updated_at"`
}

type IntegrationStatus struct {
	Enabled         bool   `json:"enabled"`
	Available       bool   `json:"available"`
	Mode            string `json:"mode"` // connected | disabled | unavailable | synthetic_demo
	SocketPath      string `json:"socket_path,omitempty"`
	MutationsPolicy string `json:"mutations_policy"`
	Message         string `json:"message"`
}

type ListResponse struct {
	Integration IntegrationStatus `json:"integration"`
	Containers  []Container       `json:"containers"`
}

type Service struct {
	db                *database.DB
	audit             *audit.Recorder
	enabled           bool
	socketPath        string
	allowMutations    bool
	mutationAllowlist map[string]struct{}
	client            *http.Client
}

func NewService(db *database.DB, auditRec *audit.Recorder, enabled bool, socketPath string, allowMutations bool, allowlist []string) *Service {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, name := range allowlist {
		allowed[strings.TrimPrefix(strings.TrimSpace(name), "/")] = struct{}{}
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	return &Service{
		db:                db,
		audit:             auditRec,
		enabled:           enabled,
		socketPath:        socketPath,
		allowMutations:    allowMutations,
		mutationAllowlist: allowed,
		client:            &http.Client{Transport: transport, Timeout: 6 * time.Second},
	}
}

func (s *Service) Status(ctx context.Context, isDemo bool) IntegrationStatus {
	if isDemo {
		return IntegrationStatus{
			Enabled:         false,
			Available:       false,
			Mode:            "synthetic_demo",
			MutationsPolicy: "read_only_demo",
			Message:         "Demonstration mode displays synthetic container records and never accesses the host Docker socket.",
		}
	}
	policy := "read_only"
	if s.allowMutations {
		policy = "allowlisted_mutations"
	}
	if !s.enabled {
		return IntegrationStatus{
			Enabled:         false,
			Available:       false,
			Mode:            "disabled",
			MutationsPolicy: policy,
			Message:         "Docker integration is disabled by default. Set INFRAOS_DOCKER_ENABLED=true on an authorized host to inspect local containers.",
		}
	}
	if _, err := os.Stat(s.socketPath); err != nil {
		return IntegrationStatus{
			Enabled:         true,
			Available:       false,
			Mode:            "unavailable",
			SocketPath:      s.socketPath,
			MutationsPolicy: policy,
			Message:         "Docker integration is enabled, but the Docker daemon socket is not reachable on this host.",
		}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/_ping", nil)
	resp, err := s.client.Do(req)
	if err != nil {
		return IntegrationStatus{
			Enabled:         true,
			Available:       false,
			Mode:            "unavailable",
			SocketPath:      s.socketPath,
			MutationsPolicy: policy,
			Message:         "Docker socket exists but daemon did not respond to ping.",
		}
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return IntegrationStatus{
			Enabled:         true,
			Available:       false,
			Mode:            "unavailable",
			SocketPath:      s.socketPath,
			MutationsPolicy: policy,
			Message:         fmt.Sprintf("Docker daemon ping returned HTTP %d.", resp.StatusCode),
		}
	}
	return IntegrationStatus{
		Enabled:         true,
		Available:       true,
		Mode:            "connected",
		SocketPath:      s.socketPath,
		MutationsPolicy: policy,
		Message:         "Connected to local Docker Engine via restricted Unix socket integration.",
	}
}

func (s *Service) List(ctx context.Context, orgID string, isDemo bool) (*ListResponse, error) {
	st := s.Status(ctx, isDemo)
	if !isDemo && st.Available {
		_ = s.syncFromDocker(ctx, orgID)
	}
	list, err := s.listStored(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return &ListResponse{Integration: st, Containers: list}, nil
}

func (s *Service) Get(ctx context.Context, orgID, containerID string, isDemo bool) (*Container, IntegrationStatus, error) {
	st := s.Status(ctx, isDemo)
	list, err := s.listStored(ctx, orgID)
	if err != nil {
		return nil, st, err
	}
	for i := range list {
		if list[i].ID == containerID {
			item := list[i]
			return &item, st, nil
		}
	}
	return nil, st, errors.New("container not found")
}

func (s *Service) Logs(ctx context.Context, orgID, containerID string, isDemo bool) ([]string, error) {
	c, st, err := s.Get(ctx, orgID, containerID, isDemo)
	if err != nil {
		return nil, err
	}
	if isDemo || c.IsSynthetic {
		return []string{
			fmt.Sprintf("[SYNTHETIC DEMO] %s starting container runtime image=%s", time.Now().UTC().Add(-4*time.Minute).Format(time.RFC3339), c.Image),
			fmt.Sprintf("[SYNTHETIC DEMO] %s health probe passed on /healthz status=200", time.Now().UTC().Add(-2*time.Minute).Format(time.RFC3339)),
			fmt.Sprintf("[SYNTHETIC DEMO] %s serving workload requests state=%s", time.Now().UTC().Add(-20*time.Second).Format(time.RFC3339), c.State),
		}, nil
	}
	if !st.Available {
		return nil, errors.New("Docker Engine is not available on this host")
	}
	url := fmt.Sprintf("http://docker/containers/%s/logs?stdout=true&stderr=true&tail=80", c.ExternalID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return nil, err
	}
	cleaned := security.RedactCredentials(stripDockerLogHeaders(raw))
	lines := strings.Split(strings.TrimSpace(cleaned), "\n")
	return lines, nil
}

func (s *Service) PerformAction(ctx context.Context, orgID, containerID, action, userID, ipAddr string, confirmed, isDemo bool) error {
	if isDemo {
		return errors.New("container control actions are blocked in demonstration mode")
	}
	if !confirmed {
		return errors.New("explicit user confirmation is required for container lifecycle operations")
	}
	switch action {
	case "start", "stop", "restart":
	default:
		return errors.New("unsupported container action; allowed actions are start, stop, restart")
	}
	if !s.enabled || !s.allowMutations {
		return errors.New("container lifecycle mutations are disabled; enable INFRAOS_DOCKER_ALLOW_MUTATIONS and configure allowlist")
	}
	c, st, err := s.Get(ctx, orgID, containerID, isDemo)
	if err != nil {
		return err
	}
	if !st.Available {
		return errors.New("Docker Engine is unavailable")
	}
	if _, ok := s.mutationAllowlist[c.Name]; !ok && !c.AllowMutations {
		return fmt.Errorf("container %q is not in the authorized mutation allowlist", c.Name)
	}

	actionCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	endpoint := fmt.Sprintf("http://docker/containers/%s/%s", c.ExternalID, action)
	req, err := http.NewRequestWithContext(actionCtx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotModified {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("docker %s failed (HTTP %d): %s", action, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	_ = s.audit.Record(ctx, audit.RecordInput{
		OrgID: orgID, ActorUserID: userID, Action: "container." + action, ResourceType: "container", ResourceID: c.ID, IPAddress: ipAddr,
		Details: map[string]any{"name": c.Name, "external_id": c.ExternalID},
	})
	_ = s.syncFromDocker(ctx, orgID)
	return nil
}

type dockerListItem struct {
	ID              string   `json:"Id"`
	Names           []string `json:"Names"`
	Image           string   `json:"Image"`
	State           string   `json:"State"`
	Status          string   `json:"Status"`
	Created         int64    `json:"Created"`
	NetworkSettings struct {
		Networks map[string]any `json:"Networks"`
	} `json:"NetworkSettings"`
}

func (s *Service) syncFromDocker(ctx context.Context, orgID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/json?all=true", nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("docker list returned HTTP %d", resp.StatusCode)
	}
	var items []dockerListItem
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&items); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, it := range items {
		name := it.ID[:12]
		if len(it.Names) > 0 {
			name = strings.TrimPrefix(it.Names[0], "/")
		}
		nets := make([]string, 0, len(it.NetworkSettings.Networks))
		for n := range it.NetworkSettings.Networks {
			nets = append(nets, n)
		}
		netsJSON, _ := json.Marshal(nets)
		health := "none"
		lowerStatus := strings.ToLower(it.Status)
		switch {
		case strings.Contains(lowerStatus, "(healthy)"):
			health = "healthy"
		case strings.Contains(lowerStatus, "(unhealthy)"):
			health = "unhealthy"
		case strings.Contains(lowerStatus, "(starting)"):
			health = "starting"
		}
		_, allowMut := s.mutationAllowlist[name]
		createdAt := time.Unix(it.Created, 0).UTC().Format(time.RFC3339)

		_, _ = s.db.Exec(ctx, `
			INSERT INTO containers (id, organization_id, external_id, name, image, state, health_status, networks_json, container_created_at, allow_mutations, is_synthetic, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(organization_id, external_id) DO UPDATE SET
				name = excluded.name, image = excluded.image, state = excluded.state,
				health_status = excluded.health_status, networks_json = excluded.networks_json,
				allow_mutations = excluded.allow_mutations, updated_at = excluded.updated_at`,
			uuid.NewString(), orgID, it.ID, name, it.Image, it.State, health, string(netsJSON), createdAt,
			database.BoolToDB(s.db.Driver, allowMut), database.BoolToDB(s.db.Driver, false), now,
		)
	}
	return nil
}

func (s *Service) listStored(ctx context.Context, orgID string) ([]Container, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, organization_id, COALESCE(host_id, ''), COALESCE(environment_id, ''),
		       external_id, name, image, state, health_status, cpu_percent, mem_usage_bytes, mem_limit_bytes,
		       networks_json, restart_count, COALESCE(container_created_at, ''),
		       CASE WHEN allow_mutations THEN 1 ELSE 0 END, CASE WHEN is_synthetic THEN 1 ELSE 0 END, updated_at
		FROM containers
		WHERE organization_id = ?
		ORDER BY name ASC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Container, 0)
	for rows.Next() {
		var c Container
		var netsRaw string
		var allowInt, synthInt int
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.HostID, &c.EnvironmentID, &c.ExternalID, &c.Name, &c.Image,
			&c.State, &c.HealthStatus, &c.CPUPercent, &c.MemUsageBytes, &c.MemLimitBytes,
			&netsRaw, &c.RestartCount, &c.ContainerCreatedAt, &allowInt, &synthInt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		c.AllowMutations = allowInt == 1
		c.IsSynthetic = synthInt == 1
		c.Networks = []string{}
		_ = json.Unmarshal([]byte(netsRaw), &c.Networks)
		out = append(out, c)
	}
	return out, rows.Err()
}

func stripDockerLogHeaders(raw []byte) string {
	var b strings.Builder
	for i := 0; i < len(raw); {
		if i+8 <= len(raw) && (raw[i] == 1 || raw[i] == 2) && raw[i+1] == 0 && raw[i+2] == 0 && raw[i+3] == 0 {
			size := int(raw[i+4])<<24 | int(raw[i+5])<<16 | int(raw[i+6])<<8 | int(raw[i+7])
			i += 8
			if size > 0 && i+size <= len(raw) {
				b.Write(raw[i : i+size])
				i += size
				continue
			}
		}
		b.WriteByte(raw[i])
		i++
	}
	return b.String()
}
