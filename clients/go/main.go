// A reference consumer: one RPC, incremental batches, no explicit pagination.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"gophergraph/ggpb"
	"gophergraph/remote/pb"
	"io"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	address := flag.String("address", "127.0.0.1:9090", "server")
	plain := flag.Bool("insecure", false, "explicit plaintext transport")
	name := flag.String("query", "territory", "territory, anti-territory, between or installed WASM name")
	from := flag.String("node", "", "external ID (including empty)")
	to := flag.String("to", "", "between target")
	timeout := flag.Duration("timeout", 30*time.Second, "required client deadline")
	flag.Parse()
	creds := credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	if *plain {
		creds = insecure.NewCredentials()
	}
	conn, err := grpc.NewClient(*address, grpc.WithTransportCredentials(creds), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(ggpb.MaxFrame+16)))
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	stub := pb.NewGraphServiceClient(conn)
	var stream grpc.ServerStreamingClient[pb.EncodedBatch]
	switch *name {
	case "territory":
		stream, err = stub.Territory(ctx, &pb.TerritoryRequest{Node: from})
	case "anti-territory":
		stream, err = stub.AntiTerritory(ctx, &pb.AntiTerritoryRequest{Node: from})
	case "between":
		stream, err = stub.Between(ctx, &pb.BetweenRequest{From: from, To: to})
	default:
		stream, err = stub.RunWasm(ctx, &pb.RunWasmRequest{QueryName: strings.TrimPrefix(*name, "wasm:"), Args: flag.Args()})
	}
	if err != nil {
		return err
	}
	var decoder ggpb.BatchDecoder
	var count, payload uint64
	for {
		wrapper, e := stream.Recv()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
		batch, e := decoder.Decode(ctx, wrapper.Ggpb)
		if e != nil {
			return e
		}
		count++
		payload += uint64(len(wrapper.Ggpb))
		if h := batch.GetHeader(); h != nil {
			fmt.Printf("query=%s nodes=%d edges=%d partial=%t\n", h.Query.Name, h.Nodes, h.Edges, h.PartialSnapshot)
		}
		// Consume typed parts/dictionary here, retaining at most one batch.
	}
	if err = decoder.Finish(); err != nil {
		return errors.Join(errors.New("incomplete result"), err)
	}
	fmt.Printf("complete batches=%d payload_bytes=%d\n", count, payload)
	return nil
}
