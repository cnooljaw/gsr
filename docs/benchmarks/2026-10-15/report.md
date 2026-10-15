# Protobuf / Sproto Go 与 TypeScript 编解码基准

采样时间：2026-10-15T03:21:46.503Z。CPU：Apple M2；系统：darwin 23.4.0 arm64。

go version go1.24.13 darwin/arm64；Node v23.11.0；V8 12.9.202.28-node.14。

基于提交 06d5941f41570cb0f042ead6b11cd5432e39d08c，包含本报告随附的工作树修改。

Go：google.golang.org/protobuf v1.36.1、cnooljaw/gosproto v0.3.0。TS：@bufbuild/protobuf 2.15.0、示例内固定并修复的 sealindx/sproto-ts 参考实现。

## 本轮观察

- 4 KiB 普通文本应用帧：Go Protobuf 编码/解码约 1.664/1.566 µs，Sproto 约 8.974/2.129 µs；TS Protobuf 约 7.877/1.067 µs，Sproto 约 19.292/1.614 µs。此用例里 Protobuf 更快。
- 单独测 payload 时结果不同：TS Sproto 的四组编码中位数均更低；Go Sproto 的 UTF-8 payload 编解码也更低。库的反射、内存分配和字符串校验语义不同，不能把所有差异都归因于格式。
- 普通文本的帧大小接近；4096 个零字节的 Sproto packed 帧为 519 字节，Protobuf 为 4107 字节，但解包耗时更高。packing 的主要收益和成本取决于内容。
- 五轮中部分项目波动明显，详见“采样波动”。采样后本机 1 分钟 load average 为 7.17，有明显后台 CPU 活动；这些数值是开发机基线，接近的结果不代表稳定优势。

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
| payload | ascii32 | 0.102 | 0.231 | 2.26 | 0.111 | 0.199 | 1.79 |
| payload | utf8_1k | 1.388 | 0.489 | 0.35 | 1.364 | 0.322 | 0.24 |
| payload | ascii4k | 0.875 | 2.268 | 2.59 | 1.041 | 0.819 | 0.79 |
| payload | zeros4k | 1.178 | 2.965 | 2.52 | 1.091 | 0.670 | 0.61 |
| frame | ascii32 | 0.184 | 1.007 | 5.46 | 0.262 | 0.562 | 2.14 |
| frame | utf8_1k | 1.549 | 2.523 | 1.63 | 1.600 | 1.033 | 0.65 |
| frame | ascii4k | 1.664 | 8.974 | 5.39 | 1.566 | 2.129 | 1.36 |
| frame | zeros4k | 1.643 | 8.915 | 5.43 | 3.618 | 7.905 | 2.18 |

## TypeScript 耗时

单位 µs/op。SP/PB 大于 1 表示本轮 Sproto 更慢；小于 1 表示更快。

| 层级 | 内容 | PB 编码 | SP 编码 | SP/PB | PB 解码 | SP 解码 | SP/PB |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| payload | ascii32 | 0.422 | 0.220 | 0.52 | 0.336 | 0.114 | 0.34 |
| payload | utf8_1k | 2.099 | 1.302 | 0.62 | 4.363 | 4.107 | 0.94 |
| payload | ascii4k | 6.042 | 1.795 | 0.30 | 0.740 | 0.499 | 0.67 |
| payload | zeros4k | 5.577 | 1.805 | 0.32 | 0.737 | 0.525 | 0.71 |
| frame | ascii32 | 1.000 | 0.952 | 0.95 | 0.650 | 0.327 | 0.50 |
| frame | utf8_1k | 2.970 | 6.102 | 2.05 | 4.862 | 4.561 | 0.94 |
| frame | ascii4k | 7.877 | 19.292 | 2.45 | 1.067 | 1.614 | 1.51 |
| frame | zeros4k | 8.682 | 9.922 | 1.14 | 1.087 | 12.118 | 11.14 |

## Go 分配

每格为 B/op / allocs/op；TS 未测堆分配。

| 层级 | 内容 | PB 编码 | SP 编码 | PB 解码 | SP 解码 |
| --- | --- | ---: | ---: | ---: | ---: |
| payload | ascii32 | 48 / 1 | 84 / 3 | 96 / 2 | 128 / 6 |
| payload | utf8_1k | 1024 / 1 | 2052 / 3 | 1088 / 2 | 1120 / 6 |
| payload | ascii4k | 4864 / 1 | 18436 / 4 | 4160 / 2 | 4192 / 6 |
| payload | zeros4k | 4864 / 1 | 18436 / 4 | 4160 / 2 | 4192 / 6 |
| frame | ascii32 | 176 / 3 | 328 / 8 | 224 / 4 | 408 / 16 |
| frame | utf8_1k | 2128 / 3 | 4264 / 8 | 2192 / 4 | 3816 / 16 |
| frame | ascii4k | 9808 / 3 | 28200 / 9 | 9104 / 4 | 14568 / 16 |
| frame | zeros4k | 9808 / 3 | 28200 / 9 | 9104 / 4 | 21800 / 21 |

