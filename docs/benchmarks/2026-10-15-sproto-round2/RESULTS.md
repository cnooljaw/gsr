# 数值 packing、字段查询与类型化构造验收

按数值 Pack → TS 字段查询 → Go 标量写出 → Go 结构体解码 → 完整帧和高压缩率解包的顺序实施。保留公开 API、官方 wire、资源预算、optional presence、失败原子性和输出独占；未引入 unsafe、代码生成、池或共享结果对象。

Go 改动在独立 gosproto 未发布工作树，基础提交仍为 v0.4.0 / `3f2491f7b2701c710fdfc9dfa7952c8e7002dd1d`。GSR 正式依赖仍是 v0.4.0，以下新 Go 结果通过临时 modfile 接入；本轮尚未提交、推送或发布。

## 实施结果

- Go Pack 对完整 word 生成精确非零 mask，只复制有效字节；全非零 FF word 维持原快路径，尾部按实际长度处理。TS Pack/Unpack 使用固定八字节展开，减少混合数值 word 的循环开销。
- TS 每条结构只取得一次 schema，低 tag 使用有界稠密索引，其余使用 Map；数组直接使用已解析的 elementType。修复未知 body 字段不消费内容、导致后续字段读错的问题，低/高 tag 跨 schema 回归先复现错误再通过。
- Go 标量读值、inline 判定和写出合并，原生 *int64 / *string 直接构造独占值；命名类型继续走通用路径。缓存子结构 Go 类型。分配次数未因追求速度删减 optional 字段。
- TS 公开 encode 仍复制独占输出；pencode 和实际 attach/Response 在 scratch 中接续写入，再返回 Pack 的独占输出。扩容保留已完成 package；package 不挤占 body 原有的 8 MiB 编码额度，超额仍拒绝。

## 相对上一轮的增量

单位 µs/op，五轮中位数，负百分比表示耗时减少。上一轮为 [首轮优化结果](../2026-10-15-sproto-next/RESULTS.md)。原有基准的消息、签名与调用方式未变。

