# AddressBook 与游戏消息编解码基准

时间：2026-10-15T06:29:05.080Z；CPU：Apple M2；darwin 23.4.0 arm64。

go version go1.24.13 darwin/arm64；Node v23.11.0；V8 12.9.202.28-node.14；基础提交 f82551ccc8a8c8615d945642e27aefd2c9dae142，包含随附工作树修改。

Go：google.golang.org/protobuf v1.36.1、cnooljaw/gosproto v0.4.0 基础的本地工作树（未发布优化版）；TS：@bufbuild/protobuf 2.15.0、示例内修正的 sealindx/sproto-ts。实际源码哈希和依赖覆盖见 environment.json。

## 口径

共用 representative.json 数据；AddressBook 对应官方 Alice/Bob/三条电话，Protobuf 使用官方 proto2 required/optional 定义。Sproto PhoneNumber 提至顶层以适应 TS parser，字段布局不变。游戏消息用整数状态位、正负坐标、数值数组、物品结构体数组和字符串数组；int32_edges 为 896 个正负 int32 边界值。两侧 Protobuf 数值数组显式 packed，负值使用 sint32。

纯 payload 编解码与 Pack/Unpack 分开测；PEncode/PDecode 是组合路径。不含 Envelope/package、网络、MAC、Dispatcher、RPC、schema 构造和 fixture 校验。输入对象复用，编码输出分配计时；每次解码创建新对象。Go 的 Sproto 资源预算与失败原子性保留。Pack 只处理预先编码的 Sproto body，Unpack 只处理预先 packed 的 body，不与 Protobuf 强造对应操作。

两端在计时前检查 raw/packed 字节数、SHA-256、完整语义往返和 padding。Sproto 固定签名已与官方 C/Lua 编码器对照；AddressBook 复现 130/83 字节，Protobuf 为 69 字节。

Go 固定 GOMAXPROCS=1、200ms/样本、5 轮；TS 每操作至少约 50ms 热身、校准约 100ms/样本、5 轮并反转顺序。以下均为中位数。CPU/分配 profile 另起进程采集，不用于表格计时。

采样前 load average：5.96 / 5.91 / 5.59；采样后：6.19 / 5.97 / 5.64；逻辑 CPU 8。这是开发机基线。

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
| addressbook | 0.343 | 0.848 | 0.569 | 1.175 | 0.265 | 0.178 | 0.829 | 1.392 |
| game_small | 0.709 | 1.672 | 1.436 | 2.272 | 0.577 | 0.487 | 1.995 | 2.820 |
| game_large | 7.896 | 17.464 | 14.781 | 23.310 | 5.500 | 5.105 | 20.249 | 28.525 |
| int32_edges | 5.219 | 4.204 | 2.224 | 2.276 | 6.063 | 5.359 | 8.257 | 7.711 |
| echo_ascii4k | 0.603 | 0.674 | 0.659 | 0.701 | 2.140 | 0.561 | 2.770 | 1.312 |

## TypeScript 耗时（µs/op）

| 消息 | PB Encode | PB Decode | SP Encode | SP Decode | Pack | Unpack | PEncode | PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 1.172 | 1.401 | 0.754 | 1.030 | 0.441 | 0.269 | 1.195 | 1.307 |
| game_small | 2.494 | 3.002 | 1.698 | 1.821 | 0.755 | 0.580 | 2.469 | 2.402 |
| game_large | 22.418 | 31.978 | 17.886 | 16.497 | 6.791 | 5.464 | 24.812 | 22.187 |
| int32_edges | 12.477 | 11.034 | 9.732 | 4.649 | 7.182 | 5.943 | 16.883 | 10.362 |
| echo_ascii4k | 5.611 | 0.712 | 1.235 | 0.487 | 0.901 | 0.979 | 2.107 | 1.452 |

## Go 分配（B/op / allocs/op）

| 消息 | PB Encode | PB Decode | SP Encode | SP Decode | Pack | Unpack | PEncode | PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 80 / 1 | 648 / 26 | 144 / 1 | 528 / 29 | 144 / 1 | 144 / 1 | 288 / 2 | 672 / 30 |
| game_small | 192 / 1 | 1208 / 53 | 352 / 1 | 1200 / 52 | 352 / 1 | 352 / 1 | 704 / 2 | 1552 / 53 |
| game_large | 2304 / 1 | 13560 / 537 | 4480 / 2 | 10416 / 532 | 3456 / 1 | 3456 / 1 | 7936 / 3 | 13872 / 533 |
| int32_edges | 2688 / 1 | 4160 / 2 | 4096 / 1 | 8288 / 4 | 4096 / 1 | 4096 / 1 | 8192 / 2 | 12384 / 5 |
| echo_ascii4k | 4864 / 1 | 4160 / 3 | 4864 / 1 | 4176 / 5 | 4864 / 1 | 4864 / 1 | 9728 / 2 | 9040 / 6 |

## 波动提示

下列项目五轮最大/最小超过 2；阈值不是统计显著性检验。

| 语言 | 项目 | min µs | median µs | max µs |
| --- | --- | ---: | ---: | ---: |
| Go | game_small/sproto_pdecode | 2.768 | 2.820 | 20.313 |

## 单独能力探针

以下是额外正确性探针，不进入性能排名；失败不被解释成更快或更小。

- boolean：通过
- mixed_signed_int64_array：通过

## 复现

```sh
npm --prefix examples/websocket-gate run bench:representative
npm --prefix examples/websocket-gate run profile:protocol
```

原始数据：[Go](go.txt)、[TS](typescript.json)、[环境](environment.json)。默认新结果位于 tmp/protocol-representative/；完整 profile 位于 tmp/protocol-profile/。profile 与前后对照见 [本轮验收](../RESULTS.md)。

本轮没有重跑官方历史 C/Lua 对 pbc-lua 的速度排名，不宣称复现它的性能倍数，也没有测 BigInt、完整 int64、浮点和浏览器。
