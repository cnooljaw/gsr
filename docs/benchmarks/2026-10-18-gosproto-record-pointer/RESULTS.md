# 整数记录指针入口优化

2026-10-18。上一轮测试增强已本地提交为 gosproto `bd1976a`，实验记录为 GSR `4bf1b32`，未推送。本轮基于 bd1976a；其生产代码与 0c1f6cf 相同。保留本轮三个生产文件的局部修改及 CHANGES、GSR 记录，均未提交。正式 GSR 依赖仍 v0.4.0，Decode 冻结。

## 改动与边界

原先两个整数记录 helper 接收三字的 reflect.Value，内部做 IsNil、重建 binding 并提取指针。现在由既有 integers 数组分支用现有 messageEncodePointer(value.Index(i), childBinding) 提取指针，helper 只接收一字 encodePointer，直接判 pointer.value == nil。只修改 encode.go、limits.go 两个调用点与 encode_integer_record.go，9 行增加、12 行删除；字段循环保持原样，未恢复上轮撤回的 Fields 捕获。

integers 仍要求实际 Go 类型匹配、nativeInt64Pointer、int64Record 和当前全部字段 owner/形状验证。命名类型、描述替换或不匹配布局继续走通用路径；nil 指针的 wire、布局项与预算次序不变。入口仅提前提取借用指针，不读取字段、增加预算或分配输出。两个阶段各自取得当前指针，没有跨阶段缓存；既有 root KeepAlive 保留。没有新增 unsafe 原语、uintptr 转换、缓存、公共 API、依赖或消息生命周期，Pack/Unpack、Decode、RPC 代码不改。

ARM64 普通构建确认：UnsafePointer 调用移至两个数组调用点，非 nil 记录仍各阶段调用一次；helper 中反射 IsNil 的通用 kind 分支消失。预检栈帧 144→128 B，写出 64→32 B，helper 汇编行数 325→253 / 281→217；调用点汇编一并保留，不将局部少指令当作整体收益。全 nil 记录原先跳过 UnsafePointer，新版会调用，这是明确待量化代价。

## 既有契约与验证

没有新增重复用例。先在基线、再在候选运行已有六项相关测试，覆盖乱序 Go 声明/忽略字段、native offset、nil/空记录、稀疏 tag、精确字节/深度/元素预算、当前值与旧输出独占、公开元信息替换。完整验收进一步覆盖 GC 保活、原生指针与命名类型回落；此轮没有新造负向夹具。

库全测与官方 C/Lua 对照、race+checkptr=2、vet 均通过。官方归档 e93fd5a 的 C/Lua/共享库指纹重核一致，显式设置 SPROTO_ORACLE_DIR，不把跳过当通过。10 秒 FuzzCodecRoundTrip 完成 441809 次。GSR 临时 modfile 接入的 protocol/sproto、debugserver、benchmarks 三包通过，Go/TS 7/7、零跳过，含 Plain/MAC 真实连接。原命令/输出见 acceptance.json、integration.json；正式 go.mod 未改。

未重复 GSR 全仓/race、Pack 专项 fuzz、byte-body RPC 差分、跨平台或 PB/TS 全矩阵。新增 pointer 入口没有改变 Pack 和 Decode；本轮是增量实验，不更新正式 PB/TS 排名。

## 固定主矩阵

原 19 body/手工帧 +19 实际 RPC +5 wide64，共 43 项，原探针、夹具不变。self、clone、两轮 compare 固定运行一次；六轮，每轮四个 ABBA/BAAB 四元组、每组四块，目标块 10 ms。Go1.24.13 darwin/arm64 / Apple M2 / GOMAXPROCS=1 / GC 开启。16512 块、4128 四元组、1032 配对轮次逐块复算到 1e-12 一致，不删异常样本、不重试。

表内百分比为六轮配对比值中位数；ns 为各轮两臂几何均值差的中位数。最后一列单独列出所有计时块累计 ns/call 的 B/A−1，含离群，不是前面中位差的另一种表示。负值代表候选耗时减少；这些统计量不是置信区间。

