# Go Sproto双主线目标

2026-10-22收尾：[gosproto优化状态](GOSPROTO-OPTIMIZATION-STATUS.md)与[完整报告](benchmarks/2026-10-22-gosproto-closure/RESULTS.md)。本轮P0～P5已完成验收；新增原生普通word Pack使整数EncodePacked两遍约快35%、大型约快14%～18%，另保留准入的小收益，purego回退恢复原指令形态。最终整数packed解码仍慢PB26.5%，不宣称全面胜出。完整库/官方oracle/purego/race/checkptr/vet、fuzz、跨架构编译、GoTS7/7和完整Benchmark通过。普通word Pack与回归已本地提交gosproto `3c505c3`；本次收尾资料随GSR提交归档，均未推送。正式依赖仍v0.4.0，库中已有删除未纳入提交。历史“下一步”不代表当前待办。

2026-10-22最新实现：[ARM64完整预检扫描](benchmarks/2026-10-22-gosproto-preflight-arm64/RESULTS.md)保留256B紧凑计数表版，完整边界/预算/错误顺序与预检后分配保持。公开整数DecodePacked两遍改善15.79%/10.33%，大型改善1.45%/5.87%，ASCII/小消息代价与clone波动全部保留。正式72项对照整数packed仍慢PB30.7%，大型快8.0%，40项分配不变；完整库官方oracle/purego/race/checkptr/vet、有限fuzz、跨架构编译及GoTS7/7通过。新源码与报告未提交，前轮91e6c01已提交未推送；正式依赖及十项用户删除保持。

2026-10-22最新复核：[递减预算同源clone控制](benchmarks/2026-10-22-gosproto-budget-clone/RESULTS.md)完成20进程1920块，26条流程命令、源码/助记符/不同符号地址及二进制核对通过。相同算法clone也有数个百分点差异；候选大型公开入口本轮+0.06%/−0.72%，未重复确认前轮小收益。旧收益与全部控制保留，不扣clone校正，候选仍归档待确认、未应用。下一步转向可直接减少状态维护的预检实验。前轮已提交0dc46d8，新报告未提交；库92a155a、正式依赖及十项用户删除保持。

2026-10-22最新实验：[递减剩余预算](benchmarks/2026-10-22-gosproto-preflight-budget/RESULTS.md)完成45进程4320块，保留候选待确认、未应用生产。整数Unpack两遍改善2.52%/1.07%，大型公开DecodePacked改善1.53%/1.71%；整数公开入口反转，高密度word有代价，栈帧144→160B。全部小收益、控制与尾差保留，下一步补同源clone控制，不设固定比例门槛。差分native/purego race/checkptr、完整库官方oracle及指纹复核通过。前轮已提交cad81b6，新报告未提交；库92a155a、正式依赖及十项用户删除保持。

2026-10-22最新实验：[普通word独立预检](benchmarks/2026-10-22-gosproto-preflight-ordinary/RESULTS.md)完成45进程4320块，含真实DecodePacked复核。稀疏word改善约16%～18%，但混合整数Unpack慢约1.4%，公开整数PDecode中位慢0.57%/6.49%，未采用通用替换；全部小收益、累计反向与控制保留。native/purego差分race/checkptr、完整库官方oracle及来源核对通过。前轮报告已提交8438a42；新实验未提交，库92a155a、正式依赖及十项用户删除保持。

2026-10-22最新实验：[Unpack预检收紧临时切片容量](benchmarks/2026-10-22-gosproto-preflight-cap/RESULTS.md)完成33进程3168交错块，未采用通用替换。稀疏word两遍改善12.37%/6.89%，但整数数组PDecode中位慢1.14%/0.95%，6/7非零word明显退化；全部小收益、累计反向、自对照及校准不足保留，不设百分比采纳门槛。native/purego差分race/checkptr、完整库官方oracle及来源核对通过。前轮诊断已提交2d95255，新报告未提交；库92a155a、正式依赖及十项用户删除保持。

2026-10-22最新诊断：[向量恢复后的解码成本](benchmarks/2026-10-22-gosproto-post-vector-cost/RESULTS.md)基于已提交库92a155a/GSR feb7c31完成9 CPU＋9全量分配及3份GC敏感性profile，80＋12条命令与源码/汇编/夹具/二进制/raw核对通过。整数数组仍优先Unpack普通mask预检循环；OnesCount8已是常量查表，不重复造表。默认kevent占比高已披露，不将profile耗时当PB新排名。无新生产改动；本轮诊断未提交，正式依赖与十项用户删除保持。

2026-10-22最新：[ARM64 Unpack向量恢复](benchmarks/2026-10-22-gosproto-unpack-vector/RESULTS.md)在完整预检后对128B～1MiB压缩输入启用VTBL及FF批量复制；其他架构/purego回退。M2同进程两遍整数数组Unpack约改善8.7%、PDecode约4.5%～5.4%，AddressBook与ASCII有局部代价。完整库、官方oracle、race/checkptr、purego、vet、有限fuzz、保护页及4096组混合wire、GoTS7/7通过。正式72项对照中数组PDecode仍慢PB约38.3%，未宣称全面胜出。前轮报告已提交76ffa2b；新源码与报告未提交，正式依赖v0.4.0及十项用户删除保持。

2026-10-22阶段收尾：[Go Sproto最终优化与72项PB对照](benchmarks/2026-10-22-gosproto-final/RESULTS.md)。本轮保留至少32项的原生整数数组编码，以及长8字节数组正文解码；386/amd64/arm64启用，其他架构与短数组保留原循环。最终整数消息Encode/PEncode较9e47281起点降低16.75%/8.01%，全部40项代表消息分配不变；大型正文领先PB，数组PDecode仍慢51.2%，未宣称全面胜出。三个候选阶段108进程2448行、完整库/oracle/race/checkptr/vet、有限fuzz及GoTS7/7验收完成；32位旧测试常量溢出已在基线复现，仅声明生产编译通过。新源码及文档留审查未提交，前轮d6d4106已提交未推送；正式依赖v0.4.0、十项用户删除保持。

