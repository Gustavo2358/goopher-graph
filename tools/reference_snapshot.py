#!/usr/bin/env python3
"""Oráculo pequeno do layout. Lê modelos normalizados, NÃO lê CSV e NÃO é a engine."""
from __future__ import annotations
import argparse
from collections import defaultdict
import json
import math
from pathlib import Path
import struct
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
LAYOUT = json.loads((ROOT / 'spec/snapshot_layout.json').read_text())
TAGS = LAYOUT['type_tags']
NAMES = {v: k for k, v in TAGS.items()}
MASK64 = (1 << 64) - 1

def align(n: int) -> int:
    return (n + 63) & ~63

def pack_array(fmt: str, values: list[int]) -> bytes:
    return b''.join(struct.pack('<' + fmt, x) for x in values)

def payload(prop: dict[str, Any], string_ids: dict[str, int]) -> int:
    typ, value = prop['type'], prop['value']
    if typ == 'Bool':
        if not isinstance(value, bool): raise ValueError('Bool requires bool')
        return int(value)
    if typ in ('Byte', 'Short', 'Int', 'Long'):
        bits = {'Byte': 8, 'Short': 16, 'Int': 32, 'Long': 64}[typ]
        if not isinstance(value, int) or not -(1 << (bits-1)) <= value < (1 << (bits-1)):
            raise ValueError(f'invalid {typ}: {value}')
        return value & MASK64
    if typ in ('Float', 'Double'):
        v = float(value)
        if math.isnan(v): return 0x7fc00000 if typ == 'Float' else 0x7ff8000000000000
        return struct.unpack('<I' if typ == 'Float' else '<Q', struct.pack('<f' if typ == 'Float' else '<d', v))[0]
    if typ in ('String', 'Date', 'Datetime'): return string_ids[value]
    raise ValueError(f'unknown type {typ}')

