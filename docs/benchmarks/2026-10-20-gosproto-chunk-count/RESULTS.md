# Body Decode：countChunks避免未使用内容视图（收益待确认）

2026-10-20。双主线目标/当前Go-PB对照及阶段诊断已本地提交GSR `386727c`，未推送。本轮实现隔离候选，**暂未应用到生产库，不将其判为确定失败或有效提速**。Body与Packing双主线保持；候选和全部正面/负面结果留存。正式依赖v0.4.0不变。

## 唯一修改与行为证明

仅decode.go的countChunks改变。原来调用readChunk并丢弃content，现在在计数循环内直接读取4字节长度，完整检查chunk边界后得到used，再执行原budget.add(1)、count++和输入推进。其他需要content的调用继续readChunk；Encode、Pack/Unpack、对象构造、实际元素Decode和字段元信息不改。

检查序列保持：len(data)<4先返回ErrDecode；size以uint64读取并先比较uint64(len(data)-4)，成功才转换int并计算used。因此0<=size<=len-4，32/64位均保证4+int(size)<=len<=MaxInt。随后才扣单元素预算，保留坏当前chunk先于预算错误的优先级与budget进度；所有chunk预检仍在数组分配前完成。返回count/错误、结果独占/失败原子性/presence不变，没有新unsafe、池、表、公开API或特定消息分支。

实际ARM64 countChunks约97→85条，栈帧由assembly-summary核对，未使用content的panicSliceAcap调用点1→0，真正推进输入的panicSliceB仍1。readChunk本已inline，候选不是通过去掉一次函数调用提速；它复制了必要长度校验以避免不使用的返回视图，具有小幅维护成本。静态减少不直接等于动态收益。

计时前通过两侧原目标契约、70入口构造/wire/值/消费长度及109形态525项growth差分、harness统计/筛选/GC/代表夹具/默认指标/vet。没有新行为，不增加镜像生产测试；未修改的Unpack预算差分不重复运行，沿用harness文件但未执行其中专用测试。

## 固定70项结果

沿用70项、self/clone/两轮compare，各项独立进程；280进程、26,880块、6,720四元组、1,680轮、1,120快照。默认GOMAXPROCS1/GOGC100/无限GOMEMLIMIT，50ms预热、清sink/GC、10ms块与3×128分配计数保持；所有构建/测试/汇编先于首个计时。没有修改块校准函数、重采择优、删样或扣控制。

| 入口 | 第1轮 | 第2轮 | 第1轮累计 ns | 第2轮累计 ns | self / clone控制 |
| --- | ---: | ---: | ---: | ---: | --- |
| body/addressbook/decode | -0.18% | +1.11% | -4.7 | +4.3 | False / False |
| body/addressbook/pdecode | -2.12% | -0.31% | -39.7 | -141.6 | True / False |
| body/game_small/decode | -1.41% | +1.75% | -23.3 | +35.3 | False / False |
| body/game_small/pdecode | -1.48% | -0.48% | -18.0 | -29.7 | False / False |
| body/game_large/decode | -0.72% | -0.10% | -234.8 | -47.4 | False / False |
| body/game_large/pdecode | +1.15% | -2.00% | +32.7 | +3149.4 | False / False |
| rpc/game_large/dispatch | -1.03% | +2.33% | -856.0 | -1077.5 | False / False |
| growth/flat/full/4096/decode | -0.12% | -1.51% | -3113.9 | -149150.5 | False / False |
| growth/flat/full/4096/pdecode | +0.76% | -0.92% | -26902.1 | -7393.4 | False / False |
| growth/flat/half/4096/decode | -3.78% | +0.60% | -9737.5 | +1115.2 | False / False |
| growth/flat/half/4096/pdecode | -1.36% | +0.72% | -9818.8 | -25189.8 | False / False |
| growth/nested/32x128/decode | -2.46% | +0.18% | -25933.8 | -3915.0 | False / False |
| growth/nested/32x128/pdecode | -1.51% | -0.03% | -165548.3 | -41669.3 | False / False |
| growth/array_record/4096/decode | -1.55% | -0.06% | +2798.3 | -30531.2 | False / False |
| growth/array_record/4096/pdecode | -2.34% | +0.28% | -88091.8 | +18288.0 | False / False |
| body/int32_edges/decode | -0.75% | -3.38% | -10.4 | -109.6 | False / False |
| body/echo_ascii4k/pdecode | -1.44% | +7.51% | -148.9 | +1106.3 | False / False |

大型纯Decode两轮方向更快，full记录纯Decode及嵌套PDecode也有同向改善，这些小收益全部保留。另一方面，大型PDecode中位+1.15%/−2.00%，累计却两轮增加32.7/3149.4ns；大型Dispatch中位−1.03%/+2.33%，累计两轮减少856.0/1077.5ns。half结构与其他组合多有方向翻转，不能只取首轮或有利统计量。

未调用countChunks的int32纯Decode也快0.75%/3.38%；不能将它归功本候选或拿来扣除目标差值。函数/链接布局变化与采样环境均未被逐一排除，不声称确定唯一噪声来源。

## 当前决定与后续

**保留待确认候选，不应用生产源码。**这是证据状态，不是收益下限。self仅8/70、clone11/70满足原控制条件（该条件是测量告警，不是最低优化收益门槛）；例如大型Decode self中位约−1.01%、IQR约6.28个百分点，clone约+3.45%、IQR约4.69个百分点，和候选−0.72%/−0.10%的观察量级相比，无法可靠区分小改善。不能将此解释为“低于某百分比不保留”，也不能仅凭静态简化宣称成功。

本轮不为等控制通过盲目重跑。后续若继续验证，先检查开发机竞争/调度及测试块的实际分布，预先确定与目标同量级的控制诊断；保留本轮数据并独立标注后续结果。更大的Body结构成本与Packing问题仍继续追踪，不因这个局部候选停住双主线，也不以TS目标替代证据。

## 来源、验证与限制

313条流程命令成功；全部对象数分配中位相同，26项字节尾差、30入口中位/累计反向保留；limiter全零，最大堆快照7,233,080B（非峰值）。四模式最长块 327.7/319.3/353.6/375.3ms，全部GC和最差轮保留。

fresh/18份Go源、生成输入/工具/夹具/harness/原采样函数/二进制/raw和冻结执行顺序核对。基线af9169d，候选只decode.go不同；主库全部Go文件仍等基线。独立库原有6个删除项（含go.mod）未恢复/提交，测试与采样使用完整归档，不能称工作区干净。没有新PB/TS排名或分配减少结论。

候选未采用，因此未执行完整库/oracle/race/checkptr/fuzz和GSR/GoTS验收；accept.py只是将来采用时在完整快照上的配方，本轮未运行。原始库已有删除不纳入候选；正式go.mod/go.sum保持HEAD。不提交本轮新报告、不推送/发布/tag，Decode继续开放。

```sh
python3 -B docs/benchmarks/2026-10-20-gosproto-chunk-count/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-chunk-count-new
python3 -B docs/benchmarks/2026-10-20-gosproto-chunk-count/analyze.py \
  tmp/gosproto-chunk-count-new
```

输出目录必须不存在。源码/二进制/逐项进程留忽略tmp，聚合raw、统计/工具、汇编和指纹随报告保存。
