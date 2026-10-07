package wasmquery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync/atomic"
	"unicode/utf8"

	"github.com/tetratelabs/wazero/api"
	"gophergraph/graph"
	"gophergraph/query"
	"gophergraph/wasmquery/internal/abi"
)

// Tokens are never reused, including across runtimes. Each execution can resolve
// only its own tokens. No guest pointer refers to a host set or snapshot byte.
var tokens atomic.Uint64

type object struct {
	nodes *graph.NodeSet
	edges *graph.EdgeSet
	sub   *query.Subgraph
	bytes uint64
}
type execution struct {
	r       *Runtime
	g       *graph.Graph
	handles map[uint64]object
	used    uint64
	err     error
	result  *query.Subgraph
	metrics Metrics
}

func (s *execution) clear() {
	s.r.activeHandles.Add(-int64(len(s.handles)))
	clear(s.handles)
	s.used = 0
	s.result = nil
}
func (s *execution) sizes() (uint64, uint64) {
	m := s.g.Metadata()
	return ((m.Nodes + 63) / 64) * 8, ((m.Edges + 63) / 64) * 8
}
func (s *execution) budget(cost, scratch uint64) error {
	if len(s.handles) >= s.r.limits.Handles {
		return fmt.Errorf("%w: handles", ErrLimit)
	}
	if cost > s.r.limits.HostBytes-s.used || scratch > s.r.limits.HostBytes-s.used-cost {
		return fmt.Errorf("%w: host sets/queue", ErrLimit)
	}
	peak := s.used + cost + scratch
	if peak > s.metrics.PeakHostBytes {
		s.metrics.PeakHostBytes = peak
	}
	return nil
}
func (s *execution) put(v object) (uint64, error) {
	if err := s.budget(v.bytes, 0); err != nil {
		return 0, err
	}
	var id uint64
	for {
		previous := tokens.Load()
		if previous >= abi.Failure-1 {
			return 0, ErrLimit
		}
		id = previous + 1
		if tokens.CompareAndSwap(previous, id) {
			break
		}
	}
	s.handles[id] = v
	s.used += v.bytes
	s.r.activeHandles.Add(1)
	if len(s.handles) > s.metrics.PeakHandles {
		s.metrics.PeakHandles = len(s.handles)
	}
	return id, nil
}
func (s *execution) get(id uint64) (object, error) {
	v, ok := s.handles[id]
	if !ok {
		return object{}, ErrHandle
	}
	return v, nil
}
func (s *execution) release(id uint64) error {
	v, err := s.get(id)
	if err != nil {
		return err
	}
	delete(s.handles, id)
	s.used -= v.bytes
	s.r.activeHandles.Add(-1)
	return nil
}

func hostCall(ctx context.Context, mod api.Module, op uint32, a, b uint64, ptr, size, out, capacity uint32) uint64 {
	s, ok := ctx.Value(executionKey{}).(*execution)
	if !ok {
		_ = mod.CloseWithExitCode(ctx, 1)
		return abi.Failure
	}
	fail := func(err error) uint64 {
		if s.err == nil {
			s.err = fmt.Errorf("host operation %d: %w", op, err)
		}
		_ = mod.CloseWithExitCode(ctx, 1)
		return abi.Failure
	}
	if s.err != nil {
		return abi.Failure
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if s.result != nil {
		return fail(ErrResult)
	}
	s.metrics.HostCalls++
	if s.metrics.HostCalls > s.r.limits.Calls {
		return fail(fmt.Errorf("%w: host calls", ErrLimit))
	}
	if size > abi.MaxMessage || capacity > abi.MaxMessage {
		return fail(fmt.Errorf("%w: ABI message", ErrLimit))
	}
	memory := mod.Memory()
	if memory == nil {
		return fail(ErrABI)
	}
	data, valid := memory.Read(ptr, size)
	if !valid || !utf8.Valid(data) {
		return fail(ErrABI)
	}
	if _, valid = memory.Read(out, capacity); !valid {
		return fail(ErrABI)
	}
	s.metrics.RequestBytes += uint64(size)
	var p abi.Params
	if size != 0 {
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p); err != nil {
			return fail(fmt.Errorf("%w: parameters", ErrABI))
		}
		if dec.Decode(new(any)) != io.EOF {
			return fail(ErrABI)
		}
	}
	if op == abi.ReadNodeProperty || op == abi.ReadEdgeProperty {
		value, err := s.readProperty(ctx, op == abi.ReadEdgeProperty, p)
		if err != nil {
			return fail(err)
		}
		// Check the text before JSON encoding: escaping can take up to six bytes per byte.
		if len(value.Value.Text) > abi.MaxMessage/6 {
			return fail(fmt.Errorf("%w: property scalar", ErrLimit))
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return fail(err)
		}
		if len(encoded) > int(capacity) || !memory.Write(out, encoded) {
			return fail(ErrABI)
		}
		s.metrics.ResponseBytes += uint64(len(encoded))
		return uint64(len(encoded))
	}
	result, err := s.dispatch(ctx, op, a, b, p)
	if err != nil {
		return fail(err)
	}
	if err = ctx.Err(); err != nil {
		return fail(err)
	}
	return result
}

