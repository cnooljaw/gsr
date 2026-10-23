# 当前阶段验证清单

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

本清单整理 Core、Cluster、日志及已落地外围示例的验证证据，不替代 API 契约。GSR 自 2026-10-12 起进入 1.0 正式内部发布阶段，当前版本变化与发布验证见 [1.0.1 发布说明](RELEASE-1.0.1.md)，最初的发布范围见 [1.0 内部发布说明](RELEASE-1.0.md)。内部发布不将历史开发机样本扩大为目标部署容量结论。实施路线图保留各轮历史切片；判断当前覆盖范围时先看这里，再进入对应测试。

下文历史记录中的示例 Gate 路径是当时运行测试的位置。当前连接实现统一到 `gate`，原 Debug/Prod 用例保留为 Plain/MAC 模式测试；重跑历史命令使用统一路径，历史结果和耗时不因迁移而改写。

## Go SP/PB当前总览（2026-10-20）

当前总览（2026-10-20）：[Go SP/PB同场五轮](benchmarks/2026-10-20-go-sp-pb/RESULTS.md)使用gosproto af9169d提交归档与PB v1.36.1，共72项360样本；大型packed编码−8.1%、解码+14.9%，int32数组packed编码−36.5%、解码+62.2%。纯Encode/Decode已领先的数值路径被Unpack拖累，保持Body与Packing双主线；Packing侧优先Unpack→PDecode，Body继续定位大型结构成本，不继续以旧大型Encode数据选题。Go-only、无新profile/TS或细小收益统计显著性结论。库工作区有用户删除，采样隔离归档未触碰；正式依赖仍v0.4.0，本轮无提交。

## 生产版阶段成本诊断（2026-10-19，只读）

前轮word预算报告已提交GSR `8e9880f`，未推送。新[生产版阶段成本诊断](benchmarks/2026-10-19-gosproto-unpack-phase-profile/RESULTS.md)只读采af9169d的8个CPU入口：大型纯Unpack预检/恢复占完整样本30.40%/32.16%，大型PDecode中Unpack累计17.67%、正文Decode累计53.88%（嵌套项不相加）。countChunks累计4.31%，inline的未用content视图cap守卫仍在；下一候选为仅计数路径消除不使用的视图，尚未实施。55条流程命令＋1条静态objdump、24快照/1827采样权重、fresh/来源核对通过，无新A/B结论。主库仍af9169d干净，报告未提交，依赖v0.4.0、Decode继续开放。

## 预检word预算（2026-10-19，未采用）

前轮组合实验已提交GSR `885505c`，未推送。新[按word记录预检预算](benchmarks/2026-10-19-gosproto-unpack-word-budget/RESULTS.md)完成固定70项：单/双字节word约快5%～7%、零段小幅改善，但大型Unpack慢6.77%～7.93%、4字节大数组PDecode两轮慢4.02%、FF接稀疏慢约33%～34%，未采用。静态指令384→372但栈帧112→128B，不能据此预测动态收益。159275差分（新增MaxInt附近预算）、目标契约及来源核对通过；控制/尾差/长尾全部保留。主库仍af9169d且干净，新报告未提交，正式依赖v0.4.0、Decode继续开放。

## 分函数与索引预检组合（2026-10-19，未采用）

前轮共享wire/分函数实验已提交 GSR `2ca8864`，未推送。新[分函数＋索引预检组合](benchmarks/2026-10-19-gosproto-unpack-split-indexed/RESULTS.md)完成固定70项：大型PDecode快3.12%/4.38%、4字节大数组快9.84%/9.23%，但普通word两轮慢约4%～33%、FF接稀疏慢约8.5%，组合未消除取舍，未采用。280进程/26,880块、目标契约及138500差分通过，控制/长尾/累计反向全部保留。主库仍af9169d且干净，新报告未提交；依赖v0.4.0，Decode继续开放。

## 共享wire控制与预检分函数（2026-10-19，均未改变主库）

[共享输入诊断](benchmarks/2026-10-19-gosproto-unpack-shared-wire/RESULTS.md)40项160进程/15,360块，160条实际metadata核对相同SHA、对齐0/1、spare cap0/64与A/B共享wire；三种业务纯Unpack/PDecode均更快，普通单/双字节word和FF接稀疏仍更慢。恢复内循环16机器字相同、旧循环地址相差20B；未确认硬件唯一原因，也未改变位置游标未采用状态。self22/40、clone9/40，失败控制/尾差/累计翻转全部保留。193测量命令与来源/fresh/原采样函数/输入/机器字独立核对通过。

[原完整预检分函数](benchmarks/2026-10-19-gosproto-unpack-size-phase/RESULTS.md)只提取旧扫描算法，扫描/恢复源码等价范围和138500差分验证通过。70项280进程/26,880块普通word约快1%～12%，但4字节PDecode基本持平、大型方向不一致、FF接稀疏观察慢24%～26%且clone漂移保留，不支持新增调用，未采用。self33/70、clone26/70，所有对象数中位相同及30字节尾差/26入口累计翻转保留；313命令成功，limiter零。主库18份源码仍af9169d且干净，未重复生产完整验收。前轮报告已提交GSR71d313d，新两个报告未提交，依赖v0.4.0、Decode继续开放。

## Unpack代价诊断与预检输入游标（2026-10-19，未采用）

[旧/当前代价诊断](benchmarks/2026-10-19-gosproto-unpack-cost-profile/RESULTS.md)8份CPU/24快照与来源/fresh指纹通过，未建立退化唯一原因。独立wire统计确认短零段没有普通word、FF接稀疏主要为固定0x11；探针首轮错误目录误扫tmp失败，更正后独立用例通过，正式依赖未变，不作全仓验证声明。

[预检输入游标](benchmarks/2026-10-19-gosproto-unpack-preflight-cursor/RESULTS.md)只改变完整预检位置表示，138,500组截断/预算/error class/len-cap-nil/独占差分、两侧已有契约和70项语义检查通过。280进程/26,880块两轮4字节PDecode快11.85%/9.45%、8字节快8.72%/8.09%，真实8字节Dispatch快11.38%/10.88%；这些收益保留。但普通1..7有效字节word普遍慢4%～33%、FF接稀疏仍慢6%～10%，未采用。无最低比例门槛；self39/70、clone33/70、尾差/累计翻转/GC离群完整保留。主库从未应用候选、全部18份源码仍af9169d且干净，未重复完整生产验收。该轮已提交GSR `71d313d`，未推送；依赖v0.4.0、Decode继续开放。

## 普通Unpack仅源游标（2026-10-19，保留）

先在f7022ac做[四入口CPU诊断](benchmarks/2026-10-19-gosproto-unpack-current-profile/RESULTS.md)，4进程/12快照，来源/fresh/raw指纹核对。后续[源游标候选](benchmarks/2026-10-19-gosproto-unpack-source-cursor/RESULTS.md)只将普通mask的源切片推进移到word外，目标/预检/预算/错误/分配/零/FF不改。70项280进程/26,880块两轮大型PDecode快1.16%/1.42%、4字节数组快4.69%/3.34%、8字节数组快4.16%/4.00%；FF接稀疏约慢11%、短零段第二轮慢14%等代价保留，不设收益比例门槛或按fixture分支修补。

self59/70、clone52/70，失败控制/27字节尾差/17入口累计翻转及全部异常保留，limiter零。8条收尾命令通过：库全测并启用固定官方oracle、race/checkptr、vet、Pack/Codec/Decode各10秒有限fuzz、GSR三包、Go/TS7/7零skip。Pack/Decode fuzz三秒后无新增执行限制保留；实际18份库源等于被测候选，正式依赖文件不变。该轮已本地提交为库 `af9169d`、GSR `6b47822`、不推送；依赖v0.4.0，Decode继续开放。

## 原生整数数组读取视图（2026-10-19，保留）

[数组读取报告](benchmarks/2026-10-19-gosproto-decode-array-view/RESULTS.md)只收窄native []int64两处4/8字节元素视图，预检/预算/符号/分配/回退/独占不变。52项208进程/19,968块，4字节大数组Decode两轮快25.88%/26.84%、PDecode6.86%/8.44%、Dispatch8.77%/8.76%；8字节大Decode28.09%/28.50%。短8字节PDecode数ns代价、8字节组合首轮累计反向、大型游戏无稳定收益，以及self21/52、clone23/52等限制保留。无最低收益比例，全部对象数中位相同。

扩展既有40组数组边界测试；冻结驱动git apply跳过测试补丁，生产补丁正确，后续指纹发现后独立强化基线与候选验收，不改计时归档。修正checked复现入口已prepare/SHA验证。8条收尾命令通过：强化基线、库全测并启用固定官方oracle、race/checkptr、vet、各10秒CodecRoundTrip/Decode fuzz、GSR三包、Go/TS7/7零skip；Decode fuzz三秒后无新增执行限制保留。该轮已本地提交为库 `f7022ac`、GSR `b3d4517`，不推送；正式依赖v0.4.0，Decode继续开放。

## 当前Decode热点与分配诊断（2026-10-19）

后续[当前Decode诊断](benchmarks/2026-10-19-gosproto-decode-current-profile/RESULTS.md)在503a725只读刷新四入口CPU/独立allocation，8进程/24快照，limiter零。大型Decode/PDecode为10416B/532、13872B/533次；整数数组为8288B/4、12384B/5次。CPU/分配范围分离，初始化/kevent/递归累计限制保留，耗时不作新排名。源码与机器码确认native数组逐元素开放尾切片仍有调整，下一候选为固定4/8字节视图并补8字节大数组观察，本轮未实施。harness/vet/40顺序/构建/profile文本/fresh及全部指纹通过，主库18份Go源码等于503a725且干净，未重复生产完整验收。该诊断已本地提交为GSR `16d1491`，未推送；依赖v0.4.0、Decode继续开放。

## Header字节偏移遍历（2026-10-19，未采用）

后续[header字节偏移](benchmarks/2026-10-19-gosproto-decode-header-offset/RESULTS.md)用初始headerEnd与步进2替代index倍增，原字节/次数/预算/错误/独占/写屏障不变，parser660→656条/320B。40项两轮大型PDecode快0.81%/1.03%，但双控制失败且有累计翻转；整数数组PDecode+1.10%/+1.24%双控制通过、含数组记录PDecode+1.68%/+1.57%，整体取舍未采用，无最低收益比例。160进程/15,360块复算，self20/40、clone19/40，尾差/全部异常保留；目标测试/harness/vet/差分/fresh和指纹通过。主库从未应用候选，18份Go源码等于503a725且干净，未重复生产完整验收。该轮报告已本地提交为GSR `9699583`，未推送；库保持503a725，依赖v0.4.0、Decode继续开放。

## 固定两字节header读取视图（2026-10-19）

后续[固定header视图](benchmarks/2026-10-19-gosproto-decode-header-view/RESULTS.md)只收窄readUint16的len/cap为2，原len/预算/错误/独占/全部字段guard不变。parser664→660条、320B不变；计时前两索引初稿证据另存，未计时。原40项两轮half4096 PDecode快2.41%/2.54%且双控制通过，大型PDecode快1.72%/1.89%；保留单行收益并接受短Echo/含数组记录等成本，self21/40、clone13/40及全部翻转/离群不删。160进程/15,360块复算、fresh/全部指纹、库全测/官方/race/checkptr/vet、有限Codec/Decode fuzz432,127/38,718次、GSR三包/GoTS7/7零跳过通过，Decode fuzz后段计数停止如实保留。该轮已本地提交为库 `503a725`、GSR `2104eca`，未推送，依赖v0.4.0、Decode继续开放。

## 原生整数写入资格前移（2026-10-19，未采用）

后续[原生整数资格前移](benchmarks/2026-10-19-gosproto-decode-integer-proof/RESULTS.md)保留全部guard，在独占分配前计算写入资格，返回同一对象用于原生或反射回退。helper仍内联、parser664条/320B不变，重复标记加载2→1；固定40项两轮大型Decode+1.76%/+1.51%，含数组记录Decode+2.31%/+1.22%，主要目标成本超过当前局部收益，未采用，不设最低收益比例。160进程/15,360块复算，self28/40、clone25/40，对象数不变，全部尾差/翻转/异常保留；目标测试/harness/vet/差分/fresh和源码/二进制核对通过。主库未应用候选、保持913c465且干净，未重复生产完整验收。该轮报告已本地提交为GSR `f7381dd`，未推送；库保持913c465，依赖v0.4.0、Decode继续开放。

## 结构数组子schema复用实验（2026-10-19，未采用）

后续[结构数组schema局部复用](benchmarks/2026-10-19-gosproto-decode-array-schema/RESULTS.md)只保存同步调用内子schema链接，208条指令不变、栈帧224→240B。固定40项两轮中，含数组记录Decode快0.92%/1.05%，但大型Decode+1.25%/+0.79%、nil长数组Decode+1.59%/+1.81%，综合取舍未采用，不设最低收益比例。160进程/15,360块复算，self27/40、clone30/40；对象数不变，字节尾差/累计翻转/全部离群保留。目标契约/harness/vet/差分/fresh与指纹通过；主库从未应用补丁，源码等于913c465、工作树干净，未重复生产完整验收。该轮报告已本地提交为GSR `26a5748`，未推送；库保持913c465，依赖v0.4.0、Decode继续开放。

## Decode解冻与入口复核（2026-10-19）

用户于2026-10-19明确解除Decode阶段性冻结，入口、解析和构造/写入可继续按证据优化。当前[入口复核](benchmarks/2026-10-19-gosproto-decode-resume/RESULTS.md)保留即时类型证明与地址借用分离：大型Decode两轮快2.37%/2.02%、nil4096 Decode快6.19%/7.06%，大型通知Dispatch小成本/PDecode控制失败及累计翻转完整保留。新40项160进程/15,360块独立复算、fresh与来源核对，库全测/官方/race/checkptr/vet、有限Codec/Decode fuzz497,350/1,217次、GSR三包及GoTS7/7零跳过通过；Decode fuzz后段计数停止保留原输出，未重复PB/TS排名。该轮已本地提交为库 `913c465`、GSR `b708108`，未推送，正式依赖仍v0.4.0，Decode继续开放。

## 非inline内容视图复用（2026-10-19）

库2ea804e/GSRb004da7上只复用记录非inline内容切片，源码等于冻结候选；216条指令/32B帧不变，panic调用点15→14，inline新增两条清零指令。复用已提交记录边界等契约、109形态525差分，无新测试；原70项四组280进程/26,880块/6,720 quartet/1,680轮/1,120快照独立复算，fresh/全部指纹通过。self50/70、clone56/70，limiter零，分配中位数69/69/69/68相同；尾差/27项累计翻转/全部离群保留。harness/vet、库全测/官方C-Lua/race+checkptr/vet、10秒Codec fuzz512,102次（5个新增interesting，无失败）、GSR三包/GoTS7/7零跳过通过。见[内容视图报告](benchmarks/2026-10-19-gosproto-record-body-view/RESULTS.md)。保留小幅收益并接受full记录请求等成本，该轮已本地提交为库 `e50dfef`、GSR `29c954b`，未推送，正式依赖不变；无失败准备/计时重试，不重复未改Pack随机fuzz。

## inline整数延后内容视图（2026-10-19）

库9067933/GSRca02105上只改私有记录写出的inline分支，生产源码等于冻结候选；212→216条指令、32B帧、panic调用点14→15。既有数组用例补10个inline/宽度边界，基线通过；负向inline上限+1新用例在32767失败、旧用例通过。原70项四组280进程/26,880块/6,720 quartet/1,680轮/1,120快照独立复算，fresh/全部指纹通过。self60/70、clone64/70、limiter零、分配中位数68/69/70/69相同；16B尾差、28项累计翻转及离群完整保留。既有契约/109形态525差分、harness/vet、库全测/官方C-Lua/race+checkptr/vet、10秒Codec fuzz470,086次、GSR三包/GoTS7/7零跳过通过。见[inline内容视图报告](benchmarks/2026-10-19-gosproto-record-inline/RESULTS.md)。保留大型游戏路径收益并接受混合长记录等局部成本；该轮已本地提交为库 `2ea804e`、GSR `b004da7`，未推送，Decode/正式依赖不变；无计时重试，不重复未改Pack的随机fuzz。

## 整数记录固定头部视图（2026-10-19）

库d2f6027/GSR7c91913上只改记录写出两处固定len/cap2视图，gofmt无变化且源码等于采样候选。216→212条指令、32B帧/14个panic调用点不变；原70项四组280进程/26,880块/6,720 quartet/1,680轮/1,120快照复算，fresh/全部指纹通过。self63/70、clone62/70，limiter均零，分配中位数70/69/68/69相同，尾差/27项累计翻转/全部离群保留。既有记录/tag/nil/元信息/预算/RPC与109形态525差分、harness/vet、库全测/官方C-Lua/race+checkptr/vet、10秒Codec fuzz436,240次、GSR三包/GoTS7/7零跳过通过。计时前一次before测试模块导入失败，独立modfile修正，零计时/失败命令完整记录；无计时重试。见[记录头部视图报告](benchmarks/2026-10-19-gosproto-record-header/RESULTS.md)。保留小幅收益并披露局部成本；该轮已本地提交为库 `9067933`、GSR `ca02105`，未推送，Decode/正式依赖不变；未重测未改Pack的随机fuzz。

## 当前编码单项进程profile（2026-10-19）

已提交库d2f6027/GSR175028b上只读诊断：16入口各CPU2秒和逐次分配20,000次，32进程/96快照；limiter均零，最大堆快照3.85MiB（非峰值）。构造原70项后只保活单项，CPU启动在GC/预热后；内存profile包含初始化/构造/预热，循环B/op为主。大型记录与整数Pack热点、4KiB kevent归因限制完整保留，不形成新A/B或PB/TS排名。harness统计/选择/GC/夹具/指标/70顺序、vet、构建、32采集、96摘要+2源码视图及fresh归档/源/二进制/profile独立核对通过。见[当前热点报告](benchmarks/2026-10-19-gosproto-current-profile/RESULTS.md)。生产未改/库干净，不重复生产全测/oracle/race/fuzz/GSR/GoTS；该轮已本地提交为GSR `7c91913`，未推送，Decode/正式依赖不变。

## Pack固定len/cap视图（2026-10-19）

前轮已提交库0b38019/GSR31b3f99。两份未计时草稿和唯一实测三索引版本均通过既有契约/17,984差分；原70项四组280进程/26,880块/6,720 quartet/1,680轮/1,120快照复算。self46/70、clone51/70，limiter全零；分配中位数70/69/68/68项相同，16B尾差保留。整数数组PEncode/真实通知/请求两轮改善，保留；ASCII/大记录成本、累计方向翻转、44.994/206.939/227.795ms等异常块完整披露。harness/vet/fresh、源/工具/二进制指纹通过；库全测/官方C-Lua/race+checkptr/vet、有限fuzz（Pack5,487且3s后停增；Codec416,938）、GSR三包/GoTS7/7零跳过通过。见[固定len/cap报告](benchmarks/2026-10-19-gosproto-pack-window/RESULTS.md)。该轮已本地提交为库 `d2f6027`、GSR `175028b`，未推送，正式依赖/Decode不变。

## Pack固定payload视图（2026-10-19）

前轮实验已提交GSR d1dceaa。新候选仅普通word安全固定视图消除重复索引+1，296→288条符号指令、112B帧不变；原70项四组280进程/26,880块/6,720 quartet/1,680轮/1,120快照独立复算。self/clone56/70，所有limiter零；分配中位数69/69/69/70项相同，16B尾差披露。两轮目标Pack/PEncode/RPC改善，保留；累计方向翻转、233.678ms异常块、wide64成本与全部失败保留。既有工作区/全mask等契约、17,984差分、harness/vet、fresh归档/源/二进制SHA通过。库全测/官方C-Lua/race+checkptr/vet、有限fuzz（Pack36,026且3s后停增；Codec442,757）、GSR三包/GoTS7/7零跳过通过；oracle复用历史SHA一致的官方core/facade，parser来自只读Skynet并单列。见[固定payload报告](benchmarks/2026-10-19-gosproto-pack-payload/RESULTS.md)。该轮已本地提交为库 `0b38019`、GSR `31b3f99`，未推送，正式依赖/Decode不变。

## 普通word连续段Pack实验（2026-10-19）

