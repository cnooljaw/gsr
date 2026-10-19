"""原70项构造、单项保活；分别采当前编码入口CPU和分配，不构造性能候选。"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--library', type=Path, required=True)
parser.add_argument('--output', type=Path, required=True)
parser.add_argument('--prepare-only', action='store_true')
options = parser.parse_args()
here = Path(__file__).resolve().parent
root = here.parents[2]
previous = here.parent / '2026-10-18-gosproto-post-layout-profile'
telemetry = here.parent / '2026-10-19-gosproto-isolated-cases/telemetry-probe.go.txt'
tool = root / 'examples/websocket-gate/benchmarks/tools/go-pairs/run.py'
spec = importlib.util.spec_from_file_location('pairs', tool)
pairs = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pairs)
plan = json.loads((here / 'plan.json').read_text())
output = options.output.resolve()
output.mkdir(parents=True, exist_ok=False)
commands = []
files = [here / n for n in ['plan.json', 'profile-probe.go.txt', 'reproduce.py']]
files += [previous / n for n in ['rpc-probe.go.txt', 'joint-probe.go.txt', 'layout-fixtures.go.txt', 'growth-probe.go.txt', 'fixture-plan.json']]
files += [telemetry, tool, tool.with_name('sampler_test.go.txt')]
bound = {p: pairs.digest(p) for p in files}


def save(path, value):
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False) + '\n')


def run(args, cwd, env=None, timeout=60):
    start = time.monotonic()
    try:
        result = subprocess.run(args, cwd=cwd, env={**os.environ, **(env or {})}, capture_output=True, text=True, timeout=timeout)
    except subprocess.TimeoutExpired as error:
        commands.append({'args': args, 'cwd': str(cwd), 'env': env or {}, 'timeout': timeout, 'elapsedSeconds': time.monotonic()-start, 'output': str(error)})
        save(output / 'commands.json', commands)
        raise
    commands.append({'args': args, 'cwd': str(cwd), 'env': env or {}, 'exitCode': result.returncode, 'elapsedSeconds': time.monotonic()-start, 'output': result.stdout+result.stderr})
    save(output / 'commands.json', commands)
    if result.returncode:
        raise SystemExit(result.stdout+result.stderr)
    return result.stdout+result.stderr


run(['python3', '-B', str(tool), '--library', str(options.library.resolve()), '--baseline', plan['baselineCommit'], '--mode', 'clone', '--prepare-only', '--output', str(output/'archive')], root)
harness = output/'archive/harness'
sampler = harness/'sampler_test.go'
sampler.write_text(sampler.read_text().replace('func pairOperations(', 'func bodyPairOperations(', 1))
(harness/'rpc_probe_test.go').write_text((previous/'rpc-probe.go.txt').read_text().replace('func pairOperations(', 'func rpcPairOperations(', 1))
for name, target in [('joint-probe.go.txt','joint_probe_test.go'), ('layout-fixtures.go.txt','layout_fixtures_test.go'), ('growth-probe.go.txt','growth_probe_test.go')]:
    (harness/target).write_bytes((previous/name).read_bytes())
(harness/'telemetry_test.go').write_bytes(telemetry.read_bytes())
(harness/'current_profile_test.go').write_bytes((here/'profile-probe.go.txt').read_bytes())
# 只扩展归档内helper参数，使Benchmark复用原构造器；不改变Test入口和正式源码。
for source in harness.glob('*.go'):
    source.write_text(''.join(line.replace('t *testing.T', 't testing.TB') if line.startswith('func ') and not line.startswith('func Test') else line for line in source.read_text().splitlines(keepends=True)))
run(['gofmt', '-w', *[p.name for p in harness.glob('*.go')]], harness)
fixture_env = {'GSR_GROWTH_FIXTURE_PLAN': str(previous/'fixture-plan.json'), 'GSR_GROWTH_CASES': ','.join(plan['growthCases']), 'GOMAXPROCS': '1'}
manifest = json.loads((output/'archive/inputs.json').read_text())
manifest['harnessGoSource'] = pairs.sources(harness)
manifest['driverInputs'] = {str(p.relative_to(root)): h for p,h in bound.items()}
manifest['diagnosticPlan'] = plan
manifest['goModFiles'] = {str(p.relative_to(output)): pairs.digest(p) for p in sorted(output.rglob('go.mod'))}
save(output/'inputs.json', manifest)
if options.prepare_only:
    print('archive prepared', flush=True)
    raise SystemExit(0)
run(['go','test','-mod=mod','-run','^(TestPairStatistics|TestPairSelection|TestPairGCCounter|TestRepresentativeFixtures|TestIsolatedMetrics|TestIsolatedCaseList)$','.', '-timeout=1m'], harness, {**fixture_env,'GSR_ISOLATED_LIST':str(output/'case-list.json')})
if json.loads((output/'case-list.json').read_text()) != plan['original70']:
    raise SystemExit('original70 mismatch')
run(['go','vet','-mod=mod','.'], harness, fixture_env)
binary = output/'current.test'
run(['go','test','-mod=mod','-c','-o',str(binary),'.'], harness, fixture_env)
manifest['binarySha256'] = pairs.digest(binary)
manifest['rawProfileSha256'] = {}
summary = []
for case in plan['cases']:
    name = case.replace('/','-')
    for kind in ['cpu','allocation']:
        meta = output/(name+'-'+kind+'.json')
        profile = output/(name+('.cpu' if kind=='cpu' else '.mem'))
        env = {**fixture_env, 'GSR_PROFILE_CASE':case, 'GSR_PROFILE_META':str(meta), 'GSR_PROFILE_KIND':kind, 'GSR_PROFILE_CPU':str(profile)}
        args = [str(binary),'-test.run=^$','-test.bench=^BenchmarkCurrentOperation$','-test.benchmem','-test.timeout=45s','-test.benchtime='+plan[kind]['benchtime']]
        if kind=='allocation':
            args += ['-test.memprofile='+str(profile),'-test.memprofilerate='+str(plan[kind]['memProfileRate'])]
        captured = run(args,harness,env,timeout=50)
        rows = re.findall(r'^BenchmarkCurrentOperation\s+(\d+)\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op', captured,re.M)
        if len(rows)!=1:
            raise SystemExit('benchmark output mismatch: '+captured)
        raw = json.loads(meta.read_text())
        if raw['Case']!=case or raw['Kind']!=kind or [s['Stage'] for s in raw['Stages']]!=plan['telemetry'] or raw['Calls']!=int(rows[0][0]):
            raise SystemExit('metadata mismatch')
        if kind=='allocation' and raw['Calls']!=20000:
            raise SystemExit('allocation count mismatch')
        if any(s['GOGC']!=100 or s['MemoryLimit']!=2**63-1 for s in raw['Stages']):
            raise SystemExit('nondefault GC settings')
        summary.append({'case':case,'kind':kind,'calls':int(rows[0][0]),'nsPerOp':float(rows[0][1]),'bytesPerOp':int(rows[0][2]),'allocsPerOp':int(rows[0][3]),'metadata':meta.name})
        manifest['rawProfileSha256'][profile.name] = pairs.digest(profile)
        for source,key in [(output/'archive/before','beforeGoSource'),(output/'archive/candidate','candidateGoSource'),(harness,'harnessGoSource')]:
            if pairs.sources(source)!=manifest[key]:
                raise SystemExit('source changed: '+str(source))
        if pairs.digest(binary)!=manifest['binarySha256'] or any(pairs.digest(p)!=h for p,h in bound.items()):
            raise SystemExit('bound input changed')
        save(output/'inputs.json',manifest)
        save(output/'measurements.json',summary)
        print(case,kind,'complete',flush=True)
# 所有采集结束后再渲染pprof，避免与采样争抢资源。
for case in plan['cases']:
    name=case.replace('/','-')
    for suffix,args,ext in [
        ('cpu-flat',['-top','-nodecount=40'],'.cpu'),
        ('cpu-cum',['-top','-cum','-nodecount=40'],'.cpu'),
        ('cpu-source',['-list=github.com/cnooljaw/gosproto\\.(packInto|ffRunBytes|writeFF|encodeBufferWithLimits|encodeMessageInto|encodeFieldInto|preflight.*|.*encodeLayout.*|RequestEncode|ResponseEncode|.*RPC.*)'],'.cpu'),
        ('alloc-space',['-top','-sample_index=alloc_space','-nodecount=30'],'.mem'),
        ('alloc-objects',['-top','-sample_index=alloc_objects','-nodecount=30'],'.mem'),
        ('alloc-source',['-list=github.com/cnooljaw/gosproto\\.(encodeBufferWithLimits|.*encodeLayout.*|RequestEncode|ResponseEncode|.*RPC.*)','-sample_index=alloc_space'],'.mem'),
    ]:
        rendered=run(['go','tool','pprof',*args,str(binary),str(output/(name+ext))],harness)
        (output/(name+'-'+suffix+'.txt')).write_text(rendered)
print('32 captures complete',flush=True)