| 实现 | 消息 | 操作 | 上一轮 | 本轮 | 耗时变化 |
| --- | --- | --- | ---: | ---: | ---: |
| Go | AddressBook | encode | 0.569 | 0.545 | -4.2% |
| Go | AddressBook | decode | 1.175 | 1.032 | -12.2% |
| Go | AddressBook | pack | 0.265 | 0.168 | -36.7% |
| Go | AddressBook | unpack | 0.178 | 0.177 | -0.3% |
| Go | AddressBook | pencode | 0.829 | 0.708 | -14.5% |
| Go | AddressBook | pdecode | 1.392 | 1.220 | -12.4% |
| Go | 小型游戏状态 | encode | 1.436 | 1.407 | -2.0% |
| Go | 小型游戏状态 | decode | 2.272 | 1.856 | -18.3% |
| Go | 小型游戏状态 | pack | 0.577 | 0.363 | -37.1% |
| Go | 小型游戏状态 | unpack | 0.487 | 0.484 | -0.5% |
| Go | 小型游戏状态 | pencode | 1.995 | 1.765 | -11.5% |
| Go | 小型游戏状态 | pdecode | 2.820 | 2.357 | -16.4% |
| Go | 大型游戏状态 | encode | 14.781 | 14.536 | -1.7% |
| Go | 大型游戏状态 | decode | 23.310 | 18.152 | -22.1% |
| Go | 大型游戏状态 | pack | 5.500 | 3.483 | -36.7% |
| Go | 大型游戏状态 | unpack | 5.105 | 5.089 | -0.3% |
| Go | 大型游戏状态 | pencode | 20.249 | 18.075 | -10.7% |
| Go | 大型游戏状态 | pdecode | 28.525 | 23.530 | -17.5% |
| Go | 896 项整数数组 | encode | 2.224 | 2.220 | -0.2% |
| Go | 896 项整数数组 | decode | 2.276 | 2.372 | +4.2% |
| Go | 896 项整数数组 | pack | 6.063 | 3.744 | -38.2% |
| Go | 896 项整数数组 | unpack | 5.359 | 5.363 | +0.1% |
| Go | 896 项整数数组 | pencode | 8.257 | 5.987 | -27.5% |
| Go | 896 项整数数组 | pdecode | 7.711 | 7.740 | +0.4% |
| TS | AddressBook | encode | 0.754 | 0.876 | +16.1% |
| TS | AddressBook | decode | 1.030 | 1.045 | +1.4% |
| TS | AddressBook | pack | 0.441 | 0.448 | +1.5% |
| TS | AddressBook | unpack | 0.269 | 0.243 | -9.6% |
| TS | AddressBook | pencode | 1.195 | 1.822 | +52.4% |
| TS | AddressBook | pdecode | 1.307 | 1.260 | -3.6% |
| TS | 小型游戏状态 | encode | 1.698 | 2.111 | +24.3% |
| TS | 小型游戏状态 | decode | 1.821 | 1.639 | -10.0% |
| TS | 小型游戏状态 | pack | 0.755 | 0.668 | -11.5% |
| TS | 小型游戏状态 | unpack | 0.580 | 0.469 | -19.1% |
| TS | 小型游戏状态 | pencode | 2.469 | 2.366 | -4.2% |
| TS | 小型游戏状态 | pdecode | 2.402 | 2.029 | -15.5% |
| TS | 大型游戏状态 | encode | 17.886 | 18.463 | +3.2% |
| TS | 大型游戏状态 | decode | 16.497 | 16.136 | -2.2% |
| TS | 大型游戏状态 | pack | 6.791 | 5.594 | -17.6% |
| TS | 大型游戏状态 | unpack | 5.464 | 4.522 | -17.2% |
| TS | 大型游戏状态 | pencode | 24.812 | 24.772 | -0.2% |
| TS | 大型游戏状态 | pdecode | 22.187 | 19.230 | -13.3% |
| TS | 896 项整数数组 | encode | 9.732 | 10.134 | +4.1% |
| TS | 896 项整数数组 | decode | 4.649 | 5.126 | +10.3% |
| TS | 896 项整数数组 | pack | 7.182 | 5.805 | -19.2% |
| TS | 896 项整数数组 | unpack | 5.943 | 4.595 | -22.7% |
| TS | 896 项整数数组 | pencode | 16.883 | 15.996 | -5.3% |
| TS | 896 项整数数组 | pdecode | 10.362 | 9.145 | -11.7% |

## 与上次正式基线的 Protobuf 对照

PB 取 [v0.4.0 正式依赖完整复测](../2026-10-15-v0.4.0/report.md) 的数据，SP 取本轮最终结果。当前同进程控制用 PB 数据仍保留在各明细报告中；小幅跨次差异不能当作统计显著收益。

| body | 操作 | Go PB | Go SP | TS PB | TS SP |
| --- | --- | ---: | ---: | ---: | ---: |
| AddressBook | encode | 0.339 | 0.545 | 1.176 | 0.876 |
| AddressBook | decode | 0.844 | 1.032 | 1.407 | 1.045 |
| 小型游戏状态 | encode | 0.682 | 1.407 | 2.534 | 2.111 |
| 小型游戏状态 | decode | 1.672 | 1.856 | 3.014 | 1.639 |
| 大型游戏状态 | encode | 7.756 | 14.536 | 22.781 | 18.463 |
| 大型游戏状态 | decode | 17.505 | 18.152 | 31.551 | 16.136 |
| 896 项整数数组 | encode | 5.030 | 2.220 | 12.514 | 10.134 |
| 896 项整数数组 | decode | 4.208 | 2.372 | 11.207 | 5.126 |
| 4 KiB ASCII body | encode | 0.617 | 0.634 | 5.828 | 1.432 |
| 4 KiB ASCII body | decode | 0.666 | 0.686 | 0.655 | 0.519 |