当前Body与测量诊断（2026-10-20）：[字段Tag局部复用](benchmarks/2026-10-20-gosproto-record-tag/RESULTS.md)确认两记录函数Tag加载2→1、指令/栈不变，但净收益未确认，未采用。原37项大型Encode+6.22%/+7.18%，同源跨模块clone本身+6.66%，不能直接归因于候选。追加单模块独立二进制96worker，Encode+1.95%、PEncode−0.86%，控制/离散仍明显，全部小收益与反向保留，不扣控制差值。原178条流程＋补充102条及两侧描述替换race/checkptr通过；后续关键Go候选补单模块复核，旧数据不回填。前轮440f436已提交，库a93bb0d及原6项删除保持，新报告未提交，双主线继续。

当前Body实验（2026-10-20）：[数组局部字段引用](benchmarks/2026-10-20-gosproto-record-fields/RESULTS.md)固定37项及补充17/32字段4项两轮完成，未采用。大型Encode+9.03%/+15.75%、PEncode+6.47%/+7.34%，回退17/32字段Encode约慢10%～12%；全部正收益与累计反向保留，不设比例门槛。无新增堆分配但fieldSize栈256→432B、记录循环指令与检查增多。178+46条测量命令、原契约/109形态525差分、两侧字段容量/乱序tag/描述替换race/checkptr回归通过；库a93bb0d和原6项删除保持，前轮28c494f已提交，新报告未提交，双主线继续。

当前Packing实验（2026-10-20）：[普通Pack共享目标窗口](benchmarks/2026-10-20-gosproto-pack-shared-window/RESULTS.md)固定55项两轮完成，静态指令296→284但大型Pack+0.50%/−0.39%、5非零word+3.37%/+4.11%，不采用通用替换。2非零word−1.63%/−0.13%、数值PEncode−1.34%/−0.03%及全部小收益/66项累计反向保留；大型PEncode中位慢、累计两轮快一并披露。250条测量命令、109形态525差分与两侧41472次重叠窗口race/checkptr回归通过；原库a93bb0d及原6项删除不动。前轮诊断125bc91已提交，新实验未提交，依赖和双主线保持。

当前双主线诊断（2026-10-20）：[指针优化后成本](benchmarks/2026-10-20-gosproto-post-pointer-cost/RESULTS.md)基于已提交a93bb0d完成9CPU+9allocation及独立实际wire审计。大型Encode的整数记录预检/写出仍累计21.50%/17.00%；大型/数值数组分别386/417与321/449个word为3～7非零字节，下一Packing候选优先普通word目标视图与扫描，不误投FF。ASCII512个FF word另看扫描/分配。80条profile命令+1条审计、fresh/源码/输入/二进制/raw核对通过，无新生产改动或PB/TS排名。前轮库a93bb0d/GSR3f23e02已提交，新诊断未提交；正式依赖、用户原6项删除及两条主线保持。

当前Body实现（2026-10-20）：[整数记录数组受控指针读取](benchmarks/2026-10-20-gosproto-record-pointers/RESULTS.md)仅在当前字段证明及实际元素指针类型核对后，同步读切片指针槽，减少两阶段逐元素反射，命名指针回退。固定37项两轮，大型Encode−18.63%/−19.05%、PEncode−15.87%/−14.34%；AddressBook PEncode+6.43%/+0.43%、数值Encode+1.61%/+2.01%及31项累计反向保留。178条测量流程、基线新回归及7条全库/oracle/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过，候选和永久回归已提交独立库a93bb0d。前轮库7b05b78/GSR2c5c653已提交，正式依赖及原6项删除不动，Body与Packing继续。

当前Body实现（2026-10-20）：[原生字段分布诊断](benchmarks/2026-10-20-gosproto-inline-shapes/RESULTS.md)沿用同一延迟内容切片候选，固定8/64字段×8分布的Encode/PEncode共32项、128进程两轮。8字段混合Encode−0.57%/−0.77%、64字段半缺失PEncode−2.15%/−2.58%；64字段负数PEncode+1.40%/+0.50%及29项累计反向全部保留。决定保留小改动、接受局部代价，不宣称业务全面提速；158条测量流程及7条全库/oracle/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7验收通过，精确源码已提交独立库7b05b78。前轮19项报告已提交ffc591f，正式依赖v0.4.0及用户原6项删除不动，Body与Packing继续。

当前Body实验（2026-10-20）：[通用Encode延迟内容切片](benchmarks/2026-10-20-gosproto-encode-inline-body/RESULTS.md)完成固定原19项、76进程两轮比较。大型Encode中位−0.36%/−1.36%，首轮累计反慢214.9ns；大型PEncode+2.23%/−0.67%方向翻转。UTF8手工帧−2.33%/−1.32%、AddressBook PEncode−0.06%/−2.11%等小收益完整保留；候选待确认、不应用生产，不设收益比例门槛。102条流程命令、既有契约/夹具wire/来源与逐块复算通过；self13/19、clone15/19，17项累计反向及3项分配尾差保留。参考文档已提交7e48d4c，生产库e43df35、正式依赖及原6项删除不变；新报告未提交，双主线继续。

2026-10-20，用户明确裁决：**同时推进Body Encode/Decode和Pack/Unpack，最终相对各自Protobuf的领先程度向TS结果靠近。**已领先PB不构成停止优化的理由；部分路径若已超过TS参考幅度，保持优势，不强行追同一个比例。

## 评价方式

测量模块身份：同进程双SP模块的同源clone曾出现约6.66%的大型Encode差异（见[诊断](benchmarks/2026-10-20-gosproto-record-tag/RESULTS.md)）。关键候选增加单一模块身份、独立二进制与进程控制复核；不得将候选差值减去控制差值，或把同阶变化直接归因于代码。单模块也有进程噪声，须保留逐轮结果。历史数据不回填，重要收益应在当前公开API基线上复核。