### 10 项目标

| 项目 | 第一轮 | 第二轮 | 配对差 ns（两轮） | 累计每次耗时比（两轮） |
| --- | ---: | ---: | ---: | ---: |
| body/game_small/sproto_encode | -6.12% | -6.41% | -45.4 / -47.8 | -23.28% / -8.30% |
| body/game_small/sproto_pencode | -2.89% | -5.27% | -31.5 / -50.6 | +3.15% / -1.95% |
| body/game_large/sproto_encode | -8.28% | -7.85% | -531.7 / -505.5 | -0.79% / -7.73% |
| body/game_large/sproto_pencode | -7.31% | -5.65% | -634.7 / -485.2 | -11.99% / -5.71% |
| rpc/game_small/notification | -4.62% | -4.96% | -46.3 / -49.6 | -0.85% / -5.13% |
| rpc/game_small/request_cancel | -4.16% | -3.58% | -43.8 / -38.0 | -4.16% / +2.11% |
| rpc/game_small/response | -4.64% | -5.03% | -46.9 / -50.3 | +0.95% / -5.07% |
| rpc/game_large/notification | -6.58% | -5.63% | -573.7 / -484.5 | -6.50% / -7.67% |
| rpc/game_large/request_cancel | -5.22% | -7.06% | -453.4 / -615.1 | +20.93% / -7.01% |
| rpc/game_large/response | -7.22% | -6.85% | -625.7 / -607.4 | +2.03% / -7.01% |

大型 Encode 稳定减少约 506～532 ns（7.85%～8.28%），PEncode 减少约 485～635 ns（5.65%～7.31%），三种 RPC 编码配对中位数均改善。小型消息对应减少约 32～51 ns。目标对象数与 B/op 中位数全部相同。结合减少的参数/反射分支与两轮业务路径方向，保留；没有设置最低百分比门槛。

但累计指标并非全部改善：第一轮大型 request_cancel 累计 +20.93%，小型 PEncode +3.15%、小型 response +0.95%、大型 response +2.03%；第二轮小型 request_cancel +2.11%。第一轮大型 request_cancel 的一块 B 达 227.172 ms、同块完成两次 GC（目标约10 ms）。这只能说明该块包含 GC，不能证明 GC 是全部原因。long-blocks.json 披露主比较所有 >100 ms 块，全部仍参与原值、累计和配对计算；其余所有块/GC 也保留。本轮不承诺改善尾延迟或所有累计吞吐。

### 其他 33 项与控制

