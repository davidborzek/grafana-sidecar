# Label specification

grafana-sidecar discovers dashboards from Docker container labels under a
configurable prefix (`GRAFANA_SIDECAR_LABEL_PREFIX`, default `grafana.dashboard`).

## Grammar

```
<prefix>.<field>            # shorthand: one dashboard, named after the container
<prefix>.<name>.<field>     # named: several dashboards per container
```

`<field>` is one of:

| Field | Type | Meaning |
| --- | --- | --- |
| `gnetId` | int | grafana.com dashboard id. Requires `revision`. |
| `revision` | int | Revision of the grafana.com dashboard. |
| `url` | string | Direct download URL. Mutually exclusive with `gnetId`/`revision`. |
| `folder` | string | Grafana folder — a subdirectory under the dashboards dir. Nesting (`a/b`) is allowed. Optional (default: root). |
| `datasource` | string | Datasource name substituted into every `${DS_*}` input. Optional. |

`<name>` is the dashboard name; with the shorthand form it defaults to the
container name. It becomes part of the output filename, so it must not contain
`/`, `\`, or `..`.

## Rules

- A dashboard must specify **either** `gnetId` + `revision` **or** `url` — never
  both, never neither. Invalid declarations are logged and skipped; they never
  abort the reconcile.
- `datasource` (a single datasource **name**) replaces every `${DS_*}` input in
  the downloaded JSON. Lines referencing a Grafana built-in datasource sentinel
  (`-- Grafana --`, `-- Mixed --`, …) are left untouched. Dashboards that use a
  datasource *variable* usually need no `datasource`.
- Every dashboard is rendered read-only (`editable: false`) — these are managed
  files, and UI edits could not be persisted anyway.

## Output

Each dashboard is written to `<folder>/<file>.json` under the dashboards
directory, where `<file>` is:

- `<container>` for the shorthand form, or
- `<container>.<name>` for the named form

so dashboards from different containers never collide. Grafana's
`foldersFromFilesStructure` maps the `<folder>` subdirectory to a Grafana folder.

## Examples

```yaml
# one grafana.com dashboard, shorthand
labels:
  grafana.dashboard.gnetId: "1860"
  grafana.dashboard.revision: "45"
  grafana.dashboard.datasource: Prometheus
  grafana.dashboard.folder: System
```

```yaml
# several dashboards on one container, named
labels:
  grafana.dashboard.overview.url: https://example.test/overview.json
  grafana.dashboard.overview.folder: Apps
  grafana.dashboard.internals.gnetId: "13240"
  grafana.dashboard.internals.revision: "2"
  grafana.dashboard.internals.datasource: Prometheus
```
