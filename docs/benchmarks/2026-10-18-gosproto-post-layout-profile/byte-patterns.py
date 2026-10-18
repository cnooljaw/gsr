"""在另一份相同源码归档中统计实际输入mask；不修改采样harness或生产库。"""
import argparse,hashlib,json,os,subprocess,time
from pathlib import Path
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--library',type=Path,required=True)
p.add_argument('--output',type=Path,required=True)
a=p.parse_args();here=Path(__file__).resolve().parent;root=here.parents[2];out=a.output.resolve();commands=[]
digest=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()


def run(args,cwd,env=None):
    started=time.monotonic();r=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=60)
    commands.append({'args':args,'cwd':str(cwd),'env':env or {},'exitCode':r.returncode,'elapsedSeconds':round(time.monotonic()-started,3),'output':r.stdout+r.stderr})
    if out.exists():(out/'pattern-commands.json').write_text(json.dumps(commands,indent=2)+'\n')
    if r.returncode:raise SystemExit(r.stdout+r.stderr)


run(['python3','-B',str(here/'reproduce.py'),'--library',str(a.library.resolve()),'--output',str(out),'--prepare-only'],root)
harness=out/'archive/harness';(harness/'byte_pattern_test.go').write_bytes((here/'byte-pattern-probe.go.txt').read_bytes())
run(['gofmt','-w','byte_pattern_test.go'],harness)
run(['go','vet','-mod=mod','.'],harness)
run(['go','test','-mod=mod','-run','^TestPackBytePatterns$','-v','.','-timeout=1m'],harness,{'GSR_PATTERN_OUTPUT':str(out/'patterns.json'),'GOMAXPROCS':'1'})
(out/'pattern-inputs.json').write_text(json.dumps({'planSha256':digest(here/'byte-pattern-plan.json'),'probeSha256':digest(here/'byte-pattern-probe.go.txt'),'driverSha256':digest(here/'byte-patterns.py'),'patternSha256':digest(out/'patterns.json'),'supplementalHarnessGoSource':{p.name:digest(p) for p in sorted(harness.glob('*.go'))}},indent=2)+'\n')
print('24 morphology observations complete;no timing')
