"""在完整正常库中验收最终选择，完成官方兼容、跨架构编译、集成及最终PB对照。"""
from pathlib import Path
import io,tarfile,json,os,subprocess,time,hashlib,sys
here=Path(__file__).resolve().parent;root=here.parents[3];out=Path(sys.argv[1]).resolve();out.mkdir(exist_ok=False);lib=out/'candidate';lib.mkdir();commands=[];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive','92a155a'],cwd='/private/tmp/gsr-gosproto-fork'))) as t:t.extractall(lib,filter='data')
for n in ['pack.go', 'unpack_size_arm64.go', 'unpack_size_other.go', 'unpack_size_arm64.s', 'unpack_size_arm64_test.go', 'unpack_size_guard_arm64_test.go', 'unpack_size_reference_arm64_test.go']:(lib/n).write_bytes((here/('selected-'+n+'.txt')).read_bytes())
def run(args,cwd,env=None,timeout=120):
 t=time.monotonic();r=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=timeout);commands.append(dict(args=args,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-t,output=r.stdout+r.stderr));(here/'acceptance.json').write_text(json.dumps(commands,indent=2)+'\n');assert r.returncode==0,r.stdout+r.stderr;print('passed',args[:4],flush=True);return r.stdout+r.stderr
run(['gofmt','-w','pack.go','unpack_size_arm64.go','unpack_size_other.go','unpack_size_arm64_test.go','unpack_size_guard_arm64_test.go','unpack_size_reference_arm64_test.go'],lib)
oracle=root/'tmp/gosproto-pack-payload/oracle';prov=json.loads((oracle/'provenance.json').read_text())
for n,r in prov['files'].items():assert sha(oracle/n)==r['sha256']
(here/'oracle-provenance.json').write_text(json.dumps(prov,indent=2)+'\n');env={'GOPROXY':'off','SPROTO_ORACLE_DIR':str(oracle)}
run(['go','test','./...','-timeout=2m'],lib,env)
run(['go','test','-tags=purego','./...','-timeout=2m'],lib,env)
run(['go','test','-race','-gcflags=all=-d=checkptr=2','./...','-timeout=2m'],lib,{**env,'GOMAXPROCS':'2'})
run(['go','vet','./...'],lib,env)
for target in ['FuzzCodecRoundTrip','FuzzPackRoundTrip']:run(['go','test','-run','^$','-fuzz','^'+target+'$','-fuzztime=10s','-parallel=2','.','-timeout=1m'],lib,{'GOPROXY':'off','GOMAXPROCS':'2'},timeout=70)
for arch in ['amd64','s390x']:
 run(['go','test','-c','-o',str(out/(arch+'.test')),'.'],lib,{'GOPROXY':'off','CGO_ENABLED':'0','GOOS':'linux','GOARCH':arch,'GOMAXPROCS':'2'})
# 既有测试含MaxInt64转int常量，32位仅编译生产代码，不伪造测试可用性。
for arch in ['386','arm']:
 run(['go','build','./...'],lib,{'GOPROXY':'off','CGO_ENABLED':'0','GOOS':'linux','GOARCH':arch,'GOMAXPROCS':'2'})
mod=out/'integration.mod';mod.write_text((root/'go.mod').read_text()+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n');(out/'integration.sum').write_bytes((root/'go.sum').read_bytes())
run(['go','test','-modfile='+str(mod),'./protocol/sproto','./examples/websocket-gate/cmd/debugserver','./examples/websocket-gate/benchmarks','-timeout=2m'],root)
run(['npm','--prefix','examples/websocket-gate','run','test:sproto:integration'],root,{'GOFLAGS':'-modfile='+str(mod)})
(here/'integration-build-info.txt').write_text(run(['go','version','-m','tmp/sproto-echo'],root))
binary=out/'final.test';run(['go','test','-modfile='+str(mod),'-c','-o',str(binary),'./examples/websocket-gate/benchmarks'],root)
source={p.name:sha(p) for p in [*lib.glob('*.go'),*lib.glob('*.s')]};binaryHash=sha(binary);bench=root/'examples/websocket-gate/benchmarks';e={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off'}
for suite in ['Representative','Protocol']:
 (here/('final-'+suite.lower()+'.txt')).write_text(run([str(binary),'-test.run=^$','-test.bench=^Benchmark'+suite+'Codec$','-test.benchmem','-test.benchtime=200ms','-test.count=3','-test.timeout=2m'],bench,e))
assert {p.name:sha(p) for p in [*lib.glob('*.go'),*lib.glob('*.s')]}==source and sha(binary)==binaryHash
(here/'accepted-source.json').write_text(json.dumps(source,indent=2)+'\n');(here/'accepted-build.json').write_text(json.dumps(dict(binarySha256=binaryHash,hashesVerifiedAfter=True),indent=2)+'\n')