| body 含 packing | 操作 | Go PB | Go SP | TS PB | TS SP |
| --- | --- | ---: | ---: | ---: | ---: |
| AddressBook | encode | 0.339 | 0.708 | 1.176 | 1.822 |
| AddressBook | decode | 0.844 | 1.220 | 1.407 | 1.260 |
| 小型游戏状态 | encode | 0.682 | 1.765 | 2.534 | 2.366 |
| 小型游戏状态 | decode | 1.672 | 2.357 | 3.014 | 2.029 |
| 大型游戏状态 | encode | 7.756 | 18.075 | 22.781 | 24.772 |
| 大型游戏状态 | decode | 17.505 | 23.530 | 31.551 | 19.230 |
| 896 项整数数组 | encode | 5.030 | 5.987 | 12.514 | 15.996 |
| 896 项整数数组 | decode | 4.208 | 7.740 | 11.207 | 9.145 |
| 4 KiB ASCII body | encode | 0.617 | 2.757 | 5.828 | 2.126 |
| 4 KiB ASCII body | decode | 0.666 | 1.256 | 0.655 | 1.502 |

| Echo 层级 | 内容 | 操作 | Go PB | Go SP | TS PB | TS SP |
| --- | --- | --- | ---: | ---: | ---: | ---: |
| payload | 32 B ASCII | encode | 0.067 | 0.089 | 0.393 | 0.148 |
| payload | 32 B ASCII | decode | 0.109 | 0.163 | 0.317 | 0.111 |
| payload | 1000 B UTF-8 | encode | 1.286 | 0.225 | 1.869 | 1.736 |
| payload | 1000 B UTF-8 | decode | 1.355 | 0.305 | 4.387 | 4.455 |
| payload | 4 KiB ASCII | encode | 0.879 | 0.598 | 5.863 | 1.529 |
| payload | 4 KiB ASCII | decode | 0.988 | 0.806 | 0.740 | 0.566 |
| payload | 4 KiB 零字节 | encode | 0.872 | 0.638 | 5.617 | 1.468 |
| payload | 4 KiB 零字节 | decode | 0.998 | 0.730 | 0.745 | 0.599 |
| frame | 32 B ASCII | encode | 0.188 | 0.307 | 0.890 | 0.678 |
| frame | 32 B ASCII | decode | 0.250 | 0.372 | 0.653 | 0.345 |
| frame | 1000 B UTF-8 | encode | 1.549 | 1.167 | 3.003 | 4.439 |
| frame | 1000 B UTF-8 | decode | 1.608 | 0.642 | 4.747 | 4.755 |
| frame | 4 KiB ASCII | encode | 1.474 | 3.994 | 7.823 | 3.218 |
| frame | 4 KiB ASCII | decode | 1.522 | 1.318 | 1.073 | 1.663 |
| frame | 4 KiB 零字节 | encode | 1.770 | 4.203 | 7.879 | 3.844 |
| frame | 4 KiB 零字节 | decode | 1.534 | 3.433 | 1.074 | 2.932 |

## TS 原生 API：同进程交错复核

最终完整 TS 采样存在明显负载波动：未改动的 PB 大型消息编码为 26.376 µs，上一轮控制值为 22.418 µs；多个 SP 操作也出现超过两倍的最慢轮次。上面的跨次原始值全部保留。下面的同进程前后交错结果用于复核代码变化，两种方法不混合取中位数；不能将跨次增加的全部耗时归因于实现回退。

原有 body 套件用 pack(encode(...))，Echo 用两次 encode 后 concat/pack，继续计入中间独占复制。它们没有替换成融合入口，因此不能用它们的耗时声称体现全部 RPC 优化。以下对 Encode/Decode、Pack/Unpack、PEncode/PDecode 与 attach 分别比较相同调用，源码分别为首轮快照和本轮；两版在同一进程中交错、反转次序，每操作热身至少约 50 ms、五轮各约 100 ms。

