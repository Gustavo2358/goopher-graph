// Package wasmquery executes sandboxed WASI commands against the public graph API.
// It owns compiled modules and per-execution state, but never owns the input Graph.
package wasmquery

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
	"gophergraph/graph"
	"gophergraph/query"
	"gophergraph/wasmquery/internal/abi"
)

var (
	ErrClosed = errors.New("wasm query runtime or result closed")
	ErrLimit  = errors.New("wasm query limit exceeded")
	ErrABI    = errors.New("invalid wasm query ABI")
	ErrHandle = errors.New("invalid wasm query handle")
	ErrResult = errors.New("query must return exactly one subgraph")
)

// Limits are per Runtime except HostBytes, Handles, Calls, ArgsBytes and Timeout,
// which apply to each execution. Zero fields select defaults; negatives are invalid.
// HostBytes accounts for set bitsets and traversal queues, not total process RSS.
type Limits struct {
	MemoryPages    uint32
	TableElements  uint32
	ModuleBytes    int
	CachedModules  int
	CacheBytes     int
	Concurrent     int
	Handles        int
	HostBytes      uint64
	Calls          uint64
	ArgsBytes      int
	Timeout        time.Duration
	CompileTimeout time.Duration
}

func (l Limits) defaults() (Limits, error) {
	if l.TableElements == 0 {
		l.TableElements = 65536
	}
	if l.MemoryPages == 0 {
		l.MemoryPages = 1024
	}
	if l.ModuleBytes == 0 {
		l.ModuleBytes = 16 << 20
	}
	if l.CachedModules == 0 {
		l.CachedModules = 16
	}
	if l.CacheBytes == 0 {
		l.CacheBytes = 128 << 20
	}
	if l.Concurrent == 0 {
		l.Concurrent = 4
	}
	if l.Handles == 0 {
		l.Handles = 256
	}
	if l.HostBytes == 0 {
		l.HostBytes = 64 << 20
	}
	if l.Calls == 0 {
		l.Calls = 10000
	}
	if l.ArgsBytes == 0 {
		l.ArgsBytes = abi.MaxMessage
	}
	if l.Timeout == 0 {
		l.Timeout = 5 * time.Second
	}
	if l.CompileTimeout == 0 {
		l.CompileTimeout = 30 * time.Second
	}
	if l.MemoryPages > 65536 || l.ModuleBytes < 1 || l.CachedModules < 1 || l.CacheBytes < 1 || l.Concurrent < 1 || l.Handles < 1 || l.ArgsBytes < 1 || l.Timeout < 0 || l.CompileTimeout < 0 {
		return l, fmt.Errorf("%w: invalid configuration", ErrLimit)
	}
	return l, nil
}

type Runtime struct {
	rt            wazero.Runtime
	limits        Limits
	life          context.Context
	cancel        context.CancelFunc
	mu            sync.Mutex
	compileMu     sync.Mutex
	active        sync.WaitGroup
	closed        bool
	closeOnce     sync.Once
	closeErr      error
	cache         map[[32]byte]*Module
	cacheBytes    int
	slots         chan struct{}
	compilations  atomic.Uint64
	executions    atomic.Uint64
	activeRuns    atomic.Int64
	activeHandles atomic.Int64
}

// Module is immutable and owned by its Runtime. Closing the Runtime invalidates it.
type Module struct {
	owner    *Runtime
	compiled wazero.CompiledModule
	digest   [32]byte
}

func (m *Module) SHA256() string { return fmt.Sprintf("%x", m.digest) }

type Stats struct {
	CachedModules     int
	CachedSourceBytes int
	Compilations      uint64
	Executions        uint64
	ActiveExecutions  int64
	ActiveHandles     int64
}

func (r *Runtime) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Stats{len(r.cache), r.cacheBytes, r.compilations.Load(), r.executions.Load(), r.activeRuns.Load(), r.activeHandles.Load()}
}

func New(ctx context.Context, limits Limits) (*Runtime, error) {
	l, err := limits.defaults()
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	life, cancel := context.WithCancel(ctx)
	rt := wazero.NewRuntimeWithConfig(life, wazero.NewRuntimeConfig().WithMemoryLimitPages(l.MemoryPages).WithCloseOnContextDone(true))
	r := &Runtime{rt: rt, limits: l, life: life, cancel: cancel, cache: make(map[[32]byte]*Module), slots: make(chan struct{}, l.Concurrent)}
	if _, err = wasi_snapshot_preview1.Instantiate(life, rt); err == nil {
		_, err = rt.NewHostModuleBuilder(abi.Module).NewFunctionBuilder().WithFunc(hostCall).Export("call").Instantiate(life)
	}
	if err != nil {
		cancel()
		_ = rt.Close(context.Background())
		return nil, err
	}
	return r, nil
}
func (r *Runtime) begin(ctx context.Context, timeout time.Duration) (context.Context, func(), error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, nil, ErrClosed
	}
	r.active.Add(1)
	r.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	stop := context.AfterFunc(r.life, cancel)
	done := func() { stop(); cancel(); r.active.Done() }
	if err := r.life.Err(); err != nil {
		done()
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		done()
		return nil, nil, err
	}
	// Wazero supports opt-in capabilities (including sockets) via context values.
	// Preserve cancellation/deadline, but do not inherit ambient runtime capabilities.
	return capabilityContext{ctx}, done, nil
}

type capabilityContext struct{ context.Context }

func (capabilityContext) Value(any) any { return nil }

