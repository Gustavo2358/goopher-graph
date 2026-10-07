package wasmquery

import (
	"context"
	"errors"
	"testing"
)

func TestExecuteReportRetainsFailureMetrics(t *testing.T) {
	r := newRuntime(t, Limits{Calls: 1})
	m := compileGuest(t, r, "../examples/wasm/shared_targets")
	g := loadGraph(t, "../fixtures/01_topology", false)
	result, report, err := r.ExecuteReport(context.Background(), m, g, []string{"A", "B"})
	if result != nil || !errors.Is(err, ErrLimit) || report.HostCalls != 2 || report.PeakHandles == 0 {
		t.Fatal(result, report, err)
	}
	if r.Stats().ActiveHandles != 0 || r.Stats().ActiveExecutions != 0 {
		t.Fatal(r.Stats())
	}
}
