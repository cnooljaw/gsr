"""正常Git归档对照三步稳定字节展开，保留预检、稀疏/尾部回退及输出契约。"""
from pathlib import Path
import subprocess,io,tarfile,json,os,time,hashlib,sys
here=Path(__file__).resolve().parent;root=here.parents[2];out=Path(sys.argv[1]).resolve();out.mkdir(exist_ok=False);commands=[];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(args,cwd,env=None):
 t=time.monotonic();r=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=120);commands.append(dict(args=args,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-t,output=r.stdout+r.stderr));(here/'commands.json').write_text(json.dumps(commands,indent=2)+'\n');assert r.returncode==0,r.stdout+r.stderr;return r.stdout+r.stderr
plan={'commits':{},'binaries':{},'sources':{},'fixtures':{},'orders':[]};env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
for arm,ref in [('A','9b2ecfb'),('B','9b2ecfb')]:
 lib=out/arm;lib.mkdir();commit=run(['git','rev-parse',ref],'/private/tmp/gsr-gosproto-fork').strip();plan['commits'][arm]=commit
 with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd='/private/tmp/gsr-gosproto-fork'))) as t:t.extractall(lib,filter='data')

 if arm=='B':
  (lib/'pack.go').write_bytes((here/'candidate-pack.go.txt').read_bytes())
 run(['gofmt','-w','pack.go'],lib)
 run(['go','test','-race','-gcflags=all=-d=checkptr=2','-run','^(TestUnpack|TestLimits|TestOfficial|TestMalformed)','-timeout=2m','.'],lib,{**env,'GOMAXPROCS':'2'})
 h=out/(arm+'-harness');h.mkdir()
 for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
 (h/'review_test.go').write_bytes((here/'benchmark.go.txt').read_bytes());(h/'unpack_shapes_test.go').write_bytes((root/'examples/websocket-gate/benchmarks/unpack_benchmark_test.go').read_bytes());(h/'go.sum').write_bytes((root/'go.sum').read_bytes())
 (h/'go.mod').write_text('module arrayreview\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n')
 run(['gofmt','-w','review_test.go'],h);run(['go','test','-mod=mod','-run','^TestRepresentativeFixtures$','.'],h,env);run(['go','vet','-mod=mod','.'],h,env)
 binary=out/(arm+'.test');run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h,env);(here/(arm+'-assembly.txt')).write_text(run(['go','tool','objdump','-s','github.com/cnooljaw/gosproto.UnpackWithLimit',str(binary)],h));plan['binaries'][arm]=sha(binary);plan['sources'][arm]={p.name:sha(p) for p in lib.glob('*.go')};plan['fixtures'][arm]={n:sha(h/n) for n in ['review_test.go','unpack_shapes_test.go','cases.json','representative.json']}
 print('built',arm,flush=True)
# self 使用同一个A二进制，位置B仅为标签；不扣减控制。
for mode in ['self','pass1','pass2']:
 for round in range(3):
  order='ABBA' if round%2==0 else 'BAAB';plan['orders'].append(dict(mode=mode,round=round,order=order))
  for position,arm in enumerate(order):
   actual='A' if mode=='self' else arm
   output=run([str(out/(actual+'.test')),'-test.run=^$','-test.bench=^BenchmarkPairUnpackReview$','-test.benchtime=200ms','-test.count=1','-test.timeout=2m'],out/(actual+'-harness'),env)
   (here/f'{mode}-{round}-{position}-{arm}.txt').write_text(output)
  print(mode,round,'done',flush=True)
for arm in ['A','B']:
 assert sha(out/(arm+'.test'))==plan['binaries'][arm]
 for n,hsh in plan['sources'][arm].items():assert sha(out/arm/n)==hsh
 for n,hsh in plan['fixtures'][arm].items():assert sha(out/(arm+'-harness')/n)==hsh
plan['go']=run(['go','version'],root).strip();plan['env']=env;plan['hashesVerifiedAfter']=True;(here/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('all fingerprints verified',flush=True)
