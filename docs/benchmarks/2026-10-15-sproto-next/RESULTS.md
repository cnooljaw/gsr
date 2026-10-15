# TS 长块与 gosproto 反射路径优化验收

先完成 TS 长 FF/零块的 Pack/Unpack，再按 [gosproto 性能方案](../../RFC-GOSPROTO-PERFORMANCE.md) 实施安全 Go 优化。保留官方 wire、资源预算、optional presence、失败原子性和输出独占；没有新增公开 API、unsafe、generator 或依赖。

TS 修改在 GSR 工作树；Go 修改在独立 gosproto 工作树，基础为已发布 v0.4.0 / `3f2491f7b2701c710fdfc9dfa7952c8e7002dd1d`，本轮尚未提交或发布。GSR 正式 go.mod/go.sum 仍固定 v0.4.0；以下 Go 新结果使用临时 modfile 接入，不能标成正式 v0.4.0 的性能。

与 [正式 v0.4.0 基线](../2026-10-15-v0.4.0/report.md) 使用同一消息、schema、工具链和脚本。最终两套基准顺序采集，各语言 72 项、每项五轮，共 720 个计时样本；profile 在计时结束后另起进程。所有固定签名、完整语义、padding 与两个 TS 能力探针通过。

耗时为 µs/op，取五轮中位数；变化百分比为耗时变化，负数表示减少。本机有桌面负载，小幅差异不构成稳定收益或显著性结论。

## Go 前后对照

纯 Encode/Decode 不含 packing；PEncode=Encode+Pack，PDecode=Unpack+Decode。

| 消息 | 操作 | v0.4.0 | 本轮 | 耗时变化 |
| --- | --- | ---: | ---: | ---: |
| AddressBook | encode | 0.871 | 0.569 | -34.7% |
| AddressBook | decode | 1.280 | 1.175 | -8.2% |
| AddressBook | pencode | 1.124 | 0.829 | -26.3% |
| AddressBook | pdecode | 1.485 | 1.392 | -6.3% |
| 小型游戏状态 | encode | 2.616 | 1.436 | -45.1% |
| 小型游戏状态 | decode | 2.983 | 2.272 | -23.8% |
| 小型游戏状态 | pencode | 3.186 | 1.995 | -37.4% |
| 小型游戏状态 | pdecode | 3.503 | 2.820 | -19.5% |
| 大型游戏状态 | encode | 26.973 | 14.781 | -45.2% |
| 大型游戏状态 | decode | 29.063 | 23.310 | -19.8% |
| 大型游戏状态 | pencode | 32.453 | 20.249 | -37.6% |
| 大型游戏状态 | pdecode | 34.298 | 28.525 | -16.8% |
| 896 项 int32 边界数组 | encode | 13.125 | 2.224 | -83.1% |
| 896 项 int32 边界数组 | decode | 7.334 | 2.276 | -69.0% |
| 896 项 int32 边界数组 | pencode | 19.226 | 8.257 | -57.1% |
| 896 项 int32 边界数组 | pdecode | 12.726 | 7.711 | -39.4% |
| 4 KiB ASCII body | encode | 0.647 | 0.659 | +1.9% |
| 4 KiB ASCII body | decode | 0.761 | 0.701 | -7.9% |
| 4 KiB ASCII body | pencode | 2.815 | 2.770 | -1.6% |
| 4 KiB ASCII body | pdecode | 1.252 | 1.312 | +4.8% |

## TS 前后对照

完整应用帧包含原生 type/session package 及 packing；未混入网络、MAC、Dispatcher、RPC 会话表或业务 Handler。

| 内容 | frame 操作 | v0.4.0 基线 | 本轮 | 耗时变化 |
| --- | --- | ---: | ---: | ---: |
| 32 B ASCII | encode | 0.595 | 0.600 | +0.7% |
| 32 B ASCII | decode | 0.329 | 0.332 | +0.9% |
| 1000 B UTF-8 | encode | 3.410 | 3.562 | +4.5% |
| 1000 B UTF-8 | decode | 4.483 | 4.617 | +3.0% |
| 4 KiB ASCII | encode | 7.660 | 3.263 | -57.4% |
| 4 KiB ASCII | decode | 1.507 | 1.584 | +5.1% |
| 4 KiB 零字节 | encode | 9.306 | 3.393 | -63.5% |
| 4 KiB 零字节 | decode | 11.854 | 2.640 | -77.7% |

