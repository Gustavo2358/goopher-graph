package graph

import (
	"errors"
	"slices"
	"testing"
)

func TestLabelIterator(t *testing.T) {
	g := fixture(t)
	defer g.Close()
	for _, id := range []NodeID{0, 1} {
		want, e := g.NodeLabels(id)
		if e != nil {
			t.Fatal(e)
		}
		it, e := g.IterateNodeLabels(id)
		if e != nil {
			t.Fatal(e)
		}
		var got []StringID
		for it.Next() {
			got = append(got, it.ID())
		}
		if it.Err() != nil || !slices.Equal(got, want) {
			t.Fatal(got, want, it.Err())
		}
	}
	if allocs := testing.AllocsPerRun(100, func() {
		it, e := g.IterateNodeLabels(0)
		if e != nil {
			t.Fatal(e)
		}
		for it.Next() {
			_ = it.ID()
		}
		if e = it.Err(); e != nil {
			t.Fatal(e)
		}
	}); allocs != 0 {
		t.Fatal("iterator allocates", allocs)
	}
	if _, e := g.IterateNodeLabels(InvalidNodeID); !errors.Is(e, ErrOutOfRange) {
		t.Fatal(e)
	}
	it, e := g.IterateNodeLabels(0)
	if e != nil {
		t.Fatal(e)
	}
	if e = g.Close(); e != nil {
		t.Fatal(e)
	}
	if it.Next() || !errors.Is(it.Err(), ErrClosed) {
		t.Fatal(it.Err())
	}
	if _, e = g.IterateNodeLabels(0); !errors.Is(e, ErrClosed) {
		t.Fatal(e)
	}
	var zero LabelIterator
	if zero.Next() || !errors.Is(zero.Err(), ErrClosed) {
		t.Fatal(zero.Err())
	}
}
