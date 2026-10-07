// remotemeasure measures a separate server over TCP, retaining one batch only.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gophergraph/ggpb"
	"gophergraph/remote/pb"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
)

type sample struct {
	Millis, FirstMillis, DecodeMillis float64
	Bytes, Batches                    uint64
	Code                              string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	address := flag.String("address", "127.0.0.1:9090", "local benchmark endpoint")
	name := flag.String("query", "territory", "territory, between-empty, or shared-targets")
	node := flag.String("node", "n000000000", "origin")
	target := flag.String("to", "n000000090", "empty target")
	count := flag.Int("count", 100, "samples")
	workers := flag.Int("concurrency", 1, "independent RPCs")
	decode := flag.Bool("decode", true, "incrementally validate GGPB")
	flag.Parse()
	if *count < 1 || *workers < 1 {
		return fmt.Errorf("positive count and concurrency required")
	}
	conn, err := grpc.NewClient(*address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(ggpb.MaxFrame+16)))
	if err != nil {
		return err
	}
	defer conn.Close()
	stub := pb.NewGraphServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_, err = stub.GetServerInfo(ctx, &pb.ServerInfoRequest{})
	cancel()
	if err != nil {
		return err
	}
	jobs := make(chan int)
	samples := make([]sample, *count)
	var wg sync.WaitGroup
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	for range *workers {
		wg.Go(func() {
			for index := range jobs {
				ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
				t := time.Now()
				var stream grpc.ServerStreamingClient[pb.EncodedBatch]
				var err error
				switch *name {
				case "territory":
					stream, err = stub.Territory(ctx, &pb.TerritoryRequest{Node: node})
				case "between-empty":
					stream, err = stub.Between(ctx, &pb.BetweenRequest{From: node, To: target})
				default:
					stream, err = stub.RunWasm(ctx, &pb.RunWasmRequest{QueryName: strings.TrimPrefix(*name, "wasm:"), Args: func() []string {
						if *name == "wasm:between" {
							return []string{*node, *target}
						}
						return []string{*node, *node}
					}()})
				}
				var decoder ggpb.BatchDecoder
				var sample sample
				for err == nil {
					var b *pb.EncodedBatch
					b, err = stream.Recv()
					if err != nil {
						break
					}
					if sample.Batches == 0 {
						sample.FirstMillis = float64(time.Since(t)) / 1e6
					}
					sample.Batches++
					sample.Bytes += uint64(len(b.Ggpb))
					if *decode {
						d := time.Now()
						_, err = decoder.Decode(ctx, b.Ggpb)
						sample.DecodeMillis += float64(time.Since(d)) / 1e6
					}
				}
				if err == io.EOF {
					err = nil
					if *decode {
						err = decoder.Finish()
					}
				}
				sample.Millis = float64(time.Since(t)) / 1e6
				sample.Code = status.Code(err).String()
				samples[index] = sample
				cancel()
			}
		})
	}
	for i := range *count {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	elapsed := time.Since(started)
	runtime.ReadMemStats(&after)
	latencies := make([]float64, 0, *count)
	firsts := make([]float64, 0, *count)
	codes := map[string]int{}
	var bytes, batches uint64
	var decodeMillis float64
	for _, s := range samples {
		codes[s.Code]++
		if s.Code == "OK" {
			latencies = append(latencies, s.Millis)
			firsts = append(firsts, s.FirstMillis)
		}
		bytes += s.Bytes
		batches += s.Batches
		decodeMillis += s.DecodeMillis
	}
	slices.Sort(latencies)
	slices.Sort(firsts)
	p := func(v []float64, q float64) any {
		if len(v) == 0 {
			return nil
		}
		return v[int(float64(len(v)-1)*q)]
	}
	var p99 any
	if len(latencies) >= 100 {
		p99 = p(latencies, .99)
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"client": "go", "query": *name, "concurrency": *workers, "count": *count, "elapsed_seconds": elapsed.Seconds(), "throughput": float64(codes["OK"]) / elapsed.Seconds(), "p50_ms": p(latencies, .5), "p95_ms": p(latencies, .95), "p99_ms": p99, "first_p50_ms": p(firsts, .5), "decode_ms": decodeMillis, "payload_bytes": bytes, "batches": batches, "codes": codes, "client_alloc_bytes": after.TotalAlloc - before.TotalAlloc, "client_allocations": after.Mallocs - before.Mallocs, "client_gc": after.NumGC - before.NumGC, "samples": samples})
}
