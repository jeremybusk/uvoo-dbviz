package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr       string
	PublicURL  string
	WebRoot    string
	Auth       AuthConfig
	ClickHouse ClickHouseConfig
	PostgREST  PostgRESTConfig
	Datasets   map[string]Dataset
	Alerts     AlertConfig
}

type AuthConfig struct {
	DevMode   bool
	Providers []OIDCProvider
}

type OIDCProvider struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Issuer              string   `json:"issuer"`
	ClientID            string   `json:"clientId"`
	ClientSecret        string   `json:"-"`
	Audience            []string `json:"audience"`
	Scopes              []string `json:"scopes"`
	TenantClaim         string   `json:"tenantClaim"`
	EmailClaim          string   `json:"emailClaim"`
	NameClaim           string   `json:"nameClaim"`
	Enabled             bool     `json:"enabled"`
	AllowIssuerPrefixes []string `json:"allowIssuerPrefixes"`
	AllowedDomains      []string `json:"allowedDomains"`
}

type ClickHouseConfig struct {
	URL             string
	User            string
	Password        string
	Database        string
	Timeout         time.Duration
	MaxRows         int
	MaxQuerySeconds int
}

type PostgRESTConfig struct {
	URL string `json:"url"`
}

type AlertConfig struct {
	Enabled       bool
	Rules         string
	WorkerKey     string
	PollSeconds   int
	LoadPersisted bool
}

type Dataset struct {
	ID                 string              `json:"id"`
	Name               string              `json:"name"`
	Table              string              `json:"table"`
	TimeColumn         string              `json:"timeColumn"`
	TenantColumn       string              `json:"tenantColumn"`
	Dimensions         []string            `json:"dimensions"`
	Filters            []string            `json:"filters"`
	FilterOperators    map[string][]string `json:"filterOperators"`
	Measures           []string            `json:"measures"`
	Aggregations       []string            `json:"aggregations"`
	DefaultMeasure     string              `json:"defaultMeasure"`
	DefaultAggregation string              `json:"defaultAggregation"`
	MaxLookbackHours   int                 `json:"maxLookbackHours"`
	MaxRows            int                 `json:"maxRows"`
}

type PublicProvider struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Issuer   string   `json:"issuer"`
	ClientID string   `json:"clientId"`
	Scopes   []string `json:"scopes"`
	Enabled  bool     `json:"enabled"`
}

type PublicConfig struct {
	Providers []PublicProvider `json:"providers"`
	Datasets  []Dataset        `json:"datasets"`
	DevMode   bool             `json:"devMode"`
}