| 项目 | 第一轮 | 第二轮 | 配对差 ns（两轮） | 累计每次耗时比（两轮） |
| --- | ---: | ---: | ---: | ---: |
| body/addressbook/sproto_encode | +0.56% | +0.74% | +3.0 / +4.0 | +0.15% / +0.59% |
| body/addressbook/sproto_pack | +1.50% | +0.99% | +1.8 / +1.2 | -2.27% / +3.71% |
| body/addressbook/sproto_pencode | -0.71% | -0.38% | -4.6 / -2.4 | -0.49% / -0.31% |
| body/game_small/sproto_pack | +0.52% | -0.38% | +1.6 / -1.1 | +1.80% / -0.20% |
| body/game_large/sproto_pack | -0.12% | -0.24% | -3.3 / -6.4 | -0.21% / -3.49% |
| body/int32_edges/sproto_encode | -0.33% | +0.84% | -5.8 / +14.9 | +0.25% / +2.82% |
| body/int32_edges/sproto_pack | +0.39% | -0.12% | +9.8 / -3.1 | +6.88% / +0.95% |
| body/int32_edges/sproto_pencode | +1.07% | -1.43% | +39.8 / -54.9 | +1.35% / -4.92% |
| body/echo_ascii4k/sproto_encode | +2.49% | +0.68% | +19.9 / +5.6 | +10.27% / +0.09% |
| body/echo_ascii4k/sproto_pack | +0.13% | +1.42% | +1.1 / +10.5 | -0.28% / +17.24% |
| body/echo_ascii4k/sproto_pencode | -0.65% | +0.36% | -6.0 / +3.2 | -4.89% / +0.98% |
| body/ascii32/frame | -0.43% | -0.30% | -1.3 / -1.3 | -0.33% / +0.34% |
| body/utf8_1k/frame | -2.22% | -0.37% | -18.0 / -3.0 | -8.74% / -0.11% |
| body/ascii4k/frame | -1.57% | +1.07% | -40.2 / +26.6 | -0.61% / +10.58% |
| body/zeros4k/frame | +1.21% | -0.98% | +45.3 / -36.0 | +14.26% / -0.64% |
| rpc/addressbook/notification | -0.59% | -0.57% | -4.0 / -3.9 | -0.83% / -8.72% |
| rpc/addressbook/request_cancel | -0.19% | -1.71% | -1.4 / -12.5 | +6.16% / -5.95% |
| rpc/addressbook/response | -0.54% | -0.17% | -3.6 / -1.1 | -0.67% / +0.48% |
| rpc/int32_edges/notification | +0.19% | -1.64% | +7.5 / -64.1 | -0.14% / -1.53% |
| rpc/int32_edges/request_cancel | +0.44% | +4.74% | +17.2 / +181.4 | -2.84% / +23.30% |
| rpc/int32_edges/response | +0.76% | -1.35% | +29.3 / -53.0 | +1.42% / -1.35% |
| rpc/echo_ascii4k/notification | +2.35% | +0.95% | +26.9 / +9.8 | +7.27% / -8.86% |
| rpc/echo_ascii4k/request_cancel | +1.23% | -0.93% | +14.7 / -10.5 | +0.81% / -0.92% |
| rpc/echo_ascii4k/response | +0.13% | -0.19% | +1.6 / -2.0 | +9.57% / -5.79% |
| rpc/ascii32/notification | -0.11% | -0.19% | -0.2 / -0.4 | +0.44% / +0.01% |
| rpc/utf8_1k/notification | -1.02% | -0.22% | -3.9 / -0.8 | -6.45% / -5.65% |
| rpc/ascii4k/notification | -0.69% | -0.18% | -7.3 / -1.9 | -0.39% / -0.40% |
| rpc/zeros4k/notification | -0.49% | -1.04% | -10.8 / -25.4 | -1.68% / -0.71% |
| body/wide64/sproto_encode | +0.10% | -0.01% | +2.1 / -0.3 | +0.07% / -0.07% |
| body/wide64/sproto_pencode | +0.50% | -0.57% | +34.9 / -39.7 | +8.22% / -10.82% |
| rpc/wide64/notification | +1.68% | -0.72% | +122.3 / -53.5 | +1.33% / -0.88% |
| rpc/wide64/request_cancel | +0.35% | +0.06% | +26.2 / +8.4 | +3.11% / +12.16% |
| rpc/wide64/response | -0.45% | -0.40% | -34.1 / -33.5 | -0.55% / +1.36% |

非目标并非全胜。原生数组不经过这个记录 helper，第二轮 int32 request_cancel 配对 +4.74%（+181.4 ns），必须保留为未解释观察；Echo 4 KiB 纯 Encode 第一轮 +2.49%、第二轮 +0.68%，RPC 通知 +2.35%/+0.95%。代码未改不能自动断言差值全部是噪声。32 B 手工帧和通知本轮没有重复性回退，不借此覆盖旧切片。

主 self 35/43、clone 34/43 通过原控制标记。所有失败项、IQR、逐轮比值、GC 和分配窗口保留在 joined.json；不是只保留通过者。控制失败是归因限制，不是小收益的一票否决，也不相减“校正”候选比值。

43 项对象数中位数全部相同；第二轮 ASCII4KiB 手工帧 14616.125→14616 B/op，零字节帧 14616→14616.125 B/op，对象数均为5；其余 B/op 相同。完整保留 ±0.125 B/op，不截断以求控制通过，本轮不强行确定这两个尾差的机制。

