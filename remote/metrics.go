package remote

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"gophergraph/remote/pb"
	"gophergraph/wasmquery"
	"sync"
	"time"
)

var durationBounds = []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2, 5, 10, 30, 60}

type QueryStatistics struct {
	Name                                                            string
	Requests, Admitted, Rejected, Cancelled, Failed, Completed      uint64
	Active                                                          int
	Duration, Execution, Encode, Send                               float64
	Buckets                                                         []uint64
	Bytes, Batches, Nodes, Edges                                    uint64
	HostCalls, HostRequestBytes, HostResponseBytes, Timeouts, Traps uint64
	PeakHandles                                                     int
	PeakHostBytes                                                   uint64
}
type telemetry struct {
	mu     sync.Mutex
	order  []string
	series map[string]*QueryStatistics
}

func newTelemetry(s *Service) *telemetry {
	t := &telemetry{series: make(map[string]*QueryStatistics)}
	for _, name := range []string{"territory", "anti-territory", "between", "wasm:unknown"} {
		t.add(name)
	}
	for _, d := range s.registry.List() {
		t.add("wasm:" + d.Name)
	}
	return t
}
func (t *telemetry) add(name string) {
	t.order = append(t.order, name)
	t.series[name] = &QueryStatistics{Name: name, Buckets: make([]uint64, len(durationBounds))}
}
func (t *telemetry) start(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.series[name]
	s.Requests++
	s.Active++
}
func renameRecord(ctx context.Context, name string) {
	r := record(ctx)
	if r.telemetry != nil {
		t := r.telemetry
		t.mu.Lock()
		defer t.mu.Unlock()
		if next, ok := t.series[name]; ok && name != r.Name {
			previous := t.series[r.Name]
			previous.Requests--
			previous.Active--
			next.Requests++
			next.Active++
			r.Name = name
		}
	} else {
		r.Name = name
	}
}
func (t *telemetry) finish(r *queryRecord, admitted bool, code codes.Code, duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.series[r.Name]
	s.Active--
	if admitted {
		s.Admitted++
	}
	switch code {
	case codes.OK:
		s.Completed++
	case codes.ResourceExhausted:
		s.Rejected++
	case codes.Canceled:
		s.Cancelled++
	case codes.DeadlineExceeded:
		s.Cancelled++
		if len(r.Name) > 5 && r.Name[:5] == "wasm:" {
			s.Timeouts++
		}
	default:
		s.Failed++
	}
	if r.Trap {
		s.Traps++
	}
	seconds := duration.Seconds()
	s.Duration += seconds
	for i, b := range durationBounds {
		if seconds <= b {
			s.Buckets[i]++
		}
	}
	s.Execution += r.Execution.Seconds()
	s.Encode += r.Encode.Seconds()
	s.Send += r.Send.Seconds()
	s.Bytes += r.Bytes
	s.Batches += r.Batches
	s.Nodes += r.Nodes
	s.Edges += r.Edges
	s.HostCalls += r.Wasm.HostCalls
	s.HostRequestBytes += r.Wasm.RequestBytes
	s.HostResponseBytes += r.Wasm.ResponseBytes
	s.PeakHandles = max(s.PeakHandles, r.Wasm.PeakHandles)
	s.PeakHostBytes = max(s.PeakHostBytes, r.Wasm.PeakHostBytes)
}
func (s *Server) QueryStatistics() []QueryStatistics {
	t := s.telemetry
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]QueryStatistics, 0, len(t.order))
	for _, name := range t.order {
		v := *t.series[name]
		v.Buckets = append([]uint64(nil), v.Buckets...)
		out = append(out, v)
	}
	return out
}
func DurationBounds() []float64 { return append([]float64(nil), durationBounds...) }
func (s *Server) Information() *pb.ServerInfoResponse {
	return proto.Clone(s.service.info).(*pb.ServerInfoResponse)
}
func (s *Server) WasmStatistics() wasmquery.Stats { return s.service.runtime.Stats() }
