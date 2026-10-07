package ingest_test

import (
	"bytes"
	"context"
	"fmt"
	"gophergraph/graph"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/filesystem"
	"gophergraph/ingest/adapters/neptune"
	"gophergraph/ingest/ports"
	"gophergraph/internal/testutil"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/file"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func snapshotBytes(t *testing.T, g *graph.Graph) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "graph")
	if _, err := snapshot.Write(context.Background(), g, file.New(path)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func compareBuilds(t *testing.T, n, e memory, keys []string, check func(*graph.Graph)) {
	t.Helper()
	compareBuildsWithOptions(t, n, e, ingest.Options{NodePropertyConflictPolicy: ingest.DropConflictingProperty, IndexProperties: keys}, func(g *graph.Graph, _ ingest.Report, _ []ports.Diagnostic) {
		if check != nil {
			check(g)
		}
	})
}

func compareBuildsWithOptions(t *testing.T, n, e ports.Catalog, base ingest.Options, check func(*graph.Graph, ingest.Report, []ports.Diagnostic)) {
	t.Helper()
	dir := t.TempDir()
	var want []byte
	var wantReport ingest.Report
	var wantDiag *diagnostics
	for _, budget := range []uint64{0, 1 << 20, 2 << 20} {
		options := base
		if budget > 0 {
			options.Scratch = filesystem.Scratch{Dir: dir}
			options.MemoryBudget = budget
		}
		diag := &diagnostics{}
		g, r, err := ingest.Build(context.Background(), n, e, neptune.Decoder{}, diag, options)
		if err != nil {
			t.Fatal(err)
		}
		if check != nil {
			check(g, r, diag.events)
		}
		got := snapshotBytes(t, g)
		if err = g.Close(); err != nil {
			t.Fatal(err)
		}
		r.Times = ingest.PhaseTimes{}
		if budget == 0 {
			want, wantReport, wantDiag = got, r, diag
		} else {
			if !bytes.Equal(got, want) {
				t.Fatalf("snapshot differs at budget %d", budget)
			}
			if !reflect.DeepEqual(r, wantReport) {
				t.Fatalf("report differs: %+v != %+v", r, wantReport)
			}
			if !reflect.DeepEqual(diag, wantDiag) {
				t.Fatalf("diagnostics differ:\ngot %+v\nwant %+v", diag, wantDiag)
			}
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Fatalf("scratch leak %v %v", entries, err)
		}
	}
}
func TestExternalFixtures(t *testing.T) {
	paths, err := filepath.Glob("../fixtures/*/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		dir := filepath.Dir(path)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			want := testutil.Read(t, dir)
			compareBuilds(t, catalog(t, filepath.Join(dir, "nodes")), catalog(t, filepath.Join(dir, "edges")), want.IndexProperties, func(g *graph.Graph) { testutil.CheckGraph(t, g, want); testutil.CheckQueries(t, g, want) })
		})
	}
}
func TestExternalLargeGroupsAndPermutations(t *testing.T) {
	// Sets on a single ID, duplicate contributions and quarantined edges span
	// many sort runs. Buffering one entity/group would defeat the memory bound.
	const size = 12000
	rows := make([]string, 0, size+8)
	for i := 0; i < size; i++ {
		rows = append(rows, fmt.Sprintf("A,L%d,v%06d,same", i%17, i))
	}
	rows = append(rows, "B,L,b,first", "B,L,b,second", "A,L,v000001,same")
	rng := rand.New(rand.NewSource(47))
	rng.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
	n := memory{"n": "~id,~label,p:String,q:String(single)\n" + strings.Join(rows, "\n") + "\n", "card": "~id,p:String(single)\nB,b\n"}
	var e strings.Builder
	e.WriteString("~id,~from,~to,~label,p:String\n")
	for i := 0; i < size; i++ {
		fmt.Fprintf(&e, "e%06d,A,B,L,v%06d\n", i, i)
	}
	e.WriteString("bad,A,A,L,x\nbad,A,B,L,y\nbad,A,missing,L,z\nparallel,A,B,L,v\nloop,B,B,L,v\n")
	compareBuilds(t, n, memory{"e": e.String()}, []string{"p", "absent", "q", "p"}, nil)
	// Changing source order/options must still give exactly the same snapshot.
	var previous []byte
	for seed := int64(0); seed < 3; seed++ {
		rand.New(rand.NewSource(seed)).Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
		n["n"] = "~id,~label,p:String,q:String(single)\n" + strings.Join(rows, "\n") + "\n"
		g, _, err := ingest.Build(context.Background(), n, memory{"e": e.String()}, neptune.Decoder{}, &diagnostics{}, ingest.Options{NodePropertyConflictPolicy: ingest.DropConflictingProperty, Scratch: filesystem.Scratch{Dir: t.TempDir()}, MemoryBudget: 1 << 20, IndexProperties: []string{"q", "p", "absent"}})
		if err != nil {
			t.Fatal(err)
		}
		got := snapshotBytes(t, g)
		if err = g.Close(); err != nil {
			t.Fatal(err)
		}
		if previous != nil && !bytes.Equal(previous, got) {
			t.Fatal("order changed snapshot")
		}
		previous = got
	}
}
func TestExternalOversizedRow(t *testing.T) {
	// A single decoded row may exceed the sort arena: it gets its own disk run.
	text := strings.Repeat("x", 600000)
	n := memory{"n": "~id,p:String\nA," + text + "\nB,short\n"}
	compareBuilds(t, n, memory{"e": "~id,~from,~to\ne,A,B\n"}, []string{"p"}, nil)
}

func TestExternalBudgetValidation(t *testing.T) {
	for _, options := range []ingest.Options{
		{MemoryBudget: 1 << 20},
		{Scratch: filesystem.Scratch{Dir: t.TempDir()}, MemoryBudget: 1},
	} {
		g, _, err := ingest.Build(context.Background(), memory{}, memory{}, neptune.Decoder{}, &diagnostics{}, options)
		if err == nil || g != nil {
			t.Fatal("invalid budget accepted", g, err)
		}
	}
}
