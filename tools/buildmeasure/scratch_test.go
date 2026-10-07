package main

import (
	"context"
	"errors"
	"gophergraph/ingest/adapters/filesystem"
	"io"
	"testing"
)

func TestMeasuredScratchCountsPartialReadsAndMappingLifetime(t *testing.T) {
	var stats scratchStats
	w, err := (measuredScratch{Scratch: filesystem.Scratch{Dir: t.TempDir()}, stats: &stats}).New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	f, err := w.Create()
	if err != nil {
		t.Fatal(err)
	}
	if n, err := f.Write([]byte("abc")); n != 3 || err != nil {
		t.Fatal(n, err)
	}
	var buffer [8]byte
	if n, err := f.ReadAt(buffer[:], 0); n != 3 || !errors.Is(err, io.EOF) {
		t.Fatal(n, err)
	}
	if n, err := f.ReadAt(buffer[:2], 1); n != 2 || err != nil || string(buffer[:2]) != "bc" {
		t.Fatal(n, err, buffer)
	}
	if n, err := f.ReadAt(buffer[:], 3); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatal(n, err)
	}
	m, err := w.Map(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	if err := w.Remove(f); err != nil {
		t.Fatal(err)
	}
	if string(m.Bytes()) != "abc" || stats.Read != 5 || stats.Written != 3 || stats.Peak != 3 || stats.Current != 0 || stats.PeakFiles != 1 || stats.Files != 0 || stats.Mapped != 3 || stats.PeakMapped != 3 {
		t.Fatal(stats)
	}
}
