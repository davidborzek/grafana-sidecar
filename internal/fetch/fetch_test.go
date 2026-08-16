package fetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/davidborzek/grafana-sidecar/internal/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubstitute(t *testing.T) {
	in := []byte("{\n\"a\": \"${DS_PROMETHEUS}\",\n\"b\": \"-- Grafana -- ${DS_X}\"\n}")
	out := string(substitute(in, "Prometheus"))

	assert.Contains(t, out, `"a": "Prometheus"`)     // plain input replaced
	assert.Contains(t, out, "-- Grafana -- ${DS_X}") // sentinel line untouched
	assert.NotContains(t, out, "${DS_PROMETHEUS}")

	// no datasource configured -> no-op
	assert.Equal(t, string(in), string(substitute(in, "")))
}

func TestRender(t *testing.T) {
	out, err := render([]byte(`{"title":"x","editable":true}`))
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(out, &doc))
	assert.Equal(t, false, doc["editable"])

	// deterministic regardless of input key order
	out2, err := render([]byte(`{"editable":true,"title":"x"}`))
	require.NoError(t, err)
	assert.Equal(t, string(out), string(out2))

	_, err = render([]byte(`not json`))
	assert.Error(t, err)
}

func TestFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/direct.json":
			_, _ = w.Write([]byte(`{"title":"direct","editable":true}`))
		case "/1860/revisions/45/download":
			_, _ = w.Write([]byte("{\n\"panels\": [{\"datasource\": \"${DS_PROMETHEUS}\"}],\n\"editable\": true\n}"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	f := New(srv.URL, 5*time.Second)

	b, err := f.Fetch(context.Background(), source.Dashboard{URL: srv.URL + "/direct.json"})
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(b, &doc))
	assert.Equal(t, false, doc["editable"])
	assert.Equal(t, "direct", doc["title"])

	b, err = f.Fetch(context.Background(), source.Dashboard{GnetID: 1860, Revision: 45, Datasource: "Prometheus"})
	require.NoError(t, err)
	assert.Contains(t, string(b), `"Prometheus"`)
	assert.NotContains(t, string(b), "DS_PROMETHEUS")

	_, err = f.Fetch(context.Background(), source.Dashboard{URL: srv.URL + "/missing"})
	assert.Error(t, err)
}
