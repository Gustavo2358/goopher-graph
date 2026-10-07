package remote

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gophergraph/ggpb"
	"gophergraph/remote/pb"
	"strings"
	"testing"
	"time"
)

func TestInstalledWasmStreamingAndDiscovery(t *testing.T) {
	s, g := serviceFixture(t)
	client := connectService(t, s)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	list, err := client.ListWasmQueries(ctx, &pb.ListWasmQueriesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Queries) != 3 || list.Queries[0].Name != "between" {
		t.Fatal(list)
	}
	first, _ := g.NodeExternalID(0)
	d, _, _ := s.registry.Lookup("shared-targets")
	stream, err := client.RunWasm(ctx, &pb.RunWasmRequest{QueryName: d.Name, Args: []string{first, first}, ExpectedSha256: &d.SHA256})
	if err != nil {
		t.Fatal(err)
	}
	payloads := receive(t, stream)
	var dec ggpb.BatchDecoder
	b, err := dec.Decode(ctx, payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	if b.GetHeader().Query.Name != "wasm:"+d.Name+"@"+d.SHA256 {
		t.Fatal(b)
	}
	headers, err := stream.Header()
	if err != nil || headers.Get("wasm-sha256")[0] != d.SHA256 {
		t.Fatal(headers, err)
	}
	if stats := s.runtime.Stats(); stats.Compilations != 3 || stats.Executions != 1 || stats.ActiveHandles != 0 {
		t.Fatal(stats)
	}
	for _, tc := range []struct {
		req  *pb.RunWasmRequest
		code codes.Code
	}{
		{&pb.RunWasmRequest{QueryName: "missing"}, codes.NotFound},
		{&pb.RunWasmRequest{QueryName: d.Name}, codes.InvalidArgument},
		{&pb.RunWasmRequest{QueryName: d.Name, Args: []string{first, first}, ExpectedSha256: new("bad")}, codes.FailedPrecondition},
		{&pb.RunWasmRequest{QueryName: d.Name, Args: []string{first, "bad\x00arg"}}, codes.InvalidArgument},
		{&pb.RunWasmRequest{QueryName: d.Name, Args: []string{first, strings.Repeat("x", 65536)}}, codes.InvalidArgument},
	} {
		st, err := client.RunWasm(ctx, tc.req)
		if err == nil {
			_, err = st.Recv()
		}
		if status.Code(err) != tc.code {
			t.Fatal(tc.code, err)
		}
	}
}
