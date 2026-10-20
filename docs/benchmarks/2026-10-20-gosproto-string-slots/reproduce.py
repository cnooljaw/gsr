"""同模块同进程字符串数组Decode实验，保留完整首遍与独占输出，不修改生产库。"""
import argparse,hashlib,io,json,os,platform,subprocess,tarfile,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--library',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();out.mkdir(exist_ok=False);here=Path(__file__).resolve().parent;root=here.parents[2];commands=[]
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(cmd,cwd,env=None,timeout=120):
 start=time.monotonic();r=subprocess.run(cmd,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=timeout);commands.append(dict(args=cmd,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 return r.stdout+r.stderr
lib=out/'gosproto';lib.mkdir();commit=run(['git','rev-parse','50c69e1'],a.library).strip()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd=a.library))) as t:t.extractall(lib,filter='data')
s=(lib/'decode.go').read_text();body=s[s.index('func decodeStringSlice('):s.index('func decodeStruct(')]
clone=body.replace('func decodeStringSlice(','func experimentStringClone(',1)
candidate=body.replace('func decodeStringSlice(','func experimentStringCandidate(',1)
old='\tresult := reflect.MakeSlice(value.Type(), count, count)\n'
new='\tresult := reflect.MakeSlice(value.Type(), count, count)\n\tvar slots []string\n\tif count > 0 && value.Type().Elem() == reflect.TypeFor[string]() {\n\t\tslots = unsafe.Slice((*string)(result.UnsafePointer()), count)\n\t}\n'
assert candidate.count(old)==1
candidate=candidate.replace(old,new).replace('\t\tresult.Index(index).SetString(string(content))','\t\tif slots != nil {\n\t\t\tslots[index] = string(content)\n\t\t} else {\n\t\t\tresult.Index(index).SetString(string(content))\n\t\t}')
selector="""
// ExperimentStringSelector 仅在实验归档中切换解码器，选择动作不计入时间块。
func ExperimentStringSelector(message any) func(string) {
 schema,err:=GetSprotoType(reflect.TypeOf(message).Elem())
 if err!=nil {panic(err)}
 seen:=make(map[*SprotoType]bool)
 fields:=[]*SprotoField{}
 var visit func(*SprotoType)
 visit=func(st *SprotoType) {
  if st==nil || seen[st] {return}
  seen[st]=true
  for _,f:=range st.Fields {
   if f.Array && f.Wire==WireBytesName && f.valueType.Kind()==reflect.Slice && f.valueType.Elem().Kind()==reflect.String {fields=append(fields,f)}
   visit(f.st)
  }
 }
 visit(schema)
 return func(mode string) {
  fn:=decoder(decodeStringSlice)
  switch mode {
  case "baseline":
  case "clone":fn=experimentStringClone
  case "candidate":fn=experimentStringCandidate
  default:panic("invalid experiment decoder")
  }
  for _,f:=range fields {f.dec=fn}
 }
}
"""
(lib/'string_experiment.go').write_text('package sproto\n\nimport ("reflect";"unsafe")\n\n'+clone+'\n'+candidate+selector)
test=(here/'strings-test.go.txt').read_text().replace('func TestStringArraySlotOwnership(t *testing.T) {','func checkStringArrayOwnership(t *testing.T, mode string) {')
test=test.replace('  for _, sample:=range cases {','  for _, sample:=range cases {\n   selectMode:=ExperimentStringSelector(sample.message)\n   selectMode(mode)\n   defer selectMode("baseline")')
test+='\n// 验证三种实验decoder的原生、命名切片和命名元素均保持所有权及预算契约。\nfunc TestStringSlotsExperiment(t *testing.T) {for _,mode:=range []string{"baseline","clone","candidate"} {t.Run(mode,func(t *testing.T){checkStringArrayOwnership(t,mode)})}}\n'
(lib/'string_experiment_test.go').write_text(test)
h=out/'harness';h.mkdir()
for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
(h/'experiment_test.go').write_bytes((here/'experiment.go.txt').read_bytes());(h/'go.sum').write_bytes((root/'go.sum').read_bytes())
(h/'go.mod').write_text('module unpackinprocess\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n')
env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
run(['gofmt','-w','string_experiment.go','string_experiment_test.go'],lib)
run(['gofmt','-w','experiment_test.go'],h)
run(['go','test','-race','-gcflags=all=-d=checkptr=2','-run','^(TestStringSlotsExperiment|TestDecode|TestStructSliceDecode)','-timeout=2m','.'],lib,env)
run(['go','test','-mod=mod','-run','^TestRepresentativeFixtures$','-timeout=30s','.'],h,env)
run(['go','vet','-mod=mod','.'],h,env)
binary=out/'experiment.test';run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h,env)
(out/'modules.txt').write_text(run(['go','list','-m','-json','github.com/cnooljaw/gosproto'],h,env))
(out/'assembly.txt').write_text(run(['go','tool','objdump','-s',r'github.com/cnooljaw/gosproto\.(decodeStringSlice|experimentStringClone|experimentStringCandidate)',str(binary)],h))
cases=[(n,op) for n in ['addressbook','game_small','game_large','int32_edges','echo_ascii4k'] for op in ['decode','pdecode']]+[(n,op) for n in ['strings_0','strings_3','strings_128','strings_512','outer128','element128'] for op in ['decode','pdecode']]
plan=dict(commit=commit,go=run(['go','version'],root).strip(),platform=platform.platform(),loadBefore=os.getloadavg(),cases=cases,modes=['self','clone','pass1','pass2'],rounds=6,quartetsPerRound=4,targetBlockMs=20,warmupMsPerArm=50,order='alternate ABBA/BAAB each quartet/round',sources={str(f.relative_to(out)):sha(f) for d in ['gosproto','harness'] for f in (out/d).glob('*.go')},fixtures={n:sha(h/n) for n in ['cases.json','representative.json']},generated={str(f.relative_to(root)):sha(f) for n in ['examples/websocket-gate/gen','examples/websocket-gate/benchmarks/gen','protocol/gen'] for f in (root/n).rglob('*.go')},binarySha256=sha(binary),formalDependency='unchanged v0.4.0; experiment adapters exist only in archive')
(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('three decoders: ownership/race/checkptr and fixture/vet/build passed',flush=True)
for mode in plan['modes']:
 for name,op in cases:
  output=out/(mode+'-'+name+'-'+op+'.json')
  run([str(binary),'-test.run=^TestStringSlotsInProcessExperiment$','-test.timeout=1m'],h,{**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':mode,'GSR_OUTPUT':str(output)},timeout=65)
  data=json.loads(output.read_text());assert (data['Name'],data['Operation'],data['Mode'])==(name,op,mode)
  print(mode,name,op,'done',flush=True)
for n,hsh in plan['sources'].items():assert sha(out/n)==hsh,n
for n,hsh in plan['fixtures'].items():assert sha(h/n)==hsh,n
for n,hsh in plan['generated'].items():assert sha(root/n)==hsh,n
assert sha(binary)==plan['binarySha256'];plan['loadAfter']=os.getloadavg();(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('all fingerprints unchanged',flush=True)
