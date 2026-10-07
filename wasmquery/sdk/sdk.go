// Package sdk is the Go authoring API for wasip1/wasm graph queries.
// Sets are opaque, immutable host handles. Call Return once as the last graph
// operation. Errors are sticky and exposed through Err and Return.
package sdk

import (
	"encoding/json"
	"errors"
	"gophergraph/wasmquery/internal/abi"
)

var ErrHost = errors.New("graph host call failed (requires GopherGraph WASM runtime)")
var ErrOwner = errors.New("sets belong to different SDK query sessions")

type Query struct{ err error }

func New() *Query { return &Query{} }
func (q *Query) Err() error {
	if q == nil {
		return ErrOwner
	}
	return q.err
}
func (q *Query) invoke(op uint32, a, b uint64, p *abi.Params, out []byte) uint64 {
	if q == nil || q.err != nil {
		return abi.Failure
	}
	var data string
	if p != nil {
		encoded, err := json.Marshal(p)
		if err != nil {
			q.err = err
			return abi.Failure
		}
		if len(encoded) > abi.MaxMessage {
			q.err = errors.New("graph parameters exceed 64 KiB")
			return abi.Failure
		}
		data = string(encoded)
	}
	var ptr *byte
	if len(out) != 0 {
		ptr = &out[0]
	}
	result := call(op, a, b, data, ptr, uint32(len(out)))
	if result == abi.Failure {
		q.err = ErrHost
	}
	return result
}
func (q *Query) same(other *Query) bool {
	if q == nil {
		return false
	}
	if q != other {
		q.err = ErrOwner
		return false
	}
	return true
}

type NodeSet struct {
	q      *Query
	handle uint64
}
type EdgeSet struct {
	q      *Query
	handle uint64
}
type Subgraph struct {
	q      *Query
	handle uint64
}
type Direction uint32

const (
	Forward Direction = iota
	Reverse
	Bidirectional
)

// Nodes selects external IDs; a missing ID fails the execution. Nodes() is empty.
func (q *Query) Nodes(ids ...string) NodeSet {
	return NodeSet{q, q.invoke(abi.Nodes, 0, 0, &abi.Params{IDs: ids}, nil)}
}
func (q *Query) NodesWithLabel(label string) NodeSet {
	return NodeSet{q, q.invoke(abi.NodesLabel, 0, 0, &abi.Params{Label: label}, nil)}
}

// NodesWithAnyLabelAndProperty selects (label1 OR label2 ...) AND property.
// Nil/empty labels select nothing; equality is typed and matches any set value.
// Requires a host with opcode 27 support.
func (q *Query) NodesWithAnyLabelAndProperty(labels []string, key string, value Value) NodeSet {
	return NodeSet{q, q.invoke(abi.NodesAnyLabelProperty, 0, 0, &abi.Params{Labels: labels, Key: key, Value: value}, nil)}
}
func (s NodeSet) nodes(op uint32, p *abi.Params) NodeSet {
	return NodeSet{s.q, s.q.invoke(op, s.handle, 0, p, nil)}
}
func (s NodeSet) edges(op uint32, labels []string) EdgeSet {
	return EdgeSet{s.q, s.q.invoke(op, s.handle, 0, &abi.Params{Labels: labels}, nil)}
}
func (s NodeSet) Out(labels ...string) NodeSet { return s.nodes(abi.Out, &abi.Params{Labels: labels}) }
func (s NodeSet) In(labels ...string) NodeSet  { return s.nodes(abi.In, &abi.Params{Labels: labels}) }
func (s NodeSet) Both(labels ...string) NodeSet {
	return s.nodes(abi.Both, &abi.Params{Labels: labels})
}
func (s NodeSet) OutE(labels ...string) EdgeSet  { return s.edges(abi.OutE, labels) }
func (s NodeSet) InE(labels ...string) EdgeSet   { return s.edges(abi.InE, labels) }
func (s NodeSet) BothE(labels ...string) EdgeSet { return s.edges(abi.BothE, labels) }
func (s NodeSet) HasLabel(label string) NodeSet  { return s.nodes(abi.Label, &abi.Params{Label: label}) }
func (s NodeSet) Has(key string, value Value) NodeSet {
	return s.nodes(abi.Property, &abi.Params{Key: key, Value: value})
}
func (s NodeSet) combine(op uint32, other NodeSet) NodeSet {
	if !s.q.same(other.q) {
		return NodeSet{s.q, 0}
	}
	return NodeSet{s.q, s.q.invoke(op, s.handle, other.handle, nil, nil)}
}
func (s NodeSet) Union(other NodeSet) NodeSet        { return s.combine(abi.Union, other) }
func (s NodeSet) Intersection(other NodeSet) NodeSet { return s.combine(abi.Intersection, other) }
func (s NodeSet) Difference(other NodeSet) NodeSet   { return s.combine(abi.Difference, other) }

// Reachable includes the seeds and expands each visited node once.
func (s NodeSet) Reachable(direction Direction, labels ...string) NodeSet {
	return s.nodes(abi.Reachable, &abi.Params{Direction: uint32(direction), Labels: labels})
}
func (s NodeSet) Territory(labels ...string) NodeSet     { return s.Reachable(Forward, labels...) }
func (s NodeSet) AntiTerritory(labels ...string) NodeSet { return s.Reachable(Reverse, labels...) }