def encode_expected(expected: dict[str, Any]) -> bytes:
    graph = expected['graph']
    nodes = sorted(graph['nodes'], key=lambda x: x['id'].encode('utf8'))
    edges = sorted(graph['edges'], key=lambda x: x['id'].encode('utf8'))
    keys = sorted(set(expected.get('index_properties', [])), key=lambda x: x.encode('utf8'))
    node_ids = {x['id']: i for i, x in enumerate(nodes)}
    if len(node_ids) != len(nodes): raise ValueError('duplicate node')
    if len({e['id'] for e in edges}) != len(edges): raise ValueError('duplicate edge')
    strings = {''} | set(keys)
    for owner in nodes + edges:
        strings.add(owner['id'])
        strings.update(owner.get('labels', [owner.get('label')]))
        for p in owner['properties']:
            strings.add(p['key'])
            if p['type'] in ('String','Date','Datetime'): strings.add(p['value'])
    if None in strings: raise ValueError('missing label')
    ss = sorted(strings, key=lambda x: x.encode('utf8'))
    sid = {x: i for i,x in enumerate(ss)}
    sections: dict[int, bytes] = {}
    b = bytearray(); offsets = [0]
    for value in ss:
        b.extend(value.encode('utf8')); offsets.append(len(b))
    sections[1] = pack_array('Q', offsets); sections[2] = bytes(b)
    sections[3] = pack_array('I', [sid[x['id']] for x in nodes])
    labels = []; loff = [0]
    label_groups: dict[tuple[int, int], set[int]] = defaultdict(set)
    prop_groups: dict[tuple[int, int, int, int], set[int]] = defaultdict(set)
    for i,node in enumerate(nodes):
        ls = sorted(set(sid[x] for x in node['labels']))
        if not ls or 0 in ls: raise ValueError('empty node label')
        labels.extend(ls); loff.append(len(labels))
        for label in ls: label_groups[(0,label)].add(i)
    sections[4] = pack_array('Q', loff); sections[5] = pack_array('I', labels)
    for kind, owners, offid, propid in ((0,nodes,6,7),(1,edges,12,13)):
        poff=[0]; pb=bytearray(); pcount=0
        for i,owner in enumerate(owners):
            props=sorted(set((sid[p['key']],TAGS[p['type']],payload(p,sid)) for p in owner['properties']))
            if kind==1 and len({x[0] for x in props}) != len(props): raise ValueError('edge cardinality')
            for key,tag,value in props:
                if key==0: raise ValueError('empty key')
                pb.extend(struct.pack('<IBBHQ',key,tag,0,0,value)); pcount+=1
                if ss[key] in keys: prop_groups[(kind,key,tag,value)].add(i)
            poff.append(pcount)
        sections[offid]=pack_array('Q',poff); sections[propid]=bytes(pb)
    sections[8]=pack_array('I',[sid[x['id']] for x in edges])
    sections[9]=pack_array('I',[node_ids[x['source']] for x in edges])
    sections[10]=pack_array('I',[node_ids[x['target']] for x in edges])
    sections[11]=pack_array('I',[sid[x['label']] for x in edges])
    forward=[[] for _ in nodes]; reverse=[[] for _ in nodes]
    for i,edge in enumerate(edges):
        a,b=node_ids[edge['source']],node_ids[edge['target']]; label=sid[edge['label']]
        if label==0: raise ValueError('empty edge label')
        forward[a].append((b,label,i)); reverse[b].append((a,label,i))
        label_groups[(1,label)].add(i)
    for lists,base in ((forward,14),(reverse,17)):
        offsets=[0]; neighbors=[]; ids=[]
        for adjacency in lists:
            for neighbor,_,edge in sorted(adjacency): neighbors.append(neighbor); ids.append(edge)
            offsets.append(len(ids))
        sections[base]=pack_array('Q',offsets)
        sections[base+1]=pack_array('I',neighbors)
        sections[base+2]=pack_array('I',ids)
    lb=bytearray(); postings=[]
    for (kind,label),members in sorted(label_groups.items()):
        lb.extend(struct.pack('<IIQQ',kind,label,len(postings),len(members)))
        postings.extend(sorted(members))
    sections[20]=bytes(lb); sections[21]=pack_array('I',postings)
    sections[22]=pack_array('I',sorted(sid[x] for x in keys))
    pb=bytearray(); postings=[]
    for (kind,key,tag,value),members in sorted(prop_groups.items()):
        pb.extend(struct.pack('<BBHIQQQ',kind,tag,0,key,value,len(postings),len(members)))
        postings.extend(sorted(members))
    sections[23]=bytes(pb); sections[24]=pack_array('I',postings)
    out=bytearray(832); entries=[]
    for sec in LAYOUT['sections']:
        data=sections[sec['id']]; stride=sec['element_size']
        if len(data)%stride: raise ValueError('invalid stride')
        offset=len(out); entries.append((sec['id'],stride,offset,len(data),len(data)//stride))
        out.extend(data); out.extend(b'\0'*(align(len(out))-len(out)))
    flags=1 if expected['load']['completeness']=='PARTIAL' else 0
    struct.pack_into('<8sIIQQQQIIQ',out,0,bytes.fromhex(LAYOUT['magic_hex']),1,64,flags,len(out),len(nodes),len(edges),len(ss),24,64)
    for i,entry in enumerate(entries): struct.pack_into('<IIQQQ',out,64+32*i,*entry)
    return bytes(out)

def decode_reference(data: bytes) -> dict[str, Any]:
    """Decoder de referência para modelos pequenos; não substitui validador robusto Go."""
    if len(data)<832: raise ValueError('truncated header')
    magic,version,hsize,flags,size,n,e,s,count,doff=struct.unpack_from('<8sIIQQQQIIQ',data)
    if (magic,version,hsize,count,doff)!=(bytes.fromhex(LAYOUT['magic_hex']),1,64,24,64) or size!=len(data) or flags & ~1:
        raise ValueError('invalid header')
    sections={}; counts={}; end=832
    for i,sec in enumerate(LAYOUT['sections']):
        sid,stride,off,length,num=struct.unpack_from('<IIQQQ',data,64+i*32)
        if sid!=sec['id'] or stride!=sec['element_size'] or off!=align(end) or length!=stride*num or off+length>size:
            raise ValueError('invalid section')
        if any(data[end:off]): raise ValueError('invalid padding')
        sections[sid]=data[off:off+length]; counts[sid]=num; end=off+length
    if align(end)!=size or any(data[end:]): raise ValueError('invalid tail')
    def arr(i: int,fmt: str) -> list[int]:
        return [x[0] for x in struct.iter_unpack('<'+fmt,sections[i])]
    so=arr(1,'Q')
    if len(so)!=s+1: raise ValueError('string offsets')
    strings=[sections[2][so[i]:so[i+1]].decode('utf8') for i in range(s)]
    def prop_list(section: int) -> list[dict[str, Any]]:
        props=[]
        for key,tag,zero,res,val in struct.iter_unpack('<IBBHQ',sections[section]):
            if zero or res or tag not in NAMES: raise ValueError('property')
            typ=NAMES[tag]
            if typ=='Bool': value=bool(val)
            elif typ in ('Byte','Short','Int','Long'): value=val if val<(1<<63) else val-(1<<64)
            elif typ in ('Float','Double'):
                value=struct.unpack('<f' if typ=='Float' else '<d',struct.pack('<I' if typ=='Float' else '<Q',val))[0]
                if math.isnan(value):value='NaN'
                elif math.isinf(value):value='Infinity' if value>0 else '-Infinity'
            else:value=strings[val]
            props.append({'key':strings[key],'type':typ,'value':value})
        return props
    nodeext=arr(3,'I'); nlo=arr(4,'Q'); nl=arr(5,'I'); npo=arr(6,'Q'); np=prop_list(7)
    edgeext=arr(8,'I'); src=arr(9,'I'); dst=arr(10,'I'); el=arr(11,'I'); epo=arr(12,'Q'); ep=prop_list(13)
    if len(nodeext)!=n or len(edgeext)!=e:raise ValueError('entity count')
    nodes=[{'id':strings[nodeext[i]],'labels':[strings[x] for x in nl[nlo[i]:nlo[i+1]]],
            'properties':np[npo[i]:npo[i+1]]} for i in range(n)]
    edges=[{'id':strings[edgeext[i]],'source':nodes[src[i]]['id'],'target':nodes[dst[i]]['id'],
            'label':strings[el[i]],'properties':ep[epo[i]:epo[i+1]]} for i in range(e)]
    expected={'graph':{'nodes':nodes,'edges':edges},'load':{'completeness':'PARTIAL' if flags else 'COMPLETE'},
              'index_properties':[strings[x] for x in arr(22,'I')]}
    # Reconstrução detecta inconsistência de CSR/postings que a leitura de modelo não usa.
    if encode_expected(expected)!=data:raise ValueError('noncanonical or inconsistent graph/index data')
    return expected

def main() -> None:
    parser=argparse.ArgumentParser(description=__doc__)
    sub=parser.add_subparsers(dest='command',required=True)
    enc=sub.add_parser('encode'); enc.add_argument('expected',type=Path); enc.add_argument('output',type=Path)
    dec=sub.add_parser('inspect'); dec.add_argument('snapshot',type=Path)
    args=parser.parse_args()
    if args.command=='encode':
        data=encode_expected(json.loads(args.expected.read_text(encoding='utf8')))
        args.output.write_bytes(data); print(f'{args.output}: {len(data)} bytes (reference, not the Go engine)')
    else: print(json.dumps(decode_reference(args.snapshot.read_bytes()),ensure_ascii=False,indent=2,allow_nan=False))

if __name__=='__main__':main()
