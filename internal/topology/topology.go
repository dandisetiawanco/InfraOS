package topology

import (
	"context"
	"errors"
	"time"

	"github.com/dandisetiawanco/InfraOS/internal/database"
	"github.com/google/uuid"
)

type Node struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"` // project | environment | host | agent | service | container | target
	Label         string         `json:"label"`
	Subtitle      string         `json:"subtitle,omitempty"`
	Status        string         `json:"status"`
	EnvironmentID string         `json:"environment_id,omitempty"`
	IsSynthetic   bool           `json:"is_synthetic"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type Edge struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	Label    string `json:"label"`
	IsManual bool   `json:"is_manual"`
}

type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type Service struct {
	db *database.DB
}

func NewService(db *database.DB) *Service {
	return &Service{db: db}
}

func (s *Service) BuildGraph(ctx context.Context, orgID, envFilter string) (*Graph, error) {
	g := &Graph{Nodes: make([]Node, 0), Edges: make([]Edge, 0)}

	// Projects
	pRows, err := s.db.Query(ctx, `SELECT id, name, description FROM projects WHERE organization_id = ?`, orgID)
	if err == nil {
		defer pRows.Close()
		for pRows.Next() {
			var id, name, desc string
			if pRows.Scan(&id, &name, &desc) == nil {
				g.Nodes = append(g.Nodes, Node{ID: "project:" + id, Type: "project", Label: name, Subtitle: desc, Status: "active"})
			}
		}
	}

	// Environments
	envQuery := `SELECT id, project_id, name, tier FROM environments WHERE organization_id = ?`
	envArgs := []any{orgID}
	if envFilter != "" {
		envQuery += ` AND id = ?`
		envArgs = append(envArgs, envFilter)
	}
	eRows, err := s.db.Query(ctx, envQuery, envArgs...)
	if err == nil {
		defer eRows.Close()
		for eRows.Next() {
			var id, projID, name, tier string
			if eRows.Scan(&id, &projID, &name, &tier) == nil {
				g.Nodes = append(g.Nodes, Node{ID: "env:" + id, Type: "environment", Label: name, Subtitle: tier, Status: "active", EnvironmentID: id})
				g.Edges = append(g.Edges, Edge{ID: "e-pe-" + id, Source: "project:" + projID, Target: "env:" + id, Label: "contains"})
			}
		}
	}

	// Hosts & Agents
	hostQuery := `SELECT h.id, COALESCE(h.environment_id, ''), h.display_name, h.os, h.arch, h.status, CASE WHEN h.is_synthetic THEN 1 ELSE 0 END, COALESCE(a.id, ''), COALESCE(a.version, ''), COALESCE(a.status, '') FROM hosts h LEFT JOIN agents a ON a.host_id = h.id WHERE h.organization_id = ?`
	hostArgs := []any{orgID}
	if envFilter != "" {
		hostQuery += ` AND h.environment_id = ?`
		hostArgs = append(hostArgs, envFilter)
	}
	hRows, err := s.db.Query(ctx, hostQuery, hostArgs...)
	if err == nil {
		defer hRows.Close()
		for hRows.Next() {
			var id, envID, name, osName, arch, status, agentID, agentVer, agentStatus string
			var synth int
			if hRows.Scan(&id, &envID, &name, &osName, &arch, &status, &synth, &agentID, &agentVer, &agentStatus) == nil {
				g.Nodes = append(g.Nodes, Node{ID: "host:" + id, Type: "host", Label: name, Subtitle: osName + "/" + arch, Status: status, EnvironmentID: envID, IsSynthetic: synth == 1})
				if envID != "" {
					g.Edges = append(g.Edges, Edge{ID: "e-eh-" + id, Source: "env:" + envID, Target: "host:" + id, Label: "runs_on"})
				}
				if agentID != "" {
					g.Nodes = append(g.Nodes, Node{ID: "agent:" + agentID, Type: "agent", Label: "Agent " + agentVer, Subtitle: name, Status: agentStatus, EnvironmentID: envID, IsSynthetic: synth == 1})
					g.Edges = append(g.Edges, Edge{ID: "e-ha-" + agentID, Source: "host:" + id, Target: "agent:" + agentID, Label: "collected_by"})
				}
			}
		}
	}

	// Services
	svcQuery := `SELECT id, COALESCE(environment_id, ''), COALESCE(host_id, ''), name, check_type, status, CASE WHEN is_synthetic THEN 1 ELSE 0 END FROM services WHERE organization_id = ?`
	svcArgs := []any{orgID}
	if envFilter != "" {
		svcQuery += ` AND environment_id = ?`
		svcArgs = append(svcArgs, envFilter)
	}
	sRows, err := s.db.Query(ctx, svcQuery, svcArgs...)
	if err == nil {
		defer sRows.Close()
		for sRows.Next() {
			var id, envID, hostID, name, checkType, status string
			var synth int
			if sRows.Scan(&id, &envID, &hostID, &name, &checkType, &status, &synth) == nil {
				g.Nodes = append(g.Nodes, Node{ID: "service:" + id, Type: "service", Label: name, Subtitle: checkType, Status: status, EnvironmentID: envID, IsSynthetic: synth == 1})
				if hostID != "" {
					g.Edges = append(g.Edges, Edge{ID: "e-hs-" + id, Source: "host:" + hostID, Target: "service:" + id, Label: "hosts"})
				} else if envID != "" {
					g.Edges = append(g.Edges, Edge{ID: "e-es-" + id, Source: "env:" + envID, Target: "service:" + id, Label: "monitors"})
				}
			}
		}
	}

	// Containers
	cQuery := `SELECT id, COALESCE(environment_id, ''), COALESCE(host_id, ''), name, image, state, CASE WHEN is_synthetic THEN 1 ELSE 0 END FROM containers WHERE organization_id = ?`
	cArgs := []any{orgID}
	if envFilter != "" {
		cQuery += ` AND environment_id = ?`
		cArgs = append(cArgs, envFilter)
	}
	cRows, err := s.db.Query(ctx, cQuery, cArgs...)
	if err == nil {
		defer cRows.Close()
		for cRows.Next() {
			var id, envID, hostID, name, image, state string
			var synth int
			if cRows.Scan(&id, &envID, &hostID, &name, &image, &state, &synth) == nil {
				g.Nodes = append(g.Nodes, Node{ID: "container:" + id, Type: "container", Label: name, Subtitle: image, Status: state, EnvironmentID: envID, IsSynthetic: synth == 1})
				if hostID != "" {
					g.Edges = append(g.Edges, Edge{ID: "e-hc-" + id, Source: "host:" + hostID, Target: "container:" + id, Label: "runs"})
				}
			}
		}
	}

	// Deployment Targets
	tQuery := `SELECT id, COALESCE(environment_id, ''), COALESCE(host_id, ''), name, adapter, current_version, CASE WHEN is_synthetic THEN 1 ELSE 0 END FROM deployment_targets WHERE organization_id = ?`
	tArgs := []any{orgID}
	if envFilter != "" {
		tQuery += ` AND environment_id = ?`
		tArgs = append(tArgs, envFilter)
	}
	tRows, err := s.db.Query(ctx, tQuery, tArgs...)
	if err == nil {
		defer tRows.Close()
		for tRows.Next() {
			var id, envID, hostID, name, adapter, version string
			var synth int
			if tRows.Scan(&id, &envID, &hostID, &name, &adapter, &version, &synth) == nil {
				sub := adapter
				if version != "" {
					sub += " · " + version
				}
				g.Nodes = append(g.Nodes, Node{ID: "target:" + id, Type: "target", Label: name, Subtitle: sub, Status: "active", EnvironmentID: envID, IsSynthetic: synth == 1})
				if hostID != "" {
					g.Edges = append(g.Edges, Edge{ID: "e-th-" + id, Source: "target:" + id, Target: "host:" + hostID, Label: "deploys_to"})
				} else if envID != "" {
					g.Edges = append(g.Edges, Edge{ID: "e-te-" + id, Source: "env:" + envID, Target: "target:" + id, Label: "target"})
				}
			}
		}
	}

	// Explicitly configured manual links
	lRows, err := s.db.Query(ctx, `SELECT id, source_type, source_id, target_type, target_id, label, CASE WHEN is_manual THEN 1 ELSE 0 END FROM topology_links WHERE organization_id = ?`, orgID)
	if err == nil {
		defer lRows.Close()
		for lRows.Next() {
			var id, st, sid, tt, tid, label string
			var manual int
			if lRows.Scan(&id, &st, &sid, &tt, &tid, &label, &manual) == nil {
				g.Edges = append(g.Edges, Edge{
					ID: "manual-" + id, Source: st + ":" + sid, Target: tt + ":" + tid, Label: label, IsManual: manual == 1,
				})
			}
		}
	}

	return g, nil
}

func (s *Service) AddManualLink(ctx context.Context, orgID, envID, sourceType, sourceID, targetType, targetID, label string) error {
	if sourceType == "" || sourceID == "" || targetType == "" || targetID == "" {
		return errors.New("source and target identifiers are required")
	}
	if label == "" {
		label = "depends_on"
	}
	var envArg any
	if envID != "" {
		envArg = envID
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO topology_links (id, organization_id, environment_id, source_type, source_id, target_type, target_id, label, is_manual, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), orgID, envArg, sourceType, sourceID, targetType, targetID, label,
		database.BoolToDB(s.db.Driver, true), time.Now().UTC().Format(time.RFC3339))
	return err
}
