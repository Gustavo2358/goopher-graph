package graphjson_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gophergraph/graph"
	"gophergraph/graphjson"
	"gophergraph/internal/testutil"
	"gophergraph/query"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"io"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type document struct {
	Query                     graphjson.Query
	Directed, PartialSnapshot bool
	Counts                    struct{ Nodes, Edges uint64 }
	Nodes                     []testutil.Node
	Edges                     []testutil.Edge
}

func open(t testing.TB, name string) *graph.Graph {
	t.Helper()
	g, err := snapshot.Open(context.Background(), mmap.New("../fixtures/snapshot_reference/"+name+".snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g
}

func encode(t testing.TB, g *graph.Graph, sub *query.Subgraph, q graphjson.Query) ([]byte, document) {
	t.Helper()
	var b bytes.Buffer
	if err := graphjson.Write(context.Background(), &b, g, sub, q); err != nil {
		t.Fatal(err)
	}
	var d document
	if err := json.Unmarshal(b.Bytes(), &d); err != nil {
		t.Fatal(err, b.String())
	}
	if !d.Directed || d.Counts.Nodes != uint64(len(d.Nodes)) || d.Counts.Edges != uint64(len(d.Edges)) || d.Nodes == nil || d.Edges == nil {
		t.Fatalf("invalid envelope: %+v", d)
	}
	return b.Bytes(), d
}

func checkProperties(t *testing.T, got, want []testutil.Property) {
	t.Helper()
	if got == nil || len(got) != len(want) {
		t.Fatalf("properties: %+v want %+v", got, want)
	}
	remaining := slices.Clone(want)
	for _, p := range got {
		// Long is always a decimal JSON string, including values within 2^53.
		if p.Type == "Long" {
			var value string
			if err := json.Unmarshal(p.Value, &value); err != nil {
				t.Fatal("Long must be a string", err)
			}
			p.Value = json.RawMessage(value)
		}
		found := slices.IndexFunc(remaining, func(w testutil.Property) bool {
			return p.Key == w.Key && p.Type == w.Type && testutil.Value(t, p).Equal(testutil.Value(t, w))
		})
		if found < 0 {
			t.Fatalf("unexpected property %+v; want %+v", p, want)
		}
		remaining = slices.Delete(remaining, found, found+1)
	}
}

func TestTopologyQueriesAgainstIndependentFixture(t *testing.T) {
	g := open(t, "topology")
	want := testutil.Read(t, "../fixtures/01_topology")
	for _, q := range want.Queries {
		t.Run(q.Kind+"/"+q.Start+"/"+q.End+"/"+strings.Join(q.Labels, ","), func(t *testing.T) {
			start, err := g.FindNode(q.Start)
			if err != nil {
				t.Fatal(err)
			}
			o := query.Options{}
			if q.Labels != nil {
				o.EdgeLabels = []graph.StringID{}
				for _, label := range q.Labels {
					id, ok, err := g.FindString(label)
					if err != nil || !ok {
						t.Fatal(label, err)
					}
					o.EdgeLabels = append(o.EdgeLabels, id)
				}
			}
			meta := graphjson.Query{Name: strings.ReplaceAll(q.Kind, "_", "-"), Node: &q.Start, EdgeLabels: q.Labels}
			var sub *query.Subgraph
			switch q.Kind {
			case "territory":
				sub, err = query.Territory(context.Background(), g, start, o)
			case "anti_territory":
				sub, err = query.AntiTerritory(context.Background(), g, start, o)
			case "between":
				end, e := g.FindNode(q.End)
				if e != nil {
					t.Fatal(e)
				}
				sub, err = query.Between(context.Background(), g, start, end, o)
				meta.Node, meta.From, meta.To = nil, &q.Start, &q.End
			}
			if err != nil {
				t.Fatal(err)
			}
			first, got := encode(t, g, sub, meta)
			second, _ := encode(t, g, sub, meta)
			if !bytes.Equal(first, second) || !reflect.DeepEqual(got.Query, meta) || got.PartialSnapshot {
				t.Fatal("metadata or determinism", string(first))
			}
			var nodes, edges []string
			for _, n := range got.Nodes {
				nodes = append(nodes, n.ID)
				i := slices.IndexFunc(want.Graph.Nodes, func(w testutil.Node) bool { return w.ID == n.ID })
				if i < 0 {
					t.Fatal(n)
				}
				w := want.Graph.Nodes[i]
				labels := slices.Clone(w.Labels)
				slices.Sort(labels)
				if !slices.Equal(n.Labels, labels) {
					t.Fatal(n.Labels, labels)
				}
				checkProperties(t, n.Properties, w.Properties)
			}
			for _, e := range got.Edges {
				edges = append(edges, e.ID)
				i := slices.IndexFunc(want.Graph.Edges, func(w testutil.Edge) bool { return w.ID == e.ID })
				if i < 0 {
					t.Fatal(e)
				}
				w := want.Graph.Edges[i]
				if e.Source != w.Source || e.Target != w.Target || e.Label != w.Label {
					t.Fatal(e, w)
				}
				checkProperties(t, e.Properties, w.Properties)
			}
			if !slices.Equal(nodes, q.Members) || !slices.Equal(edges, q.Edges) {
				t.Fatal(nodes, edges, q)
			}
		})
	}
}

func TestTypedValuesAndPartialFixtures(t *testing.T) {
	for _, tc := range []struct{ snapshot, fixture string }{
		{"typed", "03_typed_values"}, {"partial", "02_resilient"}, {"empty", "07_empty"}, {"presence", "11_csv_presence_crlf"}, {"numeric", "12_numeric_dialect"},
	} {
		t.Run(tc.snapshot, func(t *testing.T) {
			g := open(t, tc.snapshot)
			want := testutil.Read(t, "../fixtures/"+tc.fixture)
			sub, err := query.NewSubgraph(g)
			if err != nil {
				t.Fatal(err)
			}
			for i := uint64(0); i < g.Metadata().Nodes; i++ {
				if err := sub.AddNode(graph.NodeID(i)); err != nil {
					t.Fatal(err)
				}
			}
			for i := uint64(0); i < g.Metadata().Edges; i++ {
				if err := sub.AddEdge(graph.EdgeID(i)); err != nil {
					t.Fatal(err)
				}
			}
			_, got := encode(t, g, sub, graphjson.Query{Name: "all"})
			if got.PartialSnapshot != (want.Load.Completeness == "PARTIAL") || len(got.Nodes) != len(want.Graph.Nodes) || len(got.Edges) != len(want.Graph.Edges) {
				t.Fatal(got)
			}
			for i, n := range got.Nodes {
				if n.ID != want.Graph.Nodes[i].ID {
					t.Fatal(n.ID)
				}
				checkProperties(t, n.Properties, want.Graph.Nodes[i].Properties)
			}
			for i, e := range got.Edges {
				if e.ID != want.Graph.Edges[i].ID {
					t.Fatal(e.ID)
				}
				checkProperties(t, e.Properties, want.Graph.Edges[i].Properties)
			}
		})
	}
}

var errOutput = errors.New("output unavailable")

type failingWriter struct{ short bool }

func (w failingWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) / 2, nil
	}
	return 0, errOutput
}

