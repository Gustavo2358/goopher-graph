#!/usr/bin/env python3
"""Explicit local qualification; binaries/dependencies/fixtures built beforehand.
Writes raw JSONL evidence. No corporate data, deployments or downloads.
"""
import argparse
import json
from pathlib import Path
import subprocess
import sys
import threading
import time
import urllib.request
from remote_smoke import ROOT, server

def metrics(address):
    with urllib.request.urlopen("http://"+address+"/metrics",timeout=5) as f:
        rows=f.read().decode().splitlines()
    values={}
    for row in rows:
        if not row or row.startswith("#"):continue
        key,value=row.rsplit(" ",1)
        if '{' not in key:values[key]=float(value)
        elif 'query="territory"' in key or 'query="wasm:shared-targets"' in key:
            values[key]=float(value)
    return values

def cpu(process):
    fields=Path(f"/proc/{process.pid}/stat").read_text().split()
    import os
    return (int(fields[13])+int(fields[14]))/os.sysconf("SC_CLK_TCK")

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--output",default="docs/benchmarks/remote_samples.jsonl")
    p.add_argument("--count",type=int,default=100)
    a=p.parse_args()
    with open(a.output,"w") as output:
        for mode in ("locked","warm","lazy"):
            for payload in ("small","medium","large"):
                with server(ROOT/"bin/gophergraph-server",ROOT/(".measure/remote-"+payload+".snapshot"),mode,16,("--wasm-concurrency=16","--metrics-listen=127.0.0.1:0")) as (process,address,stub):
                    for client in ("go","python"):
                        for name in ("territory","shared-targets"):
                            for concurrency in (1,2,4,8,16):
                                before=metrics(process.metrics_address);start_cpu=cpu(process)
                                stop=threading.Event();samples=[]
                                def monitor():
                                    while not stop.wait(.1):
                                        try:samples.append(metrics(process.metrics_address))
                                        except Exception:pass
                                thread=threading.Thread(target=monitor);thread.start()
                                command=[str(ROOT/"bin/gophergraph-remotemeasure")] if client=="go" else [sys.executable,str(ROOT/"clients/python/client.py"),"--insecure"]
                                command += ["--address",address,"--query",name,"--node=n000000000","--count",str(a.count),"--concurrency",str(concurrency)]
                                if client=="python" and name=="shared-targets":command += ["n000000000","n000000000"]
                                try:result=json.loads(subprocess.check_output(command,text=True))
                                finally:stop.set();thread.join()
                                after=metrics(process.metrics_address)
                                result.update(mode=mode,payload=payload,server_cpu_seconds=cpu(process)-start_cpu,server_rss_peak=max([after.get("process_resident_memory_bytes",0),*[s.get("process_resident_memory_bytes",0) for s in samples]]),server_heap_peak=max([after["go_memstats_heap_alloc_bytes"],*[s["go_memstats_heap_alloc_bytes"] for s in samples]]),server_goroutines_peak=max([after["go_goroutines"],*[s["go_goroutines"] for s in samples]]),server_metric_delta={key:after[key]-value for key,value in before.items() if key.endswith("_total") or "seconds_total{" in key or "_total{" in key})
                                output.write(json.dumps(result)+"\n");output.flush()
                                print(mode,payload,client,name,concurrency,"p50",round(result["p50_ms"],2),"p95",round(result["p95_ms"],2),flush=True)
if __name__=="__main__":main()
