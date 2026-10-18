"""在固定归档中分开采集布局优化后的编码 CPU 与逐次分配 profile。"""

import argparse
import importlib.util
import json
import os
import re
from pathlib import Path
import subprocess
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--library', type=Path, required=True)
parser.add_argument('--output', type=Path, required=True)
parser.add_argument('--prepare-only', action='store_true')
options = parser.parse_args()
here = Path(__file__).resolve().parent
root = here.parents[2]
tool = root / 'examples/websocket-gate/benchmarks/tools/go-pairs/run.py'
spec = importlib.util.spec_from_file_location('pairs', tool)
pairs = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pairs)
plan = json.loads((here / 'plan.json').read_text())
output = options.output.resolve()
output.mkdir(parents=True, exist_ok=False)
commands = []
bound = {p: pairs.digest(p) for p in [here / 'plan.json', here / 'profile-probe.go.txt', here / 'reproduce.py', *[here / n for n in ['rpc-probe.go.txt','joint-probe.go.txt','layout-fixtures.go.txt','growth-probe.go.txt','fixture-plan.json','context-probe.go.txt']], tool, tool.with_name('sampler_test.go.txt')]}


def run(args, cwd, env=None, timeout=60):
    started = time.monotonic()
    try:
        result = subprocess.run(args, cwd=cwd, env={**os.environ, **(env or {})}, capture_output=True, text=True, timeout=timeout)
    except subprocess.TimeoutExpired as error:
        commands.append({'args': args, 'cwd': str(cwd), 'env': env or {}, 'elapsedSeconds': round(time.monotonic() - started, 3), 'timeout': timeout, 'output': str(error)})
        (output / 'commands.json').write_text(json.dumps(commands, indent=2) + '\n')
        raise
    commands.append({'args': args, 'cwd': str(cwd), 'env': env or {}, 'elapsedSeconds': round(time.monotonic() - started, 3), 'exitCode': result.returncode, 'output': result.stdout + result.stderr})
    (output / 'commands.json').write_text(json.dumps(commands, indent=2) + '\n')
    if result.returncode:
        raise SystemExit(result.stdout + result.stderr)
    return result.stdout


run(['python3', '-B', str(tool), '--library', str(options.library.resolve()), '--baseline', plan['baselineCommit'], '--mode', 'clone', '--prepare-only', '--output', str(output / 'archive')], root)
harness = output / 'archive/harness'
(harness / 'profile_test.go').write_bytes((here / 'profile-probe.go.txt').read_bytes())
sampler=harness/'sampler_test.go'
sampler.write_text(sampler.read_text().replace('func pairOperations(', 'func bodyPairOperations(', 1))
(harness/'rpc_probe_test.go').write_text((here/'rpc-probe.go.txt').read_text().replace('func pairOperations(', 'func rpcPairOperations(', 1))
for filename,target in [('joint-probe.go.txt','joint_probe_test.go'),('layout-fixtures.go.txt','layout_fixtures_test.go'),('growth-probe.go.txt','growth_probe_test.go'),('context-probe.go.txt','context_probe_test.go')]:
    (harness/target).write_bytes((here/filename).read_bytes())
# 仅将隔离harness的helper参数扩为testing.TB，让同一组构造可被Benchmark复用；Test签名不变。
for source in harness.glob('*.go'):
    lines=source.read_text().splitlines(keepends=True)
    source.write_text(''.join(line.replace('t *testing.T', 't testing.TB') if line.startswith('func ') and not line.startswith('func Test') else line for line in lines))
fixture_env={'GSR_GROWTH_FIXTURE_PLAN':str(here/'fixture-plan.json'),'GSR_GROWTH_CASES':','.join(plan['growthCases'])}

run(['gofmt', '-w', *[p.name for p in harness.glob('*.go')]], harness)
manifest = json.loads((output / 'archive/inputs.json').read_text())
manifest['harnessGoSource'] = pairs.sources(harness)
manifest['diagnosticPlan'] = plan
manifest['driverInputs'] = {str(p.relative_to(root)): h for p, h in bound.items()}
manifest['goModFiles'] = {str(p.relative_to(output)): pairs.digest(p) for p in sorted(output.rglob('go.mod'))}
(output / 'inputs.json').write_text(json.dumps(manifest, indent=2) + '\n')
if options.prepare_only:
    print('archive prepared', flush=True)
    raise SystemExit(0)
