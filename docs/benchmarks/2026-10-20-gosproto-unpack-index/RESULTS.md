# Unpack 非零 mask 的索引范围表达

2026-10-20，gosproto基线 `a93bb0d`。上一轮[仅固定八字节视图](../2026-10-20-gosproto-unpack-word/RESULTS.md)未消除目标检查；本轮在独立归档副本增加 `index := bits.TrailingZeros8(mask) & 7`，仍写入固定八字节目标视图。

## 正确性依据与汇编

普通mask的header==0已在外层处理，每次清除最低置位后，mask==0立即退出；所以进入写入语句时mask始终非零，TrailingZeros8的数学范围为0～7，`& 7`对所有可达值是恒等操作。没有改变wire值、错误输入接纳范围或错误优先级，也没有删除首遍输入边界/输出预算预检。目标word长度已由首遍验证；输出仍一次精确分配且独占。没有新增unsafe、缓冲池、共享结果或查表。

ARM64汇编确认目标索引检查消除，普通写入行只剩源`src[used]`的检查：

| 静态项 | 基线 | 仅视图（上轮） | 视图＋范围（本轮） |
| --- | ---: | ---: | ---: |
| UnpackWithLimit 机器字行（含填充） | 384 | 392 | 388 |
| panicIndex 调用点 | 5 | 5 | 4 |
| panicSlice* 调用点 | 8 | 10 | 10 |

本轮普通字节循环的CBNZW回跳距离由基线15条指令变为13条（见A/B汇编，距离不是整个动态循环的精确指令数），每个word仍增加视图建立的检查。调用点和静态指令数不等于动态执行次数；短稀疏word可能无法收回固定成本。A/B/C函数入口地址相同，不代表所有后续代码布局、运行状态相同。编译器仍生成TrailingZeros8处理零值所需的ORR $256；本轮不再追加第二个算法候选。

## 固定100ms采样

先完成三侧已有Pack/Unpack测试、代表夹具语义/wire/消费长度/padding检查、构建与harness vet，再固定100ms窗口；不是看到结果后延长到获得有利结论。每进程50ms预热，GOMAXPROCS1/GOGC100/GOMEMLIMIT=off。三个二进制各加载一份官方身份的SP模块：self A/A、clone A/C同源、pass1/pass2 A/B。10项×4模式×6轮×4独立进程，共960个进程、240轮、977条准备及测量流程命令，全部成功。

全部样本、控制和代价见 [ALL-CASES.md](ALL-CASES.md)、analysis.json；控制不相减，不筛掉长尾，不以最小值代替全部观测。累计按实际调用数加权，单独列出。

| 主入口 | pass1 中位 / 累计 | pass2 中位 / 累计 |
| --- | ---: | ---: |
| AddressBook Unpack | -1.60% / -5.36% | -3.02% / -5.37% |
| 大型 Unpack | -1.89% / +6.79% | -0.51% / -1.48% |
| 大型 PDecode | -6.33% / -7.08% | +1.14% / +0.36% |
| 整数数组 Unpack | -1.88% / -4.40% | -4.02% / -4.86% |
| 整数数组 PDecode | -1.68% / +3.43% | -0.40% / +9.33% |
| ASCII PDecode | +7.27% / +9.98% | +9.15% / +9.50% |

**静态目标已达成，整体净收益仍未确认，暂不并入生产库。** 整数数组与AddressBook Unpack两轮中位/累计均改善，小收益完整保留，不设最小比例门槛；但组合路径、非目标ASCII代价及控制仍不稳定，不能直接用它们证明整体优化成功。

例如整数数组Unpack同源控制中位−1.02%、IQR12.39pp，六轮包含+50.67%；大型PDecode self IQR17.42pp；ASCII PDecode self中位+8.82%、IQR29.34pp、clone中位+3.50%、IQR10.19pp。ASCII主要走未改FF分支，但函数布局可能改变，不能把其回退自动认定为纯噪声后忽略。进程测量开始load为7.24/6.48/5.72，结束约5.82/6.52/5.93，均保存；这些状态不能解释每个异常样本的确切来源。

把20ms扩大到100ms仍未解决当前机器状态与进程/GC波动。下一步不继续盲目加长或重跑相同矩阵：Unpack是纯字节操作，可在**同一模块、同一进程**内隔离基线/同源副本/候选循环，固定输入与输出所有权，用交错块和GC/分配观测区分普通word固定成本、逐有效字节成本及进程噪声；隔离结果仍需回到公开Unpack/PDecode验收，不能代替完整入口。Body字段/presence与对象构造主线保持开放。

## 回归与来源

计时后，在完整A/B副本加入上一轮已补强的全部mask回归：256 masks×8 word数量×3前缀零段×2输入容量，共12288配置、每配置4次公开调用。覆盖普通/零/FF、短长邻域、精确容量、恰好/不足预算、迟到截断、输入不变及前后结果独占。两侧race和严格checkptr共4条命令通过。复用同一回归验证本轮新范围表达，没有向生产库重复添加测试。

计时前冻结Go源码、生成输入、二进制和JSON夹具哈希，完成后全部核对；JSON夹具同时等于仓库HEAD和harness副本，正式go.mod/go.sum不变。原始命令输出、模块解析、汇编及逐轮结果保留；独立复算比例/IQR至1e-12。没有采用候选，不重复全库/oracle/fuzz/GoTS验收，也不声称本轮完成这些。独立库仍a93bb0d，原六项用户删除保持。

```sh
python3 docs/benchmarks/2026-10-20-gosproto-unpack-index/precheck.py \
  --library /private/tmp/gsr-gosproto-fork --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-unpack-index/measure.py --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-unpack-index/regression.py --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-unpack-index/analyze.py --output /absolute/new-output
```

首次输出目录必须不存在，后续阶段复用它且不得覆盖已有采样。保留Go缓存；本轮归档/二进制在 `tmp/gosproto-unpack-index-20261020`，不提交二进制。报告与相关文档留审查，正式依赖仍v0.4.0。
