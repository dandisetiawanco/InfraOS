export type Role = "owner" | "admin" | "operator" | "viewer";

export interface User {
  id: string;
  email: string;
  name: string;
  created_at: string;
}

export interface Organization {
  id: string;
  name: string;
  slug: string;
  is_demo: boolean;
  role: Role;
  created_at: string;
}

export interface SessionInfo {
  user: User;
  organization: Organization;
  organizations: Organization[];
  csrf_token: string;
  profile?: string;
  db_driver?: string;
  ai_enabled?: boolean;
}

export interface MetricPoint {
  recorded_at: string;
  cpu_percent: number;
  cpu_cores: number;
  load_1m: number;
  load_5m: number;
  load_15m: number;
  mem_total_bytes: number;
  mem_used_bytes: number;
  mem_available_bytes: number;
  mem_percent: number;
  disk_total_bytes: number;
  disk_used_bytes: number;
  disk_percent: number;
  net_rx_bytes: number;
  net_tx_bytes: number;
  net_rx_bps: number;
  net_tx_bps: number;
  uptime_seconds: number;
  is_synthetic: boolean;
}

export interface FilesystemMetric {
  mount_point: string;
  device: string;
  fs_type: string;
  total_bytes: number;
  used_bytes: number;
  free_bytes: number;
  used_percent: number;
}

export interface NetworkInterfaceMetric {
  name: string;
  addresses?: string[];
  rx_bytes: number;
  tx_bytes: number;
  rx_bps: number;
  tx_bps: number;
}

export interface OptionalMetrics {
  cpu_temperature_c?: number;
  battery?: {
    percent: number;
    state: string;
    source?: string;
  };
  processes?: {
    total_count: number;
  };
}

export interface Host {
  id: string;
  organization_id: string;
  environment_id?: string;
  environment_name?: string;
  hostname: string;
  display_name: string;
  os: string;
  arch: string;
  kernel_version: string;
  ip_address: string;
  tags: string[];
  status: "online" | "offline" | "stale" | "revoked" | "pending";
  freshness: "live" | "stale" | "unavailable" | "synthetic";
  is_synthetic: boolean;
  metadata: Record<string, unknown>;
  last_heartbeat_at?: string;
  agent_id?: string;
  agent_version?: string;
  agent_status?: string;
  latest_metric?: MetricPoint;
  filesystems?: FilesystemMetric[];
  interfaces?: NetworkInterfaceMetric[];
  optional?: OptionalMetrics;
  supported_ranges?: string[];
  created_at: string;
  updated_at: string;
}

export interface ServiceRecord {
  id: string;
  organization_id: string;
  project_id?: string;
  environment_id?: string;
  environment_name?: string;
  host_id?: string;
  name: string;
  check_type: "http" | "https" | "tcp" | "dns" | "tls";
  target: string;
  interval_seconds: number;
  timeout_ms: number;
  expected_status: number;
  failure_threshold: number;
  recovery_threshold: number;
  enabled: boolean;
  status: "healthy" | "degraded" | "down" | "unknown";
  consecutive_failures: number;
  consecutive_successes: number;
  last_checked_at?: string;
  last_latency_ms: number;
  last_error?: string;
  tls_expires_at?: string;
  uptime_percent: number | null;
  observations_count: number;
  is_synthetic: boolean;
  created_at: string;
  updated_at: string;
}

export interface ServiceCheck {
  id: number;
  service_id: string;
  checked_at: string;
  status: "up" | "down" | "degraded";
  status_code: number;
  latency_ms: number;
  failure_reason?: string;
  tls_days_remaining?: number | null;
  resolved_ips?: string[];
  is_synthetic: boolean;
}

export interface Container {
  id: string;
  organization_id: string;
  host_id?: string;
  environment_id?: string;
  external_id: string;
  name: string;
  image: string;
  state: string;
  health_status: string;
  cpu_percent: number;
  mem_usage_bytes: number;
  mem_limit_bytes: number;
  networks: string[];
  restart_count: number;
  container_created_at?: string;
  allow_mutations: boolean;
  is_synthetic: boolean;
  updated_at: string;
}

export interface ContainerIntegrationStatus {
  enabled: boolean;
  available: boolean;
  mode: "connected" | "disabled" | "unavailable" | "synthetic_demo";
  socket_path?: string;
  mutations_policy: string;
  message: string;
}

