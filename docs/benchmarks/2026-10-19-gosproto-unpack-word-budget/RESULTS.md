# 完整预检按word记录预算（未采用）

2026-10-19。前轮组合实验已本地提交 GSR `885505c`，未推送。本轮仅将预检预算由字节改为八字节word，直接对照生产库 `af9169d`。**候选未采用**，主库从未应用补丁、仍干净；收益、代价、控制失败与全部长尾保留，新报告未提交。正式依赖仍v0.4.0，Decode继续开放。

## 改动及等价性

保留单函数、remaining尾切片扫描与原恢复，不携带此前索引/分函数候选。设置 maxWords=maxBytes/8、wordsLeft=maxWords；预检produced按word计数，FF输入消费仍按字节计算，每段检查输入边界后检查wordsLeft，再递减。最后一次按(maxWords-wordsLeft)*8精确分配，后续恢复源码逐字相同。没有新函数调用、unsafe、池、查表、公开API或fixture分支。

证明依赖已有事实：每段输出长度均为8的整数倍。令已确认word数为W，则 floor((maxBytes-8W)/8)=floor(maxBytes/8)-W。因此零段扫描上限、超额判断与旧预算等价；输入截断优先于对应输出预算不足的顺序未改变。wordsLeft始终非负，最终word数不超过maxBytes/8，乘8不超过maxBytes且不溢出。maxBytes<1仍先返回ErrInvalidLimits；空输入、结果len/cap/nil与独占语义保持。

在已有预检差分中增加MaxInt-7、MaxInt-1、MaxInt三种预算，20→23种，总159,275组。这是新增的int边界覆盖，不另建镜像生产测试；原所有mask/截断/FF/零段、错误class、结果长度容量/空值、输入不变与独占差分仍在。两侧既有目标契约、109形态525项差分和70入口构造检查均在采样前通过。

实际arm64编译从384条减为372条，栈帧由112B增至128B，无新增helper调用。减少指令不意味着动态成本或速度必然降低。标准库OnesCount8在原机码中已经是查表，不重复造表；此次不改变popcount或恢复算法。

## 固定70项结果

280进程、26,880块、6,720四元组、1,680轮、1,120快照，self/clone/两轮compare顺序冻结。保留默认GOMAXPROCS1/GOGC100/无限GOMEMLIMIT、原10ms块/校准/3×128分配/清sink/GC/50ms预热，不重采择优、剔除GC或扣控制。负值更快，完整数据见[全部70项](ALL-CASES.md)。

| 入口 | 第1轮 | 第2轮 | 第1轮累计 ns | 第2轮累计 ns | self / clone 控制 |
| --- | ---: | ---: | ---: | ---: | --- |
| body/game_small/unpack | +5.36% | +5.87% | -40.7 | +30.3 | True / True |
| body/game_large/unpack | +7.93% | +6.77% | +602.2 | +329.8 | False / True |
| body/game_large/pdecode | +2.24% | +0.59% | +30.2 | -381.4 | True / True |
| rpc/game_large/dispatch | +2.11% | +1.02% | -604.3 | +251.0 | False / True |
| body/int32_edges/pdecode | +4.02% | +4.02% | +415.0 | +338.5 | False / True |
| array/width8_len896/pdecode | +4.13% | +6.24% | +654.3 | +1179.7 | False / False |
| array/width4_len3/unpack | -5.37% | -5.24% | -0.2 | -2.3 | True / False |
| shape/nonzero1x256/unpack | -6.80% | -5.15% | -95.2 | -28.3 | True / False |
| shape/nonzero2x256/unpack | -6.95% | -6.04% | -98.7 | -92.9 | False / False |
| shape/nonzero7x256/unpack | +4.51% | +4.61% | +97.2 | -25.6 | True / False |
| shape/FF512-sparse4096/unpack | +32.95% | +34.19% | +1096.2 | +1267.3 | True / False |
| shape/zeroRun8x64/unpack | -0.85% | -2.59% | +18.8 | -34.6 | True / False |
| shape/zeroRun16x64/unpack | -2.41% | -3.50% | -37.7 | -69.1 | True / True |
| shape/zeroRun64x64/unpack | -1.26% | -1.44% | -261.3 | -129.4 | True / True |
| shape/zero4096/unpack | -1.29% | -1.39% | +180.3 | -9.4 | True / True |

短数组纯Unpack、单/双有效字节word和零段获得小幅收益，全部保留。但大型Unpack慢6.77%～7.93%、4字节大数组PDecode两轮慢4.02%、8字节大数组慢4.13%～6.24%，FF接稀疏慢32.95%～34.19%。新增算术表示没有带来整体更合适的取舍，因此不替换生产实现；不是因小收益未达比例门槛。普通word3～7有效字节也多有退化，不把单/双字节收益扩张到全部稀疏输入。

中位数与累计耗时分歧同时保留：大型PDecode第二轮中位慢0.59%，累计快381.4ns；大型Dispatch第一轮中位慢2.11%，累计快604.3ns；小型Dispatch第二轮中位慢0.60%，累计快3709.0ns。不能只选有利统计量，也不能把控制漂移全部归因候选。

## 验证、异常与限制

self 48/70、clone 39/70；所有对象分配中位相同，26项字节分配尾差、22个入口中位/累计反向均保留。最大堆快照 49,684,096 B（约47.38MiB），不是峰值，limiter全零；四模式最长块 216.3/92.3/336.1/84.5 ms。全部快照/GC事件/最长块与最差轮保留，不为异常重跑。

313条命令成功；fresh归档、18份Go源码、生成输入/工具/夹具/二进制/raw/顺序和sampleBlock/blockCalls逐字核对。见[验证指纹](validation.json)、[独立复算](analyze.py)。主库仍af9169d且干净，未应用候选，因此未重复完整库/oracle/race/checkptr/fuzz或GSR/GoTS验收。不作新的PB/TS排名。go.mod/go.sum保持HEAD，未推送、发布或tag。

这一轮说明“缩减静态指令＋减少单位换算”仍不足以预测该热路径的表现。后续不继续排列计数器写法；若再优化，先针对当前生产版采集预检与恢复的独立成本证据，明确动态读取、分支或分配的主成本后再提出候选。不将未测的硬件原因写成结论，Decode继续开放。

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-unpack-word-budget/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-unpack-word-budget-new
python3 -B docs/benchmarks/2026-10-19-gosproto-unpack-word-budget/analyze.py \
  tmp/gosproto-unpack-word-budget-new
```

输出目录必须不存在。原始逐项文件、源码与二进制留在忽略tmp目录，聚合数据、工具、机器码与指纹随报告保存。
