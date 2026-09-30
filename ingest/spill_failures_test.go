package ingest_test

import (
	"context"
	"errors"
	"fmt"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/filesystem"
	"gophergraph/ingest/adapters/neptune"
	"gophergraph/ingest/ports"
	"io"
	"os"
	"strings"
	"testing"
)

var diskFault = errors.New("injected scratch failure")

type faultScratch struct {
	dir, op    string
	at         int
	calls      map[string]int
	activeMaps int
	cancel     context.CancelFunc
}

func (s *faultScratch) hit(op string) error {
	if s.calls == nil {
		s.calls = map[string]int{}
	}
	s.calls[op]++
	if op == s.op && s.calls[op] == s.at {
		if s.cancel != nil {
			s.cancel()
			return nil
		}
		// A typed SourceError from storage is still an operational failure.
		return &ports.SourceError{Kind: ports.SourceIO, Cause: diskFault}
	}
	return nil
}
func (s *faultScratch) New(ctx context.Context) (ports.Workspace, error) {
	if err := s.hit("new"); err != nil {
		return nil, err
	}
	w, err := (filesystem.Scratch{Dir: s.dir}).New(ctx)
	if err != nil {
		return nil, err
	}
	return &faultWorkspace{Workspace: w, s: s}, nil
}

type faultWorkspace struct {
	ports.Workspace
	s *faultScratch
}

func (w *faultWorkspace) Create() (ports.ScratchFile, error) {
	if err := w.s.hit("create"); err != nil {
		return nil, err
	}
	f, err := w.Workspace.Create()
	if err != nil {
		return nil, err
	}
	return &faultFile{ScratchFile: f, s: w.s}, nil
}
func (w *faultWorkspace) Remove(f ports.ScratchFile) error {
	err := w.Workspace.Remove(f.(*faultFile).ScratchFile)
	return errors.Join(err, w.s.hit("remove"))
}
func (w *faultWorkspace) Map(ctx context.Context, f ports.ScratchFile) (ports.Mapping, error) {
	if err := w.s.hit("map"); err != nil {
		return nil, err
	}
	b, err := w.Workspace.Map(ctx, f.(*faultFile).ScratchFile)
	if err != nil {
		return nil, err
	}
	w.s.activeMaps++
	return &faultMapping{Mapping: b, s: w.s}, nil
}
func (w *faultWorkspace) Close() error { return errors.Join(w.Workspace.Close(), w.s.hit("close")) }

type faultFile struct {
	ports.ScratchFile
	s *faultScratch
}

func (f *faultFile) Write(b []byte) (int, error) {
	if err := f.s.hit("write"); err != nil {
		return 0, err
	}
	if err := f.s.hit("shortwrite"); err != nil {
		return 0, nil
	}
	return f.ScratchFile.Write(b)
}
func (f *faultFile) ReadAt(b []byte, off int64) (int, error) {
	if err := f.s.hit("read"); err != nil {
		return 0, err
	}
	if err := f.s.hit("truncate"); err != nil {
		return 0, io.EOF
	}
	return f.ScratchFile.ReadAt(b, off)
}

type faultMapping struct {
	ports.Mapping
	s      *faultScratch
	closed bool
}

