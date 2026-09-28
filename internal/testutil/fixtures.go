// Package testutil supplies independent fixture assertions to integration tests.
package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"gophergraph/graph"
	"gophergraph/query"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

type Property struct {
	Key, Type string
	Value     json.RawMessage
}
type Node struct {
	ID         string
	Labels     []string
	Properties []Property
}
type Edge struct {
	ID, Source, Target, Label string
	Properties                []Property
}
type Expected struct {
	Load struct {
		Completeness string
		Nodes, Edges uint64
		Diagnostics  map[string]int    `json:"required_diagnostics"`
		Exact        map[string]uint64 `json:"report_exact"`
	}
	IndexProperties []string `json:"index_properties"`
	Graph           struct {
		Nodes []Node
		Edges []Edge
	}
	Queries []struct {
		Kind, Start, End string
		Labels           []string `json:"edge_labels"`
		Members, Edges   []string
		IDs              []string `json:"ids_default"`
	}
}

func Read(t testing.TB, dir string) Expected {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(dir, "expected.json"))
	if e != nil {
		t.Fatal(e)
	}
	var v Expected
	if e = json.Unmarshal(b, &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func Value(t testing.TB, p Property) graph.Value {
	t.Helper()
	names := []string{"", "Bool", "Byte", "Short", "Int", "Long", "Float", "Double", "String", "Date", "Datetime"}
	var k graph.ValueKind
	for i, s := range names {
		if s == p.Type {
			k = graph.ValueKind(i)
		}
	}
	var v graph.Value
	var err error
	switch {
	case k == graph.BoolKind:
		var b bool
		err = json.Unmarshal(p.Value, &b)
		v = graph.BoolValue(b)
	case k >= graph.ByteKind && k <= graph.LongKind:
		n, e := strconv.ParseInt(string(p.Value), 10, 64)
		if e != nil {
			t.Fatal(e)
		}
		v, err = graph.IntegerValue(k, n)
	case k == graph.FloatKind || k == graph.DoubleKind:
		s := string(p.Value)
		if len(s) > 0 && s[0] == '"' {
			if e := json.Unmarshal(p.Value, &s); e != nil {
				t.Fatal(e)
			}
		}
		f, e := strconv.ParseFloat(s, 64)
		if e != nil {
			t.Fatal(e)
		}
		v, err = graph.DecimalValue(k, f)
	default:
		var s string
		err = json.Unmarshal(p.Value, &s)
		if err == nil {
			v, err = graph.TextValue(k, s)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func CheckGraph(t testing.TB, g *graph.Graph, w Expected) {
	t.Helper()
	m := g.Metadata()
	if m.Nodes != uint64(len(w.Graph.Nodes)) || m.Edges != uint64(len(w.Graph.Edges)) || m.PartialLoad != (w.Load.Completeness == "PARTIAL") {
		t.Fatalf("metadata %+v expected %+v", m, w.Load)
	}
	checkProps := func(it graph.PropertyIterator, want []Property) {
		actual := make(map[string][]graph.Value)
		n := 0
		for it.Next() {
			p := it.Property()
			key, e := g.String(p.Key)
			if e != nil {
				t.Fatal(e)
			}
			actual[key] = append(actual[key], p.Value)
			n++
		}
		if it.Err() != nil {
			t.Fatal(it.Err())
		}
		if n != len(want) {
			t.Fatalf("properties count %d != %d: %+v", n, len(want), actual)
		}
		for _, p := range want {
			v := Value(t, p)
			found := false
			for _, a := range actual[p.Key] {
				if a.Equal(v) {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing property %s %s %s", p.Key, p.Type, p.Value)
			}
		}
	}
	for _, node := range w.Graph.Nodes {
		id, e := g.FindNode(node.ID)
		if e != nil {
			t.Fatal(e, node.ID)
		}
		labels, e := g.NodeLabels(id)
		if e != nil {
			t.Fatal(e)
		}
		var names []string
		for _, l := range labels {
			s, _ := g.String(l)
			names = append(names, s)
		}
		want := append([]string(nil), node.Labels...)
		sort.Strings(want)
		if fmt.Sprint(names) != fmt.Sprint(want) {
			t.Fatal(names, want)
		}
		it, e := g.NodeProperties(id)
		if e != nil {
			t.Fatal(e)
		}
		checkProps(it, node.Properties)
	}
	for _, edge := range w.Graph.Edges {
		id, e := g.FindEdge(edge.ID)
		if e != nil {
			t.Fatal(e, edge.ID)
		}
		v, e := g.Edge(id)
		if e != nil {
			t.Fatal(e)
		}
		src, _ := g.NodeExternalID(v.Source)
		dst, _ := g.NodeExternalID(v.Target)
		label, _ := g.String(v.Label)
		if src != edge.Source || dst != edge.Target || label != edge.Label {
			t.Fatal(v, edge)
		}
		it, e := g.EdgeProperties(id)
		if e != nil {
			t.Fatal(e)
		}
		checkProps(it, edge.Properties)
	}
}
func CheckQueries(t testing.TB, g *graph.Graph, w Expected) {
	t.Helper()
	for _, q := range w.Queries {
		start, e := g.FindNode(q.Start)
		if e != nil {
			t.Fatal(e)
		}
		o := query.Options{}
		if q.Labels != nil {
			o.EdgeLabels = []graph.StringID{}
			for _, s := range q.Labels {
				id, ok, e := g.FindString(s)
				if e != nil {
					t.Fatal(e)
				}
				if ok {
					o.EdgeLabels = append(o.EdgeLabels, id)
				}
			}
		}
		var s *query.Subgraph
		switch q.Kind {
		case "territory":
			s, e = query.Territory(context.Background(), g, start, o)
		case "anti_territory":
			s, e = query.AntiTerritory(context.Background(), g, start, o)
		case "between":
			end, err := g.FindNode(q.End)
			if err != nil {
				t.Fatal(err)
			}
			s, e = query.Between(context.Background(), g, start, end, o)
		default:
			t.Fatal(q.Kind)
		}
		if e != nil {
			t.Fatal(e)
		}
		var ns, es []string
		ni := s.Nodes()
		for ni.Next() {
			v, _ := g.NodeExternalID(ni.ID())
			ns = append(ns, v)
		}
		ei := s.Edges()
		for ei.Next() {
			v, _ := g.EdgeExternalID(ei.ID())
			es = append(es, v)
		}
		wn, we := append([]string(nil), q.Members...), append([]string(nil), q.Edges...)
		sort.Strings(wn)
		sort.Strings(we)
		if fmt.Sprint(ns) != fmt.Sprint(wn) || fmt.Sprint(es) != fmt.Sprint(we) {
			t.Fatalf("%s %s: nodes %q want %q edges %q want %q", q.Kind, q.Start, ns, wn, es, we)
		}
	}
}
func Payload(v graph.Value) uint64 {
	switch v.Kind() {
	case graph.BoolKind:
		b, _ := v.Bool()
		if b {
			return 1
		}
	case graph.ByteKind, graph.ShortKind, graph.IntKind, graph.LongKind:
		n, _ := v.Int64()
		return uint64(n)
	case graph.FloatKind:
		n, _ := v.Float64()
		return uint64(math.Float32bits(float32(n)))
	case graph.DoubleKind:
		n, _ := v.Float64()
		return math.Float64bits(n)
	}
	return 0
}