分开跟踪纯Encode/Decode、单独Pack/Unpack、PEncode/PDecode和真实Request/Attach/Dispatch。每个入口看相对同语言同场PB的时间比、绝对ns和分配，业务组合结果是最终验收，同时保留正文独立进展，不能用一条主线的进展替代另一条。

TS为2026-10-15第五轮历史基线，Go为[2026-10-20 当前 a93bb0d 新五轮](benchmarks/2026-10-20-go-sp-pb-current/RESULTS.md)；共享代表消息输入哈希相同，但不是同场跨语言配对。PB分别为@bufbuild/protobuf2.15.0和Go protobuf1.36.1，运行时、对象/整数模型、资源校验和分配机制不同。下列比例是目标参照，不是Go必然可达的保证，也不拿跨语言绝对耗时直接排实现优劣。UTF8字符串校验差异、原生与manual API差异继续说明。

负值表示SP耗时低于各自PB。TS未重新采样。

## Body主线参考

| 消息 | Go Encode | TS Encode | Go Decode | TS Decode |
| --- | ---: | ---: | ---: | ---: |
| addressbook | +63.5% | -34.9% | +37.6% | -41.3% |
| game_small | -5.5% | -39.9% | -7.9% | -53.2% |
| game_large | -41.6% | -35.9% | -17.5% | -57.2% |
| int32_edges | -66.3% | -76.7% | -59.5% | -58.3% |
| echo_ascii4k | +8.9% | -77.8% | +5.2% | -27.0% |

## Packed组合参考

| 消息 | Go PEncode | TS PEncode | Go PDecode | TS PDecode |
| --- | ---: | ---: | ---: | ---: |
| addressbook | +85.5% | -11.1% | +46.2% | -28.1% |
| game_small | +19.9% | -18.3% | +21.2% | -39.8% |
| game_large | -21.8% | -15.4% | +10.9% | -44.8% |
| int32_edges | -34.7% | -37.1% | +61.3% | -18.4% |
| echo_ascii4k | +28.3% | -72.8% | +90.5% | -10.6% |

## 推进顺序与约束

- **Body主线保持独立投入**：大型结构的Encode和Decode都继续开放。先定位schema/字段遍历、实际字段访问、对象构造和指针字段分配成本，再提出单一候选；不因大型Encode本轮赢PB约42%、Decode约18%而降为“基本完成”。数组作为已验证的高效路径和回归控制。countChunks是候选之一，不代表全部Body潜力。
- **Packing主线同步推进**：当前重点Unpack与packed Decode，优先验证整数数组和大型消息，同时保留普通word、零段、FF、小包和不同容量/所有权形态；Pack仍跟踪其对PEncode与真实RPC的影响。不得为了正文领先而省略packing，也不得因为packing落后就冻结正文。
- 两条主线各保持明确候选与验收证据，按热点/工程价值交替推进；一次实验只改变可解释的一处，避免在一个补丁中混入两类改写。不按机械轮数分配工作，不只追容易修改的小点。
- 参考官方Sproto的wire与数据流，参考Go Protobuf的工程实现；保持资源预算、失败原子性、presence、ownership、输出cap与公开契约，不靠删检查或换消息分布制造优势。较大重构可以讨论，但公开API/所有权等变化须按项目裁决流程另行确认。
- 真实可解释的小收益也保留，不设固定最低比例；同时公开非目标退化、控制失败、分配、累计与中位数分歧，不用TS目标为不可靠结果背书。

数据来源：[Go当前对照](benchmarks/2026-10-20-go-sp-pb/RESULTS.md)、[TS第五轮](benchmarks/2026-10-15-sproto-round5/RESULTS.md)、[迁移经验](SPROTO-OPTIMIZATION-LESSONS.md)。本文件只确认目标和对照，不代表新优化已实现，不修改历史实测数据。

## 当前增量进展（2026-10-20）

当前Body实现（2026-10-20）：[结构数组受控指针槽](benchmarks/2026-10-20-gosproto-decode-struct-slots/RESULTS.md)已保留并提交独立库 e43df35。相同对象构造下大型Decode−3.50%/−3.63%、Dispatch−5.80%/−3.04%，长结构重复改善、对象数中位不变；AddressBook PDecode+2.56%/+2.07%及全部控制/累计反向保留。新低GOGC/命名类型/元数据回退/异构指针拒绝回归、全库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过。新增unsafe仅同步借用已核对类型的新切片槽，GC屏障与反射回退保留。前轮成本诊断已提交06adf22，正式依赖v0.4.0不变，原库6项已有删除不动；Body与Packing继续。上方Go/PB比例已由 a93bb0d 单模块公开 API 同场总览更新；历史增量配对保留，不与旧 PB 绝对值相乘。

2026-10-20：[Decode/Unpack 独立进程诊断](benchmarks/2026-10-20-gosproto-decode-cost/RESULTS.md)完成18份profile和80条命令。大型Decode的对象构造、普通mask解包写出均为实测热点；下一Packing切片先测固定八字节目标视图，保留首遍预算/边界预检，Body字段/presence路径继续开放。尚无新候选实现或收益结论。

2026-10-20：[Unpack 固定八字节视图实验](benchmarks/2026-10-20-gosproto-unpack-word/RESULTS.md)完成。960个单模块独立进程、self/clone及两轮比较全部保留；大型Unpack中位两轮约−3%，但控制同阶、整数数组反向和长尾使净收益未确认。静态目标检查未减少（指令384→392），候选暂不并入；全部mask/预算/独占结果两侧race/checkptr通过。下一步先解决索引范围证明，再在明确负载条件下测量，不设最小收益比例门槛。

2026-10-20：[Unpack 非零mask索引范围实验](benchmarks/2026-10-20-gosproto-unpack-index/RESULTS.md)消除目标索引检查（panicIndex静态点5→4），源读取与完整预算/边界预检保留。固定100ms单模块独立进程两轮，整数数组Unpack中位−1.88%/−4.02%，但组合累计方向及ASCII代价不稳、控制波动仍大，净收益未确认，候选尚未并入。全部mask两侧race/checkptr通过。下一步在同一模块/进程隔离纯字节循环，再回到公开入口验收；不设最小收益比例门槛，Body继续开放。

