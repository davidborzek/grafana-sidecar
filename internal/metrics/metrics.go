// Package metrics exposes grafana-sidecar's own Prometheus metrics plus a
// liveness endpoint. Collectors live on a private registry so New is
// side-effect free.
package metrics

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds grafana-sidecar's instrumentation and its dedicated registry.
type Metrics struct {
	reg *prometheus.Registry

	reconciles    *prometheus.CounterVec
	reconcileTime prometheus.Histogram
	lastReconcile prometheus.Gauge
	lastSuccess   prometheus.Gauge
	ready         prometheus.Gauge
	containers    prometheus.Gauge
	dashboards    prometheus.Gauge
	files         prometheus.Gauge
	failed        prometheus.Gauge
	downloads     *prometheus.CounterVec
	watchRestarts prometheus.Counter
}

// New registers and returns the metric collectors on a private registry.
func New(version string) *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	m := &Metrics{
		reg: reg,
		reconciles: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grafana_sidecar_reconciles_total",
			Help: "Total reconcile runs by result.",
		}, []string{"result"}),
		reconcileTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "grafana_sidecar_reconcile_duration_seconds",
			Help:    "Duration of reconcile runs in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		lastReconcile: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "grafana_sidecar_last_reconcile_timestamp_seconds",
			Help: "Unix timestamp of the last completed reconcile.",
		}),
		lastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "grafana_sidecar_last_reconcile_success_timestamp_seconds",
			Help: "Unix timestamp of the last successful reconcile.",
		}),
		ready: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "grafana_sidecar_ready",
			Help: "1 if the last reconcile succeeded, else 0.",
		}),
		containers: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "grafana_sidecar_managed_containers",
			Help: "Containers currently declaring dashboards.",
		}),
		dashboards: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "grafana_sidecar_managed_dashboards",
			Help: "Dashboards currently declared via labels.",
		}),
		files: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "grafana_sidecar_dashboard_files",
			Help: "Dashboard files currently written to the dashboards directory.",
		}),
		failed: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "grafana_sidecar_failed_downloads",
			Help: "Dashboards that failed to download on the last reconcile.",
		}),
		downloads: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grafana_sidecar_downloads_total",
			Help: "Total dashboard download attempts by result.",
		}, []string{"result"}),
		watchRestarts: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "grafana_sidecar_watch_restarts_total",
			Help: "Total Docker event-stream resubscriptions.",
		}),
	}
	reg.MustRegister(m.reconciles, m.reconcileTime, m.lastReconcile, m.lastSuccess,
		m.ready, m.containers, m.dashboards, m.files, m.failed, m.downloads, m.watchRestarts)
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name:        "grafana_sidecar_build_info",
		Help:        "Build information, always 1.",
		ConstLabels: prometheus.Labels{"version": version},
	}, func() float64 { return 1 }))
	return m
}

// ObserveReconcile records the outcome and duration of one reconcile run.
func (m *Metrics) ObserveReconcile(success bool, d time.Duration) {
	m.reconciles.WithLabelValues(result(success)).Inc()
	m.reconcileTime.Observe(d.Seconds())
	m.lastReconcile.SetToCurrentTime()
	if success {
		m.lastSuccess.SetToCurrentTime()
		m.ready.Set(1)
	} else {
		m.ready.Set(0)
	}
}

// SetState records the current desired-state counts.
func (m *Metrics) SetState(containers, dashboards, files, failed int) {
	m.containers.Set(float64(containers))
	m.dashboards.Set(float64(dashboards))
	m.files.Set(float64(files))
	m.failed.Set(float64(failed))
}

// ObserveDownloads records download attempts from one reconcile.
func (m *Metrics) ObserveDownloads(succeeded, failed int) {
	if succeeded > 0 {
		m.downloads.WithLabelValues("success").Add(float64(succeeded))
	}
	if failed > 0 {
		m.downloads.WithLabelValues("error").Add(float64(failed))
	}
}

// ObserveWatchRestart counts a Docker event-stream resubscription.
func (m *Metrics) ObserveWatchRestart() {
	m.watchRestarts.Inc()
}

func result(success bool) string {
	if success {
		return "success"
	}
	return "error"
}

func (m *Metrics) handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "ok\n")
	})
	return mux
}

// Serve runs the /metrics and /healthz endpoints until ctx is cancelled. A
// blank addr disables the server.
func (m *Metrics) Serve(ctx context.Context, addr string, logger *slog.Logger) {
	if addr == "" {
		return
	}
	srv := &http.Server{Addr: addr, Handler: m.handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("metrics server error", "error", err)
		}
	}()
}
