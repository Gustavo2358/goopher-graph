package neptune

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"gophergraph/ingest/ports"
	"io"
	"strings"
)

var errLexical = errors.New("ambiguous CSV boundary")
var errLimit = errors.New("record limit exceeded")

type cell struct {
	text            string
	quoted, present bool
}
type framer struct {
	r            *bufio.Reader
	limits       ports.Limits
	line, record uint64
	started      bool
}

// frame retains at most MaxRecordBytes; oversized records are drained using the
// same quote state, never by searching for the next physical newline.
func (f *framer) frame(ctx context.Context) ([]byte, uint64, error) {
	if !f.started {
		f.started = true
		if b, _ := f.r.Peek(3); bytes.Equal(b, []byte{239, 187, 191}) {
			_, _ = f.r.Discard(3)
		}
	}
	for {
		start := f.line
		var raw []byte
		state := byte(0)
		columns := uint32(1)
		over := false
		var consumed uint64
		appendByte := func(b byte) {
			consumed++
			if consumed > f.limits.MaxRecordBytes {
				over = true
			}
			if !over {
				raw = append(raw, b)
			}
		}
		for {
			if consumed%4096 == 0 {
				if e := ctx.Err(); e != nil {
					return nil, start, e
				}
			}
			b, e := f.r.ReadByte()
			if e != nil {
				if !errors.Is(e, io.EOF) {
					return nil, start, e
				}
				if state == 2 {
					return nil, start, errLexical
				}
				if consumed == 0 {
					return nil, start, io.EOF
				}
				f.record++
				if over {
					return nil, start, errLimit
				}
				return raw, start, nil
			}
			if b == '\n' {
				f.line++
			}
			if state != 2 && (b == '\n' || b == '\r') {
				if b == '\r' {
					next, e := f.r.ReadByte()
					if e != nil {
						if errors.Is(e, io.EOF) {
							return nil, start, errLexical
						}
						return nil, start, e
					}
					if next != '\n' {
						return nil, start, errLexical
					}
					f.line++
				}
				if consumed == 0 {
					break
				}
				f.record++
				if over {
					return nil, start, errLimit
				}
				return raw, start, nil
			}
			appendByte(b)
			switch state {
			case 0:
				if b == '"' {
					state = 2
				} else if b == ',' {
					columns++
				} else if b != ' ' && b != '\t' {
					state = 1
				}
			case 1:
				if b == '"' {
					return nil, start, errLexical
				}
				if b == ',' {
					state = 0
					columns++
				}
			case 2:
				if b == '"' {
					state = 3
				}
			case 3:
				if b == '"' {
					state = 2
				} else if b == ',' {
					state = 0
					columns++
				} else {
					return nil, start, errLexical
				}
			}
			if columns > f.limits.MaxColumns {
				over = true
			}
		}
	}
}
func cells(raw []byte) ([]cell, error) {
	var out []cell
	var normalized bytes.Buffer
	start := 0
	quoted := false
	appendField := func(end int) {
		v := strings.Trim(string(raw[start:end]), " \t")
		c := cell{present: len(v) > 0}
		if len(v) >= 2 && v[0] == '"' {
			c.quoted = true
			c.present = true
			c.text = strings.ReplaceAll(v[1:len(v)-1], "\"\"", "\"")
		} else {
			c.text = v
		}
		if len(out) > 0 {
			normalized.WriteByte(',')
		}
		normalized.WriteString(v)
		out = append(out, c)
	}
	for i, b := range raw {
		if b == '"' {
			quoted = !quoted
		}
		if b == ',' && !quoted {
			appendField(i)
			start = i + 1
		}
	}
	appendField(len(raw))
	// The stdlib validates CSV quoting. Cell contents above retain the CRLF bytes
	// and quoted emptiness which encoding/csv intentionally normalizes.
	normalized.WriteString(",\"sentinel\"")
	r := csv.NewReader(&normalized)
	r.FieldsPerRecord = -1
	if _, e := r.Read(); e != nil {
		return nil, e
	}
	return out, nil
}
