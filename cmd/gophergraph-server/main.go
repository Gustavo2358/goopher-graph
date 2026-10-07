//go:build linux && amd64

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"gophergraph/remote/observability"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"google.golang.org/grpc/credentials"
	"gophergraph/ggpb"
	"gophergraph/remote"
	"gophergraph/remote/installedwasm"
	"gophergraph/remote/pb"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"gophergraph/wasmquery"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string) (err error) {
	f := flag.NewFlagSet("gophergraph-server", flag.ContinueOnError)
	path := f.String("snapshot", "", "immutable snapshot path (required)")
	address := f.String("listen", "127.0.0.1:9090", "gRPC listen address")
	metricsAddress := f.String("metrics-listen", "127.0.0.1:9091", "operational HTTP listener; empty disables")
	mode := f.String("residency", "locked", "locked, warm or lazy; no fallback")
	capacity := f.Int("query-capacity", 0, "active query+stream capacity; 0 derives from CPUs and query-memory-budget")
	budget := f.Uint64("query-memory-budget", 512<<20, "conservative budget for concurrent query workspaces, excluding snapshot/runtime base")
	maxDeadline := f.Duration("max-query-deadline", time.Minute, "maximum accepted CLIENT query deadline (required on RPCs)")
	grace := f.Duration("shutdown-grace", 30*time.Second, "time allowed to finish active streams")
	startupTimeout := f.Duration("startup-timeout", 5*time.Minute, "snapshot/registry preparation timeout")
	maxConnections := f.Int("max-connections", 128, "maximum accepted transport connections")
	maxStreams := f.Uint("max-streams-per-connection", 64, "HTTP/2 concurrent stream cap")
	maxRequest := f.Int("max-request-bytes", 256<<10, "bounded decoded RPC request")
	certPath := f.String("tls-cert", "", "PEM TLS server certificate")
	keyPath := f.String("tls-key", "", "PEM TLS server key")
	allowInsecure := f.Bool("allow-insecure", false, "explicitly permit plaintext on non-loopback listeners")
	var limits wasmquery.Limits
	f.IntVar(&limits.Concurrent, "wasm-concurrency", 4, "existing WASM execution slots (also bounded by global admission)")
	f.Uint64Var(&limits.HostBytes, "wasm-host-bytes", 64<<20, "per-execution host sets/queues budget")
	pages := f.Uint("wasm-memory-pages", 1024, "per-instance 64KiB memory pages")
	f.IntVar(&limits.Handles, "wasm-handles", 256, "per-execution handles")
	f.Uint64Var(&limits.Calls, "wasm-calls", 10000, "per-execution ABI calls")
	f.IntVar(&limits.ArgsBytes, "wasm-args-bytes", 65536, "per-execution args including terminators")
	f.DurationVar(&limits.Timeout, "wasm-timeout", 5*time.Second, "per-execution timeout")
	f.DurationVar(&limits.CompileTimeout, "wasm-compile-timeout", 30*time.Second, "startup compile/prepare timeout per module")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *path == "" {
		return errors.New("--snapshot is required; positional arguments are unsupported")
	}
	if *grace < 0 || *startupTimeout <= 0 || *pages == 0 || *pages > 65536 || *maxStreams == 0 || *maxStreams > 1<<20 {
		return errors.New("invalid lifecycle/WASM/stream limits")
	}
	limits.MemoryPages = uint32(*pages)
	startup, cancelStartup := context.WithTimeout(ctx, *startupTimeout)
	defer cancelStartup()
	source, err := mmap.NewResident(*path, mmap.Mode(*mode))
	if err != nil {
		return err
	}
	g, err := snapshot.Open(startup, source)
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}
	defer func() { err = errors.Join(err, g.Close()) }()
	report, err := source.Prepare(startup)
	if err != nil {
		return err
	}
	// Runtime life remains independent of SIGTERM until query readers are joined.
	rt, err := wasmquery.New(context.Background(), limits)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, rt.Close()) }()
	registry, err := installedwasm.Default(startup, rt)
	if err != nil {
		return err
	}
	m := g.Metadata()
	c, envelope, err := queryCapacity(*capacity, *budget, m.Nodes, m.Edges, limits.MemoryPages, limits.HostBytes)
	if err != nil {
		return err
	}
	info := &pb.ServerInfoResponse{Version: version, SnapshotId: report.SnapshotID, SnapshotFormat: 1, SnapshotBytes: report.Size, Nodes: m.Nodes, Edges: m.Edges, PartialSnapshot: m.PartialLoad, ResidencyMode: string(report.Mode), WarmCompleted: report.WarmCompleted, Locked: report.Locked, LockedBytes: report.LockedBytes, GgpbVersion: ggpb.Version, MaxBatchBytes: ggpb.MaxFrame, WasmCapacity: uint32(limits.Concurrent), Capabilities: []string{"native", "installed-wasm", "ggpb-stream-v1", "client-deadline-required"}, RegistrySha256: registry.SHA256()}
	service, err := remote.NewService(g, rt, registry, info)
	if err != nil {
		return err
	}
	options := remote.ServerOptions{QueryCapacity: c, MaxDeadline: *maxDeadline, MaxRequestBytes: *maxRequest, MaxConnections: *maxConnections, MaxConcurrentStreams: uint32(*maxStreams)}
	host, _, err := net.SplitHostPort(*address)
	if err != nil {
		return err
	}
	if *certPath != "" || *keyPath != "" {
		certificate, err := tls.LoadX509KeyPair(*certPath, *keyPath)
		if err != nil {
			return err
		}
		options.Credentials = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}})
	} else if ip := net.ParseIP(host); !(*allowInsecure || (ip != nil && ip.IsLoopback())) {
		return errors.New("non-loopback listener requires TLS or explicit --allow-insecure")
	}
	server, err := remote.NewServer(service, options)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return err
	}
	httpErr := make(chan error, 1)
	if *metricsAddress != "" {
		metricsListener, e := net.Listen("tcp", *metricsAddress)
		if e != nil {
			listener.Close()
			return e
		}
		handler := observability.Handler(server, observability.Options{Started: time.Now(), PrepareTime: report.PrepareTime, Observe: func() (observability.SnapshotObservation, error) {
			v, e := source.Inspect()
			return observability.SnapshotObservation{At: v.At, RSSBytes: v.RSSBytes, PSSBytes: v.PSSBytes, LockedPSSBytes: v.LockedPSSBytes}, e
		}})
		httpServer := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
		go func() { httpErr <- httpServer.Serve(metricsListener) }()
		defer func() {
			defer handler.Close()
			shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if e := httpServer.Shutdown(shutdown); e != nil {
				_ = httpServer.Close()
			}
		}()
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	fmt.Fprintf(os.Stderr, "serving gRPC=%s snapshot=%s mode=%s warm=%t locked=%t prepare=%s query_capacity=%d estimated_query_bytes=%d GOMAXPROCS=%d\n", listener.Addr(), report.SnapshotID, report.Mode, report.WarmCompleted, report.Locked, report.PrepareTime, c, envelope, runtime.GOMAXPROCS(0))
	select {
	case <-ctx.Done():
	case err = <-serveErr:
	case err = <-httpErr:
	}
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), *grace)
	defer cancelShutdown()
	shutdownErr := server.Shutdown(shutdown)
	if errors.Is(shutdownErr, context.DeadlineExceeded) {
		fmt.Fprintln(os.Stderr, "shutdown grace expired; transport cancelled and readers joined")
		shutdownErr = nil
	}
	return errors.Join(err, shutdownErr)
}

// Estimate is a conservative admission heuristic, not an RSS/allocator guarantee.
// Native BFS growth transients + multiple bitmaps; WASM bounded host+guest memory;
// bounded encoder and transport copies. Compiled-code/base/connection RSS is extra.
func queryCapacity(requested int, budget, nodes, edges uint64, pages uint32, hostBytes uint64) (int, uint64, error) {
	native := nodes*32 + 3*((nodes+63)/64)*8 + ((edges+63)/64)*8 + (8 << 20)
	wasm := uint64(pages)*65536 + hostBytes + (8 << 20)
	envelope := max(native, wasm)
	limit := min(uint64(runtime.GOMAXPROCS(0)), budget/envelope)
	if requested < 0 || limit == 0 {
		return 0, envelope, errors.New("query memory budget cannot accommodate one estimated query")
	}
	if requested == 0 {
		return int(limit), envelope, nil
	}
	if uint64(requested) > budget/envelope {
		return 0, envelope, errors.New("query capacity exceeds estimated query memory budget")
	}
	return requested, envelope, nil
}
