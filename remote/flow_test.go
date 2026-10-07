package remote

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gophergraph/ggpb"
	"gophergraph/internal/benchfixture"
	"gophergraph/remote/installedwasm"
	"gophergraph/remote/pb"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/file"
	"gophergraph/snapshot/adapters/mmap"
	"gophergraph/wasmquery"
)

// Fixed topology/set sizes, varying only property bytes. Published snapshot
// interns this value once; emitted results repeat it for every node/edge.
func flowService(t testing.TB, scalar int) *Service {
	t.Helper()
	ctx := context.Background()
	heap, err := benchfixture.StreamingGraph(ctx, 4000, scalar)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "flow.snapshot")
	if _, err = snapshot.Write(ctx, heap, file.New(path)); err != nil {
		t.Fatal(err)
	}
	heap.Close()
	source, err := mmap.NewResident(path, mmap.Locked)
	if err != nil {
		t.Fatal(err)
	}
	g, err := snapshot.Open(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	report, err := source.Prepare(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r, err := wasmquery.New(ctx, wasmquery.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	reg, err := installedwasm.Default(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewService(g, r, reg, &pb.ServerInfoResponse{SnapshotId: report.SnapshotID, ResidencyMode: "locked", Locked: true, WarmCompleted: true, LockedBytes: report.LockedBytes})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func waitAdmission(t testing.TB, s *Server, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s.Admission().Active == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("admission did not settle", s.Admission())
}
func TestSlowConsumerBoundedMemoryAndOverload(t *testing.T) {
	for _, scalar := range []int{2048, 32768} {
		t.Run(fmt.Sprint(scalar), func(t *testing.T) {
			s := flowService(t, scalar)
			server, client, _ := startServer(t, s, ServerOptions{QueryCapacity: 1})
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			stream, err := client.Territory(ctx, &pb.TerritoryRequest{Node: new("n000000000")})
			if err != nil {
				t.Fatal(err)
			}
			first, err := stream.Recv()
			if err != nil || len(first.Ggpb) == 0 {
				t.Fatal(err)
			}
			// Stop application consumption. Actual HTTP/2 flow control must keep
			// the query/token active and prevent result-wide buffering.
			time.Sleep(300 * time.Millisecond)
			runtime.GC()
			runtime.ReadMemStats(&after)
			if server.Admission().Active != 1 {
				t.Fatal("producer ignored backpressure", server.Admission())
			}
			growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			t.Logf("logical property bytes=%d paused live heap growth=%d", 8000*scalar, growth)
			if growth > 24<<20 {
				t.Fatal("buffering proportional to result", growth)
			}
			rejected, err := client.Territory(ctx, &pb.TerritoryRequest{Node: new("n000000000")})
			if err == nil {
				_, err = rejected.Recv()
			}
			if status.Code(err) != codes.ResourceExhausted {
				t.Fatal(err)
			}
			cancel()
			waitAdmission(t, server, 0)
		})
	}
}
func TestBlockedSendDeadlineAndShutdown(t *testing.T) {
	s := flowService(t, 32768)
	server, client, _ := startServer(t, s, ServerOptions{QueryCapacity: 1})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	st, err := client.Territory(ctx, &pb.TerritoryRequest{Node: new("n000000000")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.Recv(); err != nil {
		t.Fatal(err)
	}
	<-ctx.Done()
	waitAdmission(t, server, 0)
	long, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	st, err = client.Territory(long, &pb.TerritoryRequest{Node: new("n000000000")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.Recv(); err != nil {
		t.Fatal(err)
	}
	grace, end := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer end()
	started := time.Now()
	if err = server.Shutdown(grace); err != context.DeadlineExceeded {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second || server.Admission().Active != 0 {
		t.Fatal("did not cancel and join")
	}
	if s.runtime.Stats().ActiveHandles != 0 {
		t.Fatal("WASM leaked handles")
	}
}
func TestConcurrentQueriesSharedLockedGraph(t *testing.T) {
	s := flowService(t, 64)
	server, client, _ := startServer(t, s, ServerOptions{QueryCapacity: 8})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			var st grpc.ServerStreamingClient[pb.EncodedBatch]
			var err error
			if i%2 == 0 {
				st, err = client.Territory(ctx, &pb.TerritoryRequest{Node: new("n000000000")})
			} else {
				st, err = client.RunWasm(ctx, &pb.RunWasmRequest{QueryName: "shared-targets", Args: []string{"n000000000", "n000000000"}})
			}
			if err != nil {
				t.Error(err)
				return
			}
			batches := receive(t, st)
			var decoder ggpb.BatchDecoder
			h, e := decoder.Decode(ctx, batches[0])
			if e != nil || h.GetHeader().Nodes != 4000 || h.GetHeader().Edges != 4000 {
				t.Error("incorrect shared graph result", e)
			}
			if len(batches) < 3 {
				t.Error("missing records")
			}
		})
	}
	wg.Wait()
	waitAdmission(t, server, 0)
	if stats := s.runtime.Stats(); stats.Compilations != 3 || stats.ActiveHandles != 0 {
		t.Fatal(stats)
	}
}
