// Package stderr writes one escaped JSON diagnostic per line.
package stderr

import (
	"context"
	"encoding/json"
	"gophergraph/ingest/ports"
	"io"
)

type Sink struct{ encoder *json.Encoder }

func New(w io.Writer) *Sink { return &Sink{json.NewEncoder(w)} }
func (s *Sink) Emit(ctx context.Context, event ports.Diagnostic) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	return s.encoder.Encode(event)
}
