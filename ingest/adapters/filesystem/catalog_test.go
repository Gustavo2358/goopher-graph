package filesystem_test

import (
	"context"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/filesystem"
	"gophergraph/ingest/adapters/neptune"
	"gophergraph/ingest/ports"
	"gophergraph/internal/testutil"
	"os"
	"path/filepath"
	"testing"
)

type sink struct{}

func (sink) Emit(context.Context, ports.Diagnostic) error { return nil }
func TestDirectories(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	n, e := filesystem.New(dir)
	if e != nil {
		t.Fatal(e)
	}
	entries, e := n.List(ctx)
	if e != nil || len(entries) != 0 {
		t.Fatal(entries, e)
	}
	if e = os.WriteFile(filepath.Join(dir, "no-extension"), []byte("~id\nA\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Symlink(filepath.Join(dir, "no-extension"), filepath.Join(dir, "link")); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(filepath.Join(dir, "folder"), 0700); e != nil {
		t.Fatal(e)
	}
	entries, e = n.List(ctx)
	if e != nil || len(entries) != 3 {
		t.Fatal(entries, e)
	}
	for _, key := range []string{"link", "folder", "../escape"} {
		if r, e := n.Open(ctx, key); e == nil {
			r.Close()
			t.Fatal("opened", key)
		}
	}
	edgeDir := t.TempDir()
	edges, _ := filesystem.New(edgeDir)
	g, r, e := ingest.Build(ctx, n, edges, neptune.Decoder{}, sink{}, ingest.Options{})
	if e != nil || g.Metadata().Nodes != 1 || r.NodeSources.SourcesNonRegular != 2 {
		t.Fatal(r, e)
	}
	p := filepath.Join(dir, "no-extension")
	if e = os.Chmod(p, 0000); e != nil {
		t.Fatal(e)
	}
	defer os.Chmod(p, 0600)
	if os.Geteuid() != 0 {
		if f, e := n.Open(ctx, "no-extension"); e == nil {
			f.Close()
			t.Fatal("permissions bypassed")
		}
	}
}
func TestFixturesOnDisk(t *testing.T) {
	paths, _ := filepath.Glob("../../../fixtures/*/expected.json")
	if len(paths) != 12 {
		t.Fatal(len(paths))
	}
	for _, p := range paths {
		dir := filepath.Dir(p)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			w := testutil.Read(t, dir)
			n, e := filesystem.New(filepath.Join(dir, "nodes"))
			if e != nil {
				t.Fatal(e)
			}
			edges, e := filesystem.New(filepath.Join(dir, "edges"))
			if e != nil {
				t.Fatal(e)
			}
			g, _, e := ingest.Build(context.Background(), n, edges, neptune.Decoder{}, sink{}, ingest.Options{IndexProperties: w.IndexProperties})
			if e != nil {
				t.Fatal(e)
			}
			defer g.Close()
			testutil.CheckGraph(t, g, w)
		})
	}
}
