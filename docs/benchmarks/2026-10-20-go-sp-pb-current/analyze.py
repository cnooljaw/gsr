"""独立复算Go的72项五轮结果，不删离群值；按同入口对比。"""
from pathlib import Path
import json,re,statistics,collections
here=Path(__file__).parent
all_results={}
for suite,count in [('representative',40),('protocol',32)]:
 grouped=collections.defaultdict(list)
 for line in (here/(suite+'.txt')).read_text().splitlines():
  m=re.match(r'^Benchmark(?:Representative|Protocol)Codec/(\S+)\s+\d+\s+([\d.]+) ns/op(?:\s+[\d.]+ MB/s)?\s+(\d+) B/op\s+(\d+) allocs/op$',line)
  if m:grouped[m[1]].append({'ns':float(m[2]),'bytes':int(m[3]),'allocs':int(m[4])})
 assert len(grouped)==count,(suite,len(grouped))
 for name,rows in grouped.items():
  assert len(rows)==5,(name,len(rows))
  ns=[x['ns'] for x in rows]
  all_results[suite+'/'+name]={'ns':statistics.median(ns),'bytes':statistics.median(x['bytes'] for x in rows),'allocs':statistics.median(x['allocs'] for x in rows),'minNs':min(ns),'maxNs':max(ns),'maxMinRatio':max(ns)/min(ns),'samples':rows}
(here/'analysis.json').write_text(json.dumps(all_results,indent=2)+'\n')
print(len(all_results),'cases',sum(len(x['samples']) for x in all_results.values()),'samples')
