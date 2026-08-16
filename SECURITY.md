# Security policy

## Reporting a vulnerability

Please **do not** open a public issue for security vulnerabilities.

Instead, report them privately via GitHub's
[security advisories](https://github.com/davidborzek/grafana-sidecar/security/advisories/new)
("Report a vulnerability"). You will receive a response as soon as possible, and
disclosure will be coordinated with you.

## Scope

grafana-sidecar connects to the Docker API to read container labels and
downloads dashboards over HTTP(S), writing dashboard JSON into a directory
Grafana provisions from. Restrict access to the Docker socket to the minimum
required (a read-only proxy exposing only containers + events), treat the
managed dashboards directory as controlled by grafana-sidecar, and note that a
container's `grafana.dashboard.url` label causes an outbound fetch of arbitrary
content — only label containers you trust.
