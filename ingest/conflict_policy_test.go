package ingest_test

import (
	"context"
	"errors"
	"fmt"
	"gophergraph/graph"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/filesystem"
	"gophergraph/ingest/adapters/neptune"
	"gophergraph/ingest/ports"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestNodePropertyDefaultLastWins(t *testing.T) {
	diag := &diagnostics{}
	g, report, err := ingest.Build(context.Background(), memory{"n": "~id,sigla:String(single)\nA,ZZ\nA,AA\n"}, memory{}, neptune.Decoder{}, diag, ingest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	v, _ := graph.TextValue(graph.StringKind, "AA")
	nodes, err := g.NodesWithProperty(context.Background(), "sigla", v)
	if err != nil || nodes.Count() != 1 || report.Completeness != ingest.Complete {
		t.Fatalf("last contribution must win without rejection: nodes=%v report=%+v err=%v", nodes, report, err)
	}
}

func textValue(t *testing.T, text string) graph.Value {
	t.Helper()
	v, err := graph.TextValue(graph.StringKind, text)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func nodeValues(t *testing.T, g *graph.Graph, key string) []graph.Value {
	t.Helper()
	id, err := g.FindNode("A")
	if err != nil {
		t.Fatal(err)
	}
	it, err := g.NodeProperties(id)
	if err != nil {
		t.Fatal(err)
	}
	var values []graph.Value
	for it.Next() {
		p := it.Property()
		name, err := g.String(p.Key)
		if err != nil {
			t.Fatal(err)
		}
		if name == key {
			values = append(values, p.Value)
		}
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}

func TestNodePropertyConflictPolicies(t *testing.T) {
	zz, aa, empty := textValue(t, "ZZ"), textValue(t, "AA"), textValue(t, "")
	intOne, _ := graph.IntegerValue(graph.IntKind, 1)
	longOne, _ := graph.IntegerValue(graph.LongKind, 1)
	cases := []struct {
		name              string
		nodes             memory
		first, last       []graph.Value
		conflict          bool
		cardinality       bool
		rejected          uint64
		contributions     uint64
		firstLoc, lastLoc ports.Location
	}{
		{name: "input order differs from value order", nodes: memory{"n": "~id,p:String(single)\nA,ZZ\nA,AA\n"}, first: []graph.Value{zz}, last: []graph.Value{aa}, conflict: true, contributions: 2, firstLoc: ports.Location{Source: "n", Record: 2, Line: 2}, lastLoc: ports.Location{Source: "n", Record: 3, Line: 3}},
		{name: "reversed input changes winner", nodes: memory{"n": "~id,p:String(single)\nA,AA\nA,ZZ\n"}, first: []graph.Value{aa}, last: []graph.Value{zz}, conflict: true, contributions: 2, firstLoc: ports.Location{Source: "n", Record: 2, Line: 2}, lastLoc: ports.Location{Source: "n", Record: 3, Line: 3}},
		{name: "duplicate value revisited", nodes: memory{"n": "~id,p:String(single)\nA,ZZ\nA,AA\nA,ZZ\n"}, first: []graph.Value{zz}, last: []graph.Value{zz}, conflict: true, contributions: 3, firstLoc: ports.Location{Source: "n", Record: 2, Line: 2}, lastLoc: ports.Location{Source: "n", Record: 4, Line: 4}},
		{name: "identical duplicates", nodes: memory{"n": "~id,p:String(single)\nA,ZZ\nA,ZZ\n"}, first: []graph.Value{zz}, last: []graph.Value{zz}},
		{name: "missing property retains value", nodes: memory{"n": "~id,p:String(single)\nA,ZZ\nA,\n"}, first: []graph.Value{zz}, last: []graph.Value{zz}},
		{name: "explicit empty is a value", nodes: memory{"n": "~id,p:String(single)\nA,ZZ\nA,\"\"\n"}, first: []graph.Value{zz}, last: []graph.Value{empty}, conflict: true, contributions: 2, firstLoc: ports.Location{Source: "n", Record: 2, Line: 2}, lastLoc: ports.Location{Source: "n", Record: 3, Line: 3}},
		{name: "rejected record contributes nothing", nodes: memory{"n": "~id,p:String(single),q:Int(single)\nA,ZZ,1\nA,AA,bad\n"}, first: []graph.Value{zz}, last: []graph.Value{zz}, rejected: 1},
		{name: "source keys use byte order", nodes: memory{"10.csv": "~id,p:String(single)\nA,ZZ\n", "2.csv": "~id,p:String(single)\nA,AA\n"}, first: []graph.Value{zz}, last: []graph.Value{aa}, conflict: true, contributions: 2, firstLoc: ports.Location{Source: "10.csv", Record: 2, Line: 2}, lastLoc: ports.Location{Source: "2.csv", Record: 2, Line: 2}},
		{name: "typed equality", nodes: memory{"a": "~id,p:Int(single)\nA,1\n", "b": "~id,p:Long(single)\nA,1\n"}, first: []graph.Value{intOne}, last: []graph.Value{longOne}, conflict: true, contributions: 2, firstLoc: ports.Location{Source: "a", Record: 2, Line: 2}, lastLoc: ports.Location{Source: "b", Record: 2, Line: 2}},
		{name: "implicit set retains union", nodes: memory{"n": "~id,p:String\nA,ZZ\nA,AA\nA,ZZ\n"}, first: []graph.Value{aa, zz}, last: []graph.Value{aa, zz}},
		{name: "cardinality conflict still drops", nodes: memory{"a": "~id,p:String(single)\nA,ZZ\n", "b": "~id,p:String(set)[]\nA,AA;ZZ\n"}, cardinality: true},
		{name: "multiline record order", nodes: memory{"n": "~id,p:String(single)\nA,\"ZZ\nline\"\nA,AA\n"}, first: []graph.Value{textValue(t, "ZZ\nline")}, last: []graph.Value{aa}, conflict: true, contributions: 2, firstLoc: ports.Location{Source: "n", Record: 2, Line: 2}, lastLoc: ports.Location{Source: "n", Record: 3, Line: 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, policy := range []ingest.NodePropertyConflictPolicy{ingest.LastWins, ingest.FirstWins, ingest.DropConflictingProperty} {
				t.Run(policy.String(), func(t *testing.T) {
					want, location := tc.last, tc.lastLoc
					if policy == ingest.FirstWins {
						want, location = tc.first, tc.firstLoc
					}
					if policy == ingest.DropConflictingProperty && tc.conflict {
						want = nil
					}
					location.Column = "p"
					for _, indexed := range []bool{false, true} {
						options := ingest.Options{NodePropertyConflictPolicy: policy}
						if indexed {
							options.IndexProperties = []string{"p"}
						}
						compareBuildsWithOptions(t, tc.nodes, memory{}, options, func(g *graph.Graph, r ingest.Report, events []ports.Diagnostic) {
							if got := nodeValues(t, g, "p"); !reflect.DeepEqual(got, want) {
								t.Fatalf("values=%v want=%v", got, want)
							}
							partial := tc.cardinality || tc.rejected > 0 || (tc.conflict && policy == ingest.DropConflictingProperty)
							if (r.Completeness == ingest.Partial) != partial || g.Metadata().PartialLoad != partial || r.NodeSources.RecordsRejected != tc.rejected {
								t.Fatal(r, g.Metadata())
							}
							resolved := tc.conflict && policy != ingest.DropConflictingProperty
							if (r.ResolvedNodePropertyConflictGroups == 1) != resolved || (r.Warnings == 1) != resolved || (r.PropertyConflictGroups == 1) != (tc.conflict && !resolved) || (r.PropertyCardinalityConflictGroups == 1) != tc.cardinality {
								t.Fatal(r)
							}
							if resolved {
								if len(events) != 1 || events[0].Code != "NODE_PROPERTY_CONFLICT_RESOLVED" || events[0].Severity != ports.Warning || events[0].Location != location || events[0].Resolution != policy.String() || events[0].Contributions != tc.contributions || len(events[0].Related) != 2 || events[0].EntityID != "A" || !events[0].EntityIDKnown {
									t.Fatal(events)
								}
								if strings.Contains(events[0].Message, "ZZ") || strings.Contains(events[0].Message, "AA") {
									t.Fatal("diagnostic exposed a property value")
								}
							}
							for _, v := range append(append([]graph.Value{}, tc.first...), tc.last...) {
								matches, err := g.NodesWithProperty(context.Background(), "p", v)
								if err != nil {
									t.Fatal(err)
								}
								found := false
								for _, w := range want {
									found = found || v.Equal(w)
								}
								if (matches.Count() == 1) != found {
									t.Fatal("index/scan includes a discarded value", v, matches.Count())
								}
							}
						})
					}
				})
			}
		})
	}
}

func TestNodePropertyPoliciesPreserveOtherGroups(t *testing.T) {
	n := memory{"n": "~id,~label,p:String(single),q:String(single),s:String(set)[]\nA,L,ZZ,retained,one;two\nA,M,AA,,two;three\nB,L,,,\n"}
	e := memory{"e": "~id,~from,~to,~label,p:String\ne,A,B,L,one\ne,A,B,L,two\nparallel,A,B,L,value\nloop,A,A,L,value\nbad,A,B,L,one\nbad,B,A,L,two\n"}
	for _, policy := range []ingest.NodePropertyConflictPolicy{ingest.LastWins, ingest.FirstWins, ingest.DropConflictingProperty} {
		compareBuildsWithOptions(t, n, e, ingest.Options{NodePropertyConflictPolicy: policy}, func(g *graph.Graph, r ingest.Report, _ []ports.Diagnostic) {
			if got := nodeValues(t, g, "q"); !reflect.DeepEqual(got, []graph.Value{textValue(t, "retained")}) {
				t.Fatal("whole row replaced a property", got)
			}
			if len(nodeValues(t, g, "s")) != 3 || r.Edges != 3 || r.QuarantinedEdgeIDs != 1 || r.Completeness != ingest.Partial {
				t.Fatal(r)
			}
			id, _ := g.FindNode("A")
			labels, err := g.NodeLabels(id)
			if err != nil || len(labels) != 2 {
				t.Fatal(labels, err)
			}
			edge, err := g.FindEdge("e")
			if err != nil {
				t.Fatal(err)
			}
			props, err := g.EdgeProperties(edge)
			if err != nil || props.Next() || props.Err() != nil {
				t.Fatal("edge conflict changed", err, props.Err())
			}
		})
	}
}

func TestExternalNodePropertyWinnerAcrossRuns(t *testing.T) {
	var csv strings.Builder
	csv.WriteString("~id,p:String(single)\nA,ZZ-first\n")
	for i := 0; i < 16000; i++ {
		fmt.Fprintf(&csv, "A,middle-%06d-%s\n", i, strings.Repeat("m", 100))
	}
	csv.WriteString("A,AA-last\n")
	for _, policy := range []ingest.NodePropertyConflictPolicy{ingest.LastWins, ingest.FirstWins, ingest.DropConflictingProperty} {
		t.Run(policy.String(), func(t *testing.T) {
			var want []graph.Value
			if policy == ingest.LastWins {
				want = []graph.Value{textValue(t, "AA-last")}
			} else if policy == ingest.FirstWins {
				want = []graph.Value{textValue(t, "ZZ-first")}
			}
			compareBuildsWithOptions(t, memory{"n": csv.String()}, memory{}, ingest.Options{NodePropertyConflictPolicy: policy, IndexProperties: []string{"p"}}, func(g *graph.Graph, _ ingest.Report, _ []ports.Diagnostic) {
				if got := nodeValues(t, g, "p"); !reflect.DeepEqual(got, want) {
					t.Fatal("sort run order selected the winner", got)
				}
			})
		})
	}
}

func TestExternalNodePropertyOversizedWinner(t *testing.T) {
	large := strings.Repeat("Z", 600000)
	n := memory{"n": "~id,p:String(single)\nA," + large + "\nA,AA\nA," + large + "\n"}
	for _, policy := range []ingest.NodePropertyConflictPolicy{ingest.LastWins, ingest.FirstWins} {
		compareBuildsWithOptions(t, n, memory{}, ingest.Options{NodePropertyConflictPolicy: policy}, func(g *graph.Graph, _ ingest.Report, _ []ports.Diagnostic) {
			if got := nodeValues(t, g, "p"); !reflect.DeepEqual(got, []graph.Value{textValue(t, large)}) {
				t.Fatal("oversized winner lost")
			}
		})
	}
}

func TestNodePropertyInvalidPolicyBeforeIO(t *testing.T) {
	for _, disk := range []bool{false, true} {
		catalogErr := errors.New("catalog must not be read")
		n := faultCatalog{listErr: catalogErr}
		scratch := &faultScratch{dir: t.TempDir()}
		options := ingest.Options{NodePropertyConflictPolicy: 255}
		if disk {
			options.Scratch = scratch
		}
		g, r, err := ingest.Build(context.Background(), n, memory{}, neptune.Decoder{}, &diagnostics{}, options)
		if g != nil || err == nil || errors.Is(err, catalogErr) || r.NodeSources.SourcesSeen != 0 || len(scratch.calls) != 0 {
			t.Fatal(g, r, err, scratch.calls)
		}
	}
}

func TestNodePropertySourceFailurePreservesContributions(t *testing.T) {
	n := faultCatalog{entries: []ports.Entry{{Key: "b", Regular: true}, {Key: "a", Regular: true}}, open: func(key string) (io.ReadCloser, error) {
		if key == "a" {
			return io.NopCloser(&brokenTail{data: "~id,p:String(single)\nA,first\nA,prefix\n\"tail", err: diskFault}), nil
		}
		return io.NopCloser(strings.NewReader("~id,p:String(single)\nA,last\n")), nil
	}}
	for _, policy := range []ingest.NodePropertyConflictPolicy{ingest.FirstWins, ingest.LastWins} {
		compareBuildsWithOptions(t, n, memory{}, ingest.Options{NodePropertyConflictPolicy: policy}, func(g *graph.Graph, r ingest.Report, _ []ports.Diagnostic) {
			want := "last"
			if policy == ingest.FirstWins {
				want = "first"
			}
			if !reflect.DeepEqual(nodeValues(t, g, "p"), []graph.Value{textValue(t, want)}) || r.NodeSources.SourcesIOFailed != 1 || r.NodeSources.SourcesCompleted != 1 || r.NodeSources.RecordsStaged != 3 || r.Completeness != ingest.Partial || r.ResolvedNodePropertyConflictGroups != 1 {
				t.Fatal("accepted prefix lost or enumeration order selected the winner", r)
			}
		})
	}
}

func TestResolvedNodeConflictDiagnosticFailure(t *testing.T) {
	for _, disk := range []bool{false, true} {
		options := ingest.Options{}
		if disk {
			options.Scratch = filesystem.Scratch{Dir: t.TempDir()}
			options.MemoryBudget = 1 << 20
		}
		diag := &diagnostics{fail: true}
		g, _, err := ingest.Build(context.Background(), memory{"n": "~id,p:String(single)\nA,ZZ\nA,AA\n"}, memory{}, neptune.Decoder{}, diag, options)
		var failure *ingest.DiagnosticError
		if g != nil || !errors.As(err, &failure) {
			t.Fatal("resolved conflict warning failure must prevent publication", g, err)
		}
	}
}

// Locations are opaque diagnostics, not order keys. A normalized custom decoder
// can emit repeated single contributions within an event; spill must agree with
// heap even when they have the same (or unknown) location.
type repeatedPropertyDecoder struct{}
type repeatedPropertyReader struct{ emitted bool }

func (repeatedPropertyDecoder) New(context.Context, ports.Role, ports.Entry, io.Reader, ports.Limits) (ports.RecordReader, error) {
	return &repeatedPropertyReader{}, nil
}
func (r *repeatedPropertyReader) Next(context.Context) (ports.Event, error) {
	if r.emitted {
		return ports.Event{}, io.EOF
	}
	r.emitted = true
	aa, _ := graph.TextValue(graph.StringKind, "AA")
	zz, _ := graph.TextValue(graph.StringKind, "ZZ")
	return ports.Event{Record: &ports.Record{Role: ports.Nodes, ID: "A", Labels: []string{"vertex"}, Properties: []ports.Property{
		{Key: "p", Value: aa, Cardinality: ports.Single},
		{Key: "p", Value: zz, Cardinality: ports.Single},
	}}}, nil
}

func TestNodePropertyCustomDecoderContributionOrder(t *testing.T) {
	for _, policy := range []ingest.NodePropertyConflictPolicy{ingest.FirstWins, ingest.LastWins} {
		for _, disk := range []bool{false, true} {
			options := ingest.Options{NodePropertyConflictPolicy: policy}
			if disk {
				options.Scratch = filesystem.Scratch{Dir: t.TempDir()}
				options.MemoryBudget = 1 << 20
			}
			g, r, err := ingest.Build(context.Background(), memory{"n": ""}, memory{}, repeatedPropertyDecoder{}, &diagnostics{}, options)
			if err != nil {
				t.Fatal(err)
			}
			want := "ZZ"
			if policy == ingest.FirstWins {
				want = "AA"
			}
			if !reflect.DeepEqual(nodeValues(t, g, "p"), []graph.Value{textValue(t, want)}) || r.ResolvedNodePropertyConflictGroups != 1 {
				t.Fatal("order derived from location or sort", policy, disk, r)
			}
			if err := g.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
}
