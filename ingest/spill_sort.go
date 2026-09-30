package ingest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"gophergraph/ingest/ports"
	"io"
	"math"
	"sort"
)

var spillLE = binary.LittleEndian
var spillBE = binary.BigEndian

type spillFile struct {
	file   ports.ScratchFile
	w      *bufio.Writer
	size   int64
	maxRow uint64
}

func newSpill(w ports.Workspace) (*spillFile, error) {
	f, err := w.Create()
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, errors.New("scratch returned nil file")
	}
	return &spillFile{file: f, w: bufio.NewWriterSize(f, 32<<10)}, nil
}
func (f *spillFile) write(p []byte) error {
	if uint64(len(p)) > uint64(math.MaxInt64-f.size) {
		return errors.New("scratch size overflow")
	}
	n, err := f.w.Write(p)
	f.size += int64(n)
	if err == nil && n != len(p) {
		return io.ErrShortWrite
	}
	return err
}
func (f *spillFile) flush() error { return f.w.Flush() }
func (f *spillFile) row(key, value []byte) error {
	var h [16]byte
	spillLE.PutUint64(h[:], uint64(len(key)))
	spillLE.PutUint64(h[8:], uint64(len(value)))
	f.maxRow = max(f.maxRow, uint64(len(key))+uint64(len(value)))
	if err := f.write(h[:]); err != nil {
		return err
	}
	if err := f.write(key); err != nil {
		return err
	}
	return f.write(value)
}
func (f *spillFile) u32(x uint32) error {
	var b [4]byte
	spillLE.PutUint32(b[:], x)
	return f.write(b[:])
}
func (f *spillFile) u64(x uint64) error {
	var b [8]byte
	spillLE.PutUint64(b[:], x)
	return f.write(b[:])
}

type spillRow struct{ key, value []byte }
type spillReader struct {
	limit      int64
	r          *bufio.Reader
	row        spillRow
	buf        []byte
	start, end int64
	maxRow     uint64
}

