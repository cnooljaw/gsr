"""最终计时后仅补交替FF重叠用例及固定次数fuzz；不改已测生产实现。"""
from pathlib import Path
import subprocess,time,json,os,hashlib
here=Path(__file__).resolve().parent;root=here.parents[2];lib=root/'tmp/gosproto-closure-extra-20261022';records=[]
def run(args):
 start=time.monotonic();p=subprocess.run(args,cwd=lib,env={**os.environ,'GOMAXPROCS':'2','GOPROXY':'off'},capture_output=True,text=True,timeout=120);records.append(dict(args=args,exitCode=p.returncode,seconds=time.monotonic()-start,output=p.stdout+p.stderr));(here/'extra-checks.json').write_text(json.dumps(records,indent=2)+'\n');assert p.returncode==0,p.stdout+p.stderr;print('passed',args[:4],flush=True)
run(['gofmt','-w','pack_words_arm64_test.go'])
run(['go','test','-race','-gcflags=all=-d=checkptr=2','-run','^(TestPackWords|TestEncodePackedWorkspace)','.','-timeout=2m'])
run(['go','test','-run','^$','-fuzz','^FuzzPackRoundTrip$','-fuzztime=10000x','-parallel=1','-timeout=45s','.'])
(here/'extra-pack_words_arm64_test.go.txt').write_bytes((lib/'pack_words_arm64_test.go').read_bytes())
(here/'extra-test-source.json').write_text(json.dumps(dict(sha256=hashlib.sha256((lib/'pack_words_arm64_test.go').read_bytes()).hexdigest(),scope='128 alternating-FF shared-workspace cases added after timing; production sources unchanged'),indent=2)+'\n')
