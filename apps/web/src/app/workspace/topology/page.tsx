"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { TopologyGraph } from "@infraos/contracts";
import { apiFetch } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";

const columnOrder: Record<string, number> = {
  project: 0,
  environment: 1,
  host: 2,
  agent: 3,
  service: 3,
  container: 4,
  target: 4,
};

export default function TopologyPage() {
  const { session, selectedEnvId } = useWorkspace();
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [dragging, setDragging] = useState(false);
  const [dragOrigin, setDragOrigin] = useState({ x: 0, y: 0 });
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<"graph" | "table">("graph");

  const topoQuery = useQuery({
    queryKey: ["topology", session?.organization.id, selectedEnvId],
    queryFn: () =>
      apiFetch<TopologyGraph>(
        `/api/v1/topology${selectedEnvId ? `?environment_id=${encodeURIComponent(selectedEnvId)}` : ""}`,
      ),
    enabled: Boolean(session),
  });

  const layout = useMemo(() => {
    const graph = topoQuery.data;
    if (!graph) return { positions: new Map<string, { x: number; y: number }>(), width: 920, height: 520 };
    const buckets = new Map<number, typeof graph.nodes>();
    for (const n of graph.nodes) {
      const col = columnOrder[n.type] ?? 2;
      const list = buckets.get(col) || [];
      list.push(n);
      buckets.set(col, list);
    }
    const positions = new Map<string, { x: number; y: number }>();
    let maxRows = 1;
    for (const [col, list] of buckets.entries()) {
      maxRows = Math.max(maxRows, list.length);
      list.forEach((node, idx) => {
        positions.set(node.id, { x: 40 + col * 215, y: 40 + idx * 92 });
      });
    }
    return { positions, width: 1120, height: Math.max(460, maxRows * 96 + 80) };
  }, [topoQuery.data]);

  if (topoQuery.isLoading) {
    return <StateView kind="loading" title="Building infrastructure topology" description="Mapping configured projects, environments, hosts, services, containers, and targets." />;
  }
  if (topoQuery.isError || !topoQuery.data) {
    return <StateView kind="error" title="Topology unavailable" description={(topoQuery.error as Error)?.message || "API request failed."} />;
  }

  const graph = topoQuery.data;
  const selectedNode = graph.nodes.find((n) => n.id === selectedNodeId) || null;

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Infrastructure topology</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            Actual configured relationships across projects, environments, hosts, agents, services, containers, and deployment targets.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            onClick={() => setViewMode("graph")}
            className={viewMode === "graph" ? "infra-btn-primary min-h-[34px] px-3 text-xs" : "infra-btn-secondary min-h-[34px] px-3 text-xs"}
          >
            Interactive graph
          </button>
          <button
            type="button"
            onClick={() => setViewMode("table")}
            className={viewMode === "table" ? "infra-btn-primary min-h-[34px] px-3 text-xs" : "infra-btn-secondary min-h-[34px] px-3 text-xs"}
          >
            Accessible table view
          </button>
        </div>
      </header>

      {graph.nodes.length === 0 ? (
        <StateView kind="empty" title="No infrastructure nodes configured" description="Enroll a host or create services and environments to populate the topology map." />
      ) : viewMode === "graph" ? (
        <div className="grid gap-4 xl:grid-cols-[1fr_290px]">
          <div className="infra-surface overflow-hidden">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--border)] px-4 py-2.5 text-xs">
              <span className="text-[var(--text-secondary)]">
                Drag canvas to pan. Dashed lines denote manually configured relationships.
              </span>
              <div className="flex items-center gap-1.5">
                <button type="button" onClick={() => setZoom((z) => Math.max(0.5, z - 0.15))} className="infra-btn-secondary min-h-[30px] px-2.5 text-xs">
                  Zoom out
                </button>
                <button type="button" onClick={() => { setZoom(1); setPan({ x: 0, y: 0 }); }} className="infra-btn-secondary min-h-[30px] px-2.5 text-xs">
                  Reset
                </button>
                <button type="button" onClick={() => setZoom((z) => Math.min(1.8, z + 0.15))} className="infra-btn-secondary min-h-[30px] px-2.5 text-xs">
                  Zoom in
                </button>
              </div>
            </div>

            <div
              className="h-[520px] w-full cursor-grab overflow-hidden bg-[var(--canvas)] active:cursor-grabbing"
              onMouseDown={(e) => {
                setDragging(true);
                setDragOrigin({ x: e.clientX - pan.x, y: e.clientY - pan.y });
              }}
              onMouseMove={(e) => {
                if (!dragging) return;
                setPan({ x: e.clientX - dragOrigin.x, y: e.clientY - dragOrigin.y });
              }}
              onMouseUp={() => setDragging(false)}
              onMouseLeave={() => setDragging(false)}
            >
              <svg
                role="img"
                aria-label="Configured infrastructure topology graph"
                width="100%"
                height="100%"
                viewBox={`0 0 ${layout.width} ${layout.height}`}
              >
                <g transform={`translate(${pan.x}, ${pan.y}) scale(${zoom})`}>
                  {graph.edges.map((edge) => {
                    const src = layout.positions.get(edge.source);
                    const dst = layout.positions.get(edge.target);
                    if (!src || !dst) return null;
                    return (
                      <line
                        key={edge.id}
                        x1={src.x + 170}
                        y1={src.y + 32}
                        x2={dst.x}
                        y2={dst.y + 32}
                        stroke={edge.is_manual ? "var(--accent)" : "var(--border-strong)"}
                        strokeWidth={edge.is_manual ? 2 : 1.5}
                        strokeDasharray={edge.is_manual ? "5 4" : undefined}
                      />
                    );
                  })}

                  {graph.nodes.map((node) => {
                    const pos = layout.positions.get(node.id);
                    if (!pos) return null;
                    const selected = selectedNodeId === node.id;
                    return (
                      <g
                        key={node.id}
                        transform={`translate(${pos.x}, ${pos.y})`}
                        onClick={() => setSelectedNodeId(node.id)}
                        className="cursor-pointer"
                      >
                        <rect
                          width={170}
                          height={64}
                          rx={10}
                          fill="var(--surface)"
                          stroke={selected ? "var(--accent)" : "var(--border-strong)"}
                          strokeWidth={selected ? 2.5 : 1.2}
                        />
                        <text x={12} y={22} fill="var(--text-secondary)" fontSize={10} fontFamily="monospace">
                          {node.type.toUpperCase()}
                        </text>
                        <text x={12} y={40} fill="var(--text-primary)" fontSize={12} fontWeight={600}>
                          {node.label.length > 20 ? node.label.slice(0, 19) + "…" : node.label}
                        </text>
                        <text x={12} y={54} fill="var(--text-secondary)" fontSize={10}>
                          {node.status}
                        </text>
                      </g>
                    );
                  })}
                </g>
              </svg>
            </div>
          </div>

          <aside className="infra-surface space-y-3 p-5">
            <h2 className="text-sm font-semibold">Node inspector</h2>
            {selectedNode ? (
              <div className="space-y-3 text-sm">
                <div className="font-semibold">{selectedNode.label}</div>
                <div className="flex flex-wrap gap-2">
                  <StatusBadge status={selectedNode.status} />
                  <StatusBadge status="info" label={selectedNode.type} />
                </div>
                {selectedNode.subtitle ? (
                  <p className="font-mono text-xs text-[var(--text-secondary)]">{selectedNode.subtitle}</p>
                ) : null}
                <div className="border-t border-[var(--border)] pt-3">
                  <div className="mb-1.5 text-xs font-semibold text-[var(--text-secondary)]">Connected edges</div>
                  <ul className="space-y-1 font-mono text-xs">
                    {graph.edges
                      .filter((e) => e.source === selectedNode.id || e.target === selectedNode.id)
                      .map((e) => (
                        <li key={e.id}>
                          {e.label} {e.is_manual ? "(manual)" : ""}
                        </li>
                      ))}
                  </ul>
                </div>
              </div>
            ) : (
              <p className="text-xs text-[var(--text-secondary)]">
                Select any node in the graph to inspect its type, live status, and configured relationships.
              </p>
            )}
          </aside>
        </div>
      ) : (
        <div className="infra-surface infra-table-wrap">
          <table className="infra-table infra-table-stack">
            <thead>
              <tr>
                <th scope="col">Node</th>
                <th scope="col">Type</th>
                <th scope="col">Status</th>
                <th scope="col">Relationships</th>
              </tr>
            </thead>
            <tbody>
              {graph.nodes.map((node) => {
                const rels = graph.edges.filter((e) => e.source === node.id || e.target === node.id);
                return (
                  <tr key={node.id}>
                    <th scope="row" className="font-semibold">{node.label}</th>
                    <td data-label="Type" className="font-mono text-xs">{node.type}</td>
                    <td data-label="Status"><StatusBadge status={node.status} /></td>
                    <td data-label="Relationships" className="font-mono text-xs text-[var(--text-secondary)]">
                      {rels.map((r) => `${r.label}${r.is_manual ? " (manual)" : ""}`).join(", ") || "None"}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
