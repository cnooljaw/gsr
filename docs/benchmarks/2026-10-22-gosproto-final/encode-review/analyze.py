"""独立复算原生整数数组视图的全部配对轮次，保留控制和反向。"""
from pathlib import Path
import re,json,math,statistics,hashlib
here=Path(__file__).resolve().parent;plan=json.loads((here/'plan.json').read_text());commands=json.loads((here/'commands.json').read_text());assert all(c['exitCode']==0 for c in commands)
summary={};processes=0;lines=0
for order in plan['orders']:
 mode=order['mode'];r=order['round'];blocks=[]
 for position,arm in enumerate(order['order']):
  p=here/f'{mode}-{r}-{position}-{arm}.txt';data={}
  for line in p.read_text().splitlines():
   m=re.match(r'(Benchmark\S+?)(?:-\d+)?\s+(\d+)\s+([\d.]+) ns/op\s+([\d.]+) B/op\s+([\d.]+) allocs/op',line)
   if m:data[m[1]]=dict(calls=int(m[2]),ns=float(m[3]),bytes=float(m[4]),allocs=float(m[5]));lines+=1
  assert len(data)==20,(p,len(data));blocks.append((arm,data));processes+=1
 names=set(blocks[0][1]);assert all(set(d)==names for _,d in blocks)
 for name in sorted(names):
  arms={a:[d[name] for arm,d in blocks if arm==a] for a in 'AB'}
  ratio=math.exp(sum(math.log(x['ns']) for x in arms['B'])/2-sum(math.log(x['ns']) for x in arms['A'])/2)
  summary.setdefault(name,{}).setdefault(mode,[]).append(dict(round=r,ratio=ratio,arms=arms))
rows=['# 原生整数数组视图全部结果','','负值为候选耗时降低；self未扣减，三轮配对比值中位与逐轮均保留。累计按ns/op×调用数估算，因此受benchmark输出舍入限制。','','| 用例 | self中位 | pass1中位/累计 | pass2中位/累计 | pass1三轮 | pass2三轮 | B/op、alloc一致 |','| --- | ---: | ---: | ---: | --- | --- | --- |']
for name,modes in summary.items():
 pct=lambda r:(r['ratio']-1)*100
 parts=[]
 for mode in ['pass1','pass2']:
  av={a:sum(x['ns']*x['calls'] for r in modes[mode] for x in r['arms'][a])/sum(x['calls'] for r in modes[mode] for x in r['arms'][a]) for a in 'AB'}
  parts.append(f"{statistics.median(map(pct,modes[mode])):+.2f}% / {(av['B']/av['A']-1)*100:+.2f}%")
 same=all(len({x[k] for r in modes[mode] for a in 'AB' for x in r['arms'][a]})==1 for mode in ['pass1','pass2'] for k in ['bytes','allocs'])
 rows.append('| '+name.replace('BenchmarkArrayNormalReview/','').replace('BenchmarkArrayRepresentativeReview/','')+' | '+f"{statistics.median(map(pct,modes['self'])):+.2f}% | "+' | '.join(parts)+' | '+' | '.join(' / '.join(f'{pct(r):+.2f}%' for r in modes[m]) for m in ['pass1','pass2'])+' | '+str(same)+' |')
(here/'ALL-CASES.md').write_text('\n'.join(rows)+'\n');(here/'analysis.json').write_text(json.dumps(summary,indent=2)+'\n');(here/'validation.json').write_text(json.dumps(dict(processes=processes,benchmarkLines=lines,commands=len(commands),allCommandsPassed=True,hashesVerifiedAfter=plan['hashesVerifiedAfter']),indent=2)+'\n');print('\n'.join(rows))