2026-10-20：[同模块/同进程Unpack实验及采纳](benchmarks/2026-10-20-gosproto-unpack-inprocess/RESULTS.md)完成60进程/5760块。保留固定视图＋非零mask索引范围表达：整数数组Unpack两轮中位−1.03%/−2.50%，7非零字节word−3.00%/−3.87%；大型及组合路径不确定、局部小包成本与21项中位/累计反向保留。同源大型控制仍偏，不声称噪声消失。完整库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过，fuzz3310次后停增如实记录。生产代码及永久回归已提交独立库50c69e1、逐文件等于验收快照，原6项删除和正式依赖v0.4.0不变，未推送。

2026-10-20：[字符串数组受控写槽](benchmarks/2026-10-20-gosproto-string-slots/RESULTS.md)保留长数组收益及小数组代价。实际元素恰为原生string时同步借用新切片，命名元素反射回退、完整预检/复制/GC屏障保持；128/512项Decode两轮约−9%，512项PDecode约−7%，3项及现有大型Decode有局部代价，不宣称整体游戏提速。88进程/8448块、基线新增回归、完整库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过；Codec456532次，Decode13781次后停增如实记录。代码及永久回归已提交独立库aef6876且等于验收快照，原6项删除和正式依赖不变，未推送。

2026-10-20：[消息头顺序游标实验](benchmarks/2026-10-20-gosproto-header-cursor/RESULTS.md)未采用。大型Decode中位两轮+0.32%/+0.12%，PDecode−0.79%/−0.29%的小幅正向及所有反向保留；控制同阶，机器字行不变、栈320→336B。修正一次超出当前编码跨度的新增夹具，初次18个self样本与失败命令单独保留；完整v2为80进程/7680块/110条成功命令，全部20输入预检、头部上限/跳跃/预算等三parser race/checkptr通过。原库仅原6项删除，生产仍aef6876。下一Body候选考察整数记录数组入口的当前字段/类型证明复用，不改逐项wire/预算/对象独占，不跨调用缓存。


2026-10-20：[整数记录数组 Decode 证明复用](benchmarks/2026-10-20-gosproto-decode-record-proof/RESULTS.md)保留至少8项的短数组回退版。大型Decode两轮中位−9.40%/−9.50%、PDecode−7.76%/−6.22%，128/512项正文约快12%～13%；首版1/2项明显退化完整保留，第二版补3/4/7/8/9边界后消除该退化。当前可达字段/实际类型在本次数组入口核对，逐项wire/预算/独占对象与GC屏障保持，不跨调用缓存。两版150+200条命令、10752+14592块全部保留；新增回归先在基线通过，完整库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过，Decode fuzz2735次后停增如实记录。候选源码等于验收快照，独立库代码及永久回归已提交5360fe0，GSR报告同步提交；原6项删除和正式依赖v0.4.0不变。前轮GSR0c23252已提交；下一切片回到Packing成本诊断，Body继续开放，无新PB/TS排名。


2026-10-20：[Decode优化后Packing成本更新](benchmarks/2026-10-20-gosproto-packing-refresh/RESULTS.md)基于已提交5360fe0完成三消息Unpack/PDecode共12份独立CPU/全量allocation profile，56条流程及1条实际wire审计通过。大型/整数数组普通写出源码行约占完整Unpack样本25.1%/14.0%，实际386/321个word为3～6非零字节；ASCII几乎全走FF，kevent/GC上下文保留，不重算分母或当网络瓶颈。下一单一候选为按有效字节数range连续源窗口，先核对popcount/检查/栈成本再测，完整首遍和ownership保持；本轮尚未实现。无新生产改动或PB/TS排名，库5360fe0与GSR b0b1c4c已提交，新报告留审查，原6项删除及正式依赖不变，Body继续开放。


2026-10-20：[Unpack连续源窗口遍历](benchmarks/2026-10-20-gosproto-unpack-source-range/RESULTS.md)不采用通用替换。少一个循环源索引检查，但增加每word计数/窗口检查，机器字行388→396、栈不变；大型Unpack两轮+13.04%/+12.87%、整数数组+13.52%/+14.12%。固定4～7字节密度的重复收益、小包正向、21项累计反向和全部控制/长尾保留，不按最低收益比例舍弃。84进程8064块、115条命令、全部mask三入口race/checkptr、夹具/vet/预检/指纹复算通过。下一步区分固定密度和真实混合mask顺序，不直接按密度分流。本轮无新生产代码，库5360fe0、正式依赖和原6项删除不动；前轮诊断已提交63d8015，新报告留审查，Body与Packing继续。


2026-10-20：[连续源Unpack混合mask诊断](benchmarks/2026-10-20-gosproto-unpack-mixed-masks/RESULTS.md)沿用同一候选，20输入、80进程7680块及110条流程通过。真实流保持段集合/直方图/长度后分组或洗牌仍慢约11%～15%，不能把退化简单归因于原始顺序；固定4/7字节收益保留，4有效位变位置结果不稳定，不据密度直接分流。五组输入不变量、独立字节重建、三入口全部mask race/checkptr及来源核对通过。下一步以真实高频mask隔离位置/依赖，不宣称已定位全部根因。本轮无新生产代码，库5360fe0、正式依赖与原6项删除不动；前轮报告已提交c957a55，新诊断留审查，Body/Packing继续。


2026-10-20：[真实高频mask收尾诊断](benchmarks/2026-10-20-gosproto-unpack-frequent-masks/RESULTS.md)完成22入口88进程8448块/120条流程及另4份CPU profile/21条流程。实际4～6字节mask单独重复也有正向，不能把优势只归于低位连续位置；原始大型/整数数组Unpack仍慢约13%～15%，不采用连续源窗口。全部小收益、14项累计反向、控制与长尾保留，不按门槛舍弃，也不声称已定位全部退化根因。全mask三入口race/checkptr、夹具/vet/预检/指纹复算通过；无新生产代码或PB/TS排名。下一切片保留旧写出，单测首遍普通mask预检分支，完整预算/错误顺序/ownership保持，尚未实现。前轮报告已提交0a5eded，新诊断留审查；库5360fe0、正式依赖和原6项删除不变，Body继续开放。


