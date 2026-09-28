package ingest_test

import (
	"context"
	"errors"
	"fmt"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/neptune"
	"gophergraph/ingest/ports"
	"gophergraph/internal/graphdata"
	"io"
	"reflect"
	"strings"
	"testing"
)

type faultCatalog struct {
	entries []ports.Entry
	listErr error
	open    func(string) (io.ReadCloser, error)
}

func (c faultCatalog) List(context.Context) ([]ports.Entry, error)             { return c.entries, c.listErr }
func (c faultCatalog) Open(_ context.Context, s string) (io.ReadCloser, error) { return c.open(s) }

type trackedReader struct {
	io.Reader
	close func() error
}

func (r trackedReader) Close() error { return r.close() }

type brokenTail struct {
	data string
	err  error
}

func (r *brokenTail) Read(p []byte) (int, error) {
	if r.data == "" {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, r.err
}
func TestBarriersAndFailures(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("source failure")
	opened, closed := 0, 0
	nodes := faultCatalog{entries: []ports.Entry{{Key: "a", Regular: true}, {Key: "b", Regular: true}, {Key: "c", Regular: true}, {Key: "folder"}}, open: func(key string) (io.ReadCloser, error) {
		if opened != closed {
			t.Fatal("previous source still open")
		}
		if key == "c" {
			return nil, boom
		}
		opened++
		var r io.Reader = strings.NewReader("~id\nB\n")
		if key == "a" {
			r = &brokenTail{data: "~id\nA\n\"tail", err: boom}
		}
		return trackedReader{r, func() error { closed++; return nil }}, nil
	}}
	edges := faultCatalog{entries: []ports.Entry{{Key: "e", Regular: true}}, open: func(string) (io.ReadCloser, error) {
		if closed != 2 {
			t.Fatal("edges opened before node barrier")
		}
		return io.NopCloser(strings.NewReader("~id,~from,~to\ne,A,B\n")), nil
	}}
	d := &diagnostics{}
	g, r, e := ingest.Build(ctx, nodes, edges, neptune.Decoder{}, d, ingest.Options{})
	if e != nil || g.Metadata().Nodes != 2 || g.Metadata().Edges != 1 || r.NodeSources.SourcesIOFailed != 2 || r.NodeSources.SourcesNonRegular != 1 || r.NodeSources.SourcesCompleted != 1 || r.NodeSources.RecordsSeen != 2 || opened != closed {
		t.Fatal(r, e)
	}
	for _, caseName := range []string{"catalog", "sink", "cancel"} {
		t.Run(caseName, func(t *testing.T) {
			var n ports.Catalog = memory{"n": "~id\nA\n"}
			diag := &diagnostics{}
			ctx := context.Background()
			switch caseName {
			case "catalog":
				n = faultCatalog{listErr: boom}
			case "sink":
				n = memory{"n": "bad\n"}
				diag.fail = true
			case "cancel":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			g, _, e := ingest.Build(ctx, n, memory{}, neptune.Decoder{}, diag, ingest.Options{})
			if g != nil || e == nil {
				t.Fatal("operational failure accepted", e)
			}
		})
	}
}
func TestPermutationAndIdempotence(t *testing.T) {
	rows := []string{"X,L,one,a", "X,M,one,b", "X,L,two,c"}
	var expected graphdata.Data
	for i := 0; i < 6; i++ {
		order := [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}[i]
		var b strings.Builder
		b.WriteString("~id,~label,p:String(single),s:String\n")
		for _, j := range order {
			fmt.Fprintln(&b, rows[j])
			fmt.Fprintln(&b, rows[j])
		}
		g, r, e := ingest.Build(context.Background(), memory{"n": b.String()}, memory{"e": "~id,~from,~to,~label\nz,X,X,A\nz,X,X,B\nz,X,X,A\nkeep,X,X,A\nkeep,X,missing,A\n"}, neptune.Decoder{}, &diagnostics{}, ingest.Options{})
		if e != nil || r.PropertyConflictGroups != 1 || r.QuarantinedEdgeIDs != 1 || r.EdgeSources.RecordsRejected != 1 {
			t.Fatal(r, e)
		}
		var data graphdata.Data
		if e = g.InternalColumns(&data); e != nil {
			t.Fatal(e)
		}
		if i == 0 {
			expected = data
		} else if !reflect.DeepEqual(expected, data) {
			t.Fatal("order dependent")
		}
	}
}
