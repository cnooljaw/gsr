"""汇总摆放轮换，逐布局保留控制、双遍、累计及分配，不校正收益。"""
from pathlib import Path
import json,hashlib,re
here=Path(__file__).resolve().parent;summary=[];canonical={};counts={'processes':0,'rounds':0,'quartets':0,'blocks':0,'preparationCommands':0}
rows=['# 实际摆放轮换全部结果','','负值为 B 耗时下降；不减控制，不剔除 GC/长尾，不把布局混成一个校正值。','','| 布局 | 消息/操作 | clone 中位/IQR pp | pass1 中位/累计 | pass2 中位/累计 |','|---|---|---:|---:|---:|']
for order in ['012','120','201']:
 data=json.loads((here/order/'analysis.json').read_text());summary.extend(dict(order=order,**x) for x in data)
 p=json.loads((here/order/'plan.json').read_text());commands=json.loads((here/order/'commands.json').read_text());assert all(x['exitCode']==0 for x in commands);counts['preparationCommands']+=len(commands)
 v=json.loads((here/order/'validation.json').read_text())
 for k in ['processes','rounds','quartets','blocks']:counts[k]+=v[k]
 for name,op in p['cases']:
  a=[next(x for x in data if (x['name'],x['operation'],x['mode'])==(name,op,m)) for m in ['clone','pass1','pass2']]
  rows.append('| '+order+' | '+name+'/'+op+' | '+' | '.join(f"{x['medianPercent']:+.2f}% / "+(f"{x['iqrPP']:.2f}" if i==0 else f"{x['cumulativePercent']:+.2f}%") for i,x in enumerate(a))+' |')
 # 三布局定义内容必须一致，只有定义顺序不同。
 s=(here/order/'gather_experiment.go.txt').read_text();defs={}
 for name in ['packInto','packGatherClone','packGatherCandidate']:
  start=s.index('func '+name+'(');end=s.index('\nfunc ',start+1) if '\nfunc ' in s[start+1:] else len(s)
  defs[name]=re.sub(r'\s+','',s[start:end])
 canonical[order]=defs
assert canonical['012']==canonical['120']==canonical['201']
logs=json.loads((here/'commands.json').read_text());assert all(x['exitCode']==0 for x in logs)
counts.update(driverCommands=len(logs),allLoggedCommandsPassed=True,definitionsIdenticalAcrossLayouts=True,offsets=json.loads((here/'offsets.json').read_text()))
(here/'ALL-LAYOUTS.md').write_text('\n'.join(rows)+'\n');(here/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
counts['artifactSha256']={str(p.relative_to(here)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(here.rglob('*')) if p.is_file() and p.name!='validation.json' and '__pycache__' not in p.parts}
(here/'validation.json').write_text(json.dumps(counts,indent=2)+'\n');print('\n'.join(rows));print(json.dumps({k:v for k,v in counts.items() if k!='artifactSha256'},indent=2))
