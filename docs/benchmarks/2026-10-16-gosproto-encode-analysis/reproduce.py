#!/usr/bin/env python3
"""仅复现固定 game_large 的内部诊断，不安装或修改正式库。需 Go 1.24 与 gofmt。"""
from pathlib import Path
import argparse, hashlib, io, json, os, platform, statistics, subprocess, tarfile

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument('--library',required=True,help='包含 a579920 的独立 gosproto Git 仓库')
parser.add_argument('--output',required=True,help='不存在的诊断输出目录，建议放在忽略的 tmp 下')
parser.add_argument('--prepare-only',action='store_true')
args=parser.parse_args()
root=Path(__file__).resolve().parents[3]
base=Path(args.output).resolve()
if base.exists(): raise SystemExit('输出目录已存在，请指定新目录，避免覆盖旧证据')
base.mkdir(parents=True)
commit='a579920'
archive=subprocess.check_output(['git','archive',commit],cwd=Path(args.library).resolve())
fixture=next(x for x in json.loads((root/'examples/websocket-gate/benchmarks/representative.json').read_text()) if x['name']=='game_large')
common = '''package sproto
import("reflect")
type diagItem struct {
 Id *int64 `sproto:"integer,0" json:"id"`
 Count *int64 `sproto:"integer,1" json:"count"`
 Power *int64 `sproto:"integer,2" json:"power"`
}
type diagGame struct {
 PlayerId *int64 `sproto:"integer,0" json:"playerId"`
 Tick *int64 `sproto:"integer,1" json:"tick"`
 Hp *int64 `sproto:"integer,2" json:"hp"`
 Mp *int64 `sproto:"integer,3" json:"mp"`
 Level *int64 `sproto:"integer,4" json:"level"`
 X *int64 `sproto:"integer,5" json:"x"`
 Y *int64 `sproto:"integer,6" json:"y"`
 State *int64 `sproto:"integer,7" json:"state"`
 Attributes []int64 `sproto:"integer,8,array" json:"attributes"`
 SkillIds []int64 `sproto:"integer,9,array" json:"skillIds"`
 Items []*diagItem `sproto:"struct,10,array" json:"items"`
 Labels []string `sproto:"string,11,array" json:"labels"`
}
var diagItemType=reflect.TypeFor[diagItem]()
'''
helpers = '''
// 此探针只用于隔离固定 Item 模型的取值与通用分派成本，不是可发布 codec。
func diagItemSize(value reflect.Value,budget *decodeBudget,depth int,layout *encodeLayout)(int,error){
 if depth>budget.limits.MaxDepth || budget.limits.MaxBytes<2{return 0,ErrLimit}
 pos:=layout.add(0); item:=value.Interface().(*diagItem); if item==nil{return 2,nil}
 count,size,last:=0,2,-1
 for tag,p:=range [3]*int64{item.Id,item.Count,item.Power}{
  if p==nil{continue};if err:=budget.add(1);err!=nil{return 0,err}
  if tag-last>1 {count++;size+=2};last=tag;count++;size+=2
  bits:=uint64(*p);if bits>MaxEmbeddedInt{size+=4+integerWidth(bits)}
  if size>budget.limits.MaxBytes{return 0,ErrLimit}
 }
 layout.set(pos,count);return size,nil
}
func diagItemWrite(value reflect.Value,output []byte,layout *encodeLayout)int{
 count:=layout.next();writeUint16(output,uint16(count));item:=value.Interface().(*diagItem);if item==nil{return 2}
 head,body,last:=2,2+count*2,-1
 for tag,p:=range [3]*int64{item.Id,item.Count,item.Power}{
  if p==nil{continue};if skip:=skipTag(last,tag);skip>0{writeUint16(output[head:],skip);head+=2};last=tag
  bits:=uint64(*p);if bits<=MaxEmbeddedInt{writeUint16(output[head:],uint16((bits+1)*2))}else{
   width:=integerWidth(bits);writeUint16(output[head:],0);writeUint32(output[body:],uint32(width))
   if width==4{writeUint32(output[body+4:],uint32(bits))}else{writeUint64(output[body+4:],bits)};body+=4+width
  };head+=2
 };return body
}
'''
harness = '''package sproto
import("testing";"os";"encoding/json";"crypto/sha256";"fmt";"reflect")
var diagSink []byte
func diagInput(tb testing.TB)*diagGame{tb.Helper();b,err:=os.ReadFile("fixture.json");if err!=nil{tb.Fatal(err)};var v diagGame;if err=json.Unmarshal(b,&v);err!=nil{tb.Fatal(err)};w,err:=Encode(&v);if err!=nil{tb.Fatal(err)};if len(w)!=3336||fmt.Sprintf("%x",sha256.Sum256(w))!="dbf8228801e1c9bd5303836df2dfc8640f85cf9272d56564be228665d63e9559"{tb.Fatal("wire mismatch")};return &v}
// 固定原始夹具的官方字节签名，诊断分支不得改变输入或 wire。
func TestDiagnosticWire(t *testing.T){diagInput(t)}
func BenchmarkDiagnosticEncode(b *testing.B){v:=diagInput(b);b.ReportAllocs();b.ResetTimer();for range b.N{w,err:=Encode(v);if err!=nil{b.Fatal(err)};diagSink=w}}
func BenchmarkDiagnosticSize(b *testing.B){v:=diagInput(b);schema,err:=GetSprotoType(reflect.TypeFor[diagGame]());if err!=nil{b.Fatal(err)};limits,_:=normalizeLimits(Limits{});rv:=reflect.ValueOf(v);b.ReportAllocs();b.ResetTimer();for range b.N{var layout encodeLayout;budget:=decodeBudget{limits:limits};_,err:=encodedSize(schema,rv,&budget,1,&layout);if err!=nil{b.Fatal(err)}}}
func BenchmarkDiagnosticWrite(b *testing.B){v:=diagInput(b);schema,err:=GetSprotoType(reflect.TypeFor[diagGame]());if err!=nil{b.Fatal(err)};limits,_:=normalizeLimits(Limits{});rv:=reflect.ValueOf(v);var layout encodeLayout;budget:=decodeBudget{limits:limits};size,err:=encodedSize(schema,rv,&budget,1,&layout);if err!=nil{b.Fatal(err)};output:=make([]byte,size);b.ReportAllocs();b.ResetTimer();for range b.N{layout.cursor=0;encodeMessageInto(schema,rv,output,&layout)};diagSink=output}
'''
for name in ['baseline','inline','typed-item','reflect-item']:
    d=base/name;d.mkdir()
    with tarfile.open(fileobj=io.BytesIO(archive)) as t: t.extractall(d,filter='data')
    (d/'fixture.json').write_text(json.dumps(fixture['data']))
    specialized=name in ['typed-item','reflect-item']
    (d/'diagnostic.go').write_text(common+(helpers if specialized else ''))
    (d/'diagnostic_test.go').write_text(harness)
    if name!='baseline':
        p=d/'limits.go';s=p.read_text();old='return integerWidth(bits), bits <= MaxEmbeddedInt, nil'
        assert s.count(old)==1
        p.write_text(s.replace(old,'if bits <= MaxEmbeddedInt { return 0, true, nil }; return integerWidth(bits), false, nil'))
    if specialized:
        p=d/'limits.go';s=p.read_text();sig='func encodedSize(schema *SprotoType, value reflect.Value, budget *decodeBudget, depth int, layout *encodeLayout) (int, error) {'
        assert s.count(sig)==1
        p.write_text(s.replace(sig,sig+'\nif schema.typ == diagItemType { return diagItemSize(value,budget,depth,layout) }',1))
        p=d/'encode.go';s=p.read_text();sig='func encodeMessageInto(schema *SprotoType, value reflect.Value, output []byte, layout *encodeLayout) int {'
        assert s.count(sig)==1
        p.write_text(s.replace(sig,sig+'\nif schema.typ == diagItemType { return diagItemWrite(value,output,layout) }',1))
    subprocess.run(['gofmt','-w','diagnostic.go','diagnostic_test.go','limits.go','encode.go'],cwd=d,check=True)
    if name=='reflect-item':
        p=d/'diagnostic.go';s=p.read_text()
        s=s.replace('item := value.Interface().(*diagItem)\n\tif item == nil {','if value.IsNil() {')
        s=s.replace('for tag, p := range [3]*int64{item.Id, item.Count, item.Power} {\n\t\tif p == nil {','record := value.Elem()\n\tfor tag := 0; tag < 3; tag++ {\n        p := record.Field(tag)\n\t\tif p.IsNil() {')
        s=s.replace('bits := uint64(*p)','bits := uint64(p.Elem().Int())')
        assert 'item.' not in s;p.write_text(s)
        subprocess.run(['gofmt','-w','diagnostic.go'],cwd=d,check=True)
