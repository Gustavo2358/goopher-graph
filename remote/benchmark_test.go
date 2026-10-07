package remote

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"gophergraph/ggpb"
	resultpb "gophergraph/ggpb/pb"
	"gophergraph/query"
	"gophergraph/remote/installedwasm"
	"gophergraph/remote/pb"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"gophergraph/wasmquery"
	"io"
	"os"
	"testing"
	"time"
)

type generatedService struct{ *Service }

func (s generatedService) Territory(req *pb.TerritoryRequest, stream grpc.ServerStreamingServer[pb.EncodedBatch]) error {
	ctx := stream.Context()
	id, err := s.g.FindNode(*req.Node)
	if err != nil {
		return err
	}
	sub, err := query.Territory(ctx, s.g, id, query.Options{})
	if err != nil {
		return err
	}
	return ggpb.Emit(ctx, s.g, sub, ggpb.Query{Name: "territory", Node: req.Node}, ggpb.Options{}, func(batch *resultpb.Batch) error {
		bytes, e := proto.Marshal(batch)
		if e != nil {
			return e
		}
		return stream.Send(&pb.EncodedBatch{Ggpb: bytes})
	})
}

// Isolates codec paths on the same resident graph and logical results. The
// TCP cases include query/transport but not client GGPB decoding. Sink cases
// exclude query. Production still exposes only the optimized path.
func BenchmarkEncodedTransport(b *testing.B) {
	for _, scalar := range []int{64, 2048} {
		b.Run(fmt.Sprint(scalar), func(b *testing.B) {
			s := flowService(b, scalar)
			benchmarkTransports(b, s)
		})
	}
}

func benchmarkTransports(b *testing.B, s *Service) {
	ctx := context.Background()
	id, _ := s.g.FindNode("n000000000")
	sub, err := query.Territory(ctx, s.g, id, query.Options{})
	if err != nil {
		b.Fatal(err)
	}
	meta := ggpb.Query{Name: "territory", Node: new("n000000000")}
	for _, mode := range []string{"encoded-sink", "generated-sink", "encoded-tcp", "generated-tcp"} {
		b.Run(mode, func(b *testing.B) {
			var client pb.GraphServiceClient
			if mode == "encoded-tcp" {
				client = connectService(b, s)
			} else if mode == "generated-tcp" {
				client = connectService(b, generatedService{s})
			}
			var size uint64
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				switch mode {
				case "encoded-sink":
					err = ggpb.EmitEncoded(ctx, s.g, sub, meta, ggpb.Options{}, func(p []byte) error { size += uint64(len(p)); return nil })
				case "generated-sink":
					err = ggpb.Emit(ctx, s.g, sub, meta, ggpb.Options{}, func(p *resultpb.Batch) error { v, e := proto.Marshal(p); size += uint64(len(v)); return e })
				default:
					call, cancel := context.WithTimeout(ctx, time.Minute)
					stream, e := client.Territory(call, &pb.TerritoryRequest{Node: meta.Node})
					err = e
					for err == nil {
						v, e := stream.Recv()
						if e != nil {
							err = e
							break
						}
						size += uint64(len(v.Ggpb))
					}
					if err == io.EOF {
						err = nil
					}
					cancel()
				}
				if err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.SetBytes(int64(size / uint64(b.N)))
		})
	}
}

func BenchmarkCorpusTransport(b *testing.B) {
	path := os.Getenv("GOPHERGRAPH_BENCH_SNAPSHOT")
	if path == "" {
		b.Skip("explicit generated corpus path required")
	}
	ctx := context.Background()
	source, err := mmap.NewResident(path, mmap.Warm)
	if err != nil {
		b.Fatal(err)
	}
	g, err := snapshot.Open(ctx, source)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { g.Close() })
	if _, err = source.Prepare(ctx); err != nil {
		b.Fatal(err)
	}
	r, err := wasmquery.New(ctx, wasmquery.Limits{})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { r.Close() })
	reg, err := installedwasm.Default(ctx, r)
	if err != nil {
		b.Fatal(err)
	}
	s, err := NewService(g, r, reg, new(pb.ServerInfoResponse))
	if err != nil {
		b.Fatal(err)
	}
	benchmarkTransports(b, s)
}
