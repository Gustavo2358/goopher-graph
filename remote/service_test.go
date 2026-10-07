package remote

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gophergraph/ggpb"
	"gophergraph/graph"
	"gophergraph/query"
	"gophergraph/remote/installedwasm"
	"gophergraph/remote/pb"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"gophergraph/wasmquery"
)

func serviceFixture(t testing.TB) (*Service, *graph.Graph) {
	t.Helper()
	ctx := context.Background()
	g, err := snapshot.Open(ctx, mmap.New("../fixtures/snapshot_reference/topology.snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { g.Close() })
	r, err := wasmquery.New(ctx, wasmquery.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	reg, err := installedwasm.Default(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewService(g, r, reg, &pb.ServerInfoResponse{SnapshotId: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	return s, g
}
func connectService(t testing.TB, s *Service, options ...grpc.ServerOption) pb.GraphServiceClient {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(options...)
	pb.RegisterGraphServiceServer(server, s)
	go server.Serve(l)
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(l.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(ggpb.MaxFrame+16)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return pb.NewGraphServiceClient(conn)
}
func receive(t testing.TB, stream grpc.ServerStreamingClient[pb.EncodedBatch]) [][]byte {
	t.Helper()
	var out [][]byte
	var decoder ggpb.BatchDecoder
	for {
		b, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err = decoder.Decode(stream.Context(), b.Ggpb); err != nil {
			t.Fatal(err)
		}
		out = append(out, b.Ggpb)
	}
	if err := decoder.Finish(); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestNativeStreamingParity(t *testing.T) {
	s, g := serviceFixture(t)
	client := connectService(t, s)
	first, _ := g.NodeExternalID(0)
	last, _ := g.NodeExternalID(graph.NodeID(g.Metadata().Nodes - 1))
	for _, filter := range []*pb.EdgeLabelFilter{nil, {}, {Labels: []string{"absent"}}} {
		for _, name := range []string{"territory", "anti-territory", "between"} {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			var stream grpc.ServerStreamingClient[pb.EncodedBatch]
			var err error
			var result *query.Subgraph
			o, labels, _ := s.options(filter)
			a, _ := g.FindNode(first)
			b, _ := g.FindNode(last)
			meta := ggpb.Query{Name: name, Node: &first, EdgeLabels: labels}
			switch name {
			case "territory":
				stream, err = client.Territory(ctx, &pb.TerritoryRequest{Node: &first, EdgeFilter: filter})
				result, _ = query.Territory(ctx, g, a, o)
			case "anti-territory":
				stream, err = client.AntiTerritory(ctx, &pb.AntiTerritoryRequest{Node: &first, EdgeFilter: filter})
				result, _ = query.AntiTerritory(ctx, g, a, o)
			case "between":
				stream, err = client.Between(ctx, &pb.BetweenRequest{From: &first, To: &last, EdgeFilter: filter})
				result, _ = query.Between(ctx, g, a, b, o)
				meta.Node = nil
				meta.From = &first
				meta.To = &last
			}
			if err != nil {
				t.Fatal(err)
			}
			got := receive(t, stream)
			var want [][]byte
			if err = ggpb.EmitEncoded(ctx, g, result, meta, ggpb.Options{}, func(b []byte) error { want = append(want, bytes.Clone(b)); return nil }); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(want) {
				t.Fatal(name, "batch count")
			}
			for i := range got {
				if !bytes.Equal(got[i], want[i]) {
					t.Fatal(name, "wire mismatch", i)
				}
			}
			cancel()
		}
	}
	for _, req := range []*pb.TerritoryRequest{{}, {Node: new("unknown")}} {
		stream, err := client.Territory(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		_, err = stream.Recv()
		if code := status.Code(err); code != codes.InvalidArgument && code != codes.NotFound {
			t.Fatal(err)
		}
	}
}