func Load() Config {
	cfg := Config{
		Addr:      env("DBVIZ_ADDR", ":8080"),
		PublicURL: env("DBVIZ_PUBLIC_URL", "http://localhost:8080"),
		WebRoot:   env("DBVIZ_WEB_ROOT", "web/dist"),
		Auth: AuthConfig{
			DevMode: envBool("DBVIZ_AUTH_DEV_MODE", false),
			Providers: []OIDCProvider{
				{
					ID:          "google",
					Name:        "Google",
					Issuer:      "https://accounts.google.com",
					ClientID:    os.Getenv("DBVIZ_OIDC_GOOGLE_CLIENT_ID"),
					Audience:    csv("DBVIZ_OIDC_GOOGLE_AUDIENCE", os.Getenv("DBVIZ_OIDC_GOOGLE_CLIENT_ID")),
					Scopes:      []string{"openid", "email", "profile"},
					TenantClaim: env("DBVIZ_OIDC_GOOGLE_TENANT_CLAIM", "hd"),
					EmailClaim:  "email",
					NameClaim:   "name",
					Enabled:     envBool("DBVIZ_OIDC_GOOGLE_ENABLED", true),
				},
				{
					ID:                  "microsoft",
					Name:                "Microsoft",
					Issuer:              env("DBVIZ_OIDC_MICROSOFT_ISSUER", "https://login.microsoftonline.com/common/v2.0"),
					ClientID:            os.Getenv("DBVIZ_OIDC_MICROSOFT_CLIENT_ID"),
					Audience:            csv("DBVIZ_OIDC_MICROSOFT_AUDIENCE", os.Getenv("DBVIZ_OIDC_MICROSOFT_CLIENT_ID")),
					Scopes:              []string{"openid", "email", "profile"},
					TenantClaim:         env("DBVIZ_OIDC_MICROSOFT_TENANT_CLAIM", "tid"),
					EmailClaim:          "preferred_username",
					NameClaim:           "name",
					Enabled:             envBool("DBVIZ_OIDC_MICROSOFT_ENABLED", true),
					AllowIssuerPrefixes: []string{"https://login.microsoftonline.com/"},
				},
			},
		},
		ClickHouse: ClickHouseConfig{
			URL:             env("DBVIZ_CLICKHOUSE_URL", "http://localhost:8123"),
			User:            env("DBVIZ_CLICKHOUSE_USER", "default"),
			Password:        os.Getenv("DBVIZ_CLICKHOUSE_PASSWORD"),
			Database:        env("DBVIZ_CLICKHOUSE_DATABASE", "default"),
			Timeout:         time.Duration(envInt("DBVIZ_CLICKHOUSE_TIMEOUT_SECONDS", 30)) * time.Second,
			MaxRows:         envInt("DBVIZ_CLICKHOUSE_MAX_ROWS", 10000),
			MaxQuerySeconds: envInt("DBVIZ_CLICKHOUSE_MAX_QUERY_SECONDS", 20),
		},
		PostgREST: PostgRESTConfig{URL: env("DBVIZ_POSTGREST_URL", "/state")},
		Alerts: AlertConfig{
			Enabled:       envBool("DBVIZ_ALERTS_ENABLED", false),
			Rules:         os.Getenv("DBVIZ_ALERT_RULES_JSON"),
			WorkerKey:     env("DBVIZ_ALERT_WORKER_KEY", "dev-alert-worker-key"),
			PollSeconds:   envInt("DBVIZ_ALERT_POLL_SECONDS", 30),
			LoadPersisted: envBool("DBVIZ_ALERT_LOAD_PERSISTED", true),
		},
		Datasets: map[string]Dataset{
			"logs": {
				ID:           "logs",
				Name:         "Logs",
				Table:        env("DBVIZ_CLICKHOUSE_LOGS_TABLE", "otel_logs"),
				TimeColumn:   env("DBVIZ_CLICKHOUSE_LOGS_TIME_COLUMN", "timestamp"),
				TenantColumn: env("DBVIZ_CLICKHOUSE_LOGS_TENANT_COLUMN", "tenant_id"),
				Dimensions:   csvDefault("DBVIZ_CLICKHOUSE_LOGS_DIMENSIONS", []string{"service_name", "severity", "host_name"}),
				Filters:      csvDefault("DBVIZ_CLICKHOUSE_LOGS_FILTERS", []string{"service_name", "severity", "host_name", "trace_id"}),
				FilterOperators: map[string][]string{
					"service_name": {"eq", "contains"},
					"severity":     {"eq"},
					"host_name":    {"eq", "contains"},
					"trace_id":     {"eq"},
				},
				Measures:           []string{"_rows"},
				Aggregations:       []string{"count"},
				DefaultMeasure:     "_rows",
				DefaultAggregation: "count",
				MaxLookbackHours:   envInt("DBVIZ_CLICKHOUSE_LOGS_MAX_LOOKBACK_HOURS", 168),
				MaxRows:            envInt("DBVIZ_CLICKHOUSE_LOGS_MAX_ROWS", 5000),
			},
			"traces": {
				ID:           "traces",
				Name:         "Traces",
				Table:        env("DBVIZ_CLICKHOUSE_TRACES_TABLE", "otel_traces"),
				TimeColumn:   env("DBVIZ_CLICKHOUSE_TRACES_TIME_COLUMN", "timestamp"),
				TenantColumn: env("DBVIZ_CLICKHOUSE_TRACES_TENANT_COLUMN", "tenant_id"),
				Dimensions:   csvDefault("DBVIZ_CLICKHOUSE_TRACES_DIMENSIONS", []string{"service_name", "span_name", "status_code"}),
				Filters:      csvDefault("DBVIZ_CLICKHOUSE_TRACES_FILTERS", []string{"service_name", "span_name", "status_code", "trace_id"}),
				FilterOperators: map[string][]string{
					"service_name": {"eq", "contains"},
					"span_name":    {"eq", "contains"},
					"status_code":  {"eq"},
					"trace_id":     {"eq"},
				},
				Measures:           []string{"duration_ms", "_rows"},
				Aggregations:       []string{"count", "avg", "max", "p95"},
				DefaultMeasure:     "_rows",
				DefaultAggregation: "count",
				MaxLookbackHours:   envInt("DBVIZ_CLICKHOUSE_TRACES_MAX_LOOKBACK_HOURS", 168),
				MaxRows:            envInt("DBVIZ_CLICKHOUSE_TRACES_MAX_ROWS", 5000),
			},
			"metrics": {
				ID:           "metrics",
				Name:         "Metrics",
				Table:        env("DBVIZ_CLICKHOUSE_METRICS_TABLE", "otel_metrics"),
				TimeColumn:   env("DBVIZ_CLICKHOUSE_METRICS_TIME_COLUMN", "timestamp"),
				TenantColumn: env("DBVIZ_CLICKHOUSE_METRICS_TENANT_COLUMN", "tenant_id"),
				Dimensions:   csvDefault("DBVIZ_CLICKHOUSE_METRICS_DIMENSIONS", []string{"service_name", "metric_name"}),
				Filters:      csvDefault("DBVIZ_CLICKHOUSE_METRICS_FILTERS", []string{"service_name", "metric_name"}),
				FilterOperators: map[string][]string{
					"service_name": {"eq", "contains"},
					"metric_name":  {"eq", "contains"},
				},
				Measures:           []string{"value"},
				Aggregations:       []string{"avg", "sum", "max", "min", "p95"},
				DefaultMeasure:     "value",
				DefaultAggregation: "avg",
				MaxLookbackHours:   envInt("DBVIZ_CLICKHOUSE_METRICS_MAX_LOOKBACK_HOURS", 720),
				MaxRows:            envInt("DBVIZ_CLICKHOUSE_METRICS_MAX_ROWS", 10000),
			},
		},
	}

	if keycloakIssuer := os.Getenv("DBVIZ_OIDC_KEYCLOAK_ISSUER"); keycloakIssuer != "" {
		cfg.Auth.Providers = append(cfg.Auth.Providers, OIDCProvider{
			ID:           "keycloak",
			Name:         env("DBVIZ_OIDC_KEYCLOAK_NAME", "Keycloak"),
			Issuer:       keycloakIssuer,
			ClientID:     os.Getenv("DBVIZ_OIDC_KEYCLOAK_CLIENT_ID"),
			ClientSecret: os.Getenv("DBVIZ_OIDC_KEYCLOAK_CLIENT_SECRET"),
			Audience:     csv("DBVIZ_OIDC_KEYCLOAK_AUDIENCE", os.Getenv("DBVIZ_OIDC_KEYCLOAK_CLIENT_ID")),
			Scopes:       []string{"openid", "email", "profile"},
			TenantClaim:  env("DBVIZ_OIDC_KEYCLOAK_TENANT_CLAIM", "tenant_id"),
			EmailClaim:   env("DBVIZ_OIDC_KEYCLOAK_EMAIL_CLAIM", "email"),
			NameClaim:    env("DBVIZ_OIDC_KEYCLOAK_NAME_CLAIM", "name"),
			Enabled:      envBool("DBVIZ_OIDC_KEYCLOAK_ENABLED", true),
		})
	}

	if raw := os.Getenv("DBVIZ_OIDC_PROVIDERS_JSON"); raw != "" {
		var extra []OIDCProvider
		if err := json.Unmarshal([]byte(raw), &extra); err == nil {
			cfg.Auth.Providers = append(cfg.Auth.Providers, extra...)
		}
	}
	if raw := os.Getenv("DBVIZ_DATASETS_JSON"); raw != "" {
		var datasets []Dataset
		if err := json.Unmarshal([]byte(raw), &datasets); err == nil {
			for _, ds := range datasets {
				cfg.Datasets[ds.ID] = ds
			}
		}
	}
	return cfg
}

func (c Config) Public() PublicConfig {
	providers := make([]PublicProvider, 0, len(c.Auth.Providers))
	for _, provider := range c.Auth.Providers {
		providers = append(providers, PublicProvider{
			ID:       provider.ID,
			Name:     provider.Name,
			Issuer:   provider.Issuer,
			ClientID: provider.ClientID,
			Scopes:   provider.Scopes,
			Enabled:  provider.Enabled,
		})
	}
	datasets := make([]Dataset, 0, len(c.Datasets))
	for _, dataset := range c.Datasets {
		datasets = append(datasets, dataset)
	}
	return PublicConfig{
		Providers: providers,
		Datasets:  datasets,
		DevMode:   c.Auth.DevMode,
	}
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func envInt(name string, fallback int) int {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func csv(name, fallback string) []string {
	value := env(name, fallback)
	if value == "" {
		return nil
	}
	return splitCSV(value)
}

func csvDefault(name string, fallback []string) []string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return splitCSV(value)
	}
	return fallback
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
