package ingest

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type truncatedScratch struct{ data []byte }

func (f *truncatedScratch) Write(b []byte) (int, error) {
	f.data = append(f.data, b...)
	return len(b), nil
}
func (f *truncatedScratch) ReadAt(b []byte, off int64) (int, error) {
	return bytes.NewReader(f.data).ReadAt(b, off)
}
func (f *truncatedScratch) Close() error { return nil }

func TestSpillTruncationNeverLooksLikeNormalEOF(t *testing.T) {
	var full []byte
	for i := 0; i < 2; i++ {
		full = spillLE.AppendUint64(full, 3)
		full = spillLE.AppendUint64(full, 2)
		full = append(full, []byte("abcde")...)
	}
	for _, cut := range []int{0, 8, 16, 17, 21, 37, 38, 41} {
		f := &spillFile{file: &truncatedScratch{data: full[:cut]}, size: int64(len(full)), maxRow: 5}
		r := readSpill(f, 0, f.size)
		var err error
		for err == nil {
			err = r.next()
		}
		if errors.Is(err, io.EOF) {
			t.Fatalf("cut %d silently accepted a prefix", cut)
		}
	}
}