使用已有代表性夹具和实际 C2S schema，固定 session=1 覆盖同一等待项；不含外层客户端安全预检、会话分配、网络或 MAC。每项先验证固定 SHA-256 和输出快照独占。

| 消息 | API | 上一轮 | 本轮 | 耗时变化 |
| --- | --- | ---: | ---: | ---: |
| AddressBook | encode | 0.884 | 0.853 | -3.6% |
| AddressBook | decode | 1.201 | 0.882 | -26.6% |
| AddressBook | pack | 0.467 | 0.370 | -20.7% |
| AddressBook | unpack | 0.274 | 0.233 | -14.9% |
| AddressBook | pencode | 1.256 | 1.143 | -9.1% |
| AddressBook | pdecode | 1.427 | 1.068 | -25.2% |
| 小型游戏状态 | encode | 1.796 | 1.758 | -2.1% |
| 小型游戏状态 | decode | 1.841 | 1.433 | -22.2% |
| 小型游戏状态 | pack | 0.764 | 0.635 | -16.9% |
| 小型游戏状态 | unpack | 0.603 | 0.484 | -19.8% |
| 小型游戏状态 | pencode | 2.631 | 2.354 | -10.5% |
| 小型游戏状态 | pdecode | 2.458 | 2.033 | -17.3% |
| 大型游戏状态 | encode | 19.586 | 19.021 | -2.9% |
| 大型游戏状态 | decode | 16.439 | 14.134 | -14.0% |
| 大型游戏状态 | pack | 7.369 | 5.687 | -22.8% |
| 大型游戏状态 | unpack | 6.034 | 4.995 | -17.2% |
| 大型游戏状态 | pencode | 31.348 | 27.232 | -13.1% |
| 大型游戏状态 | pdecode | 22.558 | 19.662 | -12.8% |
| 896 项整数数组 | encode | 10.597 | 10.180 | -3.9% |
| 896 项整数数组 | decode | 5.398 | 4.641 | -14.0% |
| 896 项整数数组 | pack | 7.238 | 5.663 | -21.8% |
| 896 项整数数组 | unpack | 5.820 | 4.590 | -21.1% |
| 896 项整数数组 | pencode | 17.024 | 14.855 | -12.7% |
| 896 项整数数组 | pdecode | 10.735 | 9.054 | -15.7% |
| 4 KiB ASCII body | encode | 1.281 | 1.230 | -4.0% |
| 4 KiB ASCII body | decode | 0.540 | 0.484 | -10.5% |
| 4 KiB ASCII body | pack | 1.049 | 0.935 | -10.9% |
| 4 KiB ASCII body | unpack | 1.108 | 1.065 | -3.9% |
| 4 KiB ASCII body | pencode | 2.265 | 1.675 | -26.0% |
| 4 KiB ASCII body | pdecode | 1.521 | 1.995 | +31.2% |
| 32 B ASCII | rpc_encode | 0.676 | 0.500 | -25.9% |
| 1000 B UTF-8 | rpc_encode | 3.641 | 3.322 | -8.7% |
| 4 KiB ASCII | rpc_encode | 3.480 | 1.768 | -49.2% |
| 4 KiB 零字节 | rpc_encode | 3.662 | 2.367 | -35.4% |

完整原始值及模块指纹见 [API 数据](api/typescript.json)，脚本见 [benchmark-sproto-api.mjs](../../../examples/websocket-gate/scripts/benchmark-sproto-api.mjs)。它不代表完整 SprotoEchoClient 或网络吞吐。

## 内存、撤回与限制

Go 分配与上一轮逐项一致，本轮没有新增逐消息分配。首轮已有的大型布局额外 1024 B / 1 次、新建 Echo package 额外 16 B / 1 次仍保留，没有用移除对象独占换速度。

