package wasmquery

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gophergraph/graph"
	"gophergraph/internal/testutil"
	"gophergraph/snapshot"
	snapshotfile "gophergraph/snapshot/adapters/file"
	"gophergraph/snapshot/adapters/mmap"
	"gophergraph/wasmquery/internal/abi"
)

func TestComposedWasmSelection(t *testing.T) {
	r := newRuntime(t, Limits{})
	m := compileGuest(t, r, "./testdata/guest")
	for _, indexed := range []bool{false, true} {
		for _, path := range []string{"testdata/selection", "../fixtures/03_typed_values"} {
			if _, err := os.Stat(path); err != nil {
				t.Fatal(err)
			}
			expected := testutil.Read(t, path)
			var keys []string
			for _, node := range expected.Graph.Nodes {
				for _, p := range node.Properties {
					keys = append(keys, p.Key)
				}
			}
			g := loadGraph(t, path, indexed, keys...)
			for _, node := range expected.Graph.Nodes {
				for _, p := range node.Properties {
					value := testutil.Value(t, p)
					labels := append([]string{"missing"}, node.Labels...)
					labels = append(labels, node.Labels...)
					for _, ls := range [][]string{labels, nil, {}} {
						labelJSON, _ := json.Marshal(ls)
						valueJSON, _ := json.Marshal(encodeValue(value))
						args := []string{"selection", "composed", string(labelJSON), p.Key, string(valueJSON)}
						actual := execute(t, r, m, g, args...)
						a, _ := actual.Subgraph()
						args[1] = "previous"
						previous := execute(t, r, m, g, args...)
						b, _ := previous.Subgraph()
						sameSubgraph(t, g, a, b)
						// Independent expected membership, including multivalued properties.
						var want []string
						for _, candidate := range expected.Graph.Nodes {
							labelMatch := false
							for _, label := range ls {
								for _, l := range candidate.Labels {
									if l == label {
										labelMatch = true
									}
								}
							}
							if labelMatch {
								for _, prop := range candidate.Properties {
									if prop.Key == p.Key && testutil.Value(t, prop).Equal(value) {
										want = append(want, candidate.ID)
										break
									}
								}
							}
						}
						var got []string
						it := a.Nodes()
						for it.Next() {
							id, err := g.NodeExternalID(it.ID())
							if err != nil {
								t.Fatal(err)
							}
							got = append(got, id)
						}
						if !reflect.DeepEqual(got, want) {
							t.Fatalf("%s %v %v: got=%v want=%v", path, ls, p, got, want)
						}
						actual.Close()
						previous.Close()
					}
				}
			}
			assertClean(t, r)
		}
	}
}

