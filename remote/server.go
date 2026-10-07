package remote

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"gophergraph/ggpb"
	"gophergraph/remote/pb"
)

const ServiceName = "gophergraph.remote.v1.GraphService"

type ServerOptions struct {
	QueryCapacity        int
	MaxDeadline          time.Duration
	MaxRequestBytes      int
	MaxConcurrentStreams uint32
	MaxConnections       int
	Credentials          credentials.TransportCredentials
}

func (o ServerOptions) defaults() (ServerOptions, error) {
	if o.QueryCapacity < 1 {
		return o, errors.New("query capacity must be positive")
	}
	if o.MaxDeadline == 0 {
		o.MaxDeadline = time.Minute
	}
	if o.MaxRequestBytes == 0 {
		o.MaxRequestBytes = 256 << 10
	}
	if o.MaxConcurrentStreams == 0 {
		o.MaxConcurrentStreams = 64
	}
	if o.MaxConnections == 0 {
		o.MaxConnections = 128
	}
	if o.MaxDeadline < 0 || o.MaxRequestBytes < 1 || o.MaxRequestBytes > ggpb.MaxFrame || o.MaxConnections < 1 {
		return o, errors.New("invalid transport limits")
	}
	return o, nil
}

// Server lifecycle owns transport contexts; caller keeps graph/runtime open
// through Shutdown. Serve is called once. Shutdown always joins readers before
// returning, even when its grace deadline expires.
type Server struct {
	grpc         *grpc.Server
	health       *health.Server
	admission    *Admission
	service      *Service
	options      ServerOptions
	life         context.Context
	cancel       context.CancelFunc
	watch        context.Context
	cancelWatch  context.CancelFunc
	mu           sync.Mutex
	draining     bool
	streams      sync.WaitGroup
	ready        atomic.Bool
	shutdownOnce sync.Once
	shutdownErr  error
}

func NewServer(service *Service, options ServerOptions) (*Server, error) {
	o, err := options.defaults()
	if err != nil {
		return nil, err
	}
	if service == nil {
		return nil, errors.New("nil service")
	}
	gate, err := NewAdmission(o.QueryCapacity)
	if err != nil {
		return nil, err
	}
	life, cancel := context.WithCancel(context.Background())
	watch, cancelWatch := context.WithCancel(context.Background())
	s := &Server{service: service, admission: gate, options: o, life: life, cancel: cancel, watch: watch, cancelWatch: cancelWatch, health: health.NewServer()}
	service.info.QueryCapacity = uint32(o.QueryCapacity)
	service.info.MaxRequestBytes = uint32(o.MaxRequestBytes)
	service.info.MaxDeadlineMillis = uint64(o.MaxDeadline / time.Millisecond)
	optionsGRPC := []grpc.ServerOption{grpc.MaxRecvMsgSize(o.MaxRequestBytes), grpc.MaxSendMsgSize(ggpb.MaxFrame + 16), grpc.MaxConcurrentStreams(o.MaxConcurrentStreams), grpc.MaxHeaderListSize(8192), grpc.ConnectionTimeout(10 * time.Second), grpc.StaticStreamWindowSize(64 << 10), grpc.StaticConnWindowSize(1 << 20), grpc.SharedWriteBuffer(true), grpc.StreamInterceptor(s.stream)}
	if o.Credentials != nil {
		optionsGRPC = append(optionsGRPC, grpc.Creds(o.Credentials))
	}
	s.grpc = grpc.NewServer(optionsGRPC...)
	pb.RegisterGraphServiceServer(s.grpc, service)
	grpc_health_v1.RegisterHealthServer(s.grpc, s.health)
	s.health.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	s.health.SetServingStatus(ServiceName, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	return s, nil
}

type scopedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s scopedStream) Context() context.Context { return s.ctx }
func isQuery(method string) bool {
	switch method {
	case pb.GraphService_Territory_FullMethodName, pb.GraphService_AntiTerritory_FullMethodName, pb.GraphService_Between_FullMethodName, pb.GraphService_RunWasm_FullMethodName:
		return true
	}
	return false
}
func (s *Server) stream(server any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	s.mu.Lock()
	if s.draining {
		s.mu.Unlock()
		return rpcError(ErrDraining)
	}
	s.streams.Add(1)
	s.mu.Unlock()
	defer s.streams.Done()
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	life := s.life
	if strings.HasPrefix(info.FullMethod, "/grpc.health.v1.Health/") {
		life = s.watch
	}
	stop := context.AfterFunc(life, cancel)
	defer stop()
	if life.Err() != nil {
		cancel()
	}
	if isQuery(info.FullMethod) {
		deadline, ok := stream.Context().Deadline()
		if !ok || time.Until(deadline) > s.options.MaxDeadline {
			return status.Error(codes.InvalidArgument, "a client deadline within the server limit is required")
		}
		if err := ctx.Err(); err != nil {
			return rpcError(err)
		}
		release, err := s.admission.Try()
		if err != nil {
			return rpcError(err)
		}
		defer release()
		ctx = context.WithValue(ctx, recordKey{}, new(queryRecord))
	}
	return handler(server, scopedStream{stream, ctx})
}
func (s *Server) Serve(listener net.Listener) error {
	s.mu.Lock()
	if s.draining {
		s.mu.Unlock()
		return ErrDraining
	}
	s.ready.Store(true)
	s.health.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	s.health.SetServingStatus(ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)
	s.mu.Unlock()
	err := s.grpc.Serve(&boundedListener{Listener: listener, slots: make(chan struct{}, s.options.MaxConnections)})
	s.ready.Store(false)
	return err
}
func (s *Server) Ready() bool               { return s.ready.Load() }
func (s *Server) Admission() AdmissionStats { return s.admission.Stats() }
func (s *Server) Drain() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draining {
		return
	}
	s.draining = true
	s.ready.Store(false)
	s.admission.Drain()
	s.health.Shutdown()
	s.cancelWatch()
}

// No Stop overlaps GracefulStop. grpc-go 1.84.0 can hold its server mutex while
// GracefulStop waits for handlers (#9393). We join our handlers first. If grace
// expires, Stop is called BEFORE GracefulStop was ever started; it cancels the
// transport context and unblocks flow-controlled Send/Recv. Then join readers.
func (s *Server) Shutdown(ctx context.Context) error {
	s.shutdownOnce.Do(func() {
		s.Drain()
		done := make(chan struct{})
		go func() { s.streams.Wait(); close(done) }()
		select {
		case <-done:
			s.cancel()
			s.grpc.GracefulStop()
		case <-ctx.Done():
			s.shutdownErr = ctx.Err()
			s.cancel()
			s.grpc.Stop()
			<-done
		}
	})
	return s.shutdownErr
}
