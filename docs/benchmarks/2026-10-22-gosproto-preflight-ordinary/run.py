"""正常归档对照普通word固定预算预检，不改恢复、错误顺序或预算。"""
from pathlib import Path
import subprocess,io,tarfile,json,os,time,hashlib,sys
here=Path(__file__).resolve().parent;root=here.parents[2];out=Path(sys.argv[1]).resolve();out.mkdir(exist_ok=False);commands=[];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(args,cwd,env=None):
 t=time.monotonic();r=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=120);commands.append(dict(args=args,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-t,output=r.stdout+r.stderr));(here/'commands.json').write_text(json.dumps(commands,indent=2)+'\n');assert r.returncode==0,r.stdout+r.stderr;return r.stdout+r.stderr
plan={'commits':{},'binaries':{},'sources':{},'fixtures':{},'orders':[]};env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
lib=out/'gosproto';lib.mkdir();commit=run(['git','rev-parse','92a155a'],'/private/tmp/gsr-gosproto-fork').strip()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd='/private/tmp/gsr-gosproto-fork'))) as t:t.extractall(lib,filter='data')
source=(lib/'pack.go').read_text();old=source[source.index('func UnpackWithLimit('):].replace('func UnpackWithLimit(','func ExperimentUnpackOriginal(',1)

candidate=source.replace('\t\theader := remaining[0]\n', '\t\theader := remaining[0]\n'+'\t\t// 普通 word 独立完成固定八字节预算检查，避免与零段/FF尺寸合流。\n\t\tif header != 0 && header != 0xff {\n\t\t\tconsumed := 1 + bits.OnesCount8(header)\n\t\t\tif consumed > len(remaining) {\n\t\t\t\treturn nil, ErrUnpack\n\t\t\t}\n\t\t\tif maxBytes-size < 8 {\n\t\t\t\treturn nil, ErrLimit\n\t\t\t}\n\t\t\tsize += 8\n\t\t\tremaining = remaining[consumed:]\n\t\t\tcontinue\n\t\t}\n',1)
candidate=candidate.replace('\t\t} else {\n\t\t\tconsumed += bits.OnesCount8(header)\n\t\t}\n','\t\t}\n',1)
assert candidate!=source
(lib/'pack.go').write_text(candidate);(lib/'experiment.go').write_text('package sproto\nimport ("math/bits")\n'+old)
(lib/'preflight_cap_test.go').write_bytes((here/'comparison.go.txt').read_bytes())
run(['go','test','-race','-gcflags=all=-d=checkptr=2','-run','^TestPreflightCapComparison$','.','-timeout=2m'],lib,env)
h=out/'harness';h.mkdir()
for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','unpack_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
(h/'experiment_test.go').write_bytes((here/'experiment.go.txt').read_bytes());(h/'go.sum').write_bytes((root/'go.sum').read_bytes())
(h/'go.mod').write_text('module spreadpairs\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n')
run(['gofmt','-w','pack.go','experiment.go'],lib);run(['gofmt','-w','experiment_test.go'],h);run(['go','test','-mod=mod','-run','^TestRepresentativeFixtures$','.'],h,env);binary=out/'pairs.test';run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h,env)
plan=dict(commit=commit,binarySha256=sha(binary),sources={str(f.relative_to(out)):sha(f) for d in [lib,h] for f in [*d.glob('*.go'),*d.glob('*.s')]},cases=[('int32_edges','unpack'),('int32_edges','pdecode'),('game_large','unpack'),('game_large','pdecode'),('nonzero6x256','unpack'),('nonzero7x256','unpack'),('nonzero1x256','unpack'),('zero32','unpack'),('addressbook','unpack'),('addressbook','pdecode'),('ascii4096','unpack')],modes=['self','pass1','pass2'])
for mode in plan['modes']:
 for name,op in plan['cases']:
  output=out/f'{mode}-{name}-{op}.json'
  run([str(binary),'-test.run=^TestStringInProcessExperiment$','-test.timeout=1m'],h,{**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':mode,'GSR_OUTPUT':str(output)})
  (here/output.name).write_bytes(output.read_bytes());print(mode,name,op,'done',flush=True)
for n,v in plan['sources'].items():assert sha(out/n)==v
assert sha(binary)==plan['binarySha256'];plan['hashesVerifiedAfter']=True;(here/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('all fingerprints verified',flush=True)