| body | 步骤 | v0.4.0 基线 | 本轮 | 耗时变化 |
| --- | --- | ---: | ---: | ---: |
| AddressBook | pack | 0.423 | 0.441 | +4.4% |
| AddressBook | unpack | 0.262 | 0.269 | +2.5% |
| AddressBook | pencode | 1.176 | 1.195 | +1.6% |
| AddressBook | pdecode | 1.285 | 1.307 | +1.8% |
| 小型游戏状态 | pack | 0.742 | 0.755 | +1.7% |
| 小型游戏状态 | unpack | 0.559 | 0.580 | +3.7% |
| 小型游戏状态 | pencode | 2.426 | 2.469 | +1.8% |
| 小型游戏状态 | pdecode | 2.388 | 2.402 | +0.6% |
| 大型游戏状态 | pack | 6.601 | 6.791 | +2.9% |
| 大型游戏状态 | unpack | 5.320 | 5.464 | +2.7% |
| 大型游戏状态 | pencode | 24.619 | 24.812 | +0.8% |
| 大型游戏状态 | pdecode | 21.948 | 22.187 | +1.1% |
| 896 项 int32 边界数组 | pack | 6.983 | 7.182 | +2.9% |
| 896 项 int32 边界数组 | unpack | 5.687 | 5.943 | +4.5% |
| 896 项 int32 边界数组 | pencode | 16.605 | 16.883 | +1.7% |
| 896 项 int32 边界数组 | pdecode | 10.227 | 10.362 | +1.3% |
| 4 KiB ASCII body | pack | 5.572 | 0.901 | -83.8% |
| 4 KiB ASCII body | unpack | 0.948 | 0.979 | +3.3% |
| 4 KiB ASCII body | pencode | 6.730 | 2.107 | -68.7% |
| 4 KiB ASCII body | pdecode | 1.509 | 1.452 | -3.8% |

## 最终四种实现：纯 body

| 消息 | 操作 | Go PB | Go SP | TS PB | TS SP |
| --- | --- | ---: | ---: | ---: | ---: |
| AddressBook | encode | 0.343 | 0.569 | 1.172 | 0.754 |
| AddressBook | decode | 0.848 | 1.175 | 1.401 | 1.030 |
| 小型游戏状态 | encode | 0.709 | 1.436 | 2.494 | 1.698 |
| 小型游戏状态 | decode | 1.672 | 2.272 | 3.002 | 1.821 |
| 大型游戏状态 | encode | 7.896 | 14.781 | 22.418 | 17.886 |
| 大型游戏状态 | decode | 17.464 | 23.310 | 31.978 | 16.497 |
| 896 项 int32 边界数组 | encode | 5.219 | 2.224 | 12.477 | 9.732 |
| 896 项 int32 边界数组 | decode | 4.204 | 2.276 | 11.034 | 4.649 |
| 4 KiB ASCII body | encode | 0.603 | 0.659 | 5.611 | 1.235 |
| 4 KiB ASCII body | decode | 0.674 | 0.701 | 0.712 | 0.487 |

## 最终四种实现：body 含 packing

PB 使用原生 Encode/Decode，SP 使用 PEncode/PDecode；这反映当前不同 wire 的实际处理成本。

| 消息 | 操作 | Go PB | Go SP+packing | TS PB | TS SP+packing |
| --- | --- | ---: | ---: | ---: | ---: |
| AddressBook | encode | 0.343 | 0.829 | 1.172 | 1.195 |
| AddressBook | decode | 0.848 | 1.392 | 1.401 | 1.307 |
| 小型游戏状态 | encode | 0.709 | 1.995 | 2.494 | 2.469 |
| 小型游戏状态 | decode | 1.672 | 2.820 | 3.002 | 2.402 |
| 大型游戏状态 | encode | 7.896 | 20.249 | 22.418 | 24.812 |
| 大型游戏状态 | decode | 17.464 | 28.525 | 31.978 | 22.187 |
| 896 项 int32 边界数组 | encode | 5.219 | 8.257 | 12.477 | 16.883 |
| 896 项 int32 边界数组 | decode | 4.204 | 7.711 | 11.034 | 10.362 |
| 4 KiB ASCII body | encode | 0.603 | 2.770 | 5.611 | 2.107 |
| 4 KiB ASCII body | decode | 0.674 | 1.312 | 0.712 | 1.452 |

## Go 内存代价

