"""同模块 Pack header 写出顺序实验，不修改生产库。"""
import io,json,os,subprocess,tarfile,time,hashlib,sys
from pathlib import Path
here=Path(__file__).resolve().parent;root=here.parents[2];out=Path(sys.argv[1]).resolve();out.mkdir(exist_ok=False);lib=out/'gosproto';lib.mkdir();commands=[]
def run(args,cwd,env=None):
 start=time.monotonic();p=subprocess.run(args,cwd=cwd,env={**os.environ,**(env or {})},capture_output=True,text=True,timeout=120);commands.append(dict(args=args,cwd=str(cwd),env=env or {},exitCode=p.returncode,seconds=time.monotonic()-start,output=p.stdout+p.stderr));(here/'commands.json').write_text(json.dumps(commands,indent=2)+'\n');assert p.returncode==0,p.stdout+p.stderr;return p.stdout
repo='/private/tmp/gsr-gosproto-fork';commit=run(['git','rev-parse','HEAD'],repo).strip()
with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive',commit],cwd=repo))) as t:t.extractall(lib,filter='data')
s=(lib/'pack.go').read_text();body=s[s.index('func packInto('):s.index('// zeroHeaderPrefix')]
clone=body.replace('func packInto(', 'func packHeaderClone(')
candidate=body.replace('func packInto(', 'func packHeaderCandidate(')
old='\t\t\tpayload := dst[1:9]';assert candidate.count(old)==1
candidate=candidate.replace(old,'\t\t\tdst[0] = header\n'+old).replace('\t\t\tdst[0] = header\n\t\t\toffset += 1 + int(ranks>>56)','\t\t\toffset += 1 + int(ranks>>56)')
wrappers=''
for name,fn in [('Baseline','packInto'),('Clone','packHeaderClone'),('Candidate','packHeaderCandidate')]:
 wrappers+=f'func ExperimentPack{name}(src []byte) []byte {{return {fn}(make([]byte,packedBufferSize(len(src))),src)}}\n'
 wrappers+=f'func ExperimentPEncode{name}(m any)([]byte,error){{out,body,err:=encodeBufferWithLimits(m,Limits{{}},true,nil);if err!=nil{{return nil,err}};return {fn}(out,body),nil}}\n'
