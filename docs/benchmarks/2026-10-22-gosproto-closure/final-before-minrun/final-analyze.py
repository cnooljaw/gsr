"""完整复算最终72项三轮PB/SP比较，不删除离群值，不跨入口替代。"""
from pathlib import Path
import re,json,statistics,collections
here=Path(__file__).resolve().parent;summary={}
for suite,count in [('representative',40),('protocol',32)]:
 grouped=collections.defaultdict(list)
 for line in (here/('final-'+suite+'.txt')).read_text().splitlines():
  m=re.match(r'^Benchmark(?:Representative|Protocol)Codec/(\S+)\s+\d+\s+([\d.]+) ns/op(?:\s+[\d.]+ MB/s)?\s+(\d+) B/op\s+(\d+) allocs/op$',line)
  if m:grouped[m[1]].append(dict(ns=float(m[2]),bytes=int(m[3]),allocs=int(m[4])))
 assert len(grouped)==count,(suite,len(grouped))
 for name,rows in grouped.items():
  assert len(rows)==3,(suite,name,len(rows));summary[suite+'/'+name]=dict(ns=statistics.median(x['ns'] for x in rows),bytes=statistics.median(x['bytes'] for x in rows),allocs=statistics.median(x['allocs'] for x in rows),samples=rows,minNs=min(x['ns'] for x in rows),maxNs=max(x['ns'] for x in rows))
(here/'final-analysis.json').write_text(json.dumps(summary,indent=2)+'\n')
ns=lambda n,op:summary['representative/'+n+'/'+op]['ns']
rows=['# 最终Go Protobuf / Sproto正常入口对照','','时间为三轮中位µs/op，完整三轮均保留。PB固定v1.36.1；未测TS，不含网络和真实RPC。','','| 消息 | PB Encode | SP Encode | SP PEncode | PB Decode | SP Decode | SP PDecode |','| --- | ---: | ---: | ---: | ---: | ---: | ---: |']
ops=['protobuf_encode','sproto_encode','sproto_pencode','protobuf_decode','sproto_decode','sproto_pdecode']
for n in ['addressbook','game_small','game_large','int32_edges','echo_ascii4k']:rows.append('| '+n+' | '+' | '.join(f'{ns(n,op)/1000:.3f}' for op in ops)+' |')
rows+=['','正数为SP耗时更高；正文和packed分列，不合并宣布胜负。','','| 消息 | Encode vs PB | PEncode vs PB | Decode vs PB | PDecode vs PB |','| --- | ---: | ---: | ---: | ---: |']
for n in ['addressbook','game_small','game_large','int32_edges','echo_ascii4k']:
 comparisons=[('sproto_encode','protobuf_encode'),('sproto_pencode','protobuf_encode'),('sproto_decode','protobuf_decode'),('sproto_pdecode','protobuf_decode')]
 rows.append('| '+n+' | '+' | '.join(f'{(ns(n,a)/ns(n,b)-1)*100:+.1f}%' for a,b in comparisons)+' |')
rows+=['','## Echo payload/frame 全部32项','','frame为现有benchmark的手工封装，不代表库实际Rpc.Request/Dispatch；帧格式不同，分别保留。','','| 用例 | µs/op | B/op | allocs/op |','| --- | ---: | ---: | ---: |']
for k,v in summary.items():
 if k.startswith('protocol/'):rows.append('| '+k.removeprefix('protocol/')+f" | {v['ns']/1000:.3f} | {v['bytes']} | {v['allocs']} |")
(here/'COMPARISON.md').write_text('\n'.join(rows)+'\n');print('\n'.join(rows[:20]))