func TestSelectionHostBudgetsAndOwnership(t *testing.T) {
	g := loadGraph(t, "testdata/selection", false, "tag", "rank")
	for _, indexed := range []bool{false, true} {
		g = loadGraph(t, "testdata/selection", indexed, "tag", "rank")
		for _, limit := range []uint64{15, 16} {
			r := newRuntime(t, Limits{HostBytes: limit})
			s := &execution{r: r, g: g, handles: make(map[uint64]object)}
			p := abi.Params{Labels: []string{"L"}, Key: "tag", Value: abi.Value{Kind: 8, Text: "X"}}
			token, err := s.dispatch(context.Background(), abi.NodesAnyLabelProperty, 0, 0, p)
			if limit == 15 {
				if !errors.Is(err, ErrLimit) || token != 0 || len(s.handles) != 0 || s.used != 0 {
					t.Fatal(token, err, s)
				}
			} else {
				if err != nil || s.used != 8 || s.metrics.PeakHostBytes != 16 {
					t.Fatal(token, err, s.metrics)
				}
				// The input + Has output fits exactly; a redundant temporary does not.
				input, _ := s.get(token)
				before := input.nodes.Clone()
				filtered, err := s.dispatch(context.Background(), abi.Property, token, 0, p)
				if err != nil || s.used != 16 || input.nodes.Count() != before.Count() {
					t.Fatal(filtered, err, s.metrics)
				}
				out, _ := s.get(filtered)
				if out.nodes == input.nodes || !out.nodes.BelongsTo(g) {
					t.Fatal("alias/identity")
				}
				if err := s.release(token); err != nil {
					t.Fatal(err)
				}
				if out.nodes.Count() == 0 {
					t.Fatal("release affected output")
				}
			}
			s.clear()
			assertClean(t, r)
		}
	}
	// Zero nodes still consume a handle; handle limits are unchanged.
	r := newRuntime(t, Limits{Handles: 1})
	s := &execution{r: r, g: g, handles: make(map[uint64]object)}
	p := abi.Params{Value: abi.Value{Kind: 8, Text: "X"}}
	if _, err := s.dispatch(context.Background(), abi.NodesAnyLabelProperty, 0, 0, p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.dispatch(context.Background(), abi.NodesAnyLabelProperty, 0, 0, p); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	s.clear()
}

func TestPackagedFilteredV1AndV2(t *testing.T) {
	r := newRuntime(t, Limits{})
	compressed, err := os.ReadFile("testdata/filtered_v1.wasm.gz")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	oldBytes, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	old, err := r.Compile(context.Background(), oldBytes)
	if err != nil {
		t.Fatal(err)
	}
	updated := compileGuest(t, r, "../examples/wasm/filtered")
	for _, indexed := range []bool{false, true} {
		g := loadGraph(t, "testdata/selection", indexed, "tag", "rank")
		a := execute(t, r, old, g, "S", "L", "tag", "X")
		left, _ := a.Subgraph()
		b := execute(t, r, updated, g, "S", "L", "tag", "X")
		right, _ := b.Subgraph()
		sameSubgraph(t, g, left, right)
	}
}

func TestSelectionHostInvalidValueAndCancellation(t *testing.T) {
	g := loadGraph(t, "testdata/selection", false, "tag")
	r := newRuntime(t, Limits{})
	s := &execution{r: r, g: g, handles: make(map[uint64]object)}
	for _, value := range []abi.Value{{}, {Kind: 8, Bits: 1, Text: "X"}, {Kind: 4, Text: "X"}} {
		token, err := s.dispatch(context.Background(), abi.NodesAnyLabelProperty, 0, 0, abi.Params{Labels: []string{"L"}, Key: "tag", Value: value})
		if token != 0 || !errors.Is(err, graph.ErrInvalidValue) || s.used != 0 || len(s.handles) != 0 {
			t.Fatal(token, err, s)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	token, err := s.dispatch(ctx, abi.NodesAnyLabelProperty, 0, 0, abi.Params{Labels: []string{"L"}, Key: "tag", Value: abi.Value{Kind: 8, Text: "X"}})
	if token != 0 || !errors.Is(err, context.Canceled) || len(s.handles) != 0 {
		t.Fatal(token, err, s)
	}
	assertClean(t, r)
}

func TestSelectLabelsExample(t *testing.T) {
	r := newRuntime(t, Limits{})
	m := compileGuest(t, r, "../examples/wasm/select_labels")
	for _, indexed := range []bool{false, true} {
		g := loadGraph(t, "testdata/selection", indexed, "tag")
		for _, tc := range []struct{ labels, want []string }{
			{nil, nil},
			{[]string{"unknown"}, nil},
			{[]string{"L"}, []string{"A"}},
			{[]string{"L", "M", "M", "unknown"}, []string{"A", "C"}},
		} {
			result := execute(t, r, m, g, append([]string{"tag", "X"}, tc.labels...)...)
			sub, err := result.Subgraph()
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			it := sub.Nodes()
			for it.Next() {
				id, err := g.NodeExternalID(it.ID())
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, id)
			}
			if !reflect.DeepEqual(got, tc.want) || sub.EdgeCount() != 0 {
				t.Fatal(got, tc.want, sub.EdgeCount())
			}
			result.Close()
		}
		assertClean(t, r)
	}
}

// With mapped strings, decoding each property value allocates a string. Vary
// label frequency while keeping nodes, properties and the single neighbor
// fixed: neighbor-restricted Has should do the same property work in both.
func TestFilteredPropertyWorkStaysWithNeighbors(t *testing.T) {
	r := newRuntime(t, Limits{})
	m := compileGuest(t, r, "../examples/wasm/filtered")
	const n = 5000
	graphs := make([]*graph.Graph, 2)
	for variant := range graphs {
		root := t.TempDir()
		for _, role := range []string{"nodes", "edges"} {
			if err := os.Mkdir(filepath.Join(root, role), 0700); err != nil {
				t.Fatal(err)
			}
		}
		var csv bytes.Buffer
		fmt.Fprintln(&csv, "~id,~label,tag:String")
		for i := 0; i < n; i++ {
			label := "OTHER"
			if variant == 1 || i == 0 {
				label = "L"
			}
			fmt.Fprintf(&csv, "n%09d,%s,chosen-value\n", i, label)
		}
		fmt.Fprintln(&csv, "S,OTHER,")
		if err := os.WriteFile(filepath.Join(root, "nodes", "data.csv"), csv.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "edges", "data.csv"), []byte("~id,~from,~to,~label\nedge,S,n000000000,LINK\n"), 0600); err != nil {
			t.Fatal(err)
		}
		heap := loadGraph(t, root, false, "tag")
		path := filepath.Join(root, "graph.snapshot")
		if _, err := snapshot.Write(context.Background(), heap, snapshotfile.New(path)); err != nil {
			t.Fatal(err)
		}
		heap.Close()
		mapped, err := snapshot.Open(context.Background(), mmap.New(path))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { mapped.Close() })
		graphs[variant] = mapped
	}
	var allocs [2]float64
	for i, g := range graphs {
		allocs[i] = testing.AllocsPerRun(3, func() {
			result, err := r.Execute(context.Background(), m, g, []string{"S", "L", "tag", "chosen-value"})
			if err != nil {
				t.Fatal(err)
			}
			sub, err := result.Subgraph()
			if err != nil || sub.NodeCount() != 2 || sub.EdgeCount() != 1 {
				t.Fatal(sub, err)
			}
			result.Close()
		})
	}
	t.Logf("allocations/execution: rare label=%.0f, common label=%.0f", allocs[0], allocs[1])
	// Allow incidental runtime bookkeeping, but not 4999 unrelated property
	// decodes. This is a work regression, not a wall-clock performance assertion.
	if allocs[1] > allocs[0]+128 {
		t.Fatalf("property work grew with unrelated label members: %.0f -> %.0f", allocs[0], allocs[1])
	}
	assertClean(t, r)
}
