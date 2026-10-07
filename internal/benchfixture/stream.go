package benchfixture

import (
	"context"
	"fmt"
	"gophergraph/graph"
	"gophergraph/internal/graphdata"
	"slices"
	"strings"
)

// StreamingGraph interns one property value; result payload scales independently
// of snapshot size. Useful for backpressure qualification, not a business model.
func StreamingGraph(ctx context.Context, n, scalar int) (*graph.Graph, error) {
	if n < 1 || n > 1000000 || scalar < 1 || scalar > 1<<20 {
		return nil, fmt.Errorf("invalid streaming fixture size")
	}
	d := graphdata.Empty()
	value := strings.Repeat("x", scalar)
	d.Strings = []string{"", "L", "payload", value}
	for i := range n {
		d.Strings = append(d.Strings, fmt.Sprintf("n%09d", i), fmt.Sprintf("e%09d", i))
	}
	slices.Sort(d.Strings)
	sid := func(v string) uint32 { i, _ := slices.BinarySearch(d.Strings, v); return uint32(i) }
	for i := range n {
		d.NodeIDs.Heap = append(d.NodeIDs.Heap, sid(fmt.Sprintf("n%09d", i)))
		d.NodeLabelOffsets.Heap = append(d.NodeLabelOffsets.Heap, uint64(i+1))
		d.NodeLabels.Heap = append(d.NodeLabels.Heap, sid("L"))
		d.NodePropOffsets.Heap = append(d.NodePropOffsets.Heap, uint64(i+1))
		d.NodeProps.Heap = append(d.NodeProps.Heap, graphdata.Property{Key: sid("payload"), Kind: 8, Payload: uint64(sid(value))})
		d.EdgeIDs.Heap = append(d.EdgeIDs.Heap, sid(fmt.Sprintf("e%09d", i)))
		d.Sources.Heap = append(d.Sources.Heap, uint32(i))
		d.Targets.Heap = append(d.Targets.Heap, uint32((i+1)%n))
		d.EdgeLabels.Heap = append(d.EdgeLabels.Heap, sid("L"))
		d.EdgePropOffsets.Heap = append(d.EdgePropOffsets.Heap, uint64(i+1))
		d.EdgeProps.Heap = append(d.EdgeProps.Heap, graphdata.Property{Key: sid("payload"), Kind: 8, Payload: uint64(sid(value))})
	}
	graphdata.BuildCSR(d)
	if err := graphdata.BuildIndexes(ctx, d); err != nil {
		return nil, err
	}
	heap, err := graph.New(d, nil)
	if err != nil {
		return nil, err
	}
	return heap, nil
}
