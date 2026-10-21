"""独立重算全部校准决策，并检查正式块覆盖；不筛选任何计时样本。"""
from pathlib import Path
import json,statistics,hashlib
here=Path(__file__).resolve().parent;plan=json.loads((here/'plan.json').read_text());summary=[];allblocks=[]
for mode in plan['modes']:
 for name,op in plan['cases']:
  raw=json.loads((here/f'{mode}-{name}-{op}.json').read_text());c=raw['Calibration'];assert len(c)>=10
  seeds=c[:7];assert all(x['Phase']=='seed' and x['Iteration']==0 and x['Calls']==256 and x['Nanoseconds']>0 for x in seeds)
  times=[x['Nanoseconds'] for x in seeds];median=sorted(times)[3]
  calls=max(256,min(1<<16,int(60_000_000/max(1,median)*256)));i=7;iterations=[]
  for iteration in range(3):
   pilots=c[i:i+3];i+=3;assert len(pilots)==3
   assert all(x['Phase']=='pilot' and x['Iteration']==iteration and x['Calls']==calls and x['Nanoseconds']>0 for x in pilots)
   pilotMedian=statistics.median(x['Nanoseconds'] for x in pilots);iterations.append(dict(iteration=iteration,calls=calls,medianMs=pilotMedian/1e6))
   stop=pilotMedian>=40_000_000 or iteration==2 or calls==1<<16
   if stop:break
   calls=min(1<<16,max(calls+1,int(calls*60_000_000/max(1,pilotMedian))))
  assert i==len(c) and calls==raw['CallsPerBlock']
  blocks=[b for q in raw['Quartets'] for b in q['Blocks']];assert len(blocks)==96 and all(b['calls']==calls for b in blocks)
  ns=[b['nanoseconds'] for b in blocks];allblocks+=ns
  summary.append(dict(mode=mode,operation=op,calls=calls,seedMs=[x/1e6 for x in times],pilots=iterations,finalPilotMetFloor=pilotMedian>=40_000_000,blockMinMs=min(ns)/1e6,blockMedianMs=statistics.median(ns)/1e6,blockMaxMs=max(ns)/1e6,blocksBelow20ms=sum(x<20_000_000 for x in ns),blocksBelow40ms=sum(x<40_000_000 for x in ns),calibrationGC=sum(x['GC'] for x in c)))
result=dict(processes=len(summary),blocks=len(allblocks),calibrationBatches=sum(7+3*len(x['pilots']) for x in summary),allDecisionsRecomputed=True,allFinalPilotsMetFloor=all(x['finalPilotMetFloor'] for x in summary),blockMinMs=min(allblocks)/1e6,blockMedianMs=statistics.median(allblocks)/1e6,blockMaxMs=max(allblocks)/1e6,blocksBelow20ms=sum(x<20_000_000 for x in allblocks),blocksBelow40ms=sum(x<40_000_000 for x in allblocks),processesDetail=summary)
(here/'calibration-validation.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result,indent=2))
