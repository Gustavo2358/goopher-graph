package ingest_test

import (
	"context"
	"errors"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/neptune"
	"gophergraph/ingest/ports"
	"gophergraph/internal/testutil"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memory map[string]string

func (m memory) List(context.Context) ([]ports.Entry, error) {
	var out []ports.Entry
	for key := range m {
		out = append(out, ports.Entry{Key: key, Regular: true})
	}
	return out, nil
}
func (m memory) Open(_ context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(m[key])), nil
}

type diagnostics struct {
	events []ports.Diagnostic
	fail   bool
}

func (d *diagnostics) Emit(_ context.Context, e ports.Diagnostic) error {
	if d.fail {
		return errors.New("sink unavailable")
	}
	d.events = append(d.events, e)
	return nil
}
func catalog(t *testing.T, dir string) memory {
	t.Helper()
	entries, e := os.ReadDir(dir)
	if e != nil {
		t.Fatal(e)
	}
	m := memory{}
	for _, entry := range entries {
		b, e := os.ReadFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			t.Fatal(e)
		}
		m[entry.Name()] = string(b)
	}
	return m
}
func TestAllFixtures(t *testing.T) {
	paths, e := filepath.Glob("../fixtures/*/expected.json")
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range paths {
		dir := filepath.Dir(p)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			w := testutil.Read(t, dir)
			diag := &diagnostics{}
			g, r, e := ingest.Build(context.Background(), catalog(t, filepath.Join(dir, "nodes")), catalog(t, filepath.Join(dir, "edges")), neptune.Decoder{}, diag, ingest.Options{IndexProperties: w.IndexProperties})
			if e != nil {
				t.Fatal(e)
			}
			defer g.Close()
			testutil.CheckGraph(t, g, w)
			testutil.CheckQueries(t, g, w)
			counts := map[string]int{}
			for _, d := range diag.events {
				counts[d.Code]++
			}
			for code, n := range w.Load.Diagnostics {
				if counts[code] != n {
					t.Fatalf("diagnostic %s=%d want %d", code, counts[code], n)
				}
			}
			if (r.Completeness == ingest.Partial) != (w.Load.Completeness == "PARTIAL") {
				t.Fatal(r)
			}
			for _, cr := range []ingest.CatalogReport{r.NodeSources, r.EdgeSources} {
				if cr.SourcesSeen != cr.SourcesCompleted+cr.SourcesRejectedHeader+cr.SourcesIOFailed+cr.SourcesInterrupted+cr.SourcesNonRegular || cr.RecordsSeen != cr.RecordsRejected+cr.RecordsStaged {
					t.Fatal("disjoint counters", cr)
				}
			}
			for k, n := range w.Load.Exact {
				values := map[string]uint64{"nodes.records_seen": r.NodeSources.RecordsSeen, "nodes.records_rejected": r.NodeSources.RecordsRejected, "nodes.records_staged": r.NodeSources.RecordsStaged, "edges.records_seen": r.EdgeSources.RecordsSeen, "edges.records_rejected": r.EdgeSources.RecordsRejected, "edges.records_staged": r.EdgeSources.RecordsStaged, "properties_dropped": r.PropertyConflictGroups + r.PropertyCardinalityConflictGroups, "edges_quarantined": r.QuarantinedEdgeIDs}
				v, ok := values[k]
				if !ok || v != n {
					t.Fatal(k, v, n)
				}
			}
		})
	}
}
