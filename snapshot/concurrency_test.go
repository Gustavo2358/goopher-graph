package snapshot

import (
	"context"
	"gophergraph/query"
	"gophergraph/snapshot/adapters/mmap"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestConcurrentQueriesAndResourceRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "owned.snapshot")
	data, e := os.ReadFile("../fixtures/snapshot_reference/topology.snapshot")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	g, e := Open(context.Background(), mmap.New(path))
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 16)
	for worker := 0; worker < 16; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				id, e := g.FindNode("A")
				if e != nil {
					failures <- e
					return
				}
				s, e := query.Territory(context.Background(), g, id, query.Options{})
				if e != nil {
					failures <- e
					return
				}
				if s.NodeCount() != 5 || s.EdgeCount() != 8 {
					t.Errorf("concurrent result %d/%d", s.NodeCount(), s.EdgeCount())
					return
				}
				if _, e = g.NodesWithLabel(context.Background(), "PROGRAM"); e != nil {
					failures <- e
					return
				}
			}
		}()
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		t.Error(e)
	}
	if e = g.Close(); e != nil {
		t.Fatal(e)
	}
	bad := filepath.Join(dir, "bad.snapshot")
	if e = os.WriteFile(bad, []byte("broken"), 0600); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 100; i++ {
		g, e := Open(context.Background(), mmap.New(path))
		if e != nil {
			t.Fatal(e)
		}
		id, _ := g.FindNode("A")
		if _, e = query.Territory(context.Background(), g, id, query.Options{}); e != nil {
			t.Fatal(e)
		}
		if e = g.Close(); e != nil {
			t.Fatal(e)
		}
		if _, e = Open(context.Background(), mmap.New(bad)); e == nil {
			t.Fatal("corrupt accepted")
		}
	}
	maps, e := os.ReadFile("/proc/self/maps")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(maps), dir) {
		t.Fatal("mapping leaked")
	}
	fds, e := os.ReadDir("/proc/self/fd")
	if e != nil {
		t.Fatal(e)
	}
	for _, fd := range fds {
		target, _ := os.Readlink(filepath.Join("/proc/self/fd", fd.Name()))
		if strings.HasPrefix(target, dir) {
			t.Fatal("descriptor leaked", target)
		}
	}
}
