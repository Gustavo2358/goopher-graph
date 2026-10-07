package remote

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gophergraph/ggpb"
	"gophergraph/ingest"
	"gophergraph/ingest/adapters/filesystem"
	"gophergraph/ingest/adapters/neptune"
	"gophergraph/ingest/adapters/stderr"
	"gophergraph/remote/installedwasm"
	"gophergraph/remote/pb"
	"gophergraph/wasmquery"
)

// Exercises the packaged neighbor-restricted filtering through RunWasm.
func TestFilteredSelectionStreaming(t *testing.T) {
	ctx := context.Background()
	for _, indexed := range []bool{false, true} {
		root := "../wasmquery/testdata/selection"
		nodes, err := filesystem.New(filepath.Join(root, "nodes"))
		if err != nil {
			t.Fatal(err)
		}
		edges, err := filesystem.New(filepath.Join(root, "edges"))
		if err != nil {
			t.Fatal(err)
		}
		opts := ingest.Options{}
		if indexed {
			opts.IndexProperties = []string{"tag"}
		}
		g, _, err := ingest.Build(ctx, nodes, edges, neptune.Decoder{}, stderr.New(io.Discard), opts)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { g.Close() })
		r, err := wasmquery.New(ctx, wasmquery.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { r.Close() })
		registry, err := installedwasm.Default(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		descriptor, _, ok := registry.Lookup("filtered")
		if !ok || descriptor.Version != "2" {
			t.Fatal(descriptor)
		}
		service, err := NewService(g, r, registry, &pb.ServerInfoResponse{SnapshotId: "selection-fixture"})
		if err != nil {
			t.Fatal(err)
		}
		server, client, _ := startServer(t, service, ServerOptions{QueryCapacity: 1})
		count := 1
		warmup := 0
		var output *os.File
		if path := os.Getenv("GOPHERGRAPH_SELECTION_TRANSPORT_MEASURE"); path != "" {
			output, err = os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			count = 11
			warmup = 3
		}
		stats := func() QueryStatistics {
			for _, s := range server.QueryStatistics() {
				if s.Name == "wasm:filtered" {
					return s
				}
			}
			t.Fatal("missing metrics")
			return QueryStatistics{}
		}
		for i := -warmup; i < count; i++ {
			before := stats()
			deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
			started := time.Now()
			stream, err := client.RunWasm(deadline, &pb.RunWasmRequest{QueryName: descriptor.Name, ExpectedSha256: &descriptor.SHA256, Args: []string{"S", "L", "tag", "X"}})
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			var decoder ggpb.BatchDecoder
			var bytes, batches uint64
			var decode time.Duration
			for {
				message, err := stream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					cancel()
					t.Fatal(err)
				}
				bytes += uint64(len(message.Ggpb))
				batches++
				startedDecode := time.Now()
				_, err = decoder.Decode(deadline, message.Ggpb)
				decode += time.Since(startedDecode)
				if err != nil {
					cancel()
					t.Fatal(err)
				}
			}
			elapsed := time.Since(started)
			if err := decoder.Finish(); err != nil {
				cancel()
				t.Fatal(err)
			}
			cancel()
			after := stats()
			if after.Nodes-before.Nodes != 2 || after.Edges-before.Edges != 1 || r.Stats().ActiveHandles != 0 {
				t.Fatal(after, r.Stats())
			}
			if output != nil && i >= 0 {
				row := struct {
					Indexed                                                                  bool
					Sample                                                                   int
					Nodes, Edges, Bytes, Batches                                             uint64
					EndToEndMS, ExecutionMaterializationMS, EncodeMS, SendMS, ClientDecodeMS float64
				}{indexed, i, 2, 1, bytes, batches, float64(elapsed) / 1e6, (after.Execution - before.Execution) * 1000, (after.Encode - before.Encode) * 1000, (after.Send - before.Send) * 1000, float64(decode) / 1e6}
				if err := json.NewEncoder(output).Encode(row); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
