"""ARM64完整预检：同一冻结二进制包含原Go同源clone控制，不扣除控制值。"""
from pathlib import Path
import subprocess,io,tarfile,json,os,time,hashlib,sys
here=Path(__file__).resolve().parent;root=here.parents[3];out=Path(sys.argv[1]).resolve();out.mkdir(exist_ok=False);commands=[];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(args,cwd,env=None):
 t=time.monotonic();r=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=120);commands.append(dict(args=args,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-t,output=r.stdout+r.stderr));(here/'commands.json').write_text(json.dumps(commands,indent=2)+'\n');assert r.returncode==0,r.stdout+r.stderr;return r.stdout+r.stderr
plan={'commits':{},'binaries':{},'sources':{},'fixtures':{},'orders':[]};env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
lib=out/'gosproto';lib.mkdir();commit=run(['git','rev-parse','6b0430e'],'/private/tmp/gsr-gosproto-fork').strip()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd='/private/tmp/gsr-gosproto-fork'))) as t:t.extractall(lib,filter='data')
source=(lib/'pack.go').read_text()
source=(here.parent/'final/selected-pack.go.txt').read_text()
start=source.index('func packInto(');end=source.index('// zeroHeaderPrefix',start)
original=source[start:end].replace('func packInto(','func ExperimentPackInto(',1)
(lib/'experiment_pack.go').write_text('package sproto\nimport("encoding/binary";"math/bits")\n'+original)
(lib/'experiment_pack_clone.go').write_text('package sproto\nimport("encoding/binary";"math/bits")\n'+original.replace('func ExperimentPackInto(','func ExperimentPackIntoClone(',1))
limits=(lib/'limits.go').read_text();start=limits.index('func encodeWithLimits(');end=limits.index('// encodeBufferWithLimits',start)
helper=limits[start:end].replace('func encodeWithLimits(','func experimentEncodeWithLimits(',1).replace('packInto(', 'ExperimentPackInto(')
(lib/'experiment_encode.go').write_text('package sproto\n'+helper+'\nfunc ExperimentEncodePacked(message interface{})([]byte,error){return experimentEncodeWithLimits(message,Limits{},true,nil)}\nfunc ExperimentPack(src []byte)[]byte{return ExperimentPackInto(make([]byte,packedBufferSize(len(src))),src)}\n')
clone=(lib/'experiment_encode.go').read_text().replace('experimentEncodeWithLimits','experimentEncodeWithLimitsClone').replace('ExperimentPackInto','ExperimentPackIntoClone').replace('ExperimentEncodePacked','ExperimentEncodePackedClone').replace('func ExperimentPack(', 'func ExperimentPackClone(')
(lib/'experiment_encode_clone.go').write_text(clone)
for n in ['pack.go', 'pack_words_arm64.go', 'pack_words_other.go', 'pack_words_arm64.s', 'pack_words_arm64_test.go', 'pack_words_guard_arm64_test.go', 'pack_native_reference_arm64_test.go']:(lib/n).write_bytes((here/('selected-'+n+'.txt')).read_bytes())
run(['go','test','-race','-gcflags=all=-d=checkptr=2','-run','^(TestPackWords|TestEncodePackedWorkspace)','.','-timeout=2m'],lib,env)
h=out/'harness';h.mkdir()
for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','unpack_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
(h/'experiment_test.go').write_bytes((here/'experiment.go.txt').read_bytes());(h/'go.sum').write_bytes((root/'go.sum').read_bytes())
(h/'go.mod').write_text('module spreadpairs\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n')
run(['gofmt','-w','pack.go','experiment_pack.go','experiment_pack_clone.go','experiment_encode.go','experiment_encode_clone.go'],lib);run(['gofmt','-w','experiment_test.go'],h);run(['go','test','-mod=mod','-run','^TestRepresentativeFixtures$','.'],h,env);binary=out/'pairs.test';run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h,env)
plan=dict(commit=commit,binarySha256=sha(binary),sources={str(f.relative_to(out)):sha(f) for d in [lib,h] for f in [*d.glob('*.go'),*d.glob('*.s')]},cases=[('int32_edges','public'),('game_large','public'),('echo_ascii4k','public')],modes=['self','clone1','pass1','pass2','clone2'])
(here/'plan.json').write_text(json.dumps(plan,indent=2)+'\n')
for mode in plan['modes']:
 for name,op in plan['cases']:
  output=out/f'{mode}-{name}-{op}.json'
  run([str(binary),'-test.run=^TestStringInProcessExperiment$','-test.timeout=1m'],h,{**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':mode,'GSR_OUTPUT':str(output)})
  (here/output.name).write_bytes(output.read_bytes());print(mode,name,op,'done',flush=True)
for n,v in plan['sources'].items():assert sha(out/n)==v
assert sha(binary)==plan['binarySha256'];plan['hashesVerifiedAfter']=True;(here/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('all fingerprints verified',flush=True)
