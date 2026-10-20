"""从计时归档构建完整候选，先验证新增回归在基线成立，再完成跨层验收。"""
import argparse,hashlib,json,os,re,shutil,subprocess,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--experiment',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();out.mkdir(exist_ok=False);experiment=a.experiment.resolve();here=Path(__file__).resolve().parent;root=here.parents[2];commands=[];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
base=out/'baseline';shutil.copytree(experiment/'gosproto',base,ignore=shutil.ignore_patterns('record_experiment.go','record_experiment_test.go'))
test=(here/'proof-test.go.txt').read_text()+'\n'+(here/'differential-test.go.txt').read_text()
(base/'decode_integer_record_test.go').write_text(test)
lib=out/'candidate';shutil.copytree(base,lib)
s=(lib/'decode.go').read_text();start=s.index('func decodeStructSlice(');end=s.index('func decodeMessage(')
adapter=(experiment/'gosproto/record_experiment.go').read_text();a1=adapter.index('func experimentRecordCandidate(');b1=adapter.index('// canDecodeIntegerRecords',a1)
body=adapter[a1:b1].replace('experimentRecordCandidate','decodeStructSlice',1)
(lib/'decode.go').write_text(s[:start]+body+s[end:])
helper=adapter[b1:adapter.index('// ExperimentRecordSelector')]
(lib/'decode_integer_record.go').write_text('package sproto\n\nimport "unsafe"\n\n'+helper)
strip=lambda s:re.sub(r'\s+','',re.sub(r'//[^\n]*','',s));assert strip(body)==strip(adapter[a1:b1].replace('experimentRecordCandidate','decodeStructSlice',1))
def run(cmd,cwd,env=None,timeout=120):
 start=time.monotonic();r=subprocess.run(cmd,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=timeout);commands.append(dict(args=cmd,cwd=str(cwd),env=env or {},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'acceptance.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 print(cmd[0]+' '+cmd[1]+' passed',flush=True)
run(['gofmt','-w','decode_integer_record_test.go'],base)
run(['gofmt','-w','decode.go','decode_integer_record.go','decode_integer_record_test.go'],lib)
fixed={p.name:sha(p) for p in lib.glob('*.go')}
run(['go','test','-run','^TestIntegerRecordDecodeProof','-timeout=30s','.'],base,{'GOPROXY':'off'})
oracle=root/'tmp/gosproto-pack-payload/oracle';provenance=json.loads((oracle/'provenance.json').read_text())
for n,r in provenance['files'].items():assert sha(oracle/n)==r['sha256']
(out/'oracle-provenance.json').write_text(json.dumps(provenance,indent=2)+'\n')
mod=out/'integration.mod';mod.write_text((root/'go.mod').read_text()+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n');(out/'integration.sum').write_bytes((root/'go.sum').read_bytes())
env={'SPROTO_ORACLE_DIR':str(oracle),'GOPROXY':'off'}
run(['go','test','./...','-timeout=2m'],lib,env)
run(['go','test','-race','-gcflags=all=-d=checkptr=2','./...','-timeout=2m'],lib,{**env,'GOMAXPROCS':'2'})
run(['go','vet','./...'],lib,env)
for target in ['FuzzCodecRoundTrip','FuzzDecode']:
 run(['go','test','-run','^$','-fuzz','^'+target+'$','-fuzztime=10s','-parallel=2','.','-timeout=1m'],lib,{'GOMAXPROCS':'2','GOPROXY':'off'},timeout=70)
run(['go','test','-modfile='+str(mod),'./protocol/sproto','./examples/websocket-gate/cmd/debugserver','./examples/websocket-gate/benchmarks','-timeout=2m'],root)
run(['npm','--prefix','examples/websocket-gate','run','test:sproto:integration'],root,{'GOFLAGS':'-modfile='+str(mod)})
assert fixed=={p.name:sha(p) for p in lib.glob('*.go')}
(out/'accepted-source.json').write_text(json.dumps(fixed,indent=2)+'\n')
