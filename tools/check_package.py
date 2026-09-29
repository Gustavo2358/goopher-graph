#!/usr/bin/env python3
"""Confere documentação, modelos, oráculos e declarações; NÃO testa a engine Go."""
from __future__ import annotations
import csv
import io
import json
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'tools'))
from reference_snapshot import encode_expected, decode_reference, LAYOUT, TAGS

def require(condition: bool, message: str) -> None:
    if not condition: raise AssertionError(message)

def check_links() -> int:
    docs=list(ROOT.rglob('*.md'))
    for file in docs:
        text=file.read_text(encoding='utf8')
        for target in re.findall(r'\]\(([^)]+)\)', text):
            if target.startswith(('https://','http://','#','mailto:')): continue
            target=unquote(target.split('#')[0])
            if target: require((file.parent/target).exists(), f'broken link: {file.relative_to(ROOT)} -> {target}')
        require(text.count('```') % 2 == 0, f'unclosed code fence: {file}')
    return len(docs)

def check_layout() -> None:
    fields=LAYOUT['header_fields']
    used=[]
    for offset,length,_ in fields:used.extend(range(offset,offset+length))
    require(sorted(used)==list(range(64)), 'header gaps/overlap')
    require([x['id'] for x in LAYOUT['sections']]==list(range(1,25)), 'section ids')
    for name,record in LAYOUT['records'].items():
        used=[]
        for offset,length,_ in record['fields']: used.extend(range(offset,offset+length))
        require(sorted(used)==list(range(record['size'])),f'record fields {name}')

def check_dag() -> int:
    dag=json.loads((ROOT/'spec/backlog_dag.json').read_text())
    pending=set(); done=set()
    def visit(node: str) -> None:
        require(node in dag,f'unknown dependency {node}')
        require(node not in pending, f'cycle at {node}')
        if node in done:return
        pending.add(node)
        for dep in dag[node]:visit(dep)
        pending.remove(node);done.add(node)
    backlog=(ROOT/'BACKLOG.md').read_text()
    state=(ROOT/'PROGRESS.md').read_text()
    for node in dag:
        visit(node)
        require(f'## {node} ' in backlog, f'missing card {node}')
        # Apenas exige presença: depois da implementação o estado deixará de ser Pendente.
        require(f'| {node} |' in state, f'missing progress row {node}')
    return len(dag)

def norm_graph(graph: dict) -> str:
    result={'nodes':[],'edges':[]}
    for kind in ('nodes','edges'):
        for owner in sorted(graph[kind],key=lambda x:x['id']):
            row=dict(owner)
            if 'labels' in row:row['labels']=sorted(row['labels'])
            row['properties']=sorted(row['properties'],key=lambda x:(x['key'],x['type'],json.dumps(x['value'],ensure_ascii=False)))
            result[kind].append(row)
    return json.dumps(result,ensure_ascii=False,sort_keys=True,allow_nan=False)

def closure(graph: dict, labels: list[str] | None) -> tuple[list[str], list[list[bool]]]:
    ids=sorted(x['id'] for x in graph['nodes']); positions={x:i for i,x in enumerate(ids)}
    n=len(ids); matrix=[[a==b for b in range(n)] for a in range(n)]
    for e in graph['edges']:
        if labels is None or e['label'] in labels: matrix[positions[e['source']]][positions[e['target']]]=True
    for k in range(n):
        for i in range(n):
            for j in range(n):matrix[i][j]=matrix[i][j] or (matrix[i][k] and matrix[k][j])
    return ids,matrix

