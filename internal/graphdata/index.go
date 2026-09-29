package graphdata

import (
	"context"
	"encoding/binary"
	"sort"
)

type LabelEntry struct {
	Owner, Label uint32
	Start, Count uint64
}
type LabelEntries struct {
	Heap  []LabelEntry
	Bytes []byte
}

func (c LabelEntries) Len() uint64 {
	if c.Bytes != nil {
		return uint64(len(c.Bytes) / 24)
	}
	return uint64(len(c.Heap))
}
func (c LabelEntries) At(i uint64) LabelEntry {
	if c.Bytes != nil {
		b := c.Bytes[i*24:]
		return LabelEntry{binary.LittleEndian.Uint32(b), binary.LittleEndian.Uint32(b[4:]), binary.LittleEndian.Uint64(b[8:]), binary.LittleEndian.Uint64(b[16:])}
	}
	return c.Heap[i]
}

type PropertyEntry struct {
	Owner uint8
	Property
	Start, Count uint64
}
type PropertyEntries struct {
	Heap  []PropertyEntry
	Bytes []byte
}

func (c PropertyEntries) Len() uint64 {
	if c.Bytes != nil {
		return uint64(len(c.Bytes) / 32)
	}
	return uint64(len(c.Heap))
}
func (c PropertyEntries) At(i uint64) PropertyEntry {
	if c.Bytes != nil {
		b := c.Bytes[i*32:]
		return PropertyEntry{b[0], Property{binary.LittleEndian.Uint32(b[4:]), b[1], binary.LittleEndian.Uint64(b[8:])}, binary.LittleEndian.Uint64(b[16:]), binary.LittleEndian.Uint64(b[24:])}
	}
	return c.Heap[i]
}
func HasU32(c U32, a, b uint64, value uint32) bool {
	i := sort.Search(int(b-a), func(i int) bool { return c.At(a+uint64(i)) >= value })
	return uint64(i) < b-a && c.At(a+uint64(i)) == value
}
func BuildIndexes(ctx context.Context, d *Data) error {
	type membership struct {
		owner    uint8
		property Property
		id       uint32
	}
	var labels, props []membership
	for owner, total := range []uint64{d.NodeIDs.Len(), d.EdgeIDs.Len()} {
		for id := uint64(0); id < total; id++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			off, values := d.NodePropOffsets, d.NodeProps
			if owner == 0 {
				for j := d.NodeLabelOffsets.At(id); j < d.NodeLabelOffsets.At(id+1); j++ {
					labels = append(labels, membership{0, Property{Key: d.NodeLabels.At(j)}, uint32(id)})
				}
			} else {
				labels = append(labels, membership{1, Property{Key: d.EdgeLabels.At(id)}, uint32(id)})
				off, values = d.EdgePropOffsets, d.EdgeProps
			}
			for j := off.At(id); j < off.At(id+1); j++ {
				p := values.At(j)
				if HasU32(d.IndexedKeys, 0, d.IndexedKeys.Len(), p.Key) {
					props = append(props, membership{uint8(owner), p, uint32(id)})
				}
			}
		}
	}
	less := func(a, b membership) bool {
		if a.owner != b.owner {
			return a.owner < b.owner
		}
		if a.property != b.property {
			return LessProperty(a.property, b.property)
		}
		return a.id < b.id
	}
	sort.Slice(labels, func(i, j int) bool { return less(labels[i], labels[j]) })
	sort.Slice(props, func(i, j int) bool { return less(props[i], props[j]) })
	d.LabelIndex = LabelEntries{}
	d.LabelPostings = U32{}
	d.PropertyIndex = PropertyEntries{}
	d.PropertyPostings = U32{}
	for _, m := range labels {
		n := len(d.LabelIndex.Heap)
		if n == 0 || d.LabelIndex.Heap[n-1].Owner != uint32(m.owner) || d.LabelIndex.Heap[n-1].Label != m.property.Key {
			d.LabelIndex.Heap = append(d.LabelIndex.Heap, LabelEntry{Owner: uint32(m.owner), Label: m.property.Key, Start: uint64(len(d.LabelPostings.Heap))})
			n++
		}
		d.LabelIndex.Heap[n-1].Count++
		d.LabelPostings.Heap = append(d.LabelPostings.Heap, m.id)
	}
	for _, m := range props {
		n := len(d.PropertyIndex.Heap)
		if n == 0 || d.PropertyIndex.Heap[n-1].Owner != m.owner || d.PropertyIndex.Heap[n-1].Property != m.property {
			d.PropertyIndex.Heap = append(d.PropertyIndex.Heap, PropertyEntry{Owner: m.owner, Property: m.property, Start: uint64(len(d.PropertyPostings.Heap))})
			n++
		}
		d.PropertyIndex.Heap[n-1].Count++
		d.PropertyPostings.Heap = append(d.PropertyPostings.Heap, m.id)
	}
	return ctx.Err()
}
func validateIndexes(ctx context.Context, d *Data) error {
	for i := uint64(0); i < d.IndexedKeys.Len(); i++ {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		k := d.IndexedKeys.At(i)
		if k == 0 || uint64(k) >= d.StringCount() || (i > 0 && d.IndexedKeys.At(i-1) >= k) {
			return invalid("indexed keys")
		}
	}
	var total uint64
	for i := uint64(0); i < d.LabelIndex.Len(); i++ {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		v := d.LabelIndex.At(i)
		if v.Owner > 1 || v.Label == 0 || uint64(v.Label) >= d.StringCount() || v.Start != total || v.Count == 0 || v.Start > d.LabelPostings.Len() || v.Count > d.LabelPostings.Len()-v.Start {
			return invalid("label group")
		}
		if i > 0 {
			prev := d.LabelIndex.At(i - 1)
			if prev.Owner > v.Owner || (prev.Owner == v.Owner && prev.Label >= v.Label) {
				return invalid("label group order")
			}
		}
		for j := uint64(0); j < v.Count; j++ {
			if j%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			id := d.LabelPostings.At(v.Start + j)
			if j > 0 && d.LabelPostings.At(v.Start+j-1) >= id {
				return invalid("label posting order")
			}
			if v.Owner == 0 {
				if uint64(id) >= d.NodeIDs.Len() || !HasU32(d.NodeLabels, d.NodeLabelOffsets.At(uint64(id)), d.NodeLabelOffsets.At(uint64(id)+1), v.Label) {
					return invalid("node label membership")
				}
			} else if uint64(id) >= d.EdgeIDs.Len() || d.EdgeLabels.At(uint64(id)) != v.Label {
				return invalid("edge label membership")
			}
		}
		total += v.Count
	}
	if total != d.LabelPostings.Len() || total != d.NodeLabels.Len()+d.EdgeIDs.Len() {
		return invalid("label coverage")
	}
	var expected uint64
	for _, p := range []Properties{d.NodeProps, d.EdgeProps} {
		for i := uint64(0); i < p.Len(); i++ {
			if i%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if HasU32(d.IndexedKeys, 0, d.IndexedKeys.Len(), p.At(i).Key) {
				expected++
			}
		}
	}
	total = 0
	for i := uint64(0); i < d.PropertyIndex.Len(); i++ {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		v := d.PropertyIndex.At(i)
		if v.Owner > 1 || !HasU32(d.IndexedKeys, 0, d.IndexedKeys.Len(), v.Key) || !ValidPayload(v.Property, d.StringCount()) || v.Start != total || v.Count == 0 || v.Start > d.PropertyPostings.Len() || v.Count > d.PropertyPostings.Len()-v.Start {
			return invalid("property group")
		}
		if i > 0 {
			p := d.PropertyIndex.At(i - 1)
			if p.Owner > v.Owner || (p.Owner == v.Owner && !LessProperty(p.Property, v.Property)) {
				return invalid("property group order")
			}
		}
		off, props, count := d.NodePropOffsets, d.NodeProps, d.NodeIDs.Len()
		if v.Owner == 1 {
			off, props, count = d.EdgePropOffsets, d.EdgeProps, d.EdgeIDs.Len()
		}
		for j := uint64(0); j < v.Count; j++ {
			if j%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			id := uint64(d.PropertyPostings.At(v.Start + j))
			if id >= count || (j > 0 && uint64(d.PropertyPostings.At(v.Start+j-1)) >= id) {
				return invalid("property posting id")
			}
			a, b := off.At(id), off.At(id+1)
			k := sort.Search(int(b-a), func(k int) bool { return !LessProperty(props.At(a+uint64(k)), v.Property) })
			if uint64(k) >= b-a || props.At(a+uint64(k)) != v.Property {
				return invalid("property membership")
			}
		}
		total += v.Count
	}
	if total != d.PropertyPostings.Len() || total != expected {
		return invalid("property coverage")
	}
	return nil
}
