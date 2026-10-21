"""整数记录数组布局分段消费实验，保留完整首遍与独占输出，不修改生产库。"""
import argparse,hashlib,io,json,os,platform,subprocess,tarfile,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--library',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();out.mkdir(exist_ok=False);here=Path(__file__).resolve().parent;root=here.parents[2];commands=[]
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(cmd,cwd,env=None,timeout=120):
 start=time.monotonic();r=subprocess.run(cmd,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=timeout);commands.append(dict(args=cmd,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 return r.stdout+r.stderr
lib=out/'gosproto';lib.mkdir();commit=run(['git','rev-parse','2e1d44b'],a.library).strip()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd=a.library))) as t:t.extractall(lib,filter='data')
s=(lib/'encode.go').read_text();(out/'original-decode.go.txt').write_text(s)
start=s.index('func encodeFieldInto(');end=s.index('// encodeScalarInto',start)
body=s[start:end];baseline=body.replace('func encodeFieldInto(','func experimentStringBaseline(',1);clone=body.replace('func encodeFieldInto(','func experimentStringClone(',1);candidate=body.replace('func encodeFieldInto(','func experimentStringCandidate(',1)
marker='\t\t\tfor index := 0; index < value.Len(); index++ {\n\t\t\t\tvar length int'
assert candidate.count(marker)==1
candidate=candidate.replace(marker,'\t\t\tif integers && value.Len() >= 8 {return experimentIntegerArray(field,value,output,layout,childBinding,pointers)}\n'+marker,1)
wrapper='func encodeFieldInto(field *SprotoField, value reflect.Value, output []byte, layout *encodeLayout, trusted bool) int {return experimentStringSelected(field,value,output,layout,trusted)}\n\n'
(lib/'encode.go').write_text((s[:start]+wrapper+s[end:]).replace('\n\t"unsafe"',''))
record=(lib/'encode_integer_record.go').read_text();record=record[record.index('func encodeIntegerRecordInto('):].replace('func encodeIntegerRecordInto(','func experimentIntegerRecord(',1).replace('layout *encodeLayout) int {\n\tcount := layout.next()','count int) int {',1)
selector="""
var experimentStringSelected func(*SprotoField,reflect.Value,[]byte,*encodeLayout,bool) int
func init(){experimentStringSelected=experimentStringBaseline}
// ExperimentStringMode 仅在实验归档中选择编码数组路径。
func ExperimentStringMode(mode string) {
 switch mode {
 case "baseline": experimentStringSelected=experimentStringBaseline
 case "clone": experimentStringSelected=experimentStringClone
 case "candidate": experimentStringSelected=experimentStringCandidate
 default:panic("invalid experiment mode")
 }
}
"""
(lib/'string_experiment.go').write_text('package sproto\n\nimport ("reflect";"unsafe";"math")\n\n'+baseline+'\n'+clone+'\n'+candidate+record+(here/'helper.go.txt').read_text()+selector)
(lib/'string_experiment_test.go').write_bytes((here/'contract-test.go.txt').read_bytes())
(lib/'external_experiment_test.go').write_bytes((here/'external-contract-test.go.txt').read_bytes())
h=out/'harness';h.mkdir()
for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
(h/'experiment_test.go').write_bytes((here/'experiment.go.txt').read_bytes());(h/'go.sum').write_bytes((root/'go.sum').read_bytes())
(h/'go.mod').write_text('module unpackinprocess\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n')
env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
run(['gofmt','-w','encode.go','string_experiment.go','string_experiment_test.go','external_experiment_test.go'],lib)
run(['gofmt','-w','experiment_test.go'],h)
run(['go','test','-race','-gcflags=all=-d=checkptr=2','-run','^(TestLayoutSegments(External)?Contracts|TestUnknownField|TestLimits|TestNativePointerDecode|TestSchemaEvolution|TestIntegerRecordArrayBudgetsAndTags)','-timeout=2m','.'],lib,env)
run(['go','test','-mod=mod','-run','^TestRepresentativeFixtures$','-timeout=30s','.'],h,env)
run(['go','vet','-mod=mod','.'],h,env)
binary=out/'experiment.test';run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h,env)
(out/'modules.txt').write_text(run(['go','list','-m','-json','github.com/cnooljaw/gosproto'],h,env))
(out/'assembly.txt').write_text(run(['go','tool','objdump','-s',r'github.com/cnooljaw/gosproto\.(experimentStringBaseline|experimentStringClone|experimentStringCandidate|experimentIntegerArray|experimentIntegerRecord)',str(binary)],h))
cases=[(n,op) for n in ['addressbook','game_small','game_large','int32_edges','echo_ascii4k','records7','records8','records33','records128','prefix31'] for op in ['encode','pencode']]

(out/'assembly.txt').write_text('\n'.join(l.rstrip() for l in (out/'assembly.txt').read_text().splitlines())+'\n')
for name,op in cases:
 run([str(binary),'-test.run=^TestStringInProcessExperiment$','-test.timeout=30s'],h,{**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':'pass1','GSR_PREFLIGHT':'1'},timeout=35)
print('all20 fixture baseline/candidate preflights passed',flush=True)

plan=dict(commit=commit,go=run(['go','version'],root).strip(),platform=platform.platform(),loadBefore=os.getloadavg(),cases=cases,modes=['self','clone','pass1','pass2'],rounds=6,quartetsPerRound=4,targetBlockMs=20,warmupMsPerArm=50,order='alternate ABBA/BAAB each quartet/round',sources={str(f.relative_to(out)):sha(f) for d in ['gosproto','harness'] for f in (out/d).glob('*.go')},fixtures={n:sha(h/n) for n in ['cases.json','representative.json']},generated={str(f.relative_to(root)):sha(f) for n in ['examples/websocket-gate/gen','examples/websocket-gate/benchmarks/gen','protocol/gen'] for f in (root/n).rglob('*.go')},binarySha256=sha(binary),formalDependency='unchanged v0.4.0; experiment adapters exist only in archive')
(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('three array encoders: boundaries/race/checkptr and fixture/vet/build passed',flush=True)
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