func (s *execution) dispatch(ctx context.Context, op uint32, a, b uint64, p abi.Params) (uint64, error) {
	nbytes, ebytes := s.sizes()
	if op < abi.Nodes || op > abi.NodesAnyLabelProperty {
		return 0, ErrABI
	}
	switch op {
	case abi.NodesAnyLabelProperty:
		value, err := decodeValue(p.Value)
		if err != nil {
			return 0, err
		}
		if err = s.budget(nbytes, nbytes); err != nil {
			return 0, err
		}
		n, err := s.g.NodesWithAnyLabelAndProperty(ctx, p.Labels, p.Key, value)
		if err != nil {
			return 0, err
		}
		return s.put(object{nodes: n, bytes: nbytes})
	case abi.Nodes, abi.NodesLabel:
		if err := s.budget(nbytes, 0); err != nil {
			return 0, err
		}
		var n *graph.NodeSet
		var err error
		if op == abi.NodesLabel {
			n, err = s.g.NodesWithLabel(ctx, p.Label)
		} else {
			n, err = graph.NewNodeSet(s.g)
			if err == nil {
				for _, name := range p.IDs {
					if err = ctx.Err(); err != nil {
						break
					}
					var id graph.NodeID
					id, err = s.g.FindNode(name)
					if err != nil {
						break
					}
					_ = n.Add(id)
				}
			}
		}
		if err != nil {
			return 0, err
		}
		return s.put(object{nodes: n, bytes: nbytes})
	case abi.Release:
		return 0, s.release(a)
	}
	v, err := s.get(a)
	if err != nil {
		return 0, err
	}
	switch op {
	case abi.Count, abi.Empty:
		var count uint64
		switch {
		case v.nodes != nil:
			count, err = v.nodes.CountContext(ctx)
		case v.edges != nil:
			count, err = v.edges.CountContext(ctx)
		case b == 0:
			count, _, err = v.sub.CountsContext(ctx)
		case b == 1:
			_, count, err = v.sub.CountsContext(ctx)
		default:
			return 0, ErrABI
		}
		if err != nil {
			return 0, err
		}
		if op == abi.Empty {
			if count == 0 {
				return 1, nil
			}
			return 0, nil
		}
		return count, nil
	case abi.Return:
		if v.sub == nil {
			return 0, ErrHandle
		}
		s.result = v.sub
		return 0, nil
	case abi.Out, abi.In, abi.Both, abi.OutE, abi.InE, abi.BothE, abi.Reachable:
		if v.nodes == nil {
			return 0, ErrHandle
		}
		return s.traverse(ctx, op, v.nodes, p)
	case abi.Sources, abi.Targets:
		if v.edges == nil {
			return 0, ErrHandle
		}
		if err := s.budget(nbytes, 0); err != nil {
			return 0, err
		}
		n, err := graph.NewNodeSet(s.g)
		if err != nil {
			return 0, err
		}
		it := v.edges.Iterator()
		for it.Next() {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			e, err := s.g.Edge(it.ID())
			if err != nil {
				return 0, err
			}
			id := e.Source
			if op == abi.Targets {
				id = e.Target
			}
			_ = n.Add(id)
		}
		return s.put(object{nodes: n, bytes: nbytes})
	case abi.Union, abi.Intersection, abi.Difference:
		w, err := s.get(b)
		if err != nil {
			return 0, err
		}
		return s.algebra(ctx, op, v, w)
	case abi.Label, abi.Property:
		return s.filter(ctx, op, v, p)
	case abi.Induced:
		if v.nodes == nil {
			return 0, ErrHandle
		}
		if err = s.budget(nbytes+ebytes, nbytes); err != nil {
			return 0, err
		}
		opts, err := s.options(p.Labels)
		if err != nil {
			return 0, err
		}
		sub, err := query.FromNodes(ctx, s.g, v.nodes, opts)
		if err != nil {
			return 0, err
		}
		return s.put(object{sub: sub, bytes: nbytes + ebytes})
	case abi.Subgraph:
		w, err := s.get(b)
		if err != nil {
			return 0, err
		}
		if v.nodes == nil || w.edges == nil {
			return 0, ErrHandle
		}
		if err = s.budget(nbytes+ebytes, 0); err != nil {
			return 0, err
		}
		sub, err := query.NewSubgraph(s.g)
		if err != nil {
			return 0, err
		}
		ni := v.nodes.Iterator()
		for ni.Next() {
			if err = ctx.Err(); err != nil {
				return 0, err
			}
			_ = sub.AddNode(ni.ID())
		}
		ei := w.edges.Iterator()
		for ei.Next() {
			if err = ctx.Err(); err != nil {
				return 0, err
			}
			if err = sub.AddEdge(ei.ID()); err != nil {
				return 0, err
			}
		}
		return s.put(object{sub: sub, bytes: nbytes + ebytes})
	case abi.SubNodes, abi.SubEdges:
		if v.sub == nil {
			return 0, ErrHandle
		}
		if op == abi.SubNodes {
			if err = s.budget(nbytes, 0); err != nil {
				return 0, err
			}
			n, err := graph.NewNodeSet(s.g)
			if err != nil {
				return 0, err
			}
			it := v.sub.Nodes()
			for it.Next() {
				if err = ctx.Err(); err != nil {
					return 0, err
				}
				_ = n.Add(it.ID())
			}
			return s.put(object{nodes: n, bytes: nbytes})
		}
		if err = s.budget(ebytes, 0); err != nil {
			return 0, err
		}
		e, err := graph.NewEdgeSet(s.g)
		if err != nil {
			return 0, err
		}
		it := v.sub.Edges()
		for it.Next() {
			if err = ctx.Err(); err != nil {
				return 0, err
			}
			_ = e.Add(it.ID())
		}
		return s.put(object{edges: e, bytes: ebytes})
	}
	return 0, ErrABI
}

