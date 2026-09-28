package stderr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gophergraph/ingest/ports"
	"strings"
	"testing"
)

type fail struct{}

func (fail) Write([]byte) (int, error) { return 0, errors.New("full") }
func TestEscapedDiagnostics(t *testing.T) {
	var b bytes.Buffer
	d := ports.Diagnostic{Code: "BAD", EntityID: "x\n\"\x00", EntityIDKnown: true, Location: ports.Location{Source: "a\r\nb"}, Message: "invalid"}
	if e := New(&b).Emit(context.Background(), d); e != nil {
		t.Fatal(e)
	}
	if strings.Count(b.String(), "\n") != 1 || !json.Valid(bytes.TrimSpace(b.Bytes())) {
		t.Fatal(b.String())
	}
	if e := New(fail{}).Emit(context.Background(), d); e == nil {
		t.Fatal("write error lost")
	}
}
