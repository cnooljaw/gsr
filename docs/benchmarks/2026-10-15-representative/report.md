# AddressBook 与游戏消息编解码基准

时间：2026-10-15T03:41:21.780Z；CPU：Apple M2；darwin 23.4.0 arm64。

go version go1.24.13 darwin/arm64；Node v23.11.0；V8 12.9.202.28-node.14；基础提交 06d5941f41570cb0f042ead6b11cd5432e39d08c，包含随附工作树修改。

Go：google.golang.org/protobuf v1.36.1、cnooljaw/gosproto v0.3.0；TS：@bufbuild/protobuf 2.15.0、示例内固定并修正的 sealindx/sproto-ts。未修改编解码库实现。

## 口径

共用 representative.json 数据；AddressBook 对应官方 Alice/Bob/三条电话，Protobuf 使用官方 proto2 required/optional 定义。Sproto PhoneNumber 提至顶层以适应 TS parser，字段布局不变。游戏消息用整数状态位、正负坐标、数值数组、物品结构体数组和字符串数组；int32_edges 为 896 个正负 int32 边界值。两侧 Protobuf 数值数组显式 packed，负值使用 sint32。

纯 payload 编解码与 Pack/Unpack 分开测；PEncode/PDecode 是组合路径。不含 Envelope/package、网络、MAC、Dispatcher、RPC、schema 构造和 fixture 校验。输入对象复用，编码输出分配计时；每次解码创建新对象。Go 的 Sproto 资源预算与失败原子性保留。Pack 只处理预先编码的 Sproto body，Unpack 只处理预先 packed 的 body，不与 Protobuf 强造对应操作。

两端在计时前检查 raw/packed 字节数、SHA-256、完整语义往返和 padding。Sproto 固定签名已与官方 C/Lua 编码器对照；AddressBook 复现 130/83 字节，Protobuf 为 69 字节。

Go 固定 GOMAXPROCS=1、200ms/样本、5 轮；TS 每操作至少约 50ms 热身、校准约 100ms/样本、5 轮并反转顺序。以下均为中位数。CPU/分配 profile 另起进程采集，不用于表格计时。

采样前 load average：4.91 / 5.40 / 5.64；采样后：5.46 / 5.53 / 5.66；逻辑 CPU 8。这是开发机基线。

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
| addressbook | 0.331 | 0.852 | 1.887 | 1.724 | 0.270 | 0.241 | 2.158 | 1.976 |
| game_small | 0.674 | 1.683 | 3.830 | 4.411 | 0.583 | 0.548 | 4.453 | 5.011 |
| game_large | 7.567 | 17.475 | 43.212 | 41.346 | 5.683 | 5.746 | 48.919 | 47.713 |
| int32_edges | 4.908 | 4.190 | 15.259 | 7.587 | 5.975 | 6.239 | 21.207 | 13.741 |
| echo_ascii4k | 0.599 | 0.725 | 2.133 | 0.743 | 6.217 | 1.086 | 8.598 | 2.193 |

## TypeScript 耗时（µs/op）

| 消息 | PB Encode | PB Decode | SP Encode | SP Decode | Pack | Unpack | PEncode | PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 1.169 | 1.400 | 1.617 | 1.024 | 0.423 | 0.277 | 2.046 | 1.297 |
| game_small | 2.513 | 2.989 | 3.458 | 1.809 | 0.760 | 0.595 | 4.221 | 2.434 |
| game_large | 22.575 | 31.095 | 34.600 | 16.466 | 6.524 | 5.746 | 41.286 | 22.346 |
| int32_edges | 12.410 | 11.044 | 30.521 | 4.600 | 6.917 | 6.045 | 37.445 | 10.708 |
| echo_ascii4k | 5.656 | 0.668 | 1.745 | 0.497 | 14.448 | 0.996 | 16.201 | 1.428 |

## Go 分配（B/op / allocs/op）

| 消息 | PB Encode | PB Decode | SP Encode | SP Decode | Pack | Unpack | PEncode | PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 80 / 1 | 648 / 26 | 784 / 23 | 920 / 47 | 240 / 1 | 272 / 2 | 1024 / 24 | 1192 / 49 |
| game_small | 192 / 1 | 1208 / 53 | 1328 / 32 | 2336 / 107 | 448 / 1 | 672 / 2 | 1776 / 33 | 3008 / 109 |
| game_large | 2304 / 1 | 13560 / 537 | 16496 / 320 | 22112 / 1143 | 4096 / 1 | 9472 / 3 | 20592 / 321 | 31584 / 1146 |
| int32_edges | 2688 / 1 | 4160 / 2 | 16388 / 4 | 8328 / 6 | 4096 / 1 | 10752 / 3 | 20484 / 5 | 19080 / 9 |
| echo_ascii4k | 4864 / 1 | 4160 / 3 | 18436 / 4 | 4192 / 6 | 4864 / 1 | 10240 / 2 | 23300 / 5 | 14432 / 8 |

## 波动提示

下列项目五轮最大/最小超过 2；阈值不是统计显著性检验。

| 语言 | 项目 | min µs | median µs | max µs |
| --- | --- | ---: | ---: | ---: |
| Go | int32_edges/protobuf_decode | 4.135 | 4.190 | 24.173 |
| TypeScript | game_large/protobuf_encode | 22.356 | 22.575 | 50.061 |

## 单独能力探针

以下是额外正确性探针，不进入性能排名；失败不被解释成更快或更小。

- boolean：失败，Error: Sproto integer requires a safe number
- mixed_signed_int64_array：失败，AssertionError [ERR_ASSERTION]: Expected values to be strictly deep-equal:

## 复现

```sh
npm --prefix examples/websocket-gate run bench:representative
npm --prefix examples/websocket-gate run profile:protocol
```

原始数据：[Go](go.txt)、[TS](typescript.json)、[环境](environment.json)。默认新结果位于 tmp/protocol-representative/；完整 profile 位于 tmp/protocol-profile/。证据归因、profile 限制和优化顺序见 [分析与方案](ANALYSIS-AND-PLAN.md)，采集 top 汇总为 [Go](profile-go-summary.json)、[TS](profile-typescript-summary.json)。

本轮没有重跑官方历史 C/Lua 对 pbc-lua 的速度排名，不宣称复现它的性能倍数，也没有测 BigInt、完整 int64、浮点和浏览器。
