# 同模块、同进程 Unpack 对比与采纳

2026-10-20，基线gosproto `a93bb0d`。本轮采纳[上一轮索引范围候选](../2026-10-20-gosproto-unpack-index/RESULTS.md)：普通非零mask使用固定八字节目标视图，`TrailingZeros8(mask) & 7`表达可达索引范围。它对所有可达值等价，消除逐有效字节的目标检查；完整首遍输入/预算预检、源读取检查、零/FF路径、错误顺序和精确独占输出保持。生产只改pack.go、增加可观察边界回归并更新CHANGES；没有新增公开API、unsafe、池或共享结果。

## 决定与依据

**保留这项小改动，不设最小收益比例门槛，也不宣称所有消息全面提速。** 整数数组和高密度普通word两轮重复改善，机械原因可见于目标索引检查消除，代码增量很小；组合路径的小收益和大型消息的不确定性一起保留。语义安全由非零mask不变量、完整预检和回归验证保证。

| 场景 | pass1 中位 / 累计 | pass2 中位 / 累计 |
| --- | ---: | ---: |
| AddressBook Unpack | -1.34% / -1.61% | -1.50% / -6.32% |
| 整数数组 Unpack | -1.03% / -0.84% | -2.50% / -5.01% |
| 整数数组 PDecode | -0.96% / -2.64% | -0.26% / +2.05% |
| 7非零字节/word Unpack | -3.00% / -4.14% | -3.87% / -1.49% |
| 大型 Unpack | +1.77% / +5.80% | -4.39% / -8.57% |
| 大型 PDecode | -0.28% / -4.26% | +0.70% / -0.56% |
| 小型 Unpack | +0.05% / -0.32% | +1.31% / +11.79% |
| ASCII PDecode | +0.02% / +6.25% | +0.36% / -2.97% |

负值为候选耗时低。累计使用全部块的真实时间/调用数，不替代轮次几何比中位数。整数数组PDecode第二轮累计反向、大型/小型消息和其他密度的局部成本不隐藏；全部15项见 [ALL-CASES.md](ALL-CASES.md)，六轮、全部GC/分配与长块见原始JSON及analysis.json。60项模式/入口结果中21项中位/累计反向（含控制），不筛除、不只挑有利统计量。

本轮提供的是同一进程内算法差异的更直接证据，不把这些数字当作新版本跨机器的保证，也不推算新的Go/PB总排名。后续重要阶段仍刷新生产公开入口同场基线。

## 为什么改测量方法

上轮独立进程即使改为100ms仍存在很大的self/clone波动。Unpack是纯字节操作，不需要不同模块身份。本轮在一份完整归档中保留原始`UnpackWithLimit`，额外生成同源副本和候选函数；复制的同源函数体去除名称/空白后与原始逐一相等。实验函数只存在临时归档，不进入生产。

一个二进制只加载一份SP模块，每个case/mode用一个进程，A/B在进程内交错：self为同一函数A/A，clone为原始/同源副本，pass1/pass2为原始/候选。PDecode使用公共Decode与相同消息构造器组合，等价于现有DecodePacked的Unpack→Decode接线；两个臂都经过相同闭包/函数值调用，不能把这一实验当作直接调用DecodePacked的绝对耗时排名。

5种代表消息×Unpack/PDecode＋1/4/7非零字节密度及全零/全FF的4KiB输入，共15项。每臂预热50ms，先按基线256次校准到约20ms目标调用数；同一case两臂固定相同调用数。6轮×每轮4个ABBA/BAAB交错四元组，GOMAXPROCS1/GOGC100/GOMEMLIMIT=off。共60进程、360轮、1440四元组、5760时间块，全部块保留。每块在计时外读取MemStats，记录GC完成数、停顿、分配增量和堆快照，不在两臂之间强制GC，也不剔除含GC块。快照最大值不是堆峰值。

## 控制与剩余限制

