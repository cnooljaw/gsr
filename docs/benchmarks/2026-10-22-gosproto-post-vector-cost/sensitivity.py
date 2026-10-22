"""只诊断整数数组CPU采样对GC设置的敏感性，不产生正式性能排名。"""
from pathlib import Path
import os,json,subprocess,time,hashlib,sys
out=Path(sys.argv[1]).resolve();binary=out/'body.test';h=out/'harness';commands=[];sha=hashlib.sha256(binary.read_bytes()).hexdigest()
def run(args,env):
 t=time.monotonic();p=subprocess.run(args,cwd=h,env={**os.environ,**env},capture_output=True,text=True,timeout=60);commands.append(dict(args=args,env=env,exitCode=p.returncode,seconds=time.monotonic()-t,output=p.stdout+p.stderr));(out/'sensitivity-commands.json').write_text(json.dumps(commands,indent=2)+'\n');assert p.returncode==0,p.stdout+p.stderr;return p.stdout+p.stderr
for op in ['sproto_decode','sproto_unpack','sproto_pdecode']:
 stem='gc1000-int32_edges-'+op;raw=out/(stem+'.cpu');meta=out/(stem+'.json');env={'GOMAXPROCS':'1','GOGC':'1000','GOMEMLIMIT':'128MiB','GSR_BODY_CASE':'int32_edges','GSR_BODY_OP':op,'GSR_PROFILE_KIND':'cpu','GSR_PROFILE_META':str(meta),'GSR_PROFILE_CPU':str(raw)}
 (out/(stem+'-benchmark.txt')).write_text(run([str(binary),'-test.run=^$','-test.bench=^BenchmarkBodyCost$','-test.benchmem','-test.benchtime=2s','-test.timeout=1m'],env))
 for suffix,flags in [('cum',['-top','-cum','-nodecount=40']),('flat',['-top','-nodecount=40']),('source',['-list=UnpackWithLimit|unpackVectorARM64|decodeIntSlice'])]:
  text=run(['go','tool','pprof',*flags,str(binary),str(raw)],{});(out/(stem+'-'+suffix+'.txt')).write_text('\n'.join(l.rstrip() for l in text.splitlines())+'\n')
 assert hashlib.sha256(binary.read_bytes()).hexdigest()==sha
 print(op,'sensitivity done',flush=True)
