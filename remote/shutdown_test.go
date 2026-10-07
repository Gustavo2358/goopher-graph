package remote

import (
	"context"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestHealthWatchCannotHoldShutdown(t *testing.T) {
	s, _ := serviceFixture(t)
	server, _, conn := startServer(t, s, ServerOptions{QueryCapacity: 1})
	watch, err := grpc_health_v1.NewHealthClient(conn).Watch(context.Background(), &grpc_health_v1.HealthCheckRequest{Service: ServiceName})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = watch.Recv(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = server.Shutdown(ctx); err != nil {
		t.Fatal("Watch held shutdown", err)
	}
	if server.Ready() || server.Admission().Active != 0 {
		t.Fatal("not quiescent")
	}
	if err = server.Shutdown(ctx); err != nil {
		t.Fatal("non-idempotent", err)
	}
	for {
		if _, err = watch.Recv(); err != nil {
			break
		}
	}
}

type upstreamBlockingHealth struct {
	grpc_health_v1.UnimplementedHealthServer
	started, release chan struct{}
}

func (h *upstreamBlockingHealth) Check(context.Context, *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	close(h.started)
	<-h.release
	return new(grpc_health_v1.HealthCheckResponse), nil
}

// Isolate the deliberately dangerous upstream ordering in a subprocess with a
// watchdog. Mirrors #9393: client conn closes, then a still-running handler
// makes GracefulStop hold its mutex when Stop is called. Our ordering never
// enters this state, regardless of whether a future dependency fixes it.
func TestUpstreamShutdownOrdering(t *testing.T) {
	if os.Getenv("GOPHERGRAPH_GRPC_REPRO") == "1" {
		s := grpc.NewServer(grpc.WaitForHandlers(false))
		h := &upstreamBlockingHealth{started: make(chan struct{}), release: make(chan struct{})}
		grpc_health_v1.RegisterHealthServer(s, h)
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go s.Serve(l)
		conn, err := grpc.NewClient(l.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatal(err)
		}
		go grpc_health_v1.NewHealthClient(conn).Check(context.Background(), new(grpc_health_v1.HealthCheckRequest))
		<-h.started
		gracefulDone := make(chan struct{})
		go func() { s.GracefulStop(); close(gracefulDone) }()
		time.Sleep(time.Millisecond * 100)
		conn.Close()
		time.Sleep(time.Millisecond * 300)
		done := make(chan struct{})
		go func() { s.Stop(); close(done) }()
		result := "returned"
		select {
		case <-done:
		case <-time.After(time.Millisecond * 300):
			result = "blocked"
		}
		fmt.Printf("grpc-go=%s concurrent-stop=%s\n", grpc.Version, result)
		close(h.release)
		<-done
		<-gracefulDone
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestUpstreamShutdownOrdering$", "-test.v")
	cmd.Env = append(os.Environ(), "GOPHERGRAPH_GRPC_REPRO=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("watchdog/repro: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "grpc-go="+grpc.Version+" concurrent-stop=") {
		t.Fatal(string(out))
	}
	t.Log(string(out))
}
