package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func TestHelp(t *testing.T) {
	t.Chdir(t.TempDir())
	var out, diag bytes.Buffer
	if code := run([]string{"--help"}, &out, &diag); code != 0 || !strings.Contains(out.String(), "Usage:") || diag.Len() != 0 {
		t.Fatalf("code=%d out=%q diag=%q", code, out.String(), diag.String())
	}
	if code := run([]string{"--help"}, brokenWriter{}, &diag); code != 1 {
		t.Fatalf("write failure code=%d", code)
	}
	if code := run([]string{"unknown"}, &out, &diag); code != 2 {
		t.Fatalf("usage code=%d", code)
	}
}
