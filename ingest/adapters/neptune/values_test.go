package neptune

import (
	"context"
	"gophergraph/graph"
	"gophergraph/ingest/ports"
	"strings"
	"testing"
)

func TestScalarDialect(t *testing.T) {
	for _, c := range []struct {
		k     graph.ValueKind
		s     string
		valid bool
	}{{graph.ByteKind, "-128", true}, {graph.ByteKind, "128", false}, {graph.ShortKind, "32767", true}, {graph.IntKind, "2147483648", false}, {graph.LongKind, "-9223372036854775808", true}, {graph.LongKind, "0x10", false}, {graph.LongKind, "1_000", false}, {graph.FloatKind, "1e-9999", true}, {graph.FloatKind, "1e9999", false}, {graph.DoubleKind, "INF", false}, {graph.DoubleKind, "0x1p2", false}, {graph.DoubleKind, "NaN", true}, {graph.DoubleKind, "+Infinity", true}, {graph.DoubleKind, "-.3e+2", true}, {graph.DoubleKind, "nan", false}, {graph.BoolKind, "", false}, {graph.StringKind, "", true}, {graph.DateKind, "2024-02-29", true}, {graph.DateKind, "2023-02-29", false}, {graph.DateKind, "0000-01-01", false}, {graph.DatetimeKind, "2024-01-01T23:59", true}, {graph.DatetimeKind, "2024-01-01T23:59:59.123456789+1800", true}, {graph.DatetimeKind, "2024-01-01T23:59:59+18:01", false}, {graph.DatetimeKind, "2024-01-01T23:59:60Z", false}, {graph.DatetimeKind, "2024-01-01T24:00Z", false}} {
		_, _, e := scalar(c.k, c.s)
		if (e == nil) != c.valid {
			t.Fatalf("%v %q: %v", c.k, c.s, e)
		}
	}
	v, w, e := scalar(graph.BoolKind, "TRUE")
	b, _ := v.Bool()
	if e != nil || !w || b {
		t.Fatal("coercion")
	}
}
func TestArraysAndAtomicWarnings(t *testing.T) {
	ctx := context.Background()
	r, e := (Decoder{}).New(ctx, ports.Nodes, ports.Entry{}, strings.NewReader("~id,ns\\:key:String[],flag:Bool,x:Int\nA,a\\;b;;c\\n,TRUE,1\nB,z,no,bad\n"), ports.Limits{})
	if e != nil {
		t.Fatal(e)
	}
	ev, e := r.Next(ctx)
	if e != nil || len(ev.Warnings) != 1 || len(ev.Record.Properties) != 5 {
		t.Fatal(ev, e)
	}
	for i, want := range []string{"a;b", "", "c\\n"} {
		p := ev.Record.Properties[i]
		s, _ := p.Value.Text()
		if p.Key != "ns:key" || s != want {
			t.Fatal(p, s)
		}
	}
	ev, e = r.Next(ctx)
	if e != nil || ev.Rejection == nil || len(ev.Warnings) != 0 {
		t.Fatal(ev, e)
	}
	for _, h := range []string{"~id,~from,~to,x:String[]", "~id,~from,~to,x:String(set)", "~id,~from"} {
		if _, e := (Decoder{}).New(ctx, ports.Edges, ports.Entry{}, strings.NewReader(h+"\n"), ports.Limits{}); e == nil {
			t.Fatal(h)
		}
	}
}
