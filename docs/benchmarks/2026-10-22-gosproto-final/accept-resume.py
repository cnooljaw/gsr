"""保留32位既有测试编译失败，继续未完成验收，不重复已通过测试/fuzz。"""
from pathlib import Path
import subprocess,os,time,json,hashlib
here=Path(__file__).resolve().parent;root=here.parents[2];out=root/'tmp/gosproto-final-accept-20261022';lib=out/'candidate';commands=json.loads((here/'acceptance.json').read_text());sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(args,cwd,env=None,timeout=120,expected=0):
 t=time.monotonic();r=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=timeout);commands.append(dict(args=args,cwd=str(cwd),env=env or {},exitCode=r.returncode,expectedExitCode=expected,seconds=time.monotonic()-t,output=r.stdout+r.stderr));(here/'acceptance.json').write_text(json.dumps(commands,indent=2)+'\n');assert r.returncode==expected,r.stdout+r.stderr;print('checked',args[:4],flush=True);return r.stdout+r.stderr
baseline=root/'tmp/gosproto-final-native-array-20261022/A'
run(['go','test','-c','-o',str(out/'baseline-386.test'),'.'],baseline,{'GOPROXY':'off','CGO_ENABLED':'0','GOOS':'linux','GOARCH':'386','GOMAXPROCS':'2'},expected=1)
run(['go','test','-c','-o',str(out/'s390x.test'),'.'],lib,{'GOPROXY':'off','CGO_ENABLED':'0','GOOS':'linux','GOARCH':'s390x','GOMAXPROCS':'2'})
for arch in ['386','arm']:run(['go','build','./...'],lib,{'GOPROXY':'off','CGO_ENABLED':'0','GOOS':'linux','GOARCH':arch,'GOMAXPROCS':'2'})
source=(here/'accept.py').read_text();exec(compile(source[source.index("mod=out/'integration.mod'"):],str(here/'accept.py')+':tail','exec'))
