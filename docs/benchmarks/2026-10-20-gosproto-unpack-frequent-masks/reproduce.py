"""同模块同进程Unpack实验，保留完整首遍与独占输出，不修改生产库。"""
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
s=(lib/'pack.go').read_text();body=s[s.index('func UnpackWithLimit('):];assert body.count('func ')==1
clone=body.replace('func UnpackWithLimit(','func ExperimentUnpackClone(',1)
candidate=body.replace('func UnpackWithLimit(','func ExperimentUnpackCandidate(',1)
old_loop="""		used := 0
		for mask := header; ; {
			index := bits.TrailingZeros8(mask) & 7
			dst[index] = src[used]
			used++
			mask &= mask - 1
			if mask == 0 {
				break
			}
		}
		src = src[used:]"""
new_loop="""		used := bits.OnesCount8(header)
		mask := header
		for _, value := range src[:used:used] {
			index := bits.TrailingZeros8(mask) & 7
			dst[index] = value
			mask &= mask - 1
		}
		src = src[used:]"""
assert candidate.count(old_loop)==1
candidate=candidate.replace(old_loop,new_loop)
(lib/'unpack_experiment.go').write_text('package sproto\n\nimport "math/bits"\n\n'+clone+'\n'+candidate)
# 复用全部mask回归，三种入口均验证；只在实验归档增加适配函数。
test=(root/'docs/benchmarks/2026-10-20-gosproto-unpack-index/word-test.go.txt').read_text().replace('func TestUnpackWordViewBounds(t *testing.T) {','func checkWordBounds(t *testing.T, unpack func([]byte,int)([]byte,error)) {').replace('UnpackWithLimit(', 'unpack(')
test+='\n// 验证三个同进程实验入口均保持全部mask、预算、截断及独占结果契约。\nfunc TestUnpackExperimentBounds(t *testing.T) {\n for name,fn:=range map[string]func([]byte,int)([]byte,error){"baseline":UnpackWithLimit,"clone":ExperimentUnpackClone,"candidate":ExperimentUnpackCandidate} {t.Run(name,func(t *testing.T){checkWordBounds(t,fn)})}\n}\n'
(lib/'unpack_experiment_test.go').write_text(test)
h=out/'harness';h.mkdir()
for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
(h/'experiment_test.go').write_bytes((here/'experiment.go.txt').read_bytes());(h/'go.sum').write_bytes((root/'go.sum').read_bytes())
(h/'go.mod').write_text('module unpackinprocess\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n')
env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
run(['gofmt','-w','unpack_experiment.go','unpack_experiment_test.go'],lib)
run(['gofmt','-w','experiment_test.go'],h)
run(['go','test','-race','-gcflags=all=-d=checkptr=2','-run','^(TestUnpackExperimentBounds|TestPack|TestUnpack)','-timeout=2m','.'],lib,env)
run(['go','test','-mod=mod','-run','^TestRepresentativeFixtures$','-timeout=30s','.'],h,env)
run(['go','vet','-mod=mod','.'],h,env)
binary=out/'experiment.test';run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h,env)
(out/'modules.txt').write_text(run(['go','list','-m','-json','github.com/cnooljaw/gosproto'],h,env))
(out/'assembly.txt').write_text(run(['go','tool','objdump','-s',r'github.com/cnooljaw/gosproto\.(UnpackWithLimit|ExperimentUnpackClone|ExperimentUnpackCandidate)',str(binary)],h))
cases=[(n,op) for n in ['game_large','int32_edges'] for op in ['unpack','pdecode']]+[(n,'unpack') for n in ['density2','density4','density5','density6','zero4096','ff4096','short1']]+[(f'mask_{m:02x}','unpack') for m in [0x11,0x21,0xd1,0x99,0x9f,0xf9,0x1d,0x7e,0xc7,0xe2,0xfc]]

for name,op in cases:
 run([str(binary),'-test.run=^TestUnpackInProcessExperiment$','-test.timeout=30s'],h,{**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':'pass1','GSR_PREFLIGHT':'1'},timeout=35)
print('all fixtures preflight passed',flush=True)
plan=dict(commit=commit,go=run(['go','version'],root).strip(),platform=platform.platform(),loadBefore=os.getloadavg(),cases=cases,modes=['self','clone','pass1','pass2'],rounds=6,quartetsPerRound=4,targetBlockMs=20,warmupMsPerArm=50,order='alternate ABBA/BAAB each quartet/round',sources={str(f.relative_to(out)):sha(f) for d in ['gosproto','harness'] for f in (out/d).glob('*.go')},fixtures={n:sha(h/n) for n in ['cases.json','representative.json']},generated={str(f.relative_to(root)):sha(f) for n in ['examples/websocket-gate/gen','examples/websocket-gate/benchmarks/gen','protocol/gen'] for f in (root/n).rglob('*.go')},binarySha256=sha(binary),formalDependency='unchanged v0.4.0; experiment adapters exist only in archive')
(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('three implementations: bounds/race/checkptr and fixture/vet/build passed',flush=True)
for mode in plan['modes']:
 for name,op in cases:
  output=out/(mode+'-'+name+'-'+op+'.json')
  run([str(binary),'-test.run=^TestUnpackInProcessExperiment$','-test.timeout=1m'],h,{**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':mode,'GSR_OUTPUT':str(output)},timeout=65)
  data=json.loads(output.read_text());assert (data['Name'],data['Operation'],data['Mode'])==(name,op,mode)
  print(mode,name,op,'done',flush=True)
for n,hsh in plan['sources'].items():assert sha(out/n)==hsh,n
for n,hsh in plan['fixtures'].items():assert sha(h/n)==hsh,n
for n,hsh in plan['generated'].items():assert sha(root/n)==hsh,n
assert sha(binary)==plan['binarySha256'];plan['loadAfter']=os.getloadavg();(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('all fingerprints unchanged',flush=True)
