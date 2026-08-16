// Package reconcile turns the set of dashboard-carrying containers into
// provisioned dashboard files on disk and keeps the directory in sync.
package reconcile

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/davidborzek/grafana-sidecar/internal/source"
)

// stateFile records each managed file's spec hash so the reconciler can skip
// re-downloading unchanged dashboards. It deliberately has no .json extension,
// so Grafana ignores it and it is never treated as a managed dashboard.
const stateFile = ".grafana-sidecar-state"

type lister interface {
	List(ctx context.Context) ([]source.Dashboard, error)
}

type fetcher interface {
	Fetch(ctx context.Context, d source.Dashboard) ([]byte, error)
}

// Result summarizes one reconcile run.
type Result struct {
	Containers int  // containers declaring dashboards
	Dashboards int  // dashboards declared
	Downloaded int  // dashboards (re)downloaded this run
	Failed     int  // downloads that failed
	Files      int  // dashboard files present (desired)
	Changed    bool // on-disk state changed
}

// Reconciler owns the dashboards directory and keeps it in sync with the
// dashboards declared by running containers.
type Reconciler struct {
	src   lister
	fetch fetcher
	dir   string
	prune bool
	log   *slog.Logger
}

// New returns a Reconciler writing into dir. When prune is false, dashboards
// for containers that disappear (or drop their labels) are kept rather than
// deleted — the sidecar then only ever adds or updates files.
func New(src lister, f fetcher, dir string, prune bool, log *slog.Logger) *Reconciler {
	return &Reconciler{src: src, fetch: f, dir: dir, prune: prune, log: log}
}

// Run performs one full reconcile: fetch every declared dashboard (skipping
// unchanged ones), then make the directory contain exactly the desired files.
func (r *Reconciler) Run(ctx context.Context) (Result, error) {
	var res Result

	dashboards, err := r.src.List(ctx)
	if err != nil {
		return res, err
	}
	res.Dashboards = len(dashboards)
	res.Containers = countContainers(dashboards)

	state := loadState(r.dir)
	newState := make(map[string]string, len(dashboards))
	desired := make(map[string][]byte, len(dashboards))

	for _, d := range dashboards {
		rel := d.Filename()
		hash := d.SpecHash()
		existing, existErr := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(rel)))

		// Unchanged spec and the file is still there: reuse it, no download.
		if existErr == nil && state[rel] == hash {
			desired[rel] = existing
			newState[rel] = hash
			continue
		}

		content, err := r.fetch.Fetch(ctx, d)
		if err != nil {
			res.Failed++
			r.log.Warn("dashboard download failed", "container", d.Container, "file", rel, "error", err)
			// Keep the last-good file (if any) so a transient failure never
			// wipes a working dashboard; omit it from newState so we retry.
			if existErr == nil {
				desired[rel] = existing
			}
			continue
		}
		res.Downloaded++
		desired[rel] = content
		newState[rel] = hash
	}
	res.Files = len(desired)

	changed, err := r.sync(desired)
	if err != nil {
		return res, err
	}
	res.Changed = changed

	if err := saveState(r.dir, newState); err != nil {
		r.log.Warn("could not persist state", "error", err)
	}
	return res, nil
}

// sync makes r.dir hold the desired files. When pruning is enabled (the
// default) the sidecar owns the directory, so any *.json not in the desired set
// is removed; otherwise stale files are left in place. Returns whether anything
// on disk changed.
func (r *Reconciler) sync(desired map[string][]byte) (bool, error) {
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return false, err
	}
	changed := false

	rels := make([]string, 0, len(desired))
	for rel := range desired {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	for _, rel := range rels {
		p := filepath.Join(r.dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return changed, err
		}
		if old, err := os.ReadFile(p); err == nil && bytes.Equal(old, desired[rel]) {
			continue
		}
		if err := writeFile(p, desired[rel]); err != nil {
			return changed, err
		}
		changed = true
	}

	if r.prune {
		stale, err := r.staleFiles(desired)
		if err != nil {
			return changed, err
		}
		for _, p := range stale {
			if err := os.Remove(p); err != nil {
				return changed, err
			}
			changed = true
		}
		if err := pruneEmptyDirs(r.dir); err != nil {
			r.log.Warn("could not prune empty directories", "error", err)
		}
	}
	return changed, nil
}

// staleFiles returns managed *.json files no longer in the desired set.
func (r *Reconciler) staleFiles(desired map[string][]byte) ([]string, error) {
	var stale []string
	err := filepath.WalkDir(r.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(p) != ".json" {
			return nil
		}
		rel, err := filepath.Rel(r.dir, p)
		if err != nil {
			return err
		}
		if _, keep := desired[filepath.ToSlash(rel)]; !keep {
			stale = append(stale, p)
		}
		return nil
	})
	return stale, err
}

// pruneEmptyDirs removes empty subdirectories (deepest first), leaving root.
func pruneEmptyDirs(root string) error {
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != root {
			dirs = append(dirs, p)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(d)
		}
	}
	return nil
}

func writeFile(path string, content []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func countContainers(ds []source.Dashboard) int {
	seen := make(map[string]struct{})
	for _, d := range ds {
		seen[d.Container] = struct{}{}
	}
	return len(seen)
}

func loadState(dir string) map[string]string {
	m := map[string]string{}
	b, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		return m
	}
	_ = json.Unmarshal(b, &m)
	return m
}

func saveState(dir string, m map[string]string) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(dir, stateFile), b)
}