func readSpill(f *spillFile, start, end int64) *spillReader {
	r := &spillReader{r: bufio.NewReaderSize(bytes.NewReader(nil), 32<<10)}
	r.reset(f, start, end)
	return r
}
func (r *spillReader) reset(f *spillFile, start, end int64) {
	r.r.Reset(io.NewSectionReader(f.file, start, end-start))
	r.start, r.end, r.maxRow, r.limit = start, start, f.maxRow, end
	r.row = spillRow{}
}
func (r *spillReader) next() error {
	r.start = r.end
	if r.end == r.limit {
		return io.EOF
	}
	var h [16]byte
	if _, err := io.ReadFull(r.r, h[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	k, v := spillLE.Uint64(h[:]), spillLE.Uint64(h[8:])
	if k > r.maxRow || v > r.maxRow-k || k+v > uint64(math.MaxInt) || r.limit-r.end < 16 || k+v > uint64(r.limit-r.end-16) {
		return errors.New("invalid scratch row length")
	}
	n := int(k + v)
	if cap(r.buf) < n {
		r.buf = make([]byte, n)
	}
	r.buf = r.buf[:n]
	if _, err := io.ReadFull(r.r, r.buf); err != nil {
		if errors.Is(err, io.EOF) {
			return io.ErrUnexpectedEOF
		}
		return err
	}
	r.row = spillRow{r.buf[:k], r.buf[k:]}
	r.end += int64(16 + n)
	return nil
}

// externalSort uses fixed arena/descriptor buffers and a binary merge stack.
// At most 64 runs stay live; fan-in is two, independent of corpus size.
// A single oversized row is written as its own run (bounded by decoder limits).
func externalSort(ctx context.Context, ws ports.Workspace, input *spillFile, budget uint64) (*spillFile, error) {
	if err := input.flush(); err != nil {
		return nil, err
	}
	arena := make([]byte, int(budget/2))
	rows := make([]spillRow, 0, int(budget/2/48))
	used := 0
	var levels [64]*spillFile
	push := func(run *spillFile) error {
		for i := range levels {
			if levels[i] == nil {
				levels[i] = run
				return nil
			}
			var err error
			run, err = mergeRuns(ctx, ws, levels[i], run)
			if err != nil {
				return err
			}
			levels[i] = nil
		}
		return errors.New("too many scratch runs")
	}
	flush := func() error {
		if len(rows) == 0 {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		sort.Slice(rows, func(i, j int) bool { return bytes.Compare(rows[i].key, rows[j].key) < 0 })
		run, err := newSpill(ws)
		if err != nil {
			return err
		}
		for i, r := range rows {
			if i%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if err := run.row(r.key, r.value); err != nil {
				return err
			}
		}
		if err := run.flush(); err != nil {
			return err
		}
		clear(rows)
		rows = rows[:0]
		used = 0
		return push(run)
	}
	r := readSpill(input, 0, input.size)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		err := r.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		size := len(r.row.key) + len(r.row.value)
		if size > len(arena)-used || len(rows) == cap(rows) {
			if err := flush(); err != nil {
				return nil, err
			}
		}
		if size > len(arena) {
			run, err := newSpill(ws)
			if err != nil {
				return nil, err
			}
			if err = run.row(r.row.key, r.row.value); err != nil {
				return nil, err
			}
			if err = run.flush(); err != nil {
				return nil, err
			}
			if err = push(run); err != nil {
				return nil, err
			}
			continue
		}
		k := copy(arena[used:], r.row.key)
		copy(arena[used+k:], r.row.value)
		rows = append(rows, spillRow{arena[used : used+k], arena[used+k : used+size]})
		used += size
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if err := ws.Remove(input.file); err != nil {
		return nil, err
	}
	var out *spillFile
	for _, run := range levels {
		if run == nil {
			continue
		}
		if out == nil {
			out = run
			continue
		}
		var err error
		out, err = mergeRuns(ctx, ws, out, run)
		if err != nil {
			return nil, err
		}
	}
	if out == nil {
		return newSpill(ws)
	}
	return out, nil
}
func mergeRuns(ctx context.Context, ws ports.Workspace, a, b *spillFile) (*spillFile, error) {
	out, err := newSpill(ws)
	if err != nil {
		return nil, err
	}
	x, y := readSpill(a, 0, a.size), readSpill(b, 0, b.size)
	xe, ye := x.next(), y.next()
	for xe == nil || ye == nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if xe != nil && !errors.Is(xe, io.EOF) {
			return nil, xe
		}
		if ye != nil && !errors.Is(ye, io.EOF) {
			return nil, ye
		}
		if ye != nil || (xe == nil && bytes.Compare(x.row.key, y.row.key) <= 0) {
			if err = out.row(x.row.key, x.row.value); err != nil {
				return nil, err
			}
			xe = x.next()
		} else {
			if err = out.row(y.row.key, y.row.value); err != nil {
				return nil, err
			}
			ye = y.next()
		}
	}
	if !errors.Is(xe, io.EOF) {
		return nil, xe
	}
	if !errors.Is(ye, io.EOF) {
		return nil, ye
	}
	if err = out.flush(); err != nil {
		return nil, err
	}
	if err = errors.Join(ws.Remove(a.file), ws.Remove(b.file)); err != nil {
		return nil, err
	}
	return out, nil
}

// Zero-escaped terminated strings retain UTF-8 byte order, including empty IDs
// and embedded NULs; unlike delimiter concatenation they cannot collide.
func sortString(b []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		b = append(b, s[i])
		if s[i] == 0 {
			b = append(b, 255)
		}
	}
	return append(b, 0, 0)
}
func takeString(b []byte) (string, []byte, error) {
	for i := 0; i < len(b); i++ {
		if b[i] != 0 {
			continue
		}
		if i+1 >= len(b) {
			break
		}
		if b[i+1] == 0 {
			if bytes.IndexByte(b[:i], 0) < 0 {
				return string(b[:i]), b[i+2:], nil
			}
			return string(bytes.ReplaceAll(b[:i], []byte{0, 255}, []byte{0})), b[i+2:], nil
		}
		if b[i+1] != 255 {
			break
		}
		i++
	}
	return "", nil, errors.New("invalid scratch string")
}
func big32(b []byte, x uint32) []byte { return spillBE.AppendUint32(b, x) }
func big64(b []byte, x uint64) []byte { return spillBE.AppendUint64(b, x) }
