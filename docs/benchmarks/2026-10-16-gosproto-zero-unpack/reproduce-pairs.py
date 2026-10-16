from pathlib import Path
import shutil, argparse, subprocess, tarfile, io
parser=argparse.ArgumentParser(description='复现同进程旧库/新库/PB 的 Unpack 与解码组合路径专项，需要含 07350a9 的独立库')
parser.add_argument('--library',required=True)
parser.add_argument('--output',required=True)
parser.add_argument('--prepare-only',action='store_true')
args=parser.parse_args()
root=Path(__file__).resolve().parents[3]
base=Path(args.output).resolve()
if base.exists(): raise SystemExit('输出目录已存在，请指定新目录')
base.mkdir(parents=True)
fork=Path(args.library).resolve()
ab=base/'ab';ab.mkdir();before=base/'ab-baseline';before.mkdir()
archive=subprocess.check_output(['git','archive','07350a9'],cwd=fork)
with tarfile.open(fileobj=io.BytesIO(archive)) as t:t.extractall(before,filter='data')
p=before/'go.mod';p.write_text(p.read_text().replace('module github.com/cnooljaw/gosproto','module github.com/cnooljaw/gosproto-before',1))
(ab/'go.mod').write_text(f'''module encoderbench

go 1.24.0

require (
 github.com/cnooljaw/gsr v0.0.0
 github.com/cnooljaw/gosproto v0.4.0
 github.com/cnooljaw/gosproto-before v0.0.0
 google.golang.org/protobuf v1.36.1
)
replace github.com/cnooljaw/gsr => {root}
replace github.com/cnooljaw/gosproto => {fork}
replace github.com/cnooljaw/gosproto-before => {before}
''')
(ab/'go.sum').write_bytes((root/'go.sum').read_bytes())
bench=root/'examples/websocket-gate/benchmarks'
for name in ['representative_benchmark_test.go','protocol_benchmark_test.go','representative.json','cases.json','correctness.json']:(ab/name).write_bytes((bench/name).read_bytes())
s=(bench/'protocol_benchmark_test.go').read_text();fn=s[s.index('func echoCodecs('):].replace('func echoCodecs(','func beforeEchoCodecs(',1).replace('sproto.','before.')
(ab/'before_echo_test.go').write_text('''package benchmarks
import (
 before "github.com/cnooljaw/gosproto-before"
 echopb "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/example/echo/v1"
 "github.com/cnooljaw/gsr/examples/websocket-gate/gen/sproto/c2s"
 commonpb "github.com/cnooljaw/gsr/protocol/gen/go/gsr/common/v1"
 "google.golang.org/protobuf/proto"
)
'''+fn)
(ab/'paired_test.go').write_text('''package benchmarks
import (
 "bytes"
 "encoding/json"
 "os"
 "strings"
 "testing"
 before "github.com/cnooljaw/gosproto-before"
)
func runPaired(b *testing.B,name string,operations []representativeOperation){
 if os.Getenv("GSR_AB_REVERSE")=="1" {for i,j:=0,len(operations)-1;i<j;i,j=i+1,j-1 {operations[i],operations[j]=operations[j],operations[i]}}
 for _,op:=range operations {b.Run(name+"/"+op.name,func(b *testing.B){b.ReportAllocs();b.ResetTimer();for range b.N {if err:=op.run();err!=nil {b.Fatal(err)}}})}
}
// 同一进程交替旧库、新库和 PB，所有入口与正式夹具保持一致。
func BenchmarkPairedEncoder(b *testing.B){
 for _,sample:=range representativeCases(b){
  ops,wires:=representativeOperations(b,sample);checkRepresentativeWire(b,sample,wires)
  _,newSproto:=representativeMessages(sample.Type);message:=newSproto();if err:=json.Unmarshal(sample.Data,message);err!=nil {b.Fatal(err)}
  wire,err:=before.Encode(message);if err!=nil||!bytes.Equal(wire,wires["sproto_encode"]) {b.Fatalf("before wire: %v",err)}
  pair:=[]representativeOperation{{"before_encode",func()error{var err error;encodedSink,err=before.Encode(message);return err}}}
  for _,op:=range ops {if op.name=="sproto_encode"||op.name=="protobuf_encode" {pair=append(pair,op)}}
  pair=append(pair,representativeOperation{"before_pencode",func()error{var err error;encodedSink,err=before.EncodePacked(message);return err}})
  for _,op:=range ops {if op.name=="sproto_pencode" {pair=append(pair,op)}}
  runPaired(b,sample.Name,pair)
 }
}
// Echo 复用两份相同的 factory，仅替换库导入；同时比较小包与 packing 帧。
func BenchmarkPairedEcho(b *testing.B){
 content,err:=os.ReadFile("cases.json");if err!=nil {b.Fatal(err)};var cases []benchmarkCase;if err=json.Unmarshal(content,&cases);err!=nil{b.Fatal(err)}
 for _,sample:=range cases {
  text:=strings.Repeat(sample.Pattern,sample.Repeat);old,now:=beforeEchoCodecs(text),echoCodecs(text)
  var ops []representativeOperation
  for _,shape:=range []string{"payload","frame"}{
   left,right,control:=old["sproto_"+shape].encode,now["sproto_"+shape].encode,now["protobuf_"+shape].encode
   want,err:=left();if err!=nil{b.Fatal(err)};wire,err:=right();if err!=nil||!bytes.Equal(want,wire){b.Fatalf("Echo wire %s/%s %v",sample.Name,shape,err)}
   for _,op:=range []struct{name string;encode func()([]byte,error)}{{"before_"+shape,left},{"sproto_"+shape,right},{"protobuf_"+shape,control}}{
    ops=append(ops,representativeOperation{op.name,func()error{var err error;encodedSink,err=op.encode();return err}})
   }
  };runPaired(b,sample.Name,ops)
 }
}
''')

