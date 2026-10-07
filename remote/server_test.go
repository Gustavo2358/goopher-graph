package remote

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"gophergraph/ggpb"
	"gophergraph/remote/pb"
)

func startServer(t testing.TB, s *Service, o ServerOptions) (*Server, pb.GraphServiceClient, *grpc.ClientConn) {
	t.Helper()
	server, err := NewServer(s, o)
	if err != nil {
		t.Fatal(err)
	}
	if server.Ready() {
		t.Fatal("ready before Serve")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve(l)
	conn, err := grpc.NewClient(l.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(ggpb.MaxFrame+16)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		server.Shutdown(ctx)
		conn.Close()
	})
	return server, pb.NewGraphServiceClient(conn), conn
}
func TestHealthAndInformation(t *testing.T) {
	s, _ := serviceFixture(t)
	server, client, conn := startServer(t, s, ServerOptions{QueryCapacity: 2})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	health := grpc_health_v1.NewHealthClient(conn)
	h, err := health.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: ServiceName})
	if err != nil || h.Status != grpc_health_v1.HealthCheckResponse_SERVING || !server.Ready() {
		t.Fatal(h, err)
	}
	info, err := client.GetServerInfo(ctx, &pb.ServerInfoRequest{})
	if err != nil || info.QueryCapacity != 2 || info.MaxDeadlineMillis != 60000 {
		t.Fatal(info, err)
	}
	server.Drain()
	h, err = health.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: ServiceName})
	if err != nil || h.Status != grpc_health_v1.HealthCheckResponse_NOT_SERVING || server.Ready() {
		t.Fatal(h, err)
	}
	st, err := client.Territory(ctx, &pb.TerritoryRequest{Node: new("A")})
	if err == nil {
		_, err = st.Recv()
	}
	if status.Code(err) != codes.Unavailable {
		t.Fatal(err)
	}
}
func TestClientDeadlineAndAdmission(t *testing.T) {
	s, g := serviceFixture(t)
	server, client, _ := startServer(t, s, ServerOptions{QueryCapacity: 1, MaxDeadline: time.Second * 10})
	first, _ := g.NodeExternalID(0)
	for _, duration := range []time.Duration{0, time.Hour} {
		ctx := context.Background()
		cancel := func() {}
		if duration > 0 {
			ctx, cancel = context.WithTimeout(ctx, duration)
		}
		st, err := client.Territory(ctx, &pb.TerritoryRequest{Node: &first})
		if err == nil {
			_, err = st.Recv()
		}
		cancel()
		if status.Code(err) != codes.InvalidArgument {
			t.Fatal(err)
		}
	}
	release, err := server.admission.Try()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := client.Territory(ctx, &pb.TerritoryRequest{Node: &first})
	if err == nil {
		_, err = st.Recv()
	}
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatal(err)
	}
	release()
	st, err = client.Territory(ctx, &pb.TerritoryRequest{Node: &first})
	if err != nil {
		t.Fatal(err)
	}
	receive(t, st)
	if _, err = st.Recv(); err != io.EOF {
		t.Fatal(err)
	}
}
