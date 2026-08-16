// Package fetch downloads remote Grafana dashboards and renders them for
// provisioning: it resolves ${DS_*} datasource inputs and marks the dashboard
// read-only (editable:false), since provisioned dashboards are git-managed.
package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/davidborzek/grafana-sidecar/internal/source"
)

// maxDownloadBytes caps a single dashboard download to a sane size.
const maxDownloadBytes = 16 << 20 // 16 MiB

var (
	// sentinel matches Grafana's built-in datasources ("-- Grafana --",
	// "-- Mixed --", ...); lines referencing them are never substituted.
	sentinel = regexp.MustCompile(`-- .* --`)
	// dsInput matches a templated datasource input placeholder, e.g. ${DS_PROMETHEUS}.
	dsInput = regexp.MustCompile(`\$\{DS_[A-Za-z0-9_]*\}`)
)

// Fetcher downloads and renders dashboards over HTTP.
type Fetcher struct {
	cli     *http.Client
	gnetURL string
}

// New returns a Fetcher. gnetBase is the grafana.com dashboards API base used
// to resolve gnetId downloads; timeout bounds each request.
func New(gnetBase string, timeout time.Duration) *Fetcher {
	return &Fetcher{cli: &http.Client{Timeout: timeout}, gnetURL: strings.TrimRight(gnetBase, "/")}
}

// Fetch downloads the dashboard described by d and returns the rendered JSON.
func (f *Fetcher) Fetch(ctx context.Context, d source.Dashboard) ([]byte, error) {
	url := d.URL
	if url == "" {
		url = fmt.Sprintf("%s/%d/revisions/%d/download", f.gnetURL, d.GnetID, d.Revision)
	}
	raw, err := f.download(ctx, url)
	if err != nil {
		return nil, err
	}
	return render(substitute(raw, d.Datasource))
}

func (f *Fetcher) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GET %s: %s: %s", url, resp.Status, strings.TrimSpace(string(msg)))
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxDownloadBytes {
		return nil, fmt.Errorf("dashboard from %s exceeds %d bytes", url, maxDownloadBytes)
	}
	return b, nil
}

// substitute replaces every ${DS_*} input with ds, skipping lines that
// reference a Grafana built-in datasource sentinel. A blank ds is a no-op.
func substitute(b []byte, ds string) []byte {
	if ds == "" {
		return b
	}
	lines := bytes.Split(b, []byte("\n"))
	repl := []byte(ds)
	for i, ln := range lines {
		if sentinel.Match(ln) {
			continue
		}
		lines[i] = dsInput.ReplaceAll(ln, repl)
	}
	return bytes.Join(lines, []byte("\n"))
}

// render validates the dashboard JSON, forces editable:false and re-encodes it
// deterministically (Go sorts map keys) so an unchanged dashboard produces
// byte-identical output across runs — which keeps the reconcile a no-op.
func render(b []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("invalid dashboard json: %w", err)
	}
	doc["editable"] = false
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