run(['go', 'test', '-mod=mod', '-run', '^(TestPairStatistics|TestPairSelection|TestRepresentativeFixtures)$', '.', '-timeout=1m'], harness,fixture_env)
run(['go', 'vet', '-mod=mod', '.'], harness)
binary = output / 'encoding.test'
run(['go', 'test', '-mod=mod', '-c', '-o', str(binary), '.'], harness)
manifest['binarySha256'] = pairs.digest(binary)
manifest['rawProfileSha256'] = {}
for case in plan['cases']:
    for entry in plan['entries']:
        name = case + '-' + entry
        pattern = '^BenchmarkCurrentEncoding$/^' + case + '$/^' + entry + '$'
        cpu, memory = output / (name + '.cpu'), output / (name + '.mem')
        base = [str(binary), '-test.run=^$', '-test.bench=' + pattern, '-test.benchmem', '-test.timeout=45s']
        cpu_output = run(base + ['-test.benchtime=' + plan['cpu']['benchtime'], '-test.cpuprofile=' + str(cpu)], harness, {'GOMAXPROCS': '1'}, timeout=50)
        memory_output = run(base + ['-test.benchtime=' + plan['allocation']['benchtime'], '-test.memprofile=' + str(memory), '-test.memprofilerate=' + str(plan['allocation']['memProfileRate'])], harness, {'GOMAXPROCS': '1'}, timeout=50)
        expected = 'BenchmarkCurrentEncoding/' + case + '/' + entry
        for captured in [cpu_output, memory_output]:
            names = re.findall(r'^(BenchmarkCurrentEncoding/[^\s]+)\s+\d+', captured, re.M)
            if names != [expected]:
                raise SystemExit('profile selection mismatch: ' + repr(names))
        for suffix, args, profile in [
            ('cpu-flat', ['-top', '-nodecount=40'], cpu),
            ('cpu-cum', ['-top', '-cum', '-nodecount=40'], cpu),
            ('cpu-source', ['-list=github.com/cnooljaw/gosproto\\.(encodedSize|fieldSize|encodeMessageInto|encodeFieldInto|encodedIntegerRecordSize|encodeIntegerRecordInto|encodeInt64Slice|packInto)'], cpu),
            ('alloc-space', ['-top', '-sample_index=alloc_space', '-nodecount=30'], memory),
            ('alloc-objects', ['-top', '-sample_index=alloc_objects', '-nodecount=30'], memory),
            ('alloc-source', ['-list=github.com/cnooljaw/gosproto\\.(encodeBufferWithLimits|.*encodeLayout.*add|RequestEncode)', '-sample_index=alloc_space'], memory),
        ]:
            summary = run(['go', 'tool', 'pprof', *args, str(binary), str(profile)], harness)
            (output / (name + '-' + suffix + '.txt')).write_text(summary)
        for profile in [cpu, memory]:
            manifest['rawProfileSha256'][profile.name] = pairs.digest(profile)
        for source, key in [(output / 'archive/candidate', 'candidateGoSource'), (harness, 'harnessGoSource')]:
            if pairs.sources(source) != manifest[key]:
                raise SystemExit('source changed: ' + str(source))
        for p, expected in bound.items():
            if pairs.digest(p) != expected:
                raise SystemExit('input changed: ' + str(p))
        (output / 'inputs.json').write_text(json.dumps(manifest, indent=2) + '\n')
        print(name, 'complete', flush=True)

for case in plan['context']['cases']:
    for live in plan['context']['contexts']:
        name=case.replace('/','-')+'-'+live
        path=output/(name+'.json')
        env={**fixture_env,'GOMAXPROCS':'1','GSR_CONTEXT_CASE':case,'GSR_CONTEXT_LIVE':live,'GSR_CONTEXT_OUTPUT':str(path)}
        run([str(binary),'-test.run=^TestPackContextBlocks$','-test.v','-test.timeout=45s'],harness,env,timeout=50)
        raw=json.loads(path.read_text())
        if raw['Case']!=case or raw['Context']!=live or len(raw['Quartets'])!=24:
            raise SystemExit('context selection incomplete')
        manifest.setdefault('rawContextSha256',{})[path.name]=pairs.digest(path)
        if case==plan['context']['profileCase']:
            cpu=output/(name+'.cpu')
            run([str(binary),'-test.run=^$','-test.bench=^BenchmarkPackContext$','-test.benchmem','-test.benchtime='+plan['context']['cpuBenchtime'],'-test.cpuprofile='+str(cpu),'-test.timeout=30s'],harness,env,timeout=35)
            manifest['rawProfileSha256'][cpu.name]=pairs.digest(cpu)
            for suffix,args in [('cpu-flat',['-top','-nodecount=40']),('cpu-cum',['-top','-cum','-nodecount=40']),('cpu-source',['-list=github.com/cnooljaw/gosproto\\.packInto'])]:
                summary=run(['go','tool','pprof',*args,str(binary),str(cpu)],harness)
                (output/(name+'-'+suffix+'.txt')).write_text(summary)
        for source,key in [(output/'archive/candidate','candidateGoSource'),(harness,'harnessGoSource')]:
            if pairs.sources(source)!=manifest[key]:
                raise SystemExit('source changed')
        for p,expected in bound.items():
            if pairs.digest(p)!=expected:
                raise SystemExit('input changed: '+str(p))
        (output/'inputs.json').write_text(json.dumps(manifest,indent=2)+'\n')
        print(name,'complete',flush=True)
