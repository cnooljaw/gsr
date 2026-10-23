"""复核本轮全部控制实验、最终代码/测试增量与正式基准来源，不覆盖适配器模板。"""
from pathlib import Path
import json,hashlib,subprocess,re
here=Path(__file__).resolve().parent;root=here.parents[2];sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest();cohorts={}
for name,path in [('dispatch','tmp/gosproto-closure-dispatch-pairs-20261022'),('pack','tmp/gosproto-closure-pack-pairs-20261022'),('minrun','tmp/gosproto-closure-minrun-pairs-20261022')]:
 directory=here/name;out=root/path;plan=json.loads((directory/'plan.json').read_text());assert all(sha(out/n)==v for n,v in plan['sources'].items());assert sha(out/'pairs.test')==plan['binarySha256'];assert all(c['exitCode']==0 for c in json.loads((directory/'commands.json').read_text()));lib=out/'gosproto'
 if name=='dispatch':
  a=(lib/'experiment.go').read_text();b=(lib/'experiment_clone.go').read_text();assert a[a.index('func ExperimentUnpackOriginal('):]==b[b.index('func ExperimentUnpackClone('):].replace('func ExperimentUnpackClone(','func ExperimentUnpackOriginal(',1)
 else:
  a=(lib/'experiment_pack.go').read_text();b=(lib/'experiment_pack_clone.go').read_text();assert a.replace('func ExperimentPackInto(','func ExperimentPackIntoClone(',1)==b
 for f in lib.glob('experiment*.go'):(directory/('source-'+f.name+'.txt')).write_bytes(f.read_bytes().rstrip(b'\n')+b'\n')
 cohorts[name]={'processes':len(plan['cases'])*len(plan['modes']),'commands':len(json.loads((directory/'commands.json').read_text())),'sourceAndBinaryVerifiedAfter':True,'cloneSourceEqual':True}
a=here/'final';snapshot=root/'tmp/gosproto-closure-final-minrun-20261022/candidate';accepted=json.loads((a/'accepted-source.json').read_text());assert all(sha(snapshot/n)==v for n,v in accepted.items());assert not any('experiment' in n for n in accepted);assert all(c['exitCode']==0 for c in json.loads((a/'acceptance.json').read_text()));binary=root/'tmp/gosproto-closure-final-minrun-20261022/final.test';assert sha(binary)==json.loads((a/'accepted-build.json').read_text())['binarySha256'];assert sha(binary)==json.loads((here/'final-shapes-build.json').read_text())['binarySha256']
work=Path('/private/tmp/gsr-gosproto-fork');written=json.loads((here/'worktree-source.json').read_text());extra=json.loads((here/'extra-test-source.json').read_text())['sha256']
for n,v in written.items():assert sha(work/n)==v;assert v==(extra if n=='pack_words_arm64_test.go' else accepted[n]),n
old=json.loads((here.parent/'2026-10-22-gosproto-preflight-arm64/compact/final-analysis.json').read_text());new=json.loads((a/'final-analysis.json').read_text());allocationChanges=[n for n in new if n.startswith('representative/') and (new[n]['bytes'],new[n]['allocs'])!=(old[n]['bytes'],old[n]['allocs'])];assert not allocationChanges
for suite in ['PackShapes','UnpackShapes']:
 lines=[l for l in (here/(suite+'.txt')).read_text().splitlines() if l.startswith('Benchmark')];assert len(lines)==36,(suite,len(lines))
assert all(c['exitCode']==0 for c in json.loads((here/'extra-checks.json').read_text()));assert all(c['exitCode']==0 for c in json.loads((here/'final-shapes-commands.json').read_text()));assert json.loads((here/'audit.json').read_text())['binaryVerifiedAfter']
audit=json.loads((here/'audit.json').read_text());auditBinary=root/'tmp/gosproto-closure-final-guard-20261022/final.test';assert sha(auditBinary)==audit['binarySha256']
for name,value in audit['profiles'].items():assert sha(root/'tmp/gosproto-closure-audit-20261022'/(name+'.cpu'))==value
fallback=json.loads((here/'final-fallback-audit.json').read_text());assert fallback['instructions']==296 and fallback['opcodeSequencesEqual'];assert sha(root/'tmp/gosproto-closure-20261022/pure-audit-final.test')==fallback['binarySha256']
(here/'validation.json').write_text(json.dumps(dict(cohorts=cohorts,totalProcesses=sum(r['processes'] for r in cohorts.values()),finalAcceptanceCommands=len(json.loads((a/'acceptance.json').read_text())),worktreeMatchesAcceptedProduction=True,extraTestVerifiedSeparately=True,representativeAllocationChanges=allocationChanges,shapeRows=72),indent=2)+'\n');print('全部实验、最终生产文件、测试增量及基准指纹核对完成')