| 套件 | 操作 | 上一轮 B/op / allocs | 本轮 B/op / allocs |
| --- | --- | ---: | ---: | ---: |

TS 增加每份 schema 的有界索引，scratch 仍为实例私有同步缓冲。返回的 encode/pencode/RPC 数据独占，不返回可被下次编码覆盖的 scratch 视图。

- 撤回 Go 解包位置表：固定 1 KiB 元信息没有换来数值收益，专项反而约慢 7%～11%。
- 撤回 TS 非零 mask 算术方案：普通数值 Pack 回退约一成；最终采用八字节展开。
- 撤回 TS 预清零输出、仅写非零字节：数值解包改善，但 ASCII 解包约慢 18%；最终保留旧分配策略并展开普通 word。
- Go mask Pack 首版让所有 FF word 重新计算 mask，ASCII 回退；恢复原 FF 快路径后再采集最终结果。
- 高压缩率解包仍有有界二次扫描和重新分配，本轮没有通过放大默认预分配或跳过预算检查强行加速。

本机结果不承诺 SP 全场景胜过 PB。Go 结构体编码仍有明显差距，Go Unpack 数值成本尚未改善；TS 原生 API 的收益与原分阶段基准分别报告。

## 验证与来源

- Go 全部八位 mask × 4 种非零值（含 0x80/0xff）按官方 packing 规则检查；保留 FF 256-word、尾部、资源和已发布固定向量。原生及命名 optional 整数/字符串 presence、空值和 Unicode 扩展到现有回归。
- 独立库完整官方 C/Lua 对照与 race 通过，vet 通过；FuzzPackRoundTrip、FuzzCodecRoundTrip、FuzzDecode 各约 10 秒通过，分别执行 38870、616684、9782 次。有限 fuzz 不代表穷尽。
- TS 3416 组新旧 Pack/Unpack 差分通过，包含高位字节、所有边界附近长度、不同 offset、截断与有限前缀；20 组独立官方 raw/packed 向量通过。未知 body 字段错位和合并 scratch 预算边界均先复现后修复。
- GSR 相关三包 race 通过；最终 Go/TS 真实 Plain/MAC 集成六项全部通过、零跳过，覆盖长报文、Request/Response、Notify/Push、断线和坏包。未改动 Core 或 Gate，不重复运行无关全仓门禁。

最终两套原基准共 720 个样本，API 对照另 340 个；CPU/分配 profile 在计时完成后另行采集。Apple M2，darwin 23.4.0 arm64，go version go1.24.13 darwin/arm64，Node v23.11.0。代表性开始 2026-10-15T07:27:09.869Z，Echo 开始 2026-10-15T07:29:12.471Z。

代表性负载：开始 [5.36962890625, 5.58935546875, 5.3603515625]，结束 [6.87451171875, 5.94580078125, 5.51611328125]。下表提示 max/min > 2 的项目，全部样本保留，中位数不构成显著性检验。

| 套件 | 语言 | 操作 | min µs | median µs | max µs |
| --- | --- | --- | ---: | ---: | ---: |
| body | TS | addressbook/sproto_encode | 0.810 | 0.876 | 4.780 |
| body | TS | addressbook/sproto_unpack | 0.227 | 0.243 | 0.593 |
| body | TS | addressbook/sproto_pdecode | 1.108 | 1.260 | 2.227 |
| body | TS | game_small/sproto_decode | 1.522 | 1.639 | 3.905 |
| body | TS | game_large/sproto_encode | 18.282 | 18.463 | 38.783 |
| body | TS | game_large/sproto_decode | 14.038 | 16.136 | 32.977 |
| body | TS | game_large/sproto_pencode | 22.806 | 24.772 | 46.797 |
| body | TS | game_large/sproto_pdecode | 17.420 | 19.230 | 38.919 |
| body | TS | int32_edges/protobuf_encode | 12.565 | 14.226 | 42.640 |
| body | TS | int32_edges/sproto_decode | 4.631 | 5.126 | 10.024 |
| body | TS | echo_ascii4k/protobuf_encode | 5.697 | 5.913 | 13.788 |
| Echo | Go | ascii4k/sproto_frame/encode | 3.422 | 3.994 | 9.907 |
| Echo | Go | zeros4k/protobuf_payload/encode | 0.892 | 0.925 | 2.052 |
| Echo | TS | ascii32/protobuf_frame/decode | 0.665 | 0.730 | 1.788 |
| Echo | TS | zeros4k/protobuf_frame/decode | 1.155 | 1.190 | 2.879 |

