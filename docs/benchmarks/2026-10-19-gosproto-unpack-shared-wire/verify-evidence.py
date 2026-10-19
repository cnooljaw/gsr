"""独立复算本报告已归档wire元数据与恢复机器指令，不验证主库当前状态。"""
from pathlib import Path
import json,hashlib,re
root=Path.cwd();r=root/'docs/benchmarks/2026-10-19-gosproto-unpack-shared-wire';read=lambda p:json.loads(p.read_text());write=lambda p,d:p.write_text(json.dumps(d,indent=2)+'\n')
a=read(r/'analysis.json');plan=read(r/'plan.json');metadata=[];by_sample={}
for mode in plan['modes']:
 raw=read(r/mode/'raw.json')
 for row in raw['results']:
  w=row['wire'];parts=row['name'].split('/');assert parts[0]=='shared';sample,variant,op=parts[1:]
  expected=re.fullmatch(r'align([01])_spare(0|64)',variant);assert expected
  assert (w['Sample'],w['Variant'],w['Alignment'],w['Spare'])==(sample,variant,int(expected[1]),int(expected[2]))
  assert w['Capacity']==w['Length']+w['Spare'] and w['Length']>0
  assert w['SameBytesAcrossVariants'] and w['SameInputAcrossArms']
  assert w['SharedBetweenUnpackAndPDecode']==(sample in ['game_large','int32_edges','width8_len896'])
  assert re.fullmatch('[0-9a-f]{64}',w['PackedSHA256']) and re.fullmatch('[0-9a-f]{64}',w['RawSHA256'])
  fixed={k:w[k] for k in ['PackedSHA256','RawSHA256','Length','OrdinaryWords','ZeroMasks','FFRuns']}
  if sample in by_sample:assert by_sample[sample]==fixed,(mode,sample)
  else:by_sample[sample]=fixed
  metadata.append({'mode':mode,'name':row['name'],**w})
assert len(metadata)==160 and len(by_sample)==7

def extract_loop(path):
 text=path.read_text();blocks=re.split(r'(?=^TEXT )',text,flags=re.M)
 block=next(b for b in blocks if b.startswith('TEXT github.com/cnooljaw/gosproto.UnpackWithLimit('))
 ins=[]
 for line in block.splitlines()[1:]:
  match=re.search(r'0x([0-9a-f]+)\s+([0-9a-f]{8})\s+(.*)$',line)
  if match:ins.append({'address':int(match[1],16),'word':match[2],'instruction':match[3]})
 ends=[(i,z) for i,z in enumerate(ins) if re.search(r'CBNZW.*-15\(PC\)',z['instruction'])];assert len(ends)==1
 i,z=ends[0];start=z['address']-15*4;j=next(j for j,p in enumerate(ins) if p['address']==start);loop=ins[j:i+1];assert len(loop)==16
 words=[x['word'] for x in loop]
 return {'path':str(path.relative_to(root)),'sourceSha256':hashlib.sha256(path.read_bytes()).hexdigest(),'functionEntry':hex(ins[0]['address']),'startAddress':hex(start),'endAddress':hex(z['address']),'entryOffset':start-ins[0]['address'],'instructions':len(loop),'words':words,'wordsSha256':hashlib.sha256(bytes.fromhex(''.join(words))).hexdigest(),'startModulo64':start%64}
loops={m:extract_loop(r/(m+'-unpack-assembly.txt')) for m in plan['modes']}
assert loops['self']['words']==loops['clone']['words']==loops['pass1']['words']==loops['pass2']['words']
old=root/'docs/benchmarks/2026-10-19-gosproto-unpack-preflight-cursor'
previous={m:extract_loop(old/(m+'-unpack-assembly.txt')) for m in ['self','pass1']}
assert previous['self']['words']==previous['pass1']['words']==loops['self']['words']
delta=int(previous['pass1']['startAddress'],16)-int(previous['self']['startAddress'],16);assert delta==20
write(r/'wire-metadata.json',metadata);write(r/'wire-samples.json',by_sample);write(r/'restore-loop.json',{'current':loops,'previous70':previous,'identical16InstructionWords':True,'previous70AbsoluteStartDeltaBytes':delta,'limitations':'Equality covers ordinary mask innerblock only,notwhole restore/preflight orprocessorcache/predictorstate. Addresses are evidence,notproofofcause.No alignmentpadding added toproduction.'})
summary=[]
for sample in by_sample:
 row={'sample':sample,'operations':{}}
 for operation in ['unpack','pdecode']:
  picked=[x for x in a['results'] if x['name'].split('/')[1]==sample and x['name'].endswith('/'+operation)]
  if not picked:continue
  values=[(x[m]['medianRatio']-1)*100 for x in picked for m in ['pass1','pass2']]
  row['operations'][operation]={'cases':len(picked),'medianDeltaPctRange':[min(values),max(values)],'fasterCompareMedians':sum(v<0 for v in values),'slowerCompareMedians':sum(v>0 for v in values)}
 summary.append(row)
write(r/'context-summary.json',summary)
v=read(r/'validation.json');v.update(scope='Shared wire/cap/alignment/callcontext diagnosis only,same previouslyunadoptedpreflightcursor;no productionchanges',wireMetadataRecords=160,allActualAlignmentAndCapVerified=True,sameHashesAcrossAllVariantsModesAndContexts=True,wireSamples=7,restoreInner16WordsSameAcrossVersions=True,previous70RestoreLoopStartDeltaBytes=20,productionCandidateRetained=False,fullAcceptanceNotRunBecauseNoProductionChange=True,uncommittedReport=True)
write(r/'validation.json',v)
print(json.dumps({'records':160,'samples':7,'loopWords':16,'oldAddressDelta':delta,'summary':summary},indent=2))
