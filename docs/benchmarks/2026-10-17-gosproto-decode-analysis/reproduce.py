"""从固定归档建立隔离诊断副本；不修改传入库或 GSR 生产源码。"""
from pathlib import Path
import argparse
import hashlib
import io
import json
import os
import statistics
import subprocess
import tarfile

parser = argparse.ArgumentParser()
parser.add_argument('--library', required=True)
parser.add_argument('--output', required=True)
parser.add_argument('--prepare-only', action='store_true')
args = parser.parse_args()
report = Path(__file__).resolve().parent
root = report.parents[2]
base = Path(args.output).resolve()
if base.exists():
    raise SystemExit('输出目录已存在，请指定新目录')
base.mkdir(parents=True)
archive = subprocess.check_output(['git', 'archive', 'd66644a'], cwd=args.library)
for name in ['normal', 'baseline', 'fixed', 'typed']:
    target = base / name
    target.mkdir()
    with tarfile.open(fileobj=io.BytesIO(archive)) as bundle:
        bundle.extractall(target, filter='data')
    if name == 'normal':
        continue
    mod = target / 'go.mod'
    mod.write_text(mod.read_text().replace('module github.com/cnooljaw/gosproto',
                   'module github.com/cnooljaw/gosproto-diag-' + name, 1))
    if name == 'baseline':
        source = (report / 'probe-types.go.txt').read_text()
    else:
        source = (report / 'item-parser.go.txt').read_text()
        record = 'record := value.Elem()'
        store = 'record.Field(currentTag).Set(reflect.ValueOf(result))'
        if name == 'typed':
            record = 'record := value.Interface().(*DiagnosticItem)'
            store = 'switch currentTag {case 0: record.Id=result; case 1: record.Count=result; case 2: record.Power=result}'
        source = source.replace('RECORD', record).replace('STORE', store)
        decoder = target / 'decode.go'
        declaration = 'func decodeMessage(chunk []byte, schema *SprotoType, value reflect.Value, budget *decodeBudget) (int, error) {'
        decoder.write_text(decoder.read_text().replace(declaration, declaration +
            '\n if schema.typ == reflect.TypeFor[DiagnosticItem]() {return diagnosticDecodeItem(chunk,value,budget)}', 1))
    (target / 'diagnostic.go').write_text(source)
    subprocess.run(['gofmt', '-w', 'diagnostic.go', 'decode.go'], cwd=target, check=True)
bench = base / 'bench'
bench.mkdir()
for name in ['representative_benchmark_test.go', 'protocol_benchmark_test.go',
             'representative.json', 'correctness.json', 'cases.json']:
    (bench / name).write_bytes((root / 'examples/websocket-gate/benchmarks' / name).read_bytes())
(bench / 'diagnosis_test.go').write_bytes((report / 'diagnosis-probe.go.txt').read_bytes())
(bench / 'go.sum').write_bytes((root / 'go.sum').read_bytes())
mod = 'module decodediag\n\ngo 1.24.0\n\nrequire (\n github.com/cnooljaw/gsr v0.0.0\n github.com/cnooljaw/gosproto v0.4.0\n google.golang.org/protobuf v1.36.1\n'
for name in ['baseline', 'fixed', 'typed']:
    mod += ' github.com/cnooljaw/gosproto-diag-' + name + ' v0.0.0\n'
mod += ')\nreplace github.com/cnooljaw/gsr => ' + str(root) + '\n'
mod += 'replace github.com/cnooljaw/gosproto => ' + str(base / 'normal') + '\n'
for name in ['baseline', 'fixed', 'typed']:
    mod += 'replace github.com/cnooljaw/gosproto-diag-' + name + ' => ' + str(base / name) + '\n'
(bench / 'go.mod').write_text(mod)
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
expected = json.loads((report / 'diagnosis.json').read_text())
for name in ['baseline', 'fixed', 'typed']:
    assert expected['sources'][name] == {p.name: sha(p) for p in (base / name).glob('*.go')}, name
assert expected['originalGoSource'] == {p.name: sha(p) for p in (base / 'normal').glob('*.go')}
assert expected['harness'] == {p.name: sha(p) for p in bench.glob('*.go')}
for name, digest in expected['fixtures'].items():
    assert sha(bench / name) == digest, name
if args.prepare_only:
    raise SystemExit(0)
sources = {name: {p.name: sha(p) for p in (base / name).glob('*.go')}
           for name in ['normal', 'baseline', 'fixed', 'typed']}
subprocess.run(['go', 'test', '-mod=mod', '-run', '^TestDecodeDiagnosis$', '.', '-timeout=1m'], cwd=bench, check=True)
binary = base / 'diagnosis.test'
subprocess.run(['go', 'test', '-mod=mod', '-c', '-o', str(binary), '.'], cwd=bench, check=True)
rows, commands = [], []
for round_index in range(5):
    command = [str(binary), '-test.run=^TestDecodeDiagnosis$',
               '-test.bench=^Benchmark(DecodeDiagnosis|ConstructDiagnosis)$',
               '-test.benchtime=60000x', '-test.benchmem', '-test.timeout=55s']
    env = {**os.environ, 'GOMAXPROCS': '1', 'GSR_AB_REVERSE': str(round_index % 2)}
    result = subprocess.run(command, cwd=bench, env=env, text=True, capture_output=True, timeout=60)
    (base / ('round-' + str(round_index) + '.txt')).write_text(result.stdout + result.stderr)
    result.check_returncode()
    commands.append({'args': command, 'reverse': round_index % 2})
    for line in result.stdout.splitlines():
        if not line.startswith('Benchmark'):
            continue
        fields = line.split()
        rows.append({'round': round_index, 'operation': fields[0],
                     'ns': float(fields[fields.index('ns/op') - 1]),
                     'bytes': int(fields[fields.index('B/op') - 1]),
                     'allocs': int(fields[fields.index('allocs/op') - 1])})
    print('decode diagnosis round', round_index, 'done', flush=True)
assert sources == {name: {p.name: sha(p) for p in (base / name).glob('*.go')} for name in sources}
summary = []
for operation in sorted({row['operation'] for row in rows}):
    values = [row for row in rows if row['operation'] == operation]
    summary.append({'operation': operation, **{key: statistics.median(row[key] for row in values)
                                              for key in ['ns', 'bytes', 'allocs']}})
metadata = {'baseline': 'd66644a', 'goVersion': subprocess.check_output(['go', 'version'], text=True).strip(),
            'binarySha256': sha(binary), 'sources': sources, 'goMod': mod, 'gomaxprocs': 1,
            'iterations': 60000, 'commands': commands, 'rows': rows, 'summary': summary,
            'scope': 'Fixed three-int64 Item diagnostic; not a generic production decoder'}
(base / 'diagnosis.json').write_text(json.dumps(metadata, ensure_ascii=False, indent=2) + '\n')
