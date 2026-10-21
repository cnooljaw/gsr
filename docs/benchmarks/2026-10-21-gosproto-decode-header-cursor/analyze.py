"""逐块独立复算同进程结果，保留GC与长尾，不以控制差值修正候选。"""
import argparse,hashlib,json,math,re,shutil,statistics,subprocess
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();here=Path(__file__).resolve().parent;root=here.parents[2];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
plan=json.loads((out/'plan.json').read_text());commands=json.loads((out/'commands.json').read_text());assert all(c['exitCode']==0 for c in commands)
for n,h in plan['sources'].items():assert sha(out/n)==h,n
for n,h in plan['fixtures'].items():assert sha(out/'harness'/n)==h,n;assert sha(root/'examples/websocket-gate/benchmarks'/n)==h,n
for n,h in plan['generated'].items():assert sha(root/n)==h,n
assert sha(out/'experiment.test')==plan['binarySha256']
assert (out/'modules.txt').read_text().count('"Path": "github.com/cnooljaw/gosproto"')==1
original=(out/'original-decode.go.txt').read_text();original=original[original.index('func decodeIntegerRecord('):]
adapters=(out/'gosproto/string_experiment.go').read_text();start=adapters.index('func experimentStringClone(');end=adapters.index('func experimentStringCandidate(')
clone=adapters[start:end].replace('experimentStringClone','decodeIntegerRecord');assert re.sub(r'\s+','',original)==re.sub(r'\s+','',clone)
candidate=adapters[end:adapters.index('var experimentStringSelected',end)].replace('experimentStringCandidate','decodeIntegerRecord')
candidate=candidate.replace('\theaders := chunk[2:total:total]\n','').replace('\tfor len(headers) >= 2 {\n\t\tentry := int(readUint16(headers))\n\t\theaders = headers[2:]','\tfor index := 0; index < count; index++ {\n\t\tentry := int(readUint16(chunk[2+index*2 : 4+index*2 : 4+index*2]))')
assert re.sub(r'\s+','',original)==re.sub(r'\s+','',candidate)
summary=[];counts=dict(processes=0,blocks=0,quartets=0,rounds=0)
for mode in plan['modes']:
 for name,op in plan['cases']:
  f=out/(mode+'-'+name+'-'+op+'.json');raw=json.loads(f.read_text());assert (raw['Mode'],raw['Name'],raw['Operation'])==(mode,name,op);assert len(raw['Quartets'])==24
  rounds=[[] for _ in range(6)];arms={a:[] for a in 'AB'}
  for i,q in enumerate(raw['Quartets']):
   r,j=divmod(i,4);assert (q['Round'],q['Index'])==(r,j)
   order='ABBA' if (r+j)%2==0 else 'BAAB';assert q['Order']==order and ''.join(b['arm'] for b in q['Blocks'])==order
   value=0
   for b in q['Blocks']:
    assert b['calls']==raw['CallsPerBlock'] and b['calls']>0 and b['nanoseconds']>0
    value+=(1 if b['arm']=='B' else -1)*math.log(b['nanoseconds']/b['calls']);arms[b['arm']].append(b);counts['blocks']+=1
   ratio=math.exp(value/2);assert math.isclose(ratio,q['Ratio'],rel_tol=1e-12);rounds[r].append(math.log(ratio));counts['quartets']+=1
  ratios=[math.exp(sum(x)/4) for x in rounds];median=statistics.median(ratios);q=statistics.quantiles(ratios,n=4,method='inclusive')
  telemetry={}
  for arm,blocks in arms.items():
   calls=sum(b['calls'] for b in blocks);ns=sum(b['nanoseconds'] for b in blocks)
   telemetry[arm]=dict(calls=calls,nsPerOp=ns/calls,bytesPerOp=sum(b['Bytes'] for b in blocks)/calls,allocsPerOp=sum(b['Allocs'] for b in blocks)/calls,gcCycles=sum(b['GC'] for b in blocks),blocksWithGC=sum(b['GC']>0 for b in blocks),pauseNs=sum(b['PauseNs'] for b in blocks),minBlockNs=min(b['nanoseconds'] for b in blocks),maxBlockNs=max(b['nanoseconds'] for b in blocks),maxHeapSnapshot=max(max(b['HeapBefore'],b['HeapAfter']) for b in blocks))
  cumulative=telemetry['B']['nsPerOp']/telemetry['A']['nsPerOp']
  summary.append(dict(mode=mode,name=name,operation=op,medianPercent=(median-1)*100,iqrPP=(q[2]-q[0])*100,cumulativePercent=(cumulative-1)*100,roundPercent=[(r-1)*100 for r in ratios],telemetry=telemetry));counts['processes']+=1;counts['rounds']+=6
  shutil.copy2(f,here/f.name)
for n in ['commands.json','plan.json','modules.txt','assembly.txt','original-decode.go.txt']:shutil.copy2(out/n,here/n)
for n in ['string_experiment.go','string_experiment_test.go']:(here/(n+'.txt')).write_bytes((out/'gosproto'/n).read_bytes())
(here/'timed-experiment.go.txt').write_bytes((out/'harness/experiment_test.go').read_bytes())
(here/'analysis.json').write_text(json.dumps(summary,indent=2)+'\n')
rows=['# 全部同进程控制与比较','', '负值表示B耗时更低。控制不相减；全部块、GC、分配与六轮保留。','', '| 消息/操作 | self 中位/IQR pp | clone 中位/IQR pp | pass1 中位/累计 | pass2 中位/累计 |','| --- | ---: | ---: | ---: | ---: |']
for name,op in plan['cases']:
 rs=[next(r for r in summary if (r['mode'],r['name'],r['operation'])==(mode,name,op)) for mode in plan['modes']]
 rows.append('| '+name+'/'+op+' | '+' | '.join(f"{r['medianPercent']:+.2f}% / "+(f"{r['iqrPP']:.2f}" if i<2 else f"{r['cumulativePercent']:+.2f}%") for i,r in enumerate(rs))+' |')
(here/'ALL-CASES.md').write_text('\n'.join(rows)+'\n')
counts['allCommandsPassed']=True;counts['commands']=len(commands);counts['cloneBodyEqualsBaseline']=True;counts['sourceFixtureGeneratedAndBinaryHashesVerified']=True;counts['artifactSha256']={p.name:sha(p) for p in sorted(here.iterdir()) if p.is_file() and p.name!='validation.json'}
(here/'validation.json').write_text(json.dumps(counts,indent=2)+'\n');print('\n'.join(rows))
