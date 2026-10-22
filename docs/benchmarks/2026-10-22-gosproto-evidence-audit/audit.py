"""只审计已有原始测量；不重测、不删块、不校正控制、不设收益门槛。"""
from pathlib import Path
import json,math,statistics,hashlib
here=Path(__file__).resolve().parent;bench=here.parent
names=['2026-10-21-gosproto-record-width-flow','2026-10-21-gosproto-decode-lookup-range','2026-10-21-gosproto-decode-leaf-depth','2026-10-21-gosproto-layout-final-append','2026-10-21-gosproto-record-local-budget','2026-10-21-gosproto-record-header-output-cursor']
rows=[];hashes={};blocksTotal=0
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
for name in names:
 folder=bench/name;plan=json.loads((folder/'plan.json').read_text());assert plan['commit'].startswith('9048cfa'),name
 sourceAnalysis=json.loads((folder/'analysis.json').read_text());validation=json.loads((folder/'validation.json').read_text())
 ops=[op for case,op in plan['cases'] if case=='game_large'];assert len(ops)==2
 hashes[str((folder/'plan.json').relative_to(bench))]=sha(folder/'plan.json')
 for op in ops:
  for mode in ['self','clone','pass1','pass2']:
   p=folder/f'{mode}-game_large-{op}.json';h=sha(p);assert validation['artifactSha256'][p.name]==h,p
   hashes[str(p.relative_to(bench))]=h
   raw=json.loads(p.read_text());assert (raw['Name'],raw['Operation'],raw['Mode'])==('game_large',op,mode)
   assert len(raw['Quartets'])==24
   roundLogs=[[] for _ in range(6)];arms={a:[] for a in 'AB'}
   for i,q in enumerate(raw['Quartets']):
    r,j=divmod(i,4);order='ABBA' if (r+j)%2==0 else 'BAAB';assert (q['Round'],q['Index'],q['Order'])==(r,j,order)
    assert ''.join(b['arm'] for b in q['Blocks'])==order
    value=0
    for b in q['Blocks']:
     assert b['calls']==raw['CallsPerBlock'] and b['nanoseconds']>0
     value+=(1 if b['arm']=='B' else -1)*math.log(b['nanoseconds']/b['calls']);arms[b['arm']].append(b);blocksTotal+=1
    ratio=math.exp(value/2);assert math.isclose(ratio,q['Ratio'],rel_tol=1e-12);roundLogs[r].append(math.log(ratio))
   ratios=[math.exp(sum(x)/len(x)) for x in roundLogs];median=(statistics.median(ratios)-1)*100
   cumulative=(sum(b['nanoseconds'] for b in arms['B'])/sum(b['calls'] for b in arms['B']))/(sum(b['nanoseconds'] for b in arms['A'])/sum(b['calls'] for b in arms['A']));cumulative=(cumulative-1)*100
   old=next(x for x in sourceAnalysis if (x['name'],x['operation'],x['mode'])==('game_large',op,mode));assert math.isclose(median,old['medianPercent'],abs_tol=1e-10) and math.isclose(cumulative,old['cumulativePercent'],abs_tol=1e-10)
   leaveOne=[(statistics.median(ratios[:i]+ratios[i+1:])-1)*100 for i in range(6)]
   telemetry={}
   for arm,data in arms.items():
    ns=[b['nanoseconds'] for b in data];telemetry[arm]=dict(maxBlockMs=max(ns)/1e6,maxToMedian=max(ns)/statistics.median(ns),slowestBlockSharePercent=max(ns)/sum(ns)*100,blocksWithGC=sum(b['GC']>0 for b in data),gcCycles=sum(b['GC'] for b in data))
   rows.append(dict(experiment=name,operation=op,mode=mode,targetBlockMs=plan['targetBlockMs'],medianPercent=median,iqrPP=old['iqrPP'],cumulativePercent=cumulative,roundPercent=[(x-1)*100 for x in ratios],fasterRounds=sum(x<1 for x in ratios),medianCumulativeSameDirection=median*cumulative>0,leaveOneRoundMedianMin=min(leaveOne),leaveOneRoundMedianMax=max(leaveOne),leaveOneRoundSignStable=min(leaveOne)*max(leaveOne)>0,telemetry=telemetry))
(here/'analysis.json').write_text(json.dumps(rows,indent=2)+'\n');(here/'source-hashes.json').write_text(json.dumps(hashes,indent=2)+'\n')
lines=['# 大型消息已有数据审计','','所有原值保留。留一轮敏感性只作诊断，不替换正式中位/累计，不相减控制。','','| 候选/入口 | clone中位/IQR pp | pass1中位/累计 | pass2中位/累计 | 两遍更快轮数 | 留一轮中位范围 pass1 / pass2 |','|---|---:|---:|---:|---:|---|']
for name in names:
 for op in [x['operation'] for x in rows if x['experiment']==name and x['mode']=='pass1']:
  select=lambda m:next(x for x in rows if (x['experiment'],x['operation'],x['mode'])==(name,op,m))
  c,a,b=map(select,['clone','pass1','pass2'])
  lines.append('| '+name.removeprefix('2026-10-21-gosproto-')+'/'+op+f" | {c['medianPercent']:+.2f}% / {c['iqrPP']:.2f} | {a['medianPercent']:+.2f}% / {a['cumulativePercent']:+.2f}% | {b['medianPercent']:+.2f}% / {b['cumulativePercent']:+.2f}% | {a['fasterRounds']}/6, {b['fasterRounds']}/6 | [{a['leaveOneRoundMedianMin']:+.2f},{a['leaveOneRoundMedianMax']:+.2f}] / [{b['leaveOneRoundMedianMin']:+.2f},{b['leaveOneRoundMedianMax']:+.2f}] |")
(here/'ALL-CASES.md').write_text('\n'.join(lines)+'\n')
candidates=[x for x in rows if x['mode'].startswith('pass')]
counts=dict(experiments=len(names),processes=len(rows),blocks=blocksTotal,allOriginalHashesVerified=True,allRawRatiosAndPublishedSummariesRecomputed=True,candidateProcesses=len(candidates),candidateMedianCumulativeOpposite=sum(not x['medianCumulativeSameDirection'] for x in candidates),candidateLeaveOneRoundSignUnstable=sum(not x['leaveOneRoundSignStable'] for x in candidates),candidateMedianFaster=sum(x['medianPercent']<0 for x in candidates),candidateCumulativeFaster=sum(x['cumulativePercent']<0 for x in candidates))
(here/'validation.json').write_text(json.dumps(counts,indent=2)+'\n');print('\n'.join(lines));print(counts)
