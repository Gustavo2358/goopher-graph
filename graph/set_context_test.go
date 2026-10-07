package graph

import (
	"context"
	"errors"
	"testing"
)

// Cancel during the scan, not merely before entering the API.
type scanContext struct {
	context.Context
	checks int
}

func (c *scanContext) Err() error {
	c.checks++
	if c.checks > 2 {
		return context.Canceled
	}
	return nil
}
func TestSetScanCancellation(t *testing.T) {
	g := new(Graph)
	s := &NodeSet{g: g, words: make([]uint64, 8192), size: 8192 * 64}
	s.words[0] = 3
	for _, run := range []func(context.Context) error{
		func(c context.Context) error { _, e := s.CountContext(c); return e },
		func(c context.Context) error { _, e := s.CloneContext(c); return e },
		func(c context.Context) error { _, e := s.IntersectionContext(c, s); return e },
		func(c context.Context) error { _, e := s.UnionContext(c, s); return e },
	} {
		if err := run(&scanContext{Context: context.Background()}); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	clone, err := s.CloneContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	clone.words[0] = 0
	if s.Count() != 2 {
		t.Fatal("alias")
	}
}
