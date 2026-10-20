# Decode 优化后 Packing 成本更新

2026-10-20，基于已提交 gosproto `5360fe0`，包含上一轮整数记录数组 Decode 证明复用。本次完成三种现有代表消息的 Unpack/PDecode 独立 CPU 与全量 allocation profile，共12份、56条流程命令，另有1条实际wire审计。**下一候选优先普通 mask 写出循环的连续源遍历；不先改零段或 FF，不删除完整预检。** 本轮只定位成本，没有新生产候选或性能排名。

## 结果与限制

每份 CPU 单独进程、约2秒；allocation 单独进程、5000次、MemProfileRate=1。GOMAXPROCS1/GOGC100/GOMEMLIMIT=off。保留完整 profile 分母，包括 kevent、GC、调度和分配；不扣运行时样本重算占比。表中每列来自对应独立profile，父子累计占比不可相加，也不能直接当作墙钟耗时比例。

| 场景 | 独立 Unpack flat / cum | PDecode 中 Unpack cum | Unpack 循环 B/op / allocs | PDecode 循环 B/op / allocs |
| --- | ---: | ---: | ---: | ---: |
| game_large | 57.45% / 67.66% | 19.83% | 3456 / 1 | 13872 / 533 |
| int32_edges | 51.58% / 62.90% | 31.60% | 4096 / 1 | 12384 / 5 |
| echo_ascii4k | 0.00% / 20.36% | 12.94% | 4864 / 1 | 9040 / 6 |

普通写出 `dst[index] = src[used]` 所在源码行在大型 Unpack 为590ms/2.35s（约25.1%完整样本），整数数组为310ms/2.21s（约14.0%）；地址计算、内存访问、检查与邻接指令的采样归属都可能包含其中，不能说这些时间全部由边界检查造成。预检读取/预算/推进也有明显成本，暂时保持，避免同时改两处或破坏分配前的完整边界保证。

整数数组 PDecode 的 Unpack 累计31.60%、decodeIntSlice累计10.82%，值得独立推进Packing。大型PDecode仍包含decodeStructSlice累计43.53%、decodeIntegerRecord累计20.69%，后者属于前者的嵌套成本，不能相加。Body仍有空间，本轮不将其冻结，也不把一次profile当作优化收益证明。

ASCII Unpack 的 runtime.kevent占79.19%，mallocgc累计15.84%、memmove4.52%，函数自身flat没有采中。它不代表算法零成本或真实网络I/O瓶颈：同一进程没有业务网络收发，这些运行时/GC上下文会影响短操作profile。不能删掉kevent后宣布复制占比或直接推算加速上限。allocation profile包含初始化、schema构造与预热，里面出现PB/protojson初始化不代表计时循环正在运行PB；循环分配以ResetTimer后的benchmark记录为准，不把全profile对象数除以5000冒充每次分配。

带profile的ns/op完整保留在原始benchmark.txt，仅用于诊断，不作为同场PB对照，也不和旧增量结果拼接。未重采TS。

## 实际 wire

独立副本重新构造相同消息、按packed流逐word重建，再与公开Unpack及原wire核对：

| 场景 | 原始 / packed 字节 | 完整word | 3～7非零字节word | 普通packed word（含尾块） | 实际FF消费字节 |
| --- | ---: | ---: | ---: | ---: | ---: |
| game_large | 3336 / 2280 | 417 | 386 | 415 | 16 |
| int32_edges | 3593 / 2374 | 449 | 321 | 450 | 0 |
| echo_ascii4k | 4104 / 4103 | 513 | 0 | 1 | 4096 |

前两种没有全零完整word；大型的386项实际均为3～6非零字节，整数数组也无7非零字节项。ASCII几乎全在FF批量复制；不能用它评价普通mask循环。两条优化路径须分开测，小包、稀疏1/2字节、密集3～7字节和零段仍要作为回归控制。wire审计的输入哈希与既有夹具保持相同，不修改消息分布制造优势。

## 下一项可验证候选

当前普通循环每次找最低有效位，再读 `src[used]`，按mask清位来判断结束。下一实验仅改这段：先计算本word的 `n := bits.OnesCount8(header)`，取得已预检的连续源窗口，再 `range` 窗口中的字节，按当前mask位置写入固定8字节目标。源推进仍为n，目标仍推进8，输出仍由本次make清零。

预期工程依据是让Go编译器看到源遍历的确定范围，减少循环中的索引证明和终止分支；**这是待证假设**。代价是第二遍新增一次popcount和每word窗口构造，可能伤害只有1～2字节的稀疏word；需要看汇编和实测，不凭“更连续”就采用。不能沿用历史收益比例推算，也不设固定最低收益门槛。

只读核对官方C的sproto_unpack：FF走memcpy，普通word按8个位扫描并显式写零；C的指针和调用方缓冲机制与Go精确独占输出不同，不能为模仿C删除GSR已承诺的完整预检/预算/ownership。本候选借鉴按mask消费连续输入的思想，不更改wire，不复制C的输出所有权或错误处理。公开API和资源契约不变，无新generator、池或unsafe。

后续先检查新循环是否确实减少源检查、是否新增栈/分配，再用全部256种mask、截断、预算、零段、FF、独占结果已有回归核对；通过后同模块同进程self/clone及两轮比较，覆盖真实Unpack和PDecode。只有保留候选时再执行完整集成验收。本轮未实现候选，未声称这些后续检查已完成。

## 验证与来源

测量前完成代表夹具测试、vet和编译；12份profile均正常结束。计时源码、生成输入、harness、唯一SP模块、二进制及全部原始profile哈希前后核对，完整记录在inputs/commands/validation。56条profile流程命令及独立wire审计1条均成功。源码只来自5360fe0完整Git归档，未恢复原库go.mod或原6项用户删除；实际库Go文件仍等于已验收、已提交的源码。

本轮没有生产代码改动，不重复全库/race/fuzz/GoTS完整验收；上一轮这些检查的结果不冒充本轮执行。上轮已本地提交库5360fe0、GSR b0b1c4c；新诊断报告与文档留审查，尚未提交。正式依赖v0.4.0不变，未推送或发布。

```sh
python3 docs/benchmarks/2026-10-20-gosproto-packing-refresh/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output /absolute/new-profiles
python3 docs/benchmarks/2026-10-20-gosproto-packing-refresh/collect.py --output /absolute/new-profiles
python3 docs/benchmarks/2026-10-20-gosproto-packing-refresh/analyze.py docs/benchmarks/2026-10-20-gosproto-packing-refresh
python3 docs/benchmarks/2026-10-20-gosproto-packing-refresh/audit.py /absolute/new-profiles /absolute/new-audit
```

输出目录须不存在；保留Go缓存。原始profile与二进制保留在tmp/gosproto-packing-refresh-20261020，wire独立副本在tmp/gosproto-packing-refresh-audit-20261020，不提交二进制。文本、调用记录及输入/原始文件哈希随报告保存。
