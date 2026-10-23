"""最终正常二进制的阶段形状基准和只读CPU诊断，独立顺序执行。"""
from pathlib import Path
import subprocess,json,hashlib,os,time,sys
here=Path(__file__).resolve().parent;root=here.parents[2];out=Path(sys.argv[1]).resolve();out.mkdir(exist_ok=False);binary=root/'tmp/gosproto-closure-final-guard-20261022/final.test';bench=root/'examples/websocket-gate/benchmarks';commands=[];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest();before=sha(binary)
def run(args,env=None):
 t=time.monotonic();p=subprocess.run(args,cwd=bench,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=120);commands.append(dict(args=args,env=env or {},exitCode=p.returncode,seconds=time.monotonic()-t,output=p.stdout+p.stderr));(here/'audit-commands.json').write_text(json.dumps(commands,indent=2)+'\n');assert p.returncode==0,p.stdout+p.stderr;return p.stdout+p.stderr
for suite in ['PackShapes','UnpackShapes']:
 (here/(suite+'-before-minrun.txt')).write_text(run([str(binary),'-test.run=^$','-test.bench=^Benchmark'+suite+'$','-test.benchmem','-test.benchtime=150ms','-test.count=2','-test.timeout=2m'],{'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off'}))
profiles={}
for name,op in [('game_large','sproto_encode'),('game_large','sproto_decode'),('game_large','sproto_pack'),('game_large','sproto_unpack'),('int32_edges','sproto_encode'),('int32_edges','sproto_decode'),('int32_edges','sproto_pack'),('int32_edges','sproto_unpack')]:
 stem=name+'-'+op;raw=out/(stem+'.cpu');pattern='^BenchmarkRepresentativeCodec$/^'+name+'$/^'+op+'$'
 (here/(stem+'-benchmark.txt')).write_text(run([str(binary),'-test.run=^$','-test.bench='+pattern,'-test.benchmem','-test.benchtime=2s','-test.timeout=1m','-test.cpuprofile='+str(raw)],{'GOMAXPROCS':'1','GOGC':'1000','GOMEMLIMIT':'128MiB'}))
 for kind,flags in [('flat',['-top','-nodecount=35']),('cum',['-top','-cum','-nodecount=35'])]:
  text=run(['go','tool','pprof',*flags,str(binary),str(raw)]);(here/(stem+'-'+kind+'.txt')).write_text('\n'.join(l.rstrip() for l in text.splitlines())+'\n')
 profiles[stem]=sha(raw);print(stem,'profile done',flush=True)
assert sha(binary)==before
(here/'audit.json').write_text(json.dumps(dict(binarySha256=before,binaryVerifiedAfter=True,profiles=profiles,commands=len(commands),scope='Shape runs GOGC100; CPU diagnosis GOGC1000 and 128MiB limit includes setup/calibration; no profiled ns/op ranking'),indent=2)+'\n')
