package observability

import (
	"context"
	"errors"
	"gophergraph/remote"
	"gophergraph/remote/installedwasm"
	"gophergraph/remote/pb"
	"gophergraph/snapshot"
	"gophergraph/snapshot/adapters/mmap"
	"gophergraph/wasmquery"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInspectionFailureIsTelemetryOnly(t *testing.T) {
	ctx := context.Background()
	g, err := snapshot.Open(ctx, mmap.New("../../fixtures/snapshot_reference/empty.snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	r, err := wasmquery.New(ctx, wasmquery.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	reg, err := installedwasm.Default(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	service, _ := remote.NewService(g, r, reg, &pb.ServerInfoResponse{ResidencyMode: "locked", Locked: true, LockedBytes: 4096})
	s, err := remote.NewServer(service, remote.ServerOptions{QueryCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown(ctx)
	h := Handler(s, Options{Observe: func() (SnapshotObservation, error) { return SnapshotObservation{}, errors.New("restricted proc") }})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "gophergraph_snapshot_inspection_available 0") || !strings.Contains(w.Body.String(), "gophergraph_snapshot_locked 1") {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/livez", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
}