if args.prepare_only: raise SystemExit(0)
env={**os.environ,'GOMAXPROCS':'1'}
for name in ['baseline','inline','typed-item','reflect-item']:
    subprocess.run(['go','test','-c','-o',str(base/(name+'.test'))],cwd=base/name,env=env,check=True,timeout=45)
    subprocess.run([str(base/(name+'.test')),'-test.run=^TestDiagnosticWire$','-test.timeout=10s'],cwd=base/name,env=env,check=True,timeout=15)
for series,versions in [('results',['baseline','inline','typed-item']),('item-results',['baseline','reflect-item','typed-item'])]:
    rows=[]
    for r in range(5):
        for name in (versions if r%2==0 else list(reversed(versions))):
            command=[str(base/(name+'.test')),'-test.run=^$','-test.bench=^BenchmarkDiagnostic','-test.benchtime=60000x','-test.benchmem','-test.timeout=20s']
            p=subprocess.run(command,cwd=base/name,env=env,text=True,capture_output=True,check=True,timeout=25)
            (base/(series+'-'+name+'-'+str(r)+'.txt')).write_text(p.stdout+p.stderr)
            for line in p.stdout.splitlines():
                if line.startswith('Benchmark'):
                    a=line.split();rows.append(dict(round=r,variant=name,operation=a[0],ns=float(a[2]),bytes=int(a[4]),allocs=int(a[6])))
            print(series,r,name,flush=True)
    summary=[]
    for name in versions:
        for op in ['Encode','Size','Write']:
            values=[x for x in rows if x['variant']==name and x['operation']=='BenchmarkDiagnostic'+op]
            summary.append(dict(variant=name,operation=op,ns=statistics.median(x['ns'] for x in values),bytes=statistics.median(x['bytes'] for x in values),allocs=statistics.median(x['allocs'] for x in values)))
    metadata={'baseCommit':commit,'goVersion':subprocess.check_output(['go','version'],text=True).strip(),'platform':platform.platform(),'GOMAXPROCS':1,'rounds':5,'iterations':60000,'rows':rows,'summary':summary,'scope':'diagnostic only; no production library modified','files':{}}
    for name in versions:
        for p in [base/(name+'.test')]+[base/name/n for n in ['encode.go','limits.go','diagnostic.go','diagnostic_test.go','fixture.json']]:
            metadata['files'][str(p.relative_to(base))]=hashlib.sha256(p.read_bytes()).hexdigest()
    (base/(series+'.json')).write_text(json.dumps(metadata,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(summary,ensure_ascii=False,indent=2),flush=True)
