# 最终Go Protobuf / Sproto正常入口对照

时间为三轮中位µs/op，完整三轮均保留。PB固定v1.36.1；未测TS，不含网络和真实RPC。

| 消息 | PB Encode | SP Encode | SP PEncode | PB Decode | SP Decode | SP PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 0.318 | 0.529 | 0.603 | 0.815 | 0.939 | 1.148 |
| game_small | 0.676 | 0.631 | 0.790 | 1.661 | 1.466 | 1.886 |
| game_large | 7.662 | 4.004 | 5.492 | 17.240 | 12.242 | 16.716 |
| int32_edges | 4.882 | 1.359 | 2.857 | 4.256 | 1.305 | 5.888 |
| echo_ascii4k | 0.571 | 0.623 | 0.733 | 0.619 | 0.625 | 1.177 |

正数为SP耗时更高；正文和packed分列，不合并宣布胜负。

| 消息 | Encode vs PB | PEncode vs PB | Decode vs PB | PDecode vs PB |
| --- | ---: | ---: | ---: | ---: |
| addressbook | +66.5% | +89.9% | +15.2% | +40.9% |
| game_small | -6.6% | +16.8% | -11.7% | +13.5% |
| game_large | -47.7% | -28.3% | -29.0% | -3.0% |
| int32_edges | -72.2% | -41.5% | -69.3% | +38.3% |
| echo_ascii4k | +9.0% | +28.2% | +0.9% | +90.1% |

## Echo payload/frame 全部32项

frame为现有benchmark的手工封装，不代表库实际Rpc.Request/Dispatch；帧格式不同，分别保留。

| 用例 | µs/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| ascii32/protobuf_payload/encode | 0.067 | 48 | 1 |
| ascii32/protobuf_payload/decode | 0.096 | 96 | 2 |
| ascii32/sproto_payload/encode | 0.082 | 48 | 1 |
| ascii32/sproto_payload/decode | 0.154 | 112 | 5 |
| ascii32/protobuf_frame/encode | 0.187 | 176 | 3 |
| ascii32/protobuf_frame/decode | 0.247 | 224 | 4 |
| ascii32/sproto_frame/encode | 0.270 | 184 | 5 |
| ascii32/sproto_frame/decode | 0.349 | 256 | 11 |
| utf8_1k/protobuf_payload/encode | 1.273 | 1024 | 1 |
| utf8_1k/protobuf_payload/decode | 1.331 | 1088 | 2 |
| utf8_1k/sproto_payload/encode | 0.207 | 1024 | 1 |
| utf8_1k/sproto_payload/decode | 0.250 | 1104 | 5 |
| utf8_1k/protobuf_frame/encode | 1.521 | 2128 | 3 |
| utf8_1k/protobuf_frame/decode | 1.565 | 2192 | 4 |
| utf8_1k/sproto_frame/encode | 0.591 | 3096 | 5 |
| utf8_1k/sproto_frame/decode | 0.565 | 2224 | 11 |
| ascii4k/protobuf_payload/encode | 0.844 | 4864 | 1 |
| ascii4k/protobuf_payload/decode | 0.909 | 4160 | 2 |
| ascii4k/sproto_payload/encode | 0.554 | 4864 | 1 |
| ascii4k/sproto_payload/decode | 0.606 | 4176 | 5 |
| ascii4k/protobuf_frame/encode | 1.588 | 9808 | 3 |
| ascii4k/protobuf_frame/decode | 1.423 | 9104 | 4 |
| ascii4k/sproto_frame/encode | 1.669 | 14616 | 5 |
| ascii4k/sproto_frame/decode | 1.207 | 9136 | 11 |
| zeros4k/protobuf_payload/encode | 0.866 | 4864 | 1 |
| zeros4k/protobuf_payload/decode | 0.966 | 4160 | 2 |
| zeros4k/sproto_payload/encode | 0.551 | 4864 | 1 |
| zeros4k/sproto_payload/decode | 0.560 | 4176 | 5 |
| zeros4k/protobuf_frame/encode | 1.437 | 9808 | 3 |
| zeros4k/protobuf_frame/decode | 1.474 | 9104 | 4 |
| zeros4k/sproto_frame/encode | 2.529 | 14616 | 5 |
| zeros4k/sproto_frame/decode | 1.222 | 9136 | 11 |
