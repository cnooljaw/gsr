"""独立核对32份元数据、命令、源/二进制/profile指纹及fresh归档，并提取热点。"""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--output', type=Path, required=True)
parser.add_argument('--fresh', type=Path, required=True)
parser.add_argument('--library', type=Path, required=True)
args=parser.parse_args()
here=Path(__file__).resolve().parent
root=here.parents[2]
p=args.output.resolve()
fresh=args.fresh.resolve()
plan=json.loads((here/'plan.json').read_text())
manifest=json.loads((p/'inputs.json').read_text())
recreated=json.loads((fresh/'inputs.json').read_text())
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
sources=lambda path:{f.name:sha(f) for f in sorted(path.glob('*.go'))}
for key,folder in [('beforeGoSource','archive/before'),('candidateGoSource','archive/candidate'),('harnessGoSource','archive/harness')]:
    assert sources(p/folder)==manifest[key]==recreated[key],key
assert sources(args.library)==manifest['candidateGoSource']
assert manifest['goVersion']==recreated['goVersion']
assert manifest['environment']==recreated['environment']
assert manifest['generatedGoInputs']==recreated['generatedGoInputs']
assert manifest['fixtures']==recreated['fixtures']
assert manifest['driverInputs']==recreated['driverInputs']
for name,digest in manifest['driverInputs'].items():
    assert sha(root/name)==digest,name
for name,digest in manifest['rawProfileSha256'].items():
    assert sha(p/name)==digest,name
assert len(manifest['rawProfileSha256'])==32
assert sha(p/'current.test')==manifest['binarySha256']
for name,digest in manifest['goModFiles'].items():
    assert sha(p/name)==digest,name
    old=(p/name).read_text().replace(str(p),'<OUTPUT>')
    new=(fresh/name).read_text().replace(str(fresh),'<OUTPUT>')
    assert old==new,name
assert json.loads((p/'case-list.json').read_text())==plan['original70']
commands=json.loads((p/'commands.json').read_text())
assert all(c.get('exitCode')==0 for c in commands)
captures=[c for c in commands if c['args'][0]==str(p/'current.test')]
assert len(captures)==32
measurements=json.loads((p/'measurements.json').read_text())
assert len(measurements)==32
expected=[(case,kind) for case in plan['cases'] for kind in ['cpu','allocation']]
assert [(r['case'],r['kind']) for r in measurements]==expected
snapshots=[]
for row,command in zip(measurements,captures):
    raw=json.loads((p/row['metadata']).read_text())
    assert (raw['Case'],raw['Kind'],raw['Calls'])==(row['case'],row['kind'],row['calls'])
    assert command['env']['GSR_PROFILE_CASE']==row['case']
    assert command['env']['GSR_PROFILE_KIND']==row['kind']
    assert command['env']['GOMAXPROCS']=='1'
    assert [s['Stage'] for s in raw['Stages']]==plan['telemetry']
    rows=re.findall(r'^BenchmarkCurrentOperation\s+(\d+)\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',command['output'],re.M)
    assert len(rows)==1
    assert (int(rows[0][0]),float(rows[0][1]),int(rows[0][2]),int(rows[0][3]))==(row['calls'],row['nsPerOp'],row['bytesPerOp'],row['allocsPerOp'])
    if row['kind']=='allocation':assert row['calls']==20000
    for snapshot in raw['Stages']:
        assert snapshot['GOGC']==100 and snapshot['MemoryLimit']==2**63-1
        snapshots.append(snapshot)
    assert all(raw['Stages'][i]['GC']<=raw['Stages'][i+1]['GC'] for i in range(2))

# pprof已渲染表中的百分数原样保留；未进入top40用null，不解释成零。
def table(path):
    result={}
    for line in path.read_text().splitlines():
        match=re.match(r'\s*(\S+)\s+(\S+)%\s+(\S+)%\s+(\S+)\s+(\S+)%\s+(.*)',line)
        if match:result[match[6]]={'flatPercent':float(match[2]),'cumulativePercent':float(match[5])}
    assert result,path
    return result

hotspots=[]
for row in measurements[::2]:
    name=row['case'].replace('/','-')
    cpu=table(p/(name+'-cpu-cum.txt'))
    flat=table(p/(name+'-cpu-flat.txt'))
    functions=['packInto','encodedSize','encodeMessageInto','encodedIntegerRecordSize','encodeIntegerRecordInto','encodeInt64Slice (inline)']
    selected={f:cpu.get('github.com/cnooljaw/gosproto.'+f) for f in functions}
    selected['runtime.kevent']=flat.get('runtime.kevent')
    selected['runtime.mallocgc']=cpu.get('runtime.mallocgc')
    hotspots.append({'case':row['case'],'cpuBenchmark':row,'allocationBenchmark':measurements[measurements.index(row)+1],'functions':selected})

# 只从已采profile补渲染两个整数记录源码视图，不新增计时或采样。
supplement=[]
for case in ['body-game_large-sproto_pencode','rpc-game_large-request_cancel']:
    command=['go','tool','pprof','-list=github.com/cnooljaw/gosproto\\.(encodedIntegerRecordSize|encodeIntegerRecordInto)',str(p/'current.test'),str(p/(case+'.cpu'))]
    result=subprocess.run(command,capture_output=True,text=True,check=True)
    path=p/(case+'-record-source.txt')
    path.write_text(result.stdout+result.stderr)
    supplement.append({'args':command,'exitCode':result.returncode,'outputFile':path.name,'sha256':sha(path)})
(p/'analysis-commands.json').write_text(json.dumps(supplement,indent=2)+'\n')
report={'captures':32,'cpuProfiles':16,'allocationProfiles':16,'allocationCalls':320000,'snapshots':len(snapshots),'limiterValues':sorted(set(s['Limiter'] for s in snapshots)),'maximumHeapSnapshotBytes':max(s['HeapObjects'] for s in snapshots),'freshArchiveVerified':True,'sourceAndBinaryHashesVerified':True,'commandCount':len(commands),'hotspots':hotspots}
(p/'analysis.json').write_text(json.dumps(report,indent=2,ensure_ascii=False)+'\n')
print(json.dumps({k:v for k,v in report.items() if k!='hotspots'},indent=2))
