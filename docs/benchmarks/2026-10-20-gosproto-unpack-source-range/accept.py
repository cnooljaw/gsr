"""在完整候选快照验收，不恢复用户删除的原库go.mod。"""
import argparse,hashlib,json,os,re,shutil,subprocess,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--experiment',type=Path,required=True);p.add_argument('--output',type=Path,required=True);p.add_argument('--prepare-only',action='store_true');a=p.parse_args();out=a.output.resolve();out.mkdir(exist_ok=False);experiment=a.experiment.resolve();here=Path(__file__).resolve().parent;root=here.parents[2];commands=[]
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
lib=out/'gosproto';shutil.copytree(experiment/'gosproto',lib,ignore=shutil.ignore_patterns('unpack_experiment.go','unpack_experiment_test.go'))
f=lib/'pack.go';text=f.read_text();start=text.index('func UnpackWithLimit(')
adapter=(experiment/'gosproto/unpack_experiment.go').read_text();body=adapter[adapter.index('func ExperimentUnpackCandidate('):].replace('ExperimentUnpackCandidate','UnpackWithLimit',1)
f.write_text(text[:start]+body)
strip=lambda s:re.sub(r'\s+','',re.sub(r'//[^\n]*','',s))
s=(lib/'pack.go').read_text();s=s[s.index('func UnpackWithLimit('):]
adapter=(experiment/'gosproto/unpack_experiment.go').read_text();adapter=adapter[adapter.index('func ExperimentUnpackCandidate('):].replace('ExperimentUnpackCandidate','UnpackWithLimit',1);assert strip(s)==strip(adapter)
fixed={p.name:sha(p) for p in lib.glob('*.go')}
if a.prepare_only:
 (out/'accepted-source.json').write_text(json.dumps(fixed,indent=2)+'\n');print('prepared');raise SystemExit
oracle=root/'tmp/gosproto-pack-payload/oracle';prov=json.loads((oracle/'provenance.json').read_text())
for n,r in prov['files'].items():assert sha(oracle/n)==r['sha256'],n
(out/'oracle-provenance.json').write_text(json.dumps(prov,indent=2)+'\n')
mod=out/'integration.mod';mod.write_text((root/'go.mod').read_text()+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n');(out/'integration.sum').write_bytes((root/'go.sum').read_bytes())
def run(cmd,cwd,env=None,timeout=120):
 start=time.monotonic();r=subprocess.run(cmd,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=timeout);commands.append(dict(args=cmd,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'acceptance.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 assert {p.name:sha(p) for p in lib.glob('*.go')}==fixed
 print(cmd[0]+' '+cmd[1]+' passed',flush=True)
env={'SPROTO_ORACLE_DIR':str(oracle),'GOPROXY':'off'}
run(['go','test','./...','-timeout=2m'],lib,env)
run(['go','test','-race','-gcflags=all=-d=checkptr=2','./...','-timeout=2m'],lib,{**env,'GOMAXPROCS':'2'})
run(['go','vet','./...'],lib,env)
run(['go','test','-run','^$','-fuzz','^FuzzPackRoundTrip$','-fuzztime=10s','-parallel=2','.','-timeout=1m'],lib,{'GOMAXPROCS':'2','GOPROXY':'off'},timeout=70)
run(['go','test','-modfile='+str(mod),'./protocol/sproto','./examples/websocket-gate/cmd/debugserver','./examples/websocket-gate/benchmarks','-timeout=2m'],root)
run(['npm','--prefix','examples/websocket-gate','run','test:sproto:integration'],root,{'GOFLAGS':'-modfile='+str(mod)})
(out/'accepted-source.json').write_text(json.dumps(fixed,indent=2)+'\n')
