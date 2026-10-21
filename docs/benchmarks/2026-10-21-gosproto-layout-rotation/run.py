"""先验证实际地址轮换，再逐项交错三个布局；不修改正式库。"""
import argparse,hashlib,json,math,os,re,subprocess,time
from pathlib import Path
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--library',type=Path,required=True);p.add_argument('--output',type=Path,required=True);p.add_argument('--inputs',nargs=3,type=Path);a=p.parse_args()
here=Path(__file__).resolve().parent;root=here.parents[2];out=a.output.resolve();out.mkdir(exist_ok=False);orders=['012','120','201'];logs=[];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(cmd,cwd,env=None,timeout=120):
 start=time.monotonic();r=subprocess.run(cmd,cwd=cwd,env={**os.environ,**(env or {})},text=True,capture_output=True,timeout=timeout)
 logs.append(dict(args=cmd,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'commands.json').write_text(json.dumps(logs,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 return r.stdout+r.stderr
inputs={}
for index,order in enumerate(orders):
 d=a.inputs[index].resolve() if a.inputs else out/('layout-'+order)
 if not a.inputs:run(['python3',str(here/'prepare.py'),'--library',str(a.library.resolve()),'--output',str(d),'--order',order,'--prepare-only'],root)
 inputs[order]=d
plans={order:json.loads((d/'plan.json').read_text()) for order,d in inputs.items()};addresses={}
for order,d in inputs.items():
 plan=plans[order];assert plan['treeOrder']==order
 assert sha(d/'experiment.test')==plan['binarySha256']
 for n,h in plan['sources'].items():assert sha(d/n)==h,n
 text=(d/'assembly.txt').read_text();functions={}
 for block in text.split('TEXT ')[1:]:
  lines=block.splitlines();name=lines[0].split('(SB)')[0].split('.')[-1];m=re.search(r'0x([0-9a-f]+)',lines[1]);functions[name]=int(m[1],16)
 anchor=functions['ExperimentStaticNormal'];addresses[order]={name:dict(address=hex(address),offset=address-anchor,mod64=address%64) for name,address in functions.items()}
keys=[f'experiment{label}{kind}' for label in ['Baseline','Clone','Candidate'] for kind in ['Buffer','Message','Field','Record']]
for key in keys:assert len({addresses[o][key]['offset'] for o in orders})>1,(key,'unchanged placement')
for order in orders:
 assert plans[order]['staticTreeSourceRestoration']==plans['012']['staticTreeSourceRestoration']
 assert plans[order]['cases']==plans['012']['cases']
 assert {n:h for n,h in plans[order]['sources'].items() if not n.endswith('static_experiment.go')}=={n:h for n,h in plans['012']['sources'].items() if not n.endswith('static_experiment.go')}
plan=dict(orders=orders,inputs={k:str(v) for k,v in inputs.items()},versions={k:plans['012'][k] for k in ['commit','candidateCommit']},cases=plans['012']['cases'],modes=plans['012']['modes'],addresses=addresses,layoutPlans=plans,loadBefore=os.getloadavg(),scope='Only definition order rotated. Actual per-function offsets verified changed before timing. Layouts interleaved per mode/case; no control subtraction or pooling corrected gains.')
(out/'study.json').write_text(json.dumps(plan,indent=2)+'\n');print('all 12 hot-function offsets changed; 3 layouts ready',flush=True)
env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
for mode in plan['modes']:
 for name,op in plan['cases']:
  for order in orders:
   d=inputs[order];output=d/(mode+'-'+name+'-'+op+'.json')
   run([str(d/'experiment.test'),'-test.run=^TestStringInProcessExperiment$','-test.timeout=1m'],d/'harness',{**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':mode,'GSR_OUTPUT':str(output)},timeout=65)
   raw=json.loads(output.read_text());assert (raw['Name'],raw['Operation'],raw['Mode'])==(name,op,mode)
  print(mode,name,op,'all3 layouts done',flush=True)
for order,d in inputs.items():
 p=plans[order]
 for n,h in p['sources'].items():assert sha(d/n)==h,n
 for n,h in p['fixtures'].items():assert sha(d/'harness'/n)==h,n
 for n,h in p['generated'].items():assert sha(root/n)==h,n
 assert sha(d/'experiment.test')==p['binarySha256']
plan['loadAfter']=os.getloadavg();(out/'study.json').write_text(json.dumps(plan,indent=2)+'\n');print('all3 layout source/fixture/generated/binary hashes unchanged',flush=True)
