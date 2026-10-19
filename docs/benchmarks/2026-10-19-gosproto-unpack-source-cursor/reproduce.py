"""普通Unpack源游标的70项逐项进程比较；只扩展隔离归档，不修改常规工具或生产源码。"""
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
previous = here.parent/'2026-10-18-gosproto-post-layout-profile'
isolation = here.parent/'2026-10-19-gosproto-isolated-cases'
tool = root/'examples/websocket-gate/benchmarks/tools/go-pairs/run.py'
spec = importlib.util.spec_from_file_location('pairs', tool)
pairs = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pairs)
plan = json.loads((here/'plan.json').read_text())
output = options.output.resolve()
output.mkdir(parents=True, exist_ok=False)
probes = ['rpc-probe.go.txt', 'layout-fixtures.go.txt', 'growth-probe.go.txt', 'fixture-plan.json']
bound = {p: pairs.digest(p) for p in [here/'plan.json', here/'candidate.patch', here/'decode-probe.go.txt', isolation/'telemetry-probe.go.txt', here/'reproduce.py', tool, tool.with_name('sampler_test.go.txt'), *[previous/n for n in probes], root/'examples/websocket-gate/benchmarks/unpack_benchmark_test.go']}
commands = []
fixture_env = {'GSR_GROWTH_FIXTURE_PLAN': str(previous/'fixture-plan.json'), 'GSR_GROWTH_CASES': ','.join(plan['growthCases'])}
# 不覆盖外部非默认配置；发现不一致就停止，避免把调参混入采样。
for key, expected in [('GOGC', '100'), ('GOMEMLIMIT', 'off')]:
    if os.environ.get(key, '') not in {'', expected}:
        raise SystemExit('non-default '+key)


def run(args, cwd, env=None, timeout=60):
    started = time.monotonic()
    try:
        result = subprocess.run(args, cwd=cwd, env={**os.environ, **(env or {})}, capture_output=True, text=True, timeout=timeout)
    except subprocess.TimeoutExpired as error:
        commands.append({'args': args, 'cwd': str(cwd), 'env': env or {}, 'timeout': timeout, 'elapsedSeconds': time.monotonic()-started, 'output': str(error)})
        (output/'commands.json').write_text(json.dumps(commands, indent=2)+'\n')
        raise
    commands.append({'args': args, 'cwd': str(cwd), 'env': env or {}, 'exitCode': result.returncode, 'elapsedSeconds': time.monotonic()-started, 'output': result.stdout+result.stderr})
    (output/'commands.json').write_text(json.dumps(commands, indent=2)+'\n')
    if result.returncode:
        raise SystemExit(result.stdout+result.stderr)
    return result.stdout+result.stderr


def check_inputs(directory, manifest):
    for path, expected in bound.items():
        assert pairs.digest(path) == expected, str(path)
    for sub, key in [('before', 'beforeGoSource'), ('candidate', 'candidateGoSource'), ('harness', 'harnessGoSource')]:
        assert pairs.sources(directory/sub) == manifest[key], sub
    for path, expected in manifest['generatedGoInputs'].items():
        assert pairs.digest(root/path) == expected, path
    for name, expected in manifest['fixtures'].items():
        assert pairs.digest(directory/'harness'/name) == expected, name
    if 'binarySha256' in manifest:
        assert pairs.digest(directory/'pairs.test') == manifest['binarySha256']


def replace_once(text, old, new):
    assert text.count(old) == 1, old
    return text.replace(old, new, 1)