| 消息 | 步骤 | 之前 B/op / allocs | 本轮 B/op / allocs |
| --- | --- | ---: | ---: |
| AddressBook | encode | 144 / 1 | 144 / 1 |
| AddressBook | decode | 528 / 29 | 528 / 29 |
| 小型游戏状态 | encode | 352 / 1 | 352 / 1 |
| 小型游戏状态 | decode | 1248 / 54 | 1200 / 52 |
| 大型游戏状态 | encode | 3456 / 1 | 4480 / 2 |
| 大型游戏状态 | decode | 10464 / 534 | 10416 / 532 |
| 896 项 int32 边界数组 | encode | 4096 / 1 | 4096 / 1 |
| 896 项 int32 边界数组 | decode | 8312 / 5 | 8288 / 4 |
| 4 KiB ASCII body | encode | 4864 / 1 | 4864 / 1 |
| 4 KiB ASCII body | decode | 4176 / 5 | 4176 / 5 |

大型游戏消息布局记录额外占 1024 B / 1 次分配；复用输入的小消息仍为一次编码输出分配。Echo 帧每次新建的两指针 package 因安全类型化访问改变逃逸分析，额外占 16 B / 1 次分配：32 B 帧从 168 B / 4 次变为 184 B / 5 次，4 KiB 帧从 14600 B / 4 次变为 14616 B / 5 次。这里没有按每个切片元素装箱，但不能据纯 payload 结果宣称新建对象场景也没有额外分配。896 项数组的提升主要来自原生 []int64 批量循环，不应外推为所有命名类型或整数切片均有同等收益。解码没有复用上条消息的子对象，optional 指针和独占对象仍有实际分配成本。

## 采用与撤回的实现

- TS：长 FF 块使用有界 Buffer 原生非零扫描；长零块批量写 mask；解包合并连续零 word，首次缓冲耗尽后继续检查与计数。普通数值 word 沿用原循环。
- Go：schema 固定 nilability、指针形式、signedness 与目标类型；每条结构取得一次 Elem，直接按字段编号访问；原生 []int64 专用循环，其他类型保留兼容路径。
- Go：预检记录当前调用的结构体头项数及数组宽度，双游标一次写出 header/body；记录不含业务对象指针，不跨调用缓存。低 tag 使用最多 128 项稠密索引，高 tag 保留 map。
- 撤回：TS pack_seg 加零检查、逐大包 Uint32Array 视图及过早进入原生块处理，均在普通数值或小包出现回退；最终限制为长块。
- 撤回：Go 零 word Pack 分支使普通数值约慢 5–6%、ASCII Pack 约慢 16%；最终 pack.go 与发布版完全一致。这些是中间试验，不作为最终采样。

TS 的收益集中于长 ASCII/零字节；数值 Pack/Unpack 未改善，完整路径仍有小幅波动。Go 大型游戏消息仍慢于本轮 Go Protobuf；Go 整数数组纯编码/解码已快于该 PB 实现，但加入 packing 后优势不同。不跨场景平均成总排名。

## 验证增量

- 扩展现有 TS 测试：混合零块、有限输出前缀与完整长度、截断拒绝和 8 MiB 上限。保留 20 组官方 raw/packed 向量；开发期新旧实现差分 3200 组输入通过。
- 扩展 Go 整数范围、命名类型、未知/高 tag 用例；新增同一对象改变 presence、数组宽度、空/nil 数组和嵌套数量后重编码，检查旧输出不变。
- 独立 Go 库完整测试、固定 C/Lua 对照、全库 race 与 vet 通过；三个 fuzz 入口各运行约 10 秒通过。有限 fuzz 不代表穷尽。
- 临时 modfile 下，protocol/sproto、Echo 服务端和 Benchmark 包 race 通过；真实 Plain/MAC Go↔TS 集成六项全部通过、零跳过。仅清理未使用参数名后再做最终源码五轮采样，不改变运行逻辑。

## Profile 与剩余工作

大型游戏消息的 Go/TS 六步骤分别采集 CPU 与分配，摘要见 [Go](profile-go-summary.json)、[TS](profile-typescript-summary.json)、[采集环境](profile-environment.json)。CPU 采样与分配采样分开；V8 sampled heap 不完整计入 Buffer 外部内存，不能当作精确 B/op。完整 profile/二进制仍位于忽略的 tmp/。

