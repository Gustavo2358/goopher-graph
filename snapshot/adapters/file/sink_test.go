package file

import (
	"context"
	"errors"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"gophergraph/snapshot/ports"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicPublication(t *testing.T) {
	ctx := context.Background()
	g, e := snapshot.Open(ctx, mmap.New("../../../fixtures/snapshot_reference/topology.snapshot"))
	if e != nil {
		t.Fatal(e)
	}
	defer g.Close()
	small, e := snapshot.Open(ctx, mmap.New("../../../fixtures/snapshot_reference/empty.snapshot"))
	if e != nil {
		t.Fatal(e)
	}
	defer small.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "graph")
	pub, e := snapshot.Write(ctx, g, New(path))
	if e != nil || pub != ports.PublishedDurable {
		t.Fatal(pub, e)
	}
	old, e := snapshot.Open(ctx, mmap.New(path))
	if e != nil {
		t.Fatal(e)
	}
	defer old.Close()
	info, _ := os.Stat(path)
	pub, e = snapshot.Write(ctx, small, New(path))
	if e != nil {
		t.Fatal(e)
	}
	next, _ := os.Stat(path)
	if os.SameFile(info, next) {
		t.Fatal("inode reused")
	}
	if _, e = old.FindNode("A"); e != nil {
		t.Fatal("old reader invalid", e)
	}
	fresh, e := snapshot.Open(ctx, mmap.New(path))
	if e != nil {
		t.Fatal(e)
	}
	defer fresh.Close()
	if fresh.Metadata().Nodes != 0 {
		t.Fatal("new reader")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("staging leak", entries)
	}
	boom := errors.New("injected failure")
	for _, step := range []string{"sync", "close", "rename", "directory-sync"} {
		t.Run(step, func(t *testing.T) {
			s := New(path)
			switch step {
			case "sync":
				s.ops.sync = func(*os.File) error { return boom }
			case "close":
				s.ops.close = func(f *os.File) error { _ = f.Close(); return boom }
			case "rename":
				s.ops.rename = func(string, string) error { return boom }
			case "directory-sync":
				s.ops.dirSync = func(*os.File) error { return boom }
			}
			pub, e := snapshot.Write(ctx, g, s)
			if !errors.Is(e, boom) {
				t.Fatal(e)
			}
			expected := ports.NotPublished
			if step == "directory-sync" {
				expected = ports.PublishedUncertain
			}
			if pub != expected {
				t.Fatal(pub)
			}
			current, e := snapshot.Open(ctx, mmap.New(path))
			if e != nil {
				t.Fatal(e)
			}
			defer current.Close()
			want := uint64(0)
			if step == "directory-sync" {
				want = g.Metadata().Nodes
			}
			if current.Metadata().Nodes != want {
				t.Fatal("publication state")
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 1 {
				t.Fatal("staging leak")
			}
		})
	}
}
func TestTransactionFailures(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if _, e := New(filepath.Join(dir, "missing", "x")).Begin(ctx, 1); e == nil {
		t.Fatal("begin succeeded")
	}
	raw, e := New(filepath.Join(dir, "x")).Begin(ctx, 4)
	if e != nil {
		t.Fatal(e)
	}
	tx := raw.(*transaction)
	if _, e = tx.Seal(ctx); e == nil {
		t.Fatal("incomplete seal")
	}
	if _, e = tx.Write([]byte("data")); e != nil {
		t.Fatal(e)
	}
	view, e := tx.Seal(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if p, e := tx.Commit(ctx); e == nil || p != ports.NotPublished {
		t.Fatal("live lease commit")
	}
	if e = view.Close(); e != nil {
		t.Fatal(e)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if p, e := tx.Commit(cancelled); e == nil || p != ports.NotPublished {
		t.Fatal("cancelled commit")
	}
	if e = tx.Abort(); e != nil {
		t.Fatal(e)
	}
	if e = tx.Abort(); e != nil {
		t.Fatal(e)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("staging leak")
	}
	raw, e = New(filepath.Join(dir, "x")).Begin(ctx, 4)
	if e != nil {
		t.Fatal(e)
	}
	tx = raw.(*transaction)
	_ = tx.file.Close()
	if _, e = tx.Write([]byte("data")); e == nil {
		t.Fatal("write error ignored")
	}
	_ = tx.Abort()
	entries, _ = os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("staging leak")
	}
}
