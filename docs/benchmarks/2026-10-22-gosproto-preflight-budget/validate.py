"""仅在计时结束后运行候选正确性检查和静态诊断，核对冻结源码与二进制。"""
from pathlib import Path
import subprocess,json,os,time,hashlib,sys,re
here=Path(__file__).resolve().parent;root=here.parents[2];out=Path(sys.argv[1]).resolve();lib=out/'gosproto';binary=out/'pairs.test';commands=[];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
plan=json.loads((here/'plan.json').read_text())
def check():
 for n,v in plan['sources'].items():assert sha(out/n)==v,n
 assert sha(binary)==plan['binarySha256']
check()
for args,env in [(['go','test','-tags=purego','-race','-gcflags=all=-d=checkptr=2','-run','^TestPreflightCapComparison$','.','-timeout=2m'],{'GOMAXPROCS':'2','GOPROXY':'off'}),(['go','test','./...','-timeout=2m'],{'SPROTO_ORACLE_DIR':str(root/'tmp/gosproto-pack-payload/oracle'),'GOPROXY':'off'})]:
 t=time.monotonic();p=subprocess.run(args,cwd=lib,env={**os.environ,**env},capture_output=True,text=True,timeout=120);commands.append(dict(args=args,env=env,exitCode=p.returncode,seconds=time.monotonic()-t,output=p.stdout+p.stderr));(here/'extra-commands.json').write_text(json.dumps(commands,indent=2)+'\n');assert p.returncode==0,p.stdout+p.stderr
assembly={}
for sym in ['UnpackWithLimit','ExperimentUnpackOriginal']:
 p=subprocess.run(['go','tool','objdump','-s','github.com/cnooljaw/gosproto.'+sym,str(binary)],capture_output=True,text=True);assert p.returncode==0
 lines=p.stdout.splitlines();(here/(sym+'-objdump.txt')).write_text('\n'.join(x.rstrip() for x in lines)+'\n');assembly[sym]={'instructionCount':sum(bool(re.match(r'^\s+\S+\s+0x',x)) for x in lines)}
(here/'assembly-summary.json').write_text(json.dumps(assembly,indent=2)+'\n');check()
(here/'candidate-pack.go.txt').write_bytes((lib/'pack.go').read_bytes());(here/'baseline-experiment.go.txt').write_bytes((lib/'experiment.go').read_bytes())
(here/'validation.json').write_text(json.dumps(dict(sourceAndBinaryVerifiedAfter=True,mainCommands=len(json.loads((here/'commands.json').read_text())),extraCommands=len(commands),assembly=assembly,scope='Native and purego differential race/checkptr; complete candidate library with official oracle; production application decided separately'),indent=2)+'\n');print(assembly)
