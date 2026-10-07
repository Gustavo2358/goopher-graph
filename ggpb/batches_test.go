package ggpb_test

import (
	"bytes"
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"gophergraph/ggpb"
	"gophergraph/query"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"io"
	"testing"
)

func TestUnframedSequence(t *testing.T) {
	ctx := context.Background()
	g, err := snapshot.Open(ctx, mmap.New("../fixtures/snapshot_reference/typed.snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	s, err := query.NewSubgraph(g)
	if err != nil {
		t.Fatal(err)
	}
	var framed bytes.Buffer
	if err = ggpb.Write(ctx, &framed, g, s, ggpb.Query{Name: "empty"}); err != nil {
		t.Fatal(err)
	}
	r := ggpb.NewReader(&framed)
	var d ggpb.BatchDecoder
	var payloads [][]byte
	err = ggpb.EmitEncoded(ctx, g, s, ggpb.Query{Name: "empty"}, ggpb.Options{}, func(p []byte) error {
		payloads = append(payloads, bytes.Clone(p))
		got, e := d.Decode(ctx, p)
		if e != nil {
			return e
		}
		want, e := r.Next(ctx)
		if e != nil {
			return e
		}
		if !proto.Equal(got, want) {
			t.Fatal("file/stream mismatch")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = d.Finish(); err != nil {
		t.Fatal(err)
	}
	if _, err = r.Next(ctx); err != io.EOF {
		t.Fatal(err)
	}
	if _, err = d.Decode(ctx, payloads[0]); !errors.Is(err, ggpb.ErrInvalid) {
		t.Fatal("accepted trailing batch", err)
	}
	var incomplete ggpb.BatchDecoder
	if _, err = incomplete.Decode(ctx, payloads[0]); err != nil {
		t.Fatal(err)
	}
	if err = incomplete.Finish(); !errors.Is(err, ggpb.ErrInvalid) {
		t.Fatal("accepted missing end")
	}
	var reordered ggpb.BatchDecoder
	if _, err = reordered.Decode(ctx, payloads[1]); !errors.Is(err, ggpb.ErrInvalid) {
		t.Fatal("accepted end first")
	}
	c, cancel := context.WithCancel(ctx)
	cancel()
	var canceled ggpb.BatchDecoder
	if _, err = canceled.Decode(c, payloads[0]); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func FuzzBatchDecode(f *testing.F) {
	f.Add([]byte{10, 2, 8, 1})
	f.Add([]byte{26, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > ggpb.MaxFrame {
			return
		}
		var d ggpb.BatchDecoder
		_, _ = d.Decode(context.Background(), b)
		_ = d.Finish()
	})
}
