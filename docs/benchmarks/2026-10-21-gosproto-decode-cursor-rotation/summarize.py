"""保留每种布局、两遍、控制、累计和长尾，不池化或扣控制。"""
import json,hashlib,re
from pathlib import Path
here=Path(__file__).resolve().parent;summary=[];canonical={};counts=dict(processes=0,rounds=0,quartets=0,blocks=0,preparationCommands=0,timingCommands=0)
rows=['# Decode header 游标实际摆放轮换','','负数表示B更快；self与全部逐块数据见子目录。控制不相减。','','| 布局 | 入口 | clone中位/IQR pp | pass1中位/累计 | pass2中位/累计 |','|---|---|---:|---:|---:|']
for order in ['012','120','201']:
 p=here/order;a=json.loads((p/'analysis.json').read_text());summary.extend(dict(order=order,**x) for x in a)
 plan=json.loads((p/'plan.json').read_text());v=json.loads((p/'validation.json').read_text())
 for k in ['processes','rounds','quartets','blocks']:counts[k]+=v[k]
 commands=json.loads((p/'commands.json').read_text());assert all(x['exitCode']==0 for x in commands)
 for x in commands:counts['timingCommands' if 'GSR_OUTPUT' in x['env'] else 'preparationCommands']+=1
 for name,op in plan['cases']:
  values=[next(x for x in a if (x['name'],x['operation'],x['mode'])==(name,op,m)) for m in ['clone','pass1','pass2']]
  rows.append('| '+order+' | '+op+' | '+' | '.join(f"{x['medianPercent']:+.2f}% / "+(f"{x['iqrPP']:.2f}" if i==0 else f"{x['cumulativePercent']:+.2f}%") for i,x in enumerate(values))+' |')
 s=(p/'string_experiment.go.txt').read_text();defs={}
 for name in ['experimentStringBaseline','experimentStringClone','experimentStringCandidate']:
  start=s.index('func '+name+'(');ends=[len(s)]
  for token in ['\nfunc ','var experimentStringSelected']:
   if token in s[start+1:]:ends.append(s.index(token,start+1))
  defs[name]=re.sub(r'\s+','',s[start:min(ends)])
 canonical[order]=defs
assert canonical['012']==canonical['120']==canonical['201']
logs=json.loads((here/'commands.json').read_text());assert all(x['exitCode']==0 for x in logs)
counts.update(definitionsIdenticalAcrossLayouts=True,allLoggedCommandsPassed=True,driverCommands=len(logs),offsets=json.loads((here/'offsets.json').read_text()))
(here/'ALL-LAYOUTS.md').write_text('\n'.join(rows)+'\n');(here/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
counts['artifactSha256']={str(p.relative_to(here)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(here.rglob('*')) if p.is_file() and p.name!='validation.json' and '__pycache__' not in p.parts}
(here/'validation.json').write_text(json.dumps(counts,indent=2)+'\n');print('\n'.join(rows));print({k:v for k,v in counts.items() if k not in ['artifactSha256','offsets']})
