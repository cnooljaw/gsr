# Unpack 固定八字节目标视图实验

2026-10-20，基线为 gosproto `a93bb0d`。继 [Decode/Unpack 热点诊断](../2026-10-20-gosproto-decode-cost/RESULTS.md)，只改 `UnpackWithLimit` 的普通 mask 写入：首遍长度/预算校验后，每个普通 word 建立 `unpacked[offset:offset+8:offset+8]`，将目标索引从 `offset+index` 改为 `index`。wire、API、首遍验证、错误优先级、精确容量、零段、FF及独占结果保持。

## 结论

**候选暂不并入生产库，净收益未确认；不是因收益小于某个门槛而舍弃。** 大型 Unpack 两轮中位改善约3%，AddressBook亦有重复小幅改善，全部正向数据保留。但同源控制存在同阶偏差，主目标整数数组第二轮反向，组合入口及累计统计方向不稳定；后半段多项出现极端长尾。现有数据不足以把观察到的改善归因于这三行改动，也不能证明它必然没有收益。

静态目标也未达到：同一 ARM64 编译条件下，UnpackWithLimit 指令数384→392，`panicIndex` 调用点5→5，`panicSlice*` 调用点8→10；A/B/C函数入口地址相同（完整汇编及 assembly-summary.json保留）。这些是静态调用点，不代表每次执行次数。固定长度视图没有让编译器证明 `TrailingZeros8(mask)` 的结果必在0～7；仅换切片写法不足以消除目标检查。不能以“窗口更固定”推定变快。

## 单模块与控制

每个二进制只加载一份 `github.com/cnooljaw/gosproto`；A基线、B候选、C同源基线分别构建。self执行A/A；clone执行A/C；pass1、pass2执行A/B，不相减控制，也不扣除噪声来生成“修正收益”。三个模块解析、源码和二进制哈希均保存。

五种原代表消息 × Unpack/PDecode，共10项；每模式每项6轮，每轮4个独立进程，交替ABBA/BAAB。每个进程50ms预热、20ms Go自适应计时，GOMAXPROCS1/GOGC100/GOMEMLIMIT=off；共960进程、240轮、977条准备/构建/相关验证及测量命令。先运行三侧既有Pack/Unpack测试、三侧代表夹具语义/wire/消费长度/padding检查、构建与候选harness vet，再采样。没有并行重型Go任务。

| 主入口 | pass1 中位 / 累计 | pass2 中位 / 累计 |
| --- | ---: | ---: |
| AddressBook Unpack | -1.95% / -3.34% | -2.05% / -6.31% |
| 大型 Unpack | -2.99% / -13.44% | -3.39% / -3.57% |
| 大型 PDecode | +0.84% / +13.75% | -0.69% / -22.87% |
| 整数数组 Unpack | -1.02% / +0.90% | +6.07% / +21.19% |
| 整数数组 PDecode | -2.80% / -17.39% | -1.11% / +4.01% |

全部10项、两类控制、六轮与分配尾差见 [ALL-CASES.md](ALL-CASES.md)、analysis.json。累计按实际调用数加权，独立列出，不用其替代轮次几何比中位数。

## 长尾与因果限制

大型Unpack的self中位−1.25%、IQR5.10pp，同源clone中位−2.32%、IQR4.87pp，与候选约3%的观测同阶。pass2大型Unpack轮次−20.0%、+1.5%、−4.8%、−11.6%、−2.0%、+18.9%；整数数组Unpack单个进程达到20062ns，邻近通常约4900ns。全部保留，没有筛除失败轮次或截取较平稳窗口。

pass2 ASCII Unpack中位−35.06%、PDecode−10.12%，但其主要FF复制分支未改；Unpack基线单个进程达15841ns，邻近约450～650ns。这不能认定为候选带来的FF大幅加速。采样结束后一次uptime显示load averages 9.28/6.81/5.58；它只是结束后的状态，不证明每个长尾的具体外因，不能用来自动删除样本。没有在相同状态下盲目重跑，也不使用最小值代替全部观测。

20ms窗口较短、当前控制和长尾不稳定，本轮不是细小收益确认。下一次应先确认机器负载，再预先固定更长的目标采样窗口和控制；不能事后延长到得到有利结论为止。

## 边界回归

计时完成后，对完整A/B副本加入独立回归（不修改计时源码/二进制）。覆盖256种mask × 8种word数量 × 3种前缀零段 × 2种输入容量，共12288种配置：1/2/7/8/9/255/256/257 words、普通/零/FF、两段尾零、精确输出容量、恰好预算、预算不足、迟到截断、输入不变及跨调用独占输出。每配置4次公开解包调用。

相对既有固定wire和预算用例，新增全部mask跨word/零段邻域与精确切片窗口的组合覆盖。两侧race和严格checkptr均通过。第一次测试后补强“后一次结果不能覆盖前一次输出”断言，在新的v2副本重新验证；前后两次记录均保留，最终验证使用v2四条命令。生产库不添加与既有基础契约重复的临时实验测试。

没有采用候选，因此本轮不重复生产全库、官方oracle、fuzz或GoTS，不声称新增完成这些验收。独立库仍a93bb0d，仅原六项用户删除；正式依赖v0.4.0不变。

## 后续方向

Packing 下一候选先解决**索引范围证明**：普通mask循环只在mask非零时进入，TrailingZeros8数学范围为0～7；研究能否在保留完整检查的情况下把该事实表达给编译器，先确认目标检查实际减少，再启动更长窗口的单模块控制实验。保持零/FF/尾部及全部mask回归，不新增mask查表、缓冲池或共享结果。

Body 的字段/presence和大型Decode对象构造主线保持开放；本轮没有修改其实现，不把解包候选的结果替代Body结论。

```sh
python3 docs/benchmarks/2026-10-20-gosproto-unpack-word/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-unpack-word/regression.py --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-unpack-word/analyze.py --output /absolute/new-output
```

输出目录必须不存在；保留Go缓存。计时归档和二进制位于本轮 `tmp/gosproto-unpack-word-20261020`，不提交二进制。JSON夹具在采样后核对与仓库HEAD及harness副本一致；计时前源码、生成输入和二进制已冻结，采样后重新核对。该时间顺序不扩大为“计时前已冻结JSON哈希”。报告保留全部原始进程输出与独立复算脚本。
