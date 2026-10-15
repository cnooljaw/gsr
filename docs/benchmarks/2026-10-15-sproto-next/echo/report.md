# Protobuf / Sproto Go 与 TypeScript 编解码基准

采样时间：2026-10-15T06:30:49.078Z。CPU：Apple M2；系统：darwin 23.4.0 arm64。

go version go1.24.13 darwin/arm64；Node v23.11.0；V8 12.9.202.28-node.14。

基于提交 f82551ccc8a8c8615d945642e27aefd2c9dae142，包含本报告随附的工作树修改。

Go：google.golang.org/protobuf v1.36.1、cnooljaw/gosproto v0.4.0 基础的本地工作树（未发布优化版）。TS：@bufbuild/protobuf 2.15.0、示例内修正的 sealindx/sproto-ts；源码哈希见 environment.json。

## 测量范围

共用 cases.json 的 EchoRequest{text} 内容和 UTF-8 字节；两端在计时前验证固定 SHA-256、字节数和解码内容。TS Sproto 应用帧还与真实 SprotoEchoClient 的输出逐字节核对。

payload 仅测消息体，不做 packing；frame 测实际应用层格式：Protobuf Envelope(msg_id=1001, request_id=1, payload)，Sproto package(type=1, session=1)+payload 后 pack，解码包含 unpack。两种协议编号沿用现有 Echo 定义。

不含 schema 构建、网络、WebSocket、MAC、Dispatcher、安全预检、业务 Handler 或 RPC 会话表。payload 输入对象复用，frame 的外层头对象每次构造；输出分配计时，每次解码生成新对象。TS Sproto 的固定内联请求头长 6 字节，解码头后据此切分 payload，不代表通用路由成本。

Go 固定 GOMAXPROCS=1，200ms/样本、5 样本；TS 每操作至少约 50ms 热身并校准约 100ms/样本、5 样本，轮间反转顺序；均取中位数。TS 保留常规 JIT/GC，不手工 GC。两种语言顺序运行。Go 吞吐率分母是原始 text UTF-8 字节数。

## 包大小（字节）

| 内容 | text | PB payload | SP payload | PB frame | SP packed frame |
| --- | ---: | ---: | ---: | ---: | ---: |
| ascii32 | 32 | 34 | 40 | 41 | 43 |
| utf8_1k | 1000 | 1003 | 1008 | 1011 | 1012 |
| ascii4k | 4096 | 4099 | 4104 | 4107 | 4109 |
| zeros4k | 4096 | 4099 | 4104 | 4107 | 519 |

## Go 耗时

单位 µs/op。SP/PB 大于 1 表示本轮 Sproto 更慢；小于 1 表示更快。

| 层级 | 内容 | PB 编码 | SP 编码 | SP/PB | PB 解码 | SP 解码 | SP/PB |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| payload | ascii32 | 0.068 | 0.089 | 1.31 | 0.107 | 0.177 | 1.65 |
| payload | utf8_1k | 1.287 | 0.215 | 0.17 | 1.346 | 0.276 | 0.21 |
| payload | ascii4k | 0.866 | 0.561 | 0.65 | 0.967 | 0.646 | 0.67 |
| payload | zeros4k | 0.866 | 0.564 | 0.65 | 0.968 | 0.636 | 0.66 |
| frame | ascii32 | 0.187 | 0.330 | 1.76 | 0.249 | 0.409 | 1.64 |
| frame | utf8_1k | 1.545 | 1.031 | 0.67 | 1.594 | 0.626 | 0.39 |
| frame | ascii4k | 1.467 | 3.218 | 2.19 | 1.497 | 1.371 | 0.92 |
| frame | zeros4k | 1.598 | 10.121 | 6.33 | 1.884 | 3.026 | 1.61 |

## TypeScript 耗时

单位 µs/op。SP/PB 大于 1 表示本轮 Sproto 更慢；小于 1 表示更快。