2026-10-20：[Unpack普通mask预检两版实验](benchmarks/2026-10-20-gosproto-unpack-precheck-ordinary/RESULTS.md)暂不并入。首版拆普通路径导致机器字行388→420；精简版只合并分类，共用原检查，机器字行/栈不变。小型Unpack两轮约快3%保留，但大型无重复正向、整数数组不一致，组合/累计及23项反向完整披露，不按收益门槛舍弃。两版168进程16128块/230条流程、三入口全mask与新增边界/预算冲突错误顺序race/checkptr、夹具/vet/预检/来源复算通过。下一候选为一次packed产出上界证明及严格预算回退，尚未实现，所有边界/错误顺序/ownership保持。无新生产代码；前轮报告已提交6c40523，新报告留审查，库5360fe0、依赖和原6项删除不动，Body继续开放。


2026-10-20：[Unpack一次预算上界证明两版实验](benchmarks/2026-10-20-gosproto-unpack-budget-proof/RESULTS.md)证明成立但暂不采用。首版helper在固定普通mask有重复收益，却给单word约5%代价；共享版只加证明guard，无新helper，大型无一致收益、固定普通mask多数慢1%～3%。小收益、严格预算回退和39项累计反向保留，不按收益门槛舍弃。两版192进程18432块/260条流程＋1条汇编导出，三入口全mask/证明门槛/最大int/错误顺序race/checkptr、夹具/vet/预检/指纹复算通过；无新生产代码。下一Body候选看原生*string受控赋值，保持复制/独占/owner回退，尚未实现。前轮已提交7b24406，新报告留审查；库5360fe0、依赖和原6项删除不变，两条主线继续开放。


2026-10-20：[原生字符串指针受控写入](benchmarks/2026-10-20-gosproto-string-pointer/RESULTS.md)已保留。AddressBook Decode两轮−4.59%/−4.58%、PDecode−3.47%/−3.11%；64字符串Decode约快15%、128字符串记录约快9.5%，不因低于5%舍弃真实业务收益。实际类型/当前owner/Wire/Array证明后类型化赋值，复制/独立对象/GC屏障及命名/外来描述回退保持；全缺失约1%局部代价和全部28项累计反向保留，不宣称大型整数消息全面提速。104进程9984块/140条流程，新增回归先在原提交通过；10条完整验收含库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过，Decode10968次后停增如实记录。精确源码与测试等于验收快照，独立库已提交f5de803；GSR报告同步提交，未推送。依赖仍为v0.4.0，七项无关工作树删除保留，两主线继续。

2026-10-21：[字符串优化后Decode成本复核](benchmarks/2026-10-21-gosproto-decode-refresh/RESULTS.md)基于库f5de803完成18份profile、80条流程与源码/输入/二进制指纹核对。大型整数记录仍为正文热点，分配契约不变；下一Body候选限定非inline整数内容消费，尚未实现。Packing继续以真实普通mask混合输入评估，不重试已否定写法。本轮仅诊断，留审查；前轮库f5de803/GSR f81a60d已提交未推送，依赖v0.4.0和七项无关删除保持。

2026-10-21：[整数记录直接消费标量](benchmarks/2026-10-21-gosproto-record-scalar/RESULTS.md)已保留，独立库已提交ab35ee3。大型Decode四轮中位−1.82%/−1.63%/−1.67%/−0.53%，PDecode−1.84%/−1.07%/−0.71%/−1.18%；原累计反向和小型加长反向17.11%保留，不按比例门槛舍弃。指令408→388、栈192→176B，分配不变；56进程5376块/90条计时流程、三模式既有契约及8条完整生产验收通过，GoTS7/7。Decode fuzz23035次后停增如实记录。前轮GSR e38c350已提交未推送；依赖v0.4.0和七项无关删除保持。

2026-10-21：[结构数组指针局部复用](benchmarks/2026-10-21-gosproto-struct-pointer/RESULTS.md)完成14项两轮、56进程5376块/80条流程及补充三模式结构数组race/checkptr契约，候选待确认、未应用。大型Decode中位−1.05%/−0.36%，clone本身−2.13%；PDecode−0.40%/−0.69%伴累计反向。AddressBook/字符串记录的正面证据和全部17项累计反向保留，不设比例门槛。调用点3→2但指令444→448、栈368→384B；没有分配收益。前轮库ab35ee3/GSR843c5f0及格式修正e7146bf已提交未推送；新报告留审查，依赖与七项删除保持，双主线开放。

2026-10-21：[结构数组无条件提取一次指针](benchmarks/2026-10-21-gosproto-struct-pointer-direct/RESULTS.md)已保留，独立库已提交2e1d44b。大型Decode两轮中位−2.36%/−2.55%、PDecode−1.55%/−1.52%；小型Decode−1.36%/−1.33%。命名指针回退约0.6%～2%成本和16B栈增长明示，原大型累计弱收益/反向及23项方向分歧保留，不减控制或设5%门槛。72进程6912块/100条流程、8条完整验收通过，GoTS7/7。Decode fuzz6864次后停增记录。前轮报告512ae6e已提交未推送；本轮实际库仅decode.go和CHANGES改动已提交，依赖v0.4.0、七项无关删除及双主线保持。

2026-10-21：[最新六入口成本复核](benchmarks/2026-10-21-gosproto-dual-cost/RESULTS.md)基于库2e1d44b完成36份CPU/分配profile，152条流程＋6条补充源码导出通过。大型Encode的整数记录预检/写出累计18.39%/18.83%，下一Body实验限定连续整数记录布局计数的local/extra分段消费，尚未实现。Packing仍看真实普通word的mask/前缀计算与展开，不重复已否定方案。无新生产代码或PB/TS排名；前轮库2e1d44b/GSR894297d已提交未推送，当前诊断留审查，依赖及七项删除保持。

