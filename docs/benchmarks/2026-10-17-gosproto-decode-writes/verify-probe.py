"""验证 --prepare-only 建立的 typed-store 副本；不修改正式库。"""
from pathlib import Path
import argparse
import hashlib
import json
import subprocess

parser = argparse.ArgumentParser()
parser.add_argument('--prepared', required=True)
args = parser.parse_args()
base = Path(args.prepared).resolve()
report = Path(__file__).resolve().parent
expected = json.loads((report / 'typed-store.json').read_text())['goSource']
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
assert expected == {p.name: sha(p) for p in (base / 'candidate').glob('*.go')}, '请使用 typed-store 的完整原始副本'
target = base / 'external-validation'
if target.exists():
    raise SystemExit('external-validation 已存在，请使用新的复现目录')
target.mkdir()
mod = 'module storeprobe\n\ngo 1.24.0\nrequire (\n github.com/cnooljaw/gosproto v0.4.0\n github.com/cnooljaw/gosproto-before v0.0.0\n)\n'
mod += 'replace github.com/cnooljaw/gosproto => ' + str(base / 'candidate') + '\n'
mod += 'replace github.com/cnooljaw/gosproto-before => ' + str(base / 'ab-baseline') + '\n'
(target / 'go.mod').write_text(mod)
for name in ['inline_test.go', 'gc_test.go']:
    (target / name).write_bytes((report / (name + '.txt')).read_bytes())
subprocess.run(['go', 'test', '-mod=mod', '-race', '-gcflags=all=-d=checkptr=2', '-v', '.', '-timeout=1m'], cwd=target, check=True)