// Induced includes all matching edges with both endpoints in this set.
func (s NodeSet) Induced(labels ...string) Subgraph {
	return Subgraph{s.q, s.q.invoke(abi.Induced, s.handle, 0, &abi.Params{Labels: labels}, nil)}
}
func (s NodeSet) Count() uint64 { return s.q.invoke(abi.Count, s.handle, 0, nil, nil) }
func (s NodeSet) Empty() bool   { return s.q.invoke(abi.Empty, s.handle, 0, nil, nil) == 1 }
func (s NodeSet) Release()      { s.q.invoke(abi.Release, s.handle, 0, nil, nil) }

func (s EdgeSet) Sources() NodeSet {
	return NodeSet{s.q, s.q.invoke(abi.Sources, s.handle, 0, nil, nil)}
}
func (s EdgeSet) Targets() NodeSet {
	return NodeSet{s.q, s.q.invoke(abi.Targets, s.handle, 0, nil, nil)}
}
func (s EdgeSet) HasLabel(label string) EdgeSet {
	return EdgeSet{s.q, s.q.invoke(abi.Label, s.handle, 0, &abi.Params{Label: label}, nil)}
}
func (s EdgeSet) Has(key string, value Value) EdgeSet {
	return EdgeSet{s.q, s.q.invoke(abi.Property, s.handle, 0, &abi.Params{Key: key, Value: value}, nil)}
}
func (s EdgeSet) combine(op uint32, other EdgeSet) EdgeSet {
	if !s.q.same(other.q) {
		return EdgeSet{s.q, 0}
	}
	return EdgeSet{s.q, s.q.invoke(op, s.handle, other.handle, nil, nil)}
}
func (s EdgeSet) Union(other EdgeSet) EdgeSet        { return s.combine(abi.Union, other) }
func (s EdgeSet) Intersection(other EdgeSet) EdgeSet { return s.combine(abi.Intersection, other) }
func (s EdgeSet) Difference(other EdgeSet) EdgeSet   { return s.combine(abi.Difference, other) }
func (s EdgeSet) Count() uint64                      { return s.q.invoke(abi.Count, s.handle, 0, nil, nil) }
func (s EdgeSet) Empty() bool                        { return s.q.invoke(abi.Empty, s.handle, 0, nil, nil) == 1 }
func (s EdgeSet) Release()                           { s.q.invoke(abi.Release, s.handle, 0, nil, nil) }

// Subgraph combines explicit sets and adds every selected edge's endpoints.
func (q *Query) Subgraph(nodes NodeSet, edges EdgeSet) Subgraph {
	if !q.same(nodes.q) || !q.same(edges.q) {
		return Subgraph{q, 0}
	}
	return Subgraph{q, q.invoke(abi.Subgraph, nodes.handle, edges.handle, nil, nil)}
}
func (s Subgraph) Nodes() NodeSet {
	return NodeSet{s.q, s.q.invoke(abi.SubNodes, s.handle, 0, nil, nil)}
}
func (s Subgraph) Edges() EdgeSet {
	return EdgeSet{s.q, s.q.invoke(abi.SubEdges, s.handle, 0, nil, nil)}
}
func (s Subgraph) NodeCount() uint64 { return s.q.invoke(abi.Count, s.handle, 0, nil, nil) }
func (s Subgraph) EdgeCount() uint64 { return s.q.invoke(abi.Count, s.handle, 1, nil, nil) }
func (s Subgraph) Empty() bool       { return s.q.invoke(abi.Empty, s.handle, 0, nil, nil) == 1 }
func (s Subgraph) Release()          { s.q.invoke(abi.Release, s.handle, 0, nil, nil) }
func (q *Query) Return(s Subgraph) error {
	if q.same(s.q) {
		q.invoke(abi.Return, s.handle, 0, nil, nil)
	}
	return q.Err()
}

// NodeProperty/EdgeProperty read one typed value. Index selects the zero-based
// value within a property's canonical ordering, preserving node set properties.
// A missing key/index gives found=false. A missing entity is an execution error.
func (q *Query) read(op uint32, id, key string, index uint32) (Value, bool) {
	out := make([]byte, abi.MaxMessage)
	n := q.invoke(op, 0, 0, &abi.Params{ID: id, Key: key, Index: index}, out)
	if n == abi.Failure {
		return Value{}, false
	}
	var result abi.PropertyResult
	if n > uint64(len(out)) {
		q.err = ErrHost
		return Value{}, false
	}
	if err := json.Unmarshal(out[:n], &result); err != nil {
		q.err = err
		return Value{}, false
	}
	return result.Value, result.Found
}
func (q *Query) NodeProperty(id, key string, index uint32) (Value, bool) {
	return q.read(abi.ReadNodeProperty, id, key, index)
}
func (q *Query) EdgeProperty(id, key string, index uint32) (Value, bool) {
	return q.read(abi.ReadEdgeProperty, id, key, index)
}
