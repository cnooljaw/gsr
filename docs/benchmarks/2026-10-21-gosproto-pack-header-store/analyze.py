"""从全部 ABBA/BAAB 原始块重算；不删 GC/长尾，不相减控制。"""
from pathlib import Path
import json,math,statistics,hashlib,re
here=Path(__file__).resolve().parent
plan=json.loads((here/'plan.json').read_text());commands=json.loads((here/'commands.json').read_text());assert all(c['exitCode']==0 for c in commands)
summary=[];counts={'processes':0,'rounds':0,'quartets':0,'blocks':0}
for mode in plan['modes']:
 for name,op in plan['cases']:
  raw=json.loads((here/f'{mode}-{name}-{op}.json').read_text());assert (raw['Name'],raw['Operation'],raw['Mode'])==(name,op,mode)
  assert len(raw['Quartets'])==24
  rounds=[[] for _ in range(6)];arms={a:[] for a in 'AB'}
  for i,q in enumerate(raw['Quartets']):
   r,j=divmod(i,4);order='ABBA' if (r+j)%2==0 else 'BAAB';assert (q['Round'],q['Index'],q['Order'])==(r,j,order)
   assert ''.join(b['arm'] for b in q['Blocks'])==order
   value=0
   for b in q['Blocks']:
    assert b['calls']==raw['CallsPerBlock'] and b['nanoseconds']>0
    value+=(1 if b['arm']=='B' else -1)*math.log(b['nanoseconds']/b['calls']);arms[b['arm']].append(b)
   ratio=math.exp(value/2);assert math.isclose(q['Ratio'],ratio,rel_tol=1e-12);rounds[r].append(math.log(ratio))
  ratios=[math.exp(sum(x)/len(x)) for x in rounds];quantiles=statistics.quantiles(ratios,n=4,method='inclusive');telemetry={}
  for arm,blocks in arms.items():
   calls=sum(b['calls'] for b in blocks)
   telemetry[arm]={'nsPerOp':sum(b['nanoseconds'] for b in blocks)/calls,'bytesPerOp':sum(b['Bytes'] for b in blocks)/calls,'allocsPerOp':sum(b['Allocs'] for b in blocks)/calls,'gc':sum(b['GC'] for b in blocks),'minBlockNs':min(b['nanoseconds'] for b in blocks),'maxBlockNs':max(b['nanoseconds'] for b in blocks)}
  summary.append(dict(name=name,operation=op,mode=mode,medianPercent=(statistics.median(ratios)-1)*100,iqrPP=(quantiles[2]-quantiles[0])*100,cumulativePercent=(telemetry['B']['nsPerOp']/telemetry['A']['nsPerOp']-1)*100,roundPercent=[(x-1)*100 for x in ratios],telemetry=telemetry))
  counts['processes']+=1;counts['rounds']+=6;counts['quartets']+=24;counts['blocks']+=96
rows=['# 同进程全部结果','','负值表示候选耗时较低；控制不用于扣减候选。','','| 消息/操作 | self 中位/IQR pp | clone 中位/IQR pp | pass1 中位/累计 | pass2 中位/累计 |','|---|---:|---:|---:|---:|']
for name,op in plan['cases']:
 values=[next(x for x in summary if (x['mode'],x['name'],x['operation'])==(mode,name,op)) for mode in plan['modes']]
 rows.append('| '+name+'/'+op+' | '+' | '.join(f"{x['medianPercent']:+.2f}% / "+(f"{x['iqrPP']:.2f}" if i<2 else f"{x['cumulativePercent']:+.2f}%") for i,x in enumerate(values))+' |')
(here/'ALL-CASES.md').write_text('\n'.join(rows)+'\n');(here/'analysis.json').write_text(json.dumps(summary,indent=2)+'\n')
# 除命名外，clone 与基线相同；candidate 的逆变换必须还原 clone。
s=(here/'header_experiment.go.txt').read_text();a=s[s.index('func packHeaderClone('):s.index('func packHeaderCandidate(')];b=s[s.index('func packHeaderCandidate('):s.index('func ExperimentPackBaseline(')]
b=b.replace('packHeaderCandidate','packHeaderClone').replace('\t\t\tdst[0] = header\n\t\t\tpayload := dst[1:9]','\t\t\tpayload := dst[1:9]').replace('\t\t\toffset += 1 + int(ranks>>56)','\t\t\tdst[0] = header\n\t\t\toffset += 1 + int(ranks>>56)')
assert re.sub(r'\s+','',a)==re.sub(r'\s+','',b)
counts['candidateInverseEqualsClone']=True;counts['commands']=len(commands);counts['allLoggedCommandsPassed']=True
counts['artifactSha256']={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(here.iterdir()) if p.is_file() and p.name!='validation.json'}
(here/'validation.json').write_text(json.dumps(counts,indent=2)+'\n');print('\n'.join(rows))
