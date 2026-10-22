"""逐块独立复算同进程结果，保留GC与长尾，不以控制差值修正候选。"""
import argparse,hashlib,json,math,re,shutil,statistics,subprocess
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--output',type=Path,required=True);p.add_argument('--destination',type=Path);a=p.parse_args();out=a.output.resolve();script=Path(__file__).resolve().parent;here=a.destination.resolve() if a.destination else script;here.mkdir(exist_ok=True);root=script.parents[2];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
plan=json.loads((out/'plan.json').read_text());commands=json.loads((out/'commands.json').read_text());assert all(c['exitCode']==0 for c in commands)
for n,h in plan['sources'].items():assert sha(out/n)==h,n
for n,h in plan['fixtures'].items():assert sha(out/'harness'/n)==h,n;assert sha(root/'examples/websocket-gate/benchmarks'/n)==h,n
for n,h in plan['generated'].items():assert sha(root/n)==h,n
assert sha(out/'experiment.test')==plan['binarySha256']
assert (out/'modules.txt').read_text().count('"Path": "github.com/cnooljaw/gosproto"')==1
equivalence=json.loads((out/'equivalence.json').read_text());assert all(x['originalSha256']==x['restoredSha256'] for x in equivalence.values());assert equivalence['Baseline']==equivalence['Clone']
actual=(out/'gosproto/static_experiment.go').read_text();limits=(out/'original-limits.go.txt').read_text();shared=limits[limits.index('func encodeWithLimits('):limits.index('func encodedSize(')];base=(out/'original-encode.go.txt').read_text();baseRecord=(out/'gosproto/encode_integer_record.go').read_text();cand=(out/'candidate-encode.go.txt').read_text();candRecord=(out/'candidate-record.go.txt').read_text()
starts=list(re.finditer(r'func ExperimentStatic(Baseline|Clone|Candidate)\(',actual));post={};strip=lambda s:re.sub(r'\s+','',re.sub(r'//[^\n]*','',s))
for i,m in enumerate(starts):
 label=m[1];end=starts[i+1].start() if i+1<len(starts) else actual.index('var experimentStaticSelected');body=actual[m.start():end]
 for old,new in {'encodedSize':'experiment'+label+'Size','fieldSize':'experiment'+label+'FieldSize','encodedIntegerRecordSize':'experiment'+label+'RecordSize','encodeWithLimits':'ExperimentStatic'+label,'encodeBufferWithLimits':'experiment'+label+'Buffer','encodeMessageInto':'experiment'+label+'Message','encodeFieldInto':'experiment'+label+'Field','encodeIntegerRecordInto':'experiment'+label+'Record','encodeIntegerRecordArrayInto':'experiment'+label+'Array'}.items():body=re.sub(r'\b'+new+r'\b',old,body)
 enc,record=(cand,candRecord) if label=='Candidate' else (base,baseRecord)
 selectedLimits=(out/'candidate-limits.go.txt').read_text() if label=='Candidate' else limits
 expected=shared+enc[enc.index('func encodeMessageInto('):enc.index('// encodeScalarInto')]+selectedLimits[selectedLimits.index('func encodedSize('):selectedLimits.index('func (limits Limits) Validate(')]+record[record.index('func encodedIntegerRecordSize('):]
 assert strip(body)==strip(expected),label
 post[label]=True
(here/'post-gofmt-equivalence.json').write_text(json.dumps(post,indent=2)+'\n')
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
for n in ['commands.json','plan.json','modules.txt','assembly.txt','original-encode.go.txt','original-limits.go.txt','candidate-encode.go.txt','candidate-record.go.txt','candidate-limits.go.txt','equivalence.json']:shutil.copy2(out/n,here/n)
for n in ['static_experiment.go','static_experiment_test.go','external_experiment_test.go']:(here/(n+'.txt')).write_bytes((out/'gosproto'/n).read_bytes())
(here/'timed-experiment.go.txt').write_bytes((out/'harness/experiment_test.go').read_bytes())
(here/'analysis.json').write_text(json.dumps(summary,indent=2)+'\n')
rows=['# 全部同进程控制与比较','', '负值表示B耗时更低。控制不相减；全部块、GC、分配与六轮保留。','', '| 消息/操作 | self 中位/IQR pp | clone 中位/IQR pp | pass1 中位/累计 | pass2 中位/累计 |','| --- | ---: | ---: | ---: | ---: |']
for name,op in plan['cases']:
 rs=[next(r for r in summary if (r['mode'],r['name'],r['operation'])==(mode,name,op)) for mode in plan['modes']]
 rows.append('| '+name+'/'+op+' | '+' | '.join(f"{r['medianPercent']:+.2f}% / "+(f"{r['iqrPP']:.2f}" if i<2 else f"{r['cumulativePercent']:+.2f}%") for i,r in enumerate(rs))+' |')
(here/'ALL-CASES.md').write_text('\n'.join(rows)+'\n')
counts['allCommandsPassed']=True;counts['commands']=len(commands);counts['cloneBodyEqualsBaseline']=True;counts['sourceFixtureGeneratedAndBinaryHashesVerified']=True;counts['artifactSha256']={p.name:sha(p) for p in sorted(here.iterdir()) if p.is_file() and p.name!='validation.json'}
(here/'validation.json').write_text(json.dumps(counts,indent=2)+'\n');print('\n'.join(rows))
