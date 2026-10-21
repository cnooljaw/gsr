"""同模块同进程消息头游标实验，保留完整首遍与独占输出，不修改生产库。"""
import argparse,hashlib,io,json,os,platform,subprocess,tarfile,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--library',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();out.mkdir(exist_ok=False);here=Path(__file__).resolve().parent;root=here.parents[2];commands=[]
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(cmd,cwd,env=None,timeout=120):
 start=time.monotonic();r=subprocess.run(cmd,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=timeout);commands.append(dict(args=cmd,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 return r.stdout+r.stderr
lib=out/'gosproto';lib.mkdir();commit=run(['git','rev-parse','5360fe0'],a.library).strip()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd=a.library))) as t:t.extractall(lib,filter='data')
meta=(lib/'meta.go').read_text();meta=meta.replace('nativeInt64Pointer bool','nativeInt64Pointer bool\n nativeStringPointer bool')
meta=meta.replace('sf.nativeInt64Pointer = f.Type == reflect.TypeFor[*int64]()','sf.nativeInt64Pointer = f.Type == reflect.TypeFor[*int64]()\n sf.nativeStringPointer = f.Type == reflect.TypeFor[*string]()')
meta=meta.replace('st.nativeInt64Pointer = st.nativeInt64Pointer || sf.nativeInt64Pointer','st.nativeInt64Pointer = st.nativeInt64Pointer || sf.nativeInt64Pointer\n st.nativeStringPointer = st.nativeStringPointer || sf.nativeStringPointer')
(lib/'meta.go').write_text(meta)
s=(lib/'decode.go').read_text();(out/'original-decode.go.txt').write_text(s)
start=s.index('func decodeMessage(');end=s.index('// Decode 解码',start)
body=s[start:end];baseline=body.replace('func decodeMessage(','func experimentStringBaseline(',1);clone=body.replace('func decodeMessage(','func experimentStringClone(',1);candidate=body.replace('func decodeMessage(','func experimentStringCandidate(',1)
candidate=candidate.replace('schema.nativeInt64Pointer && record.Type()', '(schema.nativeInt64Pointer || schema.nativeStringPointer) && record.Type()',1)
marker='\t\tif err := field.dec('
addition="""		if field.nativeStringPointer {
			result := new(string)
			*result = string(content)
			if !pointer.setString(schema, field, result) {
				record.Field(field.index[0]).Set(reflect.ValueOf(result))
			}
			continue
		}
"""
assert candidate.count(marker)==1;candidate=candidate.replace(marker,addition+marker,1)
helper=(here/'helper.go.txt').read_text()
wrapper='func decodeMessage(chunk []byte, schema *SprotoType, value reflect.Value, budget *decodeBudget) (int,error) {return experimentStringSelected(chunk,schema,value,budget)}\n\n'
(lib/'decode.go').write_text(s[:start]+wrapper+s[end:])
selector="""
var experimentStringSelected = experimentStringBaseline
// ExperimentStringMode 只在实验归档中选择全递归parser，计时外调用。
func ExperimentStringMode(mode string) {
 switch mode {
 case "baseline": experimentStringSelected=experimentStringBaseline
 case "clone": experimentStringSelected=experimentStringClone
 case "candidate": experimentStringSelected=experimentStringCandidate
 default:panic("invalid header experiment mode")
 }
}
"""
(lib/'string_experiment.go').write_text('package sproto\n\nimport ("reflect";"unsafe")\n\n'+baseline+'\n'+clone+'\n'+candidate+helper+selector)
test=(here/'pointer-test.go.txt').read_text().replace('func TestStringPointerOwnership(t *testing.T) {','func checkStringPointerOwnership(t *testing.T) {')
test+='\n// 验证三种递归parser均保持字符串独占、元信息回退和失败原子性。\nfunc TestStringExperiment(t *testing.T){defer ExperimentStringMode("baseline");for _,mode:=range []string{"baseline","clone","candidate"}{ExperimentStringMode(mode);t.Run(mode,checkStringPointerOwnership)}}\n'
(lib/'string_experiment_test.go').write_text(test)
h=out/'harness';h.mkdir()
for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
(h/'experiment_test.go').write_bytes((here/'experiment.go.txt').read_bytes());(h/'go.sum').write_bytes((root/'go.sum').read_bytes())
(h/'go.mod').write_text('module unpackinprocess\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n')
env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
run(['gofmt','-w','meta.go','decode.go','string_experiment.go','string_experiment_test.go'],lib)
run(['gofmt','-w','experiment_test.go'],h)
run(['go','test','-race','-gcflags=all=-d=checkptr=2','-run','^(TestStringExperiment|TestUnknownField|TestLimits|TestNativePointerDecode|TestSchemaEvolution|TestIntegerRecordArrayBudgetsAndTags)','-timeout=2m','.'],lib,env)
run(['go','test','-mod=mod','-run','^TestRepresentativeFixtures$','-timeout=30s','.'],h,env)
run(['go','vet','-mod=mod','.'],h,env)
binary=out/'experiment.test';run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h,env)
(out/'modules.txt').write_text(run(['go','list','-m','-json','github.com/cnooljaw/gosproto'],h,env))
(out/'assembly.txt').write_text(run(['go','tool','objdump','-s',r'github.com/cnooljaw/gosproto\.(experimentStringBaseline|experimentStringClone|experimentStringCandidate)',str(binary)],h))
cases=[(n,op) for n in ['addressbook','game_small','game_large','int32_edges','echo_ascii4k','strings1','strings8','strings64','named64','mixed64','absent64','records_strings128','records_named128'] for op in ['decode','pdecode']]

for name,op in cases:
 run([str(binary),'-test.run=^TestStringInProcessExperiment$','-test.timeout=30s'],h,{**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':'pass1','GSR_PREFLIGHT':'1'},timeout=35)
print('all26 fixture baseline/candidate preflights passed',flush=True)

plan=dict(commit=commit,go=run(['go','version'],root).strip(),platform=platform.platform(),loadBefore=os.getloadavg(),cases=cases,modes=['self','clone','pass1','pass2'],rounds=6,quartetsPerRound=4,targetBlockMs=20,warmupMsPerArm=50,order='alternate ABBA/BAAB each quartet/round',sources={str(f.relative_to(out)):sha(f) for d in ['gosproto','harness'] for f in (out/d).glob('*.go')},fixtures={n:sha(h/n) for n in ['cases.json','representative.json']},generated={str(f.relative_to(root)):sha(f) for n in ['examples/websocket-gate/gen','examples/websocket-gate/benchmarks/gen','protocol/gen'] for f in (root/n).rglob('*.go')},binarySha256=sha(binary),formalDependency='unchanged v0.4.0; experiment adapters exist only in archive')
(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('three recursive parsers: boundaries/race/checkptr and fixture/vet/build passed',flush=True)
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
