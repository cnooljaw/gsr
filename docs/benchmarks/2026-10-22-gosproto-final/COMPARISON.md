# 最终Go Protobuf / Sproto正常入口对照

时间为三轮中位µs/op，完整三轮均保留。PB固定v1.36.1；未测TS，不含网络和真实RPC。

| 消息 | PB Encode | SP Encode | SP PEncode | PB Decode | SP Decode | SP PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 0.330 | 0.536 | 0.609 | 0.808 | 0.944 | 1.138 |
| game_small | 0.718 | 0.602 | 0.786 | 1.636 | 1.467 | 1.945 |
| game_large | 7.552 | 4.032 | 5.561 | 17.057 | 12.196 | 16.636 |
| int32_edges | 5.362 | 1.352 | 2.906 | 4.242 | 1.369 | 6.413 |
| echo_ascii4k | 0.588 | 0.619 | 0.725 | 0.625 | 0.623 | 1.108 |

正数为SP耗时更高；正文和packed分列，不合并宣布胜负。

| 消息 | Encode vs PB | PEncode vs PB | Decode vs PB | PDecode vs PB |
| --- | ---: | ---: | ---: | ---: |
| addressbook | +62.5% | +84.5% | +16.8% | +40.8% |
| game_small | -16.1% | +9.4% | -10.3% | +18.9% |
| game_large | -46.6% | -26.4% | -28.5% | -2.5% |
| int32_edges | -74.8% | -45.8% | -67.7% | +51.2% |
| echo_ascii4k | +5.2% | +23.3% | -0.2% | +77.4% |

## Echo payload/frame 全部32项

frame为现有benchmark的手工封装，不代表库实际Rpc.Request/Dispatch；帧格式不同，分别保留。

| 用例 | µs/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| ascii32/protobuf_payload/encode | 0.069 | 48 | 1 |
| ascii32/protobuf_payload/decode | 0.114 | 96 | 2 |
| ascii32/sproto_payload/encode | 0.094 | 48 | 1 |
| ascii32/sproto_payload/decode | 0.158 | 112 | 5 |
| ascii32/protobuf_frame/encode | 0.186 | 176 | 3 |
| ascii32/protobuf_frame/decode | 0.249 | 224 | 4 |
| ascii32/sproto_frame/encode | 0.276 | 184 | 5 |
| ascii32/sproto_frame/decode | 0.358 | 256 | 11 |
| utf8_1k/protobuf_payload/encode | 1.312 | 1024 | 1 |
| utf8_1k/protobuf_payload/decode | 1.343 | 1088 | 2 |
| utf8_1k/sproto_payload/encode | 0.211 | 1024 | 1 |
| utf8_1k/sproto_payload/decode | 0.252 | 1104 | 5 |
| utf8_1k/protobuf_frame/encode | 1.536 | 2128 | 3 |
| utf8_1k/protobuf_frame/decode | 1.607 | 2192 | 4 |
| utf8_1k/sproto_frame/encode | 0.593 | 3096 | 5 |
| utf8_1k/sproto_frame/decode | 0.549 | 2224 | 11 |
| ascii4k/protobuf_payload/encode | 0.843 | 4864 | 1 |
| ascii4k/protobuf_payload/decode | 0.902 | 4160 | 2 |
| ascii4k/sproto_payload/encode | 0.543 | 4864 | 1 |
| ascii4k/sproto_payload/decode | 0.557 | 4176 | 5 |
| ascii4k/protobuf_frame/encode | 1.339 | 9808 | 3 |
| ascii4k/protobuf_frame/decode | 1.412 | 9104 | 4 |
| ascii4k/sproto_frame/encode | 1.562 | 14616 | 5 |
| ascii4k/sproto_frame/decode | 1.166 | 9136 | 11 |
| zeros4k/protobuf_payload/encode | 0.869 | 4864 | 1 |
| zeros4k/protobuf_payload/decode | 0.922 | 4160 | 2 |
| zeros4k/sproto_payload/encode | 0.558 | 4864 | 1 |
| zeros4k/sproto_payload/decode | 0.592 | 4176 | 5 |
| zeros4k/protobuf_frame/encode | 1.370 | 9808 | 3 |
| zeros4k/protobuf_frame/decode | 1.470 | 9104 | 4 |
| zeros4k/sproto_frame/encode | 2.837 | 14616 | 5 |
| zeros4k/sproto_frame/decode | 1.253 | 9136 | 11 |
