# AddressBook 与游戏消息编解码基准

时间：2026-10-15T05:29:42.097Z；CPU：Apple M2；darwin 23.4.0 arm64。

go version go1.24.13 darwin/arm64；Node v23.11.0；V8 12.9.202.28-node.14；基础提交 77bbbaa1f6105ee38a94f4efbdc5e8291f7de724，包含随附工作树修改。

Go：google.golang.org/protobuf v1.36.1、cnooljaw/gosproto v0.4.0；TS：@bufbuild/protobuf 2.15.0、示例内修正的 sealindx/sproto-ts。实际源码哈希和依赖覆盖见 environment.json。

## 口径

共用 representative.json 数据；AddressBook 对应官方 Alice/Bob/三条电话，Protobuf 使用官方 proto2 required/optional 定义。Sproto PhoneNumber 提至顶层以适应 TS parser，字段布局不变。游戏消息用整数状态位、正负坐标、数值数组、物品结构体数组和字符串数组；int32_edges 为 896 个正负 int32 边界值。两侧 Protobuf 数值数组显式 packed，负值使用 sint32。

纯 payload 编解码与 Pack/Unpack 分开测；PEncode/PDecode 是组合路径。不含 Envelope/package、网络、MAC、Dispatcher、RPC、schema 构造和 fixture 校验。输入对象复用，编码输出分配计时；每次解码创建新对象。Go 的 Sproto 资源预算与失败原子性保留。Pack 只处理预先编码的 Sproto body，Unpack 只处理预先 packed 的 body，不与 Protobuf 强造对应操作。

两端在计时前检查 raw/packed 字节数、SHA-256、完整语义往返和 padding。Sproto 固定签名已与官方 C/Lua 编码器对照；AddressBook 复现 130/83 字节，Protobuf 为 69 字节。

Go 固定 GOMAXPROCS=1、200ms/样本、5 轮；TS 每操作至少约 50ms 热身、校准约 100ms/样本、5 轮并反转顺序。以下均为中位数。CPU/分配 profile 另起进程采集，不用于表格计时。

采样前 load average：4.35 / 5.29 / 5.18；采样后：6.54 / 5.64 / 5.31；逻辑 CPU 8。这是开发机基线。

## 编码大小（字节）

| 消息 | PB | Sproto raw | Sproto packed |
| --- | ---: | ---: | ---: |
| addressbook | 69 | 130 | 83 |
| game_small | 191 | 328 | 216 |
| game_large | 2081 | 3336 | 2280 |
| int32_edges | 2435 | 3593 | 2374 |
| echo_ascii4k | 4099 | 4104 | 4103 |

## Go 耗时（µs/op）

| 消息 | PB Encode | PB Decode | SP Encode | SP Decode | Pack | Unpack | PEncode | PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 0.339 | 0.844 | 0.871 | 1.280 | 0.263 | 0.177 | 1.124 | 1.485 |
| game_small | 0.682 | 1.672 | 2.616 | 2.983 | 0.566 | 0.487 | 3.186 | 3.503 |
| game_large | 7.756 | 17.505 | 26.973 | 29.063 | 5.476 | 5.092 | 32.453 | 34.298 |
| int32_edges | 5.030 | 4.208 | 13.125 | 7.334 | 6.060 | 5.353 | 19.226 | 12.726 |
| echo_ascii4k | 0.617 | 0.666 | 0.647 | 0.761 | 2.118 | 0.535 | 2.815 | 1.252 |

## TypeScript 耗时（µs/op）

| 消息 | PB Encode | PB Decode | SP Encode | SP Decode | Pack | Unpack | PEncode | PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 1.176 | 1.407 | 0.754 | 1.026 | 0.423 | 0.262 | 1.176 | 1.285 |
| game_small | 2.534 | 3.014 | 1.698 | 1.815 | 0.742 | 0.559 | 2.426 | 2.388 |
| game_large | 22.781 | 31.551 | 17.853 | 16.644 | 6.601 | 5.320 | 24.619 | 21.948 |
| int32_edges | 12.514 | 11.207 | 9.786 | 4.617 | 6.983 | 5.687 | 16.605 | 10.227 |
| echo_ascii4k | 5.828 | 0.655 | 1.229 | 0.487 | 5.572 | 0.948 | 6.730 | 1.509 |

## Go 分配（B/op / allocs/op）

| 消息 | PB Encode | PB Decode | SP Encode | SP Decode | Pack | Unpack | PEncode | PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 80 / 1 | 648 / 26 | 144 / 1 | 528 / 29 | 144 / 1 | 144 / 1 | 288 / 2 | 672 / 30 |
| game_small | 192 / 1 | 1208 / 53 | 352 / 1 | 1248 / 54 | 352 / 1 | 352 / 1 | 704 / 2 | 1600 / 55 |
| game_large | 2304 / 1 | 13560 / 537 | 3456 / 1 | 10464 / 534 | 3456 / 1 | 3456 / 1 | 6912 / 2 | 13920 / 535 |
| int32_edges | 2688 / 1 | 4160 / 2 | 4096 / 1 | 8312 / 5 | 4096 / 1 | 4096 / 1 | 8192 / 2 | 12408 / 6 |
| echo_ascii4k | 4864 / 1 | 4160 / 3 | 4864 / 1 | 4176 / 5 | 4864 / 1 | 4864 / 1 | 9728 / 2 | 9040 / 6 |

## 波动提示

下列项目五轮最大/最小超过 2；阈值不是统计显著性检验。

| 语言 | 项目 | min µs | median µs | max µs |
| --- | --- | ---: | ---: | ---: |
| Go | game_large/protobuf_decode | 17.385 | 17.505 | 44.390 |
| TypeScript | echo_ascii4k/sproto_decode | 0.485 | 0.487 | 1.307 |

## 单独能力探针

以下是额外正确性探针，不进入性能排名；失败不被解释成更快或更小。

- boolean：通过
- mixed_signed_int64_array：通过

## 复现

```sh
npm --prefix examples/websocket-gate run bench:representative
npm --prefix examples/websocket-gate run profile:protocol
```

原始数据：[Go](go.txt)、[TS](typescript.json)、[环境](environment.json)。默认新结果位于 tmp/protocol-representative/；完整 profile 位于 tmp/protocol-profile/。本轮未重采 profile；此前分析见 [优化实施与验收](../../2026-10-15-optimized/RESULTS.md)。

本轮没有重跑官方历史 C/Lua 对 pbc-lua 的速度排名，不宣称复现它的性能倍数，也没有测 BigInt、完整 int64、浮点和浏览器。
