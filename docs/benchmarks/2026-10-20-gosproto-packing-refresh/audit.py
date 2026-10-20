"""在独立副本核对三种实际wire的byte pattern；不改已采profile的源码/二进制。"""
import argparse,hashlib,json,os,shutil,subprocess,time
from pathlib import Path
p=argparse.ArgumentParser(description=__doc__);p.add_argument('profiles',type=Path);p.add_argument('output',type=Path);a=p.parse_args();src=a.profiles.resolve();out=a.output.resolve();here=Path(__file__).resolve().parent
out.mkdir(exist_ok=False)
shutil.copytree(src/'harness',out/'harness')
(out/'harness/wire_audit_test.go').write_bytes((here/'wire-audit.go.txt').read_bytes())
start=time.monotonic();r=subprocess.run(['go','test','-mod=mod','-run','^TestPackWordAudit$','.','-timeout=30s'],cwd=out/'harness',env={**os.environ,'GSR_WIRE_AUDIT':str(out/'wire-audit.json'),'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off'},capture_output=True,text=True)
(out/'commands.json').write_text(json.dumps({'args':r.args,'seconds':time.monotonic()-start,'exitCode':r.returncode,'output':r.stdout+r.stderr,'auditSourceSha256':hashlib.sha256((here/'wire-audit.go.txt').read_bytes()).hexdigest()},indent=2)+'\n')
if r.returncode:raise SystemExit(r.stdout+r.stderr)
print((out/'wire-audit.json').read_text())
