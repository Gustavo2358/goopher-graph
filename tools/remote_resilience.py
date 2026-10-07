#!/usr/bin/env python3
"""Separate-process slow consumer/RSS and overload probes on compact locked data."""
import json
import time
from pathlib import Path
import grpc
from remote_smoke import ROOT, server
from remote.pb import service_pb2 as api

def rss(process):
    for row in Path(f"/proc/{process.pid}/status").read_text().splitlines():
        if row.startswith("VmRSS:"): return int(row.split()[1])*1024
    raise RuntimeError("RSS unavailable")

def main():
    rows=[]
    for payload,scalar in (("large",2048),("wide",32768)):
        with server(ROOT/"bin/gophergraph-server",ROOT/f".measure/remote-{payload}.snapshot",capacity=1) as (process,_,stub):
            before=rss(process)
            stream=stub.Territory(api.TerritoryRequest(node="n000000000"),timeout=30)
            first=next(stream)
            assert first.ggpb
            samples=[]
            for _ in range(20):time.sleep(.05);samples.append(rss(process))
            rejected=stub.Territory(api.TerritoryRequest(node="n000000000"),timeout=5)
            try:next(rejected);raise AssertionError("overload accepted")
            except grpc.RpcError as error:assert error.code()==grpc.StatusCode.RESOURCE_EXHAUSTED
            finally:rejected.cancel()
            stream.cancel()
            # Wait for admission cleanup by retrying, never building a queue.
            for _ in range(100):
                probe=stub.Territory(api.TerritoryRequest(node="n000000000"),timeout=1)
                try:next(probe);break
                except grpc.RpcError as error:
                    assert error.code()==grpc.StatusCode.RESOURCE_EXHAUSTED
                    time.sleep(.01)
                finally:probe.cancel()
            else:raise AssertionError("token leak")
            row=dict(payload=payload,logical_property_bytes=8000*scalar,rss_before=before,rss_paused_peak=max(samples),rss_growth=max(samples)-before,overload="RESOURCE_EXHAUSTED",cancel_cleanup="pass")
            assert row["rss_growth"]<32*1024*1024,row
            rows.append(row)
    print(json.dumps(rows))
if __name__=="__main__":main()
