# 当前正式版本 Body/组合入口热点复核

基线gosproto `9048cfa`，GSR `ef7a471`。近期几个微候选未形成重复收益，本轮重新采集正式实现证据，而不是继续按静态指令数选方向。三个输入（AddressBook、大型游戏、整数数组）×四个公开入口（Encode/PEncode/Decode/PDecode）×CPU/全量分配，共24份profile。104条主流程及6条编码源码导出均成功，源码、输入、二进制、原始profile哈希核对通过。无生产改动、无PB/TS新排名。

## 热点证据

百分比保留完整CPU样本分母，flat与累计分开，父子累计不能相加：

| 消息/入口 | 函数 | flat | 累计 |
|---|---|---:|---:|
| 大型Encode | encodedIntegerRecordSize | 8.12% | 13.25% |
| 大型Encode | encodeIntegerRecordInto | 9.83% | 11.97% |
| 大型PEncode | packInto | 10.67% | 10.67% |
| 大型PEncode | encodedIntegerRecordSize | 9.33% | 9.78% |
| 大型PEncode | encodeIntegerRecordInto | 4.44% | 9.78% |
| 大型Decode | decodeIntegerRecord | 2.60% | 13.42% |
| 大型Decode | mallocgc | 2.60% | 19.48% |
| 大型Decode | reflect.New | 0% | 8.23% |
| 大型PDecode | UnpackWithLimit | 9.01% | 12.16% |
| 数组PEncode | packInto | 21.30% | 21.30% |
| 数组PDecode | UnpackWithLimit | 20.44% | 25.78% |

AddressBook Encode的通用encodedSize累计33.03%、encodeMessageInto29.82%，其路径不同于整数记录数组。不能把专用路径的一条改写推广到所有消息。

大型Decode的全量分配对象视图中，decodeIntegerRecord为57.21%，reflect.New为38.48%。这包含初始化/预热及tiny allocator计数，不等于每次调用源码new次数；不能除以5000伪造精确单次构成。它支持优先关注分配/构造路径，不能推导“删除几个检查就能解决全部Decode成本”。每个子对象与presence指针独立仍是契约，不通过合并指针存储、驻留或池化改变GC滞留范围换速度。

大型Encode分配空间中输出构造约86.22%，encodeLayout.add约12.77%。当前布局131项、栈内32与外溢99的既有审计仍适用，512B外溢并非全部浪费；不能仅为了game_large把栈数组硬扩到恰好容纳它，也不能删预检后直接分配输出。

## 正常循环分配

以下为ResetTimer后的5000次循环数据，不是全量profile总量；各格为B/op / allocs/op：

| 消息 | Encode | PEncode | Decode | PDecode |
|---|---:|---:|---:|---:|
| AddressBook | 144 / 1 | 144 / 1 | 528 / 29 | 672 / 30 |
| 大型 | 3968 / 2 | 3968 / 2 | 10416 / 532 | 13872 / 533 |
| 整数数组 | 4096 / 1 | 4096 / 1 | 8288 / 4 | 12384 / 5 |

没有出现新分配回退；PEncode不多分配完整body，PDecode在这些输入上多一个独占Unpack输出。不能用不同入口的带profile耗时直接相减，估计某个阶段精确CPU成本。

## 下一步范围

1. Body先回到整数记录Encode的预检与写出整体，检查本次调用内布局记录的产生/消费是否仍有重复工作。先对照已完成的布局分段、字段/Tag缓存等历史实验，不能把同一失败写法换名再测。新候选须有实际重复工作的源码证据，而非只少一条条件。
2. Decode不再继续叠加header/tag/depth微改写。若探索构造模型，先明确实际类型、可变描述回退、失败原子性与每对象/字段独立所有权；不默认引入generator、runtime私有分配接口或新API。更大的取舍单独裁决，本轮不实施。
3. Packing仍是整数数组组合入口的大项；保留此前已证明的分配前检查、FF/尾块及重叠工作区规则，不重试已否定的双字节写出/固定mask/来源窗口等候选。

这是下一实验的范围，不预先宣称这些方向必然更快。当前结果不足以宣布已追平PB或向TS领先幅度靠近了多少。

## 方法与限制

每个case/op/kind独立进程；GOMAXPROCS=1/GOGC=100/GOMEMLIMIT=off，50ms预热，CPU每入口2s；分配入口5000次、memprofilerate=1。先通过fixture/vet/build，再采样；原始profile/二进制留在tmp/gosproto-body-refresh-20261021，报告保存文本、metadata和SHA。

runtime.kevent占完整CPU样本20.40%～78.92%，各入口差异显著。没有删除它重归一化，也不将它解释成业务网络成本；本轮没有采集足够系统trace解释其来源。2s短采样只适合方向性定位，不能将与旧profile百分比变小当成性能进步。当前正式源码与前轮已拒绝候选不同，未混入实验选择器。

没有生产变更，验证范围为fixture、harness静态检查与证据一致性，不重复全库/race/fuzz/oracle/GoTS。

```sh
python3 docs/benchmarks/2026-10-21-gosproto-body-refresh/reproduce.py --library /private/tmp/gsr-gosproto-fork --output /absolute/new-output
python3 docs/benchmarks/2026-10-21-gosproto-body-refresh/extra.py --output /absolute/new-output
python3 docs/benchmarks/2026-10-21-gosproto-body-refresh/collect.py --output /absolute/new-output
```

输出目录须不存在。正式GSR依赖仍v0.4.0，独立库七项无关删除不恢复、不暂存。
