"""独立复算全部交错轮次，保留控制、分配尾差和中位/累计反向。"""
import argparse,hashlib,json,math,re,statistics,shutil,subprocess
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();here=Path(__file__).resolve().parent;root=here.parents[2];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
plan=json.loads((out/'plan.json').read_text());raw=json.loads((out/'results.json').read_text());commands=json.loads((out/'commands.json').read_text());assert len(raw)==40
fixtureAfter={}
for n in ['representative.json','cases.json']:
 f=root/'examples/websocket-gate/benchmarks'/n
 assert f.read_bytes()==(out/'harness'/n).read_bytes()==subprocess.check_output(['git','show','HEAD:examples/websocket-gate/benchmarks/'+n],cwd=root)
 fixtureAfter[n]=sha(f)
 assert fixtureAfter[n]==plan['fixtureSha256'][n]
for n in ['go.mod','go.sum']:assert (root/n).read_bytes()==subprocess.check_output(['git','show','HEAD:'+n],cwd=root)
for n,h in plan['sources'].items():assert sha(out/n)==h,n
for n,h in plan['generatedInputs'].items():assert sha(root/n)==h,n
for n,h in plan['binary'].items():assert sha(out/(n+'.test'))==h,n
assert all(c['exitCode']==0 for c in commands)
for side in ['A','B','C']:assert (out/(side+'-modules.txt')).read_text().count('"Path": "github.com/cnooljaw/gosproto"')==1
summary=[]
for r in raw:
 ratios=[];arms={a:[] for a in 'AB'}
 for rd in r['rounds']:
  assert len(rd['samples'])==4 and ''.join(s['arm'] for s in rd['samples'])==rd['order']
  ratio=math.exp(sum((1 if s['arm']=='B' else -1)*math.log(s['ns']) for s in rd['samples'])/2);assert abs(ratio-rd['ratio'])<1e-12;ratios.append(ratio)
  for s in rd['samples']:arms[s['arm']].append(s)
 median=statistics.median(ratios);assert abs(median-r['medianRatio'])<1e-12
 # 固定时长迭代次数不同，累计每次耗时按实际调用数加权；与轮次几何比各自保留。
 weighted={a:sum(s['ns']*s['calls'] for s in samples)/sum(s['calls'] for s in samples) for a,samples in arms.items()};cum=weighted['B']/weighted['A']
 q=statistics.quantiles(ratios,n=4,method='inclusive');assert abs((q[2]-q[0])-r['ratioIQR'])<1e-12
 row=dict(mode=r['mode'],case=r['case'],op=r['op'],medianPercent=(median-1)*100,iqrPP=(q[2]-q[0])*100,cumulativePercent=(cum-1)*100,roundPercent=[(x-1)*100 for x in ratios],allocation={a:{'bytes':sorted(set(s['bytes'] for s in samples)),'allocs':sorted(set(s['allocs'] for s in samples))} for a,samples in arms.items()});summary.append(row)
(here/'analysis.json').write_text(json.dumps(summary,indent=2)+'\n')
for p in out.iterdir():
 if p.is_file() and p.suffix in ['.txt','.json','.patch']:shutil.copy2(p,here/p.name)
assembly={}
for side in ['A','B','C']:
 s=(out/(side+'-assembly.txt')).read_text();lines=[l for l in s.splitlines() if re.search(r'\s0x[0-9a-f]+\s+[0-9a-f]+\s',l)];assembly[side]={'instructions':len(lines),'panicIndex':s.count('runtime.panicIndex'),'panicSlice':s.count('runtime.panicSlice'),'entry':re.search(r'0x[0-9a-f]+',lines[0]).group()}
(here/'assembly-summary.json').write_text(json.dumps(assembly,indent=2)+'\n')
rows=['# 全部控制与比较','', '负值为候选耗时更低；控制不相减。累计按真实调用数加权，全部轮次保留于 analysis.json。','', '| 消息/入口 | self 中位/IQR pp | clone 中位/IQR pp | pass1 中位/累计 | pass2 中位/累计 |','| --- | ---: | ---: | ---: | ---: |']
for case,op in plan['cases']:
 ds=[next(r for r in summary if (r['mode'],r['case'],r['op'])==(mode,case,op)) for mode in plan['modes']];rows.append('| '+case+'/'+op+' | '+' | '.join(f"{r['medianPercent']:+.2f}% / "+(f"{r['iqrPP']:.2f}" if i<2 else f"{r['cumulativePercent']:+.2f}%") for i,r in enumerate(ds))+' |')
(here/'ALL-CASES.md').write_text('\n'.join(rows)+'\n')
validation={'processes':len(raw)*6*4,'rounds':len(raw)*6,'commands':len(commands),'allCommandsPassed':True,'sourceInputAndBinaryHashesVerified':True,'singleSprotoModulePerBinary':True,'fixtureAfterSamplingSha256':fixtureAfter,'regression':json.loads((out/'regression-v2.json').read_text()) if (out/'regression-v2.json').exists() else None,'artifactSha256':{p.name:sha(p) for p in sorted(here.iterdir()) if p.is_file() and p.name!='validation.json'}}
(here/'validation.json').write_text(json.dumps(validation,indent=2)+'\n');print('\n'.join(rows))
