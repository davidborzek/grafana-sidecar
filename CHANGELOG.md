# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0](https://github.com/davidborzek/grafana-sidecar/compare/v0.1.0...v0.2.0) (2026-08-16)


### Features

* initial grafana-sidecar ([82c180d](https://github.com/davidborzek/grafana-sidecar/commit/82c180d99fa06e0013112158d1eba92bee03c3c5))

## [0.1.0] - 2026-08-16

Initial release.

### Added

- Event-driven reconcile over the Docker API (debounced, with periodic resync).
- Remote dashboards from container labels: `grafana.dashboard.gnetId`/`.revision`
  or `.url`, plus optional `.folder` and `.datasource`; shorthand (one per
  container) and named (`grafana.dashboard.<name>.<field>`, several per container).
- Rendering: `${DS_*}` datasource substitution and forced `editable:false`.
- Owns the dashboards directory — orphaned files and empty folders are pruned to
  match running containers; unchanged dashboards are not re-downloaded.
- `grafana_sidecar_*` metrics on `/metrics` and a `/healthz` liveness probe.
- Configuration via `GRAFANA_SIDECAR_*` environment variables.
- Multi-arch image (`linux/amd64`, `linux/arm64`) on `ghcr.io/davidborzek/grafana-sidecar`.
