// Package graph provides read-only access to a directed property multigraph.
package graph

import (
	"context"
	"errors"
	"gophergraph/internal/graphdata"
	"sort"
)

type NodeID uint32
type EdgeID uint32
type StringID uint32

const InvalidNodeID NodeID = ^NodeID(0)
const InvalidEdgeID EdgeID = ^EdgeID(0)
const InvalidStringID StringID = ^StringID(0)

type Direction uint8

const (
	Forward Direction = iota
	Reverse
)

var (
	ErrNotFound      = errors.New("not found")
	ErrClosed        = errors.New("graph closed")
	ErrOutOfRange    = errors.New("out of range")
	ErrGraphMismatch = errors.New("graph mismatch")
)

type Metadata struct {
	Nodes, Edges uint64
	PartialLoad  bool
}
type Edge struct {
	ID             EdgeID
	Source, Target NodeID
	Label          StringID
	Neighbor       NodeID
}
type Property struct {
	Key   StringID
	Value Value
}
type Graph struct {
	data     *graphdata.Data
	metadata Metadata
	release  func() error
	closeErr error
}

// New transfers exclusive ownership of validated internal columns to a Graph.
// Only implementation packages can construct graphdata.Data.
func New(d *graphdata.Data, release func() error) (*Graph, error) {
	return NewContext(context.Background(), d, release)
}

// NewContext validates internal columns while respecting cancellation.
func NewContext(ctx context.Context, d *graphdata.Data, release func() error) (*Graph, error) {
	if err := graphdata.ValidateContext(ctx, d); err != nil {
		return nil, err
	}
	return &Graph{data: d, metadata: Metadata{d.NodeIDs.Len(), d.EdgeIDs.Len(), d.Partial}, release: release}, nil
}

// InternalColumns is the capability-local bridge used by the snapshot codec.
// Its parameter cannot be constructed by consumers outside this module.
func (g *Graph) InternalColumns(dst *graphdata.Data) error {
	if err := g.check(); err != nil {
		return err
	}
	if dst == nil {
		return ErrOutOfRange
	}
	*dst = *g.data
	return nil
}
func (g *Graph) check() error {
	if g == nil || g.data == nil {
		return ErrClosed
	}
	return nil
}
func (g *Graph) Metadata() Metadata {
	if g == nil {
		return Metadata{}
	}
	return g.metadata
}
func (g *Graph) Close() error {
	if g == nil {
		return nil
	}
	if g.data != nil {
		g.data = nil
		if g.release != nil {
			g.closeErr = g.release()
			g.release = nil
		}
	}
	return g.closeErr
}
func (g *Graph) String(id StringID) (string, error) {
	if e := g.check(); e != nil {
		return "", e
	}
	if uint64(id) >= g.data.StringCount() {
		return "", ErrOutOfRange
	}
	return g.data.String(uint32(id)), nil
}
func (g *Graph) FindString(s string) (StringID, bool, error) {
	if e := g.check(); e != nil {
		return InvalidStringID, false, e
	}
	i := sort.Search(int(g.data.StringCount()), func(i int) bool { return g.data.String(uint32(i)) >= s })
	if uint64(i) < g.data.StringCount() && g.data.String(uint32(i)) == s {
		return StringID(i), true, nil
	}
	return InvalidStringID, false, nil
}
func (g *Graph) find(s string, c graphdata.U32) (uint32, error) {
	i := sort.Search(int(c.Len()), func(i int) bool { return g.data.String(c.At(uint64(i))) >= s })
	if uint64(i) < c.Len() && g.data.String(c.At(uint64(i))) == s {
		return uint32(i), nil
	}
	return ^uint32(0), ErrNotFound
}
func (g *Graph) FindNode(s string) (NodeID, error) {
	if e := g.check(); e != nil {
		return InvalidNodeID, e
	}
	v, e := g.find(s, g.data.NodeIDs)
	return NodeID(v), e
}
func (g *Graph) FindEdge(s string) (EdgeID, error) {
	if e := g.check(); e != nil {
		return InvalidEdgeID, e
	}
	v, e := g.find(s, g.data.EdgeIDs)
	return EdgeID(v), e
}
func (g *Graph) NodeExternalID(id NodeID) (string, error) {
	if e := g.check(); e != nil {
		return "", e
	}
	if uint64(id) >= g.metadata.Nodes {
		return "", ErrOutOfRange
	}
	return g.data.String(g.data.NodeIDs.At(uint64(id))), nil
}
func (g *Graph) EdgeExternalID(id EdgeID) (string, error) {
	if e := g.check(); e != nil {
		return "", e
	}
	if uint64(id) >= g.metadata.Edges {
		return "", ErrOutOfRange
	}
	return g.data.String(g.data.EdgeIDs.At(uint64(id))), nil
}
func (g *Graph) NodeLabels(id NodeID) ([]StringID, error) {
	if e := g.check(); e != nil {
		return nil, e
	}
	if uint64(id) >= g.metadata.Nodes {
		return nil, ErrOutOfRange
	}
	a, b := g.data.NodeLabelOffsets.At(uint64(id)), g.data.NodeLabelOffsets.At(uint64(id)+1)
	out := make([]StringID, b-a)
	for i := range out {
		out[i] = StringID(g.data.NodeLabels.At(a + uint64(i)))
	}
	return out, nil
}
func (g *Graph) Edge(id EdgeID) (Edge, error) {
	if e := g.check(); e != nil {
		return Edge{}, e
	}
	if uint64(id) >= g.metadata.Edges {
		return Edge{}, ErrOutOfRange
	}
	i := uint64(id)
	d := g.data
	return Edge{id, NodeID(d.Sources.At(i)), NodeID(d.Targets.At(i)), StringID(d.EdgeLabels.At(i)), NodeID(d.Targets.At(i))}, nil
}

