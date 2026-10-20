"""当前SP正文Decode与Unpack CPU/全量分配诊断；独立进程，不产生配对性能排名。"""
import argparse,hashlib,io,json,os,platform,subprocess,tarfile,time
from pathlib import Path
parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--library',type=Path,required=True)
parser.add_argument('--output',type=Path,required=True)
parser.add_argument('--prepare-only',action='store_true')
args=parser.parse_args();here=Path(__file__).resolve().parent;root=here.parents[2];out=args.output.resolve();out.mkdir(parents=True,exist_ok=False)
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
commands=[]
def run(argv,cwd,env=None,timeout=60):
 start=time.monotonic();z=subprocess.run(argv,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=timeout)
 commands.append({'args':argv,'cwd':str(cwd),'env':env or {},'exitCode':z.returncode,'seconds':time.monotonic()-start,'output':z.stdout+z.stderr});(out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
 if z.returncode:raise SystemExit(z.stdout+z.stderr)
 return z.stdout+z.stderr
lib=args.library.resolve();commit=run(['git','rev-parse','a93bb0d'],lib).strip();snapshot=out/'gosproto';snapshot.mkdir()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd=lib))) as t:t.extractall(snapshot,filter='data')
h=out/'harness';h.mkdir()
for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
(h/'body_profile_test.go').write_bytes((here/'body-profile.go.txt').read_bytes())
(h/'go.mod').write_text('module gosprotobodystudy\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\nreplace github.com/cnooljaw/gosproto => '+str(snapshot)+'\n')
(h/'go.sum').write_bytes((root/'go.sum').read_bytes())
run(['gofmt','-w','body_profile_test.go'],h)
files=[here/'body-profile.go.txt',here/'reproduce.py',*[f for name in ['examples/websocket-gate/benchmarks/gen','examples/websocket-gate/gen','protocol/gen'] for f in (root/name).rglob('*.go')],*[root/'examples/websocket-gate/benchmarks'/n for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','cases.json','representative.json']]]
frozen={str(p.relative_to(root)):sha(p) for p in files}
source={p.name:sha(p) for p in snapshot.glob('*.go')};hf={p.name:sha(p) for p in h.glob('*.go')}
plan={'date':'2026-10-20','gsrHead':run(['git','rev-parse','HEAD'],root).strip(),'libraryCommit':commit,'go':run(['go','version'],root).strip(),'platform':platform.platform(),'loadBefore':os.getloadavg(),'cases':['addressbook','game_large','int32_edges'],'ops':['sproto_decode','sproto_unpack','sproto_pdecode'],'kinds':['cpu','allocation'],'cpuTime':'2s','allocationCalls':5000,'allocationProfileRate':1,'gc':'GOMAXPROCS1/GOGC100/unlimitedGOMEMLIMIT','source':source,'harnessGo':hf,'frozenInputs':frozen,'scope':'Independent CPU and allocation processes. Allocation profile includes initialization/warmup; loop B/op and allocs are ResetTimer-only. No timing ranking, no candidate production changes; Decode/Unpack/PDecode public paths.'}
(out/'inputs.json').write_text(json.dumps(plan,indent=2)+'\n')
def check():
 assert {p.name:sha(p) for p in snapshot.glob('*.go')}==source
 assert {p.name:sha(p) for p in h.glob('*.go')}==hf
 for n,v in frozen.items():assert sha(root/n)==v,n
if args.prepare_only:check();print('prepared');raise SystemExit
binary=out/'body.test';env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
run(['go','test','-mod=mod','-run','^TestRepresentativeFixtures$','.','-timeout=30s'],h,env)
run(['go','vet','-mod=mod','.'],h,env)
run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h,env)
(out/'modules.txt').write_text(run(['go','list','-mod=mod','-m','-json','github.com/cnooljaw/gosproto','google.golang.org/protobuf'],h,env))
plan['binarySha256']=sha(binary);(out/'inputs.json').write_text(json.dumps(plan,indent=2)+'\n');check();print('built and fixtures checked',flush=True)
profiles={}
for case in plan['cases']:
 for op in plan['ops']:
  for kind in plan['kinds']:
   stem=case+'-'+op+'-'+kind;raw=out/(stem+('.cpu' if kind=='cpu' else '.alloc'));meta=out/(stem+'.json')
   e={**env,'GSR_BODY_CASE':case,'GSR_BODY_OP':op,'GSR_PROFILE_KIND':kind,'GSR_PROFILE_META':str(meta)}
   argv=[str(binary),'-test.run=^$','-test.bench=^BenchmarkBodyCost$','-test.benchmem','-test.benchtime='+('2s' if kind=='cpu' else '5000x'),'-test.timeout=1m']
   if kind=='cpu':e['GSR_PROFILE_CPU']=str(raw)
   else:argv+=['-test.memprofilerate=1','-test.memprofile='+str(raw)]
   (out/(stem+'-benchmark.txt')).write_text(run(argv,h,e,timeout=65));m=json.loads(meta.read_text());assert (m['Case'],m['Operation'],m['Kind'])==(case,op,kind)
   assert m['Calls']>0 and (kind!='allocation' or m['Calls']==5000)
   views=[('flat',['-top','-nodecount=40']),('cum',['-top','-cum','-nodecount=40'])] if kind=='cpu' else [('objects',['-top','-alloc_objects','-nodecount=40']),('space',['-top','-alloc_space','-nodecount=40'])]
   views += [('source',[r'-list=representativeOperations|github.com/cnooljaw/gosproto\.(UnpackWithLimit|zeroHeaderPrefix|decodeMessage|decodeStructSlice|messageDecodePointer|decodeField|decodeInteger)|reflect.New$'])]
   for suffix,flags in views:
    text=run(['go','tool','pprof',*flags,str(binary),str(raw)],h)
    (out/(stem+'-'+suffix+'.txt')).write_text('\n'.join(l.rstrip() for l in text.splitlines())+'\n')
   profiles[stem]={'rawSha256':sha(raw),'metadataSha256':sha(meta)};check();assert sha(binary)==plan['binarySha256'];print(stem,'done',flush=True)
plan['profiles']=profiles;plan['loadAfter']=os.getloadavg();(out/'inputs.json').write_text(json.dumps(plan,indent=2)+'\n')