上一轮 6d854d1→连续数组游标的未改 int32 Pack +9.45%（245 ns）仍未解释。本轮比较是 bd1976a→指针入口候选，当前 Pack −0.12% 等控制值不能证明旧观察已解决，旧原值/符号指令核对保留于[数组游标报告](../2026-10-18-gosproto-array-cursor/RESULTS.md)。

## 事先提出的 nil 补充矩阵

代码审查在读取候选计时结果前保存 nil-plan.json：全 nil 记录现在会提取指针，必须量化代价。主矩阵结束后才创建补充 Go 探针，并运行独立 self/clone/两轮 compare；不修改原43项探针或生产候选。四个原生 *int64 字段、忽略 padding；全 nil 长度1/32/256、混合 nil/空/填充记录256，各 Encode/PEncode，共8项。两臂事先比较完整 wire/cap，保留同一采样规则。3072 块、768 四元组、192 配对轮次独立复算一致。

| 项目 | 第一轮 | 第二轮 | 配对差 ns（两轮） | 累计每次耗时比（两轮） |
| --- | ---: | ---: | ---: | ---: |
| nil1/encode | +0.94% | +0.10% | +1.0 / +0.1 | -6.79% / +1.74% |
| nil1/pencode | -0.32% | +1.58% | -0.4 / +2.1 | -2.81% / -11.06% |
| nil32/encode | +3.63% | +3.99% | +24.4 / +26.2 | +3.68% / +4.61% |
| nil32/pencode | +3.54% | +4.05% | +26.5 / +30.6 | +8.55% / +10.16% |
| nil256/encode | +5.11% | +4.65% | +217.0 / +195.4 | +5.42% / +4.92% |
| nil256/pencode | +4.25% | +3.95% | +206.8 / +193.4 | +11.47% / +8.15% |
| mixed256/encode | -4.56% | -4.93% | -436.8 / -476.2 | -4.76% / -5.29% |
| mixed256/pencode | -2.01% | -3.21% | -248.3 / -396.4 | -2.62% / -0.73% |

全 nil 长数组确有回退：32个记录约 +24～31 ns，256个约 +193～217 ns（约4%～5%）；单元素约0～2 ns、方向混合。混合256项 Encode 约快437～476 ns、PEncode快248～396 ns。八项对象数与B/op中位数均相同；self8/8、clone6/8，clone的nil1/pencode、nil32/pencode未通过，原值完整保留。

保留的取舍是正常游戏记录及混合数组获益，而全 nil 长数组付出可量化入口代价，并非所有输入都更快。下一轮可单独实验已验证数组入口的 nil 分支位置；不能预先承诺它在非 nil 热路径没有成本。

## 复现与归档

主矩阵/补充矩阵各保存固定计划、补丁、源码/工具/夹具指纹、编译二进制指纹、全部命令和原始样本。另以 prepare-only 各重建四份归档，核对源码、工具、夹具、补丁、探针和参数，主库最终 Go 源码与编译/两套测量 candidate 一致；没有在采样期间修改 Go 源码。

冻结 plan.json 的 assessment 沿用了上轮“2-line production delta”和“baseline0c1f6cf”字样：实际基线为 bd1976a（生产内容确与0c相同），实际补丁为3文件9增12删。以 plan 顶层 baselineCommit、candidate.patch 和指纹为准；这属于计划说明勘误，没有事后改写冻结文件或采样参数。

```sh
python3 -B docs/benchmarks/2026-10-18-gosproto-record-pointer/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/record-pointer-new
python3 -B docs/benchmarks/2026-10-18-gosproto-record-pointer/analyze.py tmp/record-pointer-new
python3 -B docs/benchmarks/2026-10-18-gosproto-record-pointer/nil-reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/record-pointer-nil-new
python3 -B docs/benchmarks/2026-10-18-gosproto-record-pointer/nil-analyze.py tmp/record-pointer-nil-new
```

输出目录须不存在；固定自检与采样不应并行运行重型负载。二进制/整份源码归档位于忽略的tmp/，不入Git；复现输入、普通构建汇编与所有采样原值保留在本目录。最终本轮修改待审查、未提交；没有推送、发布或正式依赖升级。
