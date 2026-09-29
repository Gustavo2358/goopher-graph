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
func TestUnicodeControlEscaping(t *testing.T) {
	var b bytes.Buffer
	event := ports.Diagnostic{EntityID: "x\u0085\u009b", EntityIDKnown: true}
	if e := New(&b).Emit(context.Background(), event); e != nil {
		t.Fatal(e)
	}
	if strings.ContainsRune(b.String(), '\u0085') || strings.ContainsRune(b.String(), '\u009b') {
		t.Fatal("raw control", b.String())
	}
	var got ports.Diagnostic
	if e := json.Unmarshal(b.Bytes(), &got); e != nil || got.EntityID != event.EntityID {
		t.Fatal(got, e)
	}
}
