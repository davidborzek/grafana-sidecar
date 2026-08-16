package source

import (
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestParseLabels_ShorthandURL(t *testing.T) {
	got := parseLabels("grafana.dashboard", "grafana", map[string]string{
		"grafana.dashboard.url":    "https://example.test/d.json",
		"grafana.dashboard.folder": "Observability",
		"unrelated":                "x",
	}, testLogger())

	require.Len(t, got, 1)
	d := got[0]
	assert.Equal(t, "grafana", d.Container)
	assert.Equal(t, "grafana", d.Name)
	assert.Equal(t, "Observability", d.Folder)
	assert.Equal(t, "https://example.test/d.json", d.URL)
	assert.Equal(t, "Observability/grafana.json", d.Filename())
}

func TestParseLabels_ShorthandGnet(t *testing.T) {
	got := parseLabels("grafana.dashboard", "node", map[string]string{
		"grafana.dashboard.gnetId":     "1860",
		"grafana.dashboard.revision":   "45",
		"grafana.dashboard.datasource": "Prometheus",
	}, testLogger())

	require.Len(t, got, 1)
	assert.Equal(t, 1860, got[0].GnetID)
	assert.Equal(t, 45, got[0].Revision)
	assert.Equal(t, "Prometheus", got[0].Datasource)
	assert.Equal(t, "node.json", got[0].Filename())
}

func TestParseLabels_NamedMultiple(t *testing.T) {
	got := parseLabels("grafana.dashboard", "app", map[string]string{
		"grafana.dashboard.a.url":      "https://x.test/a.json",
		"grafana.dashboard.b.gnetId":   "10",
		"grafana.dashboard.b.revision": "2",
		"grafana.dashboard.b.folder":   "Apps",
	}, testLogger())

	require.Len(t, got, 2)
	byName := map[string]Dashboard{}
	for _, d := range got {
		byName[d.Name] = d
	}
	assert.Equal(t, "app.a.json", byName["a"].Filename())
	assert.Equal(t, "https://x.test/a.json", byName["a"].URL)
	assert.Equal(t, 10, byName["b"].GnetID)
	assert.Equal(t, "Apps/app.b.json", byName["b"].Filename())
}

func TestParseLabels_Invalid(t *testing.T) {
	// gnetId without revision -> dropped
	assert.Empty(t, parseLabels("grafana.dashboard", "c", map[string]string{
		"grafana.dashboard.gnetId": "1860",
	}, testLogger()))

	// url and gnetId together -> dropped
	assert.Empty(t, parseLabels("grafana.dashboard", "c", map[string]string{
		"grafana.dashboard.url":      "https://x.test",
		"grafana.dashboard.gnetId":   "1",
		"grafana.dashboard.revision": "2",
	}, testLogger()))

	// path traversal in name -> dropped
	assert.Empty(t, parseLabels("grafana.dashboard", "c", map[string]string{
		"grafana.dashboard.../evil.url": "https://x.test",
	}, testLogger()))
}

func TestSpecHash(t *testing.T) {
	a := Dashboard{Container: "c", Name: "c", GnetID: 1, Revision: 1}
	same := Dashboard{Container: "c", Name: "c", GnetID: 1, Revision: 1}
	bumped := Dashboard{Container: "c", Name: "c", GnetID: 1, Revision: 2}

	assert.Equal(t, a.SpecHash(), same.SpecHash())
	assert.NotEqual(t, a.SpecHash(), bumped.SpecHash())
}
