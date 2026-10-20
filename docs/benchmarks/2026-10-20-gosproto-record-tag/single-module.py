"""同一模块身份的独立二进制补充诊断；固定控制/比较、两入口、六轮ABBA/BAAB，不扣控制差值。"""
import argparse,json,os,hashlib,subprocess,time,re,statistics,math,shutil
from pathlib import Path
p=argparse.ArgumentParser(description=__doc__);p.add_argument('paired',type=Path);p.add_argument('output',type=Path);a=p.parse_args();src=a.paired.resolve();out=a.output.resolve();out.mkdir(exist_ok=False);here=Path(__file__).resolve().parent;root=here.parents[2];commands=[]
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(cmd,cwd,env=None):
 start=time.monotonic();r=subprocess.run(cmd,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=60)
 commands.append(dict(args=cmd,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 return r.stdout+r.stderr
h=out/'harness';h.mkdir()
for name in ['protocol_benchmark_test.go','representative_benchmark_test.go','representative.json','cases.json']:(h/name).write_bytes((root/'examples/websocket-gate/benchmarks'/name).read_bytes())
(h/'single_test.go').write_bytes((here/'single-module.go.txt').read_bytes())
(h/'go.sum').write_bytes((root/'go.sum').read_bytes())
base='module tagsinglemodule\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\n'
(h/'go.mod').write_text(base)
# 不改归档：before归档模块名经原modfile还原给独立副本。
for side,source in [('A','before'),('B','candidate')]:
 d=out/side;shutil.copytree(src/'pass1'/source,d);q=d/'go.mod';q.write_text(q.read_text().replace('github.com/cnooljaw/gosproto-before','github.com/cnooljaw/gosproto'))
 (h/(side+'.mod')).write_text(base+'replace github.com/cnooljaw/gosproto => '+str(d)+'\n');(h/(side+'.sum')).write_bytes((root/'go.sum').read_bytes())
 run(['go','test','-modfile='+str(h/(side+'.mod')),'-run','^TestRepresentativeFixtures$','.','-timeout=30s'],h)
 run(['go','test','-modfile='+str(h/(side+'.mod')),'-c','-o',str(out/(side+'.test')),'.'],h)
 (out/(side+'-assembly.txt')).write_text(run(['go','tool','objdump','-s',r'github.com/cnooljaw/gosproto\.(encodedIntegerRecordSize|encodeIntegerRecordInto)',str(out/(side+'.test'))],h))
frozen={str(f.relative_to(out)):sha(f) for d in ['A','B','harness'] for f in (out/d).glob('*.go')};binary={side:sha(out/(side+'.test')) for side in ['A','B']};results=[]
plan=dict(modes=['control','compare'],operations=['sproto_encode','sproto_pencode'],rounds=6,calls=100000,warmupMs=50,order='alternating ABBA/BAAB',singleImport='github.com/cnooljaw/gosproto',sources=frozen,binary=binary,scope='Supplementary independent process timing, not full37 repeat or corrected prior ratios; control executes A/A same binary, compare A/B; all samples retained')
(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n')
for mode in plan['modes']:
 for op in plan['operations']:
  rounds=[]
  for r in range(6):
   order='ABBA' if r%2==0 else 'BAAB';samples=[]
   for arm in order:
    binarySide='A' if mode=='control' else arm
    text=run([str(out/(binarySide+'.test')),'-test.run=^$','-test.bench=^BenchmarkTagSingleModule$','-test.benchtime=100000x','-test.benchmem','-test.timeout=30s'],h,{'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GSR_TAG_OP':op})
    m=re.search(r'BenchmarkTagSingleModule\s+(\d+)\s+([\d.]+) ns/op\s+(\d+) B/op\s+(\d+) allocs/op',text);assert m and int(m[1])==100000
    samples.append(dict(arm=arm,binary=binarySide,ns=float(m[2]),bytes=int(m[3]),allocs=int(m[4])))
   ratio=math.exp(sum((1 if s['arm']=='B' else -1)*math.log(s['ns']) for s in samples)/2)
   rounds.append(dict(order=order,samples=samples,ratio=ratio))
  ratios=[r['ratio'] for r in rounds];q=statistics.quantiles(ratios,n=4,method='inclusive')
  results.append(dict(mode=mode,op=op,rounds=rounds,medianRatio=statistics.median(ratios),ratioIQR=q[2]-q[0]))
  (out/'results.json').write_text(json.dumps(results,indent=2)+'\n');print(mode,op,(statistics.median(ratios)-1)*100,flush=True)
for name,digest in frozen.items():assert sha(out/name)==digest,name
for side,digest in binary.items():assert sha(out/(side+'.test'))==digest
print('all source/binary fingerprints unchanged',flush=True)
