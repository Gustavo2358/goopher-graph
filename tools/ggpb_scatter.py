#!/usr/bin/env python3
"""Adverse locality fixture: connected components, random targets/edge ID order.
Same node/edge counts and property shapes as benchfixture; no dependencies.
This creates a separate measurement fixture, never modifies existing fixtures.
"""
import argparse, math, pathlib
p = argparse.ArgumentParser()
p.add_argument('--nodes', type=int, default=100000)
p.add_argument('--output', required=True)
a = p.parse_args()
if not 10 <= a.nodes <= 1000000: p.error('nodes must be 10..1000000')
root = pathlib.Path(a.output)
for role in ['nodes', 'edges']: (root/role).mkdir(parents=True, exist_ok=True)
def mix(x):
    mask = (1 << 64)-1
    x = (x+0x9e3779b97f4a7c15) & mask
    x = ((x ^ (x >> 30))*0xbf58476d1ce4e5b9) & mask
    x = ((x ^ (x >> 27))*0x94d049bb133111eb) & mask
    return x ^ (x >> 31)
with (root/'nodes/data.csv').open('w') as f:
    f.write('~id,~label,group:String,rank:Int\n')
    for i in range(a.nodes): f.write(f'n{i:09d},TYPE;GROUP,g{i%10},{i}\n')
multiplier = 104729
while math.gcd(multiplier, 5*a.nodes) != 1: multiplier += 2
with (root/'edges/data.csv').open('w') as f:
    f.write('~id,~from,~to,~label,score:Int\n')
    for i in range(a.nodes):
        boundary = a.nodes*9//10
        base, end = (0, boundary) if i < boundary else (boundary, a.nodes)
        for k in range(5):
            target = base+(i-base+1)%(end-base) if k == 0 else base+mix(i*5+k)%(end-base)
            external = (5*i+k)*multiplier%(5*a.nodes)
            label = ['L','L','L','M','U'][k]
            f.write(f'e{external:09d},n{i:09d},n{target:09d},{label},{i%100}\n')