def check_fixtures() -> tuple[int,int,int]:
    files=list(ROOT.glob('fixtures/*/expected.json')); query_count=0; lexical_count=0
    for path in files:
        exp=json.loads(path.read_text(encoding='utf8')); graph=exp['graph']
        ids={x['id'] for x in graph['nodes']}
        require(len(ids)==len(graph['nodes']),f'duplicate node: {path}')
        require(len({x['id'] for x in graph['edges']})==len(graph['edges']),f'duplicate edge: {path}')
        require(exp['load']['nodes']==len(ids), f'node report: {path}')
        require(exp['load']['edges']==len(graph['edges']),f'edge report: {path}')
        for e in graph['edges']: require(e['source'] in ids and e['target'] in ids, f'dangling edge: {path}')
        for owner in graph['nodes']+graph['edges']:
            for p in owner['properties']:require(p['type'] in TAGS,f'unknown type {path}')
        for q in exp['queries']:
            names,m=closure(graph,q['edge_labels']); a=names.index(q['start'])
            if q['kind']=='territory': members={names[j] for j in range(len(names)) if m[a][j]}
            elif q['kind']=='anti_territory': members={names[j] for j in range(len(names)) if m[j][a]}
            else:
                b=names.index(q['end']);members={names[j] for j in range(len(names)) if m[a][j] and m[j][b]}
            selected={e['id'] for e in graph['edges'] if e['source'] in members and e['target'] in members and (q['edge_labels'] is None or e['label'] in q['edge_labels'])}
            require(sorted(members)==q['members'],f'query nodes: {path}, {q}')
            require(sorted(selected)==q['edges'],f'query edges: {path}, {q}')
            require(sorted(members if q['kind']=='between' else members-{q['start']})==q['ids_default'],f'query display: {path}')
            query_count+=1
        data=encode_expected(exp); decoded=decode_reference(data)
        require(norm_graph(decoded['graph'])==norm_graph(graph),f'round-trip model: {path}')
        for role in ('nodes','edges'):
            require((path.parent/role).is_dir(),f'missing catalog {path}')
            for f in (path.parent/role).iterdir():
                if f.relative_to(path.parent).as_posix() in exp['lexically_invalid_sources']:continue
                raw=f.read_bytes().decode('utf8')
                rows=list(csv.reader(io.StringIO(raw,newline=''),strict=True))
                if rows:
                    width=len(rows[0]);require(all(len(row)==width for row in rows), f'fixture arity: {f}')
                lexical_count+=1
    return len(files),query_count,lexical_count

def check_goldens() -> int:
    folder=ROOT/'fixtures/snapshot_reference'
    cases=json.loads((folder/'cases.json').read_text())
    for case in cases:
        expected=json.loads((folder/case['expected']).read_text())
        data=(folder/case['snapshot']).read_bytes()
        require(len(data)==case['bytes'],f'golden size {case}')
        require(data==encode_expected(expected),f'golden bytes {case}')
        decode_reference(data)
    return len(cases)

def check_go() -> tuple[int,str]:
    formatter=shutil.which('gofmt'); go=shutil.which('go')
    snippets=list((ROOT/'spec/api').glob('*.go.txt'))+list((ROOT/'examples').glob('*.go.txt'))
    if not formatter or not go:return 0,'NÃO EXECUTADO: toolchain Go indisponível'
    for path in snippets:
        formatted=subprocess.run([formatter,'-d',str(path)],check=True,capture_output=True,text=True)
        require(not formatted.stdout, f'snippet is not gofmt-clean: {path}')
    env=dict(__import__('os').environ,CGO_ENABLED='0',GOTOOLCHAIN='local',GOPROXY='off',GOSUMDB='off')
    probe=subprocess.run([go,'run',str(ROOT/'tools/go_semantics_probe.go')],cwd=ROOT,env=env,check=True,capture_output=True,text=True)
    return len(snippets),'PASS (sintaxe gofmt, não typecheck/engine); '+probe.stdout.strip()

def main() -> None:
    docs=check_links();check_layout();dag=check_dag()
    fixtures,queries,lexical=check_fixtures();goldens=check_goldens();snippets,gresult=check_go()
    print('Conferência do pacote — não é execução da engine')
    print(f'Markdown e links locais: {docs} arquivos, PASS')
    print(f'Backlog: {dag} fatias, DAG acíclico, PASS')
    print('Layout: 64 bytes de header, 24 seções e records sem sobreposição, PASS')
    print(f'Fixtures: {fixtures} modelos, {queries} consultas contra closure independente, PASS')
    print(f'Fontes lexicais positivas: {lexical}, PASS (exclui negativos intencionais)')
    print(f'Goldens: {goldens}, bytes e round-trip de referência, PASS')
    print(f'Go: {snippets} snippets, {gresult}')
    print('Este checker não executa a engine, mmap ou Neptune. Testes do produto: go test ./...')

if __name__=='__main__':
    try: main()
    except (AssertionError,ValueError,KeyError,UnicodeError,subprocess.CalledProcessError) as error:
        print(f'FAIL: {error}',file=sys.stderr)
        if isinstance(error,subprocess.CalledProcessError):print(error.stderr,file=sys.stderr)
        raise SystemExit(1)
