"""最终公开Pack/Unpack共36项形状基准，保留两轮全部样本。"""
from pathlib import Path
import subprocess,os,json,hashlib,time,sys
here=Path(__file__).resolve().parent;root=here.parents[2];binary=Path(sys.argv[1]).resolve();sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest();before=sha(binary);records=[]
for suite in ['PackShapes','UnpackShapes']:
 args=[str(binary),'-test.run=^$','-test.bench=^Benchmark'+suite+'$','-test.benchmem','-test.benchtime=150ms','-test.count=2','-test.timeout=2m'];env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off'};t=time.monotonic();p=subprocess.run(args,cwd=root/'examples/websocket-gate/benchmarks',env={**os.environ,**env},capture_output=True,text=True,timeout=120);records.append(dict(args=args,env=env,exitCode=p.returncode,seconds=time.monotonic()-t,output=p.stdout+p.stderr));(here/'final-shapes-commands.json').write_text(json.dumps(records,indent=2)+'\n');assert p.returncode==0,p.stdout+p.stderr;(here/(suite+'.txt')).write_text(p.stdout+p.stderr);print(suite,'passed',flush=True)
assert sha(binary)==before
(here/'final-shapes-build.json').write_text(json.dumps(dict(binarySha256=before,verifiedAfter=True),indent=2)+'\n')