// Compile caches by SHA-256 of the exact bytes. Cache admission is bounded;
// when full, it returns ErrLimit. There is no eviction of modules in use.
// The caller must not modify wasm until Compile returns.
func (r *Runtime) Compile(ctx context.Context, wasm []byte) (*Module, error) {
	ctx, done, err := r.begin(ctx, r.limits.CompileTimeout)
	if err != nil {
		return nil, err
	}
	defer done()
	if len(wasm) > r.limits.ModuleBytes {
		return nil, ErrLimit
	}
	key := sha256.Sum256(wasm)
	r.compileMu.Lock()
	defer r.compileMu.Unlock()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	m := r.cache[key]
	full := len(r.cache) >= r.limits.CachedModules || len(wasm) > r.limits.CacheBytes-r.cacheBytes
	r.mu.Unlock()
	if m != nil {
		return m, nil
	}
	if full {
		return nil, fmt.Errorf("%w: compiled module cache", ErrLimit)
	}
	bounded, err := limitTable(wasm, r.limits.TableElements)
	if err != nil {
		return nil, err
	}
	compiled, err := r.rt.CompileModule(ctx, bounded)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err == nil {
		err = validateModule(compiled)
	}
	if err != nil {
		_ = compiled.Close(context.Background())
		return nil, err
	}
	m = &Module{r, compiled, key}
	r.mu.Lock()
	r.cache[key] = m
	r.cacheBytes += len(wasm)
	r.mu.Unlock()
	r.compilations.Add(1)
	return m, nil
}
func validateModule(m wazero.CompiledModule) error {
	if len(m.ImportedMemories()) != 0 {
		return fmt.Errorf("%w: imported memory", ErrABI)
	}
	for _, f := range m.ImportedFunctions() {
		mod, name, _ := f.Import()
		if mod == abi.Module && (!slices.Equal(f.ParamTypes(), []api.ValueType{api.ValueTypeI32, api.ValueTypeI64, api.ValueTypeI64, api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32, api.ValueTypeI32}) || !slices.Equal(f.ResultTypes(), []api.ValueType{api.ValueTypeI64})) {
			return fmt.Errorf("%w: host call signature", ErrABI)
		}
		if mod != wasi_snapshot_preview1.ModuleName && (mod != abi.Module || name != "call") {
			return fmt.Errorf("%w: import %s.%s", ErrABI, mod, name)
		}
	}
	start, ok := m.ExportedFunctions()["_start"]
	if !ok || len(start.ParamTypes()) != 0 || len(start.ResultTypes()) != 0 {
		return fmt.Errorf("%w: WASI _start required", ErrABI)
	}
	return nil
}

type executionKey struct{}

// Metrics count only the GopherGraph ABI boundary, excluding WASI startup.
// Set contents never cross this boundary: only handles/scalars and bounded messages.
type Metrics struct {
	HostCalls     uint64
	RequestBytes  uint64
	ResponseBytes uint64
	PeakHandles   int
	PeakHostBytes uint64
}

// Result owns host sets independently of the guest and compiled module. Keep g
// open until use is complete. Close is idempotent; do not race it with Subgraph.
type Result struct {
	sub     *query.Subgraph
	metrics Metrics
}

func (r *Result) Subgraph() (*query.Subgraph, error) {
	if r == nil || r.sub == nil {
		return nil, ErrClosed
	}
	return r.sub, nil
}
func (r *Result) Metrics() Metrics { return r.metrics }
func (r *Result) Close() error {
	if r != nil {
		r.sub = nil
	}
	return nil
}

// Execute creates a fresh WASI instance with no filesystem, environment,
// network, stdio or wall-clock capabilities. args become os.Args[1:] in Go.
func (r *Runtime) Execute(ctx context.Context, m *Module, g *graph.Graph, args []string) (*Result, error) {
	ctx, done, err := r.begin(ctx, r.limits.Timeout)
	if err != nil {
		return nil, err
	}
	defer done()
	if m == nil || m.owner != r {
		return nil, ErrABI
	}
	if _, _, err = g.FindString(""); err != nil {
		return nil, err
	}
	bytes := 0
	for _, arg := range args {
		if strings.IndexByte(arg, 0) >= 0 {
			return nil, fmt.Errorf("%w: NUL in argument", ErrABI)
		}
		if len(arg) >= r.limits.ArgsBytes-bytes {
			return nil, fmt.Errorf("%w: arguments", ErrLimit)
		}
		bytes += len(arg) + 1
	}
	select {
	case r.slots <- struct{}{}:
	default:
		return nil, fmt.Errorf("%w: concurrent executions", ErrLimit)
	}
	defer func() { <-r.slots }()
	r.activeRuns.Add(1)
	defer r.activeRuns.Add(-1)
	r.executions.Add(1)
	s := &execution{r: r, g: g, handles: make(map[uint64]object)}
	defer s.clear()
	ctx = context.WithValue(ctx, executionKey{}, s)
	argv := append([]string{"query.wasm"}, args...)
	instance, runErr := r.rt.InstantiateModule(ctx, m.compiled, wazero.NewModuleConfig().WithName("").WithArgs(argv...))
	if instance != nil {
		defer instance.Close(context.Background())
	}
	if s.err != nil {
		return nil, s.err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	var exit *sys.ExitError
	if runErr != nil && !(errors.As(runErr, &exit) && exit.ExitCode() == 0) {
		return nil, runErr
	}
	if s.result == nil {
		return nil, ErrResult
	}
	return &Result{sub: s.result, metrics: s.metrics}, nil
}

// Close cancels and joins active executions/compilations, then frees all modules.
// Results already returned remain valid while their input Graph stays open.
func (r *Runtime) Close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()
		r.cancel()
		r.active.Wait()
		r.closeErr = r.rt.Close(context.Background())
		r.mu.Lock()
		clear(r.cache)
		r.cacheBytes = 0
		r.mu.Unlock()
	})
	return r.closeErr
}