export interface DeploymentTarget {
  id: string;
  name: string;
  environment_id?: string;
  environment_name?: string;
  host_id?: string;
  adapter: "local_http" | "simulation" | "docker_compose";
  health_url?: string;
  current_version?: string;
  current_artifact_id?: string;
  locked_by_deployment_id?: string;
  is_synthetic: boolean;
}

export interface DeploymentEvent {
  id: number;
  deployment_id: string;
  state: string;
  level: string;
  message: string;
  created_at: string;
}

export interface Deployment {
  id: string;
  target_id: string;
  target_name?: string;
  adapter?: string;
  environment_id?: string;
  environment_name?: string;
  version: string;
  commit_sha: string;
  commit_message: string;
  branch: string;
  status: string;
  trigger_type: string;
  is_simulation: boolean;
  triggered_by_email?: string;
  started_at?: string;
  finished_at?: string;
  error_message?: string;
  created_at: string;
  events?: DeploymentEvent[];
}

export interface LogEntry {
  id: number;
  host_id?: string;
  host_name?: string;
  service_id?: string;
  service_name?: string;
  environment_id?: string;
  timestamp: string;
  severity: "debug" | "info" | "warn" | "error" | "fatal";
  source_name: string;
  message: string;
  correlation_id?: string;
  fields: Record<string, unknown>;
  is_synthetic: boolean;
}

export interface AlertRule {
  id: string;
  name: string;
  rule_type: string;
  target_ref: string;
  threshold: number;
  duration_seconds: number;
  cooldown_seconds: number;
  severity: "info" | "warning" | "critical";
  auto_incident: boolean;
  webhook_url?: string;
  enabled: boolean;
  state: "ok" | "pending" | "firing";
  last_fired_at?: string;
  last_resolved_at?: string;
}

export interface AlertEvent {
  id: string;
  rule_id: string;
  rule_name?: string;
  state: "pending" | "firing" | "resolved";
  severity: string;
  summary: string;
  metric_value: number;
  threshold: number;
  incident_id?: string;
  triggered_at: string;
  resolved_at?: string;
  is_synthetic: boolean;
}

export interface IncidentTimelineEntry {
  id: number;
  event_type: string;
  author_email?: string;
  message: string;
  created_at: string;
}

export interface Incident {
  id: string;
  environment_id?: string;
  environment_name?: string;
  title: string;
  description: string;
  severity: "sev1" | "sev2" | "sev3" | "sev4";
  status: "open" | "investigating" | "identified" | "monitoring" | "resolved";
  affected_service_ids: string[];
  assigned_user_id?: string;
  assigned_email?: string;
  root_cause_notes: string;
  resolution_summary: string;
  postmortem_markdown: string;
  is_synthetic: boolean;
  opened_at: string;
  resolved_at?: string;
  timeline?: IncidentTimelineEntry[];
}

export interface OperationalInsight {
  id: string;
  category: string;
  severity: "info" | "warning" | "critical";
  title: string;
  detail: string;
  correlation_only: boolean;
  resource_type: string;
  resource_id: string;
  evidence: string[];
  detected_at: string;
}

export interface TopologyGraph {
  nodes: Array<{
    id: string;
    type: string;
    label: string;
    subtitle?: string;
    status: string;
    environment_id?: string;
    is_synthetic: boolean;
  }>;
  edges: Array<{
    id: string;
    source: string;
    target: string;
    label: string;
    is_manual: boolean;
  }>;
}

export interface OverviewData {
  is_demo: boolean;
  data_source: "live" | "stale" | "unavailable" | "synthetic";
  latest_at: string;
  summary: {
    hosts_total: number;
    hosts_online: number;
    hosts_stale: number;
    hosts_offline: number;
    services_total: number;
    services_healthy: number;
    services_degraded: number;
    services_down: number;
    service_availability_pct: number | null;
    open_incidents: number;
    avg_cpu_percent: number | null;
    avg_mem_percent: number | null;
    avg_disk_percent: number | null;
    total_net_rx_bps: number;
    total_net_tx_bps: number;
  };
  hosts: Host[];
  services: ServiceRecord[];
  recent_deployments: Deployment[];
  open_incident_list: Incident[];
  recent_alerts: AlertEvent[];
  insights: OperationalInsight[];
}

export interface AuditEvent {
  id: number;
  actor_email?: string;
  actor_agent_id?: string;
  action: string;
  resource_type: string;
  resource_id: string;
  status: string;
  ip_address: string;
  details: Record<string, unknown>;
  created_at: string;
}

export interface NotificationItem {
  id: string;
  kind: string;
  severity: string;
  title: string;
  body: string;
  link: string;
  read_at?: string;
  created_at: string;
}
