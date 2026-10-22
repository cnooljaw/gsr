"""正常Git归档对照三步稳定字节展开，保留预检、稀疏/尾部回退及输出契约。"""
from pathlib import Path
import subprocess,io,tarfile,json,os,time,hashlib,sys
here=Path(__file__).resolve().parent;root=here.parents[3];out=Path(sys.argv[1]).resolve();out.mkdir(exist_ok=False);commands=[];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(args,cwd,env=None):
 t=time.monotonic();r=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=120);commands.append(dict(args=args,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-t,output=r.stdout+r.stderr));(here/'commands.json').write_text(json.dumps(commands,indent=2)+'\n');assert r.returncode==0,r.stdout+r.stderr;return r.stdout+r.stderr
plan={'commits':{},'binaries':{},'sources':{},'fixtures':{},'orders':[]};env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
lib=out/'gosproto';lib.mkdir();commit=run(['git','rev-parse','9b2ecfb'],'/private/tmp/gsr-gosproto-fork').strip()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd='/private/tmp/gsr-gosproto-fork'))) as t:t.extractall(lib,filter='data')
source=(lib/'pack.go').read_text();old=source[source.index('func UnpackWithLimit('):].replace('func UnpackWithLimit(','func ExperimentUnpackOriginal(',1)
(lib/'pack.go').write_bytes((here.parent/'candidate-pack.go.txt').read_bytes());(lib/'experiment.go').write_text('package sproto\nimport ("math/bits")\n'+old)
h=out/'harness';h.mkdir()
for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','unpack_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
(h/'experiment_test.go').write_bytes((here/'experiment.go.txt').read_bytes());(h/'go.sum').write_bytes((root/'go.sum').read_bytes())
(h/'go.mod').write_text('module spreadpairs\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n')
run(['gofmt','-w','pack.go','experiment.go'],lib);run(['gofmt','-w','experiment_test.go'],h);run(['go','test','-mod=mod','-run','^TestRepresentativeFixtures$','.'],h,env);binary=out/'pairs.test';run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h,env)
plan=dict(commit=commit,binarySha256=sha(binary),sources={str(f.relative_to(out)):sha(f) for d in [lib,h] for f in d.glob('*.go')},cases=[('int32_edges','unpack'),('int32_edges','pdecode'),('game_large','unpack'),('game_large','pdecode'),('nonzero6x256','unpack'),('nonzero7x256','unpack'),('nonzero1x256','unpack'),('single' if False else 'zero32','unpack')],modes=['self','pass1','pass2'])
for mode in plan['modes']:
 for name,op in plan['cases']:
  output=out/f'{mode}-{name}-{op}.json'
  run([str(binary),'-test.run=^TestStringInProcessExperiment$','-test.timeout=1m'],h,{**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':mode,'GSR_OUTPUT':str(output)})
  (here/output.name).write_bytes(output.read_bytes());print(mode,name,op,'done',flush=True)
for n,v in plan['sources'].items():assert sha(out/n)==v
assert sha(binary)==plan['binarySha256'];plan['hashesVerifiedAfter']=True;(here/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('all fingerprints verified',flush=True)