# 所有归档、检查、构建先完成，计时区间不穿插 Go build/vet/profile。
for name in plan['modes']:
    mode = name if name in {'self', 'clone'} else 'compare'
    directory = output/name
    args = ['python3', '-B', str(tool), '--library', str(options.library.resolve()), '--baseline', plan['baselineCommit'], '--mode', mode, '--prepare-only', '--output', str(directory)]
    if mode == 'compare':
        args += ['--candidate-patch', str(here/'candidate.patch')]
    run(args, root)
    harness = directory/'harness'
    sampler = harness/'sampler_test.go'
    source = replace_once(sampler.read_text(), 'func pairOperations(', 'func bodyPairOperations(')
    source = replace_once(source, 'type pairResult struct {', 'type pairResult struct {\n\tTelemetry []isolatedSnapshot `json:"telemetry"`')
    source = replace_once(source, '\t\truntime.GC()\n\t\t// 每项', '\t\tencodedSink, representativeSink, decodedSink = nil, nil, \"\"\n\t\truntime.GC()\n\t\tresult.Telemetry = make([]isolatedSnapshot, 0, 4)\n\t\tresult.Telemetry = append(result.Telemetry, isolatedMetrics("after_gc_before_warmup"))\n\t\t// 每项')
    source = replace_once(source, '\t\tresult.CallsPerBlock = max(leftCalls, rightCalls)', '\t\tresult.Telemetry = append(result.Telemetry, isolatedMetrics("after_calibration"))\n\t\tresult.CallsPerBlock = max(leftCalls, rightCalls)')
    source = replace_once(source, '\t\tfor repeat := range 3 {', '\t\tresult.Telemetry = append(result.Telemetry, isolatedMetrics("after_timing"))\n\t\tfor repeat := range 3 {')
    source = replace_once(source, '\t\tresults = append(results, result)', '\t\tresult.Telemetry = append(result.Telemetry, isolatedMetrics("after_allocation"))\n\t\tresults = append(results, result)')
    sampler.write_text(source)
    (harness/'rpc_probe_test.go').write_text(replace_once((previous/'rpc-probe.go.txt').read_text(), 'func pairOperations(', 'func rpcPairOperations('))
    for filename, target in [('layout-fixtures.go.txt', 'layout_fixtures_test.go'), ('growth-probe.go.txt', 'growth_probe_test.go')]:
        (harness/target).write_bytes((previous/filename).read_bytes())
    (harness/'telemetry_probe_test.go').write_bytes((isolation/'telemetry-probe.go.txt').read_bytes())
    (harness/'decode_probe_test.go').write_bytes((here/'decode-probe.go.txt').read_bytes())
    (harness/'unpack_shapes_test.go').write_bytes((root/'examples/websocket-gate/benchmarks/unpack_benchmark_test.go').read_bytes())
    # 仅扩展隔离helper以复用同一构造；Test签名不变。
    for source_file in harness.glob('*.go'):
        source_file.write_text(''.join(line.replace('t *testing.T', 't testing.TB') if line.startswith('func ') and not line.startswith('func Test') else line for line in source_file.read_text().splitlines(keepends=True)))
    run(['gofmt', '-w', *[p.name for p in harness.glob('*.go')]], harness)
    manifest = json.loads((directory/'inputs.json').read_text())
    manifest.update(beforeGoSource=pairs.sources(directory/'before'), candidateGoSource=pairs.sources(directory/'candidate'), harnessGoSource=pairs.sources(harness), selectedCases=plan['cases'], diagnosticPlan=plan, driverInputs={str(p.relative_to(root)): h for p, h in bound.items()})
    (directory/'inputs.json').write_text(json.dumps(manifest, indent=2)+'\n')
    if options.prepare_only:
        check_inputs(directory, manifest)
        print(name, 'prepared', flush=True)
        continue
    run(['go', 'test', '-mod=mod', '-run', '^(TestPairStatistics|TestPairSelection|TestPairGCCounter|TestRepresentativeFixtures|TestIsolatedMetrics)$', '.', '-timeout=1m'], harness)
    run(['go', 'vet', '-mod=mod', '.'], harness)
    binary = directory/'pairs.test'
    run(['go', 'test', '-mod=mod', '-c', '-o', str(binary), '.'], harness)
    manifest['binarySha256'] = pairs.digest(binary)
    (directory/'inputs.json').write_text(json.dumps(manifest, indent=2)+'\n')
    run([str(binary), '-test.run=^TestIsolatedCaseList$', '-test.timeout=20s'], harness, {**fixture_env, 'GOMAXPROCS': '1', 'GSR_ISOLATED_LIST': str(directory/'case-list.json')}, timeout=25)
    assert json.loads((directory/'case-list.json').read_text()) == plan['cases']
    check_inputs(directory, manifest)
    print(name, 'built and exact70 verified', flush=True)
if options.prepare_only:
    raise SystemExit(0)

