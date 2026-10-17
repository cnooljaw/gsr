# 原生整数 Decode 的受控写入

2026-10-17。上轮回归/提案已本地提交为 gosproto `5be3b26`、GSR `2872d46`，未推送。本轮按用户“提交，继续”落地提案的最小范围；下面是封装后重新采样的数据，不复用内嵌原型结果。新实现、GC 回归和本文仍留在工作树供审查，未提交/发布；正式 GSR 依赖仍为 v0.4.0。

后续状态：本切片已本地提交为 gosproto `fc3246a`、GSR `862fa8b`，未推送/发布。后续入口及固定恢复候选均未采用，见 [筛选记录](../2026-10-17-gosproto-decode-binding/RESULTS.md)；下文保留本切片原始计时和验证状态。

## 实现范围

- 通用 parser 保留；新增私有 decodePointer 封装地址取得和类型化写入。仅原生 `*int64` 标量（inline / 合法 4/8 字节），不推广至命名类型、字符串、结构体或切片。
- 每个结构体入口确认实际类型与私有 schema.typ 一致，且原类型确有原生整数指针。每次写入仍验证字段的原生类型、owner、整数/非数组形状；不满足时沿用反射字段索引。
- scalarBits、完整 header/chunk 校验和预算不变。每字段仍 new(int64)，临时根完整成功后才发布，不复用调用方旧值，不借用输入缓冲。
- 使用 Go 类型化 `**int64` 赋值，保留编译器写屏障；不读 reflect.Value 私有布局，不保留跨调用 uintptr。共享的 encOwner/encOffset 私有名称不做无关重命名。
- Encode、Pack、Unpack、RPC、meta、生成器、TS 与夹具冻结。没有第二份 parser、池、arena 或公共 API 变化。

## 五轮同进程对照

Apple M2、Go 1.24.13 darwin/arm64、GOMAXPROCS=1、PB v1.36.1。基线 `5be3b26` 的生产代码与 `70d985c` 相同。每项固定 60000 次，五轮交替反转先后；五组代表性消息、四档 Echo、35 种 Unpack 控制，共 129 项、645 样本。[store.json](store.json) 保存全部样本、完整 Go 源码指纹及二进制指纹；[raw](raw/) 保存逐轮输出。

µs/op，中位数；PB 列为同场纯 Decode，PDecode 包含 SP 解包：

| 消息 | Decode 前 → 后 | 变化 | PDecode 前 → 后 | 变化 | PB Decode |
| --- | ---: | ---: | ---: | ---: | ---: |
| AddressBook | 0.9943 → 1.0150 | +2.1% | 1.1900 → 1.2180 | +2.4% | 0.8356 |
| 小型游戏 | 1.7460 → 1.6110 | -7.7% | 2.2570 → 2.1300 | -5.6% | 1.6570 |
| 大型游戏 | 17.4840 → 16.1890 | -7.4% | 22.9040 → 21.5480 | -5.9% | 17.7770 |
| 整数数组 | 2.2540 → 2.2670 | +0.6% | 7.7050 → 7.7010 | -0.1% | 4.1810 |
| 4 KiB ASCII | 0.6226 → 0.6372 | +2.3% | 1.1800 → 1.1550 | -2.1% | 0.5970 |

全部 SP 项的 bytes/op、allocs/op 与基线一致。大型 Decode 为 10416 B / 532 次，PDecode 为 13872 B / 533 次。大型纯 Decode 比同场 PB 约低 8.9%，但 PDecode 仍高约 21.2%；小型纯 Decode 比 PB 约低 2.8%，PDecode 仍高约 28.5%。整数数组不走本轮标量路径，基本持平。不能把这些结果称为完整路径全面胜出 PB。

封装后大型 Decode/PDecode 为 -7.4%/-5.9%，低于上轮内嵌原型的 -11.2%/-9.2%。两轮独立测量不作直接减法；本轮不为恢复原型数字删掉布局检查或拆散封装。编译器确认每字段 setInteger 成本 39、会内联；每结构体的 messageDecodePointer 成本 216，未内联，保留为下一步可单独测量的入口成本。[barrier.json](barrier.json) 同时保存实际 store 的 gcWriteBarrier2 汇编片段。

