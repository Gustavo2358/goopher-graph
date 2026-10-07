package remote

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gophergraph/remote/pb"
	"strings"
	"testing"
	"time"
)

func TestOversizedRequestReleasesAdmission(t *testing.T) {
	s, g := serviceFixture(t)
	server, client, _ := startServer(t, s, ServerOptions{QueryCapacity: 1, MaxRequestBytes: 128})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := client.Territory(ctx, &pb.TerritoryRequest{Node: new(strings.Repeat("x", 1024))})
	if err == nil {
		_, err = stream.Recv()
	}
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatal(err)
	}
	first, _ := g.NodeExternalID(0)
	stream, err = client.Territory(ctx, &pb.TerritoryRequest{Node: &first})
	if err != nil {
		t.Fatal(err)
	}
	receive(t, stream)
	if server.Admission().Active != 0 {
		t.Fatal(server.Admission())
	}
}
func TestConnectionLimit(t *testing.T) {
	s, _ := serviceFixture(t)
	_, client, conn := startServer(t, s, ServerOptions{QueryCapacity: 1, MaxConnections: 1})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := client.GetServerInfo(ctx, &pb.ServerInfoRequest{}); err != nil {
		t.Fatal(err)
	}
	second, err := grpc.NewClient(conn.Target(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	short, stop := context.WithTimeout(context.Background(), time.Second/5)
	defer stop()
	if _, err = pb.NewGraphServiceClient(second).GetServerInfo(short, &pb.ServerInfoRequest{}); err == nil {
		t.Fatal("connection cap bypassed")
	}
	if _, err = client.GetServerInfo(ctx, &pb.ServerInfoRequest{}); err != nil {
		t.Fatal("existing connection broken", err)
	}
}
