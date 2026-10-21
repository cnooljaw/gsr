"""先完成三个实际摆放的预检，再逐模式、消息和轮次交错测量。"""
from pathlib import Path
import sys,os,subprocess,json,re,hashlib,time
here=Path(__file__).resolve().parent;root=here.parents[2];out=Path(sys.argv[1]).resolve();out.mkdir(exist_ok=False);logs=[]
def run(args,cwd,env=None):
 start=time.monotonic();p=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=120);logs.append(dict(args=args,cwd=str(cwd),env=env or {},exitCode=p.returncode,seconds=time.monotonic()-start,output=p.stdout+p.stderr));(here/'commands.json').write_text(json.dumps(logs,indent=2)+'\n');assert p.returncode==0,p.stdout+p.stderr;return p.stdout
orders=['012','120','201'];plans={};offsets={}
for order in orders:
 print(run([sys.executable,str(here/'prepare.py'),str(out/order),order],root),flush=True)
 p=json.loads((here/order/'plan.json').read_text());plans[order]=p
 text=(here/order/'assembly.txt').read_text();addresses={}
 for section in text.split('TEXT ')[1:]:
  name=section.split('(SB)')[0].rsplit('.',1)[-1];m=re.search(r'\s0x([0-9a-f]+)\s',section);assert m;addresses[name]=int(m[1],16)
 anchor=min(addresses.values());offsets[order]={k:v-anchor for k,v in addresses.items()}
assert len({tuple(sorted(x.items())) for x in offsets.values()})==3
for name in ['packInto','packGatherClone','packGatherCandidate']:assert len({offsets[o][name] for o in orders})==3,name
(here/'offsets.json').write_text(json.dumps(offsets,indent=2)+'\n');print('all preparations and actual address rotation passed',flush=True)
for mode in plans['012']['modes']:
 for index,(name,op) in enumerate(plans['012']['cases']):
  for order in orders[index%3:]+orders[:index%3]:
   p=plans[order];output=here/order/f'{mode}-{name}-{op}.json';binary=out/order/'experiment.test'
   run([str(binary),'-test.run=^TestUnpackInProcessExperiment$','-test.timeout=1m'],out/order/'harness',{**p['env'],'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':mode,'GSR_OUTPUT':str(output)});print(order,mode,name,op,flush=True)
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
for order,p in plans.items():
 for n,h in p['sources'].items():assert sha(out/order/n)==h,n
 for n,h in p['fixtures'].items():assert sha(out/order/'harness'/n)==h,n
 assert sha(out/order/'experiment.test')==p['binarySha256']
 run([sys.executable,str(here/'analyze.py'),order],root)
print('all samples, fingerprints and analyses passed',flush=True)