(lib/'header_experiment.go').write_text('package sproto\nimport("encoding/binary";"math/bits")\n'+clone+candidate+wrappers)
test=(root/'docs/benchmarks/2026-10-21-gosproto-pack-ff-bitmap/bitmap_test.go.txt').read_text();test=test[:test.index('// 按密度')];test=test.replace('bitmapSink','headerSink').replace('TestBitmapEquivalence','TestHeaderEquivalence').replace('packBitmap','packHeaderCandidate');(lib/'header_experiment_test.go').write_text(test)
h=out/'harness';h.mkdir()
for n in ['protocol_benchmark_test.go','representative_benchmark_test.go','cases.json','representative.json']:(h/n).write_bytes((root/'examples/websocket-gate/benchmarks'/n).read_bytes())
(h/'go.sum').write_bytes((root/'go.sum').read_bytes());(h/'go.mod').write_text(f'module packheader\n\ngo 1.24.0\nrequire (\ngithub.com/cnooljaw/gsr v0.0.0\ngithub.com/cnooljaw/gosproto v0.4.0\ngoogle.golang.org/protobuf v1.36.1\n)\nreplace github.com/cnooljaw/gsr => {root}\nreplace github.com/cnooljaw/gosproto => {lib}\n')
sampler=(root/'docs/benchmarks/2026-10-20-gosproto-unpack-inprocess/experiment.go.txt').read_text().replace(' "reflect"\n','')
a=sampler.index(' var packed []byte');b=sampler.index(' runs:=map[string]func()')
sampler=sampler[:a]+''' var packed []byte
 var message any
 for _,sample:=range representativeCases(t){if sample.Name!=name{continue};_,wires:=representativeOperations(t,sample);checkRepresentativeWire(t,sample,wires);packed=wires["sproto_encode"];_,newMessage:=representativeMessages(sample.Type);message=newMessage();if err:=json.Unmarshal(sample.Data,message);err!=nil{t.Fatal(err)}}
 if packed==nil {var word [8]byte;density:=4;if name=="density7"{density=7}else if name!="density4"{t.Fatal(name)};for i:=0;i<density;i++{word[i]=byte(i+1)};packed=bytes.Repeat(word[:],512)}
 operation:=os.Getenv("GSR_OP")
 left,right:=sp.ExperimentPackBaseline,sp.ExperimentPackCandidate
 le,re:=sp.ExperimentPEncodeBaseline,sp.ExperimentPEncodeCandidate
 switch mode{case "self":right=left;re=le;case "clone":right=sp.ExperimentPackClone;re=sp.ExperimentPEncodeClone;case "pass1","pass2":default:t.Fatal(mode)}
 beforeInput:=append([]byte(nil),packed...)
 expected:=sp.Pack(packed)
 for _,fn:=range []func([]byte)[]byte{left,right}{got:=fn(packed);if !bytes.Equal(got,expected)||cap(got)!=cap(expected){t.Fatal("wire/cap")}}
 if operation=="pencode"{want,err:=sp.EncodePacked(message);if err!=nil{t.Fatal(err)};for _,fn:=range []func(any)([]byte,error){le,re}{got,err:=fn(message);if err!=nil||!bytes.Equal(got,want)||cap(got)!=cap(want){t.Fatal("pencode wire/cap",err)}}}
 makeRun:=func(fn func([]byte)[]byte,enc func(any)([]byte,error))func(){return func(){if operation=="pack"{encodedSink=fn(packed);return};if operation!="pencode"{t.Fatal(operation)};var err error;encodedSink,err=enc(message);if err!=nil{t.Fatal(err)}}}
''' + sampler[b:]
sampler=sampler.replace('makeRun(left)','makeRun(left,le)').replace('makeRun(right)','makeRun(right,re)')
(h/'experiment_test.go').write_text(sampler);env={'GOMAXPROCS':'1','GOGC':'100','GOMEMLIMIT':'off','GOPROXY':'off'}
run(['gofmt','-w','header_experiment.go','header_experiment_test.go'],lib);run(['gofmt','-w','experiment_test.go'],h)
run(['go','test','-race','-gcflags=all=-d=checkptr=2','-run','^(TestHeaderEquivalence|TestPack|TestUnpack)','-timeout=2m','.'],lib,env)
run(['go','test','-mod=mod','-run','^TestRepresentativeFixtures$','-timeout=30s','.'],h,env)
run(['go','vet','-mod=mod','.'],h,env);binary=out/'experiment.test';run(['go','test','-mod=mod','-c','-o',str(binary),'.'],h,env)
assembly=run(['go','tool','objdump','-s',r'github.com/cnooljaw/gosproto\.(packInto|packHeaderClone|packHeaderCandidate)',str(binary)],h)
(here/'assembly.txt').write_text('\n'.join(x.rstrip() for x in assembly.splitlines())+'\n')
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
cases=[(n,op) for n in ['game_small','game_large','int32_edges'] for op in ['pack','pencode']]+[(n,'pack') for n in ['density4','density7']]
plan=dict(commit=commit,cases=cases,modes=['self','clone','pass1','pass2'],go=run(['go','version'],root).strip(),sources={str(p.relative_to(out)):sha(p) for d in [lib,h] for p in d.glob('*.go')},fixtures={n:sha(h/n) for n in ['cases.json','representative.json']},binarySha256=sha(binary),env=env)
(here/'plan.json').write_text(json.dumps(plan,indent=2)+'\n')
for n in ['header_experiment.go','header_experiment_test.go']:(here/(n+'.txt')).write_bytes((lib/n).read_bytes())
(here/'experiment.go.txt').write_bytes((h/'experiment_test.go').read_bytes())
print('preflight passed',flush=True)
for mode in plan['modes']:
 for name,op in cases:
  output=here/f'{mode}-{name}-{op}.json';run([str(binary),'-test.run=^TestUnpackInProcessExperiment$','-test.timeout=1m'],h,{**env,'GSR_CASE':name,'GSR_OP':op,'GSR_MODE':mode,'GSR_OUTPUT':str(output)});print(mode,name,op,flush=True)
for n,v in plan['sources'].items():assert sha(out/n)==v,n
assert sha(binary)==plan['binarySha256']
print('all samples and fingerprints passed',flush=True)
