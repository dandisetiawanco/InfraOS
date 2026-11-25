PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS organizations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    is_demo INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS organization_memberships (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'operator', 'viewer')),
    created_at TEXT NOT NULL,
    UNIQUE(organization_id, user_id)
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    csrf_token TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    repository_url TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(organization_id, slug)
);

CREATE TABLE IF NOT EXISTS environments (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    tier TEXT NOT NULL CHECK (tier IN ('production', 'staging', 'development')),
    description TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    UNIQUE(organization_id, project_id, slug)
);

CREATE TABLE IF NOT EXISTS hosts (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL,
    hostname TEXT NOT NULL,
    display_name TEXT NOT NULL,
    os TEXT NOT NULL DEFAULT '',
    arch TEXT NOT NULL DEFAULT '',
    kernel_version TEXT NOT NULL DEFAULT '',
    ip_address TEXT NOT NULL DEFAULT '',
    tags_json TEXT NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('online', 'offline', 'stale', 'revoked', 'pending')),
    is_synthetic INTEGER NOT NULL DEFAULT 0,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    last_heartbeat_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    version TEXT NOT NULL DEFAULT '0.1.0',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked', 'pending')),
    last_heartbeat_at TEXT,
    last_ip TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_enrollment_tokens (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL,
    label TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    used_at TEXT,
    created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS agent_credentials (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    prefix TEXT NOT NULL,
    rotated_from_id TEXT REFERENCES agent_credentials(id) ON DELETE SET NULL,
    revoked_at TEXT,
    expires_at TEXT,
    last_used_at TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS host_metrics (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    host_id TEXT NOT NULL REFERENCES hosts(id) ON DELETE CASCADE,
    recorded_at TEXT NOT NULL,
    cpu_percent REAL NOT NULL,
    cpu_cores INTEGER NOT NULL,
    load_1m REAL NOT NULL DEFAULT 0,
    load_5m REAL NOT NULL DEFAULT 0,
    load_15m REAL NOT NULL DEFAULT 0,
    mem_total_bytes INTEGER NOT NULL,
    mem_used_bytes INTEGER NOT NULL,
    mem_available_bytes INTEGER NOT NULL,
    mem_percent REAL NOT NULL,
    disk_total_bytes INTEGER NOT NULL,
    disk_used_bytes INTEGER NOT NULL,
    disk_percent REAL NOT NULL,
    net_rx_bytes INTEGER NOT NULL DEFAULT 0,
    net_tx_bytes INTEGER NOT NULL DEFAULT 0,
    net_rx_bps REAL NOT NULL DEFAULT 0,
    net_tx_bps REAL NOT NULL DEFAULT 0,
    uptime_seconds INTEGER NOT NULL DEFAULT 0,
    filesystems_json TEXT NOT NULL DEFAULT '[]',
    interfaces_json TEXT NOT NULL DEFAULT '[]',
    optional_json TEXT NOT NULL DEFAULT '{}',
    is_synthetic INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS services (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
    environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL,
    host_id TEXT REFERENCES hosts(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    check_type TEXT NOT NULL CHECK (check_type IN ('http', 'https', 'tcp', 'dns', 'tls')),
    target TEXT NOT NULL,
    interval_seconds INTEGER NOT NULL DEFAULT 30,
    timeout_ms INTEGER NOT NULL DEFAULT 5000,
    expected_status INTEGER NOT NULL DEFAULT 200,
    failure_threshold INTEGER NOT NULL DEFAULT 2,
    recovery_threshold INTEGER NOT NULL DEFAULT 2,
    enabled INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'unknown' CHECK (status IN ('healthy', 'degraded', 'down', 'unknown')),
    consecutive_failures INTEGER NOT NULL DEFAULT 0,
    consecutive_successes INTEGER NOT NULL DEFAULT 0,
    last_checked_at TEXT,
    last_latency_ms INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    tls_expires_at TEXT,
    is_synthetic INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS service_checks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    checked_at TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('up', 'down', 'degraded')),
    status_code INTEGER NOT NULL DEFAULT 0,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    failure_reason TEXT NOT NULL DEFAULT '',
    tls_days_remaining INTEGER,
    resolved_ips_json TEXT NOT NULL DEFAULT '[]',
    is_synthetic INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS containers (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    host_id TEXT REFERENCES hosts(id) ON DELETE SET NULL,
    environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL,
    external_id TEXT NOT NULL,
    name TEXT NOT NULL,
    image TEXT NOT NULL,
    state TEXT NOT NULL,
    health_status TEXT NOT NULL DEFAULT 'none',
    cpu_percent REAL NOT NULL DEFAULT 0,
    mem_usage_bytes INTEGER NOT NULL DEFAULT 0,
    mem_limit_bytes INTEGER NOT NULL DEFAULT 0,
    networks_json TEXT NOT NULL DEFAULT '[]',
    restart_count INTEGER NOT NULL DEFAULT 0,
    container_created_at TEXT,
    allow_mutations INTEGER NOT NULL DEFAULT 0,
    is_synthetic INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL,
    UNIQUE(organization_id, external_id)
);

CREATE TABLE IF NOT EXISTS container_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    container_id TEXT NOT NULL REFERENCES containers(id) ON DELETE CASCADE,
    recorded_at TEXT NOT NULL,
    state TEXT NOT NULL,
    health_status TEXT NOT NULL DEFAULT 'none',
    cpu_percent REAL NOT NULL DEFAULT 0,
    mem_usage_bytes INTEGER NOT NULL DEFAULT 0,
    restart_count INTEGER NOT NULL DEFAULT 0,
    is_synthetic INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS deployment_targets (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
    environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL,
    host_id TEXT REFERENCES hosts(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    adapter TEXT NOT NULL CHECK (adapter IN ('local_http', 'simulation', 'docker_compose')),
    config_json TEXT NOT NULL DEFAULT '{}',
    health_url TEXT NOT NULL DEFAULT '',
    current_version TEXT NOT NULL DEFAULT '',
    current_artifact_id TEXT,
    locked_by_deployment_id TEXT,
    locked_at TEXT,
    is_synthetic INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS deployment_artifacts (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES deployment_targets(id) ON DELETE CASCADE,
    version TEXT NOT NULL,
    commit_sha TEXT NOT NULL DEFAULT '',
    commit_message TEXT NOT NULL DEFAULT '',
    branch TEXT NOT NULL DEFAULT 'main',
    image_ref TEXT NOT NULL DEFAULT '',
    checksum TEXT NOT NULL DEFAULT '',
    build_metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS deployments (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES deployment_targets(id) ON DELETE CASCADE,
    artifact_id TEXT REFERENCES deployment_artifacts(id) ON DELETE SET NULL,
    previous_artifact_id TEXT REFERENCES deployment_artifacts(id) ON DELETE SET NULL,
    environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL,
    idempotency_key TEXT,
    version TEXT NOT NULL,
    commit_sha TEXT NOT NULL DEFAULT '',
    commit_message TEXT NOT NULL DEFAULT '',
    branch TEXT NOT NULL DEFAULT 'main',
    status TEXT NOT NULL CHECK (status IN ('queued', 'preparing', 'building', 'deploying', 'verifying', 'succeeded', 'failed', 'rolling_back', 'rolled_back', 'cancelled')),
    trigger_type TEXT NOT NULL DEFAULT 'manual' CHECK (trigger_type IN ('manual', 'rollback', 'webhook', 'demo')),
    is_simulation INTEGER NOT NULL DEFAULT 0,
    triggered_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    started_at TEXT,
    finished_at TEXT,
    error_message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    UNIQUE(organization_id, idempotency_key)
);

CREATE TABLE IF NOT EXISTS deployment_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    deployment_id TEXT NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    state TEXT NOT NULL,
    level TEXT NOT NULL DEFAULT 'info',
    message TEXT NOT NULL,
    details_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS log_sources (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    host_id TEXT REFERENCES hosts(id) ON DELETE SET NULL,
    service_id TEXT REFERENCES services(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    source_type TEXT NOT NULL CHECK (source_type IN ('application', 'agent', 'monitoring', 'deployment', 'container')),
    retention_days INTEGER NOT NULL DEFAULT 14,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS log_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    source_id TEXT REFERENCES log_sources(id) ON DELETE SET NULL,
    host_id TEXT REFERENCES hosts(id) ON DELETE SET NULL,
    service_id TEXT REFERENCES services(id) ON DELETE SET NULL,
    environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL,
    timestamp TEXT NOT NULL,
    severity TEXT NOT NULL CHECK (severity IN ('debug', 'info', 'warn', 'error', 'fatal')),
    source_name TEXT NOT NULL,
    message TEXT NOT NULL,
    correlation_id TEXT NOT NULL DEFAULT '',
    fields_json TEXT NOT NULL DEFAULT '{}',
    is_synthetic INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS alert_rules (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    rule_type TEXT NOT NULL CHECK (rule_type IN ('high_cpu', 'high_memory', 'low_disk', 'agent_offline', 'service_down', 'high_latency', 'tls_expiring', 'deployment_failed')),
    target_ref TEXT NOT NULL DEFAULT '*',
    threshold REAL NOT NULL,
    duration_seconds INTEGER NOT NULL DEFAULT 60,
    cooldown_seconds INTEGER NOT NULL DEFAULT 300,
    severity TEXT NOT NULL DEFAULT 'warning' CHECK (severity IN ('info', 'warning', 'critical')),
    auto_incident INTEGER NOT NULL DEFAULT 0,
    webhook_url TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    state TEXT NOT NULL DEFAULT 'ok' CHECK (state IN ('ok', 'pending', 'firing')),
    pending_since TEXT,
    last_fired_at TEXT,
    last_resolved_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS incidents (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    severity TEXT NOT NULL DEFAULT 'sev3' CHECK (severity IN ('sev1', 'sev2', 'sev3', 'sev4')),
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'investigating', 'identified', 'monitoring', 'resolved')),
    affected_service_ids_json TEXT NOT NULL DEFAULT '[]',
    assigned_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    root_cause_notes TEXT NOT NULL DEFAULT '',
    resolution_summary TEXT NOT NULL DEFAULT '',
    postmortem_markdown TEXT NOT NULL DEFAULT '',
    dedupe_key TEXT NOT NULL DEFAULT '',
    is_synthetic INTEGER NOT NULL DEFAULT 0,
    opened_at TEXT NOT NULL,
    resolved_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS alert_events (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    rule_id TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    host_id TEXT REFERENCES hosts(id) ON DELETE SET NULL,
    service_id TEXT REFERENCES services(id) ON DELETE SET NULL,
    state TEXT NOT NULL CHECK (state IN ('pending', 'firing', 'resolved')),
    severity TEXT NOT NULL,
    summary TEXT NOT NULL,
    metric_value REAL NOT NULL DEFAULT 0,
    threshold REAL NOT NULL DEFAULT 0,
    fingerprint TEXT NOT NULL,
    incident_id TEXT REFERENCES incidents(id) ON DELETE SET NULL,
    triggered_at TEXT NOT NULL,
    resolved_at TEXT,
    is_synthetic INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS incident_timelines (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    incident_id TEXT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK (event_type IN ('status_change', 'note', 'alert_linked', 'deployment_linked', 'ai_summary', 'postmortem')),
    author_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    message TEXT NOT NULL,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS notifications (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info',
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    link TEXT NOT NULL DEFAULT '',
    read_at TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    actor_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    actor_agent_id TEXT REFERENCES agents(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'success',
    ip_address TEXT NOT NULL DEFAULT '',
    request_id TEXT NOT NULL DEFAULT '',
    details_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS integration_settings (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider IN ('github', 'docker', 'prometheus', 'otel', 'ai')),
    status TEXT NOT NULL DEFAULT 'disconnected' CHECK (status IN ('connected', 'disconnected', 'error')),
    config_json TEXT NOT NULL DEFAULT '{}',
    secret_masked TEXT NOT NULL DEFAULT '',
    secret_encrypted TEXT NOT NULL DEFAULT '',
    last_verified_at TEXT,
    updated_at TEXT NOT NULL,
    UNIQUE(organization_id, provider)
);

CREATE TABLE IF NOT EXISTS webhook_deliveries (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    delivery_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    status TEXT NOT NULL,
    payload_summary_json TEXT NOT NULL DEFAULT '{}',
    received_at TEXT NOT NULL,
    UNIQUE(provider, delivery_id)
);

CREATE TABLE IF NOT EXISTS topology_links (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL,
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT 'depends_on',
    is_manual INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS backup_records (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    driver TEXT NOT NULL,
    file_path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    sha256 TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('completed', 'verified', 'failed')),
    notes TEXT NOT NULL DEFAULT '',
    created_by TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memberships_user ON organization_memberships(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_hosts_org_env ON hosts(organization_id, environment_id);
CREATE INDEX IF NOT EXISTS idx_host_metrics_host_time ON host_metrics(host_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_host_metrics_org_time ON host_metrics(organization_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_services_org_env ON services(organization_id, environment_id);
CREATE INDEX IF NOT EXISTS idx_service_checks_service_time ON service_checks(service_id, checked_at DESC);
CREATE INDEX IF NOT EXISTS idx_containers_org ON containers(organization_id, host_id);
CREATE INDEX IF NOT EXISTS idx_deployments_org_created ON deployments(organization_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_deployment_events_dep ON deployment_events(deployment_id, id ASC);
CREATE INDEX IF NOT EXISTS idx_log_entries_org_time ON log_entries(organization_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_log_entries_sev ON log_entries(organization_id, severity, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_alert_events_org_time ON alert_events(organization_id, triggered_at DESC);
CREATE INDEX IF NOT EXISTS idx_incidents_org_status ON incidents(organization_id, status, opened_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_org_unread ON notifications(organization_id, read_at, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_org_time ON audit_events(organization_id, created_at DESC);
