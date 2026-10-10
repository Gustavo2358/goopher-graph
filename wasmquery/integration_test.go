package wasmquery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/experimental/sock"
	sharedtargets "gophergraph/examples/shared_targets"
	"gophergraph/graph"
	"gophergraph/graphjson"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/filesystem"
	"gophergraph/ingest/adapters/neptune"
	"gophergraph/ingest/adapters/stderr"
	"gophergraph/internal/benchfixture"
	"gophergraph/internal/testutil"
	"gophergraph/query"
	"gophergraph/snapshot"
	snapshotfile "gophergraph/snapshot/adapters/file"
	"gophergraph/snapshot/adapters/mmap"
	"gophergraph/wasmquery/internal/abi"
)

func buildGuest(t *testing.T, path string) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "query.wasm")
	cmd := exec.Command("go", "build", "-o", out, path)
	cmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0", "GOPROXY=off", "GOTOOLCHAIN=local")
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build guest: %v\n%s", err, data)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func newRuntime(t *testing.T, l Limits) *Runtime {
	t.Helper()
	r, err := New(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	})
	return r
}
func compileGuest(t *testing.T, r *Runtime, path string) *Module {
	t.Helper()
	m, err := r.Compile(context.Background(), buildGuest(t, path))
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func loadGraph(t *testing.T, path string, index bool, keys ...string) *graph.Graph {
	t.Helper()
	o := ingest.Options{}
	if index {
		o.IndexProperties = []string{"sigla", "line", "rank", "group"}
		if len(keys) != 0 {
			o.IndexProperties = keys
		}
	}
	nodes, err := filesystem.New(filepath.Join(path, "nodes"))
	if err != nil {
		t.Fatal(err)
	}
	edges, err := filesystem.New(filepath.Join(path, "edges"))
	if err != nil {
		t.Fatal(err)
	}
	g, _, err := ingest.Build(context.Background(), nodes, edges, neptune.Decoder{}, stderr.New(io.Discard), o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	return g
}
func execute(t *testing.T, r *Runtime, m *Module, g *graph.Graph, args ...string) *Result {
	t.Helper()
	result, err := r.Execute(context.Background(), m, g, args)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	t.Cleanup(func() { result.Close() })
	return result
}
func sameSubgraph(t *testing.T, g *graph.Graph, a, b *query.Subgraph) {
	t.Helper()
	var left, right bytes.Buffer
	if err := graphjson.Write(context.Background(), &left, g, a, graphjson.Query{Name: "comparison"}); err != nil {
		t.Fatal(err)
	}
	if err := graphjson.Write(context.Background(), &right, g, b, graphjson.Query{Name: "comparison"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left.Bytes(), right.Bytes()) {
		t.Fatalf("subgraphs differ:\n%s\n%s", &left, &right)
	}
}
func assertClean(t *testing.T, r *Runtime) {
	t.Helper()
	stats := r.Stats()
	if stats.ActiveExecutions != 0 || stats.ActiveHandles != 0 {
		t.Fatalf("retained execution state: %+v", stats)
	}
}
func TestExamplesAgainstNativeQueries(t *testing.T) {
	ctx := context.Background()
	r := newRuntime(t, Limits{})
	g := loadGraph(t, "../fixtures/01_topology", true)
	between := compileGuest(t, r, "../examples/wasm/between")
	shared := compileGuest(t, r, "../examples/wasm/shared_targets")
	filtered := compileGuest(t, r, "../examples/wasm/filtered")
	a, _ := g.FindNode("A")
	f, _ := g.FindNode("F")
	want, err := query.Between(ctx, g, a, f, query.Options{})
	if err != nil {
		t.Fatal(err)
	}
	sub, _ := execute(t, r, between, g, "A", "F").Subgraph()
	sameSubgraph(t, g, sub, want)
	ns, err := sharedtargets.Execute(ctx, g, "A", "B")
	if err != nil {
		t.Fatal(err)
	}
	want, err = query.FromNodes(ctx, g, ns, query.Options{})
	if err != nil {
		t.Fatal(err)
	}
	sub, _ = execute(t, r, shared, g, "A", "B").Subgraph()
	sameSubgraph(t, g, sub, want)
	want, _ = query.NewSubgraph(g)
	for _, id := range []string{"e1", "e5", "e7"} {
		e, _ := g.FindEdge(id)
		if err = want.AddEdge(e); err != nil {
			t.Fatal(err)
		}
	}
	sub, _ = execute(t, r, filtered, g, "A", "PROGRAM", "sigla", "CO").Subgraph()
	sameSubgraph(t, g, sub, want)
	assertClean(t, r)
}
func TestGoGuestABIAndFailureCleanup(t *testing.T) {
	r := newRuntime(t, Limits{})
	g := loadGraph(t, "../fixtures/01_topology", false)
	m := compileGuest(t, r, "./testdata/guest")
	good := execute(t, r, m, g, "coverage")
	sub, _ := good.Subgraph()
	id, _ := g.FindNode("A")
	want, err := query.Territory(context.Background(), g, id, query.Options{})
	if err != nil {
		t.Fatal(err)
	}
	sameSubgraph(t, g, sub, want)
	execute(t, r, m, g, "args", "", "line\n☃", "--flag")
	execute(t, r, m, g, "release")
	for _, tc := range []struct {
		mode string
		err  error
	}{
		{"no-result", ErrResult}, {"invalid", ErrHandle}, {"wrong-kind", ErrHandle}, {"stale", ErrHandle},
		{"unknown", nil}, {"malformed", ErrABI}, {"oversize", ErrLimit}, {"short-output", ErrABI},
		{"double-return", ErrResult}, {"handles", ErrLimit}, {"calls", ErrLimit}, {"exit", nil}, {"trap", nil},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			out, err := r.Execute(context.Background(), m, g, []string{tc.mode})
			if out != nil || err == nil || (tc.err != nil && !errors.Is(err, tc.err)) {
				t.Fatalf("out=%v error=%v want=%v", out, err, tc.err)
			}
			assertClean(t, r)
		})
	}
	// Global tokens from a previous run are never valid in the next execution.
	token := tokens.Load()
	_, err = r.Execute(context.Background(), m, g, []string{"foreign", strconv.FormatUint(token, 10)})
	if !errors.Is(err, ErrHandle) {
		t.Fatal(err)
	}
	execute(t, r, m, g, "identity", "I")
	for _, args := range [][]string{{"args", "bad\x00arg"}, {string(make([]byte, 65537))}} {
		if _, err = r.Execute(context.Background(), m, g, args); err == nil {
			t.Fatal("accepted bad args")
		}
	}
	if err = good.Close(); err != nil {
		t.Fatal(err)
	}
	good.Close()
	if _, err = good.Subgraph(); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	assertClean(t, r)
}
func TestSandboxAndFreshInstances(t *testing.T) {
	r := newRuntime(t, Limits{})
	g := loadGraph(t, "../fixtures/01_topology", false)
	m := compileGuest(t, r, "./testdata/guest")
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("host secret"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOPHERGRAPH_SECRET", "host environment")
	for range 3 {
		execute(t, r, m, g, "sandbox", secret)
	}
	// Wazero can inherit preopened sockets through context values. The capability
	// boundary must strip that opt-in, while retaining context cancellation.
	ctx := sock.WithConfig(context.Background(), sock.NewConfig().WithTCPListener("127.0.0.1", -1))
	res, err := r.Execute(ctx, m, g, []string{"sandbox", secret})
	if err != nil {
		t.Fatal("inherited ambient WASI capability", err)
	}
	res.Close()

	if _, err := os.Stat(secret + ".written"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("guest wrote host file", err)
	}
	assertClean(t, r)
}
func TestCacheConcurrencyAndResultLifetime(t *testing.T) {
	r := newRuntime(t, Limits{Concurrent: 8})
	g := loadGraph(t, "../fixtures/01_topology", false)
	data := buildGuest(t, "./testdata/guest")
	const workers = 8
	var wg sync.WaitGroup
	mods := make([]*Module, workers)
	errs := make(chan error, workers)
	for i := range workers {
		wg.Go(func() {
			var err error
			mods[i], err = r.Compile(context.Background(), append([]byte(nil), data...))
			if err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	for i := 1; i < workers; i++ {
		if mods[i] != mods[0] {
			t.Fatal("duplicate compiled modules")
		}
	}
	if r.Stats().Compilations != 1 {
		t.Fatal(r.Stats())
	}
	results := make([]*Result, workers)
	for i := range workers {
		wg.Go(func() {
			arg := []string{"A", "I"}[i%2]
			var err error
			results[i], err = r.Execute(context.Background(), mods[i], g, []string{"identity", arg})
			if err != nil {
				errs <- err
				return
			}
			sub, _ := results[i].Subgraph()
			it := sub.Nodes()
			if !it.Next() {
				errs <- errors.New("empty result")
				return
			}
			name, _ := g.NodeExternalID(it.ID())
			if name != arg {
				errs <- fmt.Errorf("state crossed executions: %s != %s", name, arg)
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	assertClean(t, r)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	for _, res := range results {
		if res != nil {
			if _, err := res.Subgraph(); err != nil {
				t.Fatal("result invalidated by runtime close", err)
			}
			res.Close()
		}
	}
	if r.Stats().CachedModules != 0 {
		t.Fatal(r.Stats())
	}
	if _, err := r.Execute(context.Background(), mods[0], g, nil); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}
func TestCancellationLimitsAndClose(t *testing.T) {
	data := buildGuest(t, "./testdata/guest")
	g := loadGraph(t, "../fixtures/01_topology", false)
	r := newRuntime(t, Limits{Timeout: time.Second, Concurrent: 1})
	m, err := r.Compile(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = r.Execute(ctx, m, g, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err = r.Execute(ctx, m, g, []string{"spin"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("cancellation did not interrupt guest")
	}
	assertClean(t, r)
	done := make(chan error, 1)
	go func() { _, err := r.Execute(context.Background(), m, g, []string{"spin"}); done <- err }()
	deadline := time.Now().Add(time.Second)
	for r.Stats().ActiveExecutions == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if r.Stats().ActiveExecutions != 1 {
		t.Fatal("guest did not start")
	}
	if _, err = r.Execute(context.Background(), m, g, []string{"identity", "A"}); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	assertClean(t, r)
	for _, tc := range []struct {
		name string
		l    Limits
		mode string
	}{
		{"host", Limits{HostBytes: 1}, "identity"}, {"handles", Limits{Handles: 1}, "identity"},
		{"calls", Limits{Calls: 1}, "identity"}, {"memory", Limits{MemoryPages: 512}, "growth"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRuntime(t, tc.l)
			m, err := r.Compile(context.Background(), data)
			if err != nil {
				t.Fatal(err)
			}
			if result, err := r.Execute(context.Background(), m, g, []string{tc.mode, "A"}); result != nil || err == nil {
				t.Fatal(result, err)
			}
			assertClean(t, r)
		})
	}
	for _, l := range []Limits{{MemoryPages: 65537}, {Concurrent: -1}, {Timeout: -1}} {
		if r, err := New(context.Background(), l); err == nil {
			r.Close()
			t.Fatal("invalid limits accepted")
		}
	}
	r2 := newRuntime(t, Limits{CachedModules: 1})
	if _, err = r2.Compile(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	other := buildGuest(t, "../examples/wasm/between")
	if _, err = r2.Compile(context.Background(), other); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	r3 := newRuntime(t, Limits{ModuleBytes: 1})
	if _, err = r3.Compile(context.Background(), data); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	r4 := newRuntime(t, Limits{CacheBytes: 1})
	if _, err = r4.Compile(context.Background(), data); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if _, err = r2.Execute(context.Background(), m, g, nil); !errors.Is(err, ErrABI) {
		t.Fatal(err)
	}
}
func TestNativeParityFixturesAndMmap(t *testing.T) {
	r := newRuntime(t, Limits{})
	m := compileGuest(t, r, "./testdata/guest")
	ctx := context.Background()
	paths, err := filepath.Glob("../fixtures/*/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		dir := filepath.Dir(path)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			model := testutil.Read(t, dir)
			heap := loadGraph(t, dir, false)
			snapshotPath := filepath.Join(t.TempDir(), "graph.snapshot")
			if _, err := snapshot.Write(ctx, heap, snapshotfile.New(snapshotPath)); err != nil {
				t.Fatal(err)
			}
			mapped, err := snapshot.Open(ctx, mmap.New(snapshotPath))
			if err != nil {
				t.Fatal(err)
			}
			defer mapped.Close()
			for _, g := range []*graph.Graph{heap, mapped} {
				for _, node := range model.Graph.Nodes {
					id, _ := g.FindNode(node.ID)
					for _, direction := range []graph.Direction{graph.Forward, graph.Reverse} {
						for _, labels := range [][]string{nil, {}, {"CALLS"}, {"missing"}} {
							opts := query.Options{}
							if labels != nil {
								opts.EdgeLabels = []graph.StringID{}
								for _, l := range labels {
									if id, ok, _ := g.FindString(l); ok {
										opts.EdgeLabels = append(opts.EdgeLabels, id)
									}
								}
							}
							n, err := query.Reachable(ctx, g, id, direction, opts)
							if err != nil {
								t.Fatal(err)
							}
							want, err := query.FromNodes(ctx, g, n, opts)
							if err != nil {
								t.Fatal(err)
							}
							b, _ := json.Marshal(labels)
							res := execute(t, r, m, g, "native", node.ID, strconv.Itoa(int(direction)), string(b))
							sub, _ := res.Subgraph()
							sameSubgraph(t, g, sub, want)
							res.Close()
						}
					}
				}
			}
		})
	}
	assertClean(t, r)
}
func TestLargeSetsStayOnHost(t *testing.T) {
	r := newRuntime(t, Limits{})
	m := compileGuest(t, r, "./testdata/guest")
	var metrics []Metrics
	for _, size := range []int{100, 100000} {
		root := t.TempDir()
		if err := benchfixture.Write(root, size); err != nil {
			t.Fatal(err)
		}
		g := loadGraph(t, root, false)
		res := execute(t, r, m, g, "large")
		sub, _ := res.Subgraph()
		if sub.NodeCount() != uint64(size*9/10) || sub.EdgeCount() != uint64(size*9/10*5) {
			t.Fatal(sub.NodeCount(), sub.EdgeCount())
		}
		metrics = append(metrics, res.Metrics())
		res.Close()
	}
	if metrics[0].HostCalls != metrics[1].HostCalls || metrics[0].RequestBytes != metrics[1].RequestBytes || metrics[0].ResponseBytes != metrics[1].ResponseBytes {
		t.Fatal("set size changed ABI traffic", metrics)
	}
	if metrics[1].RequestBytes > 1024 || metrics[1].ResponseBytes != 0 || metrics[1].HostCalls > 12 {
		t.Fatal(metrics)
	}
	if reflect.DeepEqual(metrics[0], metrics[1]) {
		t.Fatal("host storage accounting did not grow")
	}
	t.Logf("100 vs 100000 nodes: %+v", metrics)
	assertClean(t, r)
}

func TestPropertiesAgainstNativeAPI(t *testing.T) {
	r := newRuntime(t, Limits{})
	m := compileGuest(t, r, "./testdata/guest")
	ctx := context.Background()
	paths, _ := filepath.Glob("../fixtures/*/expected.json")
	kinds := map[graph.ValueKind]bool{}
	for _, path := range paths {
		dir := filepath.Dir(path)
		for _, indexed := range []bool{false, true} {
			g := loadGraph(t, dir, indexed)
			for i := uint64(0); i < g.Metadata().Nodes; i++ {
				id := graph.NodeID(i)
				name, _ := g.NodeExternalID(id)
				it, _ := g.NodeProperties(id)
				indices := map[graph.StringID]int{}
				for it.Next() {
					p := it.Property()
					key, _ := g.String(p.Key)
					value, _ := json.Marshal(encodeValue(p.Value))
					index := indices[p.Key]
					indices[p.Key]++
					kinds[p.Value.Kind()] = true
					res := execute(t, r, m, g, "property", "node", key, string(value), name, strconv.Itoa(index))
					sub, _ := res.Subgraph()
					filter, err := g.NodesWithProperty(ctx, key, p.Value)
					if err != nil {
						t.Fatal(err)
					}
					one, _ := graph.NewNodeSet(g)
					one.Add(id)
					n, err := one.Intersection(filter)
					if err != nil {
						t.Fatal(err)
					}
					want, err := query.FromNodes(ctx, g, n, query.Options{})
					if err != nil {
						t.Fatal(err)
					}
					sameSubgraph(t, g, sub, want)
					res.Close()
				}
			}
			for i := uint64(0); i < g.Metadata().Edges; i++ {
				id := graph.EdgeID(i)
				name, _ := g.EdgeExternalID(id)
				edge, _ := g.Edge(id)
				source, _ := g.NodeExternalID(edge.Source)
				it, _ := g.EdgeProperties(id)
				for it.Next() {
					p := it.Property()
					key, _ := g.String(p.Key)
					value, _ := json.Marshal(encodeValue(p.Value))
					kinds[p.Value.Kind()] = true
					res := execute(t, r, m, g, "property", "edge", key, string(value), name, source)
					sub, _ := res.Subgraph()
					filter, err := g.EdgesWithProperty(ctx, key, p.Value)
					if err != nil {
						t.Fatal(err)
					}
					want, _ := query.NewSubgraph(g)
					adj, _ := g.Adjacent(edge.Source, graph.Forward)
					for adj.Next() {
						if filter.Contains(adj.Edge().ID) {
							want.AddEdge(adj.Edge().ID)
						}
					}
					sameSubgraph(t, g, sub, want)
					res.Close()
				}
			}
		}
	}
	if len(kinds) != 10 {
		t.Fatal("not all typed values exercised", kinds)
	}
	assertClean(t, r)
}

type pollCancel struct {
	context.Context
	polls int
}

func (c *pollCancel) Err() error {
	c.polls++
	if c.polls >= 20 {
		return context.Canceled
	}
	return nil
}
func TestCancellationInsideHostTraversal(t *testing.T) {
	root := t.TempDir()
	if err := benchfixture.Write(root, 1000); err != nil {
		t.Fatal(err)
	}
	g := loadGraph(t, root, false)
	r := newRuntime(t, Limits{})
	s := &execution{r: r, g: g, handles: map[uint64]object{}}
	h, err := s.dispatch(context.Background(), abi.Nodes, 0, 0, abi.Params{IDs: []string{"n000000000"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range []uint32{abi.OutE, abi.Reachable, abi.Induced} {
		input := h
		if op == abi.Induced {
			input, err = s.dispatch(context.Background(), abi.Reachable, h, 0, abi.Params{})
			if err != nil {
				t.Fatal(err)
			}
		}
		ctx := &pollCancel{Context: context.Background()}
		before := len(s.handles)
		if _, err = s.dispatch(ctx, op, input, 0, abi.Params{}); !errors.Is(err, context.Canceled) {
			t.Fatal(op, err)
		}
		if len(s.handles) != before {
			t.Fatal("partial result registered")
		}
	}
	s.clear()
	assertClean(t, r)
}
