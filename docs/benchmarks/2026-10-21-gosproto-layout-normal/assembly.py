"""核对正常二进制的编码机器指令，源码一致不等于调用环境无偏差。"""
import argparse,hashlib,json,re,subprocess,time
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve();here=Path(__file__).resolve().parent
plan=json.loads((out/'plan.json').read_text());logs=[];summaries={};normalized={}
for label in plan['versions']:
 binary=out/(label+'.test');assert hashlib.sha256(binary.read_bytes()).hexdigest()==plan['binarySha256'][label]
 cmd=['go','tool','objdump','-s',r'github.com/cnooljaw/gosproto\.(encodeFieldInto|encodeIntegerRecordInto|encodeIntegerRecordArrayInto)',str(binary)]
 start=time.monotonic();r=subprocess.run(cmd,capture_output=True,text=True,timeout=30);logs.append(dict(args=cmd,exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));assert r.returncode==0
 (here/(label+'-assembly.txt')).write_text('\n'.join(x.rstrip() for x in r.stdout.splitlines())+'\n')
 rows=[];normalized[label]={}
 for chunk in r.stdout.split('TEXT ')[1:]:
  lines=chunk.splitlines();name=lines[0].split('(SB)')[0];instructions=[]
  for line in lines[1:]:
   m=re.search(r'0x[0-9a-f]+\s+[0-9a-f]{8}\s+(.+)',line)
   if m:instructions.append(m[1].strip())
  digest=hashlib.sha256('\n'.join(instructions).encode()).hexdigest();normalized[label][name]=digest
  rows.append(dict(symbol=name,instructionRowsIncludingPadding=len(instructions),normalizedInstructionSha256=digest))
 summaries[label]=rows
(here/'assembly-commands.json').write_text(json.dumps(logs,indent=2)+'\n')
(here/'assembly-summary.json').write_text(json.dumps({'symbols':summaries,'baselineCloneNormalizedInstructionsEqual':normalized['baseline']==normalized['clone000'],'scope':'Normalized objdump mnemonics/operands retain relative branch distances and call symbols; not a proof of all program/data layout equality.'},indent=2)+'\n')
print('baseline/clone normalized target instructions equal:',normalized['baseline']==normalized['clone000'])
