# Decode 测量：基线多批校准验证

基线为 gosproto `9048cfa`，候选仍是前轮 header 游标。本轮只验证采样基础设施：固定一种baseline/clone/candidate摆放，只测大型Decode/PDecode，不借方法变化宣称新的跨布局性能排名。生产库、GSR正式依赖和七项无关删除不变。

## 预先规定的规则

双侧预热后，仅选基线进行七批、每批256次调用，记录全部耗时和GC/分配。取七批中位估算目标60ms所需调用数，限制256～65536次。随后用三批pilot验证：中位至少40ms即停止，否则按60ms目标调整调用数；最多三个pilot阶段。达到次数上限或最后阶段也停止，未达到40ms必须报告，不能无限重试。

校准不访问候选耗时，不选择最快批次，也不删除任何批次。pilot和正式块都保留完整原始数据。正式比较使用固定等调用数，六轮×四组ABBA/BAAB，不根据正式块重新校准或补跑，不剔除GC和长尾。

这是校准规则验证，40ms是pilot目标下限，不是保证正式所有块至少40ms的断言。后续运行状态变化仍可能改变单块耗时，实际覆盖由check-calibration.py逐块统计。

## 验证方法

原始JSON增加Calibration数组。check-calibration.py独立重算七批中位、每次pilot调整/停止与最终调用数，验证正式两侧确实使用相同调用数；全部校准批次保留。常规analyze.py继续独立重算每个四元组、六轮中位/累计、GC/分配，控制不相减。

同模块同二进制三个parser仍经过既有合法/未知字段/截断/错误顺序/预算状态/结果差分、race/checkptr、真实fixture/入口预检、vet与构建。candidate逆变换还原基线。源码、fixture、生成文件、二进制哈希复核，没有新增重复行为夹具。

```sh
python3 docs/benchmarks/2026-10-21-gosproto-decode-multibatch-calibration/run.py --library /private/tmp/gsr-gosproto-fork --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-decode-multibatch-calibration/analyze.py --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-decode-multibatch-calibration/check-calibration.py
```

输出目录须不存在，重测前另存本报告目录原始样本。没有改动正式Benchmark默认工具；此规则只存在本次隔离实验。

## 校准结果

8个进程共80个校准批次（56个seed、24个pilot），所有决策与最终调用数独立复算一致。此次所有进程均在第一个pilot阶段达到40ms，正式块调用数3071～4864次。768个正式块全部超过40ms：最短46.182ms、中位59.130ms、最长132.000ms，低于20ms/40ms的数量均为0。

这些数据支持在本次大型Decode/PDecode环境里，多批规则避免了前轮单批校准生成约4ms中位块的问题；不是未来所有负载的时长保证。后续两个pilot调整分支和次数上限在本次真实运行中没有触发，不能声称已被各种运行状态验证。PDecode/pass2的seed前两批约12ms，后五批约4～5ms，所有批次原样保留；中位规则未选择最快批次，最终正式块依然超过40ms。

768个块保留全部GC/暂停/分配；没有因超过目标、较慢或较快而重测任何块。8进程、48轮、192四元组、20条记录命令全部通过。

## 候选观察与决定

| 入口 | 第一遍中位/累计 | 第二遍中位/累计 |
|---|---:|---:|
| 大型Decode | −0.30% / +1.55% | −0.36% / +0.76% |
| 大型PDecode | +1.88% / +2.19% | −0.63% / −0.58% |

负数为耗时减少。候选保留，但本次不进入生产：Decode中位有小幅改善而累计两次反向，PDecode方向也未重复。不因0.3%的幅度本身舍弃；未确认的是稳定的整体效果。

更长块仍存在控制波动：Decode self中位+1.29%、IQR4.45pp，clone−1.25%、IQR5.92pp；PDecode self−3.48%、IQR9.19pp，clone+0.36%、IQR0.97pp。控制不相减。新harness布局与前轮不同，不能把两份百分比直接相减，归因成校准修复带来的协议加速；校准解决的是短块风险，不保证消除所有噪声。没有系统trace就不指定调度、GC或硬件根因。

本轮只验证方法并观察原候选，不新增生产源码，不修改正式Benchmark默认工具，不重复全库、官方oracle、fuzz或Go/TS生产验收。旧样本及失败/反向结论继续保留。后续实验可复用多批规则并报告实际块覆盖，不继续无边界扩大同一个游标候选的测量。
