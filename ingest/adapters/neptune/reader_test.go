package neptune

import (
	"context"
	"errors"
	"gophergraph/ingest/ports"
	"io"
	"strings"
	"testing"
)

type fragments struct {
	data []byte
	size int
	end  error
}

func (r *fragments) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.end
	}
	n := min(len(p), r.size, len(r.data))
	copy(p, r.data[:n])
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, r.end
	}
	return n, nil
}
func TestPresenceAndFraming(t *testing.T) {
	input := "\ufeff~id,a:String,b:String\r\n\r\n\"\",,\"\"\r\nA,\"line1\r\nline2\",\"say \"\"hi\"\"\"\r\nB,  trim  ,\"  keep  \""
	for _, size := range []int{1, 2, 7, 4096} {
		r, e := (Decoder{}).New(context.Background(), ports.Nodes, ports.Entry{Key: "x"}, &fragments{[]byte(input), size, io.EOF}, ports.Limits{})
		if e != nil {
			t.Fatal(e)
		}
		a, e := r.Next(context.Background())
		if e != nil || a.Record.ID != "" || len(a.Record.Properties) != 1 {
			t.Fatal(a, e)
		}
		s, _ := a.Record.Properties[0].Value.Text()
		if s != "" {
			t.Fatal(s)
		}
		b, e := r.Next(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		s, _ = b.Record.Properties[0].Value.Text()
		if s != "line1\r\nline2" {
			t.Fatalf("%q", s)
		}
		b, e = r.Next(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		s, _ = b.Record.Properties[0].Value.Text()
		if s != "trim" {
			t.Fatal(s)
		}
		s, _ = b.Record.Properties[1].Value.Text()
		if s != "  keep  " {
			t.Fatal(s)
		}
		if _, e = r.Next(context.Background()); !errors.Is(e, io.EOF) {
			t.Fatal(e)
		}
	}
}
func TestRejections(t *testing.T) {
	for _, header := range []string{"~id,~from,~to", "~id,~id", "~id,x:Nope", "~id,x:Int(single)[]", "~id,x:Int,x:Long"} {
		_, e := (Decoder{}).New(context.Background(), ports.Nodes, ports.Entry{}, strings.NewReader(header+"\n"), ports.Limits{})
		var se *ports.SourceError
		if !errors.As(e, &se) || se.Kind != ports.InvalidHeader {
			t.Fatal(header, e)
		}
	}
	input := "~id,x:Int\nA,1\nbad,INF\nbad2,\"\"\n\xff,2\nB,2\n\"unclosed\nC,3\n"
	r, e := (Decoder{}).New(context.Background(), ports.Nodes, ports.Entry{}, strings.NewReader(input), ports.Limits{})
	if e != nil {
		t.Fatal(e)
	}
	valid, rejected := 0, 0
	for {
		v, e := r.Next(context.Background())
		if e != nil {
			var se *ports.SourceError
			if !errors.As(e, &se) || se.Kind != ports.UnrecoverableCSV {
				t.Fatal(e)
			}
			break
		}
		if v.Record != nil {
			valid++
		} else {
			rejected++
		}
	}
	if valid != 2 || rejected != 3 {
		t.Fatal(valid, rejected)
	}
}
func TestLimitsAndIO(t *testing.T) {
	for _, limit := range []uint64{1024, 1 << 20} {
		r, e := (Decoder{}).New(context.Background(), ports.Nodes, ports.Entry{}, strings.NewReader("~id,x:String\nA,"+strings.Repeat("x", 70000)+"\nB,ok\n"), ports.Limits{MaxRecordBytes: limit})
		if e != nil {
			t.Fatal(e)
		}
		a, e := r.Next(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		if (a.Rejection != nil) != (limit == 1024) {
			t.Fatal("limit")
		}
		b, e := r.Next(context.Background())
		if e != nil || b.Record.ID != "B" {
			t.Fatal(b, e)
		}
	}
	boom := errors.New("disk failure")
	r, e := (Decoder{}).New(context.Background(), ports.Nodes, ports.Entry{}, &fragments{[]byte("~id\nA\nB"), 4096, boom}, ports.Limits{})
	if e != nil {
		t.Fatal(e)
	}
	a, e := r.Next(context.Background())
	if e != nil || a.Record.ID != "A" {
		t.Fatal(e)
	}
	_, e = r.Next(context.Background())
	var se *ports.SourceError
	if !errors.As(e, &se) || se.Kind != ports.SourceIO {
		t.Fatal(e)
	}
}
func FuzzNeptuneCSV(f *testing.F) {
	for _, s := range []string{"~id\nA\n", "~id,x:String\n\"\",\"line\r\ntext\"\n", "~id\n\"bad", "~id,x:Double\nA,NaN\n"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<18 {
			return
		}
		ctx := context.Background()
		r, e := (Decoder{}).New(ctx, ports.Nodes, ports.Entry{}, strings.NewReader(string(b)), ports.Limits{MaxRecordBytes: 65536})
		if e != nil {
			return
		}
		for i := 0; i <= len(b)+1; i++ {
			ev, e := r.Next(ctx)
			if e != nil {
				return
			}
			if (ev.Record == nil) == (ev.Rejection == nil) {
				t.Fatal("event contract")
			}
		}
		t.Fatal("no progress")
	})
}