func (s *execution) options(labels []string) (query.Options, error) {
	o := query.Options{}
	if labels != nil {
		o.EdgeLabels = []graph.StringID{}
		for _, label := range labels {
			id, ok, err := s.g.FindString(label)
			if err != nil {
				return o, err
			}
			if ok {
				o.EdgeLabels = append(o.EdgeLabels, id)
			}
		}
	}
	return o, nil
}
func (s *execution) traverse(ctx context.Context, op uint32, input *graph.NodeSet, p abi.Params) (uint64, error) {
	direction := p.Direction
	switch op {
	case abi.Out, abi.OutE:
		direction = 0
	case abi.In, abi.InE:
		direction = 1
	case abi.Both, abi.BothE:
		direction = 2
	}
	if direction > 2 {
		return 0, ErrABI
	}
	edges := op == abi.OutE || op == abi.InE || op == abi.BothE
	recursive := op == abi.Reachable
	nbytes, ebytes := s.sizes()
	cost := nbytes
	if edges {
		cost = ebytes
	}
	var scratch uint64
	if recursive {
		scratch = s.g.Metadata().Nodes * 4
	}
	if err := s.budget(cost, scratch); err != nil {
		return 0, err
	}
	opts, err := s.options(p.Labels)
	if err != nil {
		return 0, err
	}
	allowed := make(map[graph.StringID]bool, len(opts.EdgeLabels))
	for _, l := range opts.EdgeLabels {
		allowed[l] = true
	}
	var n *graph.NodeSet
	var e *graph.EdgeSet
	if edges {
		e, err = graph.NewEdgeSet(s.g)
	} else {
		n, err = graph.NewNodeSet(s.g)
	}
	if err != nil {
		return 0, err
	}
	var queue []graph.NodeID
	if recursive {
		queue = make([]graph.NodeID, 0, int(s.g.Metadata().Nodes))
	}
	visit := func(id graph.NodeID) {
		if !n.Contains(id) {
			_ = n.Add(id)
			if recursive {
				queue = append(queue, id)
			}
		}
	}
	expand := func(id graph.NodeID) error {
		first, last := graph.Forward, graph.Reverse
		if direction < 2 {
			first = graph.Direction(direction)
			last = first
		}
		for dir := first; dir <= last; dir++ {
			it, err := s.g.Adjacent(id, dir)
			if err != nil {
				return err
			}
			for it.Next() {
				if err = ctx.Err(); err != nil {
					return err
				}
				edge := it.Edge()
				if opts.EdgeLabels != nil && !allowed[edge.Label] {
					continue
				}
				if edges {
					_ = e.Add(edge.ID)
				} else {
					visit(edge.Neighbor)
				}
			}
			if err = it.Err(); err != nil {
				return err
			}
		}
		return nil
	}
	it := input.Iterator()
	for it.Next() {
		if err = ctx.Err(); err != nil {
			return 0, err
		}
		if recursive {
			visit(it.ID())
		} else if err = expand(it.ID()); err != nil {
			return 0, err
		}
	}
	for head := 0; head < len(queue); head++ {
		if err = ctx.Err(); err != nil {
			return 0, err
		}
		if err = expand(queue[head]); err != nil {
			return 0, err
		}
	}
	return s.put(object{nodes: n, edges: e, bytes: cost})
}

