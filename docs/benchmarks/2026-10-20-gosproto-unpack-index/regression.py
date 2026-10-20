"""计时后对完整源码副本增加全部 mask 回归，不修改计时归档或二进制。"""
import argparse,hashlib,json,os,shutil,subprocess,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();here=Path(__file__).resolve().parent;commands=[]
for side in ['A','B']:
 d=out/(side+'-regression-v2');shutil.copytree(out/side,d)
 (d/'unpack_word_view_test.go').write_bytes((here/'word-test.go.txt').read_bytes())
 for flags in [['-race'],['-gcflags=all=-d=checkptr=2']]:
  cmd=['go','test',*flags,'-run','^TestUnpackWordViewBounds$','-timeout=2m','.'];start=time.monotonic();r=subprocess.run(cmd,cwd=d,env={**os.environ,'GOMAXPROCS':'1','GOPROXY':'off'},capture_output=True,text=True,timeout=125);commands.append(dict(side=side,args=cmd,exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'regression-v2.json').write_text(json.dumps(commands,indent=2)+'\n')
  if r.returncode:raise SystemExit(r.stdout+r.stderr)
print('both snapshots race and strict checkptr passed')
