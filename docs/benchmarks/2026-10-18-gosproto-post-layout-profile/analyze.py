"""独立复算同函数上下文诊断与输入形态，不删块、不修正历史比较。"""
import argparse,json,math,statistics
from pathlib import Path
p=argparse.ArgumentParser(description=__doc__);p.add_argument('directory',type=Path);a=p.parse_args();root=a.directory.resolve();rows=[];blocks_total=0
for filename in sorted(root.glob('body-*.json')):
    raw=json.loads(filename.read_text());qs=raw['Quartets'];assert len(qs)==24
    rounds=[];blocks=[]
    for start in range(0,24,4):
        logs=[]
        for q in qs[start:start+4]:
            assert (q['round'],q['index'])==(start//4,len(logs))
            order='ABBA' if (q['round']+q['index'])%2==0 else 'BAAB'
            assert ''.join(b['arm'] for b in q['blocks'])==order
            ratio=math.exp(sum((1 if b['arm']=='B' else -1)*math.log(b['nanoseconds']/b['calls']) for b in q['blocks'])/2)
            assert math.isclose(ratio,q['rightOverLeft'],rel_tol=1e-12)
            logs.append(math.log(ratio));blocks.extend(q['blocks'])
        rounds.append(math.exp(statistics.mean(logs)))
    assert len(blocks)==96
    for b in blocks:
        assert b['calls']==16384 and b['nanoseconds']>0
        assert b['After']['GC']>=b['Before']['GC']
        assert b['After']['AllocBytes']>=b['Before']['AllocBytes']
    ns=[b['nanoseconds']/b['calls'] for b in blocks];gc=[b['After']['GC']-b['Before']['GC'] for b in blocks]
    q=statistics.quantiles(rounds,n=4,method='inclusive');median=statistics.median(rounds);iqr=q[2]-q[0];within=sum(abs(r-1)<=.05 for r in rounds)
    total_ns=sum(b['nanoseconds'] for b in blocks);cpu=sum(b['After']['GCTotalCPU']-b['Before']['GCTotalCPU'] for b in blocks)
    rows.append({'case':raw['Case'],'context':raw['Context'],'function':raw['Function'],'initial':raw['Initial'],'afterWarmup':raw['AfterWarmup'],'blocks':len(blocks),'calls':96*16384,'roundRatios':rounds,'medianRatio':median,'ratioIQR':iqr,'roundsWithin5Pct':within,'diagnosticTimingAccepted':abs(median-1)<=.02 and iqr<=.03 and within>=5,'meanNsPerCall':statistics.mean(ns),'blockNsP10':statistics.quantiles(ns,n=10,method='inclusive')[0],'blockNsP90':statistics.quantiles(ns,n=10,method='inclusive')[8],'heapPeakBytes':max(b['After']['HeapBytes'] for b in blocks),'goalPeakBytes':max(b['After']['HeapGoal'] for b in blocks),'zeroGCBlocks':gc.count(0),'observedGCCycles':sum(gc),'cyclesPerBlockMin':min(gc),'cyclesPerBlockMax':max(gc),'estimatedGCCPUOverTimedWall':cpu/(total_ns*1e-9),'nsGCCountCorrelation':statistics.correlation(ns,gc) if statistics.pstdev(gc) else None,'allocationBytesPerCall':sum(b['After']['AllocBytes']-b['Before']['AllocBytes'] for b in blocks)/(96*16384)})
    blocks_total+=len(blocks)
assert len(rows)==4 and blocks_total==384
assert len({r['function'] for r in rows if 'sproto_pack' in r['case']})==1
result={'diagnosticOnly':True,'blocks':blocks_total,'quartets':96,'pairedRounds':24,'timedCalls':384*16384,'results':rows,'warning':'Counters surround timed loop,CPU metrics update coarsely;contexts are different live-heap workloads.No causal explanation for every historical anomaly or speedup/control replacement.'}
(root/'context-summary.json').write_text(json.dumps(result,indent=2)+'\n')
patterns=json.loads((root/'patterns.json').read_text());assert len(patterns)==24
summary=[]
for sample in patterns:
    assert sum(sample['Masks'].values())==sum(sample['Nonzero'])==sample['Blocks']==(sample['Bytes']+7)//8
    dense=contiguous=0
    for mask_string,count in sample['Masks'].items():
        mask=int(mask_string,16);n=mask.bit_count();assert sample['Nonzero'][n]>=count
        if 3<=n<=7:
            dense+=count
            if ((mask+(mask&-mask))&mask)==0:contiguous+=count
    summary.append({'name':sample['Name'],'blocks':sample['Blocks'],'ordinaryDenseRawWords':dense,'contiguousDenseRawWords':contiguous,'contiguousFractionOfDense':contiguous/dense if dense else None,'topMasks':sorted(sample['Masks'].items(),key=lambda v:(-v[1],v[0]))[:8]})
(root/'pattern-summary.json').write_text(json.dumps(summary,indent=2)+'\n')
print(json.dumps(result,indent=2));print('24 morphology count sums independently verified')
