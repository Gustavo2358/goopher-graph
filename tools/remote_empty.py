#!/usr/bin/env python3
import json
import subprocess
import sys
from remote_smoke import ROOT, server

def main():
    rows=[]
    for mode in ("locked","warm","lazy"):
        with server(ROOT/"bin/gophergraph-server",ROOT/".measure/remote-empty.snapshot",mode,16,("--wasm-concurrency=16",)) as (_,address,_):
            for client in ("go","python"):
                for wasm in (False,True):
                    for concurrency in (1,2,4,8,16):
                        command=[str(ROOT/"bin/gophergraph-remotemeasure")] if client=="go" else [sys.executable,str(ROOT/"clients/python/client.py"),"--insecure"]
                        name="wasm:between" if wasm else ("between-empty" if client=="go" else "between")
                        command += ["--address",address,"--query",name,"--node=n000000000","--to=n000000090","--count=100","--concurrency",str(concurrency)]
                        if client=="python" and wasm:command += ["n000000000","n000000090"]
                        result=json.loads(subprocess.check_output(command,text=True));result.update(mode=mode,payload="empty")
                        if client=="go":assert result["codes"]=={"OK":100} and result["batches"]==200
                        else:assert result["nodes"]==0 and result["edges"]==0 and result["batches"]==200
                        rows.append(result)
    with open(ROOT/"docs/benchmarks/remote_empty.jsonl","w") as output:
        for row in rows:output.write(json.dumps(row)+"\n")
    print("empty result campaign PASS",len(rows),"cases")
if __name__=="__main__":main()