2026-10-21：[整数记录布局分段候选](benchmarks/2026-10-21-gosproto-layout-segments-final/RESULTS.md)已保留，独立库已提交451998e。最终实验大型Encode两轮−2.10%/−1.95%、PEncode−1.30%/−1.41%；7项短数组Encode慢1.45%/2.36%，小收益与反向均保留。初版/去重版共160进程15360块/220条流程，9条完整验收和2条正常入口分配检查通过，GoTS7/7。选择器令两侧layout共同逃逸，实验百分比不冒充正常入口排名；正常大型仍3968B/2alloc，后续需普通入口配对。Decode fuzz12254次后停增记录。前轮05e7ab4已提交未推送；实际代码/测试与报告同步提交未推送，依赖和七项删除不动。

2026-10-21：[布局候选正常入口复核](benchmarks/2026-10-21-gosproto-layout-normal/RESULTS.md)完成1920个测量进程＋60次预检/校准、2003条流程及3条汇编导出。正常大型恢复3968B/2alloc；大型Encode候选中位−2.04%/−3.97%，但同源码clone本身+9.00%/IQR23.87pp，目标指令归一化相同，性能归因仍未确认。不扣控制、不删除小收益/19项累计反向或短数组代价。下一测量方向为无内部选择器的同二进制静态调用树，尚未实现。前轮库451998e/GSRa01f700已提交未推送，本轮无生产改动，新诊断留审查，依赖和七项删除保持。

2026-10-21：[同二进制静态调用树复核](benchmarks/2026-10-21-gosproto-layout-static/RESULTS.md)完成80进程7680块/113条流程，源码可逆/格式化后还原、三模式race/checkptr与正常入口分配对照通过。大型恢复3968B/2alloc；候选Encode中位−7.49%/−1.57%、PEncode−3.27%/−1.53%，但clone大型Encode仍+5.02%，幅度归因未确认，不扣控制或隐去27项累计反向。下一方向是确认实际函数地址改变后的静态树摆放轮换，尚未实现。本轮无生产改动，前轮报告a818d26已提交未推送，依赖及七项删除保持，新诊断留审查。

2026-10-21：[静态树摆放轮换](benchmarks/2026-10-21-gosproto-layout-rotation/RESULTS.md)确认12个关键函数实际偏移变化，三布局共120进程11520块、69条准备＋120条测量流程通过，正常分配/源码还原/契约保持。clone大型Encode偏差012为+4.93%，120/201为+0.44%/+0.37%，支持摆放相关，不指定硬件根因。候选大型Encode六次中位均改善0.80%～2.09%（一次累计反慢0.62%），PEncode六次中位/累计均改善，中位0.78%～2.11%；短数组代价和全部反向保留，不扣控制或池化校正。更支持保留451998e局部候选；下一轮回Packing主线。前轮f842999已提交未推送，本轮无生产改动，新报告留审查，依赖和七项删除保持。

2026-10-21：[Pack 普通 word 提前写 header](benchmarks/2026-10-21-gosproto-pack-header-store/RESULTS.md)完成32进程3072计时块、真实消息 Pack/PEncode 与 self/clone 控制。大型 Pack 两遍中位 +1.78%/+0.01%，PEncode +0.51%/+0.91%；固定七字节首遍 −6.85% 未重复，clone 偏差与所有累计反向保留，不扣控制。3584组 wire/共享缓冲与 race/checkptr、fixture、vet 通过，分配保持；无已确认收益，候选未应用。前轮 FF 初筛报告 d56ae56 已提交未推送，新报告留审查；依赖和独立库七项无关删除保持。

2026-10-21：[Pack 位图直接收集](benchmarks/2026-10-21-gosproto-pack-direct-gather/RESULTS.md)完成32进程3072块、真实 Pack/PEncode 与 self/clone 对照。整数数组 Pack 两遍中位 −1.85%/−1.86%、累计均改善；小型 Pack/PEncode 同向改善，大型近持平，整数数组 PEncode 未同步改善。保留小收益候选待组合路径确认，尚未改生产；七字节累计 +25.34% 与控制偏差全部保留，不扣控制。3584组 wire/共享缓冲、race/checkptr、fixture/vet通过；前轮7676be6已提交未推送，依赖及七项无关删除保持。

2026-10-21：[Pack 位图候选实际摆放轮换](benchmarks/2026-10-21-gosproto-pack-gather-rotation/RESULTS.md)确认三函数在0/1184/2368偏移轮换，48进程4608块、三布局正确性/race/checkptr/fixture/vet通过。整数数组Pack六次中位五次改善，PEncode中位/累计各三次改善；大型PEncode方向随布局变化，尚未形成稳定组合收益。保留候选源码但不应用，全部控制/长尾及大型Pack累计−40.38%保留并解释，不扣控制或设收益门槛。前轮67df7a1已提交未推送，新报告留审查；生产代码、v0.4.0依赖及七项无关删除不变。

2026-10-21：[Pack 四次双字节写出](benchmarks/2026-10-21-gosproto-pack-pair-store/RESULTS.md)完成32进程3072块及3584组 wire/共享缓冲/race/checkptr。八次 byte store 合并为四次 PutUint16 后指令296→332；大型Pack两遍 +44.26%/+45.18%，整数数组Pack +41.20%/+40.98%，PEncode亦明显退化，八项中位/累计均退化，候选不应用。全部控制和长尾保留，不作逐指令成本归因；原位图候选仍独立保留。前轮5603f75已提交未推送，新报告留审查，生产代码、正式依赖和七项无关删除不变；下一轮回Body实际热点。