# 复用既有行为契约和跨副本差分；只在正式计时开始前运行一次。
contracts = '^(TestPack|TestUnpack|TestUnpackOutputBudget|TestNativeIntegerArrayWire|TestSignedIntegerArray|TestMalformedIntegerArray|TestLimitsBeforeAllocation|TestIntegerRecordArrayBudgetsAndTags|TestNativePointerDecodeOwnership|TestNativePointerDecodePaddingAndGC|TestDecodeIntegerRecordFieldsReplacement|TestUnknownFieldAndMalformedScalar|TestSchemaEvolution|TestCodecBudgetBoundaries|TestSchemaAndMinimumByteBudget|TestRpcFrameAssembly|TestRpcFrameBodyBudget|TestRpcFrameAllocation)$'
for side in ['before', 'candidate']:
    # 前后并存的归档将before模块改名；独立modfile恢复测试自身导入，不改采样源码/模块。
    contract_mod = output/('contracts-'+side+'.mod')
    contract_mod.write_text((output/'pass1'/side/'go.mod').read_text().replace('github.com/cnooljaw/gosproto-before', 'github.com/cnooljaw/gosproto'))
    run(['go', 'test', '-modfile='+str(contract_mod), '-run', contracts, '.', '-timeout=1m'], output/'pass1'/side)
run(['go', 'test', '-mod=mod', '-run', '^(TestGrowthDifferential|TestDecodeFixtureMatrix)$', '-v', '.', '-timeout=1m'], output/'pass1'/'harness', fixture_env)

# 编译信息和反汇编同样在第一项计时之前完成。
for side in ['before', 'candidate']:
    compiler = run(['go', 'build', '-gcflags=-m=2', '.'], output/'pass1'/side)
    (output/(side+'-compiler.txt')).write_text(compiler)
for name in plan['modes']:
    assembly = run(['go', 'tool', 'objdump', '-s', r'github.com/cnooljaw/gosproto\.(UnpackWithLimit|zeroHeaderPrefix|decodeIntSlice)', str(output/name/'pairs.test')], root)
    (output/(name+'-record-assembly.txt')).write_text(assembly)

for name in plan['modes']:
    mode = name if name in {'self', 'clone'} else 'compare'
    directory = output/name
    manifest = json.loads((directory/'inputs.json').read_text())
    aggregate = None
    summaries = []
    for index, case_name in enumerate(plan['cases']):
        path = directory/('case-%02d.json' % index)
        run([str(directory/'pairs.test'), '-test.run=^TestPairSampling$', '-test.v', '-test.timeout=15s'], directory/'harness', {**fixture_env, 'GOMAXPROCS': '1', 'GSR_PAIR_MODE': mode, 'GSR_PAIR_OUTPUT': str(path), 'GSR_PAIR_BLOCK_TARGET': '10ms', 'GSR_PAIR_CASES': case_name}, timeout=20)
        raw = json.loads(path.read_text())
        assert (raw['mode'], raw['rounds'], raw['quartetsPerRound'], raw['targetBlockNs'], raw['gogcPercent']) == (mode, 6, 4, 10000000, 100)
        assert len(raw['results']) == 1 and raw['results'][0]['name'] == case_name
        result = raw['results'][0]
        assert len(result['quartets']) == 24 and len(result['roundRatios']) == 6
        assert [s['Stage'] for s in result['telemetry']] == plan['telemetry']
        assert all(s['GOGC'] == 100 and s['MemoryLimit'] == 2**63-1 for s in result['telemetry'])
        assert mode != 'self' or result['sameFunction']
        if aggregate is None:
            aggregate = {**raw, 'results': []}
        aggregate['results'].append(result)
        summaries.extend(pairs.summarize(raw))
        (directory/'raw.json').write_text(json.dumps(aggregate, separators=(',', ':'))+'\n')
        (directory/'summary.json').write_text(json.dumps(summaries, indent=2)+'\n')
        check_inputs(directory, manifest)
        if (index+1) % 10 == 0:
            print(name, index+1, '/%d complete' % len(plan['cases']), flush=True)
    manifest['rawSha256'] = pairs.digest(directory/'raw.json')
    manifest['caseRawSha256'] = {p.name: pairs.digest(p) for p in sorted(directory.glob('case-[0-9][0-9].json'))}
    (directory/'inputs.json').write_text(json.dumps(manifest, indent=2)+'\n')
    print(name, 'complete;all failures retained', flush=True)
