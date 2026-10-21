"""从已计时的候选恢复生产入口，先在原提交验证回归，再执行完整验收。"""
import argparse,hashlib,io,json,os,re,shutil,subprocess,tarfile,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--experiment',type=Path,required=True);p.add_argument('--library',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();out.mkdir(exist_ok=False);experiment=a.experiment.resolve();here=Path(__file__).resolve().parent;root=here.parents[2];sha=lambda f:hashlib.sha256(f.read_bytes()).hexdigest();commands=[]
commit=json.loads((experiment/'plan.json').read_text())['commit'];base=out/'baseline';base.mkdir()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd=a.library))) as archive:archive.extractall(base,filter='data')
lib=out/'candidate';shutil.copytree(base,lib)
adapter=(experiment/'gosproto/string_experiment.go').read_text()
start=adapter.index('func experimentStringCandidate(');end=adapter.index('func experimentIntegerRecord(',start)
body=adapter[start:end].replace('experimentStringCandidate','encodeFieldInto',1).replace('experimentIntegerRecord','encodeIntegerRecordInto').replace('experimentIntegerArray','encodeIntegerRecordArrayInto')
f=lib/'encode.go';original=f.read_text();begin=original.index('func encodeFieldInto(');finish=original.index('// encodeScalarInto',begin);f.write_text(original[:begin]+body+original[finish:])
start=end;end=adapter.index('var experimentStringSelected',start)
helpers=adapter[start:end].replace('experimentIntegerRecord','encodeIntegerRecordInto').replace('experimentIntegerArray','encodeIntegerRecordArrayInto')
f=lib/'encode_integer_record.go';original=f.read_text();prefix=original[:original.index('func encodeIntegerRecordInto(')];prefix=prefix.replace('import "fmt"','import ("fmt";"reflect";"unsafe")');f.write_text(prefix+helpers)
# 扩展同一个已有测试，增加阈值与布局分段边界，不新建重复夹具。
for folder in [base,lib]:
 f=folder/'encode_record_pointers_test.go';text=f.read_text();assert '[]int{0, 1, 3, 32, 33, 129}' in text;text=text.replace('[]int{0, 1, 3, 32, 33, 129}','[]int{0, 1, 3, 7, 8, 31, 32, 33, 64, 129}');f.write_text(text)
def run(cmd,cwd,env=None,timeout=120):
 start=time.monotonic();r=subprocess.run(cmd,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=timeout);commands.append(dict(args=cmd,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'acceptance.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 print(cmd[0]+' '+cmd[1]+' passed',flush=True)
run(['gofmt','-w','encode.go','encode_integer_record.go','encode_record_pointers_test.go'],lib)
run(['go','test','-run','^TestEncodeRecordPointerSlices$','-timeout=1m','.'],base,{'GOPROXY':'off'})
fixed={f.name:sha(f) for f in lib.glob('*.go')}
oracle=root/'tmp/gosproto-pack-payload/oracle';prov=json.loads((oracle/'provenance.json').read_text())
for n,r in prov['files'].items():assert sha(oracle/n)==r['sha256']
(out/'oracle-provenance.json').write_text(json.dumps(prov,indent=2)+'\n')
mod=out/'integration.mod';mod.write_text((root/'go.mod').read_text()+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n');(out/'integration.sum').write_bytes((root/'go.sum').read_bytes());env={'SPROTO_ORACLE_DIR':str(oracle),'GOPROXY':'off'}
run(['go','test','./...','-timeout=2m'],lib,env)
run(['go','test','-race','-gcflags=all=-d=checkptr=2','./...','-timeout=2m'],lib,{**env,'GOMAXPROCS':'2'})
run(['go','vet','./...'],lib,env)
for target in ['FuzzCodecRoundTrip','FuzzDecode']:run(['go','test','-run','^$','-fuzz','^'+target+'$','-fuzztime=10s','-parallel=2','.','-timeout=1m'],lib,{'GOPROXY':'off','GOMAXPROCS':'2'},timeout=70)
run(['go','test','-modfile='+str(mod),'./protocol/sproto','./examples/websocket-gate/cmd/debugserver','./examples/websocket-gate/benchmarks','-timeout=2m'],root)
run(['npm','--prefix','examples/websocket-gate','run','test:sproto:integration'],root,{'GOFLAGS':'-modfile='+str(mod)})
assert fixed=={f.name:sha(f) for f in lib.glob('*.go')};(out/'accepted-source.json').write_text(json.dumps(fixed,indent=2)+'\n')
