"""在固定归档中分开采集融合后编码 CPU 与逐次分配 profile。"""

import argparse
import importlib.util
import json
import os
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
bound = {p: pairs.digest(p) for p in [here / 'plan.json', here / 'profile-probe.go.txt', here / 'reproduce.py', tool, tool.with_name('sampler_test.go.txt')]}


def run(args, cwd, env=None, timeout=60):
    started = time.monotonic()
    result = subprocess.run(args, cwd=cwd, env={**os.environ, **(env or {})}, capture_output=True, text=True, timeout=timeout)
    commands.append({'args': args, 'cwd': str(cwd), 'env': env or {}, 'elapsedSeconds': round(time.monotonic() - started, 3), 'exitCode': result.returncode, 'output': result.stdout + result.stderr})
    (output / 'commands.json').write_text(json.dumps(commands, indent=2) + '\n')
    if result.returncode:
        raise SystemExit(result.stdout + result.stderr)
    return result.stdout


run(['python3', '-B', str(tool), '--library', str(options.library.resolve()), '--baseline', plan['baselineCommit'], '--mode', 'clone', '--prepare-only', '--output', str(output / 'archive')], root)
harness = output / 'archive/harness'
(harness / 'profile_test.go').write_bytes((here / 'profile-probe.go.txt').read_bytes())
run(['gofmt', '-w', 'profile_test.go'], harness)
manifest = json.loads((output / 'archive/inputs.json').read_text())
manifest['harnessGoSource'] = pairs.sources(harness)
manifest['diagnosticPlan'] = plan
manifest['driverInputs'] = {str(p.relative_to(root)): h for p, h in bound.items()}
manifest['goModFiles'] = {str(p.relative_to(output)): pairs.digest(p) for p in sorted(output.rglob('go.mod'))}
(output / 'inputs.json').write_text(json.dumps(manifest, indent=2) + '\n')
if options.prepare_only:
    print('archive prepared', flush=True)
    raise SystemExit(0)
run(['go', 'test', '-mod=mod', '-run', '^(TestPairStatistics|TestPairSelection|TestRepresentativeFixtures)$', '.', '-timeout=1m'], harness)
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
        run(base + ['-test.benchtime=' + plan['cpu']['benchtime'], '-test.cpuprofile=' + str(cpu)], harness, {'GOMAXPROCS': '1'}, timeout=50)
        run(base + ['-test.benchtime=' + plan['allocation']['benchtime'], '-test.memprofile=' + str(memory), '-test.memprofilerate=' + str(plan['allocation']['memProfileRate'])], harness, {'GOMAXPROCS': '1'}, timeout=50)
        for suffix, args, profile in [
            ('cpu-flat', ['-top', '-nodecount=40'], cpu),
            ('cpu-cum', ['-top', '-cum', '-nodecount=40'], cpu),
            ('cpu-source', ['-list=github.com/cnooljaw/gosproto\\.(encodedSize|encodeMessageInto|packInto)'], cpu),
            ('alloc-space', ['-top', '-sample_index=alloc_space', '-nodecount=30'], memory),
            ('alloc-objects', ['-top', '-sample_index=alloc_objects', '-nodecount=30'], memory),
            ('alloc-source', ['-list=github.com/cnooljaw/gosproto\\.encodeBufferWithLimits', '-sample_index=alloc_space'], memory),
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
