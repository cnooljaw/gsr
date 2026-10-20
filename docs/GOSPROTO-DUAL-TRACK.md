# Go Sproto双主线目标

当前双主线诊断（2026-10-20）：[指针优化后成本](benchmarks/2026-10-20-gosproto-post-pointer-cost/RESULTS.md)基于已提交a93bb0d完成9CPU+9allocation及独立实际wire审计。大型Encode的整数记录预检/写出仍累计21.50%/17.00%；大型/数值数组分别386/417与321/449个word为3～7非零字节，下一Packing候选优先普通word目标视图与扫描，不误投FF。ASCII512个FF word另看扫描/分配。80条profile命令+1条审计、fresh/源码/输入/二进制/raw核对通过，无新生产改动或PB/TS排名。前轮库a93bb0d/GSR3f23e02已提交，新诊断未提交；正式依赖、用户原6项删除及两条主线保持。

当前Body实现（2026-10-20）：[整数记录数组受控指针读取](benchmarks/2026-10-20-gosproto-record-pointers/RESULTS.md)仅在当前字段证明及实际元素指针类型核对后，同步读切片指针槽，减少两阶段逐元素反射，命名指针回退。固定37项两轮，大型Encode−18.63%/−19.05%、PEncode−15.87%/−14.34%；AddressBook PEncode+6.43%/+0.43%、数值Encode+1.61%/+2.01%及31项累计反向保留。178条测量流程、基线新回归及7条全库/oracle/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过，候选和永久回归已提交独立库a93bb0d。前轮库7b05b78/GSR2c5c653已提交，正式依赖及原6项删除不动，Body与Packing继续。

当前Body实现（2026-10-20）：[原生字段分布诊断](benchmarks/2026-10-20-gosproto-inline-shapes/RESULTS.md)沿用同一延迟内容切片候选，固定8/64字段×8分布的Encode/PEncode共32项、128进程两轮。8字段混合Encode−0.57%/−0.77%、64字段半缺失PEncode−2.15%/−2.58%；64字段负数PEncode+1.40%/+0.50%及29项累计反向全部保留。决定保留小改动、接受局部代价，不宣称业务全面提速；158条测量流程及7条全库/oracle/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7验收通过，精确源码已提交独立库7b05b78。前轮19项报告已提交ffc591f，正式依赖v0.4.0及用户原6项删除不动，Body与Packing继续。

当前Body实验（2026-10-20）：[通用Encode延迟内容切片](benchmarks/2026-10-20-gosproto-encode-inline-body/RESULTS.md)完成固定原19项、76进程两轮比较。大型Encode中位−0.36%/−1.36%，首轮累计反慢214.9ns；大型PEncode+2.23%/−0.67%方向翻转。UTF8手工帧−2.33%/−1.32%、AddressBook PEncode−0.06%/−2.11%等小收益完整保留；候选待确认、不应用生产，不设收益比例门槛。102条流程命令、既有契约/夹具wire/来源与逐块复算通过；self13/19、clone15/19，17项累计反向及3项分配尾差保留。参考文档已提交7e48d4c，生产库e43df35、正式依赖及原6项删除不变；新报告未提交，双主线继续。

2026-10-20，用户明确裁决：**同时推进Body Encode/Decode和Pack/Unpack，最终相对各自Protobuf的领先程度向TS结果靠近。**已领先PB不构成停止优化的理由；部分路径若已超过TS参考幅度，保持优势，不强行追同一个比例。

## 评价方式

分开跟踪纯Encode/Decode、单独Pack/Unpack、PEncode/PDecode和真实Request/Attach/Dispatch。每个入口看相对同语言同场PB的时间比、绝对ns和分配，业务组合结果是最终验收，同时保留正文独立进展，不能用一条主线的进展替代另一条。

TS为2026-10-15第五轮历史基线，Go为2026-10-20新五轮；共享代表消息输入哈希相同，但不是同场跨语言配对。PB分别为@bufbuild/protobuf2.15.0和Go protobuf1.36.1，运行时、对象/整数模型、资源校验和分配机制不同。下列比例是目标参照，不是Go必然可达的保证，也不拿跨语言绝对耗时直接排实现优劣。UTF8字符串校验差异、原生与manual API差异继续说明。

