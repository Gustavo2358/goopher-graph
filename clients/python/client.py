"""Reference official-Protobuf/gRPC client. Retains at most one decoded batch.

Run from this directory; generated modules are checked in. grpc_tools is needed
only to regenerate, not at runtime. RPC exceptions invalidate the entire prefix.
"""
import argparse
import collections
import concurrent.futures
import json
import time
import grpc
from remote.pb import service_pb2 as api
from remote.pb import service_pb2_grpc as rpc
from ggpb.pb import result_pb2 as ggpb


class Consumer:
    def __init__(self):
        self.header = None
        self.ended = False
        self.phase = None
        self.edges = False
        self.counts = collections.Counter()
        self.bytes = self.batches = 0

    def consume(self, payload):
        if self.ended or not 0 < len(payload) <= 4 * 1024 * 1024:
            raise ValueError("batch bound/order")
        self.bytes += len(payload)
        self.batches += 1
        batch = ggpb.Batch.FromString(payload)
        kind = batch.WhichOneof("payload")
        if self.header is None:
            if kind != "header" or batch.header.version != 1 or not batch.header.directed:
                raise ValueError("header/version")
            self.header = batch.header
            return batch
        if kind == "end":
            if self.phase is not None or (self.counts["node"], self.counts["edge"]) != (batch.end.nodes, batch.end.edges) or (self.header.nodes, self.header.edges) != (batch.end.nodes, batch.end.edges):
                raise ValueError("end/counts")
            self.ended = True
            return batch
        if kind != "records" or not batch.records.parts:
            raise ValueError("records required")
        records = batch.records
        if len(records.parts) > 4096 or len(records.dictionary) > 1024 or len(records.endpoint_ids) > 512:
            raise ValueError("records bound")

        def symbol(s):
            if s.ref:
                if s.text or s.ref > len(records.dictionary):
                    raise ValueError("dictionary ref")
                return records.dictionary[s.ref - 1]
            return s.text

        for part in records.parts:
            entity = part.WhichOneof("entity")
            if entity not in ("node", "edge") or (self.edges and entity == "node"):
                raise ValueError("entity order")
            self.edges |= entity == "edge"
            record = getattr(part, entity)
            if record.HasField("id"):
                if self.phase is not None:
                    raise ValueError("interleaving")
                self.phase = entity
                if entity == "edge":
                    for field in ("source", "target"):
                        ref = getattr(record, field + "_ref")
                        if ref:
                            if record.HasField(field) or ref > len(records.endpoint_ids):
                                raise ValueError("endpoint ref")
                        elif not record.HasField(field):
                            raise ValueError("missing endpoint")
                    symbol(record.label)
            elif self.phase != entity:
                raise ValueError("continuation")
            if entity == "node":
                for label in record.labels:
                    symbol(label)
            for prop in record.properties:
                symbol(prop.key)
                expected = {1: "boolean", 2: "integer", 3: "integer", 4: "integer", 5: "integer", 6: "float_value", 7: "double_value", 8: "text", 9: "text", 10: "text"}
                if prop.WhichOneof("value") != expected.get(prop.kind):
                    raise ValueError("typed property")
            if record.last:
                self.counts[entity] += 1
                self.phase = None
        return batch

    def finish(self):
        if not self.ended:
            raise ValueError("missing ResultEnd")


def query(stub, name, node, target, args, timeout):
    if name == "territory":
        stream = stub.Territory(api.TerritoryRequest(node=node), timeout=timeout)
    elif name == "anti-territory":
        stream = stub.AntiTerritory(api.AntiTerritoryRequest(node=node), timeout=timeout)
    elif name == "between":
        stream = stub.Between(api.BetweenRequest(**{"from": node, "to": target}), timeout=timeout)
    else:
        stream = stub.RunWasm(api.RunWasmRequest(query_name=name.removeprefix("wasm:"), args=args), timeout=timeout)
    consumer = Consumer()
    try:
        for batch in stream:
            consumer.consume(batch.ggpb)
        # Exhaustion checked the final RPC status; End alone is insufficient.
        consumer.finish()
        return consumer
    finally:
        stream.cancel()  # Also cancels an abandoned/failed consumer promptly.


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--address", default="127.0.0.1:9090")
    p.add_argument("--insecure", action="store_true")
    p.add_argument("--query", default="territory")
    p.add_argument("--node", default="")
    p.add_argument("--to", default="")
    p.add_argument("--timeout", type=float, default=30)
    p.add_argument("--count", type=int, default=1)
    p.add_argument("--concurrency", type=int, default=1)
    p.add_argument("args", nargs="*")
    a = p.parse_args()
    if a.count < 1 or a.concurrency < 1 or a.timeout <= 0:
        p.error("positive count/concurrency/timeout required")
    options = [("grpc.max_receive_message_length", 4 * 1024 * 1024 + 16)]
    channel = grpc.insecure_channel(a.address, options) if a.insecure else grpc.secure_channel(a.address, grpc.ssl_channel_credentials(), options)
    latencies = []
    payload_bytes = batches = errors = 0
    def run(_):
        started = time.perf_counter()
        result = query(rpc.GraphServiceStub(channel), a.query, a.node, a.to, a.args, a.timeout)
        return (time.perf_counter() - started) * 1000, result.bytes, result.batches, result.header.nodes, result.header.edges
    started = time.perf_counter()
    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=a.concurrency) as pool:
            for elapsed, size, count, nodes, edges in pool.map(run, range(a.count)):
                latencies.append(elapsed)
                payload_bytes += size
                batches += count
    finally:
        channel.close()
    latencies.sort()
    def percentile(p): return latencies[min(len(latencies)-1, int((len(latencies)-1)*p))]
    print(json.dumps(dict(client="python", query=a.query, concurrency=a.concurrency, count=a.count, elapsed_seconds=time.perf_counter()-started, p50_ms=percentile(.5), p95_ms=percentile(.95), p99_ms=percentile(.99) if a.count >= 100 else None, payload_bytes=payload_bytes, batches=batches, nodes=nodes, edges=edges, errors=errors)))

if __name__ == "__main__":
    main()
