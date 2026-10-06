package ggpb

import (
	"context"
	"encoding/binary"
	"fmt"
	"gophergraph/ggpb/pb"
	"gophergraph/graph"
	"gophergraph/query"
	"hash/crc32"
	"io"

	"google.golang.org/protobuf/proto"
)

var magic = [8]byte{'G', 'G', 'P', 'B', '\r', '\n', 0x1a, '\n'}

// Write writes magic then [uint32 LE length][protobuf Batch][CRC32 IEEE LE].
// It flushes no caller buffers and closes neither graph nor output. Any error
// leaves an incomplete prefix that must be discarded. ResultEnd is mandatory.
func Write(ctx context.Context, out io.Writer, g *graph.Graph, s *query.Subgraph, q Query) error {
	return WriteOptions(ctx, out, g, s, q, Options{})
}
func WriteOptions(ctx context.Context, out io.Writer, g *graph.Graph, s *query.Subgraph, q Query, o Options) error {
	first := true
	return EmitEncoded(ctx, g, s, q, o, func(buf []byte) error {
		if first {
			if e := writeAll(out, magic[:]); e != nil {
				return e
			}
			first = false
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		var e error
		var word [4]byte
		binary.LittleEndian.PutUint32(word[:], uint32(len(buf)))
		if e = writeAll(out, word[:]); e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if e = writeAll(out, buf); e != nil {
			return e
		}
		binary.LittleEndian.PutUint32(word[:], crc32.ChecksumIEEE(buf))
		if e = ctx.Err(); e != nil {
			return e
		}
		return writeAll(out, word[:])
	})
}
func writeAll(w io.Writer, b []byte) error {
	n, e := w.Write(b)
	if e != nil {
		return e
	}
	if n != len(b) {
		return io.ErrShortWrite
	}
	return nil
}

// Reader reads one bounded logical Batch at a time and validates ordering,
// value types, dictionary references and end counts. It does not retain entities
// or validate endpoint membership/ID uniqueness across the whole result.
// Next returns io.EOF only after a valid ResultEnd and physical EOF.
// Do not read the same Reader concurrently. Context cannot interrupt blocked I/O.
type Reader struct {
	in                           io.Reader
	started, ended               bool
	hasHeader                    bool
	expectedNodes, expectedEdges uint64
	buf                          []byte
	state                        validator
	err                          error
}

func NewReader(in io.Reader) *Reader { return &Reader{in: in} }
func (r *Reader) Next(ctx context.Context) (*pb.Batch, error) {
	if r.err != nil {
		return nil, r.err
	}
	b, e := r.next(ctx)
	if e != nil {
		r.err = e
	}
	return b, e
}
func (r *Reader) next(ctx context.Context) (*pb.Batch, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if !r.started {
		var m [8]byte
		if _, e := io.ReadFull(r.in, m[:]); e != nil {
			return nil, fmt.Errorf("%w: missing magic: %w", ErrInvalid, e)
		}
		if m != magic {
			return nil, fmt.Errorf("%w: magic", ErrInvalid)
		}
		r.started = true
	}
	var word [4]byte
	n, e := io.ReadFull(r.in, word[:])
	if r.ended {
		if n == 0 && e == io.EOF {
			return nil, io.EOF
		}
		if e != nil && e != io.EOF {
			return nil, e
		}
		return nil, fmt.Errorf("%w: trailing bytes", ErrInvalid)
	}
	if e != nil {
		return nil, fmt.Errorf("%w: missing or truncated frame: %w", ErrInvalid, e)
	}
	size := binary.LittleEndian.Uint32(word[:])
	if size == 0 || size > MaxFrame {
		return nil, ErrLimit
	}
	if cap(r.buf) < int(size) {
		r.buf = make([]byte, size)
	} else {
		r.buf = r.buf[:size]
	}
	if _, e = io.ReadFull(r.in, r.buf); e != nil {
		return nil, fmt.Errorf("%w: truncated payload: %w", ErrInvalid, e)
	}
	if _, e = io.ReadFull(r.in, word[:]); e != nil {
		return nil, fmt.Errorf("%w: truncated checksum: %w", ErrInvalid, e)
	}
	if binary.LittleEndian.Uint32(word[:]) != crc32.ChecksumIEEE(r.buf) {
		return nil, fmt.Errorf("%w: checksum", ErrInvalid)
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	b := new(pb.Batch)
	if e = (proto.UnmarshalOptions{RecursionLimit: 32}).Unmarshal(r.buf, b); e != nil {
		return nil, fmt.Errorf("%w: protobuf: %w", ErrInvalid, e)
	}
	if !r.hasHeader {
		h := b.GetHeader()
		if h == nil {
			return nil, fmt.Errorf("%w: header required", ErrInvalid)
		}
		if h.Version != Version {
			return nil, ErrVersion
		}
		if e = r.state.header(h); e != nil {
			return nil, e
		}
		r.hasHeader = true
		r.expectedNodes, r.expectedEdges = h.Nodes, h.Edges
	} else {
		switch p := b.Payload.(type) {
		case *pb.Batch_Records:
			e = r.state.records(ctx, p.Records)
		case *pb.Batch_End:
			e = r.state.end(p.End, r.expectedNodes, r.expectedEdges)
			r.ended = e == nil
		default:
			e = fmt.Errorf("%w: unexpected batch", ErrInvalid)
		}
		if e != nil {
			return nil, e
		}
		if r.state.nodesCount > r.expectedNodes || r.state.edgesCount > r.expectedEdges {
			return nil, invalid("counts exceeded")
		}
	}
	return b, nil
}
