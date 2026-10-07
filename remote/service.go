package remote

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gophergraph/ggpb"
	"gophergraph/graph"
	"gophergraph/query"
	"gophergraph/remote/installedwasm"
	"gophergraph/remote/pb"
	"gophergraph/wasmquery"
)

// Service owns no graph/runtime cleanup. Register through NewServer so admission,
// deadlines, lifecycle and transport limits apply before request decoding.
type Service struct {
	pb.UnimplementedGraphServiceServer
	g        *graph.Graph
	runtime  *wasmquery.Runtime
	registry *installedwasm.Registry
	info     *pb.ServerInfoResponse
}

func NewService(g *graph.Graph, r *wasmquery.Runtime, registry *installedwasm.Registry, info *pb.ServerInfoResponse) (*Service, error) {
	if g == nil || r == nil || registry == nil || info == nil {
		return nil, errors.New("service requires prepared graph, runtime, registry and metadata")
	}
	if _, _, err := g.FindString(""); err != nil {
		return nil, err
	}
	owned := proto.Clone(info).(*pb.ServerInfoResponse)
	limits := r.Limits()
	owned.WasmCapacity = uint32(limits.Concurrent)
	owned.WasmArgsBytes = uint32(min(limits.ArgsBytes, 65536))
	owned.WasmTimeoutMillis = uint64(limits.Timeout / time.Millisecond)
	owned.WasmMemoryPages = limits.MemoryPages
	owned.WasmHostBytes = limits.HostBytes
	return &Service{g: g, runtime: r, registry: registry, info: owned}, nil
}

type queryRecord struct {
	Name                         string
	Execution, Encode, Send      time.Duration
	Bytes, Batches, Nodes, Edges uint64
	Wasm                         wasmquery.Metrics
	Trap                         bool
	telemetry                    *telemetry
}
type recordKey struct{}

func record(ctx context.Context) *queryRecord {
	if r, ok := ctx.Value(recordKey{}).(*queryRecord); ok {
		return r
	}
	return new(queryRecord)
}
func external(id *string) error {
	if id == nil {
		return status.Error(codes.InvalidArgument, "external ID must be present")
	}
	if len(*id) > ggpb.MaxScalar || !utf8.ValidString(*id) {
		return status.Error(codes.InvalidArgument, "invalid external ID")
	}
	return nil
}
func (s *Service) options(f *pb.EdgeLabelFilter) (query.Options, []string, error) {
	if f == nil {
		return query.Options{}, nil, nil
	}
	o := query.Options{EdgeLabels: make([]graph.StringID, 0, len(f.Labels))}
	labels := make([]string, len(f.Labels))
	copy(labels, f.Labels)
	for _, label := range labels {
		if len(label) > ggpb.MaxScalar || !utf8.ValidString(label) {
			return o, nil, status.Error(codes.InvalidArgument, "invalid edge label")
		}
		id, ok, err := s.g.FindString(label)
		if err != nil {
			return o, nil, err
		}
		if ok {
			o.EdgeLabels = append(o.EdgeLabels, id)
		}
	}
	return o, labels, nil
}
func rpcError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	switch {
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "query cancelled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "query deadline exceeded")
	case errors.Is(err, ErrDraining), errors.Is(err, graph.ErrClosed), errors.Is(err, wasmquery.ErrClosed):
		return status.Error(codes.Unavailable, "server draining")
	case errors.Is(err, ErrSaturated), errors.Is(err, wasmquery.ErrLimit), errors.Is(err, ggpb.ErrLimit):
		return status.Error(codes.ResourceExhausted, "query resource limit exceeded")
	case errors.Is(err, graph.ErrNotFound):
		return status.Error(codes.NotFound, "external ID not found")
	default:
		return status.Error(codes.Internal, "query failed")
	}
}
func (s *Service) Territory(req *pb.TerritoryRequest, stream grpc.ServerStreamingServer[pb.EncodedBatch]) error {
	return s.native(stream, "territory", req.Node, nil, req.EdgeFilter)
}
func (s *Service) AntiTerritory(req *pb.AntiTerritoryRequest, stream grpc.ServerStreamingServer[pb.EncodedBatch]) error {
	return s.native(stream, "anti-territory", req.Node, nil, req.EdgeFilter)
}
func (s *Service) Between(req *pb.BetweenRequest, stream grpc.ServerStreamingServer[pb.EncodedBatch]) error {
	return s.native(stream, "between", req.From, req.To, req.EdgeFilter)
}
func (s *Service) native(stream grpc.ServerStreamingServer[pb.EncodedBatch], name string, from, to *string, f *pb.EdgeLabelFilter) error {
	ctx := stream.Context()
	rec := record(ctx)
	renameRecord(ctx, name)
	if err := external(from); err != nil {
		return err
	}
	if name == "between" {
		if err := external(to); err != nil {
			return err
		}
	}
	o, labels, err := s.options(f)
	if err != nil {
		return rpcError(err)
	}
	start, err := s.g.FindNode(*from)
	if err != nil {
		return rpcError(err)
	}
	meta := ggpb.Query{Name: name, Node: from, EdgeLabels: labels}
	var result *query.Subgraph
	t := time.Now()
	switch name {
	case "territory":
		result, err = query.Territory(ctx, s.g, start, o)
	case "anti-territory":
		result, err = query.AntiTerritory(ctx, s.g, start, o)
	case "between":
		end, e := s.g.FindNode(*to)
		if e != nil {
			return rpcError(e)
		}
		meta.Node = nil
		meta.From = from
		meta.To = to
		result, err = query.Between(ctx, s.g, start, end, o)
	}
	rec.Execution = time.Since(t)
	if err != nil {
		return rpcError(err)
	}
	return s.emit(stream, result, meta, nil)
}
func (s *Service) emit(stream grpc.ServerStreamingServer[pb.EncodedBatch], result *query.Subgraph, meta ggpb.Query, extra metadata.MD) error {
	ctx := stream.Context()
	rec := record(ctx)
	n, e, err := result.CountsContext(ctx)
	if err != nil {
		return rpcError(err)
	}
	rec.Nodes = n
	rec.Edges = e
	headers := metadata.Pairs("snapshot-id", s.info.SnapshotId, "ggpb-version", "1")
	if err = stream.SendHeader(metadata.Join(headers, extra)); err != nil {
		return rpcError(err)
	}
	t := time.Now()
	err = ggpb.EmitEncoded(ctx, s.g, result, meta, ggpb.Options{}, func(b []byte) error {
		// EmitEncoded borrows b. grpc may retain message references after Send;
		// transfer immutable ownership, then send synchronously before advancing.
		message := &pb.EncodedBatch{Ggpb: bytes.Clone(b)}
		t := time.Now()
		err := stream.Send(message)
		rec.Send += time.Since(t)
		if err == nil {
			rec.Bytes += uint64(len(b))
			rec.Batches++
		}
		return err
	})
	rec.Encode = time.Since(t) - rec.Send
	return rpcError(err)
}
func validateArgs(args []string, minArgs, maxArgs, byteLimit int) error {
	if len(args) < minArgs || len(args) > maxArgs {
		return status.Error(codes.InvalidArgument, "invalid argument count")
	}
	bytes := 0
	for _, arg := range args {
		if !utf8.ValidString(arg) || strings.IndexByte(arg, 0) >= 0 || len(arg) >= min(byteLimit, 65536)-bytes {
			return status.Error(codes.InvalidArgument, "invalid or oversized arguments")
		}
		bytes += len(arg) + 1
	}
	return nil
}
