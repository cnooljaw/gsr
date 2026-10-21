"""仅生成隔离的 FF 位图初筛实验；不修改生产代码。"""
from pathlib import Path
import sys
p=Path(sys.argv[1]);s=(p/'pack.go').read_text();s=s[s.index('func packInto('):s.index('// zeroHeaderPrefix')]
s=s.replace('func packInto(', 'func packBitmap(')
line='\t\theader := uint8(((nonzero >> 7) * 0x0102040810204080) >> 56)\n'
s=s.replace(line,'').replace('if header == 0xff {','if nonzero == 0x8080808080808080 {')
s=s.replace('\t\t// 去掉至多两个置位后是否为零',line+'\t\t// 去掉至多两个置位后是否为零')
(p/'bitmap.go').write_text('package sproto\nimport("encoding/binary";"math/bits")\n'+s)
(p/'bitmap_test.go').write_text('''package sproto
import("bytes";"testing")
var bitmapSink []byte
// 每种 mask、尾长及 FF 长度边界都必须保持 wire、输入快照和重叠写入结果。
func TestBitmapEquivalence(t *testing.T){
 for mask:=0;mask<256;mask++{for _,n:=range []int{0,1,7,8,9,15,16,31,255,256,2047,2048,2049,4096}{
 src:=make([]byte,n);for i:=range src{if mask&(1<<uint(i&7))!=0{src[i]=byte(i%255+1)}}
 snapshot:=append([]byte(nil),src...);want:=Pack(src)
 got:=packBitmap(make([]byte,packedBufferSize(n)),src)
 if !bytes.Equal(got,want)||!bytes.Equal(src,snapshot){t.Fatalf("mask=%d n=%d",mask,n)}
 // 与 EncodePacked 相同，源位于输出预留区域的后端。
 reserve:=packedBufferSize(n);buf:=make([]byte,reserve);start:=reserve-n;copy(buf[start:],src)
 got=packBitmap(buf,buf[start:]);if !bytes.Equal(got,want){t.Fatalf("overlap mask=%d n=%d",mask,n)}
 }}
}
// 按密度初筛位图分支；复用输出缓冲，只测 packInto 算法，不代表真实业务 Pack/PEncode。
func BenchmarkBitmap(b *testing.B){for _,d:=range []int{0,1,2,3,4,5,6,7,8}{
 src:=make([]byte,4096);for i:=range src{if i%8<d{src[i]=byte(i%255+1)}}
 for _,candidate:=range []bool{false,true}{name:="baseline";if candidate{name="candidate"};b.Run(string(rune('0'+d))+"/"+name,func(b *testing.B){dst:=make([]byte,packedBufferSize(len(src)));b.ResetTimer();if candidate{for i:=0;i<b.N;i++{bitmapSink=packBitmap(dst,src)}}else{for i:=0;i<b.N;i++{bitmapSink=packInto(dst,src)}}})}
}}
''')
