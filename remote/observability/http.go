// Package observability exposes only operational HTTP endpoints. Query transport
// remains exclusively gRPC. Labels come from the finite installed registry.
package observability

import (
	"fmt"
	"gophergraph/remote"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SnapshotObservation struct {
	At                                 time.Time
	RSSBytes, PSSBytes, LockedPSSBytes uint64
}
type Options struct {
	Started         time.Time
	PrepareTime     time.Duration
	WasmPrepareTime time.Duration
	Observe         func() (SnapshotObservation, error)
}

type Endpoints struct {
	mu      sync.Mutex
	closed  bool
	active  sync.WaitGroup
	handler http.Handler
}

func (e *Endpoints) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		http.Error(w, "draining", 503)
		return
	}
	e.active.Add(1)
	e.mu.Unlock()
	defer e.active.Done()
	e.handler.ServeHTTP(w, r)
}

// Close seals request registration then joins scrapes before snapshot unmap.
func (e *Endpoints) Close() { e.mu.Lock(); e.closed = true; e.mu.Unlock(); e.active.Wait() }
func Handler(s *remote.Server, o Options) *Endpoints {
	if o.Started.IsZero() {
		o.Started = time.Now()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "alive\n")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !s.Ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ready\n")
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		metric := func(name string, value any) { fmt.Fprintf(w, "%s %v\n", name, value) }
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		metric("gophergraph_uptime_seconds", time.Since(o.Started).Seconds())
		metric("gophergraph_ready", boolean(s.Ready()))
		metric("go_goroutines", runtime.NumGoroutine())
		metric("go_gomaxprocs", runtime.GOMAXPROCS(0))
		metric("go_memstats_heap_alloc_bytes", mem.HeapAlloc)
		metric("go_memstats_heap_inuse_bytes", mem.HeapInuse)
		metric("go_memstats_alloc_bytes_total", mem.TotalAlloc)
		metric("go_gc_cycles_total", mem.NumGC)
		metric("go_gc_pause_seconds_total", float64(mem.PauseTotalNs)/1e9)
		rss, err := processRSS()
		metric("gophergraph_process_rss_available", boolean(err == nil))
		if err == nil {
			metric("process_resident_memory_bytes", rss)
		}
		info := s.Information()
		fmt.Fprintf(w, "gophergraph_snapshot_info{snapshot_id=%q,mode=%q,version=%q} 1\n", info.SnapshotId, info.ResidencyMode, info.Version)
		metric("gophergraph_snapshot_bytes", info.SnapshotBytes)
		metric("gophergraph_snapshot_nodes", info.Nodes)
		metric("gophergraph_snapshot_edges", info.Edges)
		metric("gophergraph_snapshot_warm_completed", boolean(info.WarmCompleted))
		metric("gophergraph_snapshot_locked", boolean(info.Locked))
		metric("gophergraph_snapshot_locked_bytes", info.LockedBytes)
		metric("gophergraph_snapshot_prepare_seconds", o.PrepareTime.Seconds())
		metric("gophergraph_wasm_startup_prepare_seconds", o.WasmPrepareTime.Seconds())
		capacity, active, rejected := s.ConnectionStatistics()
		metric("gophergraph_connection_capacity", capacity)
		metric("gophergraph_connections_active", active)
		metric("gophergraph_connections_rejected_total", rejected)
		if o.Observe != nil {
			observation, err := o.Observe()
			metric("gophergraph_snapshot_inspection_available", boolean(err == nil))
			if err == nil {
				metric("gophergraph_snapshot_observation_timestamp_seconds", float64(observation.At.UnixNano())/1e9)
				metric("gophergraph_snapshot_rss_bytes", observation.RSSBytes)
				metric("gophergraph_snapshot_pss_bytes", observation.PSSBytes)
				metric("gophergraph_snapshot_locked_pss_bytes", observation.LockedPSSBytes)
			}
		}
		gate := s.Admission()
		metric("gophergraph_query_capacity", gate.Capacity)
		metric("gophergraph_queries_active", gate.Active)
		metric("gophergraph_queries_admitted_total", gate.Admitted)
		metric("gophergraph_queries_rejected_total", gate.Rejected)
		stats := s.WasmStatistics()
		metric("gophergraph_wasm_compilations_total", stats.Compilations)
		metric("gophergraph_wasm_cached_modules", stats.CachedModules)
		metric("gophergraph_wasm_cached_source_bytes", stats.CachedSourceBytes)
		metric("gophergraph_wasm_active", stats.ActiveExecutions)
		metric("gophergraph_wasm_active_handles", stats.ActiveHandles)
		metric("gophergraph_wasm_startup_prepared", 1)
		metric("gophergraph_wasm_capacity", info.WasmCapacity)
		fmt.Fprintln(w, "# TYPE gophergraph_query_duration_seconds histogram")
		bounds := remote.DurationBounds()
		for _, q := range s.QueryStatistics() {
			label := "{query=" + strconv.Quote(q.Name) + "}"
			for _, v := range []struct {
				name  string
				value any
			}{
				{"requests_total", q.Requests}, {"active", q.Active}, {"admitted_total", q.Admitted}, {"rejected_total", q.Rejected}, {"cancelled_total", q.Cancelled}, {"failed_total", q.Failed}, {"completed_total", q.Completed},
				{"execution_seconds_total", q.Execution}, {"encoding_seconds_total", q.Encode}, {"send_seconds_total", q.Send}, {"payload_bytes_accepted_total", q.Bytes}, {"batches_accepted_total", q.Batches}, {"result_nodes_total", q.Nodes}, {"result_edges_total", q.Edges},
				{"wasm_host_calls_total", q.HostCalls}, {"wasm_request_bytes_total", q.HostRequestBytes}, {"wasm_response_bytes_total", q.HostResponseBytes}, {"wasm_timeouts_total", q.Timeouts}, {"wasm_traps_total", q.Traps}, {"wasm_peak_handles", q.PeakHandles}, {"wasm_peak_host_bytes", q.PeakHostBytes},
			} {
				fmt.Fprintf(w, "gophergraph_query_%s%s %v\n", v.name, label, v.value)
			}
			for i, b := range bounds {
				fmt.Fprintf(w, "gophergraph_query_duration_seconds_bucket{query=%q,le=%q} %d\n", q.Name, strconv.FormatFloat(b, 'g', -1, 64), q.Buckets[i])
			}
			fmt.Fprintf(w, "gophergraph_query_duration_seconds_bucket{query=%q,le=\"+Inf\"} %d\ngophergraph_query_duration_seconds_sum%s %g\ngophergraph_query_duration_seconds_count%s %d\n", q.Name, q.Completed+q.Rejected+q.Cancelled+q.Failed, label, q.Duration, label, q.Completed+q.Rejected+q.Cancelled+q.Failed)
		}
	})
	return &Endpoints{handler: mux}
}
func boolean(v bool) int {
	if v {
		return 1
	}
	return 0
}
func processRSS() (uint64, error) {
	b, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(b))
	if len(fields) < 2 {
		return 0, fmt.Errorf("statm unavailable")
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	return pages * uint64(os.Getpagesize()), err
}
