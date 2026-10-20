"""核对混合mask诊断输入的分组/重排只改变顺序，保留段集合、直方图及总长度。"""
import argparse,hashlib,json
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--output',type=Path,required=True);a=p.parse_args();out=a.output.resolve()
plan=json.loads((out/'plan.json').read_text());files=[out/('fixture-'+name+'-'+op+'.json') for name,op in plan['cases']];facts={f.name[8:-5]:json.loads(f.read_text()) for f in files}
assert len(files)==20
keys=['PackedBytes','OutputBytes','BagSHA256','Histogram']
groups=[['game_large-unpack','game_trace_grouped-unpack','game_trace_shuffled-unpack'],['int32_edges-unpack','int_trace_grouped-unpack','int_trace_shuffled-unpack'],['mask4_cycle-unpack','mask4_shuffled-unpack'],['counts_cycle-unpack','counts_grouped-unpack','counts_shuffled-unpack'],['density4-unpack','mask4_fixed-unpack']]
for group in groups:
 for name in group[1:]:
  assert all(facts[group[0]][k]==facts[name][k] for k in keys),(group,name)
for name in ['mask4_fixed-unpack','mask4_cycle-unpack','mask4_shuffled-unpack']:
 f=facts[name];assert f['PackedBytes']==2560 and f['OutputBytes']==4096
 assert sum(f['Histogram'])==512
 assert all(n==0 or int(mask).bit_count()==4 for mask,n in enumerate(f['Histogram']))
assert facts['density4-unpack']['PackedSHA256']==facts['mask4_fixed-unpack']['PackedSHA256']
for name in ['game_large','int32_edges']:assert facts[name+'-unpack']==facts[name+'-pdecode']
for group in groups[:4]:
 assert len({facts[n]['PackedSHA256'] for n in group})==len(group),group
validation={'inputCount':len(files),'groupsVerified':groups,'allInvariantsPassed':True,'sha256':{f.name:hashlib.sha256(f.read_bytes()).hexdigest() for f in files}}
(out/'fixture-validation.json').write_text(json.dumps(validation,indent=2)+'\n');print('20 inputs and five invariant groups verified')