func (s *execution) algebra(ctx context.Context, op uint32, a, b object) (uint64, error) {
	nbytes, ebytes := s.sizes()
	if a.nodes != nil && b.nodes != nil {
		if err := s.budget(nbytes, 0); err != nil {
			return 0, err
		}
		n, err := graph.NewNodeSet(s.g)
		if err != nil {
			return 0, err
		}
		it := a.nodes.Iterator()
		for it.Next() {
			if err = ctx.Err(); err != nil {
				return 0, err
			}
			has := b.nodes.Contains(it.ID())
			if op == abi.Union || (op == abi.Intersection && has) || (op == abi.Difference && !has) {
				_ = n.Add(it.ID())
			}
		}
		if op == abi.Union {
			it = b.nodes.Iterator()
			for it.Next() {
				if err = ctx.Err(); err != nil {
					return 0, err
				}
				_ = n.Add(it.ID())
			}
		}
		return s.put(object{nodes: n, bytes: nbytes})
	}
	if a.edges != nil && b.edges != nil {
		if err := s.budget(ebytes, 0); err != nil {
			return 0, err
		}
		e, err := graph.NewEdgeSet(s.g)
		if err != nil {
			return 0, err
		}
		it := a.edges.Iterator()
		for it.Next() {
			if err = ctx.Err(); err != nil {
				return 0, err
			}
			has := b.edges.Contains(it.ID())
			if op == abi.Union || (op == abi.Intersection && has) || (op == abi.Difference && !has) {
				_ = e.Add(it.ID())
			}
		}
		if op == abi.Union {
			it = b.edges.Iterator()
			for it.Next() {
				if err = ctx.Err(); err != nil {
					return 0, err
				}
				_ = e.Add(it.ID())
			}
		}
		return s.put(object{edges: e, bytes: ebytes})
	}
	return 0, ErrHandle
}
func (s *execution) filter(ctx context.Context, op uint32, v object, p abi.Params) (uint64, error) {
	nbytes, ebytes := s.sizes()
	var value graph.Value
	var err error
	if op == abi.Property {
		value, err = decodeValue(p.Value)
		if err != nil {
			return 0, err
		}
	}
	if v.nodes != nil {
		if op == abi.Property {
			if err = s.budget(nbytes, 0); err != nil {
				return 0, err
			}
			n, err := v.nodes.Has(ctx, p.Key, value)
			if err != nil {
				return 0, err
			}
			return s.put(object{nodes: n, bytes: nbytes})
		}
		if err = s.budget(nbytes, nbytes); err != nil {
			return 0, err
		}
		n, err := s.g.NodesWithLabel(ctx, p.Label)
		if err != nil {
			return 0, err
		}
		return s.algebra(ctx, abi.Intersection, v, object{nodes: n})
	}
	if v.edges != nil {
		if err = s.budget(ebytes, ebytes); err != nil {
			return 0, err
		}
		var e *graph.EdgeSet
		if op == abi.Label {
			e, err = s.g.EdgesWithLabel(ctx, p.Label)
		} else {
			e, err = s.g.EdgesWithProperty(ctx, p.Key, value)
		}
		if err != nil {
			return 0, err
		}
		return s.algebra(ctx, abi.Intersection, v, object{edges: e})
	}
	return 0, ErrHandle
}