前轮诊断已提交GSR4ff411f。只在归档实验连续段一次word写出；基线/候选既有Pack/尾块/FF/重叠/cap/独占/分配/错误测试及17,984差分通过。原70项四组280进程、26,880块/6,720 quartet/1,680轮/1,120快照独立复算；harness/vet、每项与合并JSON、fresh归档/源/二进制SHA一致。self13/70、clone42/70，均无limiter变化；self分配69/70一致（单项16B尾差保留），clone和两轮比较70/70一致。两轮目标Pack及主要RPC回退，候选不应用；nil4096小收益、全部失败/离群/累计差完整保存。见[连续段报告](benchmarks/2026-10-19-gosproto-pack-contiguous/RESULTS.md)。库干净且仍7397804，不跑未保留候选的发布级官方/race/checkptr/fuzz/GSR/GoTS；Decode/正式依赖不变；该轮已本地提交为 GSR `d1dceaa`，未推送。

## 原70项逐项独立进程控制（2026-10-19）

以库7397804 / GSR a125ab4为只读基线，固定140个进程一次采样；self/clone均61/70控制通过，70/70分配中位数相同。全部13,440块、3,360 quartet、840轮、560状态快照独立复算，140逐项与合并JSON对齐；limiter无非零/变化，最大堆快照约4.20MB（非峰值）。旧混合控制40/70、38/70仅并列参考，17个不同入口至少一组失败与最大单轮+71.34%/+33.02%完整保留，无重跑/扣噪声。原计时块/校准源码相同，harness统计/选择/GC/fixture/指标检查与vet、fresh归档/源/二进制SHA通过。生产/正式依赖/Decode未改，不重复官方/race/checkptr/fuzz/GSR/GoTS。见[逐项进程报告](benchmarks/2026-10-19-gosproto-isolated-cases/RESULTS.md)，该轮已本地提交为 GSR `4ff411f`，未推送；库干净，后续只实验普通word Pack。

## 当前编码profile与4KiB GC上下文（2026-10-18）

前一切片已提交为库7397804、GSR 25b0b0d，未推送。只读采26份profile；固定4个同函数A/A条件共384块/96quartet/24轮复算至1e−12一致，24个byte-pattern计数和2×97个limiter点核对。full Pack的A/A−3.35%/IQR11.76pp、隔离−0.72%/1.04pp；后者仍有−24.5%单轮，zero frame隔离未通过，原值/尾差/失败全保留。原full堆峰637MB；另起无计时事件诊断确认limiter full0→1279、isolated0，独立峰值459/13MB，不回填旧数据。harness统计/选择/fixture/vet、补充诊断/vet、源/二进制/26profile SHA与fresh归档通过；库所有Go源码等于7397804、工作树干净，无生产改动，不重复官方/race/checkptr/fuzz/GSR/GoTS。见 [诊断报告](benchmarks/2026-10-18-gosproto-post-layout-profile/RESULTS.md)，该轮已本地提交为 GSR `a125ab4`，未推送，正式依赖/Decode不变。 2026-10-19收尾发现临时库HEAD/config缺失，先备份再恢复最小元数据；完整fsck/源码SHA/status通过，全refs bundle已保存于稳定工作区忽略目录，未推送，详见报告恢复记录。

## 编码布局连续索引写入（2026-10-18）

前一增长切片已提交为库bdfe1d0、GSR bf828b0，未推送。只在add复用连续逻辑索引满载判断和安全写入，容量序列不变。固定原70项/四组26880块独立复算通过；大型Encode快2.94%～4.67%、PEncode2.52%～3.19%、实际请求3.13%～3.48%，70项分配中位数相同，保留候选。草稿内联81、固定max规则76的计时前记录保留；整数记录预检帧144→112B，通用帧272B不变，安全检查未删。self40/70、clone38/70，int32/手工帧成本、未改纯Pack波动、累计方向不同及大块全保留。既有回归、109形态525项wire/cap/独占/错误差分、harness/vet/fresh归档、官方SHA、库/race/checkptr/vet、有限Pack9053/Codec475186次fuzz、GSR三包/GoTS7/7零跳过通过。Pack计数3s后停止、覆盖有限。见 [连续索引报告](benchmarks/2026-10-18-gosproto-layout-index/RESULTS.md)；该轮已本地提交为库7397804、GSR 25b0b0d，未推送，Decode与正式依赖不变。

## 编码布局按需倍增（2026-10-18）

前一诊断已提交为GSR c201237，库基线a161158，未推送。只改外溢满载增长，固定70项四组/26880块独立复算通过；4096个含整数数组记录的实际请求快8.54%/8.89%，230784B/12次→155136B/8次。32×32纯Encode慢2.89%/6.29%且多3456B，常规追加/小中型成本和两个helper帧各增16B完整披露；self34/70、clone45/70，控制失败/累计方向不同/非目标波动不删，不设最低收益比例，保留候选。扩展既有测试先原后新通过；109形态525项wire/cap/独占/预算差分、harness/vet、fresh归档指纹、官方源码SHA、库全测/race/checkptr/vet、有限Pack6822/Codec483726次fuzz、GSR三包/GoTS7/7零跳过通过。Pack fuzz3s后计数停增，限制保留。见 [按需增长报告](benchmarks/2026-10-18-gosproto-layout-growth/RESULTS.md)；该轮已本地提交为库bdfe1d0、GSR bf828b0，未推送，Decode和正式依赖不变。

## 编码布局项数与增长诊断（2026-10-18）

前缀优化已提交为库a161158、GSR d6c672c，未推送。只读诊断109种形态、317入口×128次分配观测；增长模型48项数×4策略×128次。game_large共131项，外溢99项512B一次分配；初始64反而768B/2次，精确容量模型仅省96B。长数组按需倍增模型有收益也有1057/4129项代价，未测CPU或采用生产候选；预算早期零分配失败保持可见。诊断/模型检查与vet、既有fixture、独立复算、fresh归档指纹核对通过，无分配尾差。库所有Go源码与a161158一致、工作树干净；无生产修改，不重复官方/race/checkptr/fuzz/集成。见 [布局增长诊断](benchmarks/2026-10-18-gosproto-layout-audit/RESULTS.md)，该诊断已本地提交为GSR c201237，库保持a161158，未推送，Decode及正式依赖不变。

## Pack 主循环密集前缀写出（2026-10-18）

上一轮记录已提交为GSR 13d4609，提交后两库干净、未推送。核对历史前缀源码/二进制后，仅将当前Pack密集word串行index改为字节前缀和，写入位置/顺序、FF/稀疏/尾部、offset及重叠/cap/独占不变。ARM64汇编301→297行，栈帧112B、检查调用点7/6不变，生产净减6行。固定原62项两轮：大型Pack快8.6%～9.5%、PEncode1.9%～4.6%、三种RPC2.7%～5.0%；整数数组Pack快7.2%～9.1%、PEncode4.9%～5.8%、RPC4.6%～6.3%，保留候选。小型稀疏约1～4ns代价、未改路径+21ns、累计方向不同、self52/62/clone46/62失败及0.125B尾差均保留。相关回归/17984差分、库/官方/race/checkptr/vet、有限Pack3995与Codec478204次fuzz、GSR三包/GoTS7/7通过；Pack计数停增的覆盖限制披露。见 [主循环前缀报告](benchmarks/2026-10-18-gosproto-pack-prefix-main/RESULTS.md)，已本地提交为库a161158、GSR d6c672c，未推送，正式依赖和Decode不变。

## Pack 剩余目标游标实验（2026-10-18）

上一轮诊断已提交为GSR 17996e0，提交后两库干净，未推送。只改packInto目标游标：推进剩余dst并从原packed起点返回，保留cap及FF/尾部/重叠规则。BCE总检查点未减少，ARM64汇编301→309行、栈帧112B不变；既有相关回归及17984组wire/cap/独占差分通过。固定原43项+原19形态，23808块独立复算一致。大型纯Pack+1.33%/+1.68%，大型和整数PEncode方向翻转，四/六非零长word约慢0.1μs；部分RPC/零场景改善与控制限制完整保留，综合撤回候选，不设最小收益比例。self49/62、clone38/62、累计方向不同/离群/字节尾差保留；最终全部Go源码与a3b3830相同、库干净，未重复未变生产的全验收。见 [Pack游标实验](benchmarks/2026-10-18-gosproto-pack-cursor/RESULTS.md)，该实验记录已本地提交为GSR 13d4609，库保持a3b3830，未推送；Decode及正式依赖不变。

## 指针入口优化后的热点诊断（2026-10-18）

上一轮记录已本地提交为GSR a391e42，提交后两库干净，未推送。从已提交a3b3830归档刷新四种输入的纯Encode/EncodePacked/RequestEncode+CancelSession，CPU各2秒、逐次分配另起进程各20000次，共24份profile；不作为A/B或PB/TS排名。整数数组Pack约34%/39%，大型记录预检/写出与Pack仍各自可见；父子累计不相加。大型三个入口3968B/2次，其中512B布局外溢，其余输入一次结果分配。4KiB ASCII及整数数组纯Encode的kevent/GC栈占比保留，不归因协议或据此排名。harness检查/vet/构建、选择/指纹及归档复现通过，库保持a3b3830干净，未重复生产全验收。见 [优化后热点报告](benchmarks/2026-10-18-gosproto-post-pointer-profile/RESULTS.md)，该诊断已本地提交为GSR 17996e0，未推送；下一轮先检查普通word Pack游标/BCE，再做单一候选，Decode和正式依赖不变。

## nil 调用分支前移实验（2026-10-18）

上一轮已提交为gosproto a3b3830 / GSR f37fef0，提交后两库干净，未推送。仅在两个整数记录数组调用点用IsNil跳过nil指针提取，helper仍处理布局/预算，固定43项及8项nil补充、各两轮。全nil256记录约快322～426ns，但大型Encode回退8.85%～9.09%（519～538ns）、PEncode4.15%～4.84%、RPC3.46%～5.46%，混合数组也回退，故撤回候选，不以最低百分比裁决。未进入该分支的int32纯Encode也回退约13%；内联4字节核心15条指令相同、具体原因未定，完整保留。主控制35/43、37/43与nil5/8、6/8、累计方向不同和字节尾差均披露。相关基线/候选测试、独立复算/编译/归档核对通过；最终所有Go源码与a3b3830相同、库工作树干净，没有重复未变生产的全验收。见 [nil分支报告](benchmarks/2026-10-18-gosproto-record-nil-branch/RESULTS.md)，该实验记录已本地提交为GSR a391e42，库保持a3b3830，未推送；Decode及正式依赖不变。

## 整数记录指针入口（2026-10-18）

上一轮已本地提交为gosproto bd1976a / GSR 4bf1b32，提交后两库干净，未推送。整数记录指针入口改用已验证数组分支借用现有encodePointer，helper避免三字反射值与重复nil类型分支，生产净减3行，不扩大unsafe或改变预算/wire/所有权。固定43项两轮中，大型Encode约快7.9%～8.3%、PEncode快5.6%～7.3%、三种RPC快5.2%～7.2%，小型也改善，目标分配中位数相同。补充8项nil矩阵确认全nil256记录约慢193～217ns（4%～5%），混合数组更快；明确接受该局部代价，保留候选。主self35/43、clone34/43及全部失败/累计方向相反/离群块保留，第二轮未改int32请求+4.74%未解释；旧Pack+9.45%仍不覆盖。库/官方/race/checkptr/vet、441809次有限Codec fuzz、GSR三包及Go/TS7/7通过。见 [指针入口报告](benchmarks/2026-10-18-gosproto-record-pointer/RESULTS.md)，该切片已本地提交为gosproto a3b3830 / GSR f37fef0，未推送；Decode和正式依赖继续冻结。

## 整数记录 Fields 局部复用实验（2026-10-18）

上一切片已提交为gosproto 0c1f6cf / GSR 22eb772，提交后两库干净、未推送。后续完成Fields局部复用实验：两个记录helper各捕获一次切片头，编译器确实把两次每字段描述加载移出循环。固定43项两轮中，大型纯Encode约快0.75%/1.17%，与控制偏差同阶；PEncode持平、RPC方向混合，未确认业务组合收益，生产候选撤回。保留乱序Go声明/忽略字段的预算tag回归；错误使用声明顺序时，新夹具失败、旧夹具仍通过，覆盖增量明确。最终生产代码与0c1f6cf一致，相关基线/候选测试、固定采样及归档核对通过，未重复生产全验收，见 [Fields实验](benchmarks/2026-10-18-gosproto-record-fields/RESULTS.md)。测试与记录已本地提交为gosproto bd1976a / GSR 4bf1b32，未推送；下一步单独检查记录入口反射值/指针借用成本，不扩大unsafe范围，Decode和正式依赖不变。

## 原生整数数组连续游标（2026-10-18）

上一轮诊断已提交为GSR d2be5d8，提交后两库干净，未推送。后续只调整原生[]int64的4/8字节输出游标，保留预检/宽度/预算/wire/cap/独占。编译器可消除每元素重复切片检查；43项两轮中，32/64位纯Encode约快25%/23%，PEncode约快14%/8%，大型约快3%，实际RPC也改善。对象数中位数不变，两个手工帧±0.125 B/op尾差、小包约5～8 ns及未改纯Pack第二轮+9.45%（245 ns）均披露；Pack各臂两轮符号指令一致，原因仍未确定。库/官方/race/checkptr/vet、474047次有限Codec fuzz、GSR三包和Go/TS 7/7通过；40组精确wire及故意错步长负向回归保护游标。见 [连续游标报告](benchmarks/2026-10-18-gosproto-array-cursor/RESULTS.md)，该切片已本地提交为gosproto 0c1f6cf / GSR 22eb772，未推送；正式依赖与Decode不变，不新增PB/TS排名。

## 融合后编码热点诊断（2026-10-18）

有响应请求切片已本地提交为 gosproto 6d854d1 / GSR f715e6b，未推送。随后只刷新四种既有消息的body EncodePacked与真实RequestEncode+CancelSession八条profile，CPU与逐次内存采样分开；大型消息集中在整数记录预检/写出，整数数组集中在Pack和连续写出，大型仍有512 B布局外溢。4 KiB ASCII的kevent/GC归因明显，完整保留而不据此排名。独立harness夹具/统计/选择/vet、16次采样及归档/指纹核对通过；库源码干净，未重复生产验收，见 [融合后热点报告](benchmarks/2026-10-18-gosproto-post-fusion-profile/RESULTS.md)。下一轮先单独实验整数数组写出，预算/wire/cap/独占不变；该诊断已本地提交为GSR d2be5d8，库保持6d854d1，未推送；正式依赖与Decode继续冻结。

## RPC 有响应请求单缓冲（2026-10-18）

上一切片已提交为 gosproto 0988af0 / GSR 9876a5b，提交后两库干净、未推送。后续融合声明Request且有Response的请求：锁外完成body预检和写出，原session接纳后Pack，重复session仍返回限长限容raw body。五项请求两轮均少一次分配，大型约快5.8%～6.2%、整数数组约快9.5%～11.6%、ASCII4KiB约快41%～42%。共用writer成本（ASCII纯Encode约+21 ns）、失败workspace分配（32 B invalid/full多16 B，MaxInt32重复缓冲级别多32 B）及self17/19、clone15/19控制失败全部披露。扩展504组重复body/cap/独占回归，库/官方/race/checkptr/vet、固定RPC差分、有限fuzz、GSR三包和Go/TS 7/7通过，源码/归档一致，见 [请求工作区报告](benchmarks/2026-10-18-gosproto-request-workspace/RESULTS.md)。该切片已本地提交为 gosproto 6d854d1 / GSR f715e6b，未推送；正式依赖与Decode/Pack/Unpack/TS不变；后续先重采profile再决定热点，不新增PB/TS排名。

## RPC 通知单缓冲（2026-10-18）

上一切片已提交为 gosproto b626637 / GSR b2825cf，提交后两库干净、未推送。后续仅融合有Request、无Response的通知，复用上一轮私有前缀写出，省一次body分配和复制；有响应请求的校验、锁、session登记和重复错误raw body返回保持。9项通知两轮均改善：大型约快6.5%～7.3%、整数数组约快11.7%～13.5%、ASCII4KiB约快43%～45%；10项非目标分配不变，整数数组请求约14～20 ns代价与self13/19、clone7/19控制失败完整披露。库/官方/race/checkptr/vet、3078固定RPC差分、GSR三包和Go/TS 7/7、零跳过通过，源码/归档一致，见 [通知工作区报告](benchmarks/2026-10-18-gosproto-notification-workspace/RESULTS.md)。该切片已本地提交为 gosproto 0988af0 / GSR 9876a5b，未推送；正式依赖/Decode/Pack/Unpack/TS不变，未重新测PB/TS排名。

## RPC 响应单缓冲（2026-10-18）

上一切片已提交为 gosproto f9de3d2 / GSR dbb681f，提交后两库干净、未推送。后续仅将声明body的ResponseEncode写入同一Pack工作区，省一次中间body分配/复制；普通调用传nil前缀，请求方法/登记/锁/重复错误body返回不变。相对分配回归旧2对1失败，新普通及插桩构建通过；补3个typed nil响应ErrNil/nil输出，旧实现也通过，错误的空结构测试预期与修正保留。

四次19项RPC加一次原19项body/Pack/手工帧，共9120块独立复算一致；五项响应两轮均改善，大型约快6%～8%、整数数组约快13%～15%、ASCII4KiB约快43%～44%，各少一次分配。请求/通知分配不变，AddressBook通知+1.30%/+0.98%（9/7 ns）代价、nil-prefix普通项−2.86%～+1.33%波动与控制17/19、16/19全部公开。最终库全测/固定官方/race/checkptr/vet、Pack/Codec有限fuzz（136238/220094次）、3078固定RPC差分/12312帧、GSR三包和Go/TS Plain/MAC 7/7、零跳过通过，源码与归档一致，见 [响应工作区报告](benchmarks/2026-10-18-gosproto-response-workspace/RESULTS.md)。该切片已本地提交为 gosproto b626637 / GSR b2825cf，未推送；正式依赖/Decode/Pack/Unpack/TS不变；未运行GSR全仓/race、跨平台或新PB/TS性能矩阵。

## RPC 固定私有 header（2026-10-17）

上一切片已本地提交为 gosproto 69cae73 / GSR 9ecc811，提交后两库干净、未推送。后续按独立CPU/分配profile，只对私有两int32头使用22 B调用内栈缓冲，复用整数/tag写出，保留预算与session/锁/错误顺序；普通body与Decode/Pack/Unpack/TS不变。首版请求多一次分配的失败及修正保留，最终平坦RPC三条路径均只承担body/Pack两缓冲。

固定四次19项配对共7296块独立复算一致；两轮19项均更快，32 B通知约快37%/38%、AddressBook约快14%～17%、大型约快2%～4%，通知少一次、请求/响应少两次分配。控制14/19、11/19及逐轮/GC/分配尾差完整公开。1008组API边界/2016输出、39个官方C/Lua头对照、3078固定种子/12312帧旧新差分通过；初次随机fuzz仅58次及profile采样干扰不隐藏、不冒充足量随机覆盖。最终库全测/官方/race/checkptr/vet、GSR三包与Go/TS Plain/MAC 7/7、零跳过通过，源码/归档一致，见 [RPC header报告](benchmarks/2026-10-17-gosproto-rpc-profile/RESULTS.md)。该切片已提交为 gosproto f9de3d2 / GSR dbb681f，未推送；正式依赖仍v0.4.0；未重复GSR全仓/race、跨平台或PB/TS性能矩阵。

## RPC 通知 session 逃逸（2026-10-17）

上一切片已本地提交为 gosproto dc4948d / GSR c4622bf，提交后两库干净、未推送。后续仅把 session 副本放入有响应分支；扩展既有相对分配回归，旧实现4对4失败，新实现通知少一次分配。既有RPC wire/cap/独占、预算、错误顺序、session隔离与清理用例通过。

四次固定19项真实RPC配对共7296块，独立复算一致。9种通知两轮全部少一次逻辑分配、约4 B/op；32 B通知约快2.7%/3.4%，AddressBook约快1.9%/2.1%。大型通知约慢0.7%/0.5%（68/48 ns）、ASCII请求约慢1.1%/1.2%（18/20 ns），控制14/19、10/19及全部原始失败标记保留；按分配收益、小包改善及低复杂度保留，见 [session逃逸报告](benchmarks/2026-10-17-gosproto-rpc-session/RESULTS.md)。库全测/固定官方C-Lua/race/checkptr/vet、GSR三包及Go/TS Plain/MAC 7/7、零跳过通过；最终源码与采样及归档重建一致。该切片已提交为 gosproto 69cae73 / GSR 9ecc811，未推送；当时正式依赖、Decode/body/Pack/Unpack/TS不变；未重复fuzz、GSR全仓/race或PB/TS性能。

## RPC 编码组帧工作区（2026-10-17）

