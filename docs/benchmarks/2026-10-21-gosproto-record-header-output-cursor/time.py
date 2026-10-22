"""只测已经通过准备的直接header游标归档；保留全部命令与固定样本。"""
from pathlib import Path
import os,sys,json,subprocess,time,hashlib
out=Path(sys.argv[1]).resolve();plan=json.loads((out/'plan.json').read_text());commands=json.loads((out/'commands.json').read_text());binary=out/'experiment.test';root=Path(__file__).resolve().parents[3]
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def check():
 for n,h in plan['sources'].items():assert sha(out/n)==h,n
 for n,h in plan['fixtures'].items():assert sha(out/'harness'/n)==h,n
 for n,h in plan['generated'].items():assert sha(root/n)==h,n
 assert sha(binary)==plan['binarySha256']
check();assert not any(out.glob('pass*-*.json')) and not any(out.glob('self-*.json')) and not any(out.glob('clone-*.json'))
env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
for mode in plan['modes']:
 for name,op in plan['cases']:
  output=out/f'{mode}-{name}-{op}.json';args=[str(binary),'-test.run=^TestStringInProcessExperiment$','-test.timeout=1m'];e={**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':mode,'GSR_OUTPUT':str(output)}
  start=time.monotonic();p=subprocess.run(args,cwd=out/'harness',env={**os.environ,**e},text=True,capture_output=True,timeout=65);commands.append(dict(args=args,cwd=str(out/'harness'),env=e,exitCode=p.returncode,seconds=time.monotonic()-start,output=p.stdout+p.stderr));(out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n');assert p.returncode==0,p.stdout+p.stderr;print(mode,name,op,flush=True)
check();print('all samples and hashes passed',flush=True)
