"""保留全部轮次，独立复算正常入口的配对比值，不扣减self控制。"""
from pathlib import Path
import json,re,statistics,math,hashlib
here=Path(__file__).resolve().parent;plan=json.loads((here/'plan.json').read_text());commands=json.loads((here/'commands.json').read_text());assert all(x['exitCode']==0 for x in commands)
summary={};processes=0;lines=0
for order in plan['orders']:
 mode=order['mode'];r=order['round'];blocks=[]
 for position,arm in enumerate(order['order']):
  p=here/f'{mode}-{r}-{position}-{arm}.txt';data={}
  for line in p.read_text().splitlines():
   m=re.match(r'(Benchmark\S+?)(?:-\d+)?\s+(\d+)\s+([\d.]+) ns/op\s+([\d.]+) B/op\s+([\d.]+) allocs/op',line)
   if m:data[m[1]]=dict(calls=int(m[2]),ns=float(m[3]),bytes=float(m[4]),allocs=float(m[5]));lines+=1
  assert len(data)==28,(p,len(data));blocks.append((arm,data));processes+=1
 names=set(blocks[0][1]);assert all(set(d)==names for _,d in blocks)
 for name in sorted(names):
  arms={a:[d[name] for arm,d in blocks if arm==a] for a in 'AB'}
  ratio=math.exp(sum(math.log(x['ns']) for x in arms['B'])/2-sum(math.log(x['ns']) for x in arms['A'])/2)
  record=summary.setdefault(name,{}).setdefault(mode,[]);record.append(dict(round=r,ratio=ratio,arms=arms))
rows=['# 正常入口复核','', '负值为候选耗时降低；self为同一基线二进制的标签对照，未扣减。中位基于三轮ABBA/BAAB几何配对比值，非显著性结论。','', '| 用例 | self中位 | 候选中位 | 候选三轮 | B/op A→B | alloc A→B |','| --- | ---: | ---: | --- | --- | --- |']
for name,modes in summary.items():
 pct=lambda r:(r['ratio']-1)*100
 a=[v for r in modes['compare'] for v in r['arms']['A']];b=[v for r in modes['compare'] for v in r['arms']['B']]
 ranges=lambda samples,k:f"{min(x[k] for x in samples):g}..{max(x[k] for x in samples):g}"
 rows.append('| '+name.replace('BenchmarkArrayNormalReview/','').replace('BenchmarkArrayRepresentativeReview/','')+' | '+f"{statistics.median(map(pct,modes['self'])):+.2f}% | {statistics.median(map(pct,modes['compare'])):+.2f}% | "+' / '.join(f'{pct(r):+.2f}%' for r in modes['compare'])+' | '+ranges(a,'bytes')+' → '+ranges(b,'bytes')+' | '+ranges(a,'allocs')+' → '+ranges(b,'allocs')+' |')
(here/'ALL-CASES.md').write_text('\n'.join(rows)+'\n');(here/'analysis.json').write_text(json.dumps(summary,indent=2)+'\n');(here/'validation.json').write_text(json.dumps(dict(processes=processes,benchmarkLines=lines,commands=len(commands),allCommandsPassed=True,hashesVerifiedAfter=plan['hashesVerifiedAfter']),indent=2)+'\n');print('\n'.join(rows))
