// Package stderr writes one escaped JSON diagnostic per line.
package stderr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"gophergraph/ingest/ports"
	"io"
	"unicode"
)

type Sink struct{ output io.Writer }

func New(w io.Writer) *Sink { return &Sink{output: w} }
func (s *Sink) Emit(ctx context.Context, event ports.Diagnostic) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	data, e := json.Marshal(event)
	if e != nil {
		return e
	}
	var out bytes.Buffer
	for _, r := range string(data) {
		if unicode.IsControl(r) {
			fmt.Fprintf(&out, "\\u%04x", r)
		} else {
			out.WriteRune(r)
		}
	}
	out.WriteByte('\n')
	n, e := s.output.Write(out.Bytes())
	if e == nil && n != out.Len() {
		return io.ErrShortWrite
	}
	return e
}