上一切片已本地提交为 gosproto 56f5c21 / GSR 0f0f779，提交后两库干净、未推送。后续 RPC 编码保持 body 校验和 session 登记/锁范围，省去 header 字节与 Append 拼接缓冲。864 组 header/角色/长度、1728 输出的精确 wire/cap/独占，以及独立预算、错误优先级、重复 session 输出与相对分配回归通过；分配用例在旧实现按预期失败。

真实 RPC 19 项两轮全部更快、少两次分配，大型消息约快 8%～10%、整数数组约快 18%～20%、4 KiB ASCII 约快 38%。self/clone 13/19、14/19 总控制通过，失败与波动完整保存；加原未改路径对照共 9120 块，独立复算一致。最终库全测、race/checkptr、固定官方 C/Lua、vet、Pack/Codec 有限 fuzz（143169/230271 次）、GSR 三包及 Go/TS Plain/MAC 7/7、零跳过通过，归档重建/最终源码一致，见 [RPC 工作区报告](benchmarks/2026-10-17-gosproto-rpc-workspace/RESULTS.md)。该切片已提交为 gosproto dc4948d / GSR c4622bf，未推送；当时正式依赖、Decode/Unpack/TS 不变；未跑 GSR 全仓/race 或新 PB/TS 性能矩阵。

## EncodePacked 私有共享工作区（2026-10-17）

上一切片已提交为 gosproto d1ab622 / GSR d3b3ef4，提交后干净、未推送。后续私有单缓冲实现保留到工作树；新增组合 wire/cap/独占/错误及长 FF 边界，移除间隔的负向检查在 65535 B 失败，减少一次分配的相对用例在旧实现仍失败。首次插桩下的绝对分配断言失败已记录，修正为同构建相对检查后验收通过，没有生产代码修改掩盖测试失败。

固定四次原 19 项配对共 7296 块，独立复算一致；大型 PEncode 约快 4.8%～5.5%、整数数组约快 9.5%～9.7%、ASCII 4 KiB 约快 40%，均少一次字节分配。32 B 手工帧约慢 7～9 ns，完整帧不能套用 body 组合降幅。最终普通库、race/checkptr、固定官方 C/Lua、vet、Pack/Codec 有限 fuzz（44243/228585 次）、GSR 三包及 Go/TS Plain/MAC 7/7、零跳过通过，见 [工作区记录](benchmarks/2026-10-17-gosproto-packed-workspace/RESULTS.md)。该切片已提交为 gosproto 56f5c21 / GSR 0f0f779，未推送；当时正式依赖、Decode/Unpack/RPC/TS 不变；无 GSR 全仓/race、跨平台运行或新 TS/PB 性能结论。

## Pack 小幅收益复评与组合保留（2026-10-17）

按用户裁决取消统一最低收益比例。稀疏展开和 FF 段分离完成两轮各 19 项业务复测、前后 self/clone、各 19 种固定形态检查，共 23712 块；保留组合到独立库工作树。大型 PEncode 两轮约 −3.72%/−4.24%、整数数组 −2.83%/−3.04%、零字节帧约 −20%。四非零字节合成形态 +3.33% 的代价和控制波动完整披露，不将历史布尔门槛作为自动否决，见 [复评报告](benchmarks/2026-10-17-gosproto-pack-reassessment/RESULTS.md)。

三个候选既有 Pack 用例和各 17984 组含 cap/独占检查的差分通过；最终工作树库完整 race/checkptr/官方 C/Lua、vet、Pack/Codec 有限 fuzz（615/261602 次）、GSR 三包及 Go/TS Plain/MAC 7/7、零跳过通过。所有原始块独立复算，归档重建一致。未重复 GSR 全仓/race、跨平台运行或 TS 性能；新代码/文档未提交，正式依赖和 Decode 阶段基线不变。

## Pack FF 段集中处理候选（2026-10-17，旧标准下的记录）

上一轮已本地提交为 GSR `b2b3dd7`，未推送。隔离候选将 FF 段扫描/写出从普通 word 循环分离；既有 Pack/零段/FF 尾块/解包预算用例、17984 组含 cap 相等断言的差分及业务夹具通过。

固定六项 self/clone/compare 各一次，共 1728 个块。大型 Pack/PEncode −4.59%/−3.99%，整数数组 −2.16%/−1.02%；Pack 收益不足且控制未全通过，候选撤回，未扩展正式验收或追加变体。三个 prepare-only 重建精确匹配来源，正式库干净且保持 bba3326，Decode 冻结、依赖不变。新记录未提交，未重复完整库/官方/fuzz/GSR/GoTS/race，详见 [FF 段实验](benchmarks/2026-10-17-gosproto-pack-runs/RESULTS.md)。

## 墙钟与进程 CPU 隔离诊断（2026-10-17）

上一轮已本地提交为 GSR `3a88564`，未推送。三项同函数 A/A 各保留 96 个块；墙钟计时均未通过原计时门槛，进程 CPU 均满足相同数值要求，支持存在明显执行之外等待，不能归因到具体调度因素。CPU 不替换墙钟门槛，未测 clone/分配，不称为总控制通过；正式库干净且保持 `bba3326`，Decode 继续阶段性冻结。

CPU 换算用例由失败到通过，CPU 计数、原配对统计/筛选/GC 观测和固定夹具用例、go vet 通过；全部 288 块、72 组与 18 个配对轮次独立复算一致。没有生产代码改动或重复完整库/官方/fuzz/GSR/GoTS/race 验收，正式依赖不变。该轮已本地提交为 GSR `b2b3dd7`，未推送，见 [CPU 诊断](benchmarks/2026-10-17-gosproto-pair-cpu/RESULTS.md)。

## GC 边界与聚焦校准（2026-10-17）

上一轮已提交为 GSR `f064584`，未推送。固定 128 次 6 B 分配的三轮探针验证：仅将 GC 放在奇数分配边界，就可多计 16 B，空窗口为 0 B。结合固定 Go 源码解释微小计数差值，未放宽分配门槛。

新增精确筛选、块前后完成 GC 观测及计时状态单列；40 ms 单次实验只测 10 个未通过项加两个 Pack 参照。self 总控制 1/12、clone 4/12，双控制仅一项，不能称为更稳定的新基线；不重试到通过、丢弃 GC 块或与旧通过项合并。正式库干净并保持 `bba3326`，Decode 继续阶段性冻结。

Go 选择/GC 观测/原统计与固定夹具、八个 Python 用例、go vet 和两份归档复现通过，共 2304 个计时块与 18 个分配探针窗口，见 [诊断记录](benchmarks/2026-10-17-gosproto-pair-resolution/RESULTS.md)。该轮已本地提交为 GSR `3a88564`，未推送；未比较候选，也未重复完整库/race/checkptr/官方/fuzz、GSR/GoTS 或 TS 性能验收；正式依赖不变。

## Go 配对采样工具校准（2026-10-17）

上一轮诊断已提交为 GSR `8f0003f`，未推送。新增归档中的同函数/同源码 A/A 和交错配对采样：self 11/19、clone 14/19 通过，9 项同时通过两类控制，大型/整数数组六项均在其中；未通过项保留全部轮次与分配原值，不用于确认小幅收益。

原稀疏写出候选复测小型 PEncode -3.04%，整数数组 Pack/PEncode -4.77%/-2.65%；大型 Pack 自身离散大。原 5%/2% 收益门槛仍未达到，未采用，不宣布新的 PB/SP 排名。正式库工作树干净，保持 `bba3326`，Decode 继续阶段性冻结。

三个模式的统计/固定夹具用例、四个 Python 判定用例、错误归一化副本的预期失败、go vet 和两个归档复现通过。共 342 个配对轮次、5472 个计时块，方法与限制见 [校准记录](benchmarks/2026-10-17-gosproto-pair-calibration/RESULTS.md)。该轮工具/记录已本地提交为 GSR `f064584`，未推送，正式依赖未改；未重复完整库/race/checkptr/官方/fuzz、GSR/GoTS 或 TS 性能验收。

## 稀疏 word Pack 与 A/A 诊断（2026-10-17）

上一轮已本地提交为 gosproto `bba3326` / GSR `a27ec4e`，未推送。只展开已确认至多两个非零字节的提取循环；191 项三轮中目标 Pack 约快 6%，小型 PEncode 却 +11%，未通过预设门槛，未采用。另采四项 CPU profile，保留 kevent 干扰与源码行归因限制。

另做 59 项三轮 A/A，两份 Go 源码完全相同，出现大型纯 Encode +19%、整数数组 Pack -23% 的差值。两份 Pack 的 640 条指令在解析位置差异后相同；尚未区分环境、顺序和代码位置等因素，不作为算法收益或替候选过门的理由。后续先校准测量。

候选现有精确 wire 测试、候选/A/A 归档副本的源码/harness/夹具核对通过，共保留 750 个配对样本；正式库工作树干净，生产代码未改，Decode 继续阶段性冻结。未重复完整库/race/checkptr/官方/fuzz、GSR/GoTS 或 TS 性能验收。详情见 [诊断记录](benchmarks/2026-10-17-gosproto-pack-sparse/RESULTS.md)。该轮记录已本地提交为 GSR `8f0003f`，未推送，正式依赖仍为 v0.4.0。

## 连续零 word Pack 候选筛选（2026-10-17）

上一轮已本地提交为 gosproto `faa6a21` / GSR `3626b3d`，未推送。后续三个零段候选各 191 项三轮，共 1719 个样本，零字节帧约 -34%～-40%，但 ASCII、数值组合或短包出现代价，全部撤回；未进入五轮验收，没有新的 PB/SP 排名。正式生产 Go 文件保持 `faa6a21`，Decode 继续阶段性冻结。

只保留 TestPackZeroRuns 的独立精确字节回归，先在基线通过；三个候选最终快照的 Pack/Unpack 相关测试、三个归档补丁副本的完整源码/harness/夹具指纹核对通过。没有采用生产改动，未重复完整库/race/checkptr/官方/fuzz、GSR/GoTS 或 TS 性能验收。原值、控制波动及复现见 [筛选记录](benchmarks/2026-10-17-gosproto-zero-pack/RESULTS.md)。该轮已本地提交为 gosproto `bba3326` / GSR `a27ec4e`，未推送/发布，正式依赖仍为 v0.4.0。

## Pack 普通 word 执行路径收敛（2026-10-17）

Decode 阶段性冻结记录已提交为 GSR `0db5ed0`，后续仍可回头优化。当前仅改独立库 pack.go 的完整 word 执行位置，减少逐 word 调用，原 FF、尾块、mask、预留和独占规则保留；Decode/Encode/Unpack/meta/RPC/TS 源码不变。

137 项五轮共 685 样本，大型 Pack/PEncode 约 -13.6%/-6.0%，整数数组约 -19.6%/-5.7%，分配不变；另保留三轮 411 个筛选样本。最终无 SP 中位数回退 >5%，但冻结 payload 控制和 FF 尾块代价原样保留，不宣称完整路径全面胜 PB。

复用原独立字节回归，17984 差分、完整库 race/checkptr/固定官方 C/Lua 对照、vet、Pack/Codec 各约十秒 fuzz、GSR 三包及 TS 构建/GoTS Plain/MAC 7/7、零跳过通过。Linux amd64/arm64/386 构建通过，未跨平台运行或重测 TS 性能。profile 的 kevent 干扰明确保留，不把采样份额当提速比例。具体输入、补丁、指纹、复现与范围见 [验收记录](benchmarks/2026-10-17-gosproto-pack-execution/RESULTS.md)。该轮已本地提交为 gosproto `faa6a21` / GSR `3626b3d`，未推送/发布，正式依赖仍为 v0.4.0。

## Decode 性能阶段冻结（历史：2026-10-17，2026-10-19已解除）

上一轮回归/记录已本地提交为 gosproto `a09fb00`、GSR `d1beb28`，未推送。按用户决定单独验证 messageDecodePointer 入口：18 项五轮微基准与三种能力状态测试，选择一个候选进入 129 项五轮完整对照，共 735 个样本。

入口微基准约 -38.3%，但大型 Decode/PDecode 只有 -2.25%/-1.28%；虽两项五轮方向一致、分配不变，仍未达预定 3%/2% 门槛，未采用。Decode 冻结于 `a09fb00`（生产源码同 `fc3246a`），不再扩展局部性能候选。独立库工作树干净，正式 GSR 依赖仍为 v0.4.0。

实验与正式库分离，微基准、完整对照每轮分别运行能力/夹具用例，两份 prepare-only 副本核对完整源码与输入指纹通过。没有采用生产改动，因此未重复完整库/race/checkptr/官方对照/fuzz、GSR/GoTS 或 TS 性能；不把旧验收当成本轮重跑。预定门槛、逐轮原值、冻结范围和复现见 [冻结报告](benchmarks/2026-10-17-gosproto-decode-freeze/RESULTS.md)。该记录已提交为 GSR `0db5ed0`。

## Decode 入口与固定 word 恢复筛选（2026-10-17）

上一轮已本地提交为 gosproto `fc3246a`、GSR `862fa8b`，未推送。本轮三个入口候选各 59 项三轮，较密 word 固定恢复候选另做 129 项三轮，共 918 样本；全部因组合收益不足或回退未采用，没有进入五轮正式验收。生产代码仍为 `fc3246a`。

另采基线 CPU/alloc profile 和五组固定 body mask 分布，kevent 与业务成本区分报告。只保留 TestPack 的独立普通 word 后继 FF 与八种输入偏移回归，新增 2048 组，先在未改生产实现通过，再在单独候选副本通过；计时副本保持原始测试指纹。四种归档/补丁副本全部重建出与实测相同的源码、harness 和夹具。未重复完整库/race/fuzz、GSR 接线或 TS 性能；详见 [筛选记录](benchmarks/2026-10-17-gosproto-decode-binding/RESULTS.md)。该切片已本地提交为 gosproto `a09fb00`、GSR `d1beb28`，正式依赖仍为 v0.4.0。

## 原生整数 Decode 受控写入（2026-10-17）

前轮已本地提交为 gosproto `5be3b26`、GSR `2872d46`，未推送。按提案新增私有 decodePointer，仅写原生 *int64 标量；实际类型、字段 owner/形状与反射回落、Go 写屏障、独占分配和失败原子性均保留。该切片已本地提交为 gosproto `fc3246a`、GSR `862fa8b`，正式依赖仍为 v0.4.0。

封装后 129 项五轮：大型 Decode/PDecode 约 -7.4%/-5.9%，小型约 -7.7%/-5.6%，分配不变。AddressBook 约 +2%；未改极小 Unpack +16.6% 与单独 A/A +2.6% 一并保留。纯 Decode 与完整 packing 路径区分报告，不称为全面胜 PB。

新增正式 GC/padding/跨记录独占回归。完整库 race/checkptr/固定官方 C/Lua 对照、vet、131068 差分、有限 Decode/Codec fuzz、GSR 三包与 TS 构建/GoTS Plain/MAC 7/7、零跳过通过；缺失 owner guard 的副本按预期失败。Linux amd64/arm64/386 构建通过，未跨架构运行。具体数据、编译器/写屏障证据、复现与未验证项见 [实施验收](benchmarks/2026-10-17-gosproto-decode-store/RESULTS.md)。

## Decode 记录候选与类型化写入原型（2026-10-17）

上一轮已本地提交为 gosproto `70d985c`、GSR `d0328fd`，未推送。整数叶子记录 parser 只带来约 -3.1% Decode/-1.3% PDecode，已撤回。正式库仅新增精确稀疏 header 解码预算、失败原数组与 Fields 替换语义回归，生产 Go 源码全部保持 `70d985c`。

类型化写入只在隔离副本验证：129 项五轮中，大型 Decode/PDecode 约 -11.2%/-9.2%，分配不变。字符串/未改 Unpack 控制的代价与原值一并保留。原型完整库官方对照/race/严格 checkptr/vet、131068 组差分与 GC/padding 检查、Decode/Codec 有限 fuzz，以及临时覆盖 GSR 三包和 Go/TS Plain/MAC 7/7、零跳过通过。没有跨架构运行或新的极小项 A/A 复核。

原型不等于正式依赖已升级或写入边界已采用；[实验记录](benchmarks/2026-10-17-gosproto-decode-writes/RESULTS.md) 与 [受控写入提案](RFC-GOSPROTO-DECODER-WRITES.md) 记录范围、代价和后续门槛。该轮新增正式库测试、诊断和文档已本地提交为 gosproto `5be3b26`、GSR `2872d46`；后续实施见上节。

## Decode 同分配诊断与安全 inline（2026-10-17）

前轮已本地提交为 gosproto `d66644a`、GSR `acfb070`，GSR 补丁格式属性修正为 `ae2e1bc`，均未推送。先做同分配的固定 Item 构造/赋值诊断，再仅为已确认的 inline 原生 *int64 省去重复分派；仍用安全反射赋值，未新增 unsafe、API、池或缓存。

129 项五轮中，大型 Decode/PDecode 约 -5.3%/-4.1%，小型约 -4.6%/-3.1%，分配不变。原三轮明显波动、完整矩阵极小零段约 +13% 与 A/A 复核分别保留，未改 Unpack 的耗时差异不称为算法收益。固定 Item 类型化探针不冒充通用 decoder。

131068 组旧新差分覆盖全部合法 inline 值、低预算、迟到坏帧及 packed 路径；扩展既有指针独占回归覆盖 0/1/32766/32767/MaxInt64。完整库 race/严格 checkptr（固定官方 C/Lua oracle）、vet、Decode/Codec 各约十秒 fuzz、GSR 临时覆盖三包与 Go/TS Plain/MAC 集成 7/7、零跳过通过。未重复 GSR 全仓/race、TS 性能或跨平台运行。数据和具体范围见 [Decode 验收](benchmarks/2026-10-17-gosproto-decode-analysis/RESULTS.md)，该切片已本地提交为 gosproto `70d985c`、GSR `d0328fd`，未发布。

## 普通 Unpack 候选筛选（2026-10-16）

上一轮已本地提交为 gosproto `22e287e`、GSR `1af93f4`，未推送。五个位置/计数表与游标候选各做 129 项三轮配对，共 1935 样本，均因稀疏回退或真实组合收益不足撤回；所有 Go 生产源码恢复为 `22e287e`，没有新的性能验收结论。

只保留 TestPack 新增的 2048 组独立预期字节与偏移回归，已在未改生产代码时通过。复现副本的全部 Go 源码、七份 harness 和三份 JSON 指纹与实测一致。生产代码恢复后未重复全仓/race/vet/fuzz、GSR 集成或 TS 性能；额外采集三项基线 CPU/分配 profile，kevent 样本保留且不归于业务成本。数据、候选补丁与范围见 [筛选记录](benchmarks/2026-10-16-gosproto-unpack-candidates/RESULTS.md)。已本地提交为 gosproto `d66644a`、GSR `acfb070`，补丁格式属性修正为 `ae2e1bc`；未发布，正式依赖仍为 v0.4.0。

## 连续零 mask 有界解包（2026-10-16）

上一轮已本地提交为 gosproto `07350a9`、GSR `3a78dbd`，未推送。当前仅改 Unpack 的连续零 mask 扫描及分类，保留完整预检、有限输出预算、错误优先级和独占输出；Pack、Encoder、Decoder、RPC、元信息与 TS 生产源码冻结。

最终 129 项五轮配对中，4 KiB/64 KiB 零块 Unpack 耗时约 -64.0%/-68.3%；4 KiB 零字节 Echo 完整帧约 -51.2%，比同场 PB 约低 11.5%。AddressBook Unpack 约 +10%、PDecode +1.6%，部分普通形态约 +5% 的代价和全部失败候选原样保留，分配不变。

45824 组输入/预算差分、完整独立库 race/严格 checkptr（启用固定官方 C/Lua 对照）、vet、FuzzDecode 与 FuzzPackRoundTrip 各约十秒通过。GSR 临时覆盖三包、TS 构建和 Go/TS Plain/MAC 集成 7/7、零跳过通过。未重复 GSR 全仓/race、TS 性能或跨平台执行，详见 [解包结果](benchmarks/2026-10-16-gosproto-zero-unpack/RESULTS.md)。该切片已本地提交为 gosproto `22e287e`、GSR `1af93f4`，未发布，正式依赖仍为 v0.4.0。

## 长 FF 段有界搜索（2026-10-16）

上一轮已本地提交为 gosproto `7dc6713`、GSR `e9e1f74`，未推送。当前新增切片只在长 FF 起点搜索最多 255 个后续完整非零 word，含零 word 仍走原合并规则；主循环去掉中间 input 切片与重复长度判断。输出容量、wire、预算及独占语义不变，Unpack/Encoder/Decoder/RPC/TS 生产源码冻结。

