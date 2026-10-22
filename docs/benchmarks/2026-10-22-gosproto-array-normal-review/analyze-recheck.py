"""完整保留预先指定三项复测的配对轮次。"""
from pathlib import Path
import re,json,math,statistics
here=Path(__file__).resolve().parent;summary={}
commands=json.loads((here/'recheck-commands.json').read_text());assert len(commands)==24 and all(c['exitCode']==0 for c in commands)
for mode in ['self','compare']:
 for r in range(3):
  order='ABBA' if r%2==0 else 'BAAB';blocks=[]
  for pos,arm in enumerate(order):
   s=(here/f'recheck-{mode}-{r}-{pos}-{arm}.txt').read_text();data={}
   for line in s.splitlines():
    m=re.match(r'(Benchmark\S+?)(?:-\d+)?\s+\d+\s+([\d.]+) ns/op\s+([\d.]+) B/op\s+([\d.]+) allocs/op',line)
    if m:data[m[1]]=dict(ns=float(m[2]),bytes=float(m[3]),allocs=float(m[4]))
   assert len(data)==3;blocks.append((arm,data))
  for name in blocks[0][1]:
   ratio=math.exp(sum((1 if a=='B' else -1)*math.log(d[name]['ns']) for a,d in blocks)/2)
   summary.setdefault(name,{}).setdefault(mode,[]).append(dict(round=r,percent=(ratio-1)*100,blocks=blocks))
rows=['# 短数组延长计时复测','','每项400ms Go benchmark目标，三轮ABBA/BAAB；不替换首次数据、不扣减self。','','| 项目 | self中位 | 候选中位 | 候选三轮 |','| --- | ---: | ---: | --- |']
for name,m in summary.items():rows.append('| '+name+' | '+f"{statistics.median(x['percent'] for x in m['self']):+.2f}% | {statistics.median(x['percent'] for x in m['compare']):+.2f}% | "+' / '.join(f"{x['percent']:+.2f}%" for x in m['compare'])+' |')
(here/'RECHECK.md').write_text('\n'.join(rows)+'\n');(here/'recheck-analysis.json').write_text(json.dumps(summary,indent=2)+'\n');print('\n'.join(rows))
