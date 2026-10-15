# Protobuf / Sproto Go 与 TypeScript 编解码基准

采样时间：2026-10-15T05:31:49.833Z。CPU：Apple M2；系统：darwin 23.4.0 arm64。

go version go1.24.13 darwin/arm64；Node v23.11.0；V8 12.9.202.28-node.14。

基于提交 f82551ccc8a8c8615d945642e27aefd2c9dae142。

Go：google.golang.org/protobuf v1.36.1、cnooljaw/gosproto v0.4.0。TS：@bufbuild/protobuf 2.15.0、示例内修正的 sealindx/sproto-ts；源码哈希见 environment.json。

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
| payload | ascii32 | 0.067 | 0.102 | 1.52 | 0.109 | 0.177 | 1.63 |
| payload | utf8_1k | 1.286 | 0.235 | 0.18 | 1.355 | 0.284 | 0.21 |
| payload | ascii4k | 0.879 | 0.590 | 0.67 | 0.988 | 0.665 | 0.67 |
| payload | zeros4k | 0.872 | 0.582 | 0.67 | 0.998 | 0.641 | 0.64 |
| frame | ascii32 | 0.188 | 0.368 | 1.96 | 0.250 | 0.430 | 1.72 |
| frame | utf8_1k | 1.549 | 1.073 | 0.69 | 1.608 | 0.655 | 0.41 |
| frame | ascii4k | 1.474 | 3.296 | 2.24 | 1.522 | 1.479 | 0.97 |
| frame | zeros4k | 1.770 | 7.333 | 4.14 | 1.534 | 2.849 | 1.86 |

## TypeScript 耗时

单位 µs/op。SP/PB 大于 1 表示本轮 Sproto 更慢；小于 1 表示更快。

| 层级 | 内容 | PB 编码 | SP 编码 | SP/PB | PB 解码 | SP 解码 | SP/PB |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| payload | ascii32 | 0.393 | 0.128 | 0.33 | 0.317 | 0.108 | 0.34 |
| payload | utf8_1k | 1.869 | 1.549 | 0.83 | 4.387 | 4.091 | 0.93 |
| payload | ascii4k | 5.863 | 1.255 | 0.21 | 0.740 | 0.505 | 0.68 |
| payload | zeros4k | 5.617 | 1.250 | 0.22 | 0.745 | 0.511 | 0.69 |
| frame | ascii32 | 0.890 | 0.595 | 0.67 | 0.653 | 0.329 | 0.50 |
| frame | utf8_1k | 3.003 | 3.410 | 1.14 | 4.747 | 4.483 | 0.94 |
| frame | ascii4k | 7.823 | 7.660 | 0.98 | 1.073 | 1.507 | 1.40 |
| frame | zeros4k | 7.879 | 9.306 | 1.18 | 1.074 | 11.854 | 11.04 |

## Go 分配

每格为 B/op / allocs/op；TS 未测堆分配。

| 层级 | 内容 | PB 编码 | SP 编码 | PB 解码 | SP 解码 |
| --- | --- | ---: | ---: | ---: | ---: |
| payload | ascii32 | 48 / 1 | 48 / 1 | 96 / 2 | 112 / 5 |
| payload | utf8_1k | 1024 / 1 | 1024 / 1 | 1088 / 2 | 1104 / 5 |
| payload | ascii4k | 4864 / 1 | 4864 / 1 | 4160 / 2 | 4176 / 5 |
| payload | zeros4k | 4864 / 1 | 4864 / 1 | 4160 / 2 | 4176 / 5 |
| frame | ascii32 | 176 / 3 | 168 / 4 | 224 / 4 | 256 / 11 |
| frame | utf8_1k | 2128 / 3 | 3080 / 4 | 2192 / 4 | 2224 / 11 |
| frame | ascii4k | 9808 / 3 | 14600 / 4 | 9104 / 4 | 9136 / 11 |
| frame | zeros4k | 9808 / 3 | 14600 / 4 | 9104 / 4 | 9136 / 11 |

## 采样波动

以下列出五轮最大/最小耗时超过 2 的项目。原始文件保留全部样本；这个阈值只用于提示波动，不是显著性检验。

| 语言 | 项目 | 最小 µs | 中位 µs | 最大 µs | 最大/最小 |
| --- | --- | ---: | ---: | ---: | ---: |

## 复现与解释

从仓库根目录运行：

```sh
npm --prefix examples/websocket-gate ci
npm --prefix examples/websocket-gate run bench:protocol
```

默认产物位于 tmp/protocol-benchmark/：report.md、go.txt、typescript.json、environment.json。也可在 npm 命令末尾加 -- /绝对输出目录。原始结果保留每轮耗时；Go 原始结果另含吞吐和分配。

这是当前实现对 Echo 字符串的本机基线。zeros4k 是刻意构造的高压缩率输入，不能代表普通游戏消息；未覆盖整数数组、嵌套结构、浏览器、Lua/C 实现或端到端吞吐。Go Sproto 含反射、资源预算和失败原子性成本，TS 为示例参考实现，结果不能归因为 wire 格式的理论性能，也不能用于承诺所有消息的胜负。
