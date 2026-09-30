package ingest

import (
	"bytes"
	"errors"
	"gophergraph/graph"
	"gophergraph/ingest/ports"
	"gophergraph/internal/graphdata"
	"io"
	"math"
	"sort"
)

// These are logical columns, in the same order as graphdata.Data's byte views.
// Snapshot encoding/publication remains the responsibility of snapshot.Write.
const (
	cStringOffsets = iota
	cStrings
	cNodeIDs
	cNodeLabelOffsets
	cNodeLabels
	cNodePropOffsets
	cNodeProps
	cEdgeIDs
	cSources
	cTargets
	cEdgeLabels
	cEdgePropOffsets
	cEdgeProps
	cForwardOffsets
	cForwardNeighbors
	cForwardEdges
	cReverseOffsets
	cReverseNeighbors
	cReverseEdges
	cLabelIndex
	cLabelPostings
	cIndexedKeys
	cPropertyIndex
	cPropertyPostings
	columnCount
)

func (d *diskBuilder) canonicalize(keys []string) (g *graph.Graph, err error) {
	var cols [columnCount]*spillFile
	for i := range cols {
		cols[i], err = newSpill(d.ws)
		if err != nil {
			return
		}
	}
	var mappings []ports.Mapping
	defer func() {
		if err != nil {
			err = errors.Join(err, closeMappings(mappings))
		}
	}()
	dict, err := d.dictionary(cols[cStringOffsets], cols[cStrings], &mappings)
	if err != nil {
		return nil, err
	}
	sid := func(s string) (uint32, error) {
		id, ok := dict.find(s)
		if !ok {
			return 0, errors.New("missing scratch dictionary entry")
		}
		return id, nil
	}
	keySet := make(map[uint32]bool, len(keys))
	for _, key := range keys {
		var id uint32
		id, err = sid(key)
		if err != nil {
			return
		}
		keySet[id] = true
	}
	keyIDs := make([]uint32, 0, len(keySet))
	for k := range keySet {
		keyIDs = append(keyIDs, k)
	}
	sort.Slice(keyIDs, func(i, j int) bool { return keyIDs[i] < keyIDs[j] })
	for _, k := range keyIDs {
		if err = cols[cIndexedKeys].u32(k); err != nil {
			return
		}
	}
	var runs [4]*spillFile // labels, properties, forward, reverse
	for i := range runs {
		runs[i], err = newSpill(d.ws)
		if err != nil {
			return
		}
	}
	for role := ports.Nodes; role <= ports.Edges; role++ {
		if err = d.entityColumns(role, cols, runs, keySet, sid); err != nil {
			return
		}
		if err = d.ws.Remove(d.accepted[role].file); err != nil {
			return
		}
	}
	err = closeMappings(d.nodeMaps)
	d.nodeMaps = nil
	d.nodes = diskDictionary{}
	if err != nil {
		return
	}
	for kind, input := range runs {
		var sorted *spillFile
		sorted, err = externalSort(d.b.ctx, d.ws, input, d.budget)
		if err != nil {
			return
		}
		if kind < 2 {
			err = d.postingColumns(kind, sorted, cols)
		} else {
			err = d.csrColumns(kind, sorted, cols)
		}
		if err != nil {
			return
		}
		if err = d.ws.Remove(sorted.file); err != nil {
			return
		}
	}
	var raw [columnCount][]byte
	raw[cStringOffsets], raw[cStrings] = dict.offsets, dict.data
	for i := 2; i < columnCount; i++ {
		raw[i], err = mapSpill(d.b.ctx, d.ws, cols[i], &mappings)
		if err != nil {
			return
		}
	}
	u32 := func(i int) graphdata.U32 { return graphdata.U32{Bytes: raw[i]} }
	u64 := func(i int) graphdata.U64 { return graphdata.U64{Bytes: raw[i]} }
	data := &graphdata.Data{
		StringOffsets: u64(cStringOffsets), StringBytes: raw[cStrings], NodeIDs: u32(cNodeIDs), NodeLabels: u32(cNodeLabels), NodeLabelOffsets: u64(cNodeLabelOffsets), NodePropOffsets: u64(cNodePropOffsets), NodeProps: graphdata.Properties{Bytes: raw[cNodeProps]},
		EdgeIDs: u32(cEdgeIDs), Sources: u32(cSources), Targets: u32(cTargets), EdgeLabels: u32(cEdgeLabels), EdgePropOffsets: u64(cEdgePropOffsets), EdgeProps: graphdata.Properties{Bytes: raw[cEdgeProps]},
		ForwardOffsets: u64(cForwardOffsets), ForwardNeighbors: u32(cForwardNeighbors), ForwardEdges: u32(cForwardEdges), ReverseOffsets: u64(cReverseOffsets), ReverseNeighbors: u32(cReverseNeighbors), ReverseEdges: u32(cReverseEdges),
		LabelIndex: graphdata.LabelEntries{Bytes: raw[cLabelIndex]}, LabelPostings: u32(cLabelPostings), IndexedKeys: u32(cIndexedKeys), PropertyIndex: graphdata.PropertyEntries{Bytes: raw[cPropertyIndex]}, PropertyPostings: u32(cPropertyPostings), Partial: d.b.report.Completeness == Partial,
	}
	return graph.NewContext(d.b.ctx, data, func() error { return closeMappings(mappings) })
}
func (d *diskBuilder) dictionary(off, text *spillFile, mappings *[]ports.Mapping) (dict diskDictionary, err error) {
	sorted, err := externalSort(d.b.ctx, d.ws, d.strings, d.budget)
	if err != nil {
		return
	}
	r := readSpill(sorted, 0, sorted.size)
	var prev []byte
	var count uint64
	for {
		if err = d.b.ctx.Err(); err != nil {
			return
		}
		err = r.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return
		}
		if count > 0 && bytes.Equal(prev, r.row.key) {
			continue
		}
		if count >= math.MaxUint32 {
			return dict, structuralError("string capacity")
		}
		count++
		if err = off.u64(uint64(text.size)); err != nil {
			return
		}
		if err = text.write(r.row.key); err != nil {
			return
		}
		prev = append(prev[:0], r.row.key...)
	}
	if err = off.u64(uint64(text.size)); err != nil {
		return
	}
	if err = d.ws.Remove(sorted.file); err != nil {
		return
	}
	dict.offsets, err = mapSpill(d.b.ctx, d.ws, off, mappings)
	if err != nil {
		return
	}
	dict.data, err = mapSpill(d.b.ctx, d.ws, text, mappings)
	return
}
func (d *diskBuilder) entityColumns(role ports.Role, c [columnCount]*spillFile, runs [4]*spillFile, indexed map[uint32]bool, sid func(string) (uint32, error)) error {
	input := d.accepted[role]
	r := readSpill(input, 0, input.size)
	idCol, propOff, propCol := cNodeIDs, cNodePropOffsets, cNodeProps
	if role == ports.Edges {
		idCol, propOff, propCol = cEdgeIDs, cEdgePropOffsets, cEdgeProps
	}
	if err := c[propOff].u64(0); err != nil {
		return err
	}
	if role == ports.Nodes {
		if err := c[cNodeLabelOffsets].u64(0); err != nil {
			return err
		}
	}
	var count uint64
	finish := func() error {
		if err := c[propOff].u64(uint64(c[propCol].size / 16)); err != nil {
			return err
		}
		if role == ports.Nodes {
			return c[cNodeLabelOffsets].u64(uint64(c[cNodeLabels].size / 4))
		}
		return nil
	}
	labelPosting := func(label uint32) error {
		return runs[0].row(big32(big32([]byte{byte(role)}, label), uint32(count-1)), nil)
	}
	for {
		if err := d.b.ctx.Err(); err != nil {
			return err
		}
		err := r.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		id, tag, rest, err := token(r.row.key)
		if err != nil {
			return err
		}
		switch tag {
		case 0:
			if count > 0 {
				if err = finish(); err != nil {
					return err
				}
			}
			count++
			idSID, err := sid(id)
			if err != nil {
				return err
			}
			if err = c[idCol].u32(idSID); err != nil {
				return err
			}
			if role == ports.Edges {
				source, left, err := takeString(rest)
				if err != nil {
					return err
				}
				target, left, err := takeString(left)
				if err != nil {
					return err
				}
				label, _, err := takeString(left)
				if err != nil {
					return err
				}
				src, ok := d.nodes.find(source)
				if !ok {
					return errors.New("missing accepted source")
				}
				dst, ok := d.nodes.find(target)
				if !ok {
					return errors.New("missing accepted target")
				}
				l, err := sid(label)
				if err != nil {
					return err
				}
				for _, p := range []struct {
					col   int
					value uint32
				}{{cSources, src}, {cTargets, dst}, {cEdgeLabels, l}} {
					if err = c[p.col].u32(p.value); err != nil {
						return err
					}
				}
				if err = labelPosting(l); err != nil {
					return err
				}
				if err = runs[2].row(big32(big32(big32(big32(nil, src), dst), l), uint32(count-1)), nil); err != nil {
					return err
				}
				if err = runs[3].row(big32(big32(big32(big32(nil, dst), src), l), uint32(count-1)), nil); err != nil {
					return err
				}
			}
		case 1:
			if role == ports.Edges {
				continue
			}
			label, _, err := takeString(rest)
			if err != nil {
				return err
			}
			l, err := sid(label)
			if err != nil {
				return err
			}
			if err = c[cNodeLabels].u32(l); err != nil {
				return err
			}
			if err = labelPosting(l); err != nil {
				return err
			}
		case 2:
			key, value, err := takeString(rest)
			if err != nil {
				return err
			}
			k, err := sid(key)
			if err != nil {
				return err
			}
			if len(value) < 1 {
				return io.ErrUnexpectedEOF
			}
			kind := value[0]
			var bits uint64
			if kind >= byte(graph.StringKind) {
				text, _, err := takeString(value[1:])
				if err != nil {
					return err
				}
				v, err := sid(text)
				if err != nil {
					return err
				}
				bits = uint64(v)
			} else {
				if len(value) != 9 {
					return io.ErrUnexpectedEOF
				}
				bits = spillBE.Uint64(value[1:])
			}
			var p [16]byte
			spillLE.PutUint32(p[:], k)
			p[4] = kind
			spillLE.PutUint64(p[8:], bits)
			if err = c[propCol].write(p[:]); err != nil {
				return err
			}
			if indexed[k] {
				if err = runs[1].row(big32(big64(append(big32([]byte{byte(role)}, k), kind), bits), uint32(count-1)), nil); err != nil {
					return err
				}
			}
		}
	}
	if count > 0 {
		return finish()
	}
	return nil
}
func (d *diskBuilder) postingColumns(kind int, input *spillFile, c [columnCount]*spillFile) error {
	index, post, width := cLabelIndex, cLabelPostings, 9
	if kind == 1 {
		index, post, width = cPropertyIndex, cPropertyPostings, 18
	}
	r := readSpill(input, 0, input.size)
	nextErr := r.next()
	var total uint64
	for nextErr == nil {
		if err := d.b.ctx.Err(); err != nil {
			return err
		}
		if len(r.row.key) != width {
			return errors.New("invalid scratch posting")
		}
		prefix := bytes.Clone(r.row.key[:width-4])
		start := total
		for nextErr == nil && len(r.row.key) == width && bytes.Equal(prefix, r.row.key[:width-4]) {
			if err := c[post].u32(spillBE.Uint32(r.row.key[width-4:])); err != nil {
				return err
			}
			total++
			nextErr = r.next()
			if err := d.b.ctx.Err(); err != nil {
				return err
			}
		}
		var entry [32]byte
		if kind == 0 {
			spillLE.PutUint32(entry[:], uint32(prefix[0]))
			spillLE.PutUint32(entry[4:], spillBE.Uint32(prefix[1:]))
			spillLE.PutUint64(entry[8:], start)
			spillLE.PutUint64(entry[16:], total-start)
			if err := c[index].write(entry[:24]); err != nil {
				return err
			}
		} else {
			entry[0], entry[1] = prefix[0], prefix[5]
			spillLE.PutUint32(entry[4:], spillBE.Uint32(prefix[1:]))
			spillLE.PutUint64(entry[8:], spillBE.Uint64(prefix[6:]))
			spillLE.PutUint64(entry[16:], start)
			spillLE.PutUint64(entry[24:], total-start)
			if err := c[index].write(entry[:]); err != nil {
				return err
			}
		}
	}
	if !errors.Is(nextErr, io.EOF) {
		return nextErr
	}
	return nil
}
func (d *diskBuilder) csrColumns(kind int, input *spillFile, c [columnCount]*spillFile) error {
	off, neighbor, edge := cForwardOffsets, cForwardNeighbors, cForwardEdges
	if kind == 3 {
		off, neighbor, edge = cReverseOffsets, cReverseNeighbors, cReverseEdges
	}
	if err := c[off].u64(0); err != nil {
		return err
	}
	r := readSpill(input, 0, input.size)
	var owner, total uint64
	for {
		if err := d.b.ctx.Err(); err != nil {
			return err
		}
		err := r.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if len(r.row.key) != 16 {
			return errors.New("invalid scratch adjacency")
		}
		src := uint64(spillBE.Uint32(r.row.key))
		for owner < src {
			if err = c[off].u64(total); err != nil {
				return err
			}
			owner++
		}
		if err = c[neighbor].u32(spillBE.Uint32(r.row.key[4:])); err != nil {
			return err
		}
		if err = c[edge].u32(spillBE.Uint32(r.row.key[12:])); err != nil {
			return err
		}
		total++
	}
	for owner < d.counts[0] {
		if err := d.b.ctx.Err(); err != nil {
			return err
		}
		if err := c[off].u64(total); err != nil {
			return err
		}
		owner++
	}
	return nil
}
