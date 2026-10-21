"""补充编码专用记录函数的源码视图，保留原始profile与二进制校验。"""
import argparse,hashlib,json,subprocess,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve()
plan=json.loads((out/'inputs.json').read_text());sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
assert sha(out/'body.test')==plan['binarySha256']
commands=[]
for case in plan['cases']:
 for op in ['sproto_encode','sproto_pencode']:
  stem=case+'-'+op+'-cpu';raw=out/(stem+'.cpu');assert sha(raw)==plan['profiles'][stem]['rawSha256']
  cmd=['go','tool','pprof','-list=encodedIntegerRecordSize|encodeIntegerRecordInto|encodeMessageInto|encodeFieldInto|encodedSize|fieldSize',str(out/'body.test'),str(raw)]
  start=time.monotonic();r=subprocess.run(cmd,capture_output=True,text=True,timeout=30)
  commands.append(dict(args=cmd,exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'extra-commands.json').write_text(json.dumps(commands,indent=2)+'\n')
  if r.returncode:raise SystemExit(r.stdout+r.stderr)
  (out/(stem+'-encode-source.txt')).write_text('\n'.join(l.rstrip() for l in r.stdout.splitlines())+'\n')
print('6 supplementary encoding views verified')
