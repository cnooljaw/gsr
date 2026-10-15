# AddressBook 与游戏消息编解码基准

本表为优化后结果，前后对比、验证和 Go 本地优化版复现方法见 [优化实施与验收](RESULTS.md)。Go 最终重测与 TS 采样分时执行，分别记录于 environment.json 与 environment-ts-capture.json；源文件哈希一致性已核对。

时间：2026-10-15T04:29:10.439Z；CPU：Apple M2；darwin 23.4.0 arm64。

go version go1.24.13 darwin/arm64；Node v23.11.0；V8 12.9.202.28-node.14；基础提交 d942321d47741b578aa128e103f21caceebe91cf，包含随附工作树修改。

Go：google.golang.org/protobuf v1.36.1、cnooljaw/gosproto v0.3.0 基础的本地工作树（未发布优化版）；TS：@bufbuild/protobuf 2.15.0、示例内修正的 sealindx/sproto-ts。实际源码哈希和依赖覆盖见 environment.json。

## 口径

共用 representative.json 数据；AddressBook 对应官方 Alice/Bob/三条电话，Protobuf 使用官方 proto2 required/optional 定义。Sproto PhoneNumber 提至顶层以适应 TS parser，字段布局不变。游戏消息用整数状态位、正负坐标、数值数组、物品结构体数组和字符串数组；int32_edges 为 896 个正负 int32 边界值。两侧 Protobuf 数值数组显式 packed，负值使用 sint32。

纯 payload 编解码与 Pack/Unpack 分开测；PEncode/PDecode 是组合路径。不含 Envelope/package、网络、MAC、Dispatcher、RPC、schema 构造和 fixture 校验。输入对象复用，编码输出分配计时；每次解码创建新对象。Go 的 Sproto 资源预算与失败原子性保留。Pack 只处理预先编码的 Sproto body，Unpack 只处理预先 packed 的 body，不与 Protobuf 强造对应操作。

两端在计时前检查 raw/packed 字节数、SHA-256、完整语义往返和 padding。Sproto 固定签名已与官方 C/Lua 编码器对照；AddressBook 复现 130/83 字节，Protobuf 为 69 字节。

Go 固定 GOMAXPROCS=1、200ms/样本、5 轮；TS 每操作至少约 50ms 热身、校准约 100ms/样本、5 轮并反转顺序。以下均为中位数。CPU/分配 profile 另起进程采集，不用于表格计时。

TS 采样前 load average：6.06 / 5.86 / 5.63；采样后：11.20 / 7.30 / 6.19；逻辑 CPU 8。这是开发机样本，Go 最终重测时间另记于 environment.json。

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
| addressbook | 0.340 | 0.842 | 0.868 | 1.279 | 0.263 | 0.178 | 1.121 | 1.479 |
| game_small | 0.687 | 1.668 | 2.597 | 2.976 | 0.572 | 0.484 | 3.183 | 3.471 |
| game_large | 7.740 | 17.432 | 27.045 | 29.261 | 5.486 | 5.080 | 32.499 | 34.238 |
| int32_edges | 4.940 | 4.182 | 13.102 | 7.339 | 6.052 | 5.362 | 19.172 | 12.737 |
| echo_ascii4k | 0.607 | 0.637 | 0.657 | 0.729 | 2.099 | 0.545 | 2.776 | 1.239 |

## TypeScript 耗时（µs/op）

| 消息 | PB Encode | PB Decode | SP Encode | SP Decode | Pack | Unpack | PEncode | PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 1.153 | 1.401 | 0.757 | 1.025 | 0.422 | 0.261 | 1.175 | 1.303 |
| game_small | 2.439 | 2.971 | 1.710 | 1.811 | 0.742 | 0.571 | 2.467 | 2.353 |
| game_large | 22.527 | 31.631 | 17.998 | 16.576 | 6.693 | 5.333 | 24.582 | 22.105 |
| int32_edges | 12.633 | 11.071 | 9.748 | 4.606 | 7.009 | 5.703 | 16.724 | 10.116 |
| echo_ascii4k | 5.596 | 0.658 | 1.249 | 0.492 | 5.578 | 0.988 | 6.758 | 1.477 |

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

## 单独能力探针

以下是额外正确性探针，不进入性能排名；失败不被解释成更快或更小。

- boolean：通过
- mixed_signed_int64_array：通过

## 复现

本表保留发布前同一源码的原始采样；当前 GSR 已依赖正式 v0.4.0，可按 [正式依赖说明](RESULTS.md#使用正式依赖复现) 直接执行：

```sh
npm --prefix examples/websocket-gate run bench:representative
GSR_PROFILE_SCENARIO=game_large npm --prefix examples/websocket-gate run profile:protocol
```

原始数据：[Go](go.txt)、[TS](typescript.json)、[环境](environment.json)。默认新结果位于 tmp/protocol-representative/；默认 profile 位于 tmp/protocol-profile/，本轮原始 profile 保存在 tmp/protocol-profile-optimized/。方法、汇总及边界见 [Profile 验收](RESULTS.md#profile-验收)。发布版与采样时的五份 Go 源码哈希一致，原始环境仍记录当时的 v0.3.0 + 本地覆盖，不改写历史数据。

本轮没有重跑官方历史 C/Lua 对 pbc-lua 的速度排名，不宣称复现它的性能倍数，也没有测 BigInt、完整 int64、浮点和浏览器。