负值表示SP耗时低于各自PB。TS未重新采样。

## Body主线参考

| 消息 | Go Encode | TS Encode | Go Decode | TS Decode |
| --- | ---: | ---: | ---: | ---: |
| addressbook | +77.0% | -34.9% | +16.0% | -41.3% |
| game_small | +0.5% | -39.9% | -7.0% | -53.2% |
| game_large | -27.5% | -35.9% | -13.0% | -57.2% |
| int32_edges | -65.5% | -76.7% | -59.5% | -58.3% |
| echo_ascii4k | +10.9% | -77.8% | -3.2% | -27.0% |

## Packed组合参考

| 消息 | Go PEncode | TS PEncode | Go PDecode | TS PDecode |
| --- | ---: | ---: | ---: | ---: |
| addressbook | +85.5% | -11.1% | +39.0% | -28.1% |
| game_small | +23.6% | -18.3% | +21.4% | -39.8% |
| game_large | -8.1% | -15.4% | +14.9% | -44.8% |
| int32_edges | -36.5% | -37.1% | +62.2% | -18.4% |
| echo_ascii4k | +23.2% | -72.8% | +90.6% | -10.6% |

## 推进顺序与约束

- **Body主线保持独立投入**：大型结构的Encode和Decode都继续开放。先定位schema/字段遍历、实际字段访问、对象构造和指针字段分配成本，再提出单一候选；不因大型Encode已赢PB约28%、Decode已赢约13%而降为“基本完成”。数组作为已验证的高效路径和回归控制。countChunks是候选之一，不代表全部Body潜力。
- **Packing主线同步推进**：当前重点Unpack与packed Decode，优先验证整数数组和大型消息，同时保留普通word、零段、FF、小包和不同容量/所有权形态；Pack仍跟踪其对PEncode与真实RPC的影响。不得为了正文领先而省略packing，也不得因为packing落后就冻结正文。
- 两条主线各保持明确候选与验收证据，按热点/工程价值交替推进；一次实验只改变可解释的一处，避免在一个补丁中混入两类改写。不按机械轮数分配工作，不只追容易修改的小点。
- 参考官方Sproto的wire与数据流，参考Go Protobuf的工程实现；保持资源预算、失败原子性、presence、ownership、输出cap与公开契约，不靠删检查或换消息分布制造优势。较大重构可以讨论，但公开API/所有权等变化须按项目裁决流程另行确认。
- 真实可解释的小收益也保留，不设固定最低比例；同时公开非目标退化、控制失败、分配、累计与中位数分歧，不用TS目标为不可靠结果背书。

数据来源：[Go当前对照](benchmarks/2026-10-20-go-sp-pb/RESULTS.md)、[TS第五轮](benchmarks/2026-10-15-sproto-round5/RESULTS.md)、[迁移经验](SPROTO-OPTIMIZATION-LESSONS.md)。本文件只确认目标和对照，不代表新优化已实现，不修改历史实测数据。

## 当前增量进展（2026-10-20）

当前Body实现（2026-10-20）：[结构数组受控指针槽](benchmarks/2026-10-20-gosproto-decode-struct-slots/RESULTS.md)已保留并提交独立库 e43df35。相同对象构造下大型Decode−3.50%/−3.63%、Dispatch−5.80%/−3.04%，长结构重复改善、对象数中位不变；AddressBook PDecode+2.56%/+2.07%及全部控制/累计反向保留。新低GOGC/命名类型/元数据回退/异构指针拒绝回归、全库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过。新增unsafe仅同步借用已核对类型的新切片槽，GC屏障与反射回退保留。前轮成本诊断已提交06adf22，正式依赖v0.4.0不变，原库6项已有删除不动；Body与Packing继续。上方Go/PB比例仍是af9169d的同场实测，本轮仅相对该基线的增量配对；不将旧PB绝对值与新比例相乘伪造当前排名。
