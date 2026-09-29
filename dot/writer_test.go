package dot

import (
	"bytes"
	"context"
	"errors"
	"gophergraph/query"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"os/exec"
	"strings"
	"testing"
)

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("full") }
func TestDOT(t *testing.T) {
	g, e := snapshot.Open(context.Background(), mmap.New("../fixtures/snapshot_reference/topology.snapshot"))
	if e != nil {
		t.Fatal(e)
	}
	defer g.Close()
	id, _ := g.FindNode("A")
	s, e := query.Territory(context.Background(), g, id, query.Options{})
	if e != nil {
		t.Fatal(e)
	}
	var b bytes.Buffer
	if e = Write(&b, g, s); e != nil {
		t.Fatal(e)
	}
	if strings.Count(b.String(), " -> ") != 8 || strings.Count(b.String(), "n0 -> n1") != 2 || !strings.Contains(b.String(), "n4 -> n4") {
		t.Fatal(b.String())
	}
	if e = Write(brokenWriter{}, g, s); e == nil {
		t.Fatal("output error")
	}
	dot, e := exec.LookPath("dot")
	if e != nil {
		t.Skip("external Graphviz unavailable")
	}
	cmd := exec.Command(dot, "-Tdot")
	cmd.Stdin = strings.NewReader(b.String())
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("Graphviz: %v %s", e, out)
	}
}
func TestQuoteControls(t *testing.T) {
	got := quote("x\"\\\n\r\x01\t<evil>")
	want := "\"x\\\"\\\\\\n\\r\\\\x01\\\\x09<evil>\""
	if got != want {
		t.Fatalf("%q want %q", got, want)
	}
	dot, e := exec.LookPath("dot")
	if e != nil {
		t.Skip("Graphviz unavailable")
	}
	cmd := exec.Command(dot, "-Tdot")
	cmd.Stdin = strings.NewReader("digraph G { n0 [label=" + got + "]; }")
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("escaping: %v %s", e, out)
	}
}
