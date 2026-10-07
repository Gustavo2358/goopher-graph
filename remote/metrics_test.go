package remote

import (
	"context"
	"gophergraph/remote/pb"
	"strings"
	"testing"
	"time"
)

func TestMetricsBoundedNamesAndPhases(t *testing.T) {
	s, g := serviceFixture(t)
	server, client, _ := startServer(t, s, ServerOptions{QueryCapacity: 1})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	first, _ := g.NodeExternalID(0)
	stream, err := client.Territory(ctx, &pb.TerritoryRequest{Node: &first})
	if err != nil {
		t.Fatal(err)
	}
	receive(t, stream)
	for i := range 5 {
		stream, err = client.RunWasm(ctx, &pb.RunWasmRequest{QueryName: strings.Repeat("attacker", i+1)})
		if err == nil {
			_, err = stream.Recv()
		}
		if err == nil {
			t.Fatal("missing query succeeded")
		}
	}
	var native, unknown QueryStatistics
	for _, q := range server.QueryStatistics() {
		if q.Name == "territory" {
			native = q
		}
		if q.Name == "wasm:<unknown>" {
			unknown = q
		}
		if strings.Contains(q.Name, "attacker") {
			t.Fatal("unbounded label")
		}
	}
	if native.Completed != 1 || native.Batches < 2 || native.Bytes == 0 || native.Execution <= 0 || native.Encode <= 0 || native.Send <= 0 || native.Active != 0 {
		t.Fatal(native)
	}
	if unknown.Requests != 5 || unknown.Failed != 5 || unknown.Active != 0 {
		t.Fatal(unknown)
	}
	if native.Duration < native.Execution+native.Encode+native.Send {
		t.Fatal("phase overlap", native)
	}
}
