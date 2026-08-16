// Package config loads grafana-sidecar settings from the environment.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all runtime settings. Everything is sourced from the environment
// to keep the sidecar trivially configurable in a compose file.
type Config struct {
	// DashboardsDir is the directory the sidecar owns and writes dashboard JSON
	// into. Share it (read-only) with Grafana's file provisioning provider.
	DashboardsDir string
	// LabelPrefix is the container label namespace, e.g. "grafana.dashboard" ->
	// "grafana.dashboard.<field>" and "grafana.dashboard.<name>.<field>".
	LabelPrefix string
	// GnetBaseURL is the grafana.com dashboards API base used for gnetId downloads.
	GnetBaseURL string
	// ResyncInterval is the periodic full reconcile interval, a safety net on
	// top of event-driven reconciliation.
	ResyncInterval time.Duration
	// DebounceDelay coalesces bursts of container events into one reconcile.
	DebounceDelay time.Duration
	// HTTPTimeout bounds a single dashboard download.
	HTTPTimeout time.Duration
	// LogLevel is one of debug, info, warn, error.
	LogLevel string
	// MetricsAddr is the listen address for /metrics and /healthz. Blank disables it.
	MetricsAddr string
	// Prune controls whether dashboards for containers that disappear (or drop
	// their labels) are deleted. Default true; set false to keep them.
	Prune bool
}

// Load reads the configuration from GRAFANA_SIDECAR_* environment variables,
// applying defaults for anything unset.
func Load() Config {
	return Config{
		DashboardsDir:  env("GRAFANA_SIDECAR_DASHBOARDS_DIR", "/dashboards"),
		LabelPrefix:    env("GRAFANA_SIDECAR_LABEL_PREFIX", "grafana.dashboard"),
		GnetBaseURL:    env("GRAFANA_SIDECAR_GNET_BASE_URL", "https://grafana.com/api/dashboards"),
		ResyncInterval: envDuration("GRAFANA_SIDECAR_RESYNC_INTERVAL", 5*time.Minute),
		DebounceDelay:  envDuration("GRAFANA_SIDECAR_DEBOUNCE_DELAY", 2*time.Second),
		HTTPTimeout:    envDuration("GRAFANA_SIDECAR_HTTP_TIMEOUT", 30*time.Second),
		LogLevel:       env("GRAFANA_SIDECAR_LOG_LEVEL", "info"),
		MetricsAddr:    env("GRAFANA_SIDECAR_METRICS_ADDR", ":9333"),
		Prune:          envBool("GRAFANA_SIDECAR_PRUNE", true),
	}
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
