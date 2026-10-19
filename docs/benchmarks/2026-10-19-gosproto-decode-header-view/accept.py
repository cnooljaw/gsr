"""固定两字节header视图候选收尾验收；正式依赖不变，不重测未改的Pack随机fuzz。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import time

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--library', type=Path, required=True)
parser.add_argument('--output', type=Path, required=True)
args=parser.parse_args()
here=Path(__file__).resolve().parent
root=here.parents[2]
lib=args.library.resolve()
output=args.output.resolve()
output.mkdir(exist_ok=False,parents=True)
commands=[]
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
sources=lambda p:{f.name:sha(f) for f in sorted(p.glob('*.go'))}
fixed=sources(lib)
assert fixed==sources(root/'tmp/gosproto-decode-header-view-measured/pass1/candidate')
oracle=root/'tmp/gosproto-pack-payload/oracle'
provenance=json.loads((oracle/'provenance.json').read_text())
for n,record in provenance['files'].items():
    assert sha(oracle/n)==record['sha256'],n
(output/'oracle-provenance.json').write_text(json.dumps(provenance,indent=2)+'\n')
mod=output/'gsr.mod'
mod.write_text((root/'go.mod').read_text()+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n')
# 临时modfile复用正式sum，不改变仓库依赖文件。
(output/'gsr.sum').write_bytes((root/'go.sum').read_bytes())

def run(command,cwd,env=None,timeout=120):
    start=time.monotonic()
    result=subprocess.run(command,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=timeout)
    commands.append({'args':command,'cwd':str(cwd),'env':env or {},'exitCode':result.returncode,'elapsedSeconds':time.monotonic()-start,'output':result.stdout+result.stderr})
    (output/'acceptance.json').write_text(json.dumps(commands,indent=2)+'\n')
    if result.returncode:raise SystemExit(result.stdout+result.stderr)
    assert sources(lib)==fixed,'production Go changed during acceptance'
    print(command[0]+' '+command[1]+' passed',flush=True)

run(['go','test','./...','-timeout=2m'],lib,{'SPROTO_ORACLE_DIR':str(oracle)})
run(['go','test','-race','-gcflags=all=-d=checkptr=2','./...','-timeout=2m'],lib,{'SPROTO_ORACLE_DIR':str(oracle),'GOMAXPROCS':'2'})
run(['go','vet','./...'],lib)
run(['go','test','-run','^$','-fuzz','^FuzzCodecRoundTrip$','-fuzztime=10s','-parallel=2','.','-timeout=1m'],lib,{'GOMAXPROCS':'2'},timeout=70)
run(['go','test','-run','^$','-fuzz','^FuzzDecode$','-fuzztime=10s','-parallel=2','.','-timeout=1m'],lib,{'GOMAXPROCS':'2'},timeout=70)
run(['go','test','-modfile='+str(mod),'./protocol/sproto','./examples/websocket-gate/cmd/debugserver','./examples/websocket-gate/benchmarks','-timeout=2m'],root)
run(['npm','--prefix','examples/websocket-gate','run','test:sproto:integration'],root,{'GOFLAGS':'-modfile='+str(mod)})