最终同进程 137 项 × 五轮、685 样本：4 KiB ASCII Pack 2.075 → 0.5851 µs、PEncode 2.682 → 1.142 µs，完整帧 3.352 → 1.906 µs；UTF-8 帧 1.035 → 0.6813 µs。AddressBook/小型游戏 Pack 各约 +5%，最终无 SP 中位数回退 >10%，但不称为全项加速。前缀计算、直接内嵌扫描和容量收窄等候选及中间回退分别保留，共 1780 个非最终样本，不与最终值混算。

全部 mask 的长输入独立预期字节、17984 组旧新差分、完整独立库 race/严格 checkptr（启用固定官方 C/Lua 对照）、vet 及 Pack/Codec 有限 fuzz 通过。GSR 临时覆盖三包、TypeScript 构建和 Go/TS Plain/MAC 集成 7/7、零跳过。最终来源、接线验收与范围见 [FF 搜索结果](benchmarks/2026-10-16-gosproto-ffscan/RESULTS.md)。该切片已本地提交为 gosproto `07350a9`、GSR `3a78dbd`，未发布，正式 Go 依赖仍为 v0.4.0。

## Pack 混合数值 word 优化（2026-10-16）

上一轮已本地提交为 gosproto `5e491ff`、GSR `70c84af`，未推送。本轮只改 Pack 的稀疏/混合 word 写出和 FF 状态处理；Encoder、Decoder、RPC 与 TS 生产源码冻结，输出预留、wire、预算和所有权不变。所有 word 使用条件展开的候选造成稀疏回退，已撤回。

最终同进程五轮共 115 项、575 样本，另保留三个候选共 1035 样本。整数数组 Pack 3.664 → 3.287 µs、PEncode 5.902 → 5.498 µs；大型消息分别为 3.473 → 3.301 µs、10.045 → 9.917 µs。分配不变，无 SP 项中位数回退超过 10%，但单非零字节 4 KiB 形态 +3.2%，不称为全项加速或全面胜出 PB。

扩展既有全部 mask 的精确字节用例保护 FF 合并/退出，9216 组旧新 Pack 差分、完整独立库 race/严格 checkptr（启用固定官方 C/Lua 对照）、vet 及 Pack/Codec 有限 fuzz 通过。GSR 临时覆盖三包测试、TypeScript 构建与 Go/TS Plain/MAC 集成 7/7、零跳过。最终验证范围与原始数据见 [Pack 结果](benchmarks/2026-10-16-gosproto-pack/RESULTS.md)。该切片随后已本地提交为 gosproto `7dc6713`、GSR `e9e1f74`，未发布，正式依赖仍为 v0.4.0。

## 嵌套类型校验与整数记录数组（2026-10-16）

上一轮已本地提交为 gosproto `819047e`、GSR `bf3ed8c`，未推送。后续发现父 Fields 描述替换可能使子 schema 与实际 Go 布局不一致，单独的字段 owner 检查不足；新回归在上一提交失败、在原安全反射基线通过。当前已修复类型事实传递/回落，并仅在同构整数结构数组入口复用 owner/形状校验。

两组最终配对各 49 项 × 五轮，共 490 样本，另保留 380 个候选样本。大型 Encode 相对上一提交 8.382 → 6.528 µs，同场 PB 7.606 µs；相对最小类型修复版 9.730 → 6.544 µs，同场 PB 7.588 µs。AddressBook 相对上一提交 +12.0%，相对最小修复版 +1.3%；分配不变，不称为全项胜出。

新增嵌套描述替换三种情况、同构记录数组预算/tag/命名类型对照，Codec fuzz 增加整数记录数组。完整独立库 race + 严格 checkptr（启用固定官方 C/Lua oracle）、vet、Codec/Decode 有限 fuzz 通过；GSR 临时覆盖三包测试及 Go/TS Plain/MAC 集成 7/7、零跳过。该切片 Decode、Pack 和 TS 生产源码冻结，未重复全矩阵计时、GSR 全仓测试或跨平台验证。随后已本地提交为 gosproto `5e491ff`、GSR `70c84af`，正式依赖仍为 v0.4.0，详见 [结果与复现](benchmarks/2026-10-16-gosproto-bindings/RESULTS.md)。

## 大型 Encoder 字段绑定实施（2026-10-16）

经用户同意，独立库为原生 `*int64` 字段增加受控只读 offset 访问，保留 owner/形状检查和其他类型的反射路径。安全操作码分派候选未取得稳定收益，已撤回。Decode、Pack/Unpack 和 TS 生产源码未变；该切片已本地提交为独立库 `819047e`、GSR `bf3ed8c`，正式依赖仍为 v0.4.0。后续发现的嵌套描述校验缺口见上一节，不能将本节既有回归视为覆盖了全部布局替换情况。

完整 Go 主矩阵前后共 720 样本，存在时序波动，原始值全部保留；同进程编码专项 245 样本、解码异常项复核 45 样本，分别统计。大型 Encode 14.345 → 8.456 µs（-41.1%），同次 PB 7.863 µs，分配不变。小型无整数 body 有数纳秒代价，不称为全项无回退。

独立库启用固定官方 C/Lua 对照的完整 race/严格 checkptr、vet、Codec/Decode 各约 10 秒 fuzz 通过。新增填充/嵌套整数边界、跨类型字段替换、Wire/Array 变化三项回归，现有 Codec fuzz 扩展原生指针。GSR 临时依赖三包通过，Go/TS 7/7、零跳过。Linux amd64 测试二进制交叉编译、Linux 386 库与示例构建通过；32 位测试编译被基线已有的 math.MaxInt64 → int 溢出阻塞，未声称在 Linux 或 32 位运行通过。

来源、失败候选、完整原始值、profile 和复现见 [本轮结果](benchmarks/2026-10-16-gosproto-encoder/RESULTS.md)。未重复 GSR 全仓测试或 TS 性能采样。

## 大型 Encode 机制诊断与重构提案（2026-10-16）

第三轮已本地提交：独立库 `a579920`、GSR `fdbc736`，未推送/发布，正式依赖仍为 v0.4.0。本轮只增加分析文档与可复现诊断，生产代码未变，不重复上一轮完整测试。

从该提交隔离构建通用、inline 提前返回、固定 Item 反射流程、固定 Item 类型化流程四份副本。两组共 90 个样本，各五轮、每项固定 60000 次；四份固定 body wire 探针通过。主要机制对照为 14.631 / 10.993 / 7.188 µs，分配均为 3968 B / 2 次，不能作为正式 PB 排名或通用类型覆盖结论。计时后单独采两份 CPU profile。复现脚本 prepare-only 生成的四份源码与实测输入逐字节一致；新文档执行链接/JSON/语法与 diff 检查。

推荐方案与未裁决边界见 [编码执行层重构提案](RFC-GOSPROTO-ENCODER-REWRITE.md)，原始样本见 [诊断记录](benchmarks/2026-10-16-gosproto-encode-analysis/README.md)。unsafe、生成器和官方式单遍 writer 均未实施。

## gosproto 当前基线与第三轮候选筛选（2026-10-16）

TS 经验文档已提交为 `f45a8c6`，未推送。Go 从 `6012b06` 重建两套主基准，前后各 72 项、每项五轮，共 720 个计时样本；最终另测 18 种解包形态五轮，CPU 与分配单独采集。Go 数组纯编解码已经快于同次 PB，packing 组合和大型结构体编码仍有差距。

解包展开/窗口/稀疏分支及原生指针 Addr/Interface 赋值候选均因回退或业务收益有限未采用。最终只把当前调用布局记录从 int 收窄为 uint32：64 位大型消息 Encode/PEncode 各少 512 B/op，分配次数不变，CPU 基本持平，不改 wire、API、预算或 ownership。全量四项约 10%–13% 的中位数回退原样保留，固定次数配对复核未支持持续回退。TS 与 Go Decode/Meta/Limits/Pack 生产源码未变。

独立库完整 race（启用固定官方 C/Lua 对照）、vet、FuzzDecode/FuzzPackRoundTrip/FuzzCodecRoundTrip 各约 10 秒通过。回归新增全部 mask 独立字节的 Unpack 检查，以及原生指针字段/不同 Decode 调用间的别名隔离；临时 modfile 下 GSR 三包测试及 Go/TS Plain/MAC 集成 7/7、零跳过通过。未重复 GSR 全仓测试，未重新采 TS 性能，正式 Go 依赖仍为 v0.4.0；本轮改动尚未提交或发布。

数据、候选失败值、内存代价、profile 和来源见 [第三轮报告](benchmarks/2026-10-16-gosproto-round3/RESULTS.md)。

## TS 性能阶段收尾与 Go 优化准备（2026-10-16）

第五轮实现、工具和验收报告已提交为 `9801808`，提交后工作树干净，未推送。TS 性能优化阶段基本完成，当前不再默认投入微小收益的热循环改写；正确性修复及有真实业务证据的新热点仍按需处理。

已将有效做法、失败候选、输入构造/采样陷阱、所有权与预算约束整理为 [优化经验](SPROTO-OPTIMIZATION-LESSONS.md)，并接入 [gosproto 后续方案](RFC-GOSPROTO-PERFORMANCE.md)。Go 两轮改动当前已提交为 `6012b06`、尚未发布，正式依赖仍为 v0.4.0。下一轮从该提交重新固定基线和 profile，不从初版重做，也不从 TS 胜出推断 Go 必然胜出。

本次经验提取只修改文档，核对既有源码指纹、报告与文档链接，检查 diff/格式；不重复运行未变化的代码测试或 Benchmark，不宣称产生新的性能结果。

## TS Pack 字节分布与段状态优化（2026-10-15）

第四轮已本地提交为 `d9655b8`，未推送。第五轮分析编码后字节分布，确认整数数组无 FF word、大型游戏消息以四至六个非零字节的混合 word 为主。保留原八字节条件写出，将 FF 合并检查集中到段状态循环，普通 word 跳过段收尾处理；不改 wire、容量保护或依赖。

最终构建、Go/TS Plain/MAC 集成 7 项全部通过、零跳过，9560 组新旧 Pack/Unpack 差分通过；固定官方 C/Lua 20 组向量复核通过。扩展现有包边界用例覆盖所有六/七字节 mask 在 FF 内合并、五字节退出、尾部补零和 256-word 后恢复普通 mask。测试初次因沙箱拒绝本机监听失败，放行后完整通过。Go 生产源码不变，不重复全仓 Go/race/vet。

公开与私有解包省略视图的候选未通过完整形态验收，均已撤回；最终三种解包方法体与基线逐字节相同。采样收紧为按操作配对、奇数轮反转，减少版本整组运行的阶段偏差。最终共 250 项、1250 个样本，14 种 Pack/46 种 Unpack 形态，没有 SP 项中位数回退超过 10%；大型游戏/整数数组 Pack 分别约 -7.3%/-4.9%，FF 六/七字节混合段约 -35%。当前五种代表性消息 × 两条原生组合路径为 10/10 低于同进程 PB，不扩展为协议总体、Go 或浏览器结论。

分布、显式 Buffer 工厂/视图调用、独立 Pack/PEncode CPU profile 和失败候选原始值见 [第五轮验收](benchmarks/2026-10-15-sproto-round5/RESULTS.md)。正式 Go 依赖仍为 v0.4.0，集成接收端使用已提交 gosproto `6012b06` 的临时 modfile；第五轮已提交为 `9801808`，未推送。

## TS 零头展开计数与基准输入修正（2026-10-15）

第三轮已本地提交为 GSR `72fc11a`，没有推送。第四轮仅优化连续零 mask 的计数：确认四个零头后展开扫描，保留尾部、8 MiB 限制、有界前缀和输出所有权。数值 Pack 与原生零块比较候选因混合输入回退未采用，没有增加查表或保留内存。

最终构建、Go/TS Plain/MAC 集成 7 项全部通过、零跳过；3416 组新旧 Pack/Unpack 差分、固定官方 C/Lua 20 组向量通过。扩展已有边界用例覆盖零头展开邻域、非零 byteOffset、有限输出前缀、零段后非零 mask 和截断 FF；Go 生产源码不变，不重复全仓 Go/race/vet。接收端通过临时 modfile 使用已提交 gosproto `6012b06`，正式依赖仍为 v0.4.0。

Echo 输入改为准备期只构造一次，由 PB 和两个 SP 版本共享。原分别 repeat 的字符串值虽相同，V8 可能分别使用平坦和拼接表示，造成无编码源码改动却数倍耗时差；诊断证据保留，历史记录不覆盖。原有代表性 body 已共享输入。

新增重复短零段和零前缀混合输入，共 46 种解包形态、222 项操作、1110 个样本，CPU profile 分开采集。4 KiB 零字节原生 RPC 解码 1.269 → 1.007 µs，公开 4/16/64 KiB 零块解包分别约 -30%/-37%/-45%；没有 SP 项中位数回退超过 10%，仍有采样波动，不作为统计显著性或浏览器吞吐结论。完整结果见 [第四轮验收](benchmarks/2026-10-15-sproto-round4/RESULTS.md)。第四轮已提交为 `d9655b8`，未推送。

## TS FF 扫描、数值写出与有界解包缓冲（2026-10-15）

实施前已创建本地提交 GSR `2092005` 和 gosproto `6012b06`，没有推送或发布。随后完成 TS 第三轮优化：FF 扫描门槛降至 64 B，数值数组保留预检并按 4/8 字节使用缓存 DataView 写出；Pack 尾部取消临时分配；pdecode/dispatch 使用最多保留 16 KiB 的实例私有缓冲，公开字节结果仍独占。公开 unpack 的统一预扫与宽松 FF 前缀判定均因数值/混合消息回退撤回，最终仅在前八段 FF 字节覆盖整个输入四分之三时采用精确分配。

最终 TS 构建与 Go/TS Plain/MAC 集成 7 项全部通过、零跳过；3416 组新旧 Pack/Unpack 差分、固定官方 C/Lua 20 组向量复核通过。扩展现有回归覆盖 FF 门槛 wire、数值宽度/非对齐写出与扩容、大小包/失败后复用、返回值独占和解包额度边界。没有修改生产 Go，不重复全仓 Go/race/vet。

同进程 PB 控制、SP 前后原生/分阶段入口和 29 种解包形态共 188 项、940 个样本；CPU profile 分开采集。整数数组编码约 -70%、UTF-8 RPC 编码约 -46%、4 KiB 原生解码约 -50%；一项 16 KiB 零块全量中约 +10%，保持源码不变配对复核约 +2.5%，全部原始值保留。完整数据、源码指纹、缓冲容量观测与限制见 [第三轮验收](benchmarks/2026-10-15-sproto-round3/RESULTS.md)。上述 Echo 编码百分比还需考虑第四轮发现的字符串表示因素。第三轮已提交为 `72fc11a`，正式 Go 依赖仍为 v0.4.0。

## 数值 packing、字段查询与类型化构造（2026-10-15）

