// Package graphdata holds the graph's private column representation.
package graphdata

import (
	"encoding/binary"
	"sort"
)

type U32 struct {
	Heap  []uint32
	Bytes []byte
}

func (c U32) Len() uint64 {
	if c.Bytes != nil {
		return uint64(len(c.Bytes) / 4)
	}
	return uint64(len(c.Heap))
}
func (c U32) At(i uint64) uint32 {
	if c.Bytes != nil {
		return binary.LittleEndian.Uint32(c.Bytes[i*4:])
	}
	return c.Heap[i]
}

type U64 struct {
	Heap  []uint64
	Bytes []byte
}

func (c U64) Len() uint64 {
	if c.Bytes != nil {
		return uint64(len(c.Bytes) / 8)
	}
	return uint64(len(c.Heap))
}
func (c U64) At(i uint64) uint64 {
	if c.Bytes != nil {
		return binary.LittleEndian.Uint64(c.Bytes[i*8:])
	}
	return c.Heap[i]
}

type Property struct {
	Key     uint32
	Kind    uint8
	Payload uint64
}
type Properties struct {
	Heap  []Property
	Bytes []byte
}

func (c Properties) Len() uint64 {
	if c.Bytes != nil {
		return uint64(len(c.Bytes) / 16)
	}
	return uint64(len(c.Heap))
}
func (c Properties) At(i uint64) Property {
	if c.Bytes != nil {
		b := c.Bytes[i*16:]
		return Property{binary.LittleEndian.Uint32(b), b[4], binary.LittleEndian.Uint64(b[8:])}
	}
	return c.Heap[i]
}

type Data struct {
	Strings                                                        []string
	StringOffsets                                                  U64
	StringBytes                                                    []byte
	NodeIDs, NodeLabels                                            U32
	NodeLabelOffsets, NodePropOffsets                              U64
	NodeProps                                                      Properties
	EdgeIDs, Sources, Targets, EdgeLabels                          U32
	EdgePropOffsets                                                U64
	EdgeProps                                                      Properties
	ForwardOffsets, ReverseOffsets                                 U64
	ForwardNeighbors, ForwardEdges, ReverseNeighbors, ReverseEdges U32
	Partial                                                        bool
}

func (d *Data) StringCount() uint64 {
	if d.Strings != nil {
		return uint64(len(d.Strings))
	}
	if d.StringOffsets.Len() == 0 {
		return 0
	}
	return d.StringOffsets.Len() - 1
}
func (d *Data) String(i uint32) string {
	if d.Strings != nil {
		return d.Strings[i]
	}
	return string(d.StringBytes[d.StringOffsets.At(uint64(i)):d.StringOffsets.At(uint64(i)+1)])
}
func Empty() *Data {
	return &Data{Strings: []string{""}, NodeLabelOffsets: U64{Heap: []uint64{0}}, NodePropOffsets: U64{Heap: []uint64{0}}, EdgePropOffsets: U64{Heap: []uint64{0}}, ForwardOffsets: U64{Heap: []uint64{0}}, ReverseOffsets: U64{Heap: []uint64{0}}}
}

// BuildCSR constructs both directions from the immutable edge columns.
func BuildCSR(d *Data) {
	build := func(src, dst U32) (U64, U32, U32) {
		n := d.NodeIDs.Len()
		off := make([]uint64, n+1)
		for i := uint64(0); i < src.Len(); i++ {
			off[uint64(src.At(i))+1]++
		}
		for i := uint64(1); i <= n; i++ {
			off[i] += off[i-1]
		}
		next := append([]uint64(nil), off...)
		edges := make([]uint32, src.Len())
		for e := uint64(0); e < src.Len(); e++ {
			u := src.At(e)
			edges[next[u]] = uint32(e)
			next[u]++
		}
		neighbors := make([]uint32, len(edges))
		for u := uint64(0); u < n; u++ {
			a := edges[off[u]:off[u+1]]
			sort.Slice(a, func(i, j int) bool {
				x, y := a[i], a[j]
				if dst.At(uint64(x)) != dst.At(uint64(y)) {
					return dst.At(uint64(x)) < dst.At(uint64(y))
				}
				if d.EdgeLabels.At(uint64(x)) != d.EdgeLabels.At(uint64(y)) {
					return d.EdgeLabels.At(uint64(x)) < d.EdgeLabels.At(uint64(y))
				}
				return x < y
			})
		}
		for i, e := range edges {
			neighbors[i] = dst.At(uint64(e))
		}
		return U64{Heap: off}, U32{Heap: neighbors}, U32{Heap: edges}
	}
	d.ForwardOffsets, d.ForwardNeighbors, d.ForwardEdges = build(d.Sources, d.Targets)
	d.ReverseOffsets, d.ReverseNeighbors, d.ReverseEdges = build(d.Targets, d.Sources)
}
