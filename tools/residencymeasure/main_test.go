//go:build linux && amd64 && residencybench

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
	"gophergraph/snapshot/adapters/mmap"
)

func TestPrivateCopyColdAndWarmPages(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("explicit benchmark dependency Python unavailable")
	}
	dir := t.TempDir()
	var fs unix.Statfs_t
	if err := unix.Statfs(dir, &fs); err != nil {
		t.Fatal(err)
	}
	if fs.Type == unix.TMPFS_MAGIC {
		t.Skip("page-cache eviction requires disk-backed TMPDIR, not tmpfs")
	}
	original := filepath.Join(dir, "original")
	private := filepath.Join(dir, "private")
	want := bytes.Repeat([]byte("graph-benchmark"), 10000)
	if err := os.WriteFile(original, want, 0600); err != nil {
		t.Fatal(err)
	}
	if err := copySnapshot(original, private); err != nil {
		t.Fatal(err)
	}
	if err := copySnapshot(original, private); err == nil {
		t.Fatal("must not overwrite an existing inode")
	}
	b, err := mmap.New(private).Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	f, err := os.Open(private)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, condition := range []string{"cold", "warm-cache", "warm", "cold"} {
		if err = mmap.BenchmarkPrepare(b.Bytes(), int(f.Fd()), condition); err != nil {
			t.Fatal(err)
		}
		p, err := observe(context.Background(), "python3", "../residency_probe.py", private)
		if err != nil {
			t.Fatal(err)
		}
		if p.Pages == 0 || (condition == "cold" && p.ResidentPages != 0) || (condition != "cold" && p.ResidentPages != p.Pages) {
			t.Fatal(condition, p)
		}
	}
	got, err := os.ReadFile(private)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("cache advice must preserve snapshot bytes", err)
	}
	got, err = os.ReadFile(original)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("source file must remain unchanged", err)
	}
}
