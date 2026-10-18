"""复现布局按需倍增的固定70项配对；所有构建完成后再顺序采样。"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--library',type=Path,required=True)
parser.add_argument('--output',type=Path,required=True)
parser.add_argument('--prepare-only',action='store_true')
options=parser.parse_args()
here=Path(__file__).resolve().parent
root=here.parents[2]
tool=root/'examples/websocket-gate/benchmarks/tools/go-pairs/run.py'
spec=importlib.util.spec_from_file_location('pairs',tool)
pairs=importlib.util.module_from_spec(spec)
spec.loader.exec_module(pairs)
plan=json.loads((here/'plan.json').read_text())
output=options.output.resolve()
output.mkdir(parents=True,exist_ok=False)
commands=[]
bound={p:pairs.digest(p) for p in [here/'candidate.patch',here/'plan.json',here/'fixture-plan.json',here/'rpc-probe.go.txt',here/'joint-probe.go.txt',here/'layout-fixtures.go.txt',here/'growth-probe.go.txt',here/'reproduce.py',tool,tool.with_name('sampler_test.go.txt')]}
(output/'driver-inputs.json').write_text(json.dumps({str(p.relative_to(root)):h for p,h in bound.items()},indent=2)+'\n')
fixture_env={'GSR_GROWTH_FIXTURE_PLAN':str(here/'fixture-plan.json'),'GSR_GROWTH_CASES':','.join(plan['measurement']['growthCases'])}


def run(args,cwd,env=None,timeout=60):
    started=time.monotonic()
    try:
        result=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=timeout)
        record={'args':args,'cwd':str(cwd),'env':env or {},'exitCode':result.returncode,'elapsedSeconds':round(time.monotonic()-started,3),'output':result.stdout+result.stderr}
    except subprocess.TimeoutExpired as error:
        record={'args':args,'cwd':str(cwd),'env':env or {},'timeout':timeout,'elapsedSeconds':round(time.monotonic()-started,3),'output':str(error)}
        commands.append(record)
        (output/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
        raise
    commands.append(record)
    (output/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
    if result.returncode:
        raise SystemExit(record['output'])
    return result.stdout


def check_bound():
    for path,expected in bound.items():
        if pairs.digest(path)!=expected:
            raise SystemExit('input changed: '+str(path))


for name in plan['measurement']['modes']:
    mode=name if name in {'self','clone'} else 'compare'
    directory=output/name
    args=['python3','-B',str(tool),'--library',str(options.library.resolve()),'--baseline',plan['baselineCommit'],'--mode',mode,'--output',str(directory),'--block-ms','10','--prepare-only']
    if mode=='compare':
        args+=['--candidate-patch',str(here/'candidate.patch')]
    run(args,root)
    harness=directory/'harness'
    sampler=harness/'sampler_test.go'
    sampler.write_text(sampler.read_text().replace('func pairOperations(','func bodyPairOperations(',1))
    (harness/'rpc_probe_test.go').write_text((here/'rpc-probe.go.txt').read_text().replace('func pairOperations(','func rpcPairOperations(',1))
    for filename,target in [('joint-probe.go.txt','joint_probe_test.go'),('layout-fixtures.go.txt','layout_fixtures_test.go'),('growth-probe.go.txt','growth_probe_test.go')]:
        (harness/target).write_bytes((here/filename).read_bytes())
    run(['gofmt','-w','sampler_test.go','rpc_probe_test.go','joint_probe_test.go','layout_fixtures_test.go','growth_probe_test.go'],harness)
    manifest=json.loads((directory/'inputs.json').read_text())
    manifest['harnessGoSource']=pairs.sources(harness)
    manifest['selectedCases']='70: original43 body/manualframe/actualRPC/wide64 plus9 layout shapes x3 real entries'
    manifest['driverInputs']={str(p.relative_to(root)):h for p,h in bound.items()}
    (directory/'inputs.json').write_text(json.dumps(manifest,indent=2)+'\n')
    if options.prepare_only:
        print(name,'prepared',flush=True)
        continue
    run(['go','test','-mod=mod','-run','^(TestPairStatistics|TestPairSelection|TestPairGCCounter|TestRepresentativeFixtures)$','.','-timeout=1m'],harness,fixture_env)
    if name=='pass1':
        run(['go','test','-mod=mod','-run','^TestGrowthDifferential$','-v','.','-timeout=1m'],harness,fixture_env)
        run(['go','vet','-mod=mod','.'],harness)
        for side in ['before','candidate']:
            compiler=run(['go','build','-gcflags=-m=2','.'],directory/side)
            (output/(side+'-compiler.txt')).write_text(compiler)
    binary=directory/'encoding.test'
    run(['go','test','-mod=mod','-c','-o',str(binary),'.'],harness)
    manifest['binarySha256']=pairs.digest(binary)
    (directory/'inputs.json').write_text(json.dumps(manifest,indent=2)+'\n')
    print(name,'built',flush=True)
if options.prepare_only:
    check_bound()
    raise SystemExit(0)
for name in plan['measurement']['modes']:
    mode=name if name in {'self','clone'} else 'compare'
    directory=output/name
    harness=directory/'harness'
    manifest=json.loads((directory/'inputs.json').read_text())
    run([str(directory/'encoding.test'),'-test.run=^TestPairSampling$','-test.v','-test.timeout=110s'],harness,
        {**fixture_env,'GOMAXPROCS':'1','GSR_PAIR_MODE':mode,'GSR_PAIR_OUTPUT':str(directory/'raw.json'),'GSR_PAIR_BLOCK_TARGET':'10ms','GSR_PAIR_CASES':''},timeout=115)
    for source,key in [(directory/'before','beforeGoSource'),(directory/'candidate','candidateGoSource'),(harness,'harnessGoSource')]:
        if pairs.sources(source)!=manifest[key]:
            raise SystemExit('source changed: '+str(source))
    for path,expected in manifest['generatedGoInputs'].items():
        if pairs.digest(root/path)!=expected:
            raise SystemExit('generated source changed: '+path)
    raw=json.loads((directory/'raw.json').read_text())
    if (raw['rounds'],raw['quartetsPerRound'],raw['targetBlockNs'],len(raw['results']))!=(6,4,10000000,70):
        raise SystemExit('sampling incomplete')
    for result in raw['results']:
        if len(result['quartets'])!=24 or len(result['roundRatios'])!=6 or (mode=='self' and not result['sameFunction']):
            raise SystemExit('bad selection or incomplete sampling')
    (directory/'summary.json').write_text(json.dumps(pairs.summarize(raw),indent=2)+'\n')
    check_bound()
    print(name,'complete',flush=True)
