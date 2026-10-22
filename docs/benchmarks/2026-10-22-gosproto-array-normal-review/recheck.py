"""针对首次正常入口发现的短数组回退延长计时，保留原数据。"""
from pathlib import Path
import subprocess,json,os,time,hashlib,sys
here=Path(__file__).resolve().parent;out=Path(sys.argv[1]).resolve();plan=json.loads((here/'plan.json').read_text());commands=[]
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
for arm in ['A','B']:assert sha(out/(arm+'.test'))==plan['binaries'][arm]
for mode in ['self','compare']:
 for round in range(3):
  order='ABBA' if round%2==0 else 'BAAB'
  for position,arm in enumerate(order):
   actual='A' if mode=='self' else arm;args=[str(out/(actual+'.test')),'-test.run=^$','-test.bench=^BenchmarkArrayNormalReview$/^(w4_n1|w4_n5|w8_n8)$/^decode$','-test.benchtime=400ms','-test.count=1','-test.timeout=30s'];t=time.monotonic();r=subprocess.run(args,cwd=out/(actual+'-harness'),env={**os.environ,**plan['env']},capture_output=True,text=True,timeout=35);commands.append(dict(args=args,exitCode=r.returncode,seconds=time.monotonic()-t,output=r.stdout+r.stderr));(here/'recheck-commands.json').write_text(json.dumps(commands,indent=2)+'\n');assert r.returncode==0,r.stdout+r.stderr;(here/f'recheck-{mode}-{round}-{position}-{arm}.txt').write_text(r.stdout)
  print(mode,round,'done',flush=True)
for arm in ['A','B']:assert sha(out/(arm+'.test'))==plan['binaries'][arm]