2026-10-21：[整数记录 inline 直接继续](benchmarks/2026-10-21-gosproto-record-inline-flow/RESULTS.md)保留局部控制流简化，记录写出指令196→188。7项记录Encode两遍−1.92%/−2.57%，prefix31为−2.63%/−1.89%；大型Encode小幅改善，大型PEncode中位改善但累计+1.58%/+2.04%，反向全部保留。40进程3840块/60条实验流程与8条正常入口验收通过，官方oracle/race/checkptr/vet、两项fuzz及GoTS7/7；Decode fuzz156066次后停增并正常退出。前轮39f1d53已提交未推送；独立库源码/CHANGES已提交9048cfa，报告随本次提交归档，均未推送；正式依赖与七项删除保持。

2026-10-21：[整数记录外置4/8字节直接分支](benchmarks/2026-10-21-gosproto-record-width-flow/RESULTS.md)基于9048cfa完成40进程3840块/60条流程。先确认编译器已消除重复inline判断，未为其另改代码；宽度分支候选记录指令仍188。大型Encode/PEncode双遍方向反转，prefix31 PEncode中位−0.29%/−1.84%、累计均改善，局部信号及全部控制/反向保留。因真实组合收益尚未确认且新增重复写出代码，暂不应用，非收益门槛否决。三模式契约/race/checkptr、wire/cap、正常分配parity及vet通过；生产库9048cfa、正式依赖和七项删除不变，新报告留审查。

2026-10-21：[Decode整数记录header游标](benchmarks/2026-10-21-gosproto-decode-header-cursor/RESULTS.md)完成40进程3840块/60条流程，三模式合法/错误/预算状态差分、race/checkptr及fixture/vet通过。记录parser指令388→384、栈176B保持；大型PDecode两遍中位−1.53%/−0.16%、累计均改善，大型Decode−0.72%/+1.44%反转。保留小收益候选但暂不应用，非目标Echo等全部波动和控制保留，不扣控制。实际库9048cfa、依赖和七项删除不变；前轮bfd65b7已提交未推送，新报告留审查。

2026-10-21：[Decode游标实际摆放与延长块复核](benchmarks/2026-10-21-gosproto-decode-cursor-rotation/RESULTS.md)完成三布局24进程2304块、36准备/24测量/3分析。实际parser地址轮换已确认；Decode六次中位五次改善/累计三次，PDecode中位四次/累计两次，保留候选但不应用。40ms校准目标多数达到，整体块中位39.096ms，但192块低于20ms，限制明示；不删控制/长尾、不作扣减。驱动计时前KeyError修复并复用已验证二进制，零计时样本丢弃，记录完整。前轮b40dd25已提交未推送；生产9048cfa、依赖及七项删除保持，新报告留审查。

2026-10-21：[Decode多批校准验证](benchmarks/2026-10-21-gosproto-decode-multibatch-calibration/RESULTS.md)以7批基线中位+3批pilot预定规则完成8进程768正式块、80校准批次。独立复算所有决策通过，正式块最短46.182ms/中位59.130ms，低于40ms为0；实际调整分支未触发，限制明示。Decode候选中位−0.30%/−0.36%但累计反向，PDecode+1.88%/−0.63%，控制波动仍在，保留候选但不落地。仅隔离harness方法验证，生产9048cfa、依赖及七项删除不变；前轮9ce5601已提交未推送，新报告留审查。

2026-10-21：[Decode字段查找无符号范围](benchmarks/2026-10-21-gosproto-decode-lookup-range/RESULTS.md)完成16进程1536块/30条流程。新增dense空洞/负tag map回退/Fields变nil对照，原parser错误预算差分及race/checkptr通过；负数位检查消失但符号指令仍388。大型Decode中位+0.15%/−0.93%，PDecode+0.77%/+0.19%，组合收益未确认，保留局部信号但不应用。多批校准160批独立复算，全部正式块≥44.595ms；控制和长尾不扣减。前轮c532ad1已提交未推送，新报告留审查；生产9048cfa、依赖及七项删除不变。

2026-10-21：[整数叶子局部深度检查](benchmarks/2026-10-21-gosproto-decode-leaf-depth/RESULTS.md)完成16进程1536块/30条流程，保留同位置depth+1检查但省共享depth写回/defer，私有叶子不递归且budget.add不读depth。扩展初始深度至边界的错误/预算状态差分和race/checkptr通过；符号388→292、栈176→128B，但大型Decode中位+5.63%/−1.71%，PDecode−0.79%/+1.61%，累计均慢，未应用。169校准批独立重算通过，正式152块低于40ms但无块低于20ms，限制与全部控制保留。前轮41b4769已提交未推送；生产9048cfa、依赖及七项删除不变，新报告留审查。

2026-10-21：[正式Body四入口热点复核](benchmarks/2026-10-21-gosproto-body-refresh/RESULTS.md)基于9048cfa采集24份CPU/分配profile，104条主流程+6条源码导出通过。大型Encode仍以整数记录预检/写出为业务热点；Decode分配对象视图中记录字段/reflect.New为57.21%/38.48%，保持独立所有权约束；数组组合仍受Pack/Unpack影响。正常分配未回退，kevent完整分母20.40%～78.92%保留，不把profile百分比变化当提速。下一轮先审查Encode布局产生/消费实际重复工作，不再无依据叠加header/tag/depth微改写。前轮ef7a471已提交未推送，本轮无生产改动，新诊断留审查，依赖和七项删除保持。

2026-10-21：[整数叶子布局最终追加](benchmarks/2026-10-21-gosproto-layout-final-append/RESULTS.md)完成包含预检的静态调用树24进程2304块/40条流程，三模式wire/cap/预算失败/race/checkptr与正常分配parity通过。无子布局叶子改为成功后追加最终header，nil追加0，失败私有占位可更少；成功顺序/容量/预算不变。7项和prefix31 PEncode双遍改善，但大型Encode+0.86%/+0.79%、PEncode+4.28%/+7.88%，不采用，正面小收益保留。三个校准进程触顶65536未达40ms，全部块≥30.405ms，限制明示。前轮d6fc2e8已提交未推送，生产9048cfa/依赖/七项删除保持，新报告留审查。

