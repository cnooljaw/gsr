"""同二进制静态调用树实验，保留完整首遍与独占输出，不修改生产库。"""
import argparse,hashlib,io,json,os,platform,re,subprocess,tarfile,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--order',choices=['012','120','201'],default='012');p.add_argument('--prepare-only',action='store_true');p.add_argument('--library',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();out.mkdir(exist_ok=False);here=Path(__file__).resolve().parent;root=here.parents[2];commands=[]
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(cmd,cwd,env=None,timeout=120):
 start=time.monotonic();r=subprocess.run(cmd,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=timeout);commands.append(dict(args=cmd,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 return r.stdout+r.stderr
lib=out/'gosproto';lib.mkdir();commit=run(['git','rev-parse','2e1d44b'],a.library).strip()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd=a.library))) as t:t.extractall(lib,filter='data')
candidateCommit=run(['git','rev-parse','451998e'],a.library).strip()
baseEncode=(lib/'encode.go').read_text();baseRecord=(lib/'encode_integer_record.go').read_text();limits=(lib/'limits.go').read_text()
finalEncode=run(['git','show',candidateCommit+':encode.go'],a.library);finalRecord=run(['git','show',candidateCommit+':encode_integer_record.go'],a.library)
(out/'original-encode.go.txt').write_text(baseEncode);(out/'original-limits.go.txt').write_text(limits);(out/'candidate-encode.go.txt').write_text(finalEncode);(out/'candidate-record.go.txt').write_text(finalRecord)
sharedTop=limits[limits.index('func encodeWithLimits('):limits.index('func encodedSize(')]
normalTop=sharedTop[:sharedTop.index('// encodeBufferWithLimits')].replace('func encodeWithLimits(','func ExperimentStaticNormal(',1)
trees=[];equivalence={}
for label,enc,record in [('Baseline',baseEncode,baseRecord),('Clone',baseEncode,baseRecord),('Candidate',finalEncode,finalRecord)]:
 message=enc[enc.index('func encodeMessageInto('):enc.index('func encodeFieldInto(')]
 field=enc[enc.index('func encodeFieldInto('):enc.index('// encodeScalarInto')]
 core=record[record.index('func encodeIntegerRecordInto('):]
 original=sharedTop+message+field+core
 names={'encodeWithLimits':'ExperimentStatic'+label,'encodeBufferWithLimits':'experiment'+label+'Buffer','encodeMessageInto':'experiment'+label+'Message','encodeFieldInto':'experiment'+label+'Field','encodeIntegerRecordInto':'experiment'+label+'Record','encodeIntegerRecordArrayInto':'experiment'+label+'Array'}
 transformed=original
 for name,replacement in names.items():transformed=re.sub(r'\b'+name+r'\b',replacement,transformed)
 restored=transformed
 for name,replacement in names.items():restored=re.sub(r'\b'+replacement+r'\b',name,restored)
 assert restored==original
 equivalence[label]={'originalSha256':hashlib.sha256(original.encode()).hexdigest(),'restoredSha256':hashlib.sha256(restored.encode()).hexdigest()}
 trees.append(transformed)
selector="""
var experimentStaticSelected func(interface{}, Limits, bool, []byte) ([]byte,error)
func init(){experimentStaticSelected=ExperimentStaticBaseline}
// ExperimentStringMode 只选择不接收layout指针的最外层入口，供既有契约逐模式验证。
func ExperimentStringMode(mode string) {
 switch mode {
 case "baseline": experimentStaticSelected=ExperimentStaticBaseline
 case "clone": experimentStaticSelected=ExperimentStaticClone
 case "candidate": experimentStaticSelected=ExperimentStaticCandidate
 default:panic("invalid mode")
 }
}
"""
start=limits.index('func encodeWithLimits(');end=limits.index('// encodeBufferWithLimits',start)
wrapper='func encodeWithLimits(message interface{}, limits Limits, packResult bool, prefix []byte) ([]byte,error) {return experimentStaticSelected(message,limits,packResult,prefix)}\n\n'
(lib/'limits.go').write_text(limits[:start]+wrapper+limits[end:])
(lib/'static_experiment.go').write_text('package sproto\n\nimport ("reflect";"unsafe";"math";"runtime")\n\n'+normalTop+'\n'+''.join(trees[int(i)] for i in a.order)+selector)
(lib/'static_experiment_test.go').write_bytes((here/'contract-test.go.txt').read_bytes());(lib/'external_experiment_test.go').write_bytes((here/'external-contract-test.go.txt').read_bytes())
(out/'equivalence.json').write_text(json.dumps(equivalence,indent=2)+'\n')
h=out/'harness';h.mkdir()
for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
(h/'experiment_test.go').write_bytes((here/'experiment.go.txt').read_bytes());(h/'go.sum').write_bytes((root/'go.sum').read_bytes())
(h/'go.mod').write_text('module unpackinprocess\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n')
env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
run(['gofmt','-w','limits.go','static_experiment.go','static_experiment_test.go','external_experiment_test.go'],lib)
run(['gofmt','-w','experiment_test.go'],h)
run(['go','test','-race','-gcflags=all=-d=checkptr=2','-run','^(TestLayoutSegments(External)?Contracts|TestUnknownField|TestLimits|TestNativePointerDecode|TestSchemaEvolution|TestIntegerRecordArrayBudgetsAndTags)','-timeout=2m','.'],lib,env)
run(['go','test','-mod=mod','-run','^(TestRepresentativeFixtures|TestStaticAllocationParity)$','-timeout=30s','.'],h,env)
run(['go','vet','-mod=mod','.'],h,env)
binary=out/'experiment.test';run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h,env)
(out/'modules.txt').write_text(run(['go','list','-m','-json','github.com/cnooljaw/gosproto'],h,env))
(out/'assembly.txt').write_text(run(['go','tool','objdump','-s',r'github.com/cnooljaw/gosproto\.(ExperimentStatic|experiment(Baseline|Clone|Candidate))',str(binary)],h))
cases=[(n,op) for n in ['game_small','game_large','int32_edges','records7','prefix31'] for op in ['encode','pencode']]

(out/'assembly.txt').write_text('\n'.join(l.rstrip() for l in (out/'assembly.txt').read_text().splitlines())+'\n')
for name,op in cases:
 run([str(binary),'-test.run=^TestStringInProcessExperiment$','-test.timeout=30s'],h,{**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':'pass1','GSR_PREFLIGHT':'1'},timeout=35)
print('all10 fixture baseline/candidate preflights passed',flush=True)

plan=dict(treeOrder=a.order,commit=commit,candidateCommit=candidateCommit,staticTreeSourceRestoration=equivalence,go=run(['go','version'],root).strip(),platform=platform.platform(),loadBefore=os.getloadavg(),cases=cases,modes=['self','clone','pass1','pass2'],rounds=6,quartetsPerRound=4,targetBlockMs=20,warmupMsPerArm=50,order='alternate ABBA/BAAB each quartet/round',sources={str(f.relative_to(out)):sha(f) for d in ['gosproto','harness'] for f in (out/d).glob('*.go')},fixtures={n:sha(h/n) for n in ['cases.json','representative.json']},generated={str(f.relative_to(root)):sha(f) for n in ['examples/websocket-gate/gen','examples/websocket-gate/benchmarks/gen','protocol/gen'] for f in (root/n).rglob('*.go')},binarySha256=sha(binary),formalDependency='unchanged v0.4.0; experiment adapters exist only in archive')
(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('static allocation-compatible call trees: boundaries/race/checkptr and fixture/vet/build passed',flush=True)
if a.prepare_only:print('prepared order '+a.order,flush=True);raise SystemExit
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
