package graphjson_test

import (
	"context"
	"encoding/json"
	"fmt"
	"gophergraph/graph"
	"gophergraph/graphjson"
	"gophergraph/internal/graphdata"
	"gophergraph/internal/testutil"
	"gophergraph/query"
	"math"
	"slices"
	"strconv"
	"testing"
)

func TestScalarPrecisionAndEscaping(t *testing.T) {
	// Every string position includes characters requiring JSON escaping; an empty
	// external node ID is both the origin and both endpoints of a self-loop.
	weird := "\"\\\b\f\n\r\t\x00\x01\x1f<>&\u2028\u2029Olá😀"
	text, err := graph.TextValue(graph.StringKind, weird)
	if err != nil {
		t.Fatal(err)
	}
	values := []graph.Value{text, graph.BoolValue(false), graph.BoolValue(true)}
	for _, n := range []int64{math.MinInt64, -9007199254740993, -9007199254740991, 0, 9007199254740991, 9007199254740992, 9007199254740993, math.MaxInt64} {
		v, err := graph.IntegerValue(graph.LongKind, n)
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, v)
	}
	for _, kind := range []graph.ValueKind{graph.FloatKind, graph.DoubleKind} {
		for _, n := range []float64{0, math.Copysign(0, -1), 0.1, math.SmallestNonzeroFloat32, math.SmallestNonzeroFloat64, math.MaxFloat32, math.MaxFloat64, math.Inf(1), math.Inf(-1), math.NaN()} {
			v, err := graph.DecimalValue(kind, n)
			if err != nil {
				t.Fatal(err)
			}
			values = append(values, v)
		}
	}
	d := graphdata.Empty()
	d.Strings = []string{"", weird, "other"}
	for i := range values {
		d.Strings = append(d.Strings, fmt.Sprintf("%s/%03d", weird, i))
	}
	slices.Sort(d.Strings)
	sid := func(s string) uint32 {
		i, ok := slices.BinarySearch(d.Strings, s)
		if !ok {
			t.Fatal(s)
		}
		return uint32(i)
	}
	d.NodeIDs.Heap = []uint32{sid("")}
	d.NodeLabels.Heap = []uint32{sid(weird), sid("other")}
	slices.Sort(d.NodeLabels.Heap)
	d.NodeLabelOffsets.Heap = []uint64{0, 2}
	d.NodePropOffsets.Heap = []uint64{0, uint64(len(values))}
	for i, v := range values {
		payload := testutil.Payload(v)
		if s, ok := v.Text(); ok {
			payload = uint64(sid(s))
		}
		d.NodeProps.Heap = append(d.NodeProps.Heap, graphdata.Property{Key: sid(fmt.Sprintf("%s/%03d", weird, i)), Kind: uint8(v.Kind()), Payload: payload})
	}
	d.EdgeIDs.Heap = []uint32{sid(weird)}
	d.Sources.Heap, d.Targets.Heap, d.EdgeLabels.Heap = []uint32{0}, []uint32{0}, []uint32{sid(weird)}
	d.EdgePropOffsets.Heap = []uint64{0, uint64(len(values))}
	d.EdgeProps.Heap = slices.Clone(d.NodeProps.Heap)
	graphdata.BuildCSR(d)
	if err := graphdata.BuildIndexes(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	g, err := graph.New(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	sub, err := query.Territory(context.Background(), g, 0, query.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_, doc := encode(t, g, sub, graphjson.Query{Name: weird, Node: new(""), EdgeLabels: []string{weird}})
	if doc.Query.Name != weird || doc.Query.Node == nil || *doc.Query.Node != "" || !slices.Equal(doc.Query.EdgeLabels, []string{weird}) || doc.Nodes[0].ID != "" || !slices.Contains(doc.Nodes[0].Labels, weird) || doc.Edges[0].ID != weird || doc.Edges[0].Label != weird || doc.Edges[0].Source != "" || doc.Edges[0].Target != "" {
		t.Fatal(doc)
	}
	for _, props := range [][]testutil.Property{doc.Nodes[0].Properties, doc.Edges[0].Properties} {
		if len(props) != len(values) {
			t.Fatal(len(props))
		}
		for i, p := range props {
			v := values[i]
			if p.Key != fmt.Sprintf("%s/%03d", weird, i) {
				t.Fatal(p.Key)
			}
			switch v.Kind() {
			case graph.StringKind:
				var s string
				if err := json.Unmarshal(p.Value, &s); err != nil || s != weird || p.Type != "String" {
					t.Fatal(p, err)
				}
			case graph.BoolKind:
				b, _ := v.Bool()
				if string(p.Value) != strconv.FormatBool(b) || p.Type != "Bool" {
					t.Fatal(p)
				}
			case graph.LongKind:
				n, _ := v.Int64()
				var s string
				if err := json.Unmarshal(p.Value, &s); err != nil || s != strconv.FormatInt(n, 10) || p.Type != "Long" {
					t.Fatal(p, err)
				}
			default:
				bits, name := 64, "Double"
				if v.Kind() == graph.FloatKind {
					bits, name = 32, "Float"
				}
				if p.Type != name {
					t.Fatal(p.Type)
				}
				f, _ := v.Float64()
				sameFloat(t, p.Value, f, bits)
				if math.IsNaN(f) && string(p.Value) != `"NaN"` || math.IsInf(f, 1) && string(p.Value) != `"+Inf"` || math.IsInf(f, -1) && string(p.Value) != `"-Inf"` {
					t.Fatal(p)
				}
			}
		}
	}
}