2026-10-21：[整数记录局部预算计数](benchmarks/2026-10-21-gosproto-record-local-budget/RESULTS.md)完成24进程2304块/40条流程，新增10368组成功/错误后的完整budget/layout差分并通过race/checkptr；逐字段检查及错误顺序保留，只在叶子正常/错误出口写回局部计数。指令256→252；大型PEncode中位−1.29%/−0.97%但第二遍累计反向，Encode−3.06%/+0.71%，保留局部信号但不应用，手工出口维护成本明示。多批校准243批复算，records7四进程触调用上限未达40ms，正式块≥32.837ms；控制/长尾不扣减。前轮7f34e8c已提交未推送；生产9048cfa、依赖和七项删除不变，新报告留审查。

2026-10-21：[整数记录header输出游标](benchmarks/2026-10-21-gosproto-record-header-output-cursor/RESULTS.md)完成两版静态预检、第二版24进程2304块，共56条流程。首版固定前缀增加检查而未计时；直接传递剩余header版panic点14→13、指令188→200，契约/race/checkptr和正常分配parity通过。大型Encode中位−1.26%/−0.21%，PEncode+2.26%/+0.69%，短记录Encode略慢，不应用，所有局部正面/控制/累计保留。前轮ebd5511已提交未推送，生产9048cfa/依赖/七项删除保持，新报告留审查。

2026-10-22：[近期大型证据审计](benchmarks/2026-10-22-gosproto-evidence-audit/RESULTS.md)独立复算六候选48进程4608原始块，SHA和已发布摘要一致；24候选进程7次中位/累计反向，8次留一轮中位跨零。敏感性仅诊断，不删轮、不相减控制、不设新门槛。基于局部预算轮级信号继续[返回计数集中提交](benchmarks/2026-10-22-gosproto-record-budget-return/RESULTS.md)：16进程1536块/30流程、10368状态对照/race/checkptr通过，全部块≥46.063ms。大型PEncode中位−0.12%/−0.95%但首遍累计+10.67%、prefix31反转，暂不应用。前轮e747630已提交未推送，生产9048cfa/依赖保持；期间独立库新增README/pack_test/sproto_test删除，共十项删除均未触碰，新审计与实验留审查。

2026-10-22：[原生整数数组四项解码](benchmarks/2026-10-22-gosproto-int-array-block4/RESULTS.md)保留按四项共用固定输入/输出视图，余数与命名/窄整数回退不变。16进程1536块，896项Decode两遍中位−13.82%/−18.71%，PDecode−2.77%/−4.63%；大型Decode小幅改善，PDecode首遍近持平，全部控制/累计/长尾保留。预算/错误/独占差分、官方oracle、全库race/checkptr/vet、两项fuzz及GoTS7/7通过；Decode fuzz10981次后停增并正常退出。正常源码与5项边界增量留独立库审查未提交；前轮be8592f已提交未推送，正式依赖及十项用户删除保持。下一轮补短数组性能与正常入口复核。

2026-10-22：[整数数组正常入口及短数组复核](benchmarks/2026-10-22-gosproto-array-normal-review/RESULTS.md)两个无选择器的正常Git归档完成24进程672行/35命令，896项Decode三轮中位−21.68%、PDecode−4.86%，分配不变；大型消息接近波动，不宣称稳定收益。针对首次三项短数组回退补400ms目标24进程72行复测，回退未复现；1项4字节+0.10%，8项8字节−1.40%，5项4字节−7.89%但self−18.49%，不能宣称全面短消息提速。全部数据/控制/轮次保留，不扣减控制。前轮库9e47281及GSR60e2fb6已提交未推送；本轮无生产改动，正式依赖/十项用户删除保持，新报告留审查。下一步检查数组组合Unpack热点。

2026-10-22：[最新Unpack/PDecode热点复核](benchmarks/2026-10-22-gosproto-unpack-refresh/RESULTS.md)基于9e47281完成12份CPU profile/55条流程与wire直方图。896项数组450word全为普通mask，大型417word中415普通、无零mask；预检与普通字节写出为主要剩余成本，输出分配非两者首要采样热点，小型AddressBook不同。完整kevent分母15.91%～52.02%保留，profile不当新性能排名；正常分配未变。复核旧查表/源窗口/密度/预检候选，下一轮只试普通mask两有效字节循环，仍保留所有校验。前轮badfb74已提交未推送，本轮无生产修改，新诊断留审查，正式依赖/十项删除保持。

2026-10-22：[普通mask双字节循环](benchmarks/2026-10-22-gosproto-unpack-pair-loop/RESULTS.md)完成正常A/B归档36进程720行/51流程、全部mask及预算/截断/独占race/checkptr。普通路径每轮至多两有效字节，指令388→408、静态panic14→16、栈112B不变。整数数组PDecode中位−0.81%/−2.64%有局部信号，但Unpack+1.90%/−0.01%，大型PDecode第二遍+15.92%，单word3/7有效字节双遍反向；全部控制/累计/正向保留，不设收益门槛。因归因未确认与短包风险暂不应用，生产仍9e47281；前轮0b224bd已提交未推送，依赖/十项删除保持。本轮留审查，下一步更新正常库/PB总体对照再定位投入。

2026-10-22：[三步稳定字节展开](benchmarks/2026-10-22-gosproto-unpack-spread/RESULTS.md)完成通用/密集两版。正常36进程720行极端离散保留；补同模块两版48进程4608块、独立校准复算与全mask/race/checkptr。6/7有效字节流重复改善约19%～35%，但通用大型Unpack+8.71%/+9.66%，收窄后仍+2.73%/+3.23%，整数Unpack+1.35%/+2.33%；数组或大型PDecode局部正向完整保留。两版均不应用，不设比例门槛；8KiB表未进入生产。辅助文件计时前unused import修复及短控制校准触顶限制明示。前轮库9b2ecfb/GSR1ebd141已提交未推送，正式依赖/十项删除保持，新实验留审查。
