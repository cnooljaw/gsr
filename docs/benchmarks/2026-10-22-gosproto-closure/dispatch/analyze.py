"""同进程逐块复算，不删除长尾，不扣控制。"""
from pathlib import Path
import json,math,statistics,re,subprocess
here=Path(__file__).resolve().parent;plan=json.loads((here/'plan.json').read_text());commands=json.loads((here/'commands.json').read_text());assert all(c['exitCode']==0 for c in commands);summary=[]
for mode in plan['modes']:
 for name,op in plan['cases']:
  raw=json.loads((here/f'{mode}-{name}-{op}.json').read_text());rounds=[[] for _ in range(6)];arms={a:[] for a in 'AB'}
  for i,q in enumerate(raw['Quartets']):
   r,j=divmod(i,4);assert (q['Round'],q['Index'])==(r,j);order='ABBA' if (r+j)%2==0 else 'BAAB';assert q['Order']==order and ''.join(b['arm'] for b in q['Blocks'])==order
   ratio=math.exp(sum((1 if b['arm']=='B' else -1)*math.log(b['nanoseconds']/b['calls']) for b in q['Blocks'])/2);assert math.isclose(ratio,q['Ratio'],rel_tol=1e-12);rounds[r].append(math.log(ratio))
   for b in q['Blocks']:assert b['calls']==raw['CallsPerBlock'];arms[b['arm']].append(b)
  assert len(raw['Quartets'])==24
  ratios=[math.exp(sum(x)/4) for x in rounds];rates={a:sum(b['nanoseconds'] for b in blocks)/sum(b['calls'] for b in blocks) for a,blocks in arms.items()};metrics={a:{k:sum(b[k] for b in blocks)/sum(b['calls'] for b in blocks) for k in ['Bytes','Allocs']} for a,blocks in arms.items()}
  summary.append(dict(mode=mode,name=name,operation=op,medianPercent=(statistics.median(ratios)-1)*100,cumulativePercent=(rates['B']/rates['A']-1)*100,roundPercent=[(r-1)*100 for r in ratios],metrics=metrics,minBlockMs=min(b['nanoseconds']/1e6 for blocks in arms.values() for b in blocks)))
modes=plan['modes']
rows=['# 同进程同源clone复核','','负值为B更快；所有GC/时间块保留，不扣self或clone。','','| 用例 | '+' | '.join(modes)+' |','| --- | '+' | '.join(['---:']*len(modes))+' |']
for name,op in plan['cases']:
 items=[next(x for x in summary if (x['mode'],x['name'],x['operation'])==(m,name,op)) for m in modes]
 rows.append('| '+name+'/'+op+' | '+' | '.join(f"{r['medianPercent']:+.2f}% / {r['cumulativePercent']:+.2f}%" for r in items)+' |')
(here/'analysis.json').write_text(json.dumps(summary,indent=2)+'\n');(here/'SUMMARY.md').write_text('\n'.join(rows)+'\n');print('\n'.join(rows))
