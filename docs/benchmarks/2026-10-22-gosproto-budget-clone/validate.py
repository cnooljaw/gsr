"""复核同源clone的源码、链接符号、机器码形态及测量指纹，不校正性能数据。"""
from pathlib import Path
import json,subprocess,hashlib,re,sys
here=Path(__file__).resolve().parent;out=Path(sys.argv[1]).resolve();lib=out/'gosproto';binary=out/'pairs.test';sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest();plan=json.loads((here/'plan.json').read_text())
for n,v in plan['sources'].items():assert sha(out/n)==v,n
assert sha(binary)==plan['binarySha256'];assert all(c['exitCode']==0 for c in json.loads((here/'commands.json').read_text()))
original=(lib/'experiment.go').read_text();clone=(lib/'experiment_clone.go').read_text();start=original.index('func ExperimentUnpackOriginal(');start2=clone.index('func ExperimentUnpackClone(')
assert original[start:]==clone[start2:].replace('func ExperimentUnpackClone(','func ExperimentUnpackOriginal(',1)
public=(lib/'experiment_public.go').read_text();publicClone=(lib/'experiment_public_clone.go').read_text();assert public==publicClone.replace('ExperimentDecodePackedClone','ExperimentDecodePacked').replace('ExperimentUnpackClone','ExperimentUnpackOriginal')
previous=here.parent/'2026-10-22-gosproto-preflight-budget/candidate-pack.go.txt';assert sha(lib/'pack.go')==sha(previous)
records={};opcodes={}
for name in ['ExperimentUnpackOriginal','ExperimentUnpackClone','ExperimentDecodePacked','ExperimentDecodePackedClone','UnpackWithLimit','DecodePacked']:
 symbol='github.com/cnooljaw/gosproto.'+name
 p=subprocess.run(['go','tool','objdump','-s','^'+re.escape(symbol)+'$',str(binary)],capture_output=True,text=True);assert p.returncode==0 and p.stdout
 text='\n'.join(l.rstrip() for l in p.stdout.splitlines())+'\n';(here/(name+'-objdump.txt')).write_text(text)
 lines=[re.match(r'^\s+\S+\s+(0x[0-9a-f]+)\s+[0-9a-f]+\s+(\S+)',l) for l in text.splitlines()];lines=[m for m in lines if m];records[name]={'address':lines[0][1],'instructionCount':len(lines)};opcodes[name]=[m[2] for m in lines]
assert len({v['address'] for v in records.values()})==len(records)
assert opcodes['ExperimentUnpackOriginal']==opcodes['ExperimentUnpackClone'];assert opcodes['ExperimentDecodePacked']==opcodes['ExperimentDecodePackedClone']
for n in ['pack.go','experiment.go','experiment_clone.go','experiment_public.go','experiment_public_clone.go']:(here/('baseline-experiment.go.txt' if n=='experiment.go' else n+'.txt')).write_bytes((lib/n).read_bytes())
(here/'validation.json').write_text(json.dumps(dict(sourceAndBinaryVerifiedAfter=True,candidateMatchesPriorSnapshot=True,baselineCloneBodiesEqual=True,publicCloneBodiesEqual=True,cloneOpcodeSequencesEqual=True,symbols=records,commands=len(json.loads((here/'commands.json').read_text()))),indent=2)+'\n');print(records)
