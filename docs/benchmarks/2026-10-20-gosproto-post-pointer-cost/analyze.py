"""复算profile loop分配和符号归属；不将带profile的ns/op当成新排名。"""
from pathlib import Path
import argparse,json,re
parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('directory',type=Path);args=parser.parse_args();d=args.directory
m=json.loads((d/'inputs.json').read_text());rows=[];symbols={}
for case in m['cases']:
 for op in m['ops']:
  for kind in m['kinds']:
   stem=case+'-'+op+'-'+kind
   meta=json.loads((d/(stem+'.json')).read_text());text=(d/(stem+'-benchmark.txt')).read_text()
   v=re.search(r'BenchmarkBodyCost\s+(\d+)\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',text)
   assert v and int(v[1])==meta['Calls']
   assert kind!='allocation' or meta['Calls']==5000
   rows.append({'case':case,'op':op,'kind':kind,'calls':meta['Calls'],'profiledNsPerOp':float(v[2]),'loopBytes':int(v[3]),'loopAllocs':int(v[4]),'gcCycles':meta['After']['NumGC']-meta['Before']['NumGC']})
   for suffix in (['flat','cum'] if kind=='cpu' else ['objects','space']):
    table={}
    for line in (d/(stem+'-'+suffix+'.txt')).read_text().splitlines():
     p=re.match(r'^\s+(\S+)\s+([\d.]+)%\s+[\d.]+%\s+(\S+)\s+([\d.]+)%\s+(.+)$',line)
     if p:table[p[5]]={'flat':p[1],'flatPercent':float(p[2]),'cum':p[3],'cumPercent':float(p[4])}
    assert table
    symbols[stem+'/'+suffix]=table
assert len(rows)==18
(d/'analysis.json').write_text(json.dumps({'scope':'18 independent profiles, no paired timings; percentages have each full profile denominator, parent-child not summed, allocation includes setup/warmup','loops':rows,'symbols':symbols},indent=2)+'\n')
for r in rows:
 if r['kind']=='allocation':print(r['case'],r['op'],r['loopBytes'],r['loopAllocs'])