| 层级 | 内容 | PB 编码 | SP 编码 | SP/PB | PB 解码 | SP 解码 | SP/PB |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| payload | ascii32 | 0.400 | 0.129 | 0.32 | 0.317 | 0.104 | 0.33 |
| payload | utf8_1k | 1.908 | 1.548 | 0.81 | 4.431 | 4.108 | 0.93 |
| payload | ascii4k | 5.919 | 1.322 | 0.22 | 0.790 | 0.509 | 0.64 |
| payload | zeros4k | 5.629 | 1.264 | 0.22 | 0.733 | 0.517 | 0.71 |
| frame | ascii32 | 0.875 | 0.600 | 0.69 | 0.646 | 0.332 | 0.51 |
| frame | utf8_1k | 3.092 | 3.562 | 1.15 | 4.870 | 4.617 | 0.95 |
| frame | ascii4k | 8.218 | 3.263 | 0.40 | 1.065 | 1.584 | 1.49 |
| frame | zeros4k | 8.254 | 3.393 | 0.41 | 1.089 | 2.640 | 2.43 |

## Go 分配

每格为 B/op / allocs/op；TS 未测堆分配。

| 层级 | 内容 | PB 编码 | SP 编码 | PB 解码 | SP 解码 |
| --- | --- | ---: | ---: | ---: | ---: |
| payload | ascii32 | 48 / 1 | 48 / 1 | 96 / 2 | 112 / 5 |
| payload | utf8_1k | 1024 / 1 | 1024 / 1 | 1088 / 2 | 1104 / 5 |
| payload | ascii4k | 4864 / 1 | 4864 / 1 | 4160 / 2 | 4176 / 5 |
| payload | zeros4k | 4864 / 1 | 4864 / 1 | 4160 / 2 | 4176 / 5 |
| frame | ascii32 | 176 / 3 | 184 / 5 | 224 / 4 | 256 / 11 |
| frame | utf8_1k | 2128 / 3 | 3096 / 5 | 2192 / 4 | 2224 / 11 |
| frame | ascii4k | 9808 / 3 | 14616 / 5 | 9104 / 4 | 9136 / 11 |
| frame | zeros4k | 9808 / 3 | 14616 / 5 | 9104 / 4 | 9136 / 11 |

## 采样波动

以下列出五轮最大/最小耗时超过 2 的项目。原始文件保留全部样本；这个阈值只用于提示波动，不是显著性检验。

| 语言 | 项目 | 最小 µs | 中位 µs | 最大 µs | 最大/最小 |
| --- | --- | ---: | ---: | ---: | ---: |
| Go | zeros4k/protobuf_frame/decode | 1.609 | 1.884 | 5.092 | 3.16 |
| TypeScript | ascii32/protobuf_payload/decode | 0.310 | 0.317 | 0.804 | 2.59 |
| TypeScript | ascii32/sproto_payload/decode | 0.103 | 0.104 | 0.410 | 4.00 |
| TypeScript | utf8_1k/protobuf_frame/encode | 2.942 | 3.092 | 7.315 | 2.49 |
| TypeScript | ascii4k/sproto_payload/encode | 1.235 | 1.322 | 2.727 | 2.21 |

## 复现与解释

从仓库根目录运行：

```sh
npm --prefix examples/websocket-gate ci
npm --prefix examples/websocket-gate run bench:protocol
```

默认产物位于 tmp/protocol-benchmark/：report.md、go.txt、typescript.json、environment.json。也可在 npm 命令末尾加 -- /绝对输出目录。原始结果保留每轮耗时；Go 原始结果另含吞吐和分配。

这是当前实现对 Echo 字符串的本机基线。zeros4k 是刻意构造的高压缩率输入，不能代表普通游戏消息；未覆盖整数数组、嵌套结构、浏览器、Lua/C 实现或端到端吞吐。Go Sproto 含反射、资源预算和失败原子性成本，TS 为示例参考实现，结果不能归因为 wire 格式的理论性能，也不能用于承诺所有消息的胜负。