Go 编码仍主要消耗在字段遍历、尺寸预检和反射：encodedSize 累计约 38.7%，encodeMessageInto 累计约 46.2%，reflect.Value.Elem/Field self 合计约 18.7%。累计指标存在调用关系，不相加。解码 profile 中 reflect.Value.Set 累计约 19.8%，runtime.mallocgc 累计约 23.4%；类型化构造减少动态反射构造成本，未减少必要的 optional 对象。Go Pack 热点转到 packWord（self 约 40.9%）；Go Unpack 仍有后续空间。

TS 解码 self 仍集中于 ldecode/decode_array（约 61.0% / 32.9%）；Pack/Unpack self 约 87.0% / 85.5%。这些是更短执行时间内的采样归属，不可直接用占比差值推算绝对收益。V8 sampled heap 不完整计入 Buffer 外部内存，完整 profile 不用于计时表格。

Go Echo 的 ASCII 全量帧编码也出现波动（中位数 3.994 µs，最慢 9.907 µs）。保持生产源码不变，用首轮与本轮基准二进制交错、反转顺序各测五轮：

| Go frame 编码 | 首轮 | 本轮 |
| --- | ---: | ---: |
| 32 B ASCII | 0.335 | 0.331 |
| 4 KiB ASCII | 3.290 | 3.298 |
| 4 KiB 零字节 | 7.606 | 4.701 |

ASCII 复核约 +0.2%，没有支持持续 24% 回退；零字节约 -38.2%。[首轮原始值](echo-check/before.txt)、[本轮原始值](echo-check/after.txt)、[顺序和二进制指纹](echo-check/environment.json) 单独保留，不替换全量统计。这一专项另有 30 个样本。

原始值：[代表性消息](representative/report.md)、[Echo](echo/report.md)、[输入指纹](inputs.json)；profile：[Go](profile-go-summary.json)、[TS](profile-typescript-summary.json)、[环境](profile-environment.json)。完整 profile、测试输出、中间尝试留在忽略的 tmp/，不复制独立 Go 库源码补丁。

## 复现

先按 [首轮复现说明](../2026-10-15-sproto-next/RESULTS.md) 准备临时 modfile，指向本轮优化的独立 Go 库；正式 v0.4.0 不包含这些未发布改动。随后顺序运行：

```sh
GOFLAGS=-modfile=/tmp/gsr-next-codec.mod \
  npm --prefix examples/websocket-gate run bench:representative -- /absolute/output/representative
GOFLAGS=-modfile=/tmp/gsr-next-codec.mod \
  npm --prefix examples/websocket-gate run bench:protocol -- /absolute/output/echo
node examples/websocket-gate/scripts/benchmark-sproto-api.mjs /absolute/output/api /absolute/baseline/sproto.mjs
GOFLAGS=-modfile=/tmp/gsr-next-codec.mod GSR_PROFILE_SCENARIO=game_large \
  npm --prefix examples/websocket-gate run profile:protocol -- /absolute/output/profile
```

没有测浏览器、完整 int64/BigInt、double/map 性能专项、C/Lua 速度排名或部署吞吐；TS 仍为 Node Buffer 参考实现。