func (m *faultMapping) Close() error {
	if m.closed {
		return errors.New("mapping closed twice")
	}
	m.closed = true
	m.s.activeMaps--
	return errors.Join(m.Mapping.Close(), m.s.hit("unmap"))
}
func failureCorpus() (memory, memory) {
	var n, e strings.Builder
	n.WriteString("~id,p:String\n")
	e.WriteString("~id,~from,~to,p:String\n")
	for i := 0; i < 4000; i++ {
		fmt.Fprintf(&n, "n%05d,v%05d\n", i, i)
		fmt.Fprintf(&e, "e%05d,n%05d,n00000,v%05d\n", i, i, i)
	}
	return memory{"n": n.String()}, memory{"e": e.String()}
}
func TestExternalScratchFailuresAndCancellation(t *testing.T) {
	n, e := failureCorpus()
	for _, tc := range []struct {
		op string
		at int
	}{
		{"new", 1}, {"create", 1}, {"create", 10}, {"write", 1}, {"write", 20}, {"write", 100},
		{"shortwrite", 1}, {"read", 1}, {"read", 20}, {"truncate", 2},
		{"map", 1}, {"map", 2}, {"map", 4}, {"map", 10}, {"remove", 1}, {"remove", 8}, {"close", 1}, {"unmap", 1},
	} {
		t.Run(fmt.Sprintf("%s-%d", tc.op, tc.at), func(t *testing.T) {
			s := &faultScratch{dir: t.TempDir(), op: tc.op, at: tc.at}
			g, _, err := ingest.Build(context.Background(), n, e, neptune.Decoder{}, &diagnostics{}, ingest.Options{Scratch: s, MemoryBudget: 1 << 20, IndexProperties: []string{"p"}})
			if g != nil {
				_ = g.Close()
				t.Fatal("failure returned graph")
			}
			if err == nil || s.calls[tc.op] < tc.at {
				t.Fatalf("injection not observed %v %+v", err, s.calls)
			}
			if tc.op != "shortwrite" && tc.op != "truncate" && !errors.Is(err, diskFault) {
				t.Fatal(err)
			}
			if s.activeMaps != 0 {
				t.Fatal("mapping leak", s.activeMaps)
			}
			entries, er := os.ReadDir(s.dir)
			if er != nil || len(entries) != 0 {
				t.Fatal("scratch leak", entries, er)
			}
		})
	}
	for _, op := range []string{"write", "read", "map"} {
		t.Run("cancel-"+op, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := &faultScratch{dir: t.TempDir(), op: op, at: 3, cancel: cancel}
			g, _, err := ingest.Build(ctx, n, e, neptune.Decoder{}, &diagnostics{}, ingest.Options{Scratch: s, MemoryBudget: 1 << 20})
			if g != nil || !errors.Is(err, context.Canceled) || s.activeMaps != 0 {
				t.Fatal(g, err, s.activeMaps)
			}
			entries, err := os.ReadDir(s.dir)
			if err != nil || len(entries) != 0 {
				t.Fatal(entries, err)
			}
		})
	}
}
func TestExternalSourceFailuresBarrierAndDiagnosticFailure(t *testing.T) {
	closed := 0
	n := faultCatalog{entries: []ports.Entry{{Key: "n", Regular: true}}, open: func(string) (io.ReadCloser, error) {
		return trackedReader{&brokenTail{data: "~id\nA\nB\n\"tail", err: diskFault}, func() error { closed++; return nil }}, nil
	}}
	e := faultCatalog{entries: []ports.Entry{{Key: "e", Regular: true}}, open: func(string) (io.ReadCloser, error) {
		if closed != 1 {
			t.Fatal("barrier/source close")
		}
		return io.NopCloser(strings.NewReader("~id,~from,~to\ne,A,B\nbad,A,X\n")), nil
	}}
	dir := t.TempDir()
	g, r, err := ingest.Build(context.Background(), n, e, neptune.Decoder{}, &diagnostics{}, ingest.Options{Scratch: filesystem.Scratch{Dir: dir}, MemoryBudget: 1 << 20})
	if err != nil || r.Nodes != 2 || r.Edges != 1 || r.NodeSources.SourcesIOFailed != 1 || r.EdgeSources.RecordsRejected != 1 {
		t.Fatal(r, err)
	}
	if err = g.Close(); err != nil {
		t.Fatal(err)
	}
	diag := &typedFailSink{}
	g, _, err = ingest.Build(context.Background(), memory{"n": "~id,p:Int(single)\nA,1\nA,2\n"}, memory{}, neptune.Decoder{}, diag, ingest.Options{Scratch: filesystem.Scratch{Dir: dir}, MemoryBudget: 1 << 20})
	var diagnosticErr *ingest.DiagnosticError
	if g != nil || !errors.As(err, &diagnosticErr) || diag.calls != 1 {
		t.Fatal(g, err, diag.calls)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal(entries, err)
	}
}