(ab/'packing_test.go').write_bytes(Path(__file__).resolve().parent.parent.joinpath('2026-10-16-gosproto-ffscan/packing-probe.go.txt').read_bytes())
(ab/'unpacking_test.go').write_bytes(Path(__file__).with_name('unpacking-probe.go.txt').read_bytes())
(ab/'unpack_benchmark_test.go').write_bytes((bench/'unpack_benchmark_test.go').read_bytes())
subprocess.run(["gofmt","-w","before_echo_test.go","paired_test.go"],cwd=ab,check=True)
if args.prepare_only: raise SystemExit(0)
subprocess.run(["go","test","-mod=mod","-c","-o",str(base/"ab.test"),"."],cwd=ab,check=True,timeout=60)
from pathlib import Path
import subprocess,os,json,statistics,hashlib
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
sources={p.name:sha(p)for p in fork.glob('*.go')}
binary=base/'ab.test';rows=[];commands=[]
for r in range(5):
 args=[str(binary),'-test.run=^TestRepresentativeFixtures$','-test.bench=^BenchmarkPaired(Unpacking|UnpackEcho|UnpackShapes)$','-test.benchtime=60000x','-test.benchmem','-test.timeout=55s'];env={**os.environ,'GOMAXPROCS':'1','GSR_AB_REVERSE':str(r%2)}
 p=subprocess.run(args,cwd=ab,env=env,text=True,capture_output=True,timeout=60);(base/('ab-'+str(r)+'.txt')).write_text(p.stdout+p.stderr);p.check_returncode();commands.append({'args':args,'reverse':r%2})
 for line in p.stdout.splitlines():
  if line.startswith('Benchmark'):
   a=line.split();rows.append(dict(round=r,operation=a[0],ns=float(a[a.index('ns/op')-1]),bytes=int(a[a.index('B/op')-1]),allocs=int(a[a.index('allocs/op')-1])))
 print('同进程配对',r,'完成',flush=True)
summary=[]
for op in sorted(set(x['operation']for x in rows)):
 values=[x for x in rows if x['operation']==op];summary.append({'operation':op,**{k:statistics.median(v[k]for v in values)for k in ['ns','bytes','allocs']}})
assert sources=={p.name:sha(p)for p in fork.glob('*.go')},'采样期间库源码变化'
metadata={'commands':commands,'rows':rows,'summary':summary,'binarySha256':sha(binary),'goVersion':subprocess.check_output(['go','version'],text=True).strip(),'goSource':sources,'harnessSha256':{p.name:sha(p)for p in ab.glob('*.go')},'goMod':(ab/'go.mod').read_text(),'baselineCommit':'07350a9','scope':'同进程固定 60000 次，前后顺序按轮交替；专项复核，不替换完整矩阵'}
(base/'ab-pairs.json').write_text(json.dumps(metadata,ensure_ascii=False,indent=2)+'\n')
for row in summary:print(row,flush=True)
