"""在独立副本同一二进制采基线/拒绝候选的真实Unpack CPU；不作为性能配对排名。"""
import argparse,hashlib,json,os,shutil,subprocess,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--experiment',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();out.mkdir(exist_ok=False);here=Path(__file__).resolve().parent;root=here.parents[2];commands=[];sha=lambda f:hashlib.sha256(f.read_bytes()).hexdigest()
shutil.copytree(a.experiment.resolve()/'harness',out/'harness',ignore=shutil.ignore_patterns('experiment_test.go'))
h=out/'harness';s=(root/'docs/benchmarks/2026-10-20-gosproto-packing-refresh/body-profile.go.txt').read_text().replace(' "encoding/json"',' "encoding/json"\n sp "github.com/cnooljaw/gosproto"')
s=s.replace('operations,_:=representativeOperations(b,sample)','operations,wires:=representativeOperations(b,sample)')
s=s.replace('for _,op:=range operations {if op.name==operationName {selected=op.run}}','''for _,op:=range operations {if op.name==operationName {selected=op.run}}
  if operationName!="sproto_unpack" {b.Fatal("only unpack profile")}
  unpack:=sp.UnpackWithLimit
  if os.Getenv("GSR_PROFILE_VARIANT")=="candidate" {unpack=sp.ExperimentUnpackCandidate}
  packed:=wires["sproto_pack"]
  selected=func()error {data,err:=unpack(packed,8<<20);encodedSink=data;return err}''')
(h/'body_profile_test.go').write_text(s)
def run(cmd,env=None):
 start=time.monotonic();r=subprocess.run(cmd,cwd=h,env={**os.environ,'GOPROXY':'off','GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off',**(env or {})},text=True,capture_output=True,timeout=60);commands.append(dict(args=cmd,env=env or {},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 return r.stdout+r.stderr
run(['gofmt','-w','body_profile_test.go'])
run(['go','test','-mod=mod','-run','^TestRepresentativeFixtures$','.'])
run(['go','vet','-mod=mod','.'])
binary=out/'profile.test';run(['go','test','-mod=mod','-c','-o',str(binary),'.'])
module=run(['go','list','-m','-json','github.com/cnooljaw/gosproto']);(out/'modules.txt').write_text(module)
assert module.count('"Path": "github.com/cnooljaw/gosproto"')==1
frozen={f.name:sha(f) for f in h.glob('*.go')};profiles={}
for case in ['game_large','int32_edges']:
 for variant in ['baseline','candidate']:
  stem=case+'-'+variant;raw=out/(stem+'.cpu');meta=out/(stem+'.json')
  env={'GSR_BODY_CASE':case,'GSR_BODY_OP':'sproto_unpack','GSR_PROFILE_KIND':'cpu','GSR_PROFILE_META':str(meta),'GSR_PROFILE_CPU':str(raw),'GSR_PROFILE_VARIANT':variant}
  (out/(stem+'-benchmark.txt')).write_text(run([str(binary),'-test.run=^$','-test.bench=^BenchmarkBodyCost$','-test.benchmem','-test.benchtime=2s','-test.timeout=1m'],env))
  for view,flags in [('flat',['-top','-nodecount=40']),('source',['-list=UnpackWithLimit|ExperimentUnpackCandidate']),('cum',['-top','-cum','-nodecount=40'])]:
   (out/(stem+'-'+view+'.txt')).write_text(run(['go','tool','pprof',*flags,str(binary),str(raw)]))
  profiles[stem]={'rawSha256':sha(raw),'metaSha256':sha(meta)}
  assert frozen=={f.name:sha(f) for f in h.glob('*.go')}
  print(stem,'done',flush=True)
(out/'inputs.json').write_text(json.dumps({'scope':'four independent CPU profiles; full denominators, no timing ranking','harness':frozen,'binarySha256':sha(binary),'profiles':profiles,'experimentPlanSha256':sha(a.experiment.resolve()/'plan.json')},indent=2)+'\n')
