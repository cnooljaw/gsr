"""复算正常入口独立进程的ABBA/BAAB结果，保留控制、GC与累计反向。"""
import argparse,hashlib,json,math,re,shutil,statistics
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();here=Path(__file__).resolve().parent;root=here.parents[2]
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest();plan=json.loads((out/'plan.json').read_text());logs=json.loads((out/'commands.json').read_text());assert all(x['exitCode']==0 for x in logs)
for label,files in plan['sources'].items():
 assert {f.name:sha(f) for f in (out/label).glob('*.go')}==files
 assert {f.name:sha(f) for f in (out/(label+'-harness')).glob('*.go')}==plan['harnesses'][label]
 assert sha(out/(label+'.test'))==plan['binarySha256'][label]
 modules=(out/(label+'-modules.txt')).read_text();assert modules.count('"Path": "github.com/cnooljaw/gosproto"')==1
 assert str(out/label) in modules
for n,h in plan['frozen'].items():assert sha(root/n)==h,n
assert plan['sources']['baseline']==plan['sources']['clone000']
assert len({json.dumps(x,sort_keys=True) for x in plan['harnesses'].values()})==1
summary=[];totalBlocks=0
for mode in plan['modes']:
 for name,op in plan['cases']:
  path=out/(mode+'-'+name+'-'+op+'.json');data=json.loads(path.read_text());assert (data['mode'],data['name'],data['operation'])==(mode,name,op)
  assert len(data['quartets'])==6
  arms={s:[] for s in 'AB'};ratios=[]
  calibration=plan['calibration'][name+'/'+op];hashes=(calibration['sample']['BodyHash'],calibration['sample']['PackedHash'])
  for i,q in enumerate(data['quartets']):
   order='ABBA' if i%2==0 else 'BAAB';assert q['round']==i and q['order']==order and ''.join(b['arm'] for b in q['blocks'])==order
   value=0
   for b in q['blocks']:
    assert b['Calls']==calibration['calls']==data['callsPerBlock'] and b['Calls']>0 and b['Nanoseconds']>0
    assert (b['BodyHash'],b['PackedHash'])==hashes
    expected='baseline' if b['arm']=='A' else {'self':'baseline','clone':'clone000','pass1':'candidate','pass2':'candidate'}[mode]
    assert b['label']==expected
    value+=(1 if b['arm']=='B' else -1)*math.log(b['Nanoseconds']/b['Calls']);arms[b['arm']].append(b);totalBlocks+=1
   ratio=math.exp(value/2);assert math.isclose(ratio,q['ratio'],rel_tol=1e-12);ratios.append(ratio)
  telemetry={}
  for arm,blocks in arms.items():
   calls=sum(b['Calls'] for b in blocks);telemetry[arm]=dict(calls=calls,nsPerOp=sum(b['Nanoseconds'] for b in blocks)/calls,bytesPerOp=sum(b['Bytes'] for b in blocks)/calls,allocsPerOp=sum(b['Allocs'] for b in blocks)/calls,medianBytesPerOp=statistics.median(b['Bytes']/b['Calls'] for b in blocks),medianAllocsPerOp=statistics.median(b['Allocs']/b['Calls'] for b in blocks),gcCycles=sum(b['GC'] for b in blocks),blocksWithGC=sum(b['GC']>0 for b in blocks),pauseNs=sum(b['PauseNs'] for b in blocks),minBlockNs=min(b['Nanoseconds'] for b in blocks),maxBlockNs=max(b['Nanoseconds'] for b in blocks))
  quartiles=statistics.quantiles(ratios,n=4,method='inclusive');summary.append(dict(mode=mode,name=name,operation=op,medianPercent=(statistics.median(ratios)-1)*100,iqrPP=(quartiles[2]-quartiles[0])*100,cumulativePercent=(telemetry['B']['nsPerOp']/telemetry['A']['nsPerOp']-1)*100,roundPercent=[(r-1)*100 for r in ratios],telemetry=telemetry))
  shutil.copy2(path,here/path.name)
for name in ['commands.json','plan.json',*[label+suffix for label in plan['versions'] for suffix in ['-modules.txt','-buildinfo.txt']]]:shutil.copy2(out/name,here/name)
(here/'timed-worker.go.txt').write_bytes((out/'baseline-harness/normal_worker_test.go').read_bytes())
(here/'analysis.json').write_text(json.dumps(summary,indent=2)+'\n')
rows=['# 正常公开入口：全部比较','','负值表示B耗时更低；每个时间块一个正常入口进程，控制不相减，GC/长尾不删除。','','| 消息/操作 | self 中位/IQR pp | clone 中位/IQR pp | pass1 中位/累计 | pass2 中位/累计 |','| --- | ---: | ---: | ---: | ---: |']
for name,op in plan['cases']:
 rs=[next(x for x in summary if (x['mode'],x['name'],x['operation'])==(mode,name,op)) for mode in plan['modes']]
 rows.append('| '+name+'/'+op+' | '+' | '.join(f"{s['medianPercent']:+.2f}% / "+(f"{s['iqrPP']:.2f}" if i<2 else f"{s['cumulativePercent']:+.2f}%") for i,s in enumerate(rs))+' |')
(here/'ALL-CASES.md').write_text('\n'.join(rows)+'\n')
validation=dict(measurementProcesses=totalBlocks,calibrationAndPreflightProcesses=len(plan['cases'])*3,quartets=len(summary)*6,commands=len(logs),allCommandsPassed=True,normalLibraryHasNoSelector=True,sourceFixtureHarnessAndBinaryHashesVerified=True,allWireHashesMatch=True,baselineCloneSourceEqual=True,productionSourceModified=False,artifactSha256={p.name:sha(p) for p in sorted(here.iterdir()) if p.is_file() and p.name!='validation.json'})
(here/'validation.json').write_text(json.dumps(validation,indent=2)+'\n');print('\n'.join(rows))
