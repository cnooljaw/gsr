"""正常公开入口的独立二进制/进程配对；库内无选择器，保留所有块与控制。"""
import argparse,hashlib,io,json,math,os,platform,subprocess,tarfile,time
from pathlib import Path
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--library',type=Path,required=True);p.add_argument('--output',type=Path,required=True);a=p.parse_args()
here=Path(__file__).resolve().parent;root=here.parents[2];out=a.output.resolve();out.mkdir(exist_ok=False)
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest();commands=[]
env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
def run(argv,cwd,extra=None,timeout=60):
 start=time.monotonic();r=subprocess.run(argv,cwd=cwd,env={**os.environ,**env,**(extra or {})},text=True,capture_output=True,timeout=timeout)
 commands.append(dict(args=argv,cwd=str(cwd),env={**env,**(extra or {})},exitCode=r.returncode,seconds=time.monotonic()-start,output=r.stdout+r.stderr));(out/'commands.json').write_text(json.dumps(commands,indent=2)+'\n')
 if r.returncode:raise SystemExit(r.stdout+r.stderr)
 return r.stdout+r.stderr
versions={label:run(['git','rev-parse',ref],a.library).strip() for label,ref in [('baseline','2e1d44b'),('clone000','2e1d44b'),('candidate','451998e')]}
sources={};harnesses={};binaries={}
for label,commit in versions.items():
 lib=out/label;lib.mkdir()
 with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd=a.library))) as archive:archive.extractall(lib,filter='data')
 assert not any('ExperimentStringMode' in f.read_text() for f in lib.glob('*.go'))
 h=out/(label+'-harness');h.mkdir()
 for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
 (h/'normal_worker_test.go').write_bytes((here/'worker.go.txt').read_bytes())
 (h/'go.mod').write_text('module normalpairs\n\ngo 1.24.0\n\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => '+str(root)+'\nreplace github.com/cnooljaw/gosproto => '+str(lib)+'\n')
 (h/'go.sum').write_bytes((root/'go.sum').read_bytes())
 run(['gofmt','-w','normal_worker_test.go'],h)
 run(['go','test','-mod=mod','-run','^TestRepresentativeFixtures$','-timeout=30s','.'],h)
 run(['go','vet','-mod=mod','.'],h)
 binary=out/(label+'.test');run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h)
 (out/(label+'-modules.txt')).write_text(run(['go','list','-m','-json','github.com/cnooljaw/gosproto'],h))
 (out/(label+'-buildinfo.txt')).write_text('\n'.join(l.rstrip() for l in run(['go','version','-m',str(binary)],root).splitlines())+'\n')
 sources[label]={f.name:sha(f) for f in lib.glob('*.go')};harnesses[label]={f.name:sha(f) for f in h.glob('*.go')};binaries[label]=sha(binary)
assert sources['baseline']==sources['clone000']
assert harnesses['baseline']==harnesses['clone000']==harnesses['candidate']
cases=[(n,op) for n in ['addressbook','game_small','game_large','int32_edges','echo_ascii4k','records7','records8','records33','records128','prefix31'] for op in ['encode','pencode']]
inputs=[here/'worker.go.txt',here/'reproduce.py',*[f for n in ['protocol/gen','examples/websocket-gate/gen','examples/websocket-gate/benchmarks/gen'] for f in (root/n).rglob('*.go')],*[root/'examples/websocket-gate/benchmarks'/n for n in ['cases.json','representative.json','protocol_benchmark_test.go','representative_benchmark_test.go']]]
plan=dict(date='2026-10-21',gsrHead=run(['git','rev-parse','HEAD'],root).strip(),versions=versions,go=run(['go','version'],root).strip(),platform=platform.platform(),loadBefore=os.getloadavg(),sources=sources,harnesses=harnesses,binarySha256=binaries,frozen={str(f.relative_to(root)):sha(f) for f in inputs},cases=cases,modes=['self','clone','pass1','pass2'],rounds=6,quartetsPerRound=1,targetBlockMs=40,warmupMs=20,scope='One normal API block per process; no mutable library selector, same canonical module per binary; all GC/tails retained.')
(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n')
def block(label,name,op,calls):
 raw=out/'current-block.json'
 run([str(out/(label+'.test')),'-test.run=^TestNormalEncodeBlock$','-test.timeout=10s'],out/(label+'-harness'),dict(GSR_CASE=name,GSR_OP=op,GSR_CALLS=str(calls),GSR_OUTPUT=str(raw)),timeout=15)
 data=json.loads(raw.read_text());assert (data['Name'],data['Operation'],data['Calls'])==(name,op,calls)
 assert data['Nanoseconds']>0
 return data
calibration={};hashes={}
for name,op in cases:
 sample=block('baseline',name,op,256);calls=max(1,min(1<<20,int(40_000_000*256/sample['Nanoseconds'])))
 calibration[name+'/'+op]=dict(calls=calls,sample=sample);hashes[name+'/'+op]=(sample['BodyHash'],sample['PackedHash'])
 for label in ['clone000','candidate']:
  other=block(label,name,op,1);assert (other['BodyHash'],other['PackedHash'])==hashes[name+'/'+op]
plan['calibration']=calibration;(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('all three binaries built; 60 normal API preflights/calibrations passed',flush=True)
for mode in plan['modes']:
 for name,op in cases:
  calls=calibration[name+'/'+op]['calls'];right={'self':'baseline','clone':'clone000','pass1':'candidate','pass2':'candidate'}[mode]
  result=dict(mode=mode,name=name,operation=op,callsPerBlock=calls,quartets=[])
  output=out/(mode+'-'+name+'-'+op+'.json')
  for rnd in range(6):
   order='ABBA' if rnd%2==0 else 'BAAB';q=dict(round=rnd,order=order,blocks=[]);value=0
   for arm in order:
    label='baseline' if arm=='A' else right;d=block(label,name,op,calls)
    assert (d['BodyHash'],d['PackedHash'])==hashes[name+'/'+op]
    d.update(arm=arm,label=label);q['blocks'].append(d);value+=(1 if arm=='B' else -1)*math.log(d['Nanoseconds']/calls)
   q['ratio']=math.exp(value/2);result['quartets'].append(q);output.write_text(json.dumps(result,indent=2)+'\n')
  print(mode,name,op,'done',flush=True)
for label,files in sources.items():
 assert {f.name:sha(f) for f in (out/label).glob('*.go')}==files
 assert {f.name:sha(f) for f in (out/(label+'-harness')).glob('*.go')}==harnesses[label]
 assert sha(out/(label+'.test'))==binaries[label]
for n,h in plan['frozen'].items():assert sha(root/n)==h,n
plan['loadAfter']=os.getloadavg();(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n');print('all normal source/input/binary hashes unchanged',flush=True)
