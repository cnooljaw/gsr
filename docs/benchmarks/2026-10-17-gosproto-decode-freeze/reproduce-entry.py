from pathlib import Path
import subprocess,os,json,statistics,hashlib
import argparse, shutil, tarfile, io
parser=argparse.ArgumentParser(description='复现 Decode 冻结前的独立入口实验')
parser.add_argument('--library',required=True)
parser.add_argument('--output',required=True)
parser.add_argument('--prepare-only',action='store_true')
args=parser.parse_args()
base=Path(args.output).resolve()
if base.exists():raise SystemExit('输出目录已存在，请指定新目录')
base.mkdir(parents=True)
fork=base/'micro';fork.mkdir()
archive=subprocess.check_output(['git','archive','a09fb00'],cwd=args.library)
with tarfile.open(fileobj=io.BytesIO(archive))as t:t.extractall(fork,filter='data')
report=Path(__file__).resolve().parent
shutil.copy2(report/'entry_benchmark.go.txt',fork/'entry_benchmark_test.go')
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
assert json.loads((report/'micro.json').read_text())['goSource']=={p.name:sha(p)for p in fork.glob('*.go')}
if args.prepare_only:raise SystemExit(0)
with (base/'compiler.txt').open('w')as log:
 subprocess.run(['go','test','-c','-o',str(base/'micro.test'),'-gcflags=github.com/cnooljaw/gosproto=-m=2','.'],cwd=fork,stdout=log,stderr=subprocess.STDOUT,check=True,timeout=60)
binary=base/'micro.test';fork=base/'micro';sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest();sources={p.name:sha(p)for p in fork.glob('*.go')};rows=[];commands=[]
for r in range(5):
 args=[str(binary),'-test.run=^TestEntryCapabilities$','-test.bench=^Benchmark(DecodeEntry|EntryComponents)$','-test.benchtime=2000000x','-test.benchmem','-test.timeout=20s']
 p=subprocess.run(args,cwd=fork,env={**os.environ,'GOMAXPROCS':'1','GSR_AB_REVERSE':str(r%2)},text=True,capture_output=True,timeout=25);p.check_returncode();(base/('micro-'+str(r)+'.txt')).write_text(p.stdout+p.stderr);commands.append({'args':args,'reverse':r%2})
 for line in p.stdout.splitlines():
  if line.startswith('Benchmark'):
   a=line.split();rows.append({'round':r,'operation':a[0],'ns':float(a[a.index('ns/op')-1]),'bytes':int(a[a.index('B/op')-1]),'allocs':int(a[a.index('allocs/op')-1])})
summary=[]
for op in sorted(set(x['operation']for x in rows)):
 values=[x for x in rows if x['operation']==op];summary.append({'operation':op,**{k:statistics.median(v[k]for v in values)for k in ['ns','bytes','allocs']}})
assert len(rows)==90
assert sources=={p.name:sha(p)for p in fork.glob('*.go')}
metadata={'baseCommit':'a09fb00','goSource':sources,'binarySha256':sha(binary),'goVersion':subprocess.check_output(['go','version'],text=True).strip(),'iterations':2000000,'rounds':5,'commands':commands,'rows':rows,'summary':summary,'scope':'Same-process entry only; all candidate helpers coexist. Native, no native, actual-type mismatch; exactNative recalculated each iteration. Global sink assignment common to variants; no subtraction of component timings.'}
(base/'micro.json').write_text(json.dumps(metadata,ensure_ascii=False,indent=2)+'\n')
for row in summary:print(row,flush=True)
