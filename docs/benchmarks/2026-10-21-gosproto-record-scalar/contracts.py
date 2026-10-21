"""在独立副本中运行三种parser的既有契约，不修改计时快照。"""
import argparse,hashlib,json,os,shutil,subprocess,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--output',type=Path,required=True);a=p.parse_args()
here=Path(__file__).resolve().parent;root=here.parents[2];out=a.output.resolve();out.mkdir(exist_ok=False)
source=root/'tmp/gosproto-record-scalar-20261021-run2/gosproto';target=out/'gosproto';shutil.copytree(source,target)
(target/'record_scalar_contract_test.go').write_bytes((here/'contract-test.go.txt').read_bytes())
(target/'record_scalar_external_contract_test.go').write_bytes((here/'external-contract-test.go.txt').read_bytes())
commands=[]
for cmd in [['gofmt','-w','record_scalar_contract_test.go','record_scalar_external_contract_test.go'],['go','test','-race','-gcflags=all=-d=checkptr=2','-run','^TestRecordScalar(External)?Contracts$','-timeout=2m','.']]:
 start=time.monotonic();r=subprocess.run(cmd,cwd=target,env={**os.environ,'GOMAXPROCS':'1','GOGC':'100','GOPROXY':'off'},text=True,capture_output=True,timeout=125)
 commands.append(dict(args=cmd,seconds=time.monotonic()-start,exitCode=r.returncode,output=r.stdout+r.stderr))
 (here/'contracts.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
print('three parser contracts passed')
