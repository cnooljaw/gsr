"""固定八字节解包目标视图：单模块独立二进制，自身/同源控制及两轮比较。"""
import argparse,hashlib,io,json,math,os,re,statistics,subprocess,tarfile,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--library',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args()
here=Path(__file__).resolve().parent;root=here.parents[2];out=a.output.resolve();out.mkdir(exist_ok=False);commands=[]
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(cmd,cwd,env=None,timeout=60):
 start=time.monotonic();r=subprocess.run(cmd,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=timeout)
 commands.append(dict(args=cmd,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 return r.stdout+r.stderr
lib=a.library.resolve();commit=run(['git','rev-parse','a93bb0d'],lib).strip();blob=subprocess.check_output(['git','archive',commit],cwd=lib)
for side in ['A','B','C']:
 d=out/side;d.mkdir()
 with tarfile.open(fileobj=io.BytesIO(blob)) as t:t.extractall(d,filter='data')
f=out/'B/pack.go';s=f.read_text();old='\t\tused := 0\n\t\tfor mask := header; ; {';new='\t\t// 首遍已证明完整 word 的输出长度，固定视图让目标索引限于 0～7。\n\t\tdst := unpacked[offset : offset+8 : offset+8]\n\t\tused := 0\n\t\tfor mask := header; ; {';assert s.count(old)==1;s=s.replace(old,new).replace('unpacked[offset+index] = src[used]','dst[index] = src[used]');f.write_text(s)
patch=subprocess.run(['diff','-u',str(out/'A/pack.go'),str(f)],capture_output=True,text=True);assert patch.returncode==1;(out/'candidate.patch').write_text(patch.stdout)
h=out/'harness';h.mkdir()
for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','representative.json','cases.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
(h/'single_test.go').write_bytes((here/'single-module.go.txt').read_bytes())
base='module wordstudy\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\n'
env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
for side in ['A','B','C']:
 (h/(side+'.mod')).write_text(base+'replace github.com/cnooljaw/gosproto => '+str(out/side)+'\n');(h/(side+'.sum')).write_bytes((root/'go.sum').read_bytes())
 run(['go','test','-run','^(TestPack|TestUnpack)','-timeout=30s','.'],out/side,env)
 run(['go','test','-modfile='+str(h/(side+'.mod')),'-run','^TestRepresentativeFixtures$','.','-timeout=30s'],h,env)
 run(['go','test','-modfile='+str(h/(side+'.mod')),'-c','-o',str(out/(side+'.test')),'.'],h,env)
 (out/(side+'-assembly.txt')).write_text(run(['go','tool','objdump','-s',r'github.com/cnooljaw/gosproto\.UnpackWithLimit',str(out/(side+'.test'))],h))
 (out/(side+'-modules.txt')).write_text(run(['go','list','-modfile='+str(h/(side+'.mod')),'-m','-json','github.com/cnooljaw/gosproto'],h,env))
run(['go','vet','-modfile='+str(h/'B.mod'),'.'],h,env)
cases=[(n,o) for n in ['addressbook','game_small','game_large','int32_edges','echo_ascii4k'] for o in ['sproto_unpack','sproto_pdecode']]
source={str(f.relative_to(out)):sha(f) for d in ['A','B','C','harness'] for f in (out/d).glob('*.go')};binary={side:sha(out/(side+'.test')) for side in ['A','B','C']}
fixtures={str(f.relative_to(root)):sha(f) for n in ['examples/websocket-gate/gen','examples/websocket-gate/benchmarks/gen','protocol/gen'] for f in (root/n).rglob('*.go')}
plan=dict(commit=commit,cases=cases,modes=['self','clone','pass1','pass2'],rounds=6,benchtime='20ms',warmupMs=50,order='alternating ABBA/BAAB',sources=source,binary=binary,generatedInputs=fixtures,scope='single official SP module per binary; self A/A, clone A/C same source, comparisons A/B; no correction by control subtraction')
(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('built three single-module binaries, fixtures/related tests/vet passed',flush=True)
results=[]
for mode in plan['modes']:
 for case,op in cases:
  rounds=[]
  for r in range(6):
   order='ABBA' if r%2==0 else 'BAAB';samples=[]
   for arm in order:
    side='A' if arm=='A' or mode=='self' else ('C' if mode=='clone' else 'B')
    text=run([str(out/(side+'.test')),'-test.run=^$','-test.bench=^BenchmarkWordSingleModule$','-test.benchtime=20ms','-test.benchmem','-test.timeout=30s'],h,{**env,'GSR_WORD_CASE':case,'GSR_WORD_OP':op})
    m=re.search(r'BenchmarkWordSingleModule\s+(\d+)\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',text);assert m
    samples.append(dict(arm=arm,binary=side,calls=int(m[1]),ns=float(m[2]),bytes=int(m[3]),allocs=int(m[4])))
   ratio=math.exp(sum((1 if s['arm']=='B' else -1)*math.log(s['ns']) for s in samples)/2);rounds.append(dict(order=order,samples=samples,ratio=ratio))
  ratios=[r['ratio'] for r in rounds];q=statistics.quantiles(ratios,n=4,method='inclusive');results.append(dict(mode=mode,case=case,op=op,rounds=rounds,medianRatio=statistics.median(ratios),ratioIQR=q[2]-q[0]))
  (out/'results.json').write_text(json.dumps(results,indent=2)+'\n');print(mode,case,op,round((statistics.median(ratios)-1)*100,3),flush=True)
for f,h in source.items():assert sha(out/f)==h,f
for f,h in fixtures.items():assert sha(root/f)==h,f
for side,h in binary.items():assert sha(out/(side+'.test'))==h
print('all fingerprints unchanged',flush=True)
