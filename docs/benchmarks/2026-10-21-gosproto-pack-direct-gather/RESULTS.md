# Pack 位图直接收集

基线：gosproto `451998ea0dcf6262bdd4ca0ab6c686d0fb738fac`，完整 Git 归档、隔离模块，生产工作树不受实验生成代码影响。

候选将 header 的计算从：

```go
uint8(((nonzero >> 7) * 0x0102040810204080) >> 56)
```

改成：

```go
uint8((nonzero * 0x0002040810204081) >> 56)
```

nonzero 的每个字节只有最高位可能置位。直接乘法把第 i 个字节的位收集到结果第 i 位，uint64 的溢出按 Go 无符号模运算处理；全部 256 种位图已验证等价。完整 packInto 测试继续验证实际不同字节值与边界，不能仅用整数恒等式替代 wire 检查。

这减少了生成 header 表达式中的依赖步骤，但不等于机器指令总数减少：本 arm64 二进制基线/clone/候选均为 296 条指令。普通分支的 ranks 仍需要原有右移，不能声称删除了整条路径上的所有移位。FF 判断、稀疏分支、八次 payload 写出和尾块算法不变。

## 方法

沿用上一轮同模块 ABBA/BAAB sampler：game_small/game_large/int32_edges × Pack/PEncode，加固定四/七非零字节 Pack，共八项。self、clone、pass1、pass2 四模式；每进程六轮、每轮四组四块。两侧各预热 50ms，基线 256 次校准目标 20ms；GOMAXPROCS=1、GOGC=100、GOMEMLIMIT=off。全部 GC、分配和长尾保留，控制不扣减，没有摆放轮换。

candidate 的逆变换必须还原 clone；源码、fixture 和二进制记录哈希，测量前后验证源码和二进制未变。真实 fixture 在预检及每个实验进程校验。公开 Pack 包含一次独占缓冲分配；PEncode 共用正常 encodeBufferWithLimits，每个模式静态调用自己的 Pack，没有在 encodeLayout 路径添加选择器。

## 正确性与复现

3584 组（256 mask × 14 长度）验证候选 wire、输入快照、共享输出后端源区域写入。现有 TestPack/TestUnpack 与新增 TestGatherEquivalence 一同运行 race/checkptr；harness fixture、vet 与构建也纳入 commands.json。

从 GSR 根目录运行：

```sh
python3 docs/benchmarks/2026-10-21-gosproto-pack-direct-gather/run.py tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-pack-direct-gather/analyze.py
```

脚本从 `/private/tmp/gsr-gosproto-fork` 的 HEAD 导出，请先确认其等于上述基线；重测将覆盖本报告目录内样本，需先另存。完整结果见 ALL-CASES.md，逐块 telemetry 见各 JSON。

## 实测结论

| 场景 | 第一遍中位/累计 | 第二遍中位/累计 |
|---|---:|---:|
| 小型 Pack | −0.54% / −1.59% | −1.39% / −4.67% |
| 小型 PEncode | −0.18% / −4.92% | −0.24% / −5.75% |
| 大型 Pack | +0.25% / +4.85% | −0.51% / −6.62% |
| 大型 PEncode | −0.09% / −0.26% | +0.02% / +0.05% |
| 整数数组 Pack | −1.85% / −1.80% | −1.86% / −1.35% |
| 整数数组 PEncode | +0.14% / +0.85% | +1.00% / −2.75% |

负数为耗时减少。保留候选，等待组合路径确认，尚未修改生产实现；不以收益小于某个百分比为由丢弃。整数数组 Pack 两遍中位和累计都改善，小型 Pack/PEncode 也有一致正面方向；但单独 Pack 的收益没有同步反映到整数数组 PEncode，大型消息仅接近持平。仍需控制实验确认，不能将本轮结果宣传为全面改善。

固定七字节 Pack 两遍中位 −1.81%/−1.40%，但累计 +0.47%/+25.34%，保留全部长尾，不按“离群”删除。该项 clone 中位 +2.33%、IQR 2.00pp；整数数组 Pack clone 为 +1.77%、IQR 4.66pp。小型 self 的 IQR 达 15.23pp。这些控制提醒时间噪声和摆放偏差仍在，没有做相减校正或硬件归因。下一次应优先复核整数数组的 Pack/PEncode 组合，而非重复扫全部固定密度消息。

32 进程、192 轮、768 个四元组、3072 个计时块，41 条记录命令全部成功。正常入口分配保持：小型 Pack/PEncode 352B/1alloc，大型 Pack 3456B/1alloc、PEncode 3968B/2alloc，整数数组两者 4096B/1alloc；精确 telemetry 保留全部尾数。

本轮未运行全库、官方 oracle、fuzz 或 Go/TS 集成；所需生产验收应在候选确认并准备落地时执行。本轮没有修改公开 API、wire、资源预算、错误行为、所有权或缓冲容量。
