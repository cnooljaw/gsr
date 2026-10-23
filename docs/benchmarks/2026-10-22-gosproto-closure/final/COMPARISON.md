# 最终Go Protobuf / Sproto正常入口对照

时间为三轮中位µs/op，完整三轮均保留。PB固定v1.36.1；未测TS，不含网络和真实RPC。

| 消息 | PB Encode | SP Encode | SP PEncode | PB Decode | SP Decode | SP PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 0.326 | 0.540 | 0.598 | 0.749 | 0.945 | 1.149 |
| game_small | 0.667 | 0.619 | 0.699 | 1.636 | 1.473 | 1.824 |
| game_large | 7.595 | 4.038 | 4.565 | 17.098 | 12.299 | 16.025 |
| int32_edges | 4.894 | 1.345 | 1.832 | 4.165 | 1.300 | 5.269 |
| echo_ascii4k | 0.549 | 0.605 | 0.725 | 0.633 | 0.622 | 1.173 |

正数为SP耗时更高；正文和packed分列，不合并宣布胜负。

| 消息 | Encode vs PB | PEncode vs PB | Decode vs PB | PDecode vs PB |
| --- | ---: | ---: | ---: | ---: |
| addressbook | +65.8% | +83.6% | +26.1% | +53.3% |
| game_small | -7.2% | +4.8% | -10.0% | +11.5% |
| game_large | -46.8% | -39.9% | -28.1% | -6.3% |
| int32_edges | -72.5% | -62.6% | -68.8% | +26.5% |
| echo_ascii4k | +10.1% | +31.9% | -1.7% | +85.3% |

## Echo payload/frame 全部32项

frame为现有benchmark的手工封装，不代表库实际Rpc.Request/Dispatch；帧格式不同，分别保留。

| 用例 | µs/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| ascii32/protobuf_payload/encode | 0.067 | 48 | 1 |
| ascii32/protobuf_payload/decode | 0.101 | 96 | 2 |
| ascii32/sproto_payload/encode | 0.089 | 48 | 1 |
| ascii32/sproto_payload/decode | 0.156 | 112 | 5 |
| ascii32/protobuf_frame/encode | 0.183 | 176 | 3 |
| ascii32/protobuf_frame/decode | 0.246 | 224 | 4 |
| ascii32/sproto_frame/encode | 0.271 | 184 | 5 |
| ascii32/sproto_frame/decode | 0.356 | 256 | 11 |
| utf8_1k/protobuf_payload/encode | 1.294 | 1024 | 1 |
| utf8_1k/protobuf_payload/decode | 1.338 | 1088 | 2 |
| utf8_1k/sproto_payload/encode | 0.210 | 1024 | 1 |
| utf8_1k/sproto_payload/decode | 0.250 | 1104 | 5 |
| utf8_1k/protobuf_frame/encode | 1.533 | 2128 | 3 |
| utf8_1k/protobuf_frame/decode | 1.572 | 2192 | 4 |
| utf8_1k/sproto_frame/encode | 0.597 | 3096 | 5 |
| utf8_1k/sproto_frame/decode | 0.571 | 2224 | 11 |
| ascii4k/protobuf_payload/encode | 0.843 | 4864 | 1 |
| ascii4k/protobuf_payload/decode | 0.910 | 4160 | 2 |
| ascii4k/sproto_payload/encode | 0.538 | 4864 | 1 |
| ascii4k/sproto_payload/decode | 0.561 | 4176 | 5 |
| ascii4k/protobuf_frame/encode | 1.392 | 9808 | 3 |
| ascii4k/protobuf_frame/decode | 1.403 | 9104 | 4 |
| ascii4k/sproto_frame/encode | 1.487 | 14616 | 5 |
| ascii4k/sproto_frame/decode | 1.220 | 9136 | 11 |
| zeros4k/protobuf_payload/encode | 0.843 | 4864 | 1 |
| zeros4k/protobuf_payload/decode | 0.910 | 4160 | 2 |
| zeros4k/sproto_payload/encode | 0.552 | 4864 | 1 |
| zeros4k/sproto_payload/decode | 0.536 | 4176 | 5 |
| zeros4k/protobuf_frame/encode | 1.311 | 9808 | 3 |
| zeros4k/protobuf_frame/decode | 1.439 | 9104 | 4 |
| zeros4k/sproto_frame/encode | 1.917 | 14616 | 5 |
| zeros4k/sproto_frame/decode | 1.224 | 9136 | 11 |
