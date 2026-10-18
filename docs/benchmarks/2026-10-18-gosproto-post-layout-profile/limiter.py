"""在独立归档中读取GC limiter事件；不重跑A/A、不计时。"""
import argparse,hashlib,json,os,subprocess,time
from pathlib import Path
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--library',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args()
here=Path(__file__).resolve().parent;root=here.parents[2];out=a.output.resolve();commands=[];digest=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()


def run(args,cwd,env=None):
    start=time.monotonic();r=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=60)
    commands.append({'args':args,'cwd':str(cwd),'env':env or {},'exitCode':r.returncode,'elapsedSeconds':round(time.monotonic()-start,3),'output':r.stdout+r.stderr})
    if out.exists():(out/'limiter-commands.json').write_text(json.dumps(commands,indent=2)+'\n')
    if r.returncode:raise SystemExit(r.stdout+r.stderr)


run(['python3','-B',str(here/'reproduce.py'),'--library',str(a.library.resolve()),'--output',str(out),'--prepare-only'],root)
harness=out/'archive/harness';(harness/'limiter_test.go').write_bytes((here/'limiter-probe.go.txt').read_bytes());run(['gofmt','-w','limiter_test.go'],harness);run(['go','vet','-mod=mod','.'],harness)
plan=json.loads((here/'plan.json').read_text())
for live in ['full','isolated']:
    run(['go','test','-mod=mod','-run','^TestPackLimiterEvents$','-v','.','-timeout=1m'],harness,{'GOMAXPROCS':'1','GSR_GROWTH_FIXTURE_PLAN':str(here/'fixture-plan.json'),'GSR_GROWTH_CASES':','.join(plan['growthCases']),'GSR_CONTEXT_CASE':'body/echo_ascii4k/sproto_pack','GSR_CONTEXT_LIVE':live,'GSR_LIMITER_OUTPUT':str(out/(live+'.json'))})
(out/'limiter-inputs.json').write_text(json.dumps({'planSha256':digest(here/'limiter-plan.json'),'probeSha256':digest(here/'limiter-probe.go.txt'),'driverSha256':digest(here/'limiter.py'),'rawSha256':{live:digest(out/(live+'.json')) for live in ['full','isolated']},'supplementalHarnessGoSource':{p.name:digest(p) for p in sorted(harness.glob('*.go'))}},indent=2)+'\n')
print('fixed limiter event diagnosis complete;no timing')