## 采样波动

以下列出五轮最大/最小耗时超过 2 的项目。原始文件保留全部样本；这个阈值只用于提示波动，不是显著性检验。

| 语言 | 项目 | 最小 µs | 中位 µs | 最大 µs | 最大/最小 |
| --- | --- | ---: | ---: | ---: | ---: |
| Go | ascii32/protobuf_payload/decode | 0.109 | 0.111 | 0.368 | 3.38 |
| Go | ascii32/sproto_frame/encode | 0.640 | 1.007 | 2.174 | 3.40 |
| Go | zeros4k/protobuf_payload/decode | 0.972 | 1.091 | 2.457 | 2.53 |
| Go | zeros4k/sproto_payload/encode | 2.126 | 2.965 | 7.610 | 3.58 |
| Go | zeros4k/sproto_payload/decode | 0.638 | 0.670 | 1.641 | 2.57 |
| Go | zeros4k/protobuf_frame/decode | 2.309 | 3.618 | 11.659 | 5.05 |
| Go | zeros4k/sproto_frame/encode | 8.417 | 8.915 | 17.118 | 2.03 |
| TypeScript | ascii32/sproto_payload/encode | 0.215 | 0.220 | 0.442 | 2.05 |
| TypeScript | ascii32/protobuf_frame/encode | 0.868 | 1.000 | 2.977 | 3.43 |
| TypeScript | ascii32/sproto_frame/encode | 0.935 | 0.952 | 2.079 | 2.22 |

## 复现与解释

从仓库根目录运行：

```sh
npm --prefix examples/websocket-gate ci
npm --prefix examples/websocket-gate run bench:protocol
```

默认产物位于 tmp/protocol-benchmark/：report.md、go.txt、typescript.json、environment.json。也可在 npm 命令末尾加 -- /绝对输出目录。原始结果保留每轮耗时；Go 原始结果另含吞吐和分配。

这是当前实现对 Echo 字符串的本机基线。zeros4k 是刻意构造的高压缩率输入，不能代表普通游戏消息；未覆盖整数数组、嵌套结构、浏览器、Lua/C 实现或端到端吞吐。Go Sproto 含反射、资源预算和失败原子性成本，TS 为示例参考实现，结果不能归因为 wire 格式的理论性能，也不能用于承诺所有消息的胜负。

## TS packing 中的无用调试计算

第一轮测量发现 write_ff 为每个字节调用 toString("hex") 并拼接到一个从未使用的字符串。删除的仅是字符串声明与拼接，保留原复制和 packing 算法。真实 Plain/MAC 集成测试五项通过、零跳过；所有基准 fixture 的字节数、SHA-256 和往返检查也通过。

下面的独立前后对照使用同一初版基准（均复用固定 package 头对象），仅删除这段无用计算，以隔离优化效果。最终横向对比表使用修正口径：两种协议均每次创建外层头对象，因此不将不同口径数据混成一行。

| 内容 | 删除前 TS SP frame 编码 µs | 删除后 µs | 删除前/删除后 |
| --- | ---: | ---: | ---: |
| ascii32 | 3.514 | 1.050 | 3.35 |
| utf8_1k | 86.601 | 6.269 | 13.81 |
| ascii4k | 452.330 | 19.641 | 23.03 |
| zeros4k | 9.932 | 9.886 | 1.00 |

前后对照不是对官方 C/Lua Sproto 的优化结论；它揭示的是当前 TS 参考代码中残留的调试开销。

## 保存的原始数据

- [最终 Go 五轮结果与分配](go.txt)
- [最终 TypeScript 五轮结果](typescript.json)
- [最终运行环境与工作树状态](environment.json)
- [删除调试计算之前的 TS 结果](typescript-before-debug-removal.json)
- [仅删除调试计算之后的 TS 结果](typescript-after-debug-removal.json)
- [首轮采样环境](environment-before-debug-removal.json)
- [Benchmark 使用说明与实现](../../../examples/websocket-gate/benchmarks/README.md)

最终表格由 scripts/benchmark-protocol.mjs 生成；本节和“本轮观察”为本次结果解读，普通重跑不会自动复制这些结论。没有新增第三方依赖或修改公共 API。
