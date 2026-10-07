package ggpb

import (
	"context"
	"fmt"
	"gophergraph/ggpb/pb"

	"google.golang.org/protobuf/proto"
)

// BatchDecoder validates a sequence of unframed GGPB payloads incrementally.
// It retains only ordering/count state, never a result-wide entity table.
// Each decoded message is owned by the caller. A Decoder is not concurrent.
// Finish must be called only after the transport reports successful completion.
type BatchDecoder struct {
	state         validator
	header, ended bool
	nodes, edges  uint64
	err           error
}

func (d *BatchDecoder) Decode(ctx context.Context, payload []byte) (*pb.Batch, error) {
	if d.err != nil {
		return nil, d.err
	}
	b, err := d.decode(ctx, payload)
	if err != nil {
		d.err = err
	}
	return b, err
}
func (d *BatchDecoder) decode(ctx context.Context, payload []byte) (*pb.Batch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if d.ended {
		return nil, invalid("batch after end")
	}
	if len(payload) == 0 || len(payload) > MaxFrame {
		return nil, ErrLimit
	}
	b := new(pb.Batch)
	if err := (proto.UnmarshalOptions{RecursionLimit: 32}).Unmarshal(payload, b); err != nil {
		return nil, fmt.Errorf("%w: protobuf: %w", ErrInvalid, err)
	}
	if !d.header {
		h := b.GetHeader()
		if h == nil {
			return nil, invalid("header required")
		}
		if h.Version != Version {
			return nil, ErrVersion
		}
		if err := d.state.header(h); err != nil {
			return nil, err
		}
		d.header = true
		d.nodes, d.edges = h.Nodes, h.Edges
	} else {
		var err error
		switch p := b.Payload.(type) {
		case *pb.Batch_Records:
			err = d.state.records(ctx, p.Records)
		case *pb.Batch_End:
			err = d.state.end(p.End, d.nodes, d.edges)
			d.ended = err == nil
		default:
			err = invalid("unexpected batch")
		}
		if err != nil {
			return nil, err
		}
		if d.state.nodesCount > d.nodes || d.state.edgesCount > d.edges {
			return nil, invalid("counts exceeded")
		}
	}
	return b, nil
}
func (d *BatchDecoder) Finish() error {
	if d.err != nil {
		return d.err
	}
	if !d.ended {
		return invalid("missing end")
	}
	return nil
}