15项self的中位差全部在±1%以内，较前一轮控制更集中。这是观测，不是新增固定收益门槛。大多数同源控制也更集中，但大型Unpack clone仍有−11.01%中位差、IQR8.23pp；7非零字节clone为+2.27%、IQR1.06pp。即使函数体相同，不同函数地址/布局、GC相位和系统负载仍可能有影响，当前证据不能将偏差唯一归为其中某一原因。控制不相减，不声称已经消除所有噪声。

候选仍有每word的视图建立成本，普通字节越少，固定成本越难摊薄；全部小包、零、FF和密度对照都保留。高密度及整数数组的重复正向与小代码增量足以支持保留，但大型Unpack、整体PDecode和ASCII的细小差异不作确定因果归因。块级ReadMemStats自身可能影响下一块执行状态；校准仅一次且基于基线，实际块长度可偏离20ms，全部实际时长保留。

分配形态保持：整数数组Unpack为4096B/1次，PDecode为12384B/5次；大型Unpack约3456B/1次，PDecode约13872B/533次。块级累计偶有极小字节/对象尾差，原始增量保留，不把近似值说成每一块完全相等。

## 正确性验收与实际工作区

准备阶段，三个实现同时通过全部mask回归与既有Pack/Unpack测试的race＋严格checkptr；代表消息先校验语义、固定wire、消费长度和padding，两个臂在计时外再次核对解包字节、输出容量和Decode结果。候选适配函数体与最终保留函数去除名称/注释/空白后相等，逐字段代码未另行优化。

随后使用**不含实验适配函数**的完整候选快照验收：

- 全库测试＋固定官方C/Lua oracle，reference文件/工具哈希核对。
- 全库race＋严格checkptr、go vet。
- FuzzPackRoundTrip限时10s，两worker，56个baseline种子；3310次执行，3秒后计数未再增长，约11秒PASS退出。不延长、不重跑，也不把它描述为充分fuzz覆盖；没有留下可供事后归因的运行中栈。
- GSR protocol/sproto、debugserver、benchmarks三包测试。
- Go/TS集成7/7通过、零跳过；真实服务器二进制build-info确认替换到了候选完整快照。

六条完整验收命令全部成功。新增永久回归覆盖256种mask×8种word数量×3种前缀零段×2种输入容量，共12288种配置，每配置4次公开解包调用：包含普通/零/FF、短长边界、恰好/不足预算、迟到截断、输入不变、精确输出容量及跨调用独占。相对原有固定wire/预算测试，新增组合覆盖本轮窗口与索引范围表达，不依赖私有实现细节。

验收后将pack.go和回归放入 `/private/tmp/gsr-gosproto-fork`；所有Go源码逐一等于验收快照，CHANGES同步记录收益范围。原库六项已有删除保持，未恢复原go.mod，正式GSR依赖仍v0.4.0。gosproto实现及永久回归已提交为 `50c69e1`，仅原六项删除保留；未推送或发布。

## 复现与证据

```sh
python3 docs/benchmarks/2026-10-20-gosproto-unpack-inprocess/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-unpack-inprocess/analyze.py --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-unpack-inprocess/accept.py \
  --experiment /absolute/new-output --output /absolute/new-acceptance-output
```

输出目录均须不存在。验收脚本从本轮实验归档复制完整源码，排除临时适配函数，再重建候选并断言函数体相同；依赖本仓库现有oracle和生成夹具。脚本准备部分整理后，通过prepare-only重建核对全部Go哈希等于实际已验收快照，没有重复运行同一份代码的全套验收。本轮原始目录为 `tmp/gosproto-unpack-inprocess-20261020`，完整验收目录为 `tmp/gosproto-unpack-inprocess-accept-20261020`；不提交二进制。保留Go缓存。

源码、二进制、生成输入及JSON夹具在采样前冻结并于结束后核对；Python独立复算全部块比例，精度1e-12。验收来源、实际库源码、集成二进制哈希分别保存。Body Encode/Decode主线保持开放；本轮不以解包局部收益替代Body结论。
