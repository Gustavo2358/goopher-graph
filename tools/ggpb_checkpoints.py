#!/usr/bin/env python3
"""Measure four fixed cases at one checkpoint using already-built tools/data."""
import argparse, hashlib, json, pathlib, subprocess
p = argparse.ArgumentParser()
p.add_argument('--bin', required=True)
p.add_argument('--fixtures', required=True)
p.add_argument('--work', required=True)
p.add_argument('--checkpoint', required=True)
p.add_argument('--repeats', type=int, default=3)
a = p.parse_args()
work = pathlib.Path(a.work); work.mkdir(parents=True, exist_ok=True)
def run(tool, *args):
    return subprocess.check_output([str(pathlib.Path(a.bin)/tool), *map(str, args)], text=True)
with (work/(a.checkpoint+'.jsonl')).open('w') as log:
    for n, empty in [(100, False), (10000, False), (100000, False), (100, True)]:
        snapshot = pathlib.Path(a.fixtures)/str(n)/'graph.snapshot'
        case = 'empty' if empty else str(n)
        for fmt in ['json', 'ggpb', 'ggpb-inline']:
            payload = work/(case+'.'+fmt)
            digest = None
            for rep in range(a.repeats):
                for mode in ['encode', 'decode']:
                    args = ['--mode', mode, '--format', fmt]
                    if mode == 'encode':
                        args += ['--snapshot', snapshot, '--output', payload]
                        if empty: args += ['--query', 'between', '--to', 'n000000090']
                    else: args += ['--input', payload]
                    r = json.loads(run('resultmeasure', *args))
                    if mode == 'encode':
                        with payload.open('rb') as f: current = hashlib.file_digest(f, 'sha256').hexdigest()
                        if digest and current != digest: raise ValueError('non-deterministic '+case)
                        digest = current; r['SHA256'] = current
                    r.update(Checkpoint=a.checkpoint, Case=case, Repeat=rep, Environment='cold')
                    log.write(json.dumps(r)+'\n'); log.flush()
            args = ['--snapshot', snapshot, '--format', fmt, '--repeats', a.repeats, '--output', payload]
            if empty: args += ['--empty']
            for line in run('encodingmeasure', *args).splitlines():
                r = json.loads(line); r.update(Checkpoint=a.checkpoint, Case=case, Mode='encode', Environment='resident')
                log.write(json.dumps(r)+'\n'); log.flush()
        print(a.checkpoint, case, flush=True)
