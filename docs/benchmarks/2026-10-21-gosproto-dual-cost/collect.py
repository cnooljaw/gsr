"""保存独立进程诊断文本并核对源码、输入、二进制和原始 profile 哈希。"""
import argparse,hashlib,json,re,shutil
from pathlib import Path
parser=argparse.ArgumentParser();parser.add_argument('--output',type=Path,required=True);args=parser.parse_args()
here=Path(__file__).resolve().parent;root=here.parents[2];out=args.output.resolve()
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
plan=json.loads((out/'inputs.json').read_text());commands=json.loads((out/'commands.json').read_text())
assert len(plan['profiles'])==36 and all(c['exitCode']==0 for c in commands)
assert sha(out/'body.test')==plan['binarySha256']
for name,h in plan['source'].items():assert sha(out/'gosproto'/name)==h
for name,h in plan['harnessGo'].items():assert sha(out/'harness'/name)==h
for name,h in plan['frozenInputs'].items():assert sha(root/name)==h
for stem,h in plan['profiles'].items():
 assert sha(out/(stem+('.cpu' if stem.endswith('-cpu') else '.alloc')))==h['rawSha256']
 assert sha(out/(stem+'.json'))==h['metadataSha256']
for p in out.iterdir():
 if p.is_file() and p.suffix in ['.json','.txt']:shutil.copy2(p,here/p.name)
summary=[]
for case in plan['cases']:
 for op in plan['ops']:
  stem=case+'-'+op
  txt=(out/(stem+'-cpu-flat.txt')).read_text()
  nodes=[]
  for line in txt.splitlines():
   cols=line.split()
   if len(cols)>=6 and cols[1].endswith('%'):
    nodes.append({'flat':cols[0],'flatPercent':float(cols[1][:-1]),'cum':cols[3],'cumPercent':float(cols[4][:-1]),'function':' '.join(cols[5:])})
  alloc=(out/(stem+'-allocation-benchmark.txt')).read_text();m=re.search(r'BenchmarkBodyCost\s+5000\s+[\d.]+ ns/op\s+(\d+) B/op\s+(\d+) allocs/op',alloc);assert m,stem
  summary.append({'case':case,'op':op,'nodes':nodes,'loopBytes':int(m[1]),'loopAllocs':int(m[2])})
(here/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
refs={}
for rel in ['lualib-src/sproto/sproto.c','lualib-src/sproto/lsproto.c']:
 refs[rel]=sha(root.parent/'skynet'/rel)
(here/'reference.json').write_text(json.dumps({'readOnlyOfficialReference':str(root.parent/'skynet'),'sha256':refs,'lines':{'sproto.c':'1386-1449','lsproto.c':'704-721'}},indent=2)+'\n')
validation={'profiles':36,'commands':len(commands),'allCommandsPassed':True,'sourceFixtureHarnessAndBinaryHashesVerified':True,'rawProfilesRetainedAt':str(out),'noProductionChanges':True,'artifactSha256':{str(p.relative_to(here)):sha(p) for p in sorted(here.iterdir()) if p.is_file() and p.name!='validation.json'}}
(here/'validation.json').write_text(json.dumps(validation,indent=2)+'\n')
print('verified',len(commands),'commands and 36 raw profiles')
for r in summary:
 print(r['case'],r['op'],r['loopBytes'],r['loopAllocs'],[(n['function'],n['flatPercent'],n['cumPercent']) for n in r['nodes'] if 'gosproto.' in n['function'] or 'kevent' in n['function']][:5])
