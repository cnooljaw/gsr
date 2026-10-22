# 最新Unpack / PDecode热点复核

基于已提交的正常库 `9e47281`，本轮完成三消息×Unpack/PDecode×两遍，共12份CPU profile和55条流程命令。没有生产源码改动，没有新性能排名；目标是整数数组Decode改善后，重新确认组合路径的剩余成本。

## 实际wire说明该优化什么

统计独立于计时；packed流按格式逐段解释，先复用既有wire摘要与语义检查。直方图覆盖所有256个header，FF段按自身扩展长度计word，不将payload字节当header。

| 消息 | 正文B | packed B | 输出word | 普通mask | 零mask | FF段/word | 普通mask有效字节数分布 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 896整数数组 | 3593 | 2374 | 450 | 450 | 0 | 0/0 | 1位×1、2位×128、3位×1、4位×64、5位×128、6位×128 |
| 大型游戏 | 3336 | 2280 | 417 | 415 | 0 | 2/2 | 1位×1、2位×28、3位×29、4位×177、5位×87、6位×93 |
| AddressBook | 130 | 83 | 17 | 15 | 0 | 2/2 | 2位×8、3位×1、4位×4、6位×1、7位×1 |

[完整分布](patterns.json)。本轮输入中零段根本未执行，FF只处理少量word；优化这两个分支难以直接改善这里的数组或大型组合。没有扩展为“所有Sproto消息都这样”的结论。

## CPU采样证据

[逐遍源码归属表](SUMMARY.md)、[机器可读摘要](summary.json)。以下秒数均为各自独立profile中的采样量，不同入口调用数不同，不能直接相减当耗时。

| Unpack单入口 | 两遍预检flat | 两遍普通写出阶段flat | 两遍普通字节赋值flat | 两遍make行累计 |
| --- | --- | --- | --- | --- |
| 896整数数组 | 0.62 / 0.58 | 0.69 / 0.79 | 0.47 / 0.48 | 0.12 / 0.16 |
| 大型游戏 | 0.65 / 0.70 | 0.85 / 0.85 | 0.51 / 0.53 | 0.07 / 0.09 |
| AddressBook | 0.44 / 0.45 | 0.58 / 0.43 | 0.36 / 0.26 | 0.50 / 0.39 |

预检flat不含内联OnesCount8子帧：数组另有0.16/0.09秒，大型0.12/0.11秒，完整表单列。make列为调用累计，其余列为flat，不能混加成总CPU；赋值行属于写出阶段，不能重复加。

数组PDecode中Unpack累计占完整Total samples的40.00%/36.32%；大型20.35%/21.12%；AddressBook13.25%/9.09%。完整profile保留runtime.kevent，十二份占比15.91%～52.02%，不剔除再归一化，不推断硬件或调度原因。源码行归属是采样线索，不能把赋值行的全部成本等同于一次store或据此承诺提速。

数组和大型消息里，首遍校验与普通字节恢复均值得关注；make不是这两者的首要采样热点。AddressBook相反存在较明显分配累计成本，因此不能把长消息结论推广到小消息。

## 复制与分配审计

当前Unpack先确认完整输入和确切输出大小，随后make一次，再恢复有效字节。没有先生成另一份完整输出再复制；FF调用copy只是该段解包所需写入。PDecode本身调用Unpack再Decode；独占解包缓冲是现有所有权约束的一部分。

Unpack基准数组4096 B/1 alloc，大型3456 B/1 alloc，AddressBook144 B/1 alloc；B/op包含Go分配size class，不能当成输出len/cap。例如数组解包长度450×8=3600，实际B/op为4096。PDecode分别12384 B/5 alloc、13872 B/533 alloc、672 B/30 alloc，与上一轮正常入口一致。profile运行中的ns/op不作为新的无profile性能基线。

不通过池化/借用输出或一遍动态append消除这份独占分配；此轮不改变预算检查、错误先后、精确cap或失败原子性。

## 下一实验与已排除方向

已重新阅读旧候选：[普通Unpack早期筛选](../2026-10-16-gosproto-unpack-candidates/RESULTS.md)、[连续源窗口](../2026-10-20-gosproto-unpack-source-range/RESULTS.md)、[混合mask](../2026-10-20-gosproto-unpack-mixed-masks/RESULTS.md)、[真实高频mask](../2026-10-20-gosproto-unpack-frequent-masks/RESULTS.md)、[预检普通分支](../2026-10-20-gosproto-unpack-precheck-ordinary/RESULTS.md)。查表、源窗口、按密度/固定mask推断通用性能和单纯拆普通分支均已有负面或不确定证据，不因这次profile再重复同一改写。

下一轮仅实验普通mask循环每轮恢复至多两个有效字节：继续按TrailingZeros8和清最低位获取相同顺序，保留每次有效读写范围，只减少两字节之间的循环回跳。它仍会有终止判断，不能预先声称指令更少或一定更快；先看编译结果、全部mask/尾部/预算回归，再测实际混合wire、稀疏/密集与小包及PDecode。若新增分支或代码体积抵消收益，如实保留结果，不用固定收益百分比裁决。

该候选尚未实施，本轮仅完成可复核的最新诊断，生产代码保持已验收版本。

## 验证与复现

完整Git归档、既有fixture/wire校验、harness vet、编译及十二次采集均通过；源文件、fixture与二进制SHA采集前后相同。[计划与指纹](plan.json)、[执行日志](commands.json)、[模块解析](modules.txt)。没有新增运行语义，不重复跑全库race/fuzz；上轮验收仍在其报告中。

```sh
python3 docs/benchmarks/2026-10-22-gosproto-unpack-refresh/run.py tmp/unpack-refresh-new
python3 docs/benchmarks/2026-10-22-gosproto-unpack-refresh/analyze.py
```

原始二进制CPU profile保存在忽略的tmp采集目录，Git保留源码级/flat/cum文本与采集元数据。正式依赖仍v0.4.0，用户十项删除不动。前轮报告已提交 `badfb74` 未推送；本轮新报告和harness留审查、未提交。
