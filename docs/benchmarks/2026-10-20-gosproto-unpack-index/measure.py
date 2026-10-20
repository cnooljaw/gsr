"""复用预检二进制；先固定100ms窗口，再进行自身/同源控制及两轮比较。"""
import argparse,hashlib,json,math,os,re,statistics,subprocess,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();here=Path(__file__).resolve().parent;root=here.parents[2];h=out/'harness'
assert not (out/'results.json').exists()
commands=json.loads((out/'commands.json').read_text());commit='a93bb0d66678c33e328c94d39036f8ff9772a5e9';env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(cmd,cwd,env=None,timeout=60):
 start=time.monotonic();r=subprocess.run(cmd,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=timeout)
 commands.append(dict(args=cmd,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 return r.stdout+r.stderr
cases=[(n,o) for n in ['addressbook','game_small','game_large','int32_edges','echo_ascii4k'] for o in ['sproto_unpack','sproto_pdecode']]
source={str(f.relative_to(out)):sha(f) for d in ['A','B','C','harness'] for f in (out/d).glob('*.go')};binary={side:sha(out/(side+'.test')) for side in ['A','B','C']}
fixtures={str(f.relative_to(root)):sha(f) for n in ['examples/websocket-gate/gen','examples/websocket-gate/benchmarks/gen','protocol/gen'] for f in (root/n).rglob('*.go')}
plan=dict(loadBefore=os.getloadavg(),fixtureSha256={n:sha(h/n) for n in ['representative.json','cases.json']},commit=commit,cases=cases,modes=['self','clone','pass1','pass2'],rounds=6,benchtime='100ms',warmupMs=50,order='alternating ABBA/BAAB',sources=source,binary=binary,generatedInputs=fixtures,scope='single official SP module per binary; self A/A, clone A/C same source, comparisons A/B; no correction by control subtraction')
(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('built three single-module binaries, fixtures/related tests/vet passed',flush=True)
results=[]
for mode in plan['modes']:
 for case,op in cases:
  rounds=[]
  for r in range(6):
   order='ABBA' if r%2==0 else 'BAAB';samples=[]
   for arm in order:
    side='A' if arm=='A' or mode=='self' else ('C' if mode=='clone' else 'B')
    text=run([str(out/(side+'.test')),'-test.run=^$','-test.bench=^BenchmarkWordSingleModule$','-test.benchtime=100ms','-test.benchmem','-test.timeout=30s'],h,{**env,'GSR_WORD_CASE':case,'GSR_WORD_OP':op})
    m=re.search(r'BenchmarkWordSingleModule\s+(\d+)\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',text);assert m
    samples.append(dict(arm=arm,binary=side,calls=int(m[1]),ns=float(m[2]),bytes=int(m[3]),allocs=int(m[4])))
   ratio=math.exp(sum((1 if s['arm']=='B' else -1)*math.log(s['ns']) for s in samples)/2);rounds.append(dict(order=order,samples=samples,ratio=ratio))
  ratios=[r['ratio'] for r in rounds];q=statistics.quantiles(ratios,n=4,method='inclusive');results.append(dict(mode=mode,case=case,op=op,rounds=rounds,medianRatio=statistics.median(ratios),ratioIQR=q[2]-q[0]))
  (out/'results.json').write_text(json.dumps(results,indent=2)+'\n');print(mode,case,op,round((statistics.median(ratios)-1)*100,3),flush=True)
for f,h in source.items():assert sha(out/f)==h,f
for f,h in fixtures.items():assert sha(root/f)==h,f
for side,h in binary.items():assert sha(out/(side+'.test'))==h
plan['loadAfter']=os.getloadavg();(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('all fingerprints unchanged',flush=True)