func TestErrorsAndConcurrentDeterminism(t *testing.T) {
	g := open(t, "topology")
	sub, err := query.Territory(context.Background(), g, 0, query.Options{})
	if err != nil {
		t.Fatal(err)
	}
	q := graphjson.Query{Name: "territory", Node: new("A"), EdgeLabels: []string{"READS", "CALLS", "READS", "unknown"}}
	first, doc := encode(t, g, sub, q)
	if !slices.Equal(doc.Query.EdgeLabels, []string{"CALLS", "READS", "unknown"}) || !slices.Equal(q.EdgeLabels, []string{"READS", "CALLS", "READS", "unknown"}) {
		t.Fatal("filter normalization mutated caller")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			var b bytes.Buffer
			if err := graphjson.Write(context.Background(), &b, g, sub, q); err != nil || !bytes.Equal(b.Bytes(), first) {
				t.Error("concurrent write", err)
			}
		})
	}
	wg.Wait()
	for _, short := range []bool{false, true} {
		want := errOutput
		if short {
			want = io.ErrShortWrite
		}
		if err := graphjson.Write(context.Background(), failingWriter{short}, g, sub, q); !errors.Is(err, want) {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := graphjson.Write(ctx, io.Discard, g, sub, q); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	other := open(t, "empty")
	if err := graphjson.Write(context.Background(), io.Discard, other, sub, q); !errors.Is(err, graph.ErrGraphMismatch) {
		t.Fatal(err)
	}
	if err := graphjson.Write(context.Background(), io.Discard, g, nil, q); !errors.Is(err, graph.ErrGraphMismatch) {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	if err := graphjson.Write(context.Background(), io.Discard, g, sub, q); !errors.Is(err, graph.ErrClosed) {
		t.Fatal(err)
	}
}

func sameFloat(t *testing.T, raw json.RawMessage, value float64, bits int) {
	t.Helper()
	s := string(raw)
	if len(s) > 0 && s[0] == '"' {
		if err := json.Unmarshal(raw, &s); err != nil {
			t.Fatal(err)
		}
	}
	f, err := strconv.ParseFloat(s, bits)
	if err != nil || (math.IsNaN(value) != math.IsNaN(f)) || (!math.IsNaN(value) && math.Float64bits(f) != math.Float64bits(value)) {
		t.Fatalf("float %s != %v (%d bits): %v", raw, value, bits, err)
	}
}
