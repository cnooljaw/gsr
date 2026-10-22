# 最终Go Protobuf / Sproto正常入口对照

时间为三轮中位µs/op，完整三轮均保留。PB固定v1.36.1；未测TS，不含网络和真实RPC。

| 消息 | PB Encode | SP Encode | SP PEncode | PB Decode | SP Decode | SP PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 0.321 | 0.658 | 0.600 | 0.829 | 1.025 | 1.202 |
| game_small | 0.658 | 0.675 | 0.788 | 1.663 | 1.502 | 1.878 |
| game_large | 7.404 | 4.016 | 5.545 | 17.850 | 12.909 | 16.430 |
| int32_edges | 4.822 | 1.365 | 2.836 | 4.104 | 1.404 | 5.362 |
| echo_ascii4k | 0.636 | 0.658 | 0.837 | 0.785 | 0.796 | 1.433 |

正数为SP耗时更高；正文和packed分列，不合并宣布胜负。

| 消息 | Encode vs PB | PEncode vs PB | Decode vs PB | PDecode vs PB |
| --- | ---: | ---: | ---: | ---: |
| addressbook | +105.0% | +87.1% | +23.7% | +45.0% |
| game_small | +2.6% | +19.7% | -9.7% | +12.9% |
| game_large | -45.8% | -25.1% | -27.7% | -8.0% |
| int32_edges | -71.7% | -41.2% | -65.8% | +30.7% |
| echo_ascii4k | +3.4% | +31.6% | +1.4% | +82.4% |

## Echo payload/frame 全部32项

frame为现有benchmark的手工封装，不代表库实际Rpc.Request/Dispatch；帧格式不同，分别保留。

| 用例 | µs/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| ascii32/protobuf_payload/encode | 0.069 | 48 | 1 |
| ascii32/protobuf_payload/decode | 0.117 | 96 | 2 |
| ascii32/sproto_payload/encode | 0.087 | 48 | 1 |
| ascii32/sproto_payload/decode | 0.158 | 112 | 5 |
| ascii32/protobuf_frame/encode | 0.211 | 176 | 3 |
| ascii32/protobuf_frame/decode | 0.247 | 224 | 4 |
| ascii32/sproto_frame/encode | 0.271 | 184 | 5 |
| ascii32/sproto_frame/decode | 0.358 | 256 | 11 |
| utf8_1k/protobuf_payload/encode | 1.247 | 1024 | 1 |
| utf8_1k/protobuf_payload/decode | 1.365 | 1088 | 2 |
| utf8_1k/sproto_payload/encode | 0.227 | 1024 | 1 |
| utf8_1k/sproto_payload/decode | 0.272 | 1104 | 5 |
| utf8_1k/protobuf_frame/encode | 1.523 | 2128 | 3 |
| utf8_1k/protobuf_frame/decode | 1.625 | 2192 | 4 |
| utf8_1k/sproto_frame/encode | 0.645 | 3096 | 5 |
| utf8_1k/sproto_frame/decode | 0.596 | 2224 | 11 |
| ascii4k/protobuf_payload/encode | 0.908 | 4864 | 1 |
| ascii4k/protobuf_payload/decode | 1.060 | 4160 | 2 |
| ascii4k/sproto_payload/encode | 0.647 | 4864 | 1 |
| ascii4k/sproto_payload/decode | 0.720 | 4176 | 5 |
| ascii4k/protobuf_frame/encode | 1.513 | 9808 | 3 |
| ascii4k/protobuf_frame/decode | 1.743 | 9104 | 4 |
| ascii4k/sproto_frame/encode | 2.077 | 14616 | 5 |
| ascii4k/sproto_frame/decode | 1.419 | 9136 | 11 |
| zeros4k/protobuf_payload/encode | 0.891 | 4864 | 1 |
| zeros4k/protobuf_payload/decode | 1.047 | 4160 | 2 |
| zeros4k/sproto_payload/encode | 0.607 | 4864 | 1 |
| zeros4k/sproto_payload/decode | 0.857 | 4176 | 5 |
| zeros4k/protobuf_frame/encode | 1.574 | 9808 | 3 |
| zeros4k/protobuf_frame/decode | 1.642 | 9104 | 4 |
| zeros4k/sproto_frame/encode | 2.953 | 14616 | 5 |
| zeros4k/sproto_frame/decode | 1.319 | 9136 | 11 |
