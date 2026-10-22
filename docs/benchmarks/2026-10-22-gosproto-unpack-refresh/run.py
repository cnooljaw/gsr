"""最新正常库Unpack/PDecode双遍源码级CPU诊断及wire分布；不产生性能排名。"""
from pathlib import Path
import subprocess,tarfile,io,os,time,json,hashlib,sys
here=Path(__file__).resolve().parent;root=here.parents[2];out=Path(sys.argv[1]).resolve();out.mkdir(exist_ok=False);commands=[];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(args,cwd,env=None):
 t=time.monotonic();r=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=60);commands.append(dict(args=args,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-t,output=r.stdout+r.stderr));(here/'commands.json').write_text(json.dumps(commands,indent=2)+'\n');assert r.returncode==0,r.stdout+r.stderr;return r.stdout+r.stderr
lib=out/'gosproto';lib.mkdir();commit=run(['git','rev-parse','9e47281'],'/private/tmp/gsr-gosproto-fork').strip()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd='/private/tmp/gsr-gosproto-fork'))) as t:t.extractall(lib,filter='data')
h=out/'harness';h.mkdir()
for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
(h/'body_profile_test.go').write_bytes((here/'body-profile.go.txt').read_bytes());(h/'pattern_test.go').write_bytes((here/'pattern.go.txt').read_bytes());(h/'go.sum').write_bytes((root/'go.sum').read_bytes());(h/'go.mod').write_text('module unpackrefresh\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n')
env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
run(['gofmt','-w','body_profile_test.go','pattern_test.go'],h);run(['go','test','-mod=mod','-run','^(TestRepresentativeFixtures|TestUnpackPatterns)$','.'],h,{**env,'GSR_PATTERN_OUTPUT':str(here/'patterns.json')});run(['go','vet','-mod=mod','.'],h,env)
binary=out/'profile.test';run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h,env);(here/'modules.txt').write_text(run(['go','list','-m','-json','github.com/cnooljaw/gosproto'],h,env))
plan=dict(commit=commit,go=run(['go','version'],root).strip(),binarySha256=sha(binary),sources={str(p.relative_to(out)):sha(p) for d in [lib,h] for p in d.glob('*.go')},fixtures={n:sha(h/n) for n in ['cases.json','representative.json']},env=env,cases=['int32_edges','game_large','addressbook'],operations=['sproto_unpack','sproto_pdecode'],passes=2)
for passIndex in range(2):
 for case in plan['cases']:
  for op in plan['operations']:
   stem=f'pass{passIndex+1}-{case}-{op}';cpu=out/(stem+'.cpu');meta=here/(stem+'.json')
   (here/(stem+'-benchmark.txt')).write_text(run([str(binary),'-test.run=^$','-test.bench=^BenchmarkBodyCost$','-test.benchtime=2s','-test.timeout=30s'],h,{**env,'GSR_BODY_CASE':case,'GSR_BODY_OP':op,'GSR_PROFILE_KIND':'cpu','GSR_PROFILE_META':str(meta),'GSR_PROFILE_CPU':str(cpu)}))
   for name,args in [('flat',['-top','-nodecount=25']),('cum',['-top','-cum','-nodecount=25']),('source',['-list=github.com/cnooljaw/gosproto.UnpackWithLimit'])]:
    (here/(stem+'-'+name+'.txt')).write_text(run(['go','tool','pprof',*args,str(binary),str(cpu)],h))
   print(stem,'done',flush=True)
for n,v in plan['sources'].items():assert sha(out/n)==v
for n,v in plan['fixtures'].items():assert sha(h/n)==v
assert sha(binary)==plan['binarySha256'];plan['hashesVerifiedAfter']=True;(here/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('all fingerprints verified',flush=True)
