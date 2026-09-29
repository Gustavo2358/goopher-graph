package snapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gophergraph/graph"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/filesystem"
	"gophergraph/ingest/adapters/neptune"
	ingestports "gophergraph/ingest/ports"
	"gophergraph/internal/testutil"
	"gophergraph/snapshot/ports"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type backing struct {
	data     []byte
	closes   int
	closeErr error
}

func (b *backing) Bytes() []byte { return b.data }
func (b *backing) Close() error  { b.closes++; return b.closeErr }

type source struct {
	b   *backing
	err error
}

func (s source) Acquire(context.Context) (ports.Backing, error) { return s.b, s.err }

type transaction struct {
	bytes.Buffer
	maxWrite                               int
	writeErr, sealErr, commitErr, closeErr error
	publication                            ports.Publication
	sealed                                 *backing
	commits, aborts                        int
}

func (t *transaction) Write(b []byte) (int, error) {
	if t.writeErr != nil {
		return 0, t.writeErr
	}
	if t.maxWrite > 0 && len(b) > t.maxWrite {
		b = b[:t.maxWrite]
	}
	return t.Buffer.Write(b)
}
func (t *transaction) Seal(context.Context) (ports.Backing, error) {
	if t.sealErr != nil {
		return nil, t.sealErr
	}
	t.sealed = &backing{data: t.Buffer.Bytes(), closeErr: t.closeErr}
	return t.sealed, nil
}
func (t *transaction) Commit(context.Context) (ports.Publication, error) {
	t.commits++
	return t.publication, t.commitErr
}
func (t *transaction) Abort() error { t.aborts++; return nil }

type sink struct {
	t   *transaction
	err error
}

func (s sink) Begin(context.Context, uint64) (ports.Transaction, error) { return s.t, s.err }

type discard struct{}

func (discard) Emit(context.Context, ingestports.Diagnostic) error { return nil }
func build(t testing.TB, dir string) *graph.Graph {
	t.Helper()
	w := testutil.Read(t, dir)
	n, e := filesystem.New(filepath.Join(dir, "nodes"))
	if e != nil {
		t.Fatal(e)
	}
	edges, e := filesystem.New(filepath.Join(dir, "edges"))
	if e != nil {
		t.Fatal(e)
	}
	g, _, e := ingest.Build(context.Background(), n, edges, neptune.Decoder{}, discard{}, ingest.Options{IndexProperties: w.IndexProperties})
	if e != nil {
		t.Fatal(e)
	}
	return g
}
func TestIndependentGoldens(t *testing.T) {
	base := "../fixtures/snapshot_reference"
	b, e := os.ReadFile(filepath.Join(base, "cases.json"))
	if e != nil {
		t.Fatal(e)
	}
	var cases []struct {
		Snapshot, Expected string
		Bytes              int
	}
	if e = json.Unmarshal(b, &cases); e != nil {
		t.Fatal(e)
	}
	for _, c := range cases {
		t.Run(c.Snapshot, func(t *testing.T) {
			dir := filepath.Dir(filepath.Join(base, c.Expected))
			w := testutil.Read(t, dir)
			want, e := os.ReadFile(filepath.Join(base, c.Snapshot))
			if e != nil {
				t.Fatal(e)
			}
			back := &backing{data: want}
			g, e := Open(context.Background(), source{b: back})
			if e != nil {
				t.Fatal(e)
			}
			testutil.CheckGraph(t, g, w)
			testutil.CheckQueries(t, g, w)
			_ = g.Close()
			_ = g.Close()
			if back.closes != 1 {
				t.Fatal(back.closes)
			}
			heap := build(t, dir)
			tx := &transaction{maxWrite: 7, publication: ports.PublishedDurable}
			pub, e := Write(context.Background(), heap, sink{t: tx})
			if e != nil || pub != ports.PublishedDurable {
				t.Fatal(pub, e)
			}
			if !bytes.Equal(want, tx.Buffer.Bytes()) {
				a := tx.Buffer.Bytes()
				for i := 0; i < min(len(a), len(want)); i++ {
					if a[i] != want[i] {
						t.Fatalf("byte %d: got %x want %x; lengths %d/%d", i, a[i], want[i], len(a), len(want))
					}
				}
				t.Fatal("lengths", len(a), len(want))
			}
			if tx.sealed.closes != 1 || tx.commits != 1 {
				t.Fatal("seal lifecycle")
			}
		})
	}
}
func TestFailuresCloseAndAbort(t *testing.T) {
	b := &backing{data: []byte("bad")}
	if g, e := Open(context.Background(), source{b: b}); g != nil || !errors.Is(e, ErrCorruptSnapshot) || b.closes != 1 {
		t.Fatal(g, e, b.closes)
	}
	g := build(t, "../fixtures/01_topology")
	boom := errors.New("failure")
	for _, tx := range []*transaction{{writeErr: boom}, {sealErr: boom}, {closeErr: boom}} {
		p, e := Write(context.Background(), g, sink{t: tx})
		if p != ports.NotPublished || e == nil || tx.commits != 0 || tx.aborts != 1 {
			t.Fatal(p, e, tx)
		}
	}
	tx := &transaction{publication: ports.PublishedUncertain, commitErr: boom}
	p, e := Write(context.Background(), g, sink{t: tx})
	if p != ports.PublishedUncertain || !errors.Is(e, boom) {
		t.Fatal(p, e)
	}
}
func FuzzSnapshotDecode(f *testing.F) {
	for _, name := range []string{"empty", "topology", "typed"} {
		b, e := os.ReadFile("../fixtures/snapshot_reference/" + name + ".snapshot")
		if e != nil {
			f.Fatal(e)
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		b := &backing{data: data}
		g, e := Open(context.Background(), source{b: b})
		if e == nil {
			_ = g.Close()
		}
		if b.closes != 1 {
			t.Fatal("owner leak", b.closes)
		}
	})
}
func TestIndependentPythonReader(t *testing.T) {
	g := build(t, "../fixtures/03_typed_values")
	tx := &transaction{publication: ports.PublishedDurable}
	if _, e := Write(context.Background(), g, sink{t: tx}); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "graph.snapshot")
	if e := os.WriteFile(path, tx.Buffer.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
	out, e := exec.Command("python3", "../tools/reference_snapshot.py", "inspect", path).CombinedOutput()
	if e != nil {
		t.Fatalf("independent reader: %v\n%s", e, out)
	}
	if !json.Valid(out) {
		t.Fatal("invalid oracle output")
	}
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }
func TestNoProgressWrite(t *testing.T) {
	if n, e := (fullWriter{zeroWriter{}}).Write([]byte("data")); n != 0 || !errors.Is(e, io.ErrShortWrite) {
		t.Fatal(n, e)
	}
}
