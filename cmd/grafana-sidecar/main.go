// Command grafana-sidecar watches Docker containers and provisions the remote
// Grafana dashboards declared on their labels into a directory Grafana reads.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/docker/docker/client"
	"github.com/urfave/cli/v3"

	"github.com/davidborzek/grafana-sidecar/internal/config"
	"github.com/davidborzek/grafana-sidecar/internal/fetch"
	"github.com/davidborzek/grafana-sidecar/internal/metrics"
	"github.com/davidborzek/grafana-sidecar/internal/reconcile"
	"github.com/davidborzek/grafana-sidecar/internal/source"
)

// version is overridable at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cmd := &cli.Command{
		Name:    "grafana-sidecar",
		Usage:   "provision remote Grafana dashboards from Docker container labels",
		Version: version,
		Description: "Configured via GRAFANA_SIDECAR_* environment variables (see the README).\n" +
			"grafana-sidecar watches the Docker API for containers carrying grafana.dashboard\n" +
			"labels, downloads each declared dashboard (grafana.com id or url), and writes it\n" +
			"into a directory provisioned by Grafana's file provider.",
		Action: runApp,
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		os.Exit(1)
	}
}

func runApp(_ context.Context, _ *cli.Command) error {
	cfg := config.Load()
	logger := newLogger(cfg.LogLevel)
	if err := run(cfg, logger); err != nil {
		logger.Error("fatal", "error", err)
		return cli.Exit("", 1)
	}
	return nil
}

func run(cfg config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dockerCli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer func() { _ = dockerCli.Close() }()

	m := metrics.New(version)
	m.Serve(ctx, cfg.MetricsAddr, logger)

	src := source.New(dockerCli, cfg.LabelPrefix, logger)
	f := fetch.New(cfg.GnetBaseURL, cfg.HTTPTimeout)
	rec := reconcile.New(src, f, cfg.DashboardsDir, cfg.Prune, logger)

	logger.Info("grafana-sidecar started",
		"version", version,
		"dashboards_dir", cfg.DashboardsDir,
		"label_prefix", cfg.LabelPrefix,
		"resync_interval", cfg.ResyncInterval,
		"prune", cfg.Prune,
	)

	reconcileOnce(ctx, rec, m, logger)

	changes := src.Watch(ctx, m.ObserveWatchRestart)
	ticker := time.NewTicker(cfg.ResyncInterval)
	defer ticker.Stop()

	var debounce <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down")
			return nil
		case _, ok := <-changes:
			if !ok {
				changes = nil
				continue
			}
			if debounce == nil {
				debounce = time.After(cfg.DebounceDelay)
			}
		case <-debounce:
			debounce = nil
			reconcileOnce(ctx, rec, m, logger)
		case <-ticker.C:
			reconcileOnce(ctx, rec, m, logger)
		}
	}
}

func reconcileOnce(ctx context.Context, rec *reconcile.Reconciler, m *metrics.Metrics, logger *slog.Logger) {
	start := time.Now()
	res, err := rec.Run(ctx)

	m.ObserveReconcile(err == nil, time.Since(start))
	if err == nil {
		m.SetState(res.Containers, res.Dashboards, res.Files, res.Failed)
		m.ObserveDownloads(res.Downloaded, res.Failed)
		if res.Changed {
			logger.Info("dashboards changed",
				"containers", res.Containers,
				"dashboards", res.Dashboards,
				"files", res.Files,
				"downloaded", res.Downloaded,
				"failed", res.Failed,
			)
		}
	} else {
		logger.Error("reconcile failed", "error", err)
	}
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}
