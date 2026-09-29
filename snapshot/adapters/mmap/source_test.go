package mmap

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMappingLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data")
	want := []byte("stable bytes")
	if e := os.WriteFile(path, want, 0600); e != nil {
		t.Fatal(e)
	}
	b, e := New(path).Acquire(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(b.Bytes(), want) {
		t.Fatal("mapped bytes")
	}
	if e = b.Close(); e != nil {
		t.Fatal(e)
	}
	if b.Bytes() != nil {
		t.Fatal("retained view")
	}
	if e = b.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = New(dir).Acquire(context.Background()); e == nil {
		t.Fatal("directory")
	}
}
