# 当前已提交 gosproto / Protobuf 同场基线

2026-10-20，gosproto `a93bb0d`；Go 1.24.13、Apple M2、Protobuf v1.36.1。正式依赖仍为 v0.4.0，使用 Git 归档及临时 modfile；没有修改库实现。一个测试二进制仅加载一份官方模块身份的 Sproto。

原有 72 项、每项五轮 200ms，共 360 个样本；GOMAXPROCS=1、GOGC=100、GOMEMLIMIT=off。先构建并校验代表消息的语义、wire、消费长度和 padding，再采样；全部样本保留。分配、逐轮数据及范围见 analysis.json。

## 正文与 Packing（µs/op）

| 消息 | PB Encode | SP Encode | Pack | PEncode | PB Decode | SP Decode | Unpack | PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 0.326 | 0.533 | 0.105 | 0.605 | 0.809 | 1.113 | 0.168 | 1.182 |
| game_small | 0.661 | 0.625 | 0.224 | 0.793 | 1.633 | 1.504 | 0.457 | 1.979 |
| game_large | 7.449 | 4.350 | 2.028 | 5.828 | 17.003 | 14.033 | 4.617 | 18.855 |
| int32_edges | 4.804 | 1.619 | 1.935 | 3.135 | 4.127 | 1.670 | 4.980 | 6.657 |
| echo_ascii4k | 0.568 | 0.619 | 0.596 | 0.729 | 0.619 | 0.651 | 0.570 | 1.180 |

## 相对各自语言的 Protobuf

负值表示 Sproto 更快。Go 为本轮同场实测；TS 为 2026-10-15 第五轮历史数据（Node 23.11、PB 2.15.0），没有重跑 TS。这是同语言比例参照，不是跨语言绝对性能排名。

| 消息 | Go Encode | TS Encode | Go Decode | TS Decode | Go PEncode | TS PEncode | Go PDecode | TS PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | +63.5% | -34.9% | +37.6% | -41.3% | +85.5% | -11.1% | +46.2% | -28.1% |
| game_small | -5.5% | -39.9% | -7.9% | -53.2% | +19.9% | -18.3% | +21.2% | -39.8% |
| game_large | -41.6% | -35.9% | -17.5% | -57.2% | -21.8% | -15.4% | +10.9% | -44.8% |
| int32_edges | -66.3% | -76.7% | -59.5% | -58.3% | -34.7% | -37.1% | +61.3% | -18.4% |
| echo_ascii4k | +8.9% | -77.8% | +5.2% | -27.0% | +28.3% | -72.8% | +90.5% | -10.6% |

## 分配（B/op / allocs/op）

| 消息 | PB Encode | SP Encode | Pack | PEncode | PB Decode | SP Decode | Unpack | PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 80 / 1 | 144 / 1 | 144 / 1 | 144 / 1 | 648 / 26 | 528 / 29 | 144 / 1 | 672 / 30 |
| game_small | 192 / 1 | 352 / 1 | 352 / 1 | 352 / 1 | 1208 / 53 | 1200 / 52 | 352 / 1 | 1552 / 53 |
| game_large | 2304 / 1 | 3968 / 2 | 3456 / 1 | 3968 / 2 | 13560 / 537 | 10416 / 532 | 3456 / 1 | 13872 / 533 |
| int32_edges | 2688 / 1 | 4096 / 1 | 4096 / 1 | 4096 / 1 | 4160 / 2 | 8288 / 4 | 4096 / 1 | 12384 / 5 |
| echo_ascii4k | 4864 / 1 | 4864 / 1 | 4864 / 1 | 4864 / 1 | 4160 / 3 | 4176 / 5 | 4864 / 1 | 9040 / 6 |

## 采样限制

每套件采用 Go benchmark 的固定顺序、单进程 count=5，属于总览，不是独立进程 ABBA 的细小收益验收。没有丢弃长尾，也不把历史基线差值归因于某一提交；旧版 af9169d 的 PB 和 SP 均可能受机器状态影响。没有把旧 PB 耗时乘以局部优化比例。

未 packed 正文与 PB 是不同 wire 格式；最终发送性能同时看 PEncode/PDecode。单独 Pack/Unpack 的中位数不要求与正文相加等于组合路径。数值数组分别使用 SP int64 / PB sint32；字符串 UTF-8 校验、presence、资源校验及对象模型差异保留。Echo frame 是手工拼包，不代替融合 Rpc API。没有新 CPU profile，也未重复运行无代码变化的全库/race/fuzz/oracle/GoTS。

## 波动最大的项目

| 项目 | min µs | median µs | max µs |
| --- | ---: | ---: | ---: |
| representative/addressbook/sproto_decode | 0.977 | 1.113 | 2.298 |
| protocol/ascii32/protobuf_payload/encode | 0.067 | 0.081 | 0.144 |
| protocol/utf8_1k/sproto_frame/decode | 0.574 | 0.588 | 1.098 |
| representative/echo_ascii4k/sproto_unpack | 0.539 | 0.570 | 0.882 |
| representative/addressbook/sproto_pencode | 0.600 | 0.605 | 0.879 |
| representative/addressbook/sproto_unpack | 0.164 | 0.168 | 0.226 |
| protocol/ascii4k/sproto_frame/encode | 1.502 | 1.534 | 2.057 |
| protocol/ascii4k/sproto_payload/decode | 0.573 | 0.614 | 0.783 |
| representative/echo_ascii4k/sproto_pack | 0.574 | 0.596 | 0.783 |
| representative/int32_edges/sproto_encode | 1.608 | 1.619 | 2.136 |

## 下一轮方向

- Body：大型 Encode 本轮快 PB 41.6%，已经达到并超过历史 TS 的相对领先幅度，但不是冻结理由。优先定位 AddressBook 的字段/presence 固定成本（Encode +63.5%、Decode +37.6%），以及大型 Decode 的对象构造与字段访问（Go -17.5%，历史 TS -57.2%）。先用同一模块的 profile 和调用计数定位，再做单一候选。
- Packing：整数数组 Unpack 4.980 µs，大于其正文 Decode 1.670 µs；大型 Unpack 4.617 µs，组合 Decode 仍输 PB 10.9%。优先检查扫描、输出写入与组合入口的额外工作，不删除预算/边界检查、不借用结果缓冲；Pack 保持普通 word、零段、FF、尾部和小包控制。
- ASCII 的正文 Encode/Decode 本轮分别慢 PB 8.9%/5.2%，packed Decode 慢 90.5%；需要分别定位字节复制/分配和 Unpack，不用大型结构的结果代替字符串结论。

仅凭这些独立入口耗时不能断言具体瓶颈函数；当前没有新 profile。下一步在上述两个主线中各定位一个可验证热点，保持 Decode 开放。

全部项目及历史观测见 [ALL-CASES.md](ALL-CASES.md)。历史差值不作为某一提交的净收益证明。

复现：`python3 docs/benchmarks/2026-10-20-go-sp-pb-current/reproduce.py --library /private/tmp/gsr-gosproto-fork --output /absolute/new-output`。输出目录必须不存在；随后将输出中的文本/JSON复制到本报告目录，运行 `analyze.py` 独立复算 72 项中位数。模块解析及 build-info 同时保留。库工作区原六项删除未恢复，正式依赖未变。
