"""保存两版前后指纹、clone源码身份及已验收生产源码对应关系，避免覆盖harness模板。"""
from pathlib import Path
import json,hashlib,subprocess,sys
here=Path(__file__).resolve().parent;root=here.parents[2];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest();records={}
for directory,output in [(here,root/'tmp/gosproto-preflight-arm64-pairs-20261022-run2'),(here/'compact',root/'tmp/gosproto-preflight-arm64-compact-20261022')]:
 plan=json.loads((directory/'plan.json').read_text());assert all(sha(output/n)==v for n,v in plan['sources'].items());assert sha(output/'pairs.test')==plan['binarySha256'];assert all(c['exitCode']==0 for c in json.loads((directory/'commands.json').read_text()))
 lib=output/'gosproto';old=(lib/'experiment.go').read_text();clone=(lib/'experiment_clone.go').read_text();assert old[old.index('func ExperimentUnpackOriginal('):]==clone[clone.index('func ExperimentUnpackClone('):].replace('func ExperimentUnpackClone(','func ExperimentUnpackOriginal(',1)
 public=(lib/'experiment_public.go').read_text();assert public==(lib/'experiment_public_clone.go').read_text().replace('ExperimentDecodePackedClone','ExperimentDecodePacked').replace('ExperimentUnpackClone','ExperimentUnpackOriginal')
 for n in ['experiment.go','experiment_clone.go','experiment_public.go','experiment_public_clone.go']:(directory/('source-'+n+'.txt')).write_bytes((lib/n).read_bytes())
 p=subprocess.run(['go','tool','objdump','-s','github.com/cnooljaw/gosproto.unpackedSizeARM64',str(output/'pairs.test')],capture_output=True,text=True);assert p.returncode==0 and p.stdout;(directory/'size-objdump.txt').write_text('\n'.join(l.rstrip() for l in p.stdout.splitlines())+'\n')
 records[directory.name]=dict(sourceAndBinaryVerifiedAfter=True,cloneBodiesEqual=True,commands=len(json.loads((directory/'commands.json').read_text())))
q=here/'compact';accepted=json.loads((q/'accepted-source.json').read_text());prod=root/'tmp/gosproto-preflight-arm64-accept-20261022/candidate';work=Path('/private/tmp/gsr-gosproto-fork');names=list(json.loads((q/'worktree-source.json').read_text()))
for n in names:assert sha(prod/n)==accepted[n]==sha(work/n),n
assert all(c['exitCode']==0 for c in json.loads((q/'acceptance.json').read_text()))
old=json.loads((here.parent/'2026-10-22-gosproto-unpack-vector/final-analysis.json').read_text());new=json.loads((q/'final-analysis.json').read_text());allocationChanges=[n for n in new if n.startswith('representative/') and (new[n]['bytes'],new[n]['allocs'])!=(old[n]['bytes'],old[n]['allocs'])];assert not allocationChanges
(here/'validation.json').write_text(json.dumps(dict(cohorts=records,acceptanceCommands=len(json.loads((q/'acceptance.json').read_text())),worktreeMatchesAcceptedSnapshot=True,representativeAllocationChanges=allocationChanges),indent=2)+'\n');print('两版指纹、clone及7个生产文件核对完成')
