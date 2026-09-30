//go:build wasip1 && wasm

package main

import (
	"encoding/json"
	"fmt"
	"gophergraph/wasmquery/internal/abi"
	"gophergraph/wasmquery/sdk"
	"net"
	"os"
	"strconv"
)

//go:wasmimport gophergraph_v1 call
func raw(uint32, uint64, uint64, string, *byte, uint32) uint64

var state int

func check(b bool) {
	if !b {
		os.Exit(7)
	}
}
func main() {
	q := sdk.New()
	mode := os.Args[1]
	switch mode {
	case "spin":
		for {
			state++
		}
	case "no-result":
		return
	case "exit":
		os.Exit(9)
	case "trap":
		panic("guest trap")
	case "invalid":
		raw(abi.Count, 42, 0, "", nil, 0)
	case "unknown":
		raw(999, 0, 0, "", nil, 0)
	case "malformed":
		raw(abi.Nodes, 0, 0, "{", nil, 0)
	case "oversize":
		raw(abi.Nodes, 0, 0, string(make([]byte, abi.MaxMessage+1)), nil, 0)
	case "short-output":
		raw(abi.ReadNodeProperty, 0, 0, `{"id":"A","key":"sigla","labels":null}`, nil, 0)
	case "wrong-kind":
		n := raw(abi.Nodes, 0, 0, `{"ids":["A"]}`, nil, 0)
		raw(abi.Sources, n, 0, "", nil, 0)
	case "stale":
		n := q.Nodes("A")
		n.Release()
		n.Count()
	case "double-return":
		sub := q.Nodes("A").Induced()
		q.Return(sub)
		q.Return(sub)
	case "foreign":
		q.Nodes("A") // A stale token must fail even after this execution allocates a set.
		token, _ := strconv.ParseUint(os.Args[2], 10, 64)
		raw(abi.Count, token, 0, "", nil, 0)
	case "handles":
		for i := 0; i < 1000; i++ {
			q.Nodes("A")
		}
	case "calls":
		n := q.Nodes("A")
		for i := 0; i < 10000; i++ {
			n.Count()
		}
	case "release":
		for i := 0; i < 500; i++ {
			n := q.Nodes("A")
			n.Release()
		}
		q.Return(q.Nodes("A").Induced())
	case "growth":
		keep := make([][]byte, 0)
		for {
			b := make([]byte, 1<<20)
			b[0] = 1
			keep = append(keep, b)
			state += int(keep[len(keep)-1][0])
		}
	case "sandbox":
		_, err := os.ReadFile(os.Args[2])
		check(err != nil)
		check(os.WriteFile(os.Args[2]+".written", []byte("bad"), 0600) != nil)
		check(os.Getenv("GOPHERGRAPH_SECRET") == "")
		_, err = net.Dial("tcp", "127.0.0.1:1")
		check(err != nil)
		var b [1]byte
		n, _ := os.Stdin.Read(b[:])
		check(n == 0)
		fmt.Print("guest output must not become graphjson")
		state++
		check(state == 1)
		check(len(os.Args) == 3)
		q.Return(q.Nodes("A").Induced())
	case "args":
		check(len(os.Args) == 5 && os.Args[2] == "" && os.Args[3] == "line\n☃" && os.Args[4] == "--flag")
		q.Return(q.Nodes("A").Induced())
	case "identity":
		state++
		check(state == 1)
		q.Return(q.Nodes(os.Args[2]).Induced())
	case "large":
		n := q.Nodes("n000000000").Territory()
		edges := n.OutE()
		check(edges.Targets().Count() == n.Count())
		q.Return(q.Subgraph(n, edges))
	case "property":
		var v sdk.Value
		check(json.Unmarshal([]byte(os.Args[4]), &v) == nil)
		if os.Args[2] == "node" {
			got, found := q.NodeProperty(os.Args[5], os.Args[3], 0)
			if len(os.Args) > 6 {
				index, _ := strconv.Atoi(os.Args[6])
				got, found = q.NodeProperty(os.Args[5], os.Args[3], uint32(index))
			}
			check(found && got == v)
			q.Return(q.Nodes(os.Args[5]).Has(os.Args[3], v).Induced())
		} else {
			got, found := q.EdgeProperty(os.Args[5], os.Args[3], 0)
			check(found && got == v)
			q.Return(q.Subgraph(q.Nodes(), q.Nodes(os.Args[6]).OutE().Has(os.Args[3], v)))
		}
	case "native":
		direction, _ := strconv.Atoi(os.Args[3])
		var labels []string
		if len(os.Args) > 4 {
			check(json.Unmarshal([]byte(os.Args[4]), &labels) == nil)
		}
		q.Return(q.Nodes(os.Args[2]).Reachable(sdk.Direction(direction), labels...).Induced(labels...))
	case "coverage":
		a, b := q.Nodes("A"), q.Nodes("B")
		check(a.Out().Count() == 2 && b.In().Count() == 2 && b.Both().Count() == 2)
		check(a.OutE().Count() == 3 && b.InE().Count() == 3 && b.BothE().Count() == 4)
		check(a.Out("missing").Empty() && a.Out([]string{}...).Empty())
		check(a.Out("CALLS").Count() == 2)
		check(a.OutE().Targets().Count() == 2 && a.OutE().Sources().Count() == 1)
		check(q.NodesWithLabel("PROGRAM").Count() == 5 && q.NodesWithLabel("missing").Empty())
		check(a.Out().HasLabel("PROGRAM").Has("sigla", sdk.String("CO")).Count() == 1)
		check(a.OutE().HasLabel("CALLS").Has("line", sdk.Int(10)).Count() == 1)
		check(a.OutE().Has("line", sdk.Long(10)).Empty())
		check(a.Union(b).Count() == 2 && a.Intersection(b).Empty() && a.Union(b).Difference(a).Count() == 1)
		left, right := a.OutE(), b.InE()
		check(left.Union(right).Count() == 4 && left.Intersection(right).Count() == 2 && left.Difference(right).Count() == 1)
		check(q.Nodes("A", "I").Territory().Count() == 6)
		check(q.Nodes("F").BothE().Count() == 2 && q.Nodes("F").Both().Count() == 2)
		check(q.Nodes("I").Reachable(sdk.Bidirectional).Count() == 1)
		v, found := q.NodeProperty("A", "sigla", 0)
		check(found && v == sdk.String("CO"))
		_, found = q.NodeProperty("A", "sigla", 1)
		check(!found)
		_, found = q.NodeProperty("A", "absent", 0)
		check(!found)
		v, found = q.EdgeProperty("e1", "line", 0)
		check(found && v == sdk.Int(10))
		sub := q.Subgraph(q.Nodes("I"), left)
		check(!sub.Empty() && sub.NodeCount() == 4 && sub.EdgeCount() == 3)
		check(sub.Nodes().Count() == 4 && sub.Edges().Count() == 3)
		sub.Release()
		q.Return(q.Nodes("A").Territory().Induced())
	default:
		os.Exit(2)
	}
}
