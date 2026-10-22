# 原生整数数组优化：正常入口与短数组复核

保留已提交的 `9e47281`，本轮不新增生产代码。移除上一轮同模块实验选择器后，正常公开入口仍复现896项数组的Decode/PDecode收益。短数组首次出现的三项回退在延长计时后均未复现；不将其解释为全部短消息稳定提速。

## 方法与证据

基线完整Git归档 `9048cfa`，优化归档 `9e47281`。两个库保持原包名和正常调用路径，分别构建独立benchmark二进制，没有分发选择器、克隆decoder或运行时模式切换。复用既有真实fixture、wire摘要和语义校验；短数组覆盖1/3/4/5/8/33项、4/8字节有符号整数，分别测Decode/PDecode。结果对象写入相同全局sink。

每模式三轮ABBA/BAAB，每进程Go benchmark目标100ms：12个self进程均运行同一个A二进制，只保留位置标签；12个compare进程按标签运行A/B。共24进程、672行测量、35条构建/验证/测量命令通过。三轮比值使用两次B与两次A耗时的几何配对比，中位和每轮完整公布；self不相减。Go自行校准每个子项调用次数，不采用上一轮固定同进程调用数，也不跨两种harness合并结果。

[全部28项数据](ALL-CASES.md)、[原始逐轮分析](analysis.json)、[执行命令](commands.json)、[提交/源码/fixture/二进制SHA](plan.json)。计时前后源码与二进制SHA一致，两个归档fixture测试和harness vet通过。该轮只加benchmark/报告，不重复上一轮已完成的全库oracle/race/fuzz验收。

## 正常入口结果

负值表示优化版本耗时降低。

| 入口 | 三轮配对变化 | 中位 | self中位 |
| --- | --- | ---: | ---: |
| 896整数数组 Decode | −25.05% / −21.68% / −20.58% | −21.68% | −0.47% |
| 896整数数组 PDecode | −2.27% / −4.86% / −5.10% | −4.86% | +0.37% |
| 大型游戏 Decode | −0.62% / +0.62% / +1.05% | +0.62% | +8.34% |
| 大型游戏 PDecode | −1.31% / +0.16% / −0.99% | −0.99% | −1.77% |

大型游戏消息变化接近控制波动，不能把上一轮同进程的小收益直接当作正常入口稳定收益。33项数组Decode的4/8字节两种宽度三轮均改善，中位−4.28%/−4.42%，局部证据保留，不推广到所有短消息。

全部28项A/B的B/op与allocs/op一致。整数数组仍8288 B/4 alloc、PDecode12384 B/5 alloc；大型游戏仍10416 B/532 alloc、PDecode13872 B/533 alloc。

## 短数组异常与预定复测

首次100ms扫描中，4字节1项Decode中位+19.28%，4字节5项+2.11%，8字节8项+21.76%。针对这三项，不改变消息或二进制，将目标延长至400ms；仍各三轮self/compare ABBA/BAAB，共24进程72行测量。首次数据全部保留。

| 入口 | 复测三轮 | 复测中位 | self中位 |
| --- | --- | ---: | ---: |
| 4字节1项 Decode | +0.38% / −0.51% / +0.10% | +0.10% | +0.24% |
| 4字节5项 Decode | −5.07% / −13.06% / −7.89% | −7.89% | −18.49% |
| 8字节8项 Decode | 约−0.00% / −3.72% / −1.40% | −1.40% | −1.85% |

[完整复测](RECHECK.md)及[命令](recheck-commands.json)。1项4字节接近持平，8项8字节没有持续回退，5项4字节改善但self波动很大，不据此宣称稳定提速。所有进程成功退出，没有删除轮次或减去控制；不同次序、代码摆放、GC与系统噪声均未被本实验分离归因，不添加硬件原因推测。

## 结论与后续

正常入口确认了长整数数组优化方向，保留实现。短数组尚未确认持续回退；这里不是统计显著性证明，尤其不能因一次复测变快就宣称全面胜出。未测新的Protobuf数据，不更新PB正式排名。

下一步回到数组组合路径的Unpack实际热点：目前Decode已经明确改善，PDecode收益较小，优先检查正常入口profile中的重复扫描/复制和输出分配。保留资源上限、失败原子性、独占结果与官方wire，不直接叠加更大展开或修改小数组阈值。

## 复现

```sh
python3 docs/benchmarks/2026-10-22-gosproto-array-normal-review/run.py tmp/array-normal-new
python3 docs/benchmarks/2026-10-22-gosproto-array-normal-review/analyze.py
python3 docs/benchmarks/2026-10-22-gosproto-array-normal-review/recheck.py tmp/array-normal-new
python3 docs/benchmarks/2026-10-22-gosproto-array-normal-review/analyze-recheck.py
```

独立库十项用户删除未触碰；所有构建使用Git归档。GSR正式gosproto依赖保持v0.4.0。本轮新报告和benchmark留审查，未提交。
