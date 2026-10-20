# Body Decode非负tag表示（候选保留待确认）

2026-10-20。上一轮Unpack局部word实验已提交GSR `36f03f2`，未推送。本轮回到Body主线，仅将decodeMessage的tag计数由int改为uint32，并在原范围检查后转int查询字段。**候选未应用生产，保留小收益、成本与全部样本待确认。**不以“已经领先PB”停止正文优化；正式依赖v0.4.0及双主线不变。

## 范围、正确性与增量测试

三处源码改变：tag以uint32零值开始，跳跃增量转uint32，FieldByTag入口转int。实际字段的tag>65535拒绝位置、奇数头跳跃/未知字段、inline/chunk/scalar、公开Fields即时读取和逐字段布局证明均未改。不缓存schema或字段、不给对象分配/预算/错误/ownership增加捷径，Pack/Unpack、countChunks待确认候选也未混入。

一个message的头数是uint16，最多65535项；每项增加至多32768（实际字段仅+1），因此最大累计65535×32768=2147450880，小于MaxInt32与MaxUint32。不会发生uint32回绕，跨32/64位转换无截断；字段查找仍在tag<=65535检查之后。纯跳跃链结束时原代码允许最终tag超过65535，新代码保持该行为，不偷偷强化wire拒绝规则。

新增隔离公共API回归TestDecodeTagCounterBoundaries，在两库上比较5形态×2种预算，共10组：最大合法tag65535、最大附近未知tag、越界实际字段、65535个最大跳跃头、65534个最大跳跃头之后的实际字段。验证消费长度/错误class、正确值与独占指针、失败原值。相比原常规schema/预算用例，这是最大累计范围与跳跃结束语义的增量；没有在多层重复造夹具。测试在未修改基线和候选上均通过。

实际ARM64 decodeMessage总指令仍660条；inline的FieldByTag负数位检查1→0，但其他数据流/转换/布局仍有成本，不能按少一分支预告加速。原字段查找的公开Fields边界守卫不删除，没有新增unsafe或API。

## 固定70项

self/clone/两轮compare共280进程、26,880块、6720四元组、1680轮、1120快照，原矩阵/单项进程/10ms块/3×128分配/50ms预热与校准保持；GOMAXPROCS1/GOGC100/无限GOMEMLIMIT。所有测试/构建/vet/汇编先于首块采样，冻结来源、顺序与原sampleBlock/blockCalls，不重采择优/删长尾/扣控制。

| 入口 | 第1轮 | 第2轮 | 第1轮累计ns | 第2轮累计ns | self / clone控制 |
| --- | ---: | ---: | ---: | ---: | --- |
| body/addressbook/decode | -0.22% | +0.35% | -124.5 | +43.4 | True / True |
| body/game_small/decode | -1.86% | -0.61% | -19.6 | +74.3 | False / True |
| body/game_small/pdecode | -0.17% | -0.85% | +0.3 | -114.4 | False / False |
| rpc/game_small/dispatch | -0.83% | -0.17% | -16.1 | -38.9 | True / True |
| body/game_large/decode | -0.08% | -4.51% | +139.8 | -681.3 | True / True |
| body/game_large/pdecode | +1.07% | +0.30% | +1554.6 | +83.2 | True / False |
| rpc/game_large/dispatch | +0.70% | +0.27% | +106.6 | +644.1 | False / True |
| growth/flat/full/4096/decode | +0.01% | -0.30% | -5903.3 | -5255.5 | True / True |
| growth/flat/full/4096/pdecode | -0.72% | -0.10% | +141622.9 | +31323.5 | False / False |
| growth/nested/32x128/decode | -0.26% | +0.01% | -954.2 | -11523.1 | False / False |
| growth/nested/32x128/pdecode | +0.20% | -1.32% | -11783.1 | -7413.1 | False / False |
| growth/array_record/4096/decode | +0.21% | +0.80% | -23025.3 | +2279.7 | True / True |
| body/int32_edges/decode | -0.03% | -0.59% | -184.6 | -8.5 | True / False |

大型纯Decode两轮观察更快，但首轮仅−0.08%，累计反而增加139.8ns；次轮−4.51%、累计减少681.3ns，含1.1636最差轮，不可只公布次轮“快4.5%”。大型PDecode+1.07%/+0.30%、Dispatch+0.70%/+0.27%，中位与累计均为两轮成本。不能用Body结果覆盖组合成本，也不据此否定Body独立进展。

小型纯Decode−1.86%/−0.61%、PDecode−0.17%/−0.85%，Dispatch−0.83%/−0.17%、累计少16.1/38.9ns，两类Dispatch控制通过；这些数十ns及亚百分比收益完整保留，不按固定比例抛弃。但小型Decode第二轮中位更快、累计慢74.3ns。full记录PDecode两轮中位略快而累计慢141622.9/31323.5ns，不能只选有利统计量。

当前选择暂不应用、保留待确认：主目标Body改善幅度不稳定，长结构/组合没有形成明确的整体取舍；不是最低收益门槛。小型Dispatch与其他局部收益不抹掉，也不能将本轮认定为所有路径必然失败。后续若重评，必须预先说明要解决的证据问题，不为等好结果盲目重跑。下一步应检查更大的结构构造/字段执行成本，不继续仅排列tag条件；双主线继续朝TS参考领先程度推进。

## 验证、控制及来源

self48/70、clone41/70，失败控制/全部轮次保留，所有对象分配中位相同；28字节尾差、28入口中位/累计反向和全部GC块/最差轮保留。最大堆快照5,927,496B非峰值、limiter零；四模式最长块 146.9/106.6/99.4/164.1ms。大型纯Decode两类控制通过，但组合/其他目标并非全部通过，不将控制通过当作统计显著性证明。

313条流程命令成功：两侧原目标契约、70入口语义、109形态525项差分、10组新增tag边界、harness测试/vet/编译/机器码及采样。fresh/18份Go源/工具/夹具/生成输入/harness/二进制/raw/原采样函数/冻结顺序核对。候选只decode.go不同，生产库全部Go源仍af9169d。

候选未采用，未重复完整库/oracle/race/checkptr/fuzz或GSR/GoTS验收，accept.py为未来在完整归档采用时配方、本轮未运行。没有PB/TS新排名；原库6个既有删除（含go.mod）未恢复或修改，不能称工作树干净。正式go.mod/go.sum保持HEAD，新报告未提交，不推送/发布/tag，Decode继续开放。

```sh
python3 -B docs/benchmarks/2026-10-20-gosproto-decode-tag-counter/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-decode-tag-counter-new
python3 -B docs/benchmarks/2026-10-20-gosproto-decode-tag-counter/analyze.py \
  tmp/gosproto-decode-tag-counter-new
```

输出目录必须不存在。原始源码/二进制/逐项进程在忽略tmp，聚合raw/复算/汇编/来源指纹随报告保留。
