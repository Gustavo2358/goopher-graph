package remote

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gophergraph/ggpb"
	"gophergraph/remote/installedwasm"
	"gophergraph/remote/pb"
	"time"
)

func (s *Service) RunWasm(req *pb.RunWasmRequest, stream grpc.ServerStreamingServer[pb.EncodedBatch]) error {
	ctx := stream.Context()
	d, m, ok := s.registry.Lookup(req.QueryName)
	if !ok {
		return status.Error(codes.NotFound, "installed query not found")
	}
	rec := record(ctx)
	rec.Name = d.Name
	if req.ExpectedSha256 != nil && *req.ExpectedSha256 != d.SHA256 {
		return status.Error(codes.FailedPrecondition, "installed query hash mismatch")
	}
	if err := validateArgs(req.Args, d.MinArgs, d.MaxArgs); err != nil {
		return err
	}
	t := time.Now()
	result, metrics, err := s.runtime.ExecuteReport(ctx, m, s.g, req.Args)
	rec.Execution = time.Since(t)
	rec.Wasm = metrics
	if err != nil {
		return rpcError(err)
	}
	defer result.Close()
	sub, err := result.Subgraph()
	if err != nil {
		return rpcError(err)
	}
	identity := "wasm:" + d.Name + "@" + d.SHA256
	return s.emit(stream, sub, ggpb.Query{Name: identity}, metadata.Pairs("wasm-query", d.Name, "wasm-sha256", d.SHA256))
}
func descriptor(d installedwasm.Descriptor) *pb.WasmQueryInfo {
	return &pb.WasmQueryInfo{Name: d.Name, Sha256: d.SHA256, Version: d.Version, Description: d.Description, Abi: d.ABI, MinArgs: uint32(d.MinArgs), MaxArgs: uint32(d.MaxArgs), Parameters: d.Parameters}
}
func (s *Service) ListWasmQueries(ctx context.Context, _ *pb.ListWasmQueriesRequest) (*pb.ListWasmQueriesResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, rpcError(err)
	}
	response := &pb.ListWasmQueriesResponse{RegistrySha256: s.registry.SHA256()}
	for _, d := range s.registry.List() {
		response.Queries = append(response.Queries, descriptor(d))
	}
	return response, nil
}
func (s *Service) GetServerInfo(ctx context.Context, _ *pb.ServerInfoRequest) (*pb.ServerInfoResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, rpcError(err)
	}
	return proto.Clone(s.info).(*pb.ServerInfoResponse), nil
}
