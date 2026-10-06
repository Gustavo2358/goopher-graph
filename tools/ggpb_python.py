#!/usr/bin/env python3
"""Optional official-Protobuf interoperability consumer (not a Go dependency).
Generate result_pb2.py from ggpb/pb/result.proto, add its directory to PYTHONPATH.
Outputs counts/type histogram while retaining only one bounded frame.
"""
import argparse, collections, json, struct, zlib
import result_pb2
p=argparse.ArgumentParser();p.add_argument('input');a=p.parse_args()
def exact(f,n):
    data=f.read(n)
    if len(data)!=n: raise ValueError('truncated GGPB')
    return data
counts=collections.Counter();types=collections.Counter();phase=None;header=None;ended=False
with open(a.input,'rb') as f:
    if exact(f,8)!=b'GGPB\r\n\x1a\n': raise ValueError('magic')
    while True:
        prefix=f.read(4)
        if not prefix:
            if not ended: raise ValueError('missing ResultEnd')
            break
        if ended or len(prefix)!=4: raise ValueError('trailing/truncated framing')
        size=struct.unpack('<I',prefix)[0]
        if not 0<size<=4*1024*1024: raise ValueError('frame limit')
        data=exact(f,size)
        if zlib.crc32(data)!=struct.unpack('<I',exact(f,4))[0]: raise ValueError('CRC32')
        batch=result_pb2.Batch.FromString(data);kind=batch.WhichOneof('payload')
        if header is None:
            if kind!='header' or batch.header.version!=1: raise ValueError('version/header')
            header=batch.header
            continue
        if kind=='records':
            records=batch.records
            def symbol(s):
                if s.ref:
                    if s.text or s.ref>len(records.dictionary): raise ValueError('dictionary')
                    return records.dictionary[s.ref-1]
                return s.text
            for part in records.parts:
                entity=part.WhichOneof('entity')
                if entity not in ('node','edge'): raise ValueError('part')
                record=getattr(part,entity)
                if record.HasField('id'):
                    if phase is not None: raise ValueError('interleaving')
                    phase=entity
                    if entity=='edge':
                        if not record.HasField('source') or not record.HasField('target'): raise ValueError('endpoints')
                        symbol(record.label)
                elif phase!=entity: raise ValueError('continuation')
                if entity=='node':
                    for label in record.labels: symbol(label)
                for prop in record.properties:
                    symbol(prop.key)
                    expected={1:'boolean',2:'integer',3:'integer',4:'integer',5:'integer',6:'float_value',7:'double_value',8:'text',9:'text',10:'text'}
                    if prop.WhichOneof('value')!=expected.get(prop.kind): raise ValueError('typed value')
                    types[result_pb2.Kind.Name(prop.kind)]+=1
                if record.last:
                    counts[entity]+=1;phase=None
        elif kind=='end':
            if phase is not None or (counts['node'],counts['edge'])!=(batch.end.nodes,batch.end.edges) or (header.nodes,header.edges)!=(batch.end.nodes,batch.end.edges): raise ValueError('counts/end')
            ended=True
        else: raise ValueError('unexpected batch')
print(json.dumps({'nodes':counts['node'],'edges':counts['edge'],'types':dict(types),'partialSnapshot':header.partial_snapshot},sort_keys=True))
