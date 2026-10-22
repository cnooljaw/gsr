"""复算源码采样归属，保持完整CPU分母，不把profile比例当速度。"""
from pathlib import Path
import re,json,hashlib
here=Path(__file__).resolve().parent;plan=json.loads((here/'plan.json').read_text());commands=json.loads((here/'commands.json').read_text());assert all(c['exitCode']==0 for c in commands)
def seconds(s):
 if s=='.':return 0.0
 return float(s[:-2])/1000 if s.endswith('ms') else float(s[:-1])
rows=['# 最新Unpack源码采样','', '单位为秒；所有比例使用完整Total samples。预检flat不含其inline OnesCount8子帧，后者单列；make列为该行累计，其他列为flat，不能直接全部相加得总CPU。','', '| 消息/入口/遍 | 总采样 | kevent | Unpack flat/cum | 预检flat | OnesCount子帧 | 写出flat | make累计 | 普通字节赋值flat |','| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |'];summary=[]
for i in range(2):
 for case in plan['cases']:
  for op in plan['operations']:
   stem=f'pass{i+1}-{case}-{op}';source=(here/(stem+'-source.txt')).read_text();flat=(here/(stem+'-flat.txt')).read_text();total=seconds(re.search(r'Total: (\S+)',source)[1]);lines={}
   for line in source.splitlines():
    m=re.match(r'\s*(\.|[\d.]+(?:ms|s))\s+(\.|[\d.]+(?:ms|s))\s+(\d+):',line)
    if m:lines[int(m[3])]=(seconds(m[1]),seconds(m[2]))
   routine=re.search(r'\n\s*(\S+)\s+(\S+) \(flat, cum\)',source);f,c=map(seconds,routine.groups())
   kevent=0.0
   for line in flat.splitlines():
    if line.rstrip().endswith('runtime.kevent'):kevent=seconds(line.split()[0])
   r=dict(passIndex=i+1,case=case,operation=op,totalSeconds=total,keventSeconds=kevent,unpackFlatSeconds=f,unpackCumSeconds=c,preflightFlatSeconds=sum(v[0] for n,v in lines.items() if 194<=n<=233),popcountChildSeconds=lines[223][1]-lines[223][0],writeFlatSeconds=sum(v[0] for n,v in lines.items() if 235<=n<=272),makeCumSeconds=lines[234][1],ordinaryStoreFlatSeconds=lines[262][0]);summary.append(r)
   rows.append(f"| {case}/{op}/pass{i+1} | {total:.2f} | {kevent:.2f} | {f:.2f}/{c:.2f} | {r['preflightFlatSeconds']:.2f} | {r['popcountChildSeconds']:.2f} | {r['writeFlatSeconds']:.2f} | {r['makeCumSeconds']:.2f} | {r['ordinaryStoreFlatSeconds']:.2f} |")
(here/'SUMMARY.md').write_text('\n'.join(rows)+'\n');(here/'summary.json').write_text(json.dumps(summary,indent=2)+'\n');(here/'validation.json').write_text(json.dumps(dict(commands=len(commands),allCommandsPassed=True,profiles=len(summary),hashesVerifiedAfter=plan['hashesVerifiedAfter']),indent=2)+'\n');print('\n'.join(rows))