Go 编码 CPU 中，encodedSize 累计约 44.2%、inlineHeader 累计约 13.7%；这是更短总耗时内的采样占比，不能与旧占比相减推算耗时，也不能将存在调用关系的累计值相加。解码 reflect.New 累计约 32.2%，分配对象归属仍以它为主，符合 optional 指针和独占子对象的成本。下一步若继续优化，应针对具体字段访问及对象构造验证，不能靠移除预算检查或 optional presence。

TS 普通游戏数值数据的 Pack/Unpack 仍主要落在 pack/lunpack 循环（self 约 92.3% / 89.3%），本轮长块方案没有解决这些混合 word 的开销。这里的 profile 仅采 game_large，不拿它解释零字节专项的精确函数占比。

## 来源、波动与复现

Apple M2；darwin 23.4.0 arm64；go version go1.24.13 darwin/arm64；Node v23.11.0。Go PB v1.36.1、TS PB 2.15.0，TS SP 为本项目修正的 sealindx/sproto-ts。代表性采样开始 2026-10-15T06:29:05.080Z，Echo 开始 2026-10-15T06:30:49.078Z。

代表性采样 load average：开始 [5.9619140625, 5.9072265625, 5.58984375]，结束 [6.18994140625, 5.96533203125, 5.638671875]。下面列出最终样本 max/min > 2 的项目，保留全部慢样本；中位数不是显著性检验。

| 套件 | 实现 | 项目 | min µs | median µs | max µs |
| --- | --- | --- | ---: | ---: | ---: |
| 代表性 | Go | game_small/sproto_pdecode | 2.768 | 2.820 | 20.313 |
| Echo | Go | zeros4k/protobuf_frame/decode | 1.609 | 1.884 | 5.092 |
| Echo | TS | ascii32/protobuf_payload/decode | 0.310 | 0.317 | 0.804 |
| Echo | TS | ascii32/sproto_payload/decode | 0.103 | 0.104 | 0.410 |
| Echo | TS | utf8_1k/protobuf_frame/encode | 2.942 | 3.092 | 7.315 |
| Echo | TS | ascii4k/sproto_payload/encode | 1.235 | 1.322 | 2.727 |

Echo 全量采样的 Go 零字节帧编码中位数为 10.121 µs，相对旧基线 7.333 µs 高约 38%；同一时段 PB 解码也出现明显波动。随后保持源码不变，用正式版和优化版二进制交错、反转顺序各测五轮，零字节帧为 7.247 → 7.257 µs（约 +0.1%），小帧为 0.367 → 0.329 µs（约 -10.3%）。该复核不支持持续 38% 回退，不能因此删除原始全量样本或替换其统计值。复核的 [正式版原始值](echo-check/before.txt)、[优化版原始值](echo-check/after.txt)、[顺序与环境](echo-check/environment.json) 另存；额外 16 B 分配在复核中仍存在。

原始五轮、完整分配和逐步骤结果：[代表性消息](representative/report.md)、[Echo payload/frame](echo/report.md)。源码和输入哈希见 [inputs.json](inputs.json)，两套 environment.json 与 profile 环境中的编解码源码指纹一致。夹具、wire 字节数与所有固定签名均未变；GSR 不保存独立 Go 库源码补丁。

复现本轮 Go 结果需独立库的同一优化源码；只运行当前 GSR 正式依赖会使用已发布 v0.4.0。先准备临时 modfile，再顺序执行：

```sh
cp go.mod /tmp/gsr-next-codec.mod
cp go.sum /tmp/gsr-next-codec.sum
go mod edit -modfile=/tmp/gsr-next-codec.mod \
  -replace=github.com/cnooljaw/gosproto=/absolute/path/to/optimized/gosproto
GOFLAGS=-modfile=/tmp/gsr-next-codec.mod \
  npm --prefix examples/websocket-gate run bench:representative -- /absolute/output/representative
GOFLAGS=-modfile=/tmp/gsr-next-codec.mod \
  npm --prefix examples/websocket-gate run bench:protocol -- /absolute/output/echo
GOFLAGS=-modfile=/tmp/gsr-next-codec.mod GSR_PROFILE_SCENARIO=game_large \
  npm --prefix examples/websocket-gate run profile:protocol -- /absolute/output/profile
```

未测浏览器、BigInt、完整 int64 或 double 的性能专项、map、Lua/C 速度排名、并发/网络吞吐。本轮 TS 仍为 Node Buffer 参考实现；本机微基准不能作为协议固有速度或部署容量承诺。
