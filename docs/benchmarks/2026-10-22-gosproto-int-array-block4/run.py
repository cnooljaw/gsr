"""同模块同进程原生整数数组四项展开实验，保留预算与独占输出，不修改生产库。"""
import argparse,hashlib,io,json,os,platform,subprocess,tarfile,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--library',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();out.mkdir(exist_ok=False);here=Path(__file__).resolve().parent;root=here.parents[2];commands=[]
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(cmd,cwd,env=None,timeout=120):
 start=time.monotonic();r=subprocess.run(cmd,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=timeout);commands.append(dict(args=cmd,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 return r.stdout+r.stderr
lib=out/'gosproto';lib.mkdir();commit=run(['git','rev-parse','9048cfa'],a.library).strip()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd=a.library))) as t:t.extractall(lib,filter='data')
s=(lib/'decode.go').read_text();start=s.index('func decodeIntSlice(');end=s.index('// countChunks',start)
body=s[start:end];(out/'original-decode.go.txt').write_text(body)
baseline=body.replace('func decodeIntSlice(','func experimentStringBaseline(',1);clone=body.replace('func decodeIntSlice(','func experimentStringClone(',1)
candidate=body.replace('func decodeIntSlice(','func experimentStringCandidate(',1)
startLoop=candidate.index('\t\tif width == 4 {');endLoop=candidate.index('\t\t*value.Addr()',startLoop)
loops=''
for width in [4,8]:
 prefix='\t\tif width == 4 {\n' if width==4 else '\t\t} else {\n'
 loops+=prefix+'\t\t\tindex := 0\n\t\t\tfor ; index < len(result)-3; index += 4 {\n\t\t\t\tvalues := result[index:index+4:index+4]\n'+f'\t\t\t\tblock := data[index*{width}:index*{width}+{width*4}:index*{width}+{width*4}]\n'
 for j in range(4):
  expr=f'readUint{width*8}(block[{j*width}:{(j+1)*width}:{(j+1)*width}])'
  value='int64(int32('+expr+'))' if width==4 else 'int64('+expr+')'
  loops+=f'\t\t\t\tvalues[{j}] = {value}\n'
 loops+='\t\t\t}\n\t\t\tfor ; index < len(result); index++ {\n'
 expr=f'readUint{width*8}(data[index*{width}:index*{width}+{width}:index*{width}+{width}])'
 value='int64(int32('+expr+'))' if width==4 else 'int64('+expr+')'
 loops+=f'\t\t\t\tresult[index] = {value}\n\t\t\t}}\n'
loops+='\t\t}\n'
candidate=candidate[:startLoop]+loops+candidate[endLoop:]
(out/'candidate.go.txt').write_text(candidate)
wrapper='func decodeIntSlice(inline uint16, present bool, data []byte, field *SprotoField, value reflect.Value, budget *decodeBudget) error {return experimentStringSelected(inline,present,data,field,value,budget)}\n\n'
(lib/'decode.go').write_text(s[:start]+wrapper+s[end:])
selector="""
var experimentStringSelected = experimentStringBaseline
// ExperimentStringMode 仅在实验快照中选择整数数组 decoder，计时外调用。
func ExperimentStringMode(mode string) {
 switch mode {
 case "baseline": experimentStringSelected=experimentStringBaseline
 case "clone": experimentStringSelected=experimentStringClone
 case "candidate": experimentStringSelected=experimentStringCandidate
 default:panic("invalid experiment mode")
 }
}
"""
(lib/'string_experiment.go').write_text('package sproto\n\nimport ("reflect";"math")\n\n'+baseline+'\n'+clone+'\n'+candidate+selector)
(lib/'string_experiment_test.go').write_bytes((here/'boundary-test.go.txt').read_bytes())
h=out/'harness';h.mkdir()
for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
(h/'experiment_test.go').write_bytes((here/'experiment.go.txt').read_bytes());(h/'go.sum').write_bytes((root/'go.sum').read_bytes())
(h/'go.mod').write_text('module unpackinprocess\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n')
env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
run(['gofmt','-w','decode.go','string_experiment.go','string_experiment_test.go'],lib)
run(['gofmt','-w','experiment_test.go'],h)
run(['go','test','-race','-gcflags=all=-d=checkptr=2','-run','^(TestIntArrayBlockDifferential|TestUnknownField|TestLimits|TestNativePointerDecode|TestSchemaEvolution|TestIntegerRecordArrayBudgetsAndTags)','-timeout=2m','.'],lib,env)
run(['go','test','-mod=mod','-run','^TestRepresentativeFixtures$','-timeout=30s','.'],h,env)
run(['go','vet','-mod=mod','.'],h,env)
binary=out/'experiment.test';run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h,env)
(out/'modules.txt').write_text(run(['go','list','-m','-json','github.com/cnooljaw/gosproto'],h,env))
(out/'assembly.txt').write_text(run(['go','tool','objdump','-s',r'github.com/cnooljaw/gosproto\.(experimentStringBaseline|experimentStringClone|experimentStringCandidate)',str(binary)],h))
(out/'assembly.txt').write_text('\n'.join(x.rstrip() for x in (out/'assembly.txt').read_text().splitlines())+'\n')
cases=[(n,op) for n in ['int32_edges','game_large'] for op in ['decode','pdecode']]

for name,op in cases:
 run([str(binary),'-test.run=^TestStringInProcessExperiment$','-test.timeout=30s'],h,{**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':'pass1','GSR_PREFLIGHT':'1'},timeout=35)
print('all selected fixture baseline/candidate preflights passed',flush=True)

plan=dict(calibration='7 seed batches median -> 3 pilot batches, max3 iterations; baseline only; all batches retained',commit=commit,go=run(['go','version'],root).strip(),platform=platform.platform(),loadBefore=os.getloadavg(),cases=cases,modes=['self','clone','pass1','pass2'],rounds=6,quartetsPerRound=4,targetBlockMs=60,warmupMsPerArm=50,order='alternate ABBA/BAAB each quartet/round',sources={str(f.relative_to(out)):sha(f) for d in ['gosproto','harness'] for f in (out/d).glob('*.go')},fixtures={n:sha(h/n) for n in ['cases.json','representative.json']},generated={str(f.relative_to(root)):sha(f) for n in ['examples/websocket-gate/gen','examples/websocket-gate/benchmarks/gen','protocol/gen'] for f in (root/n).rglob('*.go')},binarySha256=sha(binary),formalDependency='unchanged v0.4.0; experiment adapters exist only in archive')
(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('three integer-array decoders: boundaries/race/checkptr and fixture/vet/build passed',flush=True)
for mode in plan['modes']:
 for name,op in cases:
  output=out/(mode+'-'+name+'-'+op+'.json')
  run([str(binary),'-test.run=^TestStringInProcessExperiment$','-test.timeout=1m'],h,{**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':mode,'GSR_OUTPUT':str(output)},timeout=65)
  data=json.loads(output.read_text());assert (data['Name'],data['Operation'],data['Mode'])==(name,op,mode)
  print(mode,name,op,'done',flush=True)
for n,hsh in plan['sources'].items():assert sha(out/n)==hsh,n
for n,hsh in plan['fixtures'].items():assert sha(h/n)==hsh,n
for n,hsh in plan['generated'].items():assert sha(root/n)==hsh,n
assert sha(binary)==plan['binarySha256'];plan['loadAfter']=os.getloadavg();(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('all fingerprints unchanged',flush=True)