继续按数值 Pack、TS 字段索引、Go 标量写出、Go 类型化构造、完整帧与解包的顺序优化。Go 使用精确 word mask 并保留 FF 快路径；TS 展开八字节 Pack/Unpack，按 schema 建立有界 tag 索引；Go 合并标量读取与写出，对原生 *int64/*string 使用类型化构造，命名类型保留兼容路径。TS pencode/attach/Response 减少中间复制，公开 encode 保留直接路径和独占输出。未新增公开 API、unsafe、代码生成或依赖。

扩展 Go/TS 全部 mask 与高位字节、原生/命名 optional 标量测试。TS 未知 body 字段错位以及合并缓冲挤占 8 MiB body 编码额度的问题均由新增用例先复现再修复；package/body 原有独立额度、超限拒绝及输出快照保持。独立 Go 库官方 C/Lua 对照、完整 race、vet 和三个各约 10 秒的有限 fuzz 通过；最终 TS 3416 组新旧 Pack/Unpack 差分通过。GSR 受影响三包 race 和最终 Plain/MAC 集成六项全部通过、零跳过。

原有两套完整五轮基准共 720 个样本，另采 TS 同进程前后对照 340 个样本及 Go Echo 波动复核 30 个样本；profile 在计时完成后单独采集。Go 大型消息 Pack 约 5.500 → 3.483 µs，解码约 23.310 → 18.152 µs，分配与首轮逐项一致。TS 完整采样出现明显波动，PB 控制项也变慢，全部原始值保留；同进程交错中大型消息解码约 -14%、Pack/Unpack 约 -23%/-17%，4 KiB ASCII RPC 编码约 -49%。Go ASCII 帧专项未支持持续回退，零字节帧编码约 -38%。Go 查表解包和 TS 预清零输出的回退尝试已撤回。

实现、完整数据、同进程复核、profile 和覆盖限制见 [第二轮验收](benchmarks/2026-10-15-sproto-round2/RESULTS.md)。Go 源码仍在独立库未发布工作树，GSR 正式依赖仍为 v0.4.0，验收使用临时 modfile；本轮未提交或发布。

## TS 长块与 gosproto 反射路径优化（2026-10-15）

先优化 TS 长 FF/零块 Pack/Unpack，再按性能方案改进独立 Go 库的字段元信息、原生 []int64 批量循环、当前调用布局记录与有界稠密 tag 索引。公开 API、官方 wire、资源预算、失败原子性和输出独占不变。Go 改动尚未提交/发布，GSR 正式依赖仍为 v0.4.0，验收使用临时 modfile。

扩展现有测试覆盖有限输出前缀、混合零块、截断、命名/unsigned 数组与未知高 tag；新增同一对象改变 presence、数组宽度、空/nil 数组和嵌套数量后重编码的回归。独立库完整回归、固定 C/Lua 对照、race 与 vet 通过，三个 fuzz 入口各约 10 秒通过。临时覆盖下，Sproto Dispatcher、Echo 服务端和 Benchmark 包 race 通过，Plain/MAC Go↔TS 集成六项全部通过、零跳过；TS 开发期新旧实现 3200 组差分通过。

两套完整 Go/TS × PB/SP 五轮采样（720 个计时样本）与 game_large 六步骤 Go/TS CPU/分配 profile 已完成，采样互不并行。大型游戏消息 Go 纯编码约 26.973 → 14.781 µs，整数数组约 13.125 → 2.224 µs；TS 4 KiB ASCII 帧编码约 7.660 → 3.263 µs，零字节帧解码约 11.854 → 2.640 µs。大型 Go 编码多 1024 B / 1 次布局分配，每次新建 Echo 帧头另多 16 B / 1 次；数值 packing 未获得同等收益，Go Pack 零 word 尝试已撤回。

完整 Echo 采样出现 Go 零字节帧编码约 38% 的疑似回退，保持源码不变交错复核为 7.247 → 7.257 µs，未支持持续回退；全量慢样本仍保留。实现范围、前后数据、复核、profile 与来源见 [本轮验收](benchmarks/2026-10-15-sproto-next/RESULTS.md)，历史正式基线不改写。

## v0.4.0 正式依赖完整性能复测（2026-10-15）

依赖升级提交 `f82551c` 后，完成 Go/TS × Protobuf/Sproto 的两套现有 Benchmark：每种语言 40 项代表性 body 操作和 32 项 Echo payload/frame 操作，各五轮，共 720 个计时样本。全部语义、固定 wire、padding 检查通过，两个 TS 能力探针通过；全程顺序采集，未修改编解码器或重复采集 profile。代表性采样在依赖提交前启动，使用已升级工作树；Echo 在提交后启动，源码指纹一致。

完整四种实现对比、包大小、Go 分配、图表与两项 max/min 超过 2 的采样波动见 [完整复测报告](benchmarks/2026-10-15-v0.4.0/report.md)。原始五轮和环境作为正式基线保留；中间生成脚本与临时数据继续忽略。本机结果不扩展为所有协议类型、语言性能或部署吞吐结论。

## gosproto v0.4.0 发布与 GSR 正式依赖升级（2026-10-15）

[gosproto v0.4.0](https://github.com/cnooljaw/gosproto/releases/tag/v0.4.0) 已正式发布，标签指向 `3f2491f7b2701c710fdfc9dfa7952c8e7002dd1d`。本机官方 C/Lua 对照、完整 race 与 vet 通过；GitHub 分支和标签两次 CI 均通过，包含固定官方对照、race、vet 和三个各 30 秒的有限 fuzz 入口。发布版五份 Go 编解码源文件与优化 Benchmark 保存的哈希一致，未改写历史采样环境或重新宣称性能结果。

GSR 已正常 require v0.4.0，go.sum 使用发布校验和，无 replace 或临时 modfile。`go mod tidy` 仅移除旧版校验和；`GSR_PROCESS_TEST=1 go test -race ./... -timeout=2m` 与 `go vet ./...` 通过，包含跨进程回归、Sproto Dispatcher 和 Benchmark 固定 wire 验证。真实 Plain/MAC Go↔TS 集成六项全部通过、零跳过，覆盖官方向量、安全整数、会话隔离、长报文、Request/Response、Notify/Push 和坏包关闭。协议类型未变化，无需重新生成。

## Sproto 正确性与五步优化验收（2026-10-15）

基线提交为 `d942321`。TS boolean 与混合正负 int64 数组缺陷先由现有测试新增用例复现，再修复；20 组独立官方 C/Lua raw/packed 向量、非法整数拒绝及输出快照检查通过。固定官方源码哈希检查通过，真实 Plain/MAC Go↔TS Echo 六项测试全部通过、零跳过，包含长报文、Notify/Push 与坏包关闭。

Go 库已完成头部流式解码、数组一次分配、最终缓冲直写及 Pack/Unpack 优化。独立库完整回归、官方对照、race 和 vet 通过；有限 FuzzDecode 10 秒、FuzzPackRoundTrip 15 秒及 FuzzCodecRoundTrip 15 秒通过，数组写入调整后另跑一轮 CodecRoundTrip。保留官方 wire、资源预算和解码失败原子性，未改变公开 API。

两种语言各 40 项五轮基准、32 字节小包专项及 game_large 六步骤的独立 CPU/分配 profile 已完成。大型游戏消息 TS Encode 约 34.6 → 18.0 µs；Go Encode 320 → 1 次分配，Decode 1143 → 534 次分配。TS/Go 中出现回退的尝试已撤回，完整结果与限制见 [优化实施与验收](benchmarks/2026-10-15-optimized/RESULTS.md)。

上述采样发生在 Go 优化提交 `931e2a6` 发布前，当时 GSR 正式依赖为 v0.3.0；优化接线和测量使用临时 modfile，并保存实际依赖覆盖及源码哈希。当时 GSR 默认依赖的全仓 race/vet 亦通过，与本地优化库验收分开记录。后续 v0.4.0 发布和正式依赖升级结果见上一节。

## AddressBook、数值/数组与分阶段 Profile（2026-10-15）

补充独立代表性消息套件：官方 AddressBook、两档游戏状态、896 元素 int32 边界数组、4 KiB ASCII。五组 Sproto raw/packed 与固定官方 C/Lua 逐组 SHA-256 对照通过，六份官方源码由 GitHub API 验证固定版本一致；AddressBook 复现 Sproto 130/83 字节、PB 69 字节。Go 与 TS 固定 wire、完整语义和 padding 检查通过，Go 夹具测试通过。两种语言各 40 项、五轮采样全部完成，分开报告 Encode、Decode、Pack、Unpack 及组合路径。

对 AddressBook/game_large 各六步骤采集 Go CPU/分配与 TS CPU/V8 分配，共 24 组，完整 profile 留在 tmp，环境、命令和 top 汇总随报告保存。CPU 与分配采集分开，profile 不用作计时结果。新 TS 能力探针暴露 boolean 与混合正负 int64 数组问题，明确列为未通过，没有跳过后宣称支持；本轮只测 int32 数组，不修改编解码器。未重复运行无关全仓/race 测试。结果、profile 限制和待实施优化方案见 [新报告](benchmarks/2026-10-15-representative/report.md) 与 [分析](benchmarks/2026-10-15-representative/ANALYSIS-AND-PLAN.md)。

## Protobuf / Sproto Go 与 TS 编解码 Benchmark（2026-10-15）

新增共用 Echo 字符串夹具及固定 wire SHA-256，Go/TS 各测 32 项，覆盖短 ASCII、UTF-8、4 KiB ASCII、零字节密集输入的 payload/frame 编码和解码。两种语言在计时前均验证字节签名与语义往返；TS 具名 payload schema 的应用帧另与真实 Echo 客户端逐字节核对。五轮采样、环境、分配、包大小、波动范围及命令见 [报告](benchmarks/2026-10-15/report.md)。仅报告本机微基准，不扩展到数组、嵌套结构或端到端吞吐。

测量定位到 TS write_ff 中逐字节生成未使用的十六进制调试字符串；删除该无用计算后，同口径 4 KiB ASCII 帧编码中位耗时由约 452 µs 降至 19.6 µs。保留独立前后原始数据，最终横向比较统一为每次构造外层头对象。移除调试计算后，现有 Go/TS Sproto Plain/MAC 集成测试五项全通过、零跳过；最终基准全部完成，wire 检查通过，TS 构建与 diff 检查通过。部分本机样本存在明显负载波动，报告中显式列出；没有重复运行无关全仓/race 测试。

## TS Sproto 长报文与解包扩容（2026-10-15）

扩展现有 Plain/MAC Go↔TS 集成用例，验证 2 KiB、4 KiB 附近的非零长字符串，以及零字节密集、零/非零混合字符串的 Request/Response 和 Notify/Push。非零边界通过后，高压缩率字符串在两种模式下均复现 truncated Sproto data；问题来自示例 TS 参考解包器在缓冲区不足时提前停止扫描，或跳过放不下的 FF 块而未计入输出长度。

按官方 sproto_unpack 的行为，缓冲不足时仍完整扫描、推进输入和输出计数，计算有限的完整输出长度后按确切大小重新分配，并保留 8 MiB 上限。现有单元用例增加高压缩率和 FF 连续块的往返断言，以及超预算拒绝；无需启动 Go 服务端即可保护此回归。修复后 `npm run test:sproto:integration` 五项全部通过（零跳过），包含新增的长报文场景；diff 检查通过。修复只影响 TS 联调参考，不新增 SDK 或公共协议模型。

## GSR 升级 gosproto v0.3.0（2026-10-15）

GSR 已正常 require github.com/cnooljaw/gosproto v0.3.0，go.sum 固定发布校验和，无 replace。更新现有 sprotodump 包装脚本，生成时剔除已移除的 MethodName 字段；C2S/S2C 产物重新生成，两次生成结果哈希一致。GSR 的 Agent/Dispatcher 未依赖被删的网络 Service helper，无需增加兼容层。

使用正式依赖与真实生成物（无临时 modfile/overlay）运行 protocol/sproto 和 Go Echo package race，通过。`npm run test:sproto:integration` 五项全部通过（零跳过），覆盖 Plain/MAC、Unicode 请求、业务拒绝、Notify/Push 和坏包关闭。`go test ./... -timeout=2m` 与 `go vet ./...` 通过；新增验证集中在依赖和生成接线，没有重复添加底层编解码测试。

## Sproto 修复库与双协议纵向切片（2026-10-15）

独立 cnooljaw/gosproto v0.2.0 保留 xjdrew v0.1.0 基线和许可证，修复负整数数组、空数组、坏宽度、原生 double、溢出、schema 缓存失败及有限字节/深度/元素预算；回归、官方 C/Lua 对照、race、vet 和 30 秒有限 fuzz 通过；同一提交的 GitHub CI 在 Ubuntu/Lua 5.4 下也通过官方对照、race、vet 和 fuzz。固定官方版本自身的 64 位 todword 解码缺陷已记录；完整边界采用独立官方编码向量与 Go 编码逐字节比对，并验证 Go 解码，不复制错误解码。

独立 module 正常安装已发布 v0.2.0，64 位整数数组和 double 往返通过；GSR 已移除开发 replace，go.sum 固定校验和，发布模块下相关 package 全测通过。

GSR 新增 protocol/sproto 的构造校验、原生零 type、Response/Notify/Push、session、同步 Context、重复响应、坏包与错误契约测试。复用原 Echo 业务和连接夹具，Go 与 TypeScript 通过真正的 Plain/MAC WebSocket 链路验证 Unicode 请求、业务拒绝、Notify→Push 和坏包关闭；MainService 打开 Gate 后退出，连接 Agent 继续运行。`npm run test:sproto:integration` 全部五项通过（零跳过），认证固定向量 `test:crypto` 保持通过；固定 sprotodump 能重建 Go 产物。

`GSR_PROCESS_TEST=1 go test -race ./... -timeout=2m` 通过（Gate 约 35 秒），覆盖现有 Protobuf 和跨进程回归；全仓 go vet 通过。新增验证的增量为 Sproto 原生分发及跨语言接线，未为相同 Core 契约重复建立夹具。TS 示例仅验收扁平 Echo 与安全整数，不扩展为完整浏览器 SDK、map、BigInt 或 double 支持结论。新 GSR API 尚未发布；历史记录中的“尚未接入”保留当时含义。

## Sproto 官方与第二候选库对照（2026-10-14）

固定官方 cloudwu/sproto 提交编译 C/Lua 实现，实际验证负整数数组、标量、double、规范空数组、非规范空数组和非法非空零宽度数组。用官方向量检查 davyxu/gosproto 固定提交及 xjdrew/gosproto v0.1.0：davyxu 五条门禁均未通过，xjdrew 通过负整数标量，其余四条未通过。另更正此前将零宽度无元素输入笼统称为坏包的表述：官方接受该输入，Go 候选库触发 panic，仍属于兼容性缺陷。

候选源码、用例、编译及模块替换仅存在于临时目录，不修改 GSR 生产依赖、协议、生成代码或 sibling 参考仓库。第三方生成器只做源码检查，未运行；未重复运行无变化的 GSR 测试。固定版本、限制及实际结果见 [接入计划](SPROTO-INTEGRATION.md#官方对照与-davyxu-候选验证2026-10-14)。

## Gate 安全模式命名与 Sproto 前置验证（2026-10-14）

按 [ADR 0013](adr/0013-client-protocol-boundaries.md) 将 Gate Protocol 类型、常量和配置字段改为 SecurityMode，保持枚举值、默认 MAC、握手、seq/MAC、Plain 绑定限制与生命周期。调用方与既有测试同步迁移，不新增兼容别名或重复测试夹具。

`go test ./gate . ./examples/websocket-gate/cmd/debugserver -timeout=2m` 通过（Gate 约 33 秒），覆盖现有 MAC/Plain、应用接线和 Protobuf Echo。测试夹具的变量/用例文字随后同步命名，由 `go test ./... -run '^$' -timeout=2m` 验证全仓编译；不因纯命名重复运行全仓行为、race、TypeScript 或进程恢复门禁。Go 格式、diff 和相关 Markdown 本地链接检查通过。

Sproto 仍未进入生产依赖或公开 API。隔离 module 的 `gosproto v0.1.0` 最小用例复现负整数数组改变数值、double 不支持及坏数组除零 panic；这些是候选库未通过的接入门禁，不是 GSR 的失败测试。候选范围、首次 clone 无进展及后续步骤见 [接入计划](SPROTO-INTEGRATION.md)。尚未验证 Go/TS Sproto Echo。

## v1.0.1 发布验收（2026-10-14）

统一 Node 运行对象后的发布代码已通过 `GSR_PROCESS_TEST=1 go test -race ./... -timeout=2m` 和 `go vet ./...`。全仓 race 覆盖默认 Go 行为测试、双进程正常停机与强杀后恢复；根包约 2 秒、双节点约 19 秒、Gate 约 35 秒，未变化的部分 package 使用缓存，不重复普通全仓测试。

版本文件、发布说明、API/术语、业务指南和模板已统一到 v1.0.1；模板参考链接固定到 v1.0.1 标签。Go 格式、diff 与发布相关 Markdown 本地链接检查通过。相对 v1.0.0 的六项变化和源码不兼容迁移见 [发布说明](RELEASE-1.0.1.md)。未重跑 TypeScript 认证测试，未启用真实 MySQL、负载或 Gate 压力门禁。

## 统一 Node 运行对象（2026-10-14）

按用户确认合并 application 与 Node，删除包装及名称注册转发；Run 创建唯一 Node 并管理关闭，ServiceContext.Node 返回所属对象。地址表改为 nodeAddresses，公开五个方法、通配监听及关闭顺序保持不变。内部状态与关闭方法仍未导出。

扩展 TestConcurrentServiceClusterBindings，在旧实现上先复现同一回调取得不同包装对象的问题；合并后保护同 Run 的 Service 共用节点、各自通信来源仍绑定 Self。现有 TestRunNodesCommunicateWithoutSingleton 增加不同 Run 的 Node 对象隔离，保留名称重建、Send/Call、入口退出和最终收尾检查；名称竞争和 benchmark 夹具迁移到 Node，不另建网络夹具。

`go test -race . ./examples/http-inbound ./examples/twonode -timeout=2m` 通过，`go test ./... -run '^$' -timeout=2m` 全仓编译通过（含测试和 benchmark 夹具）；go doc 核对公开方法集仍为五项。未重复全仓行为/race、跨进程恢复、外部依赖或压力验收；此前 API 迁移的全仓验证记录见下节。

## Node 与 Service 日志入口调整（2026-10-14）

按 [ADR 0012](adr/0012-node-capabilities-and-service-logging.md) 迁移节点名称/接入能力，Cluster 视图只通信，Logger 绑定视图只记录，应用配置无 Logger 注入字段。旧 v1.0.0 标签不移动，调整后的接口随 v1.0.1 发布。

扩展原有日志并发与快照用例：旧方法集首先被 Close 暴露检查拒绝，修复后验证共享 FIFO、来源、快照和 owner/ServiceLogger 调用位置。HTTP 与 Gate 既有接线测试等待 MainService OnStop 后继续真实请求；名称归目标和最终端口释放沿用现有验收。原应用占端口/重试用例改用非本机访问 host，检查仍按配置 port 通配绑定；Go 在本机可把 0.0.0.0 的实际地址显示为 IPv6 unspecified，验证按通配语义判断，生产绑定不变。未新增重复网络夹具。

`GSR_PROCESS_TEST=1 go test -race ./... -timeout=2m` 和 `go vet ./...` 均通过；全仓 race 覆盖默认行为及双进程正常停机/强杀恢复（双节点约 17 秒、Gate 约 35 秒），不重复已由 race 覆盖的普通测试。此前普通全仓检查发现残留 Gate 日志夹具类型及监听显示形式断言问题，修正后由上述全仓验收覆盖。

更新后的独立业务项目骨架以临时 module + 本地 replace 编译运行，输出 count=7，入口退出后节点继续运行并在 SIGTERM 后有序退出。Cluster 教程接入双节点源码后，两个独立进程真实 TCP 四参数往返及有序退出通过。文档链接和 diff 检查通过；未重跑 TypeScript（客户端/wire 未改）、外部数据库、容量或压力门禁。

## 1.0 内部发布验收（2026-10-12）

代码基线 `b2a9f16` 已通过同日 `go test ./... -timeout=2m`。本轮补跑 `go vet ./...`、`GSR_PROCESS_TEST=1 go test -race ./... -timeout=2m` 和 WebSocket 示例的 `npm run test:crypto`，全部通过；race 包含双进程正常停机与强杀后恢复，TypeScript 包含编译及 5 条认证测试。首次 race 受沙箱 TCP 监听限制，在允许本机监听的环境中运行同一命令后完整通过。仅补充版本与发布文档，没有修改 Go/TypeScript 实现或重新生成协议代码；未启用负载、Gate 压力或真实 MySQL 门禁。环境、耗时与接入范围见 [1.0 内部发布说明](RELEASE-1.0.md)。

## Cluster 参数教程与覆盖补充（2026-10-12）

新增 [节点间调用教程](CLUSTER-CALL-TUTORIAL.md)，说明默认 Codec 的类型规范化、多位置参数与列表参数的区别、强类型 Handler、返回值及错误。扩展已有 `TestValueCodecRoundTripsSupportedValues` 和 `TestTCPClusterRoundTrip`，补充 Unicode/空字符串、64 位整数极值、类型化切片、定长数组、字节数组、typed map 和 nil/空集合；TCP 用例复用原有连接与 Core Echo Service，验证请求和结果往返，不新增重复夹具或修改生产实现。

`go test -race ./cluster -timeout=2m` 通过（约 2 秒）。教程第 5 节的接收 Service 与调用代码已接入双节点启动结构，在临时目录编译后以两个独立进程经真实 TCP 调用，实际输出 `text=hello count=3 items=[10 20] meta=map[active:true]`，两端均有序退出。首次定向 TCP 测试受到沙箱监听限制，后续验证在允许本机监听的环境中完成。本轮未重跑全仓测试、vet、跨进程恢复门禁或压力测试；1.0 发布验收记录保持为此前基线。

## 已覆盖的运行链

2026-10-09 Gate / Protocol 边界切片：将协议分发、通用 proto、生成类型及生成器移到同级 `protocol/`；Gate 不再依赖 Protobuf，ClientSession 接纳字节并复制快照，Protocol 在发送边界编码完整 Envelope。扩展既有队列测试，先复现缓冲区复用影响待发送消息的问题，再验证不透明字节、快照和空消息条目开销；既有协议与示例测试改为检查字节写回。首次 Gate race 验证发现 HTTP 请求头超时用例先收到 Agent 构造通知、后更新接入计数的断言竞态，复用已有等待函数后 Gate 包 race 通过（约 35 秒），未修改生产接入行为。Protocol、Echo 示例及应用 OpenGate 的 race 测试、生成器和通用类型测试通过；重新生成 Go/TypeScript 类型、TypeScript 五条认证用例及全仓仅编译检查通过。生产依赖清单确认 Gate 与 Protocol 互不导入；未运行全仓行为测试、全仓 race 或长时压力测试。

2026-10-09 Gate 统一切片：连接实现、私有公共流程和原有两套测试迁移到 `gate`，应用入口收敛为 `OpenGate`；MAC 为默认模式，Plain 必须显式选择并绑定回环/私有 IP。保留既有认证、接纳、背压和停止覆盖，调整 Plain 的 TCP 容量及 HTTP 请求头超时契约；扩展配置测试覆盖未知模式和 Plain 小帧上限，新增 Plain+TLS 用例复用现有夹具。`go test -race ./gate ./examples/websocket-gate/cmd/debugserver -timeout=3m` 通过（约 35 秒/2 秒）；随后配置、Plain+TLS、应用 OpenGate 接线和简化后生命周期的定向 race 测试通过。`go test ./... -run '^$' -timeout=2m` 全仓编译检查通过，未重复运行全仓行为测试、全仓 race、TypeScript 或长时压力验收。

应用 Timer 身份切片：TestServiceTimerPreservesOwnIdentity 经 gsr.Run 在 OnStart 登记零延时 Timer，确认回调 Source 为零、Self 为拥有者；回调发起的本地 Call 使用拥有者来源，绑定 ClusterClient 仍使用 Self，实际排空的 JSON 日志也记录拥有者 ServiceID。相关测试、全仓测试、vet 和 race 通过；未修改生产代码，此测试不包含远端 Timer 调用或计时精度测量。

应用子 Service 创建失败隔离切片：TestServiceChildStartupFailureIsolation 从 gsr.Run / self.CreateService 覆盖 OnStart 返回错误、panic 和 typed nil。失败不发布有效 ID，已进入启动的实例停止一次，无效实例不调用生命周期；后续健康实例获得不同 ID，Call 成功且 Source 为创建者，最终正常停止。相关测试、全仓测试、vet 和 race 通过；未修改生产实现，未重跑跨进程演练。

应用节点配置切片：新增 Run 非法节点表测试，覆盖空名称、缺失 host/port、非数字或越界端口、非法 host 分隔符及未加括号的 IPv6，确认业务入口不会执行。节点表拆分测试覆盖域名、IPv6、端口上下界、端口零仅保留在监听表，以及输入表/监听表/出站表互不共享 map。相关测试、全仓测试和 vet 通过；未修改生产实现或并发逻辑，本轮不重复运行 race 和跨进程演练。

应用入口故障收尾切片：TestRunCleanupAfterOpen 覆盖真实监听建立后入口启动失败，以及正常取消时子 Service 的 OnStop 返回错误。验证入口和子 Service 各停止一次、启动与停止错误均保留、子 Service 停止日志先于最终日志且全部排空，并通过重绑原端口确认监听释放。全仓测试、vet 和 race 通过；未修改生产代码，未重跑跨进程演练，不覆盖无法返回的 OnStop 或阻塞 Writer。

Counter 入口统一切片：最小本地示例也改用 gsr.Run 和应用层 ServiceContext。三个命令由简单 Handle 校验并分派，不增加 Core dispatcher 的应用层适配框架；有限演示在 OnStop 通知宿主取消等待，Exit 本身仍不停止整个节点。真实运行输出保持 count = 5 / count = 0，新增输出失败保留错误测试。全仓测试、vet 和 race 通过；本轮未重跑跨进程演练，未修改 Runtime/Cluster 生产实现。

应用入口迁移切片：移除公开 app.Host，新增 gsr.Run 与应用层 ServiceContext/CommandContext，self.Cluster() 自动绑定 Self，self.Logger() 绑定同一来源。MainService 创建业务 Service、Register、按节点表 Open 后 Exit；双节点示例不再持有 Host 或 Client。测试覆盖多 Service 并发来源隔离、转调不继承上一跳、同进程两节点名称与连接独立、纯调用节点无需 Open、启动失败回滚，以及双进程正常停机/强杀后的恢复。全仓 go test、go vet、启用 GSR_PROCESS_TEST=1 的全仓 race 通过。Open 取节点表端口绑定所有接口；测试端口零仅供动态监听，不进入出站路由。下方 Host 描述属于历史切片，不代表当前应用接口。

统一 Command 编码切片：移除 CommandCodecRegistry 和 app.Config.Commands，Cluster 默认使用统一 ValueCodec；合法请求交给目标 Service 判断 Command。新增 adapter 与真实 TCP 测试，覆盖跨 Service 复用同一编号、未知 Send 无回包、未知 Call 返回 ErrUnknownCommand，以及后续正常调用。原未知命令坏帧测试改为坏载荷诊断，保留限频、编解码 panic、错误回包和连接恢复验证。全仓测试、vet、启用 GSR_PROCESS_TEST=1 的全仓 race（含双进程正常停机及强杀恢复）通过。self.Cluster() 和应用入口替换尚待落地；下方历史切片的 Command 清单描述不再代表当前接口。

应用宿主首个切片：新增 app.Host，集中默认 codec 组装、共享 Logger、初始化失败回滚、入口启动和固定顺序关闭。测试覆盖正常取消、入口失败、构造回滚、重复关闭及未注册 Command 拒绝；双节点示例已改用 Host。全仓测试、vet，以及启用 GSR_PROCESS_TEST=1 的全仓 race（包含正常停机和强杀恢复）通过。按节点名称 Open 尚未落地，当前宿主保留显式 ListenConfig；不宣称已完成全部易用性方案。

双节点启动组装切片：宿主只创建普通 MainService，由其创建 Echo/Caller 后 Exit，监听和资源关闭仍归宿主。新增 `TestMainServiceCreatesEcho` 验证入口关闭接纳后 Echo 仍可调用；改造前后均运行跨进程正常停机与强杀恢复门禁。改造后全仓测试、vet，以及启用 `GSR_PROCESS_TEST=1` 的全仓 race 通过；不改变 wire 协议、业务命令或关闭顺序。

退出失败隔离切片补充 `TestServiceExitDoesNotHidePanic`：OnStart 或 Handler 请求 Exit 后 panic，仍返回 ErrServicePanicked，而非正常退出成功；启动失败不返回有效 ID，Handler 失败后拒绝新消息。两条路径均只执行一次 OnStop、清空 PendingCall，健康 Service 仍可调用。相关测试、Runtime 全包、全仓测试、vet 和 race 通过；未修改生产代码。

退出与整体停止交错切片补充 `TestServiceExitOverlapsRuntimeStop`：Handler 已请求 Exit 但未返回时，Runtime Stop 以停止错误完成在途 Call；取消一次 Stop 等待不取消关闭流程。释放 Handler 后正常 Stop/Close，OnStop 仅执行一次，PendingCall 归零。相关测试、Runtime 全包、全仓测试、vet 和 race 通过；使用同步屏障固定交错顺序，不是随机压力或强杀测试，未修改生产代码。

后续退出隔离切片补充 `TestServiceCreateInHandlerAndExitWithSlowStop`：普通 Handler 创建并调用新 Service 时 Source 正确；自身 Exit 后，即使 OnStop 受控阻塞，当前 Call 已返回、新实例仍可调用，退出者拒绝新消息，释放 OnStop 后只清理一次且 PendingCall 归零。相关测试、Runtime 全包、全仓测试、vet 和 race 通过；本轮未修改生产实现，不代表能强杀阻塞的 OnStop。

2026-09-16：ServiceContext.CreateService / Exit 和 MainService Counter 示例已通过全仓 `go test ./... -timeout=2m`、`go vet ./...`、`go test -race ./... -timeout=5m`。首次沙箱运行的 TCP bind 被拒绝，授权后完整通过；部分未变化 package 使用缓存。新增 [退出测试](../runtime/service_exit_test.go) 覆盖启动期退出、当前 Call 正常返回后退出、排队 Call 失败、Timer 清理，以及创建者启动失败或退出不停止已创建的 Service。这不是强杀或生产容量验证。

| 能力 | 主要测试证据 | 证据边界 |
| --- | --- | --- |
| 本地 Service 生命周期、Send、Call、Timer | [Service](../runtime/service_test.go)、[Call](../runtime/call_test.go)、[Timer](../runtime/timer_test.go) | 本地行为与关闭契约，不代表真实业务负载 |
| 规模与关闭后的资源状态 | [规模测试](../runtime/scale_test.go) | 包含 10,000 个空闲 Service 和 10,000 个 Timer；不是吞吐、尾延迟或长期内存稳定性承诺 |
| 公开 Cluster 调用、Open 与关闭 | [公开出站](../cluster/cluster_test.go)、[Open](../cluster/open_test.go)、[关闭边界](../cluster/open_boundary_test.go) | 同一测试进程内的真实 TCP 回环，不是独立进程部署验收 |
| 断线、按需恢复与不重放 | [出站连接](../cluster/outbound_peer_test.go)、[TCP 集成](../cluster/tcp_integration_test.go) | 覆盖半写、断线和下一请求恢复；不承诺结果不确定请求可安全重试 |
| 远端 Service panic 与新实例隔离 | [TCP 故障链路](../cluster/tcp_service_failure_test.go) | 结构化 panic 错误不泄露内部文本；健康 Service 可继续调用，新实例获得新 ID，旧 ID 不误投；同进程真实 TCP，不是进程崩溃测试 |
| Call 取消与迟到回复 | [TCP 迟到回复](../cluster/tcp_late_reply_test.go) | 远端 Handler 已进入后取消本地等待；迟到回复报告未知 Session 并丢弃，同一连接前后健康调用成功，两端 PendingCall 清理；不覆盖业务副作用撤销或 deadline 精度 |
| 目标 Mailbox 满载与恢复 | [TCP Mailbox](../cluster/tcp_mailbox_test.go) | 受控阻塞目标并本地填满默认队列，远端 Call 返回 ErrMailboxFull；健康 Service 继续调用，排空后原连接及原目标恢复；不是远端 Send 失败回执或吞吐测试 |
| 名称映射更新与旧 ID 隔离 | [名称映射](../cluster/name_rebinding_test.go) | 外层发布新映射后，后续名称 Call 到达新实例，旧 ID 仍返回已关闭；不提供注册发现、不迁移在途请求 |
| 坏帧、超限与残帧隔离 | [协议故障](../cluster/open_protocol_test.go)、[响应读取](../cluster/response_reader_test.go) | 合法后续调用继续运行；不可恢复帧边界关闭连接，不把残帧送入 Handler |
| 重复坏帧诊断与日志 | [双连接日志](../cluster/open_logging_test.go)、[入站连接](../cluster/inbound_connection_test.go) | 每连接同类首条、抑制计数和结束汇总；不是全局限流或可靠审计 |
| FIFO 日志、文件滚动及失败收尾 | [日志顺序](../logging/logger_test.go)、[文件](../logging/file_test.go)、[输出器生命周期](../logging/writer_lifecycle_test.go) | 覆盖短写、Flush 故障、保留历史和借用所有权；不保证 fsync、磁盘崩溃恢复或云端收到 |
| Service 内 MySQL 产品 CRUD 示例 | [业务 Service 测试](../examples/mysql-product/service_test.go)、[入口与可选真实 MySQL 测试](../examples/mysql-product/main_test.go) | 默认使用 SQL mock；显式启用的本机专用库测试已完成真实驱动的创建、查询、更新和删除，不覆盖生产部署、并发库存一致性或断线后写入结果判定 |
| Service 内 Redis Hash 缓存示例 | [缓存测试](../examples/redis-user-cache/cache_test.go)、[业务 Service 测试](../examples/redis-user-cache/service_test.go) | 使用自动启动的本机临时 Redis 验证 Hash、TTL、故障和 Service 隔离；缓存不是权威数据源，不覆盖跨节点一致性 |
| Service 内 HTTP 出站示例 | [资料刷新测试](../examples/http-outbound/service_test.go) | 使用本机测试 HTTP 服务验证有界等待、错误映射和状态保留；不覆盖真实第三方服务或自动重试策略 |
| 基础设施 HTTP 入站示例 | [房间人数接口测试](../examples/http-inbound/handler_test.go)、[管理路由业务测试](../examples/http-inbound/admin_test.go) | 使用本机 HTTP 验证只读查询、有界 Call、安全错误响应，以及 `gsr.Run` 的三条 REST 路由经本地业务 Service 汇总真实 TCP 远端结果；不提供认证或公网接入能力 |

## 当前检查结果

2026-10-08 简化公开概念与默认接线：

- 推荐入口统一为 `gsr.Run + MainService`，HTTP 接线移到 `examples/http-inbound/main_service.go`；原三条管理路由及跨节点汇总测试继续使用这份源码。
- 删除独立公开 ConnectionAgent、RequestHandler/ResponseWriter 及其配置和错误；内存往返、匿名回复和字节流测试改用生产 localRequestAdapter 与真实 Runtime。保留 Call 返回值、Domain Error、未知错误、panic 安全转换、Send 错误归属、Session 分离和原连接回复覆盖；writer panic 与非法请求检查迁到现有适配器测试，不为已删除对象保留构造测试。
- 协议 Dispatcher 构造改为 Registry + ClientSender，内部建立同目录解码/写回不变量；新增缺失依赖及独立强类型 Push 用例，原 Response/Notify/错误语义测试继续复用。Echo 与两种 Gate 的协议接线均已更新，业务 wire 格式不变。
- 延长 OnStart 发布窗口的测试先复现 DebugGate 正常连接被 1011 关闭，修复后正常收发；两种 Gate 共用有界接纳重试。扩展原创建失败用例，复现 OnStart 返回 MailboxFull/ServiceNotFound 被误重试的问题；失败改为终止性内部错误并保留原始日志，两种 Gate 的创建失败及迟到创建清理验证通过。
- 定向回归、一次 `go test ./... -timeout=5m`、最终协议包全测、相关 Cluster/Gate/协议/HTTP 接线定向 race 和 diff 检查通过。本轮未执行全仓 vet/race、跨进程故障演练、TypeScript 或生产压力测试；客户端 wire 和 TypeScript 代码未修改。

2026-10-01 应用名称表跨节点收尾：`TestRunNodesCommunicateWithoutSingleton` 增加真实 TCP 调用后的目标 Service 退出检查；远端首次按名 Call 正常返回，目标 `OnStop` 完成后再次按同名 Call 得到 `ErrServiceNotFound`，不再解析到旧 ServiceID。该定向测试的 race 检查通过；本轮只扩展验收，不修改生产实现，也不代表动态发现或在途请求迁移。

同日应用入口故障收尾补强：`TestRunConstructionRollback` 分别覆盖 Runtime 配置无效、Cluster 配置无效和 typed-nil 入口 Service，确认构造错误可识别、组装失败不运行业务生命周期、共享 Logger 最终排空；`TestRunCleanupAfterOpen` 在已创建子 Service 且 Open 后覆盖 OnStart 返回错误或 panic、正常停机时子 Service 返回错误、入口 OnStop panic，确认原错误与收尾错误均保留、子 Service/入口各停止一次、日志顺序及监听端口释放。两项定向 race 测试通过；只加测试，不修改生产实现。

同日应用 Open 组装边界：已有 Cluster 测试覆盖占端口后重试，已有 Run 测试覆盖未知节点、重复 Open 和退出释放监听；新增 `TestRunOpenRetryAfterBindFailure` 只验证按节点名 Open 在真实绑定失败后不发布 `ListenAddress`，释放占用后同一应用可重试成功。夹具使用与生产 Open 相同的 TCP 地址解析，避免 IPv4 与 IPv6 通配地址不相冲突造成假阴性。定向 race 测试通过；不重复扩大底层测试矩阵。

同日补充同名重绑定：旧目标退出且远端确认名称失效后，仍在运行的 Service 创建并注册新目标；远端在原连接上再次按名称 Call，得到新实例回复。定向 race 测试通过；只验证后续请求重新解析，不迁移已经发出的请求。

同日补充名称注册与退出竞争：`TestServiceNameRegistrationRacesCleanup` 同时发起 128 次注册和目标清理，允许注册在清理前成功或在清理后被拒绝；全部结束后确认名称及反向索引无残留，已退出 ID 不能再次注册。定向 race 测试重复 10 次通过；这是名称表同步边界测试，不代表完整 Service 生命周期压力。

同日补充远端按名 Send：同名新实例注册后，调用端在原连接上 Send，测试等待新目标 Handler 明确处理，再执行原有按名 Call；真实 TCP 定向 race 测试通过。Send 的 nil 返回仍仅表示本地接纳，成功处理是本测试的额外观测，不改变协议保证。

2026-09-15 名称映射切片：权限恢复后 TestClusterNameRebindingAfterServiceFailure 的真实 TCP 测试及 race 均通过，关闭此前审核服务容量故障造成的未验证项。通过公开 Open 的外层 ResolveService 和 ClusterClient.Call 验证故障实例、未更新映射的错误、新实例映射发布及旧 ID 不误投。仅测试使用原子值模拟外层注册表，不新增发现能力，不承诺映射变更会迁移在途请求；也不将该正确性验证当作名称查询的性能测量。

2026-09-15 Mailbox 背压切片：TestTCPCallMailboxFullAndRecovery 使用真实 TCP Call，确定性填满默认容量 Mailbox，检查满载结构化错误、健康目标隔离、已接纳消息按序排空、拒绝消息不执行、原目标恢复，以及两端 PendingCall 清理。Runtime admission 拒绝计数恰为一次，不改容量、不自动重试、不修改生产实现。

2026-09-15 迟到回复切片：TestTCPCallCancellationDiscardsLateReply 使用 channel 控制远端 Handler 进入与释放，确认调用方取消后先恢复、其他 Service 可在旧 Handler 返回前完成 Call；随后等待真实迟到响应的诊断，再验证新 Call 的结果及原连接继续可用。只有测试使用阻塞控制，不新增生产 API、不强杀 Handler、不改变取消或回复协议，也不重复运行持续压力测试。

2026-09-15 Call 故障切片：TestTCPCallServicePanicAndReplacement 默认验证一次完整故障/重建链路；`GSR_CALL_STABILITY=1` 额外持续两分钟，每 10ms 尝试一轮串行闭环检查（迟到不追赶）。首轮 120.001 秒完成 11,803 轮：旧 ID 预期返回 ErrServiceClosed 11,803 次，健康 Service 与新实例合计成功回复 23,606 次，无意外错误、旧 ID 误投或 PendingCall 残留。最初 panic 仅执行一次，并校验远端 ErrServicePanicked 不含私有 panic 文本；这些数值不包括最初的一次性检查。未修改生产代码。

这是故障后的持续正确性检查，不是两分钟一万条/秒的 Call 容量测试；同一测试进程内走真实 TCP，并非前一轮独立进程混合负载。默认行为测试及其 race 已通过，持续部分以非 race 运行。专项允许 150 秒命令超时以容纳两分钟观察及初始化/清理，不扩大普通测试门禁。

```sh
go test -race ./cluster -run '^TestTCPCallServicePanicAndReplacement$' -v -timeout=2m
GSR_CALL_STABILITY=1 go test ./cluster -run '^TestTCPCallServicePanicAndReplacement$' -v -timeout=150s
```

2026-09-15 规模切片：已实际运行现有空闲 Service/Timer 规模测试及固定次数 benchmark，结果见 [本地规模基线](PERFORMANCE-BASELINE.md)。这是单机小样本，不替代持续负载或生产验收；该切片未修改 Go 代码。

2026-09-28 Gate 多慢连接背压样本：`TestMACGateMultipleSlowClientsDoNotBlockHealthyRequests` 在本机以四条不读取数据的客户端和一条健康连接运行 5 秒。四条慢连接均成功写入 12 条后持续遇到有界队列拒绝，健康连接完成 50 次请求/响应；合计 `OutboundQueueFull=19857`，`RejectedConnections=0`、`InboundMailboxFull=0`，五个 Agent 均收到断开通知。定向测试与对应 `-race` 检查通过；这是短时行为验收，不是目标部署容量或长时稳定性结论。

2026-09-22 WebSocket Gate 短时压力切片：在 `GSR_GATE_STRESS=1` 下运行 3 秒慢客户端、16 连接持续发送和 8 轮坏握手/坏 MAC。慢客户端发送 12 条、出站队列满载拒绝 2,976 次；16 连接发送与 Agent 处理均为 4,622 条，`InboundMailboxFull=0`、连接拒绝为 0；失败阶段 8 次全部拒绝且 8 条已认证连接均正常回收。命令为 `GSR_GATE_STRESS_SECONDS=3 GSR_GATE_SLOW_CLIENT_SECONDS=3 GSR_GATE_FAILURE_ATTEMPTS=8 go test ./examples/websocket-gate -run '^TestMACGate(LongSlowClientBackpressure|MixedConnectionStress|FailurePhaseStress)$' -count=1 -v`。这是本机短样本，只验证有限队列、连接回收和失败阶段的当前行为，不构成多连接长时容量结论。

同日 30 秒长样本：16 条连接发送与 Agent 处理均为 47,586 条，`InboundMailboxFull=0`、连接拒绝为 0；慢客户端样本先观察到队列满载，随后因写超时关闭连接，`ClientSession.Send` 返回 `ErrClientSessionClosed`，按连接隔离契约计为正常终止，不计为业务错误。

同日 2 分钟多连接样本：`GSR_GATE_STRESS=1 GSR_GATE_STRESS_SECONDS=120 go test ./examples/websocket-gate -run '^TestMACGateMixedConnectionStress$' -count=1 -v -timeout=150s` 使用 16 条连接，发送与 Agent 处理均为 189,983 条，`InboundMailboxFull=0`、连接拒绝为 0，测试耗时约 120 秒。该样本证明当前机器和测试负载下连接/Agent 链路可持续运行，不是部署容量、长期稳定性或生产限流结论。

2026-09-23 Gate 压力验收断言收紧：多连接测试现在等候最多 5 秒排空，并要求客户端成功写入数与 Agent 处理数相等、Mailbox 满载和连接拒绝均为零；失败阶段测试在坏握手与坏 MAC 后额外执行一次健康请求/响应。3 秒短样本中非 race 处理 4,784 条、race 处理 4,783 条，均与各自成功写入数相等；两轮 Mailbox 满载与连接拒绝均为零。失败阶段各重复 3 次坏握手与坏 MAC，计数为 `rejected=3`、`accepted=4`，随后健康往返成功。目标测试、对应 race、prodgate 全包和 vet 通过；这次仅加固验收，不改生产 Gate。

同日慢连接隔离切片：`TestMACGateSlowClientDoesNotBlockHealthyConnection` 持续向不读取数据的客户端发送，确认出站队列已发生满载后，在压力窗口内通过另一条真实 WebSocket 连接完成认证请求、Agent 投递及 Protobuf 响应往返；慢连接持续发送至测试结束，校验其成功发送和队列拒绝计数均大于零、连接未提前关闭。专项重复 10 次、prodgate 全包和全仓 race 通过；只验证短时双连接隔离，不作为慢网络长时或容量测试，未修改生产 Gate。

同日跨连接重放切片：`TestMACGateRejectsCrossConnectionReplayBeforeDelivery` 先在源连接发送并成功处理一条认证请求，再将完全相同的 `seq | Envelope | MAC` 帧送到另一条独立握手连接。目标连接以 `1008 policy_violation` 关闭，目标 Agent 未收到该帧；第三条新连接使用自己的密钥和序号 1 成功完成请求/响应。专项重复 10 次、prodgate 全包、全仓测试、全仓 race 和 vet 通过；只验证当前 Gate 实现的连接间帧隔离，不扩展为客户端身份认证或业务防重。

同日非法 X25519 握手集成切片：`TestMACGateRejectsInvalidX25519KeyAndRecovers` 通过真实生产 Listener 发送全零低阶公钥，连接上限设为 1，确认握手以协议错误关闭、Agent 未创建、连接计入拒绝；随后有效客户端仍能占用归还的名额并完成认证请求/响应。专项重复 10 次、prodgate 全包、对应 race 通过；只验证握手拒绝与容量恢复，不构成密码协议审计。

同日同连接重放切片：`TestMACGateRejectsSameConnectionReplayBeforeDelivery` 先在生产 Gate 上成功处理 seq=1 请求并读取响应，再重发完全相同的认证帧。Gate 以 `1008 policy_violation` 关闭连接，Agent 未重复收到消息并收到断开通知。专项重复 10 次、prodgate 全包、全仓测试、全仓 race 和 vet 通过；和跨连接重放测试一起覆盖生产 Gate 的两种旧帧拒绝路径，不代表完整安全审计。

同日认证后超限帧切片：`TestMACGateRejectsOversizedAuthenticatedFrame` 在完成握手后发送完整长度超限的认证业务帧，确认 Gate 返回 `1009 message_too_big`、累计 `FrameTooLarge`、不投递 Agent 并通知断开；随后新连接可正常完成请求/响应。专项重复 10 次、prodgate 全包、全仓测试、全仓 race 和 vet 通过；只验证单连接帧上限与故障恢复，不是吞吐或内存压力测试。

同日认证后短帧切片：`TestMACGateRejectsMalformedAuthenticatedFrame` 在完成握手后发送短于序号与 MAC 最小长度的二进制帧，确认 Gate 返回 `1002 protocol_error`、不投递 Agent 并通知连接断开。定向测试 `go test ./examples/websocket-gate -run '^TestMACGateRejectsMalformedAuthenticatedFrame$'` 通过；本轮只新增独立集成测试，未重复运行全包或全仓验证。

同日慢客户端回压长窗口试验：设置 `GSR_GATE_SLOW_CLIENT_SECONDS=120` 后，慢客户端成功写入 12 条，队列累计拒绝 9,994 次；Gate 在约 15 秒时因写超时关闭连接，Agent 收到稳定关闭结果。定向压力测试 `GSR_GATE_STRESS=1 GSR_GATE_SLOW_CLIENT_SECONDS=120 go test ./examples/websocket-gate -run '^TestMACGateLongSlowClientBackpressure$' -timeout=130s -v` 通过。这里的 120 秒是 Agent 压力循环的最长窗口，并非实际 soak 时长；本次验证写超时与有界回压，不代表两分钟慢客户端稳定性验收。

同日失败阶段压力扩展：设置 `GSR_GATE_FAILURE_ATTEMPTS=100`，连续执行 100 次坏握手和 100 次坏 MAC；故障连接未投递坏帧，随后健康连接仍完成请求/响应。测试耗时约 1.52 秒，指标为 `rejected=100`、`accepted=101`、`inbound_mailbox_full=0`。定向测试 `GSR_GATE_STRESS=1 GSR_GATE_FAILURE_ATTEMPTS=100 go test ./examples/websocket-gate -run '^TestMACGateFailurePhaseStress$' -timeout=90s -v` 通过；这是短时恢复压力样本，不代表长时故障洪泛容量验收。

同日失败阶段 1,000 轮扩展：连续执行 1,000 次坏握手和 1,000 次坏 MAC 后，健康连接仍完成请求/响应，指标为 `rejected=1000`、`accepted=1001`、`inbound_mailbox_full=0`；测试耗时约 15.38 秒。定向测试 `GSR_GATE_STRESS=1 GSR_GATE_FAILURE_ATTEMPTS=1000 go test ./examples/websocket-gate -run '^TestMACGateFailurePhaseStress$' -timeout=90s -v` 通过。连接按序执行，本结果不代表并发故障洪泛容量或长时稳定性。

2026-09-23 失败阶段 3,000 轮扩展：连续执行 3,000 次坏握手和 3,000 次坏 MAC 后，健康连接仍完成请求/响应，指标为 `rejected=3000`、`accepted=3001`、`inbound_mailbox_full=0`；测试耗时约 46.39 秒。定向测试 `GSR_GATE_STRESS=1 GSR_GATE_FAILURE_ATTEMPTS=3000 go test ./examples/websocket-gate -run '^TestMACGateFailurePhaseStress$' -v` 通过。连接按序执行，本结果不代表并发故障洪泛容量或长时稳定性。

2026-09-23 失败阶段 5,000 轮扩展：连续执行 5,000 次坏握手和 5,000 次坏 MAC 后，健康连接仍完成请求/响应，指标为 `rejected=5000`、`accepted=5001`、`inbound_mailbox_full=0`；测试耗时约 76.22 秒。定向测试 `GSR_GATE_STRESS=1 GSR_GATE_FAILURE_ATTEMPTS=5000 go test ./examples/websocket-gate -run '^TestMACGateFailurePhaseStress$' -timeout=110s -v` 通过。连接按序执行，本结果不代表并发故障洪泛容量或长时稳定性。

2026-09-23 并发故障隔离：8 条已认证坏 MAC 连接同时发送故障帧期间，健康连接完成 16 次请求/响应，坏帧均未进入 Agent，`inbound_mailbox_full=0`。定向测试与对应 `-race` 测试均通过；此样本验证有限并发下的连接隔离，不代表高并发容量。

2026-09-23 并发非法握手隔离：8 条连接同时提交全零低阶 X25519 公钥并按协议错误关闭时，健康连接完成 8 次认证请求/响应；无效握手未创建 Agent，`rejected=8`、`inbound_mailbox_full=0`。专项重复 5 次、对应 `-race` 测试和 prodgate 包全测通过；只验证有限并发下握手失败的连接隔离，不代表高并发容量或密码协议审计。

2026-09-23 Agent 构造器 panic 恢复：构造器 panic 只拒绝当前连接并返回 WebSocket 内部错误关闭；后续连接成功创建 Agent 并完成请求/响应。定向测试和对应 `-race` 测试均通过。

2026-09-23 Agent 启动失败恢复：Agent `OnStart` 返回错误时 Runtime 执行该实例的 `OnStop` 清理，Gate 拒绝当前连接后仍能接受后续连接并完成请求/响应。定向测试、对应 `-race` 测试和 prodgate 包全测均通过。

2026-09-23 Gate 监听启动失败恢复：占用监听地址时创建 Gate Service 失败但 Runtime 继续可用；释放地址后可在同一 Runtime 创建并启动新的 Gate。定向测试、对应 `-race` 测试和 prodgate 包全测均通过。

2026-09-23 Gate 关闭资源回收：`Runtime.Close()` 关闭活动客户端并等待 Gate I/O goroutine 退出，之后 Agent 的 `ClientSession.Send` 返回 `ErrClientSessionClosed`，且测试立即成功重绑原监听地址。相关关闭与 Agent 启动失败用例 race 检查、`prodgate` 包全测通过；两类服务端主动关闭连接的 TLS 清理用例重复运行 10 次通过。

2026-09-23 Gate Stop 资源释放：单独调用 `Runtime.Stop()` 后可立即重绑 Gate 原监听地址，再调用 `Runtime.Close()` 完成最终释放。定向测试、对应 `-race` 测试和 prodgate 包全测均通过。

2026-09-23 Stop 中断待握手连接：Gate 握手超时设为 30 秒、客户端完成 WebSocket Upgrade 后不发送应用握手；`Runtime.Stop()` 在 2 秒门限内主动发送 `going_away` 并完成收尾，握手超时数和已接受连接数均为零，Agent 未创建。专项重复 3 次、对应 `-race` 测试和 prodgate 包全测通过。

2026-09-24 并发待握手连接 Stop：8 条已完成 WebSocket Upgrade、尚未发送应用握手的连接同时由 `Runtime.Stop()` 关闭；8 条均收到 `going_away`，Gate 等待所有处理 goroutine 退出，`HandshakeTimeouts=0`、`AcceptedConnections=0`、`RejectedConnections=8`，Agent 未创建。专项重复 3 次、对应 `-race` 测试和 prodgate 包全测通过。

2026-09-24 并发活动客户端 Stop：8 条已认证连接由 `Runtime.Stop()` 同时关闭，每条都收到 `going_away`，对应 `ClientSession.Send` 均返回 `ErrClientSessionClosed`；指标为 `AcceptedConnections=8`、`RejectedConnections=0`。专项重复 3 次、对应 `-race` 测试和 prodgate 包全测通过。

2026-09-24 Agent/Gate 幂等关闭：活动连接关闭时，Gate 的 `OnStop` 和 Agent 的 `OnStop` 均关闭同一个 `ClientSession`；`Runtime.Stop()` 成功完成，客户端连接关闭，重复调用 `ClientSession.Close()` 返回成功。专项重复 5 次、对应 `-race` 测试和 prodgate 包全测通过。

2026-09-23 Gate 连接容量回收：连接上限为 1 时，未完成 TLS 握手的 TCP 连接占用名额，超限连接被关闭；占用连接关闭后，新的 WSS 连接可完成 HTTP Upgrade。定向测试重复运行 10 次、对应 `-race` 测试和 prodgate 包全测均通过。

2026-09-23 握手超时容量回收：连接上限为 1 时，已完成 WebSocket Upgrade 但未完成应用握手的连接继续占用名额，期间第二条 TCP 连接被拒绝；首条握手超时关闭后，后续 WSS 客户端完成认证和请求/响应。专项重复 5 次、对应 `-race` 测试和 prodgate 包全测均通过。

2026-09-23 上行序号耗尽边界：Go 服务端接纳一次 `uint64` 最大值序号后拒绝回绕到 1；TypeScript 客户端能生成最大序号帧，之后保持序号不变并返回耗尽错误。Go 专项测试与 `npm run test:crypto`（含 TypeScript 编译）通过。协议规定上限后结束当前连接并重新握手；本切片只补测试和说明，不改生产算法。

2026-09-23 C/S 请求编号上限：协议分发测试将最大 `uint32 request_id` 编码进 Envelope，确认 Handler 收到原值且 Response 原样回传。该测试、protocol 全包测试和 vet 通过；客户端 request_id 分配器仍由业务客户端实现，示例 CLI 固定使用 1。

同日 Agent 内部超时切片：`TestMACGateReturnsErrorAfterAgentCallTimeout` 通过真实生产 Gate 验证 Agent 调用本地目标 Service 超时后，客户端收到原 `request_id` 的 `INTERNAL`；目标 Service 释放后产生的迟到 Core 回复不会再次写回客户端。重复运行 5 次、prodgate 全包、vet 和 race 均通过。该切片只确认 Agent/Runtime 的内部 Call 超时归属，不把它扩展为客户端业务请求的统一超时或重试策略。

后续热点切片新增 TestRuntimeHotMailboxLoad（GSR_LOAD_TEST=1 显式启用）：4 个发送者、10 万次固定尝试、容量 64/1024，逐条核对接纳与处理结果并记录到达延迟分位数。它是短突发测试，不替代下文的持续负载验收；未修改生产实现。

Timer 集中到期切片新增 TestRuntimeTimerBurstLoad，沿用 GSR_LOAD_TEST=1：10,000 个 Timer 同轮到期，校验接纳/拒绝计数、消息唯一性和零 Source。实测见规模基线；使用 fake clock，不代表真实时钟精度或定时业务必达保证。

日志测量切片新增 TestLoggingBatchAccounting 与 BenchmarkLoggingBatch：比较禁用、io.Discard、临时文件输出，分别报告成功接纳、满载丢弃及关闭排空成本。本次固定小字段单发送者样本无丢弃；多生产者、慢盘和真实业务日志频率仍未测量。

后续 TestLoggerConcurrentFileLoad 补充 8 个生产者共享文件 Logger 的 10 万条突发，逐条核对接纳、唯一性、文件顺序与丢弃计数。首轮写入 97,097 条、满载丢弃 2,903 条，符合有界非阻塞契约但不代表零丢日志容量达标；慢盘、真实 Service 组合和持续负载仍未覆盖，具体口径见性能基线。

日志隔离切片新增 TestRuntimeLoggingFailureIsolation：受控阻塞 Writer 填满队列以及终止输出失败时，业务返回值、领域错误、Runtime 独立诊断与资源关闭均不被日志基础设施改变。测试不包含真实慢盘性能测量；业务若主动将日志错误返回给调用方，仍属于业务自己的选择。

双进程示例切片补充：已在临时工作目录启动两个独立进程，观察 Send 接收和 Call 成功；停止接收端后调用端继续运行并记录 connection refused，重启接收端后后续新操作恢复成功；最后两个进程均以退出码 0 结束并写出 process stopped。此为本机手动演练，不等同于自动化跨进程门禁或长期负载测试。

- 全仓 `go test ./... -timeout=5m` 与 `go vet ./...` 在前一测试切片通过；本轮仅整理文档，没有改动 Go 代码。
- 本轮补跑 `go test -race ./... -timeout=5m` 通过，关闭了前一轮权限审核超时导致的未验证项；部分未变化 package 使用 Go 测试缓存。
- `git diff --check` 用于文档与补丁格式检查。
- 前述故障验证切片未运行 benchmark；后续 2026-09-15 规模切片的独立测量见本节链接，不把普通测试总耗时当作性能结果。

## 仍需单独验证的范围

当前用户暂定吞吐目标：单进程 10,000 条消息/秒。已新增 TestRuntimePaced10000Load，10 秒样本实际输入约 9,861 条/秒、零满载拒绝，不能据此宣称严格达到目标或完整业务容量验收通过；具体口径见性能基线。

后续逐条文件日志对照的实际输入降至约 6,422 条/秒，出现 200 条 Mailbox 满载拒绝与 332 条日志丢弃；同期关闭日志项也仅约 8,237 条/秒且存在较大尾延迟。保留全部结果，不以最初较好样本代替当前证据；尚不能单独归因于日志，也未达到容量验收条件。

再补生成器唤醒/批次及 GC 聚合观测后，同一生产实现的新样本文件模式约 9,989 条/秒、无拒绝或日志丢弃；旧抖动未复现。该结果不构成性能修复或根因定位，完整原始指标与限制见性能基线。

已验证 Go 原生 `-trace` 对单个文件日志定速子项的采集及完整解析，命令与判读边界见性能基线。采集样本计数守恒且无丢弃，未复现旧抖动；它只验证诊断入口，不替代无采集开销的容量测量。

TestRuntimePacedMultiServiceLoad 已补 8 个真实 Service 共享文件 Logger 的总量定速场景：实际约 9,959 条/秒，每个 Service 正确处理 12,500 条，总计 100,000 条且日志无丢弃。它是无业务计算的 10 秒样本，不代表每个 Service 一万条、长期容量或 Service 内 Call 负载；完整指标见性能基线。

1. **独立进程运行链**：已有 [双进程示例](../examples/twonode/README.md) 和显式启用的 TestTwoNodeProcessRecovery、TestTwoNodeProcessCrashRecovery 门禁，覆盖独立配置、日志目录、Send/Call、正常停机或 SIGKILL 后调用端继续运行与重启后的新请求恢复。强杀发生在首轮 Call 完成后，不覆盖在途执行或日志持久性。默认集成测试仍主要在单进程内运行；跨进程测试不覆盖命令行参数解析、跨机器部署或网络分区。
2. **持续负载和容量**：已有空闲规模基线，但持续热点调用、日志慢盘、Timer 集中到期等组合负载还需目标机器上的容量测算。不能据此决定预先更换 Scheduler 或 Timer 架构。
3. **部署与运维**：进程退出信号、外部日志采集器、磁盘配额与告警还没有完整部署验收；SLS 属于外层采集，不给 Core 添加云 SDK。
4. **安全与发现**：认证协议、动态发现仍按已有设计边界留待实际需求。当前可信内网模式不提供对端身份保证，不能直接当成公网服务使用。

## 下一步建议

用户确认的 10,000 个 Service、9,800 Send/s、80% 本地 + 20% 跨节点、5 秒预热后正式运行 3 分钟已完成。权限恢复后短跑、短跑 race 及正式计数/关闭检查通过：正式处理 176.4 万条，MailboxFull 为零，各组 P99 为 0.874～1.242ms，目标平均使用 0.0476 核。完整资源数据、尖峰与测量边界见 [混合负载验证](MIXED-LOAD-VALIDATION.md)。可暂不优化 Scheduler，转回正确性与业务迁移；这不是重 Handler、多核扩展或生产内网容量的证明。

基础空载测量已包含 Send、Service 内本地 Call、Timer 突发、单/多 Service 定速共享日志及多生产者日志突发。本地 Call 最新固定次数复核约 1.496μs/op，但不是每秒一万条定速或跨节点容量验收。已完成的 80/20 混合场景全部是 Send；下一项容量工作仍需由业务确定 Call 占比、处理成本、日志频率和验收门槛，不继续以重复空 Handler 样本替代业务验收。

最小双进程运行示例及启动说明已提供：应用用启动参数选配置，每个进程配置独立日志目录，调用端无需 Open，接收端显式 Open；业务只演示 Send 与 Call，资源按既有顺序关闭。

这一步先证明框架能被应用清楚地组装和运行，不引入通用配置框架、发现服务、网关或新 Runtime 生命周期 API。自动化跨进程正常停机、强杀与重启恢复门禁已补充；持续负载、实际部署和其它故障场景仍需独立验收，不宣称生产验收完成。

2026-09-24 Gate 断开收尾切片：新增 `TestMACGateAgentStopAfterClientDisconnectKeepsServing`，验证客户端先断开后 Gate 关闭 `ClientSession`、Agent 收到断开通知并在 `OnStop` 再次关闭时不报错；随后第二个客户端仍能完成认证、请求/响应往返。定向测试重复 5 次、定向 race 重复 3 次及 `go test ./examples/websocket-gate` 均通过；只增加测试和文档，未修改生产实现或运行全仓验证。

2026-09-24 Gate 并发停机切片：新增 `TestMACGateClientDisconnectOverlapsRuntimeStop`，并发触发客户端断开和 Runtime.Stop，验证运行时停机完成、Agent.OnStop 只执行一次、重复关闭后写回返回 `ErrClientSessionClosed`。定向测试重复 10 次、定向 race 重复 5 次、`go test ./examples/websocket-gate` 与该 package 全量 race 均通过；只增加测试和文档，未修改生产实现或运行全仓验证。

2026-09-24 Gate 停机错误切片：生产 Gate 与 DebugGate 原先都会忽略 `ClientSession` 底层真实关闭失败，导致 Runtime.Stop 无法报告该资源错误。两处 OnStop 现在聚合并返回各连接的关闭错误，继续处理后续清理；新增 `TestMACGateStopReportsClientCloseFailure` 和 `TestPlainGateStopReportsClientCloseFailure` 通过注入返回关闭错误的测试连接验证。定向测试重复 3 次，`go test ./examples/websocket-gate ./examples/websocket-gate`、对应两包全量 race 与 `go vet` 均通过；不改变正常断开错误归一化规则，未运行全仓验证。

2026-09-24 Gate 多连接错误聚合切片：扩展两个停机测试，为两条连接分别注入不同 Close 帧写错误和 socket close 错误，确认 Gate 关闭所有连接且 `OnStop` 返回的错误保留四类原因。定向测试重复 3 次、`go test ./examples/websocket-gate ./examples/websocket-gate` 与对应两包全量 race 均通过。只强化测试与文档，没有生产代码改动。

2026-09-24 Gate Close 帧错误切片：生产 Gate 和 DebugGate 现在同时保留 WebSocket Close 帧写入与底层 socket 关闭的真实错误；对端已断开错误仍归一化为正常收尾。两个定向测试同时注入 Close 帧写入和 socket 关闭错误，确认 `errors.Is` 可分别识别两种原因。

2026-09-24 Gate 停机关闭错误聚合复核：扩展生产 Gate 和 DebugGate 的停机测试，在两条连接上同时注入各自的 Close 帧写入错误与 socket 关闭错误，确认 `OnStop` 通过 `errors.Is` 保留所有原因并继续关闭全部连接。两个定向测试重复 3 次、对应定向 race 重复 3 次通过；只扩展测试与文档。

2026-09-24 Gate 出站写失败可观测性：生产 Gate 的连接写循环现在将 deadline 设置和 WebSocket 写入错误返回给 Gate，由现有连接级错误日志记录；不改变 `ClientSession.Send` 仅表示队列接纳的语义。定向测试、prodgate 全包测试和全量 race 均通过，日志字段保留错误原因。

2026-09-24 Gate 写 deadline 错误复核：Gorilla `Conn.SetWriteDeadline` 只保存 deadline 并始终返回 nil，内部写入时底层 deadline 错误也会被忽略。生产 Gate 现在在 WebSocket 握手、客户端写循环和 Close 帧写入前显式检查底层连接的 deadline 设置结果；DebugGate 的写循环和 Close 帧也检查该结果。失败会关闭连接，生产 Gate 还会记录连接故障。定向测试分别注入底层 deadline 与消息写入错误，prodgate、debuggate 全包测试及两包 race 均通过；未运行全仓。

2026-09-24 Gate 握手写 deadline 测试：新增生产握手路径定向测试，注入底层 socket 写 deadline 失败，确认握手响应不再尝试写入，错误保留且连接关闭。定向测试及对应 race 测试通过。

2026-09-24 Gate 握手读 deadline 测试：新增定向测试模拟底层读 deadline 设置失败，确认握手保留错误并立即以内部错误码关闭连接。prodgate 全包测试及全包 race 均通过。

2026-09-24 Gate 握手 deadline 清理测试：注入握手响应发送后的读 deadline 清除失败，确认 33 字节响应先送达，随后连接以内部错误码关闭，避免遗留握手 deadline 影响业务读循环。prodgate 全包测试和新增用例 race 通过；全包 race 在既有并发坏握手测试中读到 7/8 条拒绝计数后失败，待单独核对该测试的计数观察时序。

2026-09-24 Gate 并发握手计数同步：客户端读到关闭帧时，ServeHTTP 可能尚未更新拒绝计数；测试改为通过现有等待辅助函数观察稳定计数。定向 race 与 prodgate 全包 race 均通过。

2026-09-24 Gate 握手响应写失败测试：注入底层 socket 写错误，确认握手返回原始错误且客户端收不到响应帧。定向测试、prodgate 全包测试及定向 race 均通过。

2026-09-24 Gate 超大握手计数断言：超限握手集成测试现在同时确认帧超限、连接拒绝、未接纳且未创建 Agent。定向测试、prodgate 全包测试及该用例 race 均通过。

2026-09-24 Gate 非法握手接纳计数：非二进制及格式非法握手测试也明确断言连接未计入接纳数。两个定向测试、prodgate 全包测试及定向 race 均通过。

2026-09-24 Gate 断开通知持续重试：生产 Gate 不再因 5 秒上限放弃 Agent Mailbox 满载时的断开通知；连接处理协程在通知入队前继续占用 `MaxConnections` 名额，使待处理数量有界。DebugGate 改用 Infrastructure Call 确认 Agent Mailbox 接纳，并保留满载时的连接映射；单独停止 Gate 时，`OnStop` 在其生命周期上下文内重试通知剩余 Agent。生产回归测试将 Mailbox 满载保持超过旧 5 秒上限后再恢复，DebugGate 集成测试覆盖 Mailbox 恢复后收到通知。两 Gate package 全测与对应 race 均通过；未运行全仓验证。

2026-09-25 Gate 停机通知公平性：修复 Gate 单独停止时逐个等待 Agent 的队头阻塞；现在每轮都会尝试所有待通知 Agent，仅将 Mailbox 满载者留到下一轮。生产 Gate 与 DebugGate 新增双 Agent 测试，确认一个 Agent 卡住时健康 Agent 仍及时收到断开通知，随后满载 Agent 恢复也能完成收尾。两个 package 全量测试与 race 均通过；未运行全仓验证。

2026-09-25 Gate 停机入站屏障：客户端帧接纳现在与 Gate 的停机状态切换共享读写锁；停机标记生效后，生产 Gate 与 DebugGate 都拒绝继续投递帧，避免逐条关闭连接期间仍有新消息进入 Agent Mailbox。两个定向测试、两包全量测试及仓库级 `go test -race ./...` 均通过。

2026-09-25 生产 Gate 握手帧配置下限：配置的 `MaxFrameBytes` 小于 33 字节时无法容纳固定客户端握手帧；`NewGateService` 现在在启动前返回 `ErrInvalidConfig`，并增加边界测试覆盖 32 字节拒绝、33 字节接受。定向测试和 prodgate 全包测试通过。

2026-09-28 Gate HTTP 空闲连接回收：普通 HTTP 请求返回 404/405 后，keep-alive 连接此前可能无限占住 `MaxConnections` 名额。生产 Gate 为未升级 HTTP 连接设置 5 秒 idle timeout；新增真实 TCP 测试确认 404 响应仍可 keep-alive，随后连接按时关闭。定向测试与 `go test ./examples/websocket-gate` 通过；WebSocket 升级连接不受该 idle timeout 影响。

2026-09-28 DebugGate HTTP 空闲连接回收：真实 TCP 回归测试先确认 404 后的 keep-alive 连接超过 5 秒仍不关闭，再验证设置 5 秒 idle timeout 后按时关闭。定向测试与 `go test ./examples/websocket-gate` 通过。DebugGate 的 `MaxConnections` 只统计进入 `/ws` GET Handler 的连接，仍不限制原始 TCP 接纳数；它只供受信任环境调试。

2026-09-28 Gate 停机断线通知交接：两种 Gate 的连接处理协程原先在 Agent Mailbox 满载时无限重试断线通知，`OnStop` 等待协程退出便可能越过自身 context 期限。两条受控测试先复现该阻塞；现在停机标记生效后连接协程结束重试，由 `OnStop` 对剩余映射按有界上下文补发。定向测试、两 Gate 包全测及对应包的 race 测试均通过；满载 Agent 在期限内仍不接纳时，`OnStop` 按预期返回 deadline 错误。

2026-09-28 DebugGate 连接拒绝计数：Agent 构造失败时，Gate 内部创建路径与 HTTP 入口原先各计数一次，一条连接得到 `RejectedConnections=2`。回归测试先复现，再验证构造返回 nil 和 Agent `OnStart` 失败都只计一次；定向测试与 DebugGate 包测试通过。

2026-09-28 Gate Origin 部署约束（历史记录，已由下一条裁决取代）：生产 Gate 与 DebugGate 的真实 WebSocket 升级测试确认，浏览器式跨主机 `Origin` 收到 HTTP 403，同 Host 的 `Origin` 可以升级；这是当时 WebSocket 库的默认策略，非应用密钥握手错误。两条定向测试及两 Gate 包测试通过；当时尚未新增配置或放开来源。

2026-09-28 Gate Origin 接入裁决：用户决定不以 `Origin` 作为安全边界，两种 Gate 均显式允许任意来源，保留原生、小程序和 Web 客户端一致的接入语义。业务 token 鉴权仍归 Agent，Echo 示例不声称已实现登录。两条跨主机 Origin 测试先复现旧 403，修改后验证升级成功；生产 Gate 还完成应用密钥握手。两 Gate 包测试通过。

2026-09-28 双 Gate 公共流程重构：示例内部 `gatecommon` 统一 Agent 构造 panic/空值处理、正常断线重试、停机时公平补发和 Runtime 停止错误判断；生产与调试 Gate 保留各自的 WebSocket 入口、监听、认证、容量及日志行为。相关定向测试、两 Gate 包测试及两包 race 测试通过，未运行全仓验证。

2026-09-28 生产 Gate 握手前容量诊断：TCP 接纳上限拒绝连接时，原先只有 `RejectedConnections` 计数、没有共享 Logger 记录。定向测试先复现日志缺失；现在每次拒绝都会尝试向共享 Logger 接纳一条绑定 Gate ServiceID 的 `reason=capacity` 结构化日志，Logger 满载时仍遵守有界非阻塞与丢弃计数契约，不读取或记录未认证帧。同步收紧既有坏 MAC 日志测试，直接检查完整输出不含原始 payload。生产 Gate 包测试与容量诊断定向 race 测试通过，未运行全仓验证。

2026-09-28 生产 Gate 日志故障隔离：在 Gate 运行中提前关闭共享 Logger 后，握手前 TCP 连接上限仍拒绝超额连接、`RejectedConnections` 仍准确增加，`Runtime.Stop` 正常完成。新增定向测试及对应 race 测试通过；本轮未修改生产实现，也不改变应用通常应在 Runtime 停止后再关闭 Logger 的资源顺序。

2026-09-28 生产 Gate 容量拒绝与健康流量组合：一条已认证连接占用唯一连接名额，四轮共 16 条额外原始 TCP 连接被拒绝；每轮拒绝后同一健康连接仍完成一次认证请求/响应，Agent 收到的请求字节匹配，`AcceptedConnections=1`、`RejectedConnections=16`、`InboundMailboxFull=0`。定向测试及对应 race 测试通过；本轮只增加验收，不修改生产实现，也不代表持续故障洪泛容量结论。

2026-09-28 生产 Gate 不完整 HTTP 请求头回收：普通 HTTP WebSocket 入口的唯一 TCP 名额由只发部分请求头的客户端占用，额外连接被拒绝并计数；约 5 秒 `ReadHeaderTimeout` 后该连接关闭，新的 HTTP 请求恢复得到 404。定向测试和对应 race 测试通过；本轮只验证已有超时配置，不修改生产实现，也不等同于公网 Slowloris 防护验收。

2026-09-28 DebugGate 不完整 HTTP 请求头回收：只发部分请求头的 TCP 连接约 5 秒后由 `ReadHeaderTimeout` 关闭；在其等待期间，另一条 WebSocket 连接仍成功创建 Agent 并收到断开通知。计数为 `AcceptedConnections=1`、`RejectedConnections=0`，确认 DebugGate 的连接上限只覆盖进入 `/ws` Handler 的请求，不覆盖原始 TCP 接纳。定向测试及对应 race 测试通过，本轮未修改生产实现。

2026-09-28 双 Gate 停机中断不完整 HTTP 请求头：生产 Gate 的普通 HTTP 入口与 DebugGate 均先确认连接正等待剩余请求头，再调用带 2 秒期限的 `Runtime.Stop`；两者都在期限内返回，客户端随后读到 EOF，无需等待 5 秒 `ReadHeaderTimeout`。两条定向测试及对应 race 测试通过；本轮未修改生产实现，不覆盖 TLS 握手中的原始 TCP 连接。

2026-09-28 生产 Gate 停机中断原始 TLS 握手：直连 WSS 入口接纳 TCP 连接后客户端不发送 ClientHello；确认连接仍等待握手，再以 2 秒期限停止 Runtime。停机完成后客户端读到 EOF，未创建 Agent，也未进入应用握手超时计数。定向测试及对应 race 测试通过；本轮未修改生产实现，不证明任意网络半开连接的跨机器行为。

2026-09-28 生产 Gate 原始 TLS 握手自然超时：直连 WSS 入口的唯一 TCP 名额由不发送 ClientHello 的连接占用，额外 TCP 连接被拒绝；约 5 秒后该握手连接关闭，新的 TLS 握手成功，说明容量名额已释放。定向测试及对应 race 测试通过；本轮只验证已有 `ReadHeaderTimeout` 的 TLS 阶段效果，不修改生产实现，也不把它混同于 WebSocket 升级后的应用握手超时计数。

2026-09-28 生产 Gate 的 HTTP/TLS 内部日志收口：真实坏 TLS 首包先复现 `net/http` 将握手错误写到标准日志、共享 Logger 没有对应记录；现通过 `http.Server.ErrorLog` 将安全的错误类别写入 Gate 来源绑定的共享 Logger，未配置 Logger 时标准错误也只输出类别。适配器不转录原始错误行，合成 panic 文本测试确认未认证内容不会出现在结构化日志。定向测试、生产 Gate 包测试和定向 race 测试通过；本轮未改 DebugGate 的 `net/http` 内部日志入口。

2026-09-28 DebugGate 的 HTTP 内部日志入口：测试先确认其 `http.Server.ErrorLog` 为空；现与生产 Gate 共用安全类别适配器，保持各自 Gate 来源绑定。定向测试覆盖 DebugGate 的接线与敏感文本过滤，并复验生产 Gate 的真实坏 TLS 首包和合成 panic 文本。两种 Gate 的受影响包测试通过；本轮未改变业务或客户端帧协议。

2026-09-28 无共享 Logger 的 Gate HTTP 内部日志：共用适配器的定向测试覆盖 TLS、Accept、Handler panic 和未知错误类别；Go 标准日志中只保留类别，不包含测试中的未认证原文。文档澄清标准日志默认写标准错误，但可由宿主重定向；本轮不改变生产实现。

2026-09-28 生产 Gate 在共享 Logger 已关闭时的 TLS 错误：真实坏 TLS 首包触发 `net/http` 错误日志后，连接仍关闭并归还容量；随后健康 TLS 连接仍可建立，Runtime 停机正常。定向测试及其 race 测试通过，验证日志失败不会把单条坏连接扩大为 Gate 接入故障；本轮不修改生产实现。

2026-09-28 Gate HTTP 内部日志满载：阻塞底层日志写入并填满容量为 1 的共享队列后，HTTP 错误日志适配器立即返回，丢弃计数增加 1；日志输出恢复后能正常关闭 Logger。定向测试及其 race 测试通过；本轮只验证满载 admission，不修改 Gate 或 Logger 的生产实现。

2026-09-28 生产 Gate 失败阶段压力样本扩展：`TestMACGateFailurePhaseStress` 在原应用握手/MAC 失败前加入坏 TLS 首包，确认 TLS 阶段没有创建 Agent，三类故障之后健康连接仍可请求/响应。显式开关下默认每阶段 32 次样本通过；每阶段 8 次样本及其定向 race 通过。坏 TLS 首包本身不计入应用层拒绝数，独立的 TCP 容量拒绝仍会计数；这是本机短时样本，不代表目标部署容量或完整故障矩阵。本轮不修改生产实现。

2026-09-28 客户端 WebSocket Close 原因脱敏：真实生产 Gate 握手前 Close 帧测试先复现客户端自填文本进入结构化日志；修复后连接故障诊断保留 `peer_close_code`，不记录对端文本。DebugGate 的相同日志边界用定向测试覆盖。两种 Gate 共用安全字段选择，受影响包测试和定向 race 测试通过；不改变关闭或 Agent 投递语义。

2026-09-28 ProdGate TLS 配置收口：构造测试先复现 `GetConfigForClient` 可替换根配置，现在启动前以 `ErrInvalidConfig` 拒绝；`GetCertificate` 动态选证书的真实 TLS 握手仍成功，Gate 最低版本为 TLS 1.2，实际协商 HTTP/1.1。定向测试和 `go test ./examples/websocket-gate -timeout=2m` 通过，未运行全仓验证。首次包测试中既有的客户端 Close/停机竞态使一次运行返回 `websocket: close sent`，该用例单独运行及包测试复核通过；本轮未修改该关闭路径。

2026-09-28 Gate 重复关闭收尾：公共出站路径在客户端 Close 后可能再次尝试发送 Close，Gorilla 返回 `ErrCloseSent`；现在与 EOF、连接已关闭等情况一样视为关闭完成，仍保留其它真实 Close 帧和 socket 关闭错误。定向回归测试先复现错误归类，再验证生产 Gate 的客户端 Close/停机以及两 Gate 的注入关闭错误；公共包、ProdGate、DebugGate 包测试通过。未修改数据帧协议或关闭顺序，未运行全仓和 race 验证。

2026-09-28 动态证书失败隔离：新增真实 TLS 测试，让 `GetCertificate` 首次返回错误、第二次返回有效证书；首条握手失败后健康连接仍成功，Runtime 正常停止，共享日志不包含回调错误原文。定向测试和 ProdGate 包测试通过；本轮只新增验收与 RFC 说明，未修改生产实现，也未运行全仓或 race 验证。

2026-09-29 动态证书 panic 隔离：把同一真实 TLS 回归测试扩展为返回错误和 panic 两种子场景。证书回调首次 panic 时仅当前握手失败，下一条健康连接成功，Runtime 正常停止，Gate 日志不含 panic 原文。定向测试、定向 race 和 ProdGate 包测试通过；本轮只扩展验收与契约说明，未修改生产实现，也未运行全仓验证。

2026-09-29 Go/TypeScript 生产 Gate 联调：以 `go run ./cmd/debugserver --prod --addr 127.0.0.1:0` 启动现有示例，TypeScript CLI 使用 `--secure` 实际完成固定 X25519 握手及上行 MAC。普通 Echo 请求返回原 `request_id=1` 和文本，Notify 返回 Push，空文本返回业务 `TEXT_REQUIRED` 对应响应；三次 CLI 运行均成功，服务端停止后回环端口释放。本次走 `ws://`，模拟 Ingress 已终止 TLS 的内段，不是跨语言直连 WSS 或目标部署验收；未修改生产代码或客户端协议。

2026-09-29 Go/TypeScript 直连 WSS 示例：`debugserver --prod` 新增成对的 `--tls-cert`/`--tls-key` 文件参数，默认 WS 行为不变。使用临时自签名且包含 `127.0.0.1` SAN 的证书启动 `wss://`，TypeScript CLI 通过 `NODE_EXTRA_CA_CERTS` 信任该测试证书后，以 `--secure` 完成 TLS、X25519、上行 MAC 和 Echo 往返，返回原 `request_id=1`。服务端停止后端口释放，临时证书与私钥已删除；这是本机联调，不代表公网部署、证书管理或目标容量验收。

## 双主线目标确认（2026-10-20）

当前目标（2026-10-20，用户裁决）：同时推进Body Encode/Decode与Pack/Unpack，最终相对各自PB的领先幅度向TS结果靠近。已小幅领先不等于Body完成；Unpack组合缺口也不构成冻结正文的理由。Go/TS逐入口参考、边界与执行规则见[双主线目标](GOSPROTO-DUAL-TRACK.md)。本轮只复算历史TS与当前Go的相对比例、核对共享夹具哈希和文档，不重跑测试/Benchmark，不产生新的性能结果。

## countChunks Body实验（2026-10-20，收益待确认）

当前Body实验（2026-10-20）：[countChunks直接计数](benchmarks/2026-10-20-gosproto-chunk-count/RESULTS.md)已完成固定70项两轮比较。大型Decode观察−0.72%/−0.10%，部分结构数组有小收益；self8/70、clone11/70，组合/累计多有翻转，收益尚不能可靠区分。候选保留待确认、未应用生产，不因低于比例门槛丢弃。既有契约/差分、fresh与来源核对通过；库原有删除保持，正式依赖不变。上一轮双主线目标/对照已提交386727c，新实验未提交，Body与Packing继续开放。

## Packing局部word组装（2026-10-20，未采用）

当前Packing实验（2026-10-20）：[普通Unpack局部word组装](benchmarks/2026-10-20-gosproto-unpack-word-store/RESULTS.md)固定70项两轮完成，正确性/159275差分及来源核对通过；大型Unpack慢8.51%/9.03%、4/8字节大数组慢约5.5%～7.6%、普通word密度普遍回退，候选未采用。短零段小收益、失败控制/累计翻转全部保留，不设固定收益门槛。前轮Body待确认报告已提交bf1203e，新实验未提交；原库已有删除未动，生产Go源仍af9169d，双主线保持。

## Body非负tag计数（2026-10-20，待确认）

当前Body实验（2026-10-20）：[非负tag计数表示](benchmarks/2026-10-20-gosproto-decode-tag-counter/RESULTS.md)完成固定70项两轮，去掉查找负数检查但总指令仍660。大型Decode−0.08%/−4.51%，首轮累计却慢139.8ns；大型PDecode/Dispatch两轮略慢，小型Dispatch−0.83%/−0.17%及全部小收益保留。候选待确认、暂不应用，不设最低收益门槛。新增10组最大tag/跳跃链公共API对照及既有契约/差分/来源核对通过。前轮Packing报告已提交36f03f2，新报告未提交；生产Go源仍af9169d、用户原有删除不动，双主线继续。

## Body构造与字段执行诊断（2026-10-20）

当前Body诊断（2026-10-20）：[正文构造与执行成本](benchmarks/2026-10-20-gosproto-body-cost/RESULTS.md)完成PB/SP三消息的12CPU＋12allocation独立profile。大型SP Decode仍10416B/532次、PB13560B/537次，不能默认删对象；PB子消息数组也用reflect.New。下一项隔离同构造/同对象数量的结构数组目标槽写入，保留类型证明、GC屏障、错误/预算/ownership，不新建公共factory或池；原型本轮未实现。Encode的记录预检/写出继续跟踪，Packing不冻结。前轮tag报告已提交c4f06ee，新诊断未提交，生产源af9169d及用户已有删除保持。

## Body结构数组受控指针槽（2026-10-20，已保留未提交）

当前Body实现（2026-10-20）：[结构数组受控指针槽](benchmarks/2026-10-20-gosproto-decode-struct-slots/RESULTS.md)已保留并提交独立库 e43df35。相同对象构造下大型Decode−3.50%/−3.63%、Dispatch−5.80%/−3.04%，长结构重复改善、对象数中位不变；AddressBook PDecode+2.56%/+2.07%及全部控制/累计反向保留。新低GOGC/命名类型/元数据回退/异构指针拒绝回归、全库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过。新增unsafe仅同步借用已核对类型的新切片槽，GC屏障与反射回退保留。前轮成本诊断已提交06adf22，正式依赖v0.4.0不变，原库6项已有删除不动；Body与Packing继续。

2026-10-20：已完成 [a93bb0d 单模块 Go/PB 当前基线](benchmarks/2026-10-20-go-sp-pb-current/RESULTS.md)，72 项 × 5 轮，夹具及源码/模块/二进制哈希核对通过。大型 Encode −41.6%、PEncode −21.8%，PDecode +10.9%；保留全部原始波动，正式依赖不变。

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