type EdgeIterator struct {
	g         *Graph
	direction Direction
	pos, end  uint64
	value     Edge
	err       error
}

func (g *Graph) Adjacent(id NodeID, direction Direction) (EdgeIterator, error) {
	if e := g.check(); e != nil {
		return EdgeIterator{}, e
	}
	if uint64(id) >= g.metadata.Nodes || direction > Reverse {
		return EdgeIterator{}, ErrOutOfRange
	}
	off := g.data.ForwardOffsets
	if direction == Reverse {
		off = g.data.ReverseOffsets
	}
	return EdgeIterator{g: g, direction: direction, pos: off.At(uint64(id)), end: off.At(uint64(id) + 1)}, nil
}
func (it *EdgeIterator) Next() bool {
	if it.err != nil {
		return false
	}
	if it.g == nil {
		it.err = ErrClosed
		return false
	}
	if e := it.g.check(); e != nil {
		it.err = e
		return false
	}
	if it.pos >= it.end {
		return false
	}
	d := it.g.data
	edges, neighbors := d.ForwardEdges, d.ForwardNeighbors
	if it.direction == Reverse {
		edges, neighbors = d.ReverseEdges, d.ReverseNeighbors
	}
	it.value, _ = it.g.Edge(EdgeID(edges.At(it.pos)))
	it.value.Neighbor = NodeID(neighbors.At(it.pos))
	it.pos++
	return true
}
func (it *EdgeIterator) Edge() Edge { return it.value }
func (it *EdgeIterator) Err() error { return it.err }

type PropertyIterator struct {
	g        *Graph
	edge     bool
	pos, end uint64
	value    Property
	err      error
}

func (g *Graph) NodeProperties(id NodeID) (PropertyIterator, error) {
	return g.properties(uint64(id), false)
}
func (g *Graph) EdgeProperties(id EdgeID) (PropertyIterator, error) {
	return g.properties(uint64(id), true)
}
func (g *Graph) properties(id uint64, edge bool) (PropertyIterator, error) {
	if e := g.check(); e != nil {
		return PropertyIterator{}, e
	}
	off, count := g.data.NodePropOffsets, g.metadata.Nodes
	if edge {
		off, count = g.data.EdgePropOffsets, g.metadata.Edges
	}
	if id >= count {
		return PropertyIterator{}, ErrOutOfRange
	}
	return PropertyIterator{g: g, edge: edge, pos: off.At(id), end: off.At(id + 1)}, nil
}
func (it *PropertyIterator) Next() bool {
	if it.err != nil {
		return false
	}
	if it.g == nil {
		it.err = ErrClosed
		return false
	}
	if e := it.g.check(); e != nil {
		it.err = e
		return false
	}
	if it.pos >= it.end {
		return false
	}
	props := it.g.data.NodeProps
	if it.edge {
		props = it.g.data.EdgeProps
	}
	p := props.At(it.pos)
	v := Value{kind: ValueKind(p.Kind), payload: p.Payload}
	if p.Kind >= 8 {
		v.payload = 0
		v.text = it.g.data.String(uint32(p.Payload))
	}
	it.value = Property{StringID(p.Key), v}
	it.pos++
	return true
}
func (it *PropertyIterator) Property() Property { return it.value }
func (it *PropertyIterator) Err() error         { return it.err }
