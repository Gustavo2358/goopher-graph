package ingest

import (
	"gophergraph/graph"
	"gophergraph/internal/graphdata"
	"math"
	"sort"
)

func payload(v graph.Value, sids map[string]uint32) uint64 {
	if s, ok := v.Text(); ok {
		return uint64(sids[s])
	}
	switch v.Kind() {
	case graph.BoolKind:
		b, _ := v.Bool()
		if b {
			return 1
		}
	case graph.ByteKind, graph.ShortKind, graph.IntKind, graph.LongKind:
		n, _ := v.Int64()
		return uint64(n)
	case graph.FloatKind:
		f, _ := v.Float64()
		return uint64(math.Float32bits(float32(f)))
	case graph.DoubleKind:
		f, _ := v.Float64()
		return math.Float64bits(f)
	}
	return 0
}
func (b *builder) canonicalize(keys []string) (*graph.Graph, error) {
	stringsSet := map[string]bool{"": true}
	for _, key := range keys {
		stringsSet[key] = true
	}
	for _, owners := range []map[string]*entity{b.nodes, b.edges} {
		for _, e := range owners {
			if err := b.ctx.Err(); err != nil {
				return nil, err
			}
			stringsSet[e.id] = true
			for l := range e.labels {
				stringsSet[l] = true
			}
			for key, g := range e.props {
				stringsSet[key] = true
				for v := range g.values {
					if s, ok := v.Text(); ok {
						stringsSet[s] = true
					}
				}
			}
		}
	}
	if uint64(len(stringsSet)) > math.MaxUint32 {
		return nil, structuralError("string capacity")
	}
	ss := make([]string, 0, len(stringsSet))
	for s := range stringsSet {
		ss = append(ss, s)
	}
	sort.Strings(ss)
	sids := make(map[string]uint32, len(ss))
	for i, s := range ss {
		sids[s] = uint32(i)
	}
	d := graphdata.Empty()
	d.Strings = ss
	d.Partial = b.report.Completeness == Partial
	nids := map[string]uint32{}
	props := func(e *entity) []graphdata.Property {
		var p []graphdata.Property
		for key, g := range e.props {
			for v := range g.values {
				p = append(p, graphdata.Property{Key: sids[key], Kind: uint8(v.Kind()), Payload: payload(v, sids)})
			}
		}
		sort.Slice(p, func(i, j int) bool { return graphdata.LessProperty(p[i], p[j]) })
		return p
	}
	for _, id := range sortedEntities(b.nodes) {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		e := b.nodes[id]
		nids[id] = uint32(len(d.NodeIDs.Heap))
		d.NodeIDs.Heap = append(d.NodeIDs.Heap, sids[id])
		var labels []uint32
		for label := range e.labels {
			labels = append(labels, sids[label])
		}
		sort.Slice(labels, func(i, j int) bool { return labels[i] < labels[j] })
		d.NodeLabels.Heap = append(d.NodeLabels.Heap, labels...)
		d.NodeLabelOffsets.Heap = append(d.NodeLabelOffsets.Heap, uint64(len(d.NodeLabels.Heap)))
		d.NodeProps.Heap = append(d.NodeProps.Heap, props(e)...)
		d.NodePropOffsets.Heap = append(d.NodePropOffsets.Heap, uint64(len(d.NodeProps.Heap)))
	}
	for _, id := range sortedEntities(b.edges) {
		if err := b.ctx.Err(); err != nil {
			return nil, err
		}
		e := b.edges[id]
		d.EdgeIDs.Heap = append(d.EdgeIDs.Heap, sids[id])
		d.Sources.Heap = append(d.Sources.Heap, nids[e.source])
		d.Targets.Heap = append(d.Targets.Heap, nids[e.target])
		d.EdgeLabels.Heap = append(d.EdgeLabels.Heap, sids[e.label])
		d.EdgeProps.Heap = append(d.EdgeProps.Heap, props(e)...)
		d.EdgePropOffsets.Heap = append(d.EdgePropOffsets.Heap, uint64(len(d.EdgeProps.Heap)))
	}
	keySet := map[uint32]bool{}
	for _, key := range keys {
		keySet[sids[key]] = true
	}
	for key := range keySet {
		d.IndexedKeys.Heap = append(d.IndexedKeys.Heap, key)
	}
	sort.Slice(d.IndexedKeys.Heap, func(i, j int) bool { return d.IndexedKeys.Heap[i] < d.IndexedKeys.Heap[j] })
	graphdata.BuildCSR(d)
	if err := graphdata.BuildIndexes(b.ctx, d); err != nil {
		return nil, err
	}
	if e := b.ctx.Err(); e != nil {
		return nil, e
	}
	return graph.New(d, nil)
}
