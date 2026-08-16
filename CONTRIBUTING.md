# Contributing

Thanks for your interest in grafana-sidecar! Contributions are welcome.

## Development

grafana-sidecar is a standard Go module (Go 1.26+). No code generation or extra
tooling is required.

```sh
go build ./...      # build
go vet ./...        # vet
go test ./...       # unit tests
```

Formatting is enforced in CI — run `gofmt -w .` before committing.

### Extending grafana-sidecar

The architecture is deliberately small — grafana-sidecar watches the Docker API
for containers carrying `grafana.dashboard.*` labels, downloads each declared
dashboard, renders it (datasource substitution + `editable:false`), and syncs
the dashboards directory. See the [README](README.md), the label grammar in
[LABEL-SPEC.md](LABEL-SPEC.md), and the implementations under `internal/source`,
`internal/fetch`, and `internal/reconcile`.

## Pull requests

- Keep changes focused; one logical change per PR.
- Use [Conventional Commits](https://www.conventionalcommits.org/) for commit
  messages (`feat:`, `fix:`, `docs:`, `refactor:`, `ci:` …).
- Add or update tests for behavioural changes.
- Make sure `gofmt`, `go vet`, and `go test ./...` pass.

## Reporting issues

Use the issue templates. For security-sensitive reports, see
[SECURITY.md](SECURITY.md).

## Releases

Releases are automated — no manual tagging:

- **[release-please](https://github.com/googleapis/release-please)** watches
  `main` and, from the Conventional Commit history, maintains a "release PR"
  that bumps the version and updates `CHANGELOG.md`. Merging it creates the tag
  and the GitHub release.
- **[goreleaser](https://goreleaser.com/)** then builds the binaries and the
  multi-arch (`amd64`/`arm64`) image, pushes it to
  `ghcr.io/davidborzek/grafana-sidecar`, and attaches archives + checksums to
  the release — in the same workflow run (hanging off release-please's output,
  so no PAT is needed).
