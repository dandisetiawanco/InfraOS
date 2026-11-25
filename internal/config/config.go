package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Profile                  string
	APIHost                  string
	APIPort                  int
	WebURL                   string
	AllowedOrigins           []string
	ExposeAPIDocs            bool
	DBDriver                 string
	SQLitePath               string
	PostgresDSN              string
	RedisAddr                string
	SessionTTL               time.Duration
	CookieSecure             bool
	LoginRateLimitPerMin     int
	MonitoringLocalAllowlist []string
	MonitoringMaxConcurrency int
	DockerEnabled            bool
	DockerSocket             string
	DockerAllowMutations     bool
	DockerMutationAllowlist  []string
	AIProvider               string
	AIEndpoint               string
	AIAPIKey                 string
	AIModel                  string
	OTELEnabled              bool
	OTELExporterEndpoint     string
	MetricsRetentionDays     int
	LogsRetentionDays        int
	ChecksRetentionDays      int
	EmbeddedWorker           bool
	BackupDir                string
	SecretEncryptionKey      string
}

func Load() (Config, error) {
	cfg := Config{
		Profile:                  envOrDefault("INFRAOS_PROFILE", "lightweight"),
		APIHost:                  envOrDefault("INFRAOS_API_HOST", "127.0.0.1"),
		APIPort:                  envInt("INFRAOS_API_PORT", 8080),
		WebURL:                   envOrDefault("INFRAOS_WEB_URL", "http://localhost:3000"),
		AllowedOrigins:           envCSV("INFRAOS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:3001,http://127.0.0.1:3000,http://127.0.0.1:3001"),
		ExposeAPIDocs:            envBool("INFRAOS_EXPOSE_API_DOCS", true),
		DBDriver:                 strings.ToLower(envOrDefault("INFRAOS_DB_DRIVER", "sqlite")),
		SQLitePath:               envOrDefault("INFRAOS_SQLITE_PATH", "./data/infraos.db"),
		PostgresDSN:              os.Getenv("INFRAOS_POSTGRES_DSN"),
		RedisAddr:                os.Getenv("INFRAOS_REDIS_ADDR"),
		SessionTTL:               time.Duration(envInt("INFRAOS_SESSION_TTL_HOURS", 24)) * time.Hour,
		CookieSecure:             envBool("INFRAOS_COOKIE_SECURE", false),
		LoginRateLimitPerMin:     envInt("INFRAOS_LOGIN_RATE_LIMIT_PER_MIN", 10),
		MonitoringLocalAllowlist: envCSV("INFRAOS_MONITORING_LOCAL_ALLOWLIST", "http://127.0.0.1:8080,http://localhost:8080,http://127.0.0.1:8090,http://localhost:8090"),
		MonitoringMaxConcurrency: envInt("INFRAOS_MONITORING_MAX_CONCURRENCY", 16),
		DockerEnabled:            envBool("INFRAOS_DOCKER_ENABLED", false),
		DockerSocket:             envOrDefault("INFRAOS_DOCKER_SOCKET", "/var/run/docker.sock"),
		DockerAllowMutations:     envBool("INFRAOS_DOCKER_ALLOW_MUTATIONS", false),
		DockerMutationAllowlist:  envCSV("INFRAOS_DOCKER_MUTATION_ALLOWLIST", ""),
		AIProvider:               strings.ToLower(envOrDefault("INFRAOS_AI_PROVIDER", "disabled")),
		AIEndpoint:               os.Getenv("INFRAOS_AI_ENDPOINT"),
		AIAPIKey:                 os.Getenv("INFRAOS_AI_API_KEY"),
		AIModel:                  os.Getenv("INFRAOS_AI_MODEL"),
		OTELEnabled:              envBool("INFRAOS_OTEL_ENABLED", false),
		OTELExporterEndpoint:     os.Getenv("INFRAOS_OTEL_EXPORTER_ENDPOINT"),
		MetricsRetentionDays:     envInt("INFRAOS_METRICS_RETENTION_DAYS", 7),
		LogsRetentionDays:        envInt("INFRAOS_LOGS_RETENTION_DAYS", 14),
		ChecksRetentionDays:      envInt("INFRAOS_CHECKS_RETENTION_DAYS", 14),
		EmbeddedWorker:           envBool("INFRAOS_EMBEDDED_WORKER", true),
		BackupDir:                envOrDefault("INFRAOS_BACKUP_DIR", "./backups"),
		SecretEncryptionKey:      envOrDefault("INFRAOS_SECRET_KEY", "infraos-dev-local-key-change-in-production"),
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	switch c.Profile {
	case "lightweight", "infrastructure", "production":
	default:
		return fmt.Errorf("invalid INFRAOS_PROFILE %q: must be lightweight, infrastructure, or production", c.Profile)
	}
	if c.APIPort < 1 || c.APIPort > 65535 {
		return fmt.Errorf("invalid INFRAOS_API_PORT %d", c.APIPort)
	}
	switch c.DBDriver {
	case "sqlite":
		if strings.TrimSpace(c.SQLitePath) == "" {
			return fmt.Errorf("INFRAOS_SQLITE_PATH is required for sqlite driver")
		}
	case "postgres":
		if strings.TrimSpace(c.PostgresDSN) == "" {
			return fmt.Errorf("INFRAOS_POSTGRES_DSN is required for postgres driver")
		}
	default:
		return fmt.Errorf("unsupported INFRAOS_DB_DRIVER %q", c.DBDriver)
	}
	if c.MonitoringMaxConcurrency < 1 || c.MonitoringMaxConcurrency > 128 {
		return fmt.Errorf("INFRAOS_MONITORING_MAX_CONCURRENCY must be between 1 and 128")
	}
	if c.Profile == "production" && !c.CookieSecure {
		return fmt.Errorf("INFRAOS_COOKIE_SECURE must be true in production profile")
	}
	return nil
}

func (c Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.APIHost, c.APIPort)
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes"
}

func envCSV(key, fallback string) []string {
	raw := os.Getenv(key)
	if strings.TrimSpace(raw) == "" {
		raw = fallback
	}
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
