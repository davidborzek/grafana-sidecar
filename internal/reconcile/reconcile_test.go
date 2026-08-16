package reconcile

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/davidborzek/grafana-sidecar/internal/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLister struct{ ds []source.Dashboard }

func (f fakeLister) List(context.Context) ([]source.Dashboard, error) { return f.ds, nil }

type fakeFetcher struct {
	calls map[string]int
	fail  map[string]bool
}

func (f *fakeFetcher) Fetch(_ context.Context, d source.Dashboard) ([]byte, error) {
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[d.Filename()]++
	if f.fail[d.Filename()] {
		return nil, errors.New("boom")
	}
	return []byte("content:" + d.Filename() + "\n"), nil
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func exists(t *testing.T, p string) {
	t.Helper()
	_, err := os.Stat(p)
	assert.NoError(t, err, "expected %s to exist", p)
}

func notExists(t *testing.T, p string) {
	t.Helper()
	_, err := os.Stat(p)
	assert.True(t, os.IsNotExist(err), "expected %s to be gone", p)
}

func TestReconcile_WritesAndSkips(t *testing.T) {
	dir := t.TempDir()
	ff := &fakeFetcher{}
	rec := New(fakeLister{[]source.Dashboard{
		{Container: "c", Name: "c", Folder: "System", GnetID: 1, Revision: 1},
		{Container: "c", Name: "x", GnetID: 2, Revision: 1},
	}}, ff, dir, true, discard())

	res, err := rec.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, res.Files)
	assert.Equal(t, 2, res.Downloaded)
	assert.Equal(t, 1, res.Containers)
	assert.True(t, res.Changed)
	exists(t, filepath.Join(dir, "System", "c.json"))
	exists(t, filepath.Join(dir, "c.x.json"))

	// unchanged specs -> reused from disk, no downloads, no change
	res2, err := rec.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, res2.Downloaded)
	assert.False(t, res2.Changed)
	assert.Equal(t, 1, ff.calls["System/c.json"])
	assert.Equal(t, 1, ff.calls["c.x.json"])
}

func TestReconcile_GC(t *testing.T) {
	dir := t.TempDir()
	ff := &fakeFetcher{}
	_, err := New(fakeLister{[]source.Dashboard{
		{Container: "c", Name: "c", Folder: "F", GnetID: 1, Revision: 1},
		{Container: "c", Name: "x", Folder: "F", GnetID: 2, Revision: 1},
	}}, ff, dir, true, discard()).Run(context.Background())
	require.NoError(t, err)
	exists(t, filepath.Join(dir, "F", "c.json"))
	exists(t, filepath.Join(dir, "F", "c.x.json"))

	// only one dashboard remains -> the other file (and now-empty dir) is pruned
	res, err := New(fakeLister{[]source.Dashboard{
		{Container: "c", Name: "c", GnetID: 1, Revision: 1},
	}}, ff, dir, true, discard()).Run(context.Background())
	require.NoError(t, err)
	assert.True(t, res.Changed)
	exists(t, filepath.Join(dir, "c.json"))
	notExists(t, filepath.Join(dir, "F", "c.x.json"))
	notExists(t, filepath.Join(dir, "F"))
}

func TestReconcile_KeepOnFailure(t *testing.T) {
	dir := t.TempDir()
	ff := &fakeFetcher{}
	d := source.Dashboard{Container: "c", Name: "c", GnetID: 1, Revision: 1}
	_, err := New(fakeLister{[]source.Dashboard{d}}, ff, dir, true, discard()).Run(context.Background())
	require.NoError(t, err)
	exists(t, filepath.Join(dir, "c.json"))

	// spec changes (new revision) but the download fails: the last-good file
	// must be kept, and the failure counted.
	d2 := d
	d2.Revision = 2
	ff.fail = map[string]bool{d2.Filename(): true}
	res, err := New(fakeLister{[]source.Dashboard{d2}}, ff, dir, true, discard()).Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, res.Failed)
	exists(t, filepath.Join(dir, "c.json"))

	b, err := os.ReadFile(filepath.Join(dir, "c.json"))
	require.NoError(t, err)
	assert.Equal(t, "content:c.json\n", string(b))
}

func TestReconcile_NoPruneKeepsOrphans(t *testing.T) {
	dir := t.TempDir()
	ff := &fakeFetcher{}
	_, err := New(fakeLister{[]source.Dashboard{
		{Container: "c", Name: "c", GnetID: 1, Revision: 1},
		{Container: "c", Name: "x", GnetID: 2, Revision: 1},
	}}, ff, dir, false, discard()).Run(context.Background())
	require.NoError(t, err)
	exists(t, filepath.Join(dir, "c.json"))
	exists(t, filepath.Join(dir, "c.x.json"))

	// one dashboard disappears; with prune disabled its file is kept
	res, err := New(fakeLister{[]source.Dashboard{
		{Container: "c", Name: "c", GnetID: 1, Revision: 1},
	}}, ff, dir, false, discard()).Run(context.Background())
	require.NoError(t, err)
	assert.False(t, res.Changed)
	exists(t, filepath.Join(dir, "c.json"))
	exists(t, filepath.Join(dir, "c.x.json")) // orphan kept
}
