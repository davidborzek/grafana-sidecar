---
name: Bug report
about: Report a problem with grafana-sidecar
labels: bug
---

**What happened**

A clear description of the bug.

**Expected behaviour**

What you expected to happen instead.

**Configuration**

- Relevant `GRAFANA_SIDECAR_*` variables (dashboards dir, label prefix, intervals):
- The container `grafana.dashboard.*` labels that triggered the behaviour:
- `/metrics` output if relevant (e.g. `grafana_sidecar_failed_downloads`,
  `grafana_sidecar_downloads_total`):

**Logs**

Run with `GRAFANA_SIDECAR_LOG_LEVEL=debug` and paste the relevant output.

**Environment**

- grafana-sidecar image tag / version:
- Docker version:
- Grafana version:
