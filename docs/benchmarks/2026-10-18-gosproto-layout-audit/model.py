"""复现增长模型；输入固定为第一阶段 raw，不反复修改生产或诊断程序。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--raw', type=Path, required=True)
parser.add_argument('--output', type=Path, required=True)
options = parser.parse_args()
here = Path(__file__).resolve().parent
output = options.output.resolve()
output.mkdir(parents=True, exist_ok=False)
digest = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
inputs = {str(p): digest(p) for p in [options.raw.resolve(), here/'model-plan.json', here/'growth-model.go.txt', here/'model.py']}
source = output/'growth-model.go'
source.write_bytes((here/'growth-model.go.txt').read_bytes())
commands = []


def run(args):
    started = time.monotonic()
    result = subprocess.run(args, cwd=output, env={**os.environ, 'GOMAXPROCS': '1'}, capture_output=True, text=True, timeout=60)
    commands.append({'args': args, 'cwd': str(output), 'env': {'GOMAXPROCS': '1'}, 'elapsedSeconds': round(time.monotonic()-started,3), 'exitCode': result.returncode, 'output': result.stdout+result.stderr})
    (output/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
    if result.returncode:
        raise SystemExit(result.stdout+result.stderr)


run(['gofmt','-w',str(source)])
run(['go','vet',str(source)])
binary = output/'growth-model'
run(['go','build','-o',str(binary),str(source)])
run([str(binary),str(options.raw.resolve()),str(output/'model.json')])
for p, expected in inputs.items():
    if digest(Path(p)) != expected:
        raise SystemExit('model input changed: '+p)
(output/'inputs.json').write_text(json.dumps({'inputs':inputs,'goSourceSha256':digest(source),'binarySha256':digest(binary),'modelSha256':digest(output/'model.json')},indent=2)+'\n')
print(commands[-1]['output'],end='')
