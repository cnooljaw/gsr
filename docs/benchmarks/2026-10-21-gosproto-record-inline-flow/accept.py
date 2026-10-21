"""在正常生产入口验收已计时的整数记录候选，不修改实际库。"""
from pathlib import Path
import io,json,os,subprocess,tarfile,time,hashlib,sys
here=Path(__file__).resolve().parent;root=here.parents[2];out=Path(sys.argv[1]).resolve();out.mkdir(exist_ok=False);lib=out/'candidate';lib.mkdir();commands=[]
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
plan=json.loads((here/'plan.json').read_text())
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',plan['commit']],cwd='/private/tmp/gsr-gosproto-fork'))) as t:t.extractall(lib,filter='data')
(lib/'encode_integer_record.go').write_bytes((here/'candidate-record.go.txt').read_bytes())
def run(args,cwd,env=None,timeout=120):
 start=time.monotonic();p=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=timeout);commands.append(dict(args=args,cwd=str(cwd),env=env or {},exitCode=p.returncode,seconds=time.monotonic()-start,output=p.stdout+p.stderr));(here/'acceptance.json').write_text(json.dumps(commands,indent=2)+'\n');assert p.returncode==0,p.stdout+p.stderr;print('passed',args[:3],flush=True)
run(['gofmt','-w','encode_integer_record.go'],lib)
oracle=root/'tmp/gosproto-pack-payload/oracle';prov=json.loads((oracle/'provenance.json').read_text())
for n,r in prov['files'].items():assert sha(oracle/n)==r['sha256']
(here/'oracle-provenance.json').write_text(json.dumps(prov,indent=2)+'\n')
env={'GOPROXY':'off','SPROTO_ORACLE_DIR':str(oracle)}
run(['go','test','./...','-timeout=2m'],lib,env)
run(['go','test','-race','-gcflags=all=-d=checkptr=2','./...','-timeout=2m'],lib,{**env,'GOMAXPROCS':'2'})
run(['go','vet','./...'],lib,env)
for target in ['FuzzCodecRoundTrip','FuzzDecode']:run(['go','test','-run','^$','-fuzz','^'+target+'$','-fuzztime=10s','-parallel=2','.','-timeout=1m'],lib,{'GOPROXY':'off','GOMAXPROCS':'2'},timeout=70)
mod=out/'integration.mod';mod.write_text((root/'go.mod').read_text()+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n');(out/'integration.sum').write_bytes((root/'go.sum').read_bytes())
run(['go','test','-modfile='+str(mod),'./protocol/sproto','./examples/websocket-gate/cmd/debugserver','./examples/websocket-gate/benchmarks','-timeout=2m'],root)
run(['npm','--prefix','examples/websocket-gate','run','test:sproto:integration'],root,{'GOFLAGS':'-modfile='+str(mod)})
(here/'accepted-source.json').write_text(json.dumps({p.name:sha(p) for p in lib.glob('*.go')},indent=2)+'\n')
print('normal-entry acceptance passed',flush=True)
