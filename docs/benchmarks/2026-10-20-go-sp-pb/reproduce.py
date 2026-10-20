from pathlib import Path
import subprocess,json,hashlib,os,time,tarfile,io,platform
import argparse
parser=argparse.ArgumentParser()
parser.add_argument('--library',type=Path,required=True)
parser.add_argument('--output',type=Path,required=True)
args=parser.parse_args()
root=Path(__file__).resolve().parents[3];lib=args.library.resolve();out=args.output.resolve();out.mkdir(parents=True,exist_ok=False)
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
commands=[]
def run(args,cwd=root,env=None,timeout=120,file=None):
 start=time.monotonic();z=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=timeout);commands.append({'args':args,'cwd':str(cwd),'env':env or {},'exitCode':z.returncode,'seconds':time.monotonic()-start,'output':z.stdout+z.stderr});(out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
 if file:(out/file).write_text(z.stdout+z.stderr)
 if z.returncode:raise SystemExit(z.stdout+z.stderr)
 return z.stdout
baseline=subprocess.check_output(['git','rev-parse','af9169d'],cwd=lib).decode().strip();snapshot=out/'gosproto';snapshot.mkdir();blob=subprocess.check_output(['git','archive',baseline],cwd=lib)
with tarfile.open(fileobj=io.BytesIO(blob)) as t:t.extractall(snapshot,filter='data')
mod=out/'bench.mod';mod.write_text((root/'go.mod').read_text()+'\nreplace github.com/cnooljaw/gosproto => '+str(snapshot)+'\n');(out/'bench.sum').write_bytes((root/'go.sum').read_bytes())
files=[*list((root/'examples/websocket-gate/benchmarks').rglob('*.go')),*list((root/'examples/websocket-gate/benchmarks').glob('*.json')),*list((root/'examples/websocket-gate/gen/go').rglob('*.go')),*list((root/'examples/websocket-gate/gen/sproto').rglob('*.go')),*list((root/'protocol/gen/go').rglob('*.go'))]
files=[f for f in files if 'tools' not in f.parts]
frozen={str(f.relative_to(root)):sha(f) for f in files}
env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off'}
meta={'date':'2026-10-20','gsrHead':subprocess.check_output(['git','rev-parse','HEAD'],cwd=root).decode().strip(),'gsrStatus':subprocess.check_output(['git','status','--short'],cwd=root).decode(),'gosprotoCommit':baseline,'libraryWorktreeStatus':subprocess.check_output(['git','status','--short'],cwd=lib).decode(),'goSources':{f.name:sha(f) for f in sorted(snapshot.glob('*.go'))},'frozenInputs':frozen,'go':run(['go','version']).strip(),'os':platform.platform(),'cpu':run(['sysctl','-n','machdep.cpu.brand_string']).strip(),'loadBefore':os.getloadavg(),'parameters':{'count':5,'benchtime':'200ms','GOMAXPROCS':1,'GOGC':100,'GOMEMLIMIT':'off'},'formalDependency': 'gosproto v0.4.0 unchanged; temp modfile uses af9169d archive','scope':'Go only, original40 body +32 Echo payload/frame; no production RPC/Dispatch or TS comparison'}
binary=out/'benchmark.test';run(['go','test','-modfile='+str(mod),'-c','-o',str(binary),'./examples/websocket-gate/benchmarks'],env=env)
meta['binarySha256']=sha(binary);run(['go','version','-m',str(binary)],file='build-info.txt')
cwd=root/'examples/websocket-gate/benchmarks'
run([str(binary),'-test.run=^TestRepresentativeFixtures$','-test.v','-test.timeout=30s'],cwd,env,file='correctness.txt');print('build and fixture checks passed',flush=True)
for name in ['Representative','Protocol']:
 run([str(binary),'-test.run=^$','-test.bench=^Benchmark'+name+'Codec$','-test.benchmem','-test.benchtime=200ms','-test.count=5','-test.timeout=2m'],cwd,env,timeout=130,file=name.lower()+'.txt');print(name,'5 rounds complete',flush=True)
for f,h in frozen.items():assert sha(root/f)==h,f
assert {f.name:sha(f) for f in snapshot.glob('*.go')}==meta['goSources'];meta['loadAfter']=os.getloadavg();meta['inputsUnchanged']=True
for n in ['go.mod','go.sum']:assert (root/n).read_bytes()==subprocess.check_output(['git','show','HEAD:'+n],cwd=root)
(out/'environment.json').write_text(json.dumps(meta,indent=2)+'\n');print(out,flush=True)