## 代价与未改控制

AddressBook Decode/PDecode 约 +2.1%/+2.4%；4 KiB ASCII 纯 Decode 约 +2.3%，PDecode 约 -2.1%。Echo payload/frame 各项在约 ±1.6% 内。字符串不使用本轮整数写入，仍保留完整测量值。

主矩阵未改的 zeroWords15 Unpack 为 54.76 → 63.83 ns（+16.6%），nonzero1x256 为 1267 → 1094 ns（-13.7%）；大型纯 Unpack 4943 → 5259 ns（+6.4%）。这些都不称为本轮 Unpack 算法收益或回退原因。

另建两侧生产源码都为 `5be3b26` 的 A/A 二进制，对两个 >10% 控制各重测五轮，[aa.json](aa.json) 与 raw/aa-*.txt 独立保存：zeroWords15 为 50.71 → 52.02 ns（+2.6%），nonzero1x256 为 1222 → 1127 ns（-7.8%）。A/A 仍存在同源码/不同模块路径差异，但未复现主矩阵极小零段的 +16.6%；无法完全解释该主矩阵差异，不用 A/A 替换原值，也不宣称全部无回退。

## 验证与增量覆盖

新增正式库 TestNativePointerDecodePaddingAndGC：非零 offset、不同 padding、稀疏 tag、48 次解码与反复 GC，检查 inline/8 字节整数及跨记录、跨调用独占。它在安全基线先通过，随后验证正式候选；原有跨布局 Fields 替换、精确预算、命名类型和失败原子性用例保留。

[validation.json](validation.json) 保存命令、环境、输出与耗时：

- 完整库 race、严格 checkptr、vet；设置 SPROTO_ORACLE_DIR 启用固定官方 C/Lua 对照，源码指纹核对通过。
- 131068 组基线差分（全部合法 inline、低预算、迟到坏帧、packed），外部 padding/GC 与异布局字段回落；稳定错误、消费长度、值和失败时的原指针一致。
- FuzzDecode / FuzzCodecRoundTrip 各约十秒，分别 11304 / 554691 次通过。Decode 后段报告 0 新执行，但正常退出 PASS，未延长超时或重试。
- GSR 临时 modfile 指向真实库工作树，三个受影响包及 TS 构建/GoTS Plain/MAC 7/7、零跳过通过；正式 go.mod 没有变化。
- Linux amd64/arm64/386 生产代码构建通过，只有当前 darwin/arm64 的运行和写屏障证据，没有跨架构执行结论。

再从可复现候选副本仅移除 Decode 的 owner 判断、保留其余 guard，TestDecodeIntegerRecordFieldsReplacement 按预期失败，见 [negative-owner.json](negative-owner.json)。正式工作树未受此负面对照修改。没有重复 GSR 全仓/race、TS 性能或 CPU/alloc profile。

## 复现

需要包含 `5be3b26` 的独立库，以及当前 GSR 的固定 harness/夹具：

```sh
python3 docs/benchmarks/2026-10-17-gosproto-decode-store/reproduce-pairs.py \
  --library /path/to/gosproto --output /tmp/gsr-decode-store-reproduce
```

脚本从已提交基线归档，在副本应用 tests.patch / store.patch，再核对完整 Go、七份 harness 与三份夹具指纹。`--prepare-only` 已实测重建成功；脚本不修改传入库。默认重跑五轮 129 项，不保证复现相同 ns/op。verify-probe.py 可对同一 prepare-only 副本重跑外部差分，复用历史目录中固定的 inline/GC 探针；本轮实测的是工作树外部差分，重建副本仅做完整指纹核对，未重复执行相同测试。

[inputs.json](inputs.json) 固定来源、原始 modfile、未变 TS 与官方 oracle 指纹；[patches](patches/) 固定本轮代码和回归。前轮失败 parser/内嵌原型保留在 [历史实验](../2026-10-17-gosproto-decode-writes/RESULTS.md)，不混入本轮中位数。
