# 最终Go Protobuf / Sproto正常入口对照

时间为三轮中位µs/op，完整三轮均保留。PB固定v1.36.1；未测TS，不含网络和真实RPC。

| 消息 | PB Encode | SP Encode | SP PEncode | PB Decode | SP Decode | SP PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 0.333 | 0.523 | 0.586 | 0.814 | 1.005 | 1.153 |
| game_small | 0.652 | 0.612 | 0.687 | 1.647 | 1.471 | 2.092 |
| game_large | 7.448 | 4.010 | 4.485 | 17.174 | 12.408 | 15.995 |
| int32_edges | 5.576 | 1.361 | 1.825 | 4.103 | 1.323 | 5.391 |
| echo_ascii4k | 0.561 | 0.613 | 0.736 | 0.611 | 0.650 | 1.176 |

正数为SP耗时更高；正文和packed分列，不合并宣布胜负。

| 消息 | Encode vs PB | PEncode vs PB | Decode vs PB | PDecode vs PB |
| --- | ---: | ---: | ---: | ---: |
| addressbook | +57.2% | +76.2% | +23.5% | +41.7% |
| game_small | -6.3% | +5.2% | -10.7% | +27.0% |
| game_large | -46.2% | -39.8% | -27.8% | -6.9% |
| int32_edges | -75.6% | -67.3% | -67.8% | +31.4% |
| echo_ascii4k | +9.2% | +31.1% | +6.4% | +92.6% |

## Echo payload/frame 全部32项

frame为现有benchmark的手工封装，不代表库实际Rpc.Request/Dispatch；帧格式不同，分别保留。

| 用例 | µs/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| ascii32/protobuf_payload/encode | 0.067 | 48 | 1 |
| ascii32/protobuf_payload/decode | 0.109 | 96 | 2 |
| ascii32/sproto_payload/encode | 0.080 | 48 | 1 |
| ascii32/sproto_payload/decode | 0.142 | 112 | 5 |
| ascii32/protobuf_frame/encode | 0.185 | 176 | 3 |
| ascii32/protobuf_frame/decode | 0.245 | 224 | 4 |
| ascii32/sproto_frame/encode | 0.269 | 184 | 5 |
| ascii32/sproto_frame/decode | 0.354 | 256 | 11 |
| utf8_1k/protobuf_payload/encode | 1.280 | 1024 | 1 |
| utf8_1k/protobuf_payload/decode | 1.335 | 1088 | 2 |
| utf8_1k/sproto_payload/encode | 0.217 | 1024 | 1 |
| utf8_1k/sproto_payload/decode | 0.279 | 1104 | 5 |
| utf8_1k/protobuf_frame/encode | 1.531 | 2128 | 3 |
| utf8_1k/protobuf_frame/decode | 1.592 | 2192 | 4 |
| utf8_1k/sproto_frame/encode | 0.626 | 3096 | 5 |
| utf8_1k/sproto_frame/decode | 0.574 | 2224 | 11 |
| ascii4k/protobuf_payload/encode | 0.961 | 4864 | 1 |
| ascii4k/protobuf_payload/decode | 0.938 | 4160 | 2 |
| ascii4k/sproto_payload/encode | 0.551 | 4864 | 1 |
| ascii4k/sproto_payload/decode | 0.606 | 4176 | 5 |
| ascii4k/protobuf_frame/encode | 1.394 | 9808 | 3 |
| ascii4k/protobuf_frame/decode | 1.487 | 9104 | 4 |
| ascii4k/sproto_frame/encode | 1.574 | 14616 | 5 |
| ascii4k/sproto_frame/decode | 1.213 | 9136 | 11 |
| zeros4k/protobuf_payload/encode | 0.871 | 4864 | 1 |
| zeros4k/protobuf_payload/decode | 0.960 | 4160 | 2 |
| zeros4k/sproto_payload/encode | 0.565 | 4864 | 1 |
| zeros4k/sproto_payload/decode | 0.586 | 4176 | 5 |
| zeros4k/protobuf_frame/encode | 1.408 | 9808 | 3 |
| zeros4k/protobuf_frame/decode | 1.536 | 9104 | 4 |
| zeros4k/sproto_frame/encode | 2.055 | 14616 | 5 |
| zeros4k/sproto_frame/decode | 1.263 | 9136 | 11 |
