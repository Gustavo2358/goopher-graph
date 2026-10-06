#!/usr/bin/env python3
"""Repeat isolated producer/consumer measurements on existing benchfixture data.
Build tools first; arguments: --bin directory --work directory --repeats 5.
No downloads; raw JSONL and generated payloads stay in the work directory.
"""
import argparse, hashlib, json, pathlib, subprocess
p=argparse.ArgumentParser()
p.add_argument('--bin', required=True);p.add_argument('--work', required=True)
p.add_argument('--repeats',type=int,default=5)
a=p.parse_args();tools=pathlib.Path(a.bin);work=pathlib.Path(a.work);work.mkdir(parents=True,exist_ok=True)
def run(tool,*args):
    return subprocess.check_output([str(tools/tool),*map(str,args)],text=True)
hashes={}
with (work/'samples.jsonl').open('w') as samples:
    for n in [100,10000,100000]:
        root=work/str(n);snapshot=root/'graph.snapshot'
        if not snapshot.exists():
            run('benchdata','--nodes',n,'--output',root)
            run('gophergraph','build','--nodes',root/'nodes','--edges',root/'edges','--output',snapshot)
        queries=[('territory','n000000010'),('anti-territory','n000000010'),('between','n000000010')]
        if n==100:
            queries.append(('between','n000000090')) # deliberately empty result
        for query,to in queries:
            case=f'{n}/{query}/{to}'
            for rep in range(a.repeats):
                # Rotate ordering so no one format always runs first.
                formats=['json','ggpb','ggpb-inline'];formats=formats[rep%3:]+formats[:rep%3]
                for fmt in formats:
                    payload=root/f'{query}-{to}.{fmt}'
                    for mode in ['encode','decode']:
                        args=['--mode',mode,'--format',fmt,'--query',query,'--to',to]
                        args+=['--snapshot',snapshot,'--output',payload] if mode=='encode' else ['--input',payload]
                        result=json.loads(run('resultmeasure',*args))
                        if mode=='encode':
                            with payload.open('rb') as f: digest=hashlib.file_digest(f,'sha256').hexdigest()
                            key=(case,fmt)
                            if key in hashes and hashes[key]!=digest: raise ValueError('nondeterministic output: '+case+' '+fmt)
                            hashes[key]=digest
                            result['SHA256']=digest
                        result.update(Case=case,Repeat=rep)
                        samples.write(json.dumps(result)+'\n');samples.flush()
            print(case,flush=True)
