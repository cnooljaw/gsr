"""从已提交归档复现布局项数、实际增长与未插桩入口分配诊断；不计 CPU。"""

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
bound = {p: pairs.digest(p) for p in [here / 'plan.json', here / 'layout-hook.go.txt', here / 'layout-probe.go.txt', here / 'reproduce.py', tool, tool.with_name('sampler_test.go.txt')]}


def run(args, cwd, env=None, timeout=60):
    started = time.monotonic()
    try:
        result = subprocess.run(args, cwd=cwd, env={**os.environ, **(env or {})}, capture_output=True, text=True, timeout=timeout)
        record = {'args': args, 'cwd': str(cwd), 'env': env or {}, 'elapsedSeconds': round(time.monotonic()-started, 3), 'exitCode': result.returncode, 'output': result.stdout + result.stderr}
    except subprocess.TimeoutExpired as error:
        record = {'args': args, 'cwd': str(cwd), 'env': env or {}, 'elapsedSeconds': round(time.monotonic()-started, 3), 'timeout': timeout, 'output': str(error)}
        commands.append(record)
        (output / 'commands.json').write_text(json.dumps(commands, indent=2) + '\n')
        raise
    commands.append(record)
    (output / 'commands.json').write_text(json.dumps(commands, indent=2) + '\n')
    if result.returncode:
        raise SystemExit(record['output'])
    return result.stdout


run(['python3', '-B', str(tool), '--library', str(options.library.resolve()), '--baseline', plan['baselineCommit'], '--mode', 'clone', '--prepare-only', '--output', str(output / 'archive')], root)
harness = output / 'archive/harness'
helper = output / 'archive/candidate/diagnostic_layout.go'
helper.write_bytes((here / 'layout-hook.go.txt').read_bytes())
(harness / 'layout_test.go').write_bytes((here / 'layout-probe.go.txt').read_bytes())
run(['gofmt', '-w', str(helper), 'layout_test.go'], harness)
manifest = json.loads((output / 'archive/inputs.json').read_text())
manifest['diagnosticOnlyGoSource'] = pairs.sources(output / 'archive/candidate')
manifest['harnessGoSource'] = pairs.sources(harness)
manifest['plan'] = plan
manifest['driverInputs'] = {str(p.relative_to(root)): h for p, h in bound.items()}
manifest['goModFiles'] = {str(p.relative_to(output)): pairs.digest(p) for p in sorted(output.rglob('go.mod'))}
(output / 'inputs.json').write_text(json.dumps(manifest, indent=2) + '\n')
if options.prepare_only:
    print('archive prepared', flush=True)
    raise SystemExit(0)
# baseline 源码不注入 helper，分配计数调用的都是这个模块。
if pairs.sources(output / 'archive/before') != manifest['beforeGoSource']:
    raise SystemExit('baseline source changed')
run(['go', 'vet', '-mod=mod', '.'], harness)
binary = output / 'layout.test'
run(['go', 'test', '-mod=mod', '-c', '-o', str(binary), '.'], harness)
manifest['binarySha256'] = pairs.digest(binary)
(output / 'inputs.json').write_text(json.dumps(manifest, indent=2) + '\n')
run([str(binary), '-test.run=^TestLayoutAudit$', '-test.v', '-test.timeout=1m'], harness,
    {'GOMAXPROCS': '1', 'GSR_LAYOUT_PLAN': str(here / 'plan.json'), 'GSR_LAYOUT_OUTPUT': str(output / 'raw.json')}, timeout=65)
for directory, key in [('before', 'beforeGoSource'), ('candidate', 'diagnosticOnlyGoSource'), ('harness', 'harnessGoSource')]:
    if pairs.sources(output / 'archive' / directory) != manifest[key]:
        raise SystemExit('source changed: ' + directory)
for p, expected in bound.items():
    if pairs.digest(p) != expected:
        raise SystemExit('driver changed: ' + str(p))
for p, expected in manifest['generatedGoInputs'].items():
    if pairs.digest(root / p) != expected:
        raise SystemExit('generated source changed: ' + p)
manifest['rawSha256'] = pairs.digest(output / 'raw.json')
(output / 'inputs.json').write_text(json.dumps(manifest, indent=2) + '\n')
print('layout diagnosis complete', flush=True)
