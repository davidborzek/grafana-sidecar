// Package source discovers dashboard-carrying containers from the Docker engine
// and watches for lifecycle changes.
//
// A container declares one or more remote dashboards through labels:
//
//	grafana.dashboard.gnetId / .revision / .url / .folder / .datasource
//	grafana.dashboard.<name>.gnetId / .revision / .url / .folder / .datasource
//
// The unnamed (shorthand) form is a single dashboard named after the container;
// the named form allows several dashboards on one container. Each dashboard is
// either a grafana.com dashboard (gnetId + revision) or a direct url.
package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

// Dashboard is one remote dashboard declared on a container via labels.
type Dashboard struct {
	Container  string // owning container (namespaces the output file)
	Name       string // dashboard name (label segment, or the container name)
	Folder     string // Grafana folder (a subdir under the dashboards dir)
	GnetID     int    // grafana.com dashboard id (with Revision) ...
	Revision   int    // ... its revision ...
	URL        string // ... OR a direct download URL
	Datasource string // optional: replaces every ${DS_*} input
}

// Filename is the JSON file the sidecar manages for this dashboard, relative to
// the dashboards directory (forward slashes). It is namespaced by container so
// two containers never collide, and nested under Folder so Grafana's
// foldersFromFilesStructure maps it to the right folder.
func (d Dashboard) Filename() string {
	base := d.Container
	if d.Name != d.Container {
		base = d.Container + "." + d.Name
	}
	return path.Join(d.Folder, base+".json")
}

// SpecHash changes whenever anything affecting the rendered output changes, so
// the reconciler can skip re-downloading an unchanged dashboard.
func (d Dashboard) SpecHash() string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		d.Folder, d.Name, strconv.Itoa(d.GnetID), strconv.Itoa(d.Revision), d.URL, d.Datasource,
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

// Source lists and watches containers via the Docker API.
type Source struct {
	cli    client.APIClient
	prefix string
	log    *slog.Logger
}

// New returns a Source using the given Docker client and label prefix.
func New(cli client.APIClient, prefix string, log *slog.Logger) *Source {
	return &Source{cli: cli, prefix: prefix, log: log}
}

// List returns every dashboard declared by a container (running or stopped),
// sorted by output filename for a stable desired state. Stopped containers keep
// their dashboards; only removing the container drops them.
func (s *Source) List(ctx context.Context) ([]Dashboard, error) {
	summaries, err := s.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	var out []Dashboard
	for _, c := range summaries {
		out = append(out, parseLabels(s.prefix, containerName(c), c.Labels, s.log)...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Filename() < out[j].Filename() })
	return out, nil
}

type builder struct {
	gnetID, revision, url, folder, datasource string
}

// parseLabels turns one container's labels into its declared dashboards. It is
// pure (no Docker types) so the label grammar is unit-testable in isolation.
func parseLabels(prefix, name string, labels map[string]string, log *slog.Logger) []Dashboard {
	prefixDot := prefix + "."
	builders := map[string]*builder{}
	var order []string
	get := func(n string) *builder {
		b, ok := builders[n]
		if !ok {
			b = &builder{}
			builders[n] = b
			order = append(order, n)
		}
		return b
	}

	for k, v := range labels {
		if !strings.HasPrefix(k, prefixDot) {
			continue
		}
		rest := k[len(prefixDot):]
		field, dbName := rest, name
		if i := strings.LastIndex(rest, "."); i >= 0 {
			field, dbName = rest[i+1:], rest[:i]
		}
		v = strings.TrimSpace(v)
		b := get(dbName)
		switch field {
		case "gnetId":
			b.gnetID = v
		case "revision":
			b.revision = v
		case "url":
			b.url = v
		case "folder":
			b.folder = v
		case "datasource":
			b.datasource = v
		default:
			log.Warn("ignoring unknown dashboard label field", "container", name, "label", k)
		}
	}

	var out []Dashboard
	for _, dbName := range order {
		d, err := builders[dbName].build(name, dbName)
		if err != nil {
			log.Warn("ignoring invalid dashboard label", "container", name, "dashboard", dbName, "error", err)
			continue
		}
		out = append(out, d)
	}
	return out
}

func (b *builder) build(containerName, name string) (Dashboard, error) {
	d := Dashboard{Container: containerName, Name: name, Folder: b.folder, URL: b.url, Datasource: b.datasource}
	if name == "" {
		return d, errors.New("empty dashboard name")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return d, fmt.Errorf("invalid dashboard name %q", name)
	}
	if strings.Contains(b.folder, "..") || strings.HasPrefix(b.folder, "/") || strings.Contains(b.folder, `\`) {
		return d, fmt.Errorf("invalid folder %q", b.folder)
	}
	if b.url != "" {
		if b.gnetID != "" || b.revision != "" {
			return d, errors.New("set either url or gnetId+revision, not both")
		}
		return d, nil
	}
	if b.gnetID == "" {
		return d, errors.New("needs gnetId+revision or url")
	}
	id, err := strconv.Atoi(b.gnetID)
	if err != nil || id <= 0 {
		return d, fmt.Errorf("invalid gnetId %q", b.gnetID)
	}
	rev, err := strconv.Atoi(b.revision)
	if err != nil || rev <= 0 {
		return d, fmt.Errorf("gnetId needs a valid revision, got %q", b.revision)
	}
	d.GnetID, d.Revision = id, rev
	return d, nil
}

func containerName(c container.Summary) string {
	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	if len(c.ID) >= 12 {
		return c.ID[:12]
	}
	return c.ID
}

// Watch emits a signal whenever a relevant container event occurs. It
// resubscribes automatically on stream errors and stops when ctx is cancelled.
func (s *Source) Watch(ctx context.Context, onRestart func()) <-chan struct{} {
	out := make(chan struct{}, 1)
	go func() {
		defer close(out)
		// Track container existence, not run state: create/destroy (and label
		// updates) change the desired set, but a merely stopped container keeps
		// its dashboard until it is removed (List uses All: true).
		f := filters.NewArgs(
			filters.Arg("type", "container"),
			filters.Arg("event", "create"),
			filters.Arg("event", "start"),
			filters.Arg("event", "destroy"),
			filters.Arg("event", "update"),
		)
		for ctx.Err() == nil {
			msgs, errs := s.cli.Events(ctx, events.ListOptions{Filters: f})
		stream:
			for {
				select {
				case <-ctx.Done():
					return
				case <-msgs:
					signal(out)
				case err := <-errs:
					if ctx.Err() == nil && err != nil {
						s.log.Warn("docker event stream interrupted, resubscribing", "error", err)
						if onRestart != nil {
							onRestart()
						}
						time.Sleep(time.Second)
					}
					break stream
				}
			}
		}
	}()
	return out
}

func signal(ch chan<- struct{}) {
	select {
	case ch <- struct{}{}:
	default: // a reconcile is already pending
	}
}
