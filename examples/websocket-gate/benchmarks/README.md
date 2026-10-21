# 双协议编解码 Benchmark

当前Body与测量诊断（2026-10-20）：[字段Tag局部复用](../../../docs/benchmarks/2026-10-20-gosproto-record-tag/RESULTS.md)确认两记录函数Tag加载2→1、指令/栈不变，但净收益未确认，未采用。原37项大型Encode+6.22%/+7.18%，同源跨模块clone本身+6.66%，不能直接归因于候选。追加单模块独立二进制96worker，Encode+1.95%、PEncode−0.86%，控制/离散仍明显，全部小收益与反向保留，不扣控制差值。原178条流程＋补充102条及两侧描述替换race/checkptr通过；后续关键Go候选补单模块复核，旧数据不回填。前轮440f436已提交，库a93bb0d及原6项删除保持，新报告未提交，双主线继续。

当前Body实验（2026-10-20）：[数组局部字段引用](../../../docs/benchmarks/2026-10-20-gosproto-record-fields/RESULTS.md)固定37项及补充17/32字段4项两轮完成，未采用。大型Encode+9.03%/+15.75%、PEncode+6.47%/+7.34%，回退17/32字段Encode约慢10%～12%；全部正收益与累计反向保留，不设比例门槛。无新增堆分配但fieldSize栈256→432B、记录循环指令与检查增多。178+46条测量命令、原契约/109形态525差分、两侧字段容量/乱序tag/描述替换race/checkptr回归通过；库a93bb0d和原6项删除保持，前轮28c494f已提交，新报告未提交，双主线继续。

当前Packing实验（2026-10-20）：[普通Pack共享目标窗口](../../../docs/benchmarks/2026-10-20-gosproto-pack-shared-window/RESULTS.md)固定55项两轮完成，静态指令296→284但大型Pack+0.50%/−0.39%、5非零word+3.37%/+4.11%，不采用通用替换。2非零word−1.63%/−0.13%、数值PEncode−1.34%/−0.03%及全部小收益/66项累计反向保留；大型PEncode中位慢、累计两轮快一并披露。250条测量命令、109形态525差分与两侧41472次重叠窗口race/checkptr回归通过；原库a93bb0d及原6项删除不动。前轮诊断125bc91已提交，新实验未提交，依赖和双主线保持。

当前双主线诊断（2026-10-20）：[指针优化后成本](../../../docs/benchmarks/2026-10-20-gosproto-post-pointer-cost/RESULTS.md)基于已提交a93bb0d完成9CPU+9allocation及独立实际wire审计。大型Encode的整数记录预检/写出仍累计21.50%/17.00%；大型/数值数组分别386/417与321/449个word为3～7非零字节，下一Packing候选优先普通word目标视图与扫描，不误投FF。ASCII512个FF word另看扫描/分配。80条profile命令+1条审计、fresh/源码/输入/二进制/raw核对通过，无新生产改动或PB/TS排名。前轮库a93bb0d/GSR3f23e02已提交，新诊断未提交；正式依赖、用户原6项删除及两条主线保持。

当前Body实现（2026-10-20）：[整数记录数组受控指针读取](../../../docs/benchmarks/2026-10-20-gosproto-record-pointers/RESULTS.md)仅在当前字段证明及实际元素指针类型核对后，同步读切片指针槽，减少两阶段逐元素反射，命名指针回退。固定37项两轮，大型Encode−18.63%/−19.05%、PEncode−15.87%/−14.34%；AddressBook PEncode+6.43%/+0.43%、数值Encode+1.61%/+2.01%及31项累计反向保留。178条测量流程、基线新回归及7条全库/oracle/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过，候选和永久回归已提交独立库a93bb0d。前轮库7b05b78/GSR2c5c653已提交，正式依赖及原6项删除不动，Body与Packing继续。

当前Body实现（2026-10-20）：[原生字段分布诊断](../../../docs/benchmarks/2026-10-20-gosproto-inline-shapes/RESULTS.md)沿用同一延迟内容切片候选，固定8/64字段×8分布的Encode/PEncode共32项、128进程两轮。8字段混合Encode−0.57%/−0.77%、64字段半缺失PEncode−2.15%/−2.58%；64字段负数PEncode+1.40%/+0.50%及29项累计反向全部保留。决定保留小改动、接受局部代价，不宣称业务全面提速；158条测量流程及7条全库/oracle/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7验收通过，精确源码已提交独立库7b05b78。前轮19项报告已提交ffc591f，正式依赖v0.4.0及用户原6项删除不动，Body与Packing继续。

当前Body实验（2026-10-20）：[通用Encode延迟内容切片](../../../docs/benchmarks/2026-10-20-gosproto-encode-inline-body/RESULTS.md)完成固定原19项、76进程两轮比较。大型Encode中位−0.36%/−1.36%，首轮累计反慢214.9ns；大型PEncode+2.23%/−0.67%方向翻转。UTF8手工帧−2.33%/−1.32%、AddressBook PEncode−0.06%/−2.11%等小收益完整保留；候选待确认、不应用生产，不设收益比例门槛。102条流程命令、既有契约/夹具wire/来源与逐块复算通过；self13/19、clone15/19，17项累计反向及3项分配尾差保留。参考文档已提交7e48d4c，生产库e43df35、正式依赖及原6项删除不变；新报告未提交，双主线继续。

当前目标（2026-10-20，用户裁决）：同时推进Body Encode/Decode与Pack/Unpack，最终相对各自PB的领先幅度向TS结果靠近。已小幅领先不等于Body完成；Unpack组合缺口也不构成冻结正文的理由。Go/TS逐入口参考、边界与执行规则见[双主线目标](../../../docs/GOSPROTO-DUAL-TRACK.md)。

当前Body实现（2026-10-20）：[结构数组受控指针槽](../../../docs/benchmarks/2026-10-20-gosproto-decode-struct-slots/RESULTS.md)已保留并提交独立库 e43df35。相同对象构造下大型Decode−3.50%/−3.63%、Dispatch−5.80%/−3.04%，长结构重复改善、对象数中位不变；AddressBook PDecode+2.56%/+2.07%及全部控制/累计反向保留。新低GOGC/命名类型/元数据回退/异构指针拒绝回归、全库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过。新增unsafe仅同步借用已核对类型的新切片槽，GC屏障与反射回退保留。前轮成本诊断已提交06adf22，正式依赖v0.4.0不变，原库6项已有删除不动；Body与Packing继续。

当前Body诊断（2026-10-20）：[正文构造与执行成本](../../../docs/benchmarks/2026-10-20-gosproto-body-cost/RESULTS.md)完成PB/SP三消息的12CPU＋12allocation独立profile。大型SP Decode仍10416B/532次、PB13560B/537次，不能默认删对象；PB子消息数组也用reflect.New。下一项隔离同构造/同对象数量的结构数组目标槽写入，保留类型证明、GC屏障、错误/预算/ownership，不新建公共factory或池；原型本轮未实现。Encode的记录预检/写出继续跟踪，Packing不冻结。前轮tag报告已提交c4f06ee，新诊断未提交，生产源af9169d及用户已有删除保持。

当前Body实验（2026-10-20）：[非负tag计数表示](../../../docs/benchmarks/2026-10-20-gosproto-decode-tag-counter/RESULTS.md)完成固定70项两轮，去掉查找负数检查但总指令仍660。大型Decode−0.08%/−4.51%，首轮累计却慢139.8ns；大型PDecode/Dispatch两轮略慢，小型Dispatch−0.83%/−0.17%及全部小收益保留。候选待确认、暂不应用，不设最低收益门槛。新增10组最大tag/跳跃链公共API对照及既有契约/差分/来源核对通过。前轮Packing报告已提交36f03f2，新报告未提交；生产Go源仍af9169d、用户原有删除不动，双主线继续。

当前Packing实验（2026-10-20）：[普通Unpack局部word组装](../../../docs/benchmarks/2026-10-20-gosproto-unpack-word-store/RESULTS.md)固定70项两轮完成，正确性/159275差分及来源核对通过；大型Unpack慢8.51%/9.03%、4/8字节大数组慢约5.5%～7.6%、普通word密度普遍回退，候选未采用。短零段小收益、失败控制/累计翻转全部保留，不设固定收益门槛。前轮Body待确认报告已提交bf1203e，新实验未提交；原库已有删除未动，生产Go源仍af9169d，双主线保持。

当前Body实验（2026-10-20）：[countChunks直接计数](../../../docs/benchmarks/2026-10-20-gosproto-chunk-count/RESULTS.md)已完成固定70项两轮比较。大型Decode观察−0.72%/−0.10%，部分结构数组有小收益；self8/70、clone11/70，组合/累计多有翻转，收益尚不能可靠区分。候选保留待确认、未应用生产，不因低于比例门槛丢弃。既有契约/差分、fresh与来源核对通过；库原有删除保持，正式依赖不变。上一轮双主线目标/对照已提交386727c，新实验未提交，Body与Packing继续开放。

从仓库根目录运行：

```sh
npm --prefix examples/websocket-gate ci
npm --prefix examples/websocket-gate run bench:protocol
```

已安装锁定依赖时不必重复 `npm ci`。脚本先构建 TS，再顺序运行 Go 和 Node 基准，默认产物在 `tmp/protocol-benchmark/`。指定输出目录：

```sh
npm --prefix examples/websocket-gate run bench:protocol -- /absolute/output/directory
```

Go 基准也可以单独运行：

```sh
GOMAXPROCS=1 go test ./examples/websocket-gate/benchmarks \
  -run '^$' -bench '^BenchmarkProtocolCodec$' -benchmem -benchtime=200ms -count=5 -timeout=2m
```

这里的 `-count=5` 用于性能采样；不改变日常测试保留缓存的规则。Node 基准在构建后运行 `node examples/websocket-gate/client/dist/protocol_benchmark.js`，标准输出为五轮原始数据及中位数 JSON。追加 `--case ascii32` 可单独复测 32 字节小包。

前轮[原生整数数组读取视图](../../../docs/benchmarks/2026-10-19-gosproto-decode-array-view/RESULTS.md)在503a725上保留两处4/8字节读取收窄。52项两轮大数组Decode快26%～29%，4字节PDecode/真实Dispatch快7%～9%；短包代价、控制失败与8字节组合累计反向单列，不替代四实现排名。库/官方/race/checkptr/fuzz及GSR/GoTS验收通过。测试补丁归档偏差与修正复现入口亦有记录。前轮诊断已提交GSR `16d1491`，该轮已本地提交为库 `f7022ac`、GSR `b3d4517`，依赖v0.4.0、Decode继续开放。

前轮[普通Unpack仅源游标](../../../docs/benchmarks/2026-10-19-gosproto-unpack-source-cursor/RESULTS.md)在f7022ac上只改普通mask的源消费方式。70项两轮大型PDecode快1.16%/1.42%，4/8字节大数组快约3.3%～4.7%；FF接稀疏约慢11%、短零段第二轮慢14%的代价和失败控制完整保留，无收益比例门槛。库/官方/race/checkptr/vet/三项有限fuzz及GSR/GoTS通过；原数组读取轮已提交库f7022ac/GSRb3d4517，该轮已本地提交为库 `af9169d`、GSR `6b47822`，依赖v0.4.0、Decode继续开放。

前轮[预检输入位置游标](../../../docs/benchmarks/2026-10-19-gosproto-unpack-preflight-cursor/RESULTS.md)完成138,500组错误/截断/预算差分及70项固定两轮：4/8字节大数组PDecode快约8%～12%，但普通word形态普遍慢4%～33%、FF接稀疏仍变慢，当前未采用通用替换。全部正面结果和代价保留，不设收益比例门槛；旧/当前CPU与实际wire诊断另列。主库仍已提交af9169d/GSR6b47822且干净，新报告未提交、依赖v0.4.0、Decode继续开放。

当前[共享wire40项诊断](../../../docs/benchmarks/2026-10-19-gosproto-unpack-shared-wire/RESULTS.md)控制源字节/对齐/cap后，业务与合成形态方向仍相反；恢复16条机器字相同，未证明唯一硬件原因。随后[原预检分函数70项实验](../../../docs/benchmarks/2026-10-19-gosproto-unpack-size-phase/RESULTS.md)未带来稳定组合收益，未采用。普通word收益、FF代价和所有失败控制均保留，不设收益比例门槛。主库仍af9169d且干净，前轮报告已提交GSR71d313d，新两个报告未提交，依赖v0.4.0、Decode继续开放。

前轮共享wire/分函数实验已提交 GSR `2ca8864`，未推送。新[分函数＋索引预检组合](../../../docs/benchmarks/2026-10-19-gosproto-unpack-split-indexed/RESULTS.md)完成固定70项：大型PDecode快3.12%/4.38%、4字节大数组快9.84%/9.23%，但普通word两轮慢约4%～33%、FF接稀疏慢约8.5%，组合未消除取舍，未采用。280进程/26,880块、目标契约及138500差分通过，控制/长尾/累计反向全部保留。主库仍af9169d且干净，新报告未提交；依赖v0.4.0，Decode继续开放。

前轮组合实验已提交GSR `885505c`，未推送。新[按word记录预检预算](../../../docs/benchmarks/2026-10-19-gosproto-unpack-word-budget/RESULTS.md)完成固定70项：单/双字节word约快5%～7%、零段小幅改善，但大型Unpack慢6.77%～7.93%、4字节大数组PDecode两轮慢4.02%、FF接稀疏慢约33%～34%，未采用。静态指令384→372但栈帧112→128B，不能据此预测动态收益。159275差分（新增MaxInt附近预算）、目标契约及来源核对通过；控制/尾差/长尾全部保留。主库仍af9169d且干净，新报告未提交，正式依赖v0.4.0、Decode继续开放。

前轮word预算报告已提交GSR `8e9880f`，未推送。新[生产版阶段成本诊断](../../../docs/benchmarks/2026-10-19-gosproto-unpack-phase-profile/RESULTS.md)只读采af9169d的8个CPU入口：大型纯Unpack预检/恢复占完整样本30.40%/32.16%，大型PDecode中Unpack累计17.67%、正文Decode累计53.88%（嵌套项不相加）。countChunks累计4.31%，inline的未用content视图cap守卫仍在；下一候选为仅计数路径消除不使用的视图，尚未实施。55条流程命令＋1条静态objdump、24快照/1827采样权重、fresh/来源核对通过，无新A/B结论。主库仍af9169d干净，报告未提交，依赖v0.4.0、Decode继续开放。

当前总览（2026-10-20）：[Go SP/PB同场五轮](../../../docs/benchmarks/2026-10-20-go-sp-pb/RESULTS.md)使用gosproto af9169d提交归档与PB v1.36.1，共72项360样本；大型packed编码−8.1%、解码+14.9%，int32数组packed编码−36.5%、解码+62.2%。纯Encode/Decode已领先的数值路径被Unpack拖累，保持Body与Packing双主线；Packing侧优先Unpack→PDecode，Body继续定位大型结构成本，不继续以旧大型Encode数据选题。Go-only、无新profile/TS或细小收益统计显著性结论。库工作区有用户删除，采样隔离归档未触碰；正式依赖仍v0.4.0，本轮无提交。

## Go 局部优化的配对校准

小幅优化先用 [Go 配对工具](tools/go-pairs/README.md) 校准同函数和同源码 A/A，再解释候选差值。工具在独立归档中交错采样并保留全部轮次，不修改正式依赖；控制异常的项目需要解释或补证据，不能自动判为无价值优化。它不代替上面的四实现完整 Benchmark。

首轮 [校准与原候选复测](../../../docs/benchmarks/2026-10-17-gosproto-pair-calibration/RESULTS.md) 中，9/19 项同时通过两类控制；原候选仍未达到保留门槛，正式库未改；该轮工具与记录已本地提交为 GSR `f064584`。

后续 [GC 边界与聚焦校准](../../../docs/benchmarks/2026-10-17-gosproto-pair-resolution/RESULTS.md) 解释了 16 B 的 tiny 分配窗口差值，但单纯拉长到 40 ms 未稳定控制，已提交为 GSR `3a88564`。再做三项 [墙钟/进程 CPU 诊断](../../../docs/benchmarks/2026-10-17-gosproto-pair-cpu/RESULTS.md)，CPU 更稳定但不替换验收，已提交为 GSR `b2b3dd7`。后续 [FF 段集中处理候选](../../../docs/benchmarks/2026-10-17-gosproto-pack-runs/RESULTS.md) 未达收益与控制要求，已撤回，新记录待审查。工具默认与严格总门槛不变。

上述撤回为旧标准下的历史判断。按用户裁决取消统一最低收益比例后，[两轮复评](../../../docs/benchmarks/2026-10-17-gosproto-pack-reassessment/RESULTS.md) 保留稀疏展开与 FF 段分离组合，真实业务的小幅累计收益与合成形态代价分别披露，库/GoTS 接线验收通过。工具的原始标记保持不变，当前工程判断以复评报告为准；新代码和记录待审查。

上一切片已提交为 gosproto d1ab622 / GSR d3b3ef4。后续 [EncodePacked 工作区](../../../docs/benchmarks/2026-10-17-gosproto-packed-workspace/RESULTS.md) 在 API 不变的前提下省一次分配；两轮原 19 项同时检查公共 Encode/Pack 和手工帧。body PEncode 改善不等同于 RPC/完整帧改善，32 B 帧的小幅代价单列披露；该切片已本地提交为 gosproto 56f5c21 / GSR 0f0f779，未推送。

后续 [真实 RPC 工作区](../../../docs/benchmarks/2026-10-17-gosproto-rpc-workspace/RESULTS.md) 专门测 RequestEncode/ResponseEncode：五种代表消息的通知、请求加 CancelSession、响应，以及四种 Echo 通知，共 19 项。两轮均更快、少两次分配，大型约快 8%～10%、整数数组约快 18%～20%；控制失败与完整块、原未改路径对照一并保留。它包含真实 RPC 编码/session 成本，区别于下文原 frame 的手工组帧，不包含网络或 Dispatch，也不替换原四实现矩阵。该切片已本地提交为 gosproto dc4948d / GSR c4622bf，未推送。

[通知 session 逃逸](../../../docs/benchmarks/2026-10-17-gosproto-rpc-session/RESULTS.md) 复用同一19项真实RPC探针，仅把session存储放入有响应分支；9种通知两轮全部少一次逻辑分配、约4 B/op，小包有约2%～3%的CPU改善。大型通知及ASCII请求的小幅计时代价、self/clone失败及原始数据完整保留；不设最低收益比例，不概括为全面加速。库与Go/TS验收通过，该切片已提交为 gosproto 69cae73 / GSR 9ecc811，未推送。

[RPC profile与固定私有头](../../../docs/benchmarks/2026-10-17-gosproto-rpc-profile/RESULTS.md) 保留分离CPU/内存采样及失败诊断。候选仅以有界栈头复用现有整数/tag写出，19项真实RPC两轮全部更快，32 B约快37%/38%，大型约快2%～4%；通知少一个、请求/响应少两个对象。1008组API边界、39官方头向量、3078固定差分及库/GoTS验收通过；控制失败与仅58次有限随机fuzz均公开。该切片已提交为 gosproto f9de3d2 / GSR dbb681f，未推送；正式依赖与原PB/TS矩阵未变。

[响应单缓冲](../../../docs/benchmarks/2026-10-18-gosproto-response-workspace/RESULTS.md) 仅融合声明body的ResponseEncode；五项响应两轮均更快、少一次分配，大型约快6%～8%、整数数组约快13%～15%、ASCII4KiB约快43%～44%。共用写出增加私有字节前缀，普通调用传nil；请求/通知及原body/Pack/手工帧单独测量，约9/7 ns通知代价和控制失败完整披露。库/官方/差分/GoTS验收通过，该切片已本地提交为 gosproto b626637 / GSR b2825cf，未推送；不是RequestEncode、网络或PB/TS新排名。

[通知单缓冲](../../../docs/benchmarks/2026-10-18-gosproto-notification-workspace/RESULTS.md) 只融合有Request、无Response的通知，通用encoder及有响应请求流程不变。沿用19项探针，9项通知两轮均更快、少一次分配，大型约快6.5%～7.3%、整数数组约快11.7%～13.5%、ASCII4KiB约快43%～45%；10项非目标及约14～20 ns请求代价、A/A失败完整保留。库/官方/固定差分/GoTS通过，该切片已本地提交为 gosproto 0988af0 / GSR 9876a5b，未推送；正式依赖、Decode及原PB/TS矩阵未变。

[有响应请求单缓冲](../../../docs/benchmarks/2026-10-18-gosproto-request-workspace/RESULTS.md) 在锁外完成预检/编码，登记后Pack，重复session仍返回限长限容raw body。五项请求两轮均少一次分配，大型约快5.8%～6.2%、整数数组约快9.5%～11.6%、ASCII4KiB约快41%～42%；共用writer的小幅计时代价、失败workspace内存成本、全部A/A失败及原值一并保留。库/官方/race/checkptr/固定差分/fuzz/GoTS验收通过，该切片已本地提交为 gosproto 6d854d1 / GSR f715e6b，未推送；正式依赖、Decode及原PB/TS矩阵不变。

[融合后热点诊断](../../../docs/benchmarks/2026-10-18-gosproto-post-fusion-profile/RESULTS.md) 从6d854d1归档采四种输入×body/RPC八条路径，CPU与逐次内存采样分开。定位大型记录预检/写出、整数数组Pack/写出及512 B布局外溢；4 KiB ASCII的kevent干扰和全部原值保留，不把profile耗时当配对基线。源码未改，独立harness检查/归档核对通过，诊断已本地提交为GSR d2be5d8，未推送；不是新的A/B或PB/TS排名。

[整数数组连续游标](../../../docs/benchmarks/2026-10-18-gosproto-array-cursor/RESULTS.md) 只改4/8字节输出游标，编译器可消除重复切片检查。43项包含原body/RPC及混合64位新诊断；两轮32/64位纯Encode约快25%/23%、PEncode约快14%/8%，大型约快3%，对象数中位数不变。小包代价、两个字节计数尾差、未改Pack第二轮+9.45%及各臂符号汇编一致的未解释观察完整保留；库/官方/race/checkptr/fuzz/GoTS验收通过，该切片已本地提交为gosproto 0c1f6cf / GSR 22eb772，未推送；不更新正式依赖或PB/TS排名。

[整数记录Fields局部复用](../../../docs/benchmarks/2026-10-18-gosproto-record-fields/RESULTS.md) 固定43项、两轮，原值和编译证据完整保留。大型纯Encode小幅下降与控制同阶，PEncode无改善、RPC方向混合，未采用生产候选；只保留Go字段乱序/忽略字段回归和记录，已本地提交为gosproto bd1976a / GSR 4bf1b32、未推送。负向新旧夹具对照证实覆盖增量；该轮最终生产为0c1f6cf，当轮未重复生产全验收，不以新基线Pack控制覆盖上轮未解释差值。

[整数记录指针入口](../../../docs/benchmarks/2026-10-18-gosproto-record-pointer/RESULTS.md) 保留已验证数组入口的现有指针借用，减少helper反射值/重复nil分支。43项两轮大型Encode约快8%、PEncode/RPC约快5%～7%，分配次数不变；独立nil矩阵确认全nil256记录约慢0.2μs，混合记录仍改善。全部控制失败、累计与配对方向不同、未改请求+4.74%和旧Pack未解释差值保留；官方/race/checkptr/fuzz/GSR/GoTS通过。该切片已本地提交为gosproto a3b3830 / GSR f37fef0，未推送；不改变正式依赖或PB/TS排名。

[nil调用分支实验](../../../docs/benchmarks/2026-10-18-gosproto-record-nil-branch/RESULTS.md) 原43项+原nil8项、固定两轮。全nil256项约快0.32～0.43μs，但正常大型Encode慢约9%、组合入口及混合数组也回退，撤回候选，库保持a3b3830干净。int32纯Encode的约13%非目标回退、完整控制/累计/尾差和核心指令核对保留，未重复未变生产的全验收。该记录已本地提交为GSR a391e42，未推送；后续先刷新当前profile，Decode与依赖不变。

[优化后热点诊断](../../../docs/benchmarks/2026-10-18-gosproto-post-pointer-profile/RESULTS.md) 基于a3b3830，四种输入各纯Encode/EncodePacked/实际RPC，CPU与逐次内存分开采24份profile。整数数组Pack约34%/39%，大型仍有512B布局外溢；runtime/GC栈、初始化对象、插桩耗时与全部原值保留，不推导新排名。独立harness检查/vet/归档核对通过，生产未改、库干净；该诊断已本地提交为GSR 17996e0，未推送。下一轮先核对普通word Pack游标/BCE，不同时改记录或扩大unsafe。

[Pack剩余目标游标](../../../docs/benchmarks/2026-10-18-gosproto-pack-cursor/RESULTS.md) 固定62项两轮，原43项业务加19种形态，相关回归与17984差分通过。检查点未减少、汇编略增加；主要PEncode方向翻转、密集长word约慢0.1μs，部分RPC/零场景收益保留，综合撤回候选。全部控制/原值/累计和恢复指纹保留，库保持a3b3830干净；该记录已本地提交为GSR 13d4609，未推送，Decode与正式依赖不变。

[主循环密集前缀写出](../../../docs/benchmarks/2026-10-18-gosproto-pack-prefix-main/RESULTS.md) 核对旧helper候选后只重评当前主循环的目标位置计算，62项固定两轮。大型/整数Pack约快7%～10%，PEncode与真实RPC同向改善；小型稀疏约1～4ns代价和全部控制/累计/尾差保留。对象数中位数不变，相关回归/17984差分/官方/race/checkptr/vet/有限fuzz/GSR/GoTS验收通过；已本地提交为库a161158、GSR d6c672c，未推送，不更新正式依赖或PB/TS排名。

[布局项数与增长诊断](../../../docs/benchmarks/2026-10-18-gosproto-layout-audit/RESULTS.md) 覆盖109形态/317入口，独立未插桩分配与48项数增长模型。大型99项外溢512B一次容纳，减小初始容量会增配；长数组增长值得实验，但中间长度代价、预算早期失败均保留。未测CPU、不改生产代码；诊断/vet/fixture/独立复算/归档核对通过，库保持a161158；该诊断已本地提交为GSR c201237，未推送。

[布局按需倍增](../../../docs/benchmarks/2026-10-18-gosproto-layout-growth/RESULTS.md) 固定70项/四组，长数组真实入口与分配改善，4096个含整数数组记录的请求快约8.5%～8.9%、少75648B/4次分配。中间长度增配、常规追加/预检帧成本与所有控制失败/波动完整保留；既有扩展/525差分/官方/race/checkptr/vet/有限fuzz/GSR/GoTS通过，已本地提交为库bdfe1d0、GSR bf828b0，未推送，不改正式依赖或Decode。

[布局连续索引写入](../../../docs/benchmarks/2026-10-18-gosproto-layout-index/RESULTS.md) 复用count保持容量序列，固定原70项四组。大型Encode/PEncode/真实请求约快2.5%～4.7%，全部分配中位数不变，整数记录预检帧144→112B；int32/手工帧代价、未改纯Pack波动及所有控制/累计原值保留。既有回归/525差分/官方/race/checkptr/vet/有限fuzz/GSR/GoTS7/7通过，已本地提交为库7397804、GSR 25b0b0d，未推送，正式依赖和Decode不变。

[当前热点与GC上下文](../../../docs/benchmarks/2026-10-18-gosproto-post-layout-profile/RESULTS.md) 26份profile、同函数384块与补充limiter事件显示：混合夹具存活可触发GC CPU limiter并放大波动，隔离并不保证全部控制通过；完整原值保留，不用新诊断替代旧比较。整数数组组合Pack仍占37%～41%，先补单项独立进程采样再选候选。只读诊断/vet/独立复算/指纹通过，库干净；该轮已本地提交为 GSR `a125ab4`，未推送。

[原70项逐项独立进程控制](../../../docs/benchmarks/2026-10-19-gosproto-isolated-cases/RESULTS.md) 固定140个进程，self/clone均61/70通过（旧混合40/70、38/70），分配中位数全部相同，limiter观测全零；最大堆快照约4.20MB，非峰值。仍有17个不同入口至少一组失败，离群/原值全保留；13,440块/560快照独立复算、harness/vet/fresh指纹通过。该轮已本地提交为 GSR `4ff411f`，未推送；生产未改，不作新PB/TS排名，后续用逐项进程补充实现比较，保留混合历史。

[普通word连续段实验](../../../docs/benchmarks/2026-10-19-gosproto-pack-contiguous/RESULTS.md) 固定原70项/四组280进程；批量写出使目标Pack和主要RPC两轮回退，增加32条符号指令，候选仅存归档未应用到库。既有契约/17,984差分、harness/vet、26,880块独立复算/fresh指纹通过。self/clone13/70、42/70且limiter均零，失败、小收益和离群全部保留；该轮已本地提交为 GSR `d1dceaa`，未推送；生产/Decode/正式依赖不变。

[固定payload视图](../../../docs/benchmarks/2026-10-19-gosproto-pack-payload/RESULTS.md) 只消除普通word重复索引+1，指令296→288，帧不变；原70项两轮大型Pack快8.87%/9.13%，整数数组PEncode快4.70%/4.19%，主要RPC与小收益保留。控制56/70、56/70，全部累计翻转/尾差/异常块/wide64成本披露。17,984差分、复算/fresh指纹、库/官方/race/checkptr/vet/有限fuzz/GSR/GoTS通过。该轮已本地提交为库 `0b38019`、GSR `31b3f99`，未推送，Decode/正式依赖不变。

[固定len/cap视图](../../../docs/benchmarks/2026-10-19-gosproto-pack-window/RESULTS.md) 两份草稿只预检，唯一三索引候选固定原70项/四组比较；整数数组PEncode快6.07%/3.73%，真实通知快5.68%/5.17%，精确候选保留。self46/70、clone51/70，ASCII请求/大记录成本、累计翻转/全部失败/尾差保留。既有契约/17,984差分、复算/fresh、库/官方/race/checkptr/vet/有限fuzz/GSR/GoTS7/7通过。该轮已本地提交为库 `d2f6027`、GSR `175028b`，未推送，Decode/正式依赖不变。

[当前编码profile](../../../docs/benchmarks/2026-10-19-gosproto-current-profile/RESULTS.md) 从d2f6027只读采16入口/32份CPU与分配，原70构造后逐项保活、CPU在GC/预热后启动。96快照limiter零，源/二进制/profile/fresh及独立元数据核对通过；整数Pack/大型记录热点和4KiB runtime归因限制保留，不作新A/B或PB/TS排名。生产未改；该轮已本地提交为GSR `7c91913`，未推送。

[记录头部视图](../../../docs/benchmarks/2026-10-19-gosproto-record-header/RESULTS.md) 两处固定len/cap2使216→212条指令；原70项两轮4096条full请求快2.50%/1.19%，half/嵌套与低比例收益均保留，局部成本/27项累计翻转不删。self63/70、clone62/70，目标分配不变；既有契约/525差分/复算/fresh、库/官方/race/checkptr/vet/有限Codec fuzz/GSR/GoTS7/7通过。该轮已本地提交为库 `9067933`、GSR `ca02105`，未推送，Decode/正式依赖不变。

[inline内容视图](../../../docs/benchmarks/2026-10-19-gosproto-record-inline/RESULTS.md) 原70项两轮大型请求快3.18%/2.44%、响应快2.29%/2.44%，分配不变；接受混合长记录等成本、28项累计翻转全留。既有数组测试补10个边界、基线/负向覆盖证据通过；self60/70、clone64/70，复算/fresh、库/官方/race/checkptr/vet/有限Codec fuzz/GSR/GoTS7/7通过。该轮已本地提交为库 `2ea804e`、GSR `b004da7`，未推送，Decode/正式依赖不变。

[非inline内容视图复用](../../../docs/benchmarks/2026-10-19-gosproto-record-body-view/RESULTS.md) 原70项两轮大型PEncode快0.51%/0.77%、half4096请求少1.39/0.98µs，分配不变，full请求等成本/27项累计翻转全留。self50/70、clone56/70，216条/32B帧不变、panic调用点15→14；既有契约/525差分/复算/fresh、库/官方/race/checkptr/vet/有限Codec fuzz/GSR/GoTS7/7通过。该轮已本地提交为库 `e50dfef`、GSR `29c954b`，未推送，正式依赖不变。

用户于2026-10-19明确解除Decode阶段性冻结，入口、解析和构造/写入可继续按证据优化。当前[入口复核](../../../docs/benchmarks/2026-10-19-gosproto-decode-resume/RESULTS.md)保留即时类型证明与地址借用分离：大型Decode两轮快2.37%/2.02%、nil4096 Decode快6.19%/7.06%，大型通知Dispatch小成本/PDecode控制失败及累计翻转完整保留。新40项160进程/15,360块独立复算、fresh与来源核对，库全测/官方/race/checkptr/vet、有限Codec/Decode fuzz497,350/1,217次、GSR三包及GoTS7/7零跳过通过；Decode fuzz后段计数停止保留原输出，未重复PB/TS排名。该轮已本地提交为库 `913c465`、GSR `b708108`，未推送，正式依赖仍v0.4.0，Decode继续开放。

后续[结构数组schema局部复用](../../../docs/benchmarks/2026-10-19-gosproto-decode-array-schema/RESULTS.md)只保存同步调用内子schema链接，208条指令不变、栈帧224→240B。固定40项两轮中，含数组记录Decode快0.92%/1.05%，但大型Decode+1.25%/+0.79%、nil长数组Decode+1.59%/+1.81%，综合取舍未采用，不设最低收益比例。160进程/15,360块复算，self27/40、clone30/40；对象数不变，字节尾差/累计翻转/全部离群保留。目标契约/harness/vet/差分/fresh与指纹通过；主库从未应用补丁，源码等于913c465、工作树干净，未重复生产完整验收。该轮报告已本地提交为GSR `26a5748`，未推送；库保持913c465，依赖v0.4.0、Decode继续开放。

后续[原生整数资格前移](../../../docs/benchmarks/2026-10-19-gosproto-decode-integer-proof/RESULTS.md)保留全部guard，在独占分配前计算写入资格，返回同一对象用于原生或反射回退。helper仍内联、parser664条/320B不变，重复标记加载2→1；固定40项两轮大型Decode+1.76%/+1.51%，含数组记录Decode+2.31%/+1.22%，主要目标成本超过当前局部收益，未采用，不设最低收益比例。160进程/15,360块复算，self28/40、clone25/40，对象数不变，全部尾差/翻转/异常保留；目标测试/harness/vet/差分/fresh和源码/二进制核对通过。主库未应用候选、保持913c465且干净，未重复生产完整验收。该轮报告已本地提交为GSR `f7381dd`，未推送；库保持913c465，依赖v0.4.0、Decode继续开放。

后续[固定header视图](../../../docs/benchmarks/2026-10-19-gosproto-decode-header-view/RESULTS.md)只收窄readUint16的len/cap为2，原len/预算/错误/独占/全部字段guard不变。parser664→660条、320B不变；计时前两索引初稿证据另存，未计时。原40项两轮half4096 PDecode快2.41%/2.54%且双控制通过，大型PDecode快1.72%/1.89%；保留单行收益并接受短Echo/含数组记录等成本，self21/40、clone13/40及全部翻转/离群不删。160进程/15,360块复算、fresh/全部指纹、库全测/官方/race/checkptr/vet、有限Codec/Decode fuzz432,127/38,718次、GSR三包/GoTS7/7零跳过通过，Decode fuzz后段计数停止如实保留。该轮已本地提交为库 `503a725`、GSR `2104eca`，未推送，依赖v0.4.0、Decode继续开放。

后续[header字节偏移](../../../docs/benchmarks/2026-10-19-gosproto-decode-header-offset/RESULTS.md)用初始headerEnd与步进2替代index倍增，原字节/次数/预算/错误/独占/写屏障不变，parser660→656条/320B。40项两轮大型PDecode快0.81%/1.03%，但双控制失败且有累计翻转；整数数组PDecode+1.10%/+1.24%双控制通过、含数组记录PDecode+1.68%/+1.57%，整体取舍未采用，无最低收益比例。160进程/15,360块复算，self20/40、clone19/40，尾差/全部异常保留；目标测试/harness/vet/差分/fresh和指纹通过。主库从未应用候选，18份Go源码等于503a725且干净，未重复生产完整验收。该轮报告已本地提交为GSR `9699583`，未推送；库保持503a725，依赖v0.4.0、Decode继续开放。

后续[当前Decode诊断](../../../docs/benchmarks/2026-10-19-gosproto-decode-current-profile/RESULTS.md)在503a725只读刷新四入口CPU/独立allocation，8进程/24快照，limiter零。大型Decode/PDecode为10416B/532、13872B/533次；整数数组为8288B/4、12384B/5次。CPU/分配范围分离，初始化/kevent/递归累计限制保留，耗时不作新排名。源码与机器码确认native数组逐元素开放尾切片仍有调整，下一候选为固定4/8字节视图并补8字节大数组观察，本轮未实施。harness/vet/40顺序/构建/profile文本/fresh及全部指纹通过，主库18份Go源码等于503a725且干净，未重复生产完整验收。该诊断已本地提交为GSR `16d1491`，未推送；依赖v0.4.0、Decode继续开放。





## 比较口径

`cases.json` 是两种语言共用的消息内容：32 字节 ASCII、1000 字节 UTF-8、4096 字节 ASCII 和 4096 字节零字节文本。每条消息均是现有 EchoRequest 的一个 text 字段，复用现有 Protobuf 生成物与 Go Sproto 生成物。`echo.sproto` 仅把现有 echo.request 的同一字段布局命名为 EchoRequest，便于 TS 单独测量 payload；TS 在计时前核对其应用帧与真实 SprotoEchoClient 输出一致。

每条消息测八项：Protobuf/Sproto × payload/frame × encode/decode。payload 不 pack；frame 按示例现有格式封装 type/session 或 msg_id/request_id，Sproto 包含 pack/unpack。输入 payload 对象复用，帧头对象每次构造；包含输出分配和新对象解码。Go 记录 ns/op、B/op、allocs/op；TS 记录 ns/op，不估算内存分配。

`cases.json` 的字节数和 SHA-256 固定编码结果。Go 和 TS 在计时前均验证签名及语义往返，遇到不一致立即失败。这些签名首先由 Go 现有生成类型编码建立，再由 TS 验证；它们是跨语言基准夹具，不替代独立官方 Sproto 对照。协议变更后应核对变更原因，不要为了通过基准直接重写签名。

不含网络、Gate、MAC、Dispatcher、业务 Handler、安全预检和 RPC 会话表。TS 的固定内联 package 头长 6 字节，解码后按这个已知长度切分 body；没有测通用路由。Go Sproto 含库本身的反射、资源预算与失败原子性成本；两种库也保留各自的字符串校验语义。不要把结果解释为协议格式的理论性能。

## 报告

[Decode 记录候选与受控写入原型](../../../docs/benchmarks/2026-10-17-gosproto-decode-writes/RESULTS.md) 保存被撤回的整数叶子 parser，以及完整通用流程中的类型化整数写入隔离原型。后者五轮大型 Decode/PDecode 约 -11.2%/-9.2%，分配不变；字符串与未改 Unpack 控制的代价保留。该轮提案与回归已本地提交为独立库 `5be3b26`、GSR `2872d46`；原型未作为当时正式实现。

[Decode 同分配诊断与安全 inline 验收](../../../docs/benchmarks/2026-10-17-gosproto-decode-analysis/RESULTS.md) 区分固定流程、反射赋值与构造成本；保留通用/固定探针各自数据。安全 inline 最终 129 项五轮中，大型 Decode/PDecode 约 -5.3%/-4.1%，分配不变；原三轮波动、完整矩阵极小项代价及 A/A 控制分别公开。固定 Item 探针不作为生产结果，正式依赖仍为 v0.4.0。

[普通 Unpack 候选筛选](../../../docs/benchmarks/2026-10-16-gosproto-unpack-candidates/RESULTS.md) 记录五个未采用候选、1935 个三轮样本和可复现补丁。位置/计数表、游标及稀疏直接恢复均未通过业务收益与形态代价检查；生产代码仍为已提交的零段优化，只新增独立字节回归。额外保存大型 Decode 的对象构造/分配 profile，不作为新排名。

[连续零 mask 有界解包](../../../docs/benchmarks/2026-10-16-gosproto-zero-unpack/RESULTS.md) 是最新 Unpack 专项：129 项五轮配对，4 KiB/64 KiB 零块约 -64.0%/-68.3%，4 KiB 零字节帧约 -51.2%。AddressBook Unpack 约 +10%、PDecode +1.6% 和普通形态代价原样保留；35 种形态、45824 组输入/预算差分、失败候选与 profile 一并记录。完整预检、分配和 wire 不变，正式依赖仍为 v0.4.0。

[长 FF 段有界搜索](../../../docs/benchmarks/2026-10-16-gosproto-ffscan/RESULTS.md) 是上一轮 Pack 专项：137 项五轮配对，4 KiB ASCII Pack/PEncode 约 -71.8%/-57.4%，完整帧约 -43.1%，UTF-8 帧约 -34.2%。保留小包约 5% 的 Pack 代价、39 种合成形态、失败候选与 17984 组差分。预算、wire、输出容量不变，正式依赖仍为 v0.4.0。

[Pack 混合数值 word 优化](../../../docs/benchmarks/2026-10-16-gosproto-pack/RESULTS.md) 是上一轮 packing 专项：稀疏/混合写出分开，简化 FF 状态，wire 和输出容量不变。115 项五轮配对中，整数数组 Pack/PEncode 约 -10.3%/-6.8%，大型消息约 -5.0%/-1.3%；记录 28 种合成形态、失败候选、9216 组差分与小包代价。正式依赖仍为 v0.4.0。

[嵌套类型校验与整数记录数组](../../../docs/benchmarks/2026-10-16-gosproto-bindings/RESULTS.md) 是上一轮编码专项：修复描述替换时的 Go 布局核对，数组入口复用当前字段验证。大型 Encode 相对上一提交 8.382 → 6.528 µs，同场 PB 7.606 µs；AddressBook 的校验成本及最小修复版对照分别公开。两组共 490 个配对样本、失败候选与复现补丁保留，正式依赖仍为 v0.4.0。

[大型结构体 Encoder 字段绑定](../../../docs/benchmarks/2026-10-16-gosproto-encoder/RESULTS.md) 保存原生 `*int64` 只读 offset 访问的完整 Go 主矩阵、同进程专项、候选回退与平台范围。大型 Encode 同进程 14.345 → 8.456 µs，同次 PB 7.863 µs；分配不变，少数小 body 有固定开销，正式依赖仍为 v0.4.0。复现脚本仅在忽略目录建立新旧库并列的临时测试模块，不改生产依赖。

[Go 当前基线与第三轮筛选](../../../docs/benchmarks/2026-10-16-gosproto-round3/RESULTS.md) 从 gosproto `6012b06` 重新采样；保留布局记录的内存收窄，解包展开/窗口和原生指针赋值候选未采用。完整主套件、配对复核、profile 与失败值分别记录，TS 实现冻结，正式 Go 依赖仍为 v0.4.0。

[数值 packing、字段查询与类型化构造](../../../docs/benchmarks/2026-10-15-sproto-round2/RESULTS.md) 是第二轮结果；保留与首轮及正式 v0.4.0 的对照。原有两套基准不改变调用方式，实际 pencode/attach 的缓冲融合收益单独测量。

[TS 长块与 Go 反射路径优化验收](../../../docs/benchmarks/2026-10-15-sproto-next/RESULTS.md) 保存相对 v0.4.0 的前后对照、两套完整五轮采样、分配代价、profile 和撤回的尝试。TS 修改在当前工作树；Go 使用独立库未发布优化源码及临时 modfile，正式依赖仍为 v0.4.0，复现时需按该报告指定源码。

[v0.4.0 正式依赖完整复测](../../../docs/benchmarks/2026-10-15-v0.4.0/report.md) 汇总 Go/TS × Protobuf/Sproto 的两套基准：每种语言 72 项、每项五轮，同时列出纯 body、packing、应用帧、wire 大小、Go 分配和采样波动。

[2026-10-15 本机结果](../../../docs/benchmarks/2026-10-15/report.md) 包含完整耗时、大小、分配、实现版本和原始数据。此基线仅覆盖 Echo 字符串，后续比较整数数组、嵌套结构或真实游戏消息时，应新增相同语义的夹具并分别报告。

## AddressBook、游戏数值与数组

新增独立消息体套件，保留上面的 Echo 应用帧套件：

```sh
npm --prefix examples/websocket-gate run bench:representative
npm --prefix examples/websocket-gate run profile:protocol
```

两条命令顺序执行，不要同时采集性能计时与 profile。无需启动服务器、访问网络或安装 Lua；普通运行使用已保存的生成类型和 wire 签名。

`representative.json` 为两端共用数据：官方 AddressBook；16 属性/8 技能/8 物品和 256 属性/64 技能/128 物品的两档游戏状态；896 个 int32 边界值；4 KiB ASCII 对照。游戏状态还包含整数状态位、正负坐标和字符串标签。Protobuf 的游戏数值数组显式 packed，负值采用 sint32；AddressBook 保留官方 proto2 presence。TS 参考 parser 使用顶层 PhoneNumber，Sproto 字段布局不变。

每组八项：PB Encode/Decode、SP Encode/Decode、单独 Pack/Unpack、Encode+Pack 与 Unpack+Decode。没有 Envelope/package，以分离编解码器与 packing 成本。两端先核对完整语义、raw/packed 签名与 padding，再热身计时。默认输出在 `tmp/protocol-representative/`；可同旧套件一样追加绝对输出目录。

Go 单独运行：

```sh
GOMAXPROCS=1 go test ./examples/websocket-gate/benchmarks \
  -run '^TestRepresentativeFixtures$' -bench '^BenchmarkRepresentativeCodec$' \
  -benchmem -benchtime=200ms -count=5 -timeout=2m
```

TS 构建后可单独做一致性检查：

```sh
node examples/websocket-gate/client/dist/representative_benchmark.js --mode check
```

额外能力探针记录 boolean 与混合正负 int64 数组的现状，失败时输出诊断，性能排名不纳入失败输入。两项在基线失败，当前修复后均通过；`correctness.json` 保存 20 组独立官方 raw/packed 向量，由现有 Sproto 客户端测试验证布尔、整数宽度与符号、稀疏字段及 packing 边界。游戏性能夹具继续使用原有整数状态和 int32 数组，不因修复而更换比较数据。

Profile 默认对 AddressBook/game_large 的六个独立步骤逐个采集，Go CPU 和全量分配用不同进程，TS CPU 和采样分配用不同时段。完整文件位于 `tmp/protocol-profile/`，汇总 JSON 保存 self/cumulative CPU、Go alloc_space/alloc_objects 及 V8 sampled heap 归属。TS 堆采样包含已回收对象，但不完整计入 Buffer 外部内存，不能等同精确 B/op。开销与采样归因限制见 [分析和优化方案](../../../docs/benchmarks/2026-10-15-representative/ANALYSIS-AND-PLAN.md)。

[新套件结果](../../../docs/benchmarks/2026-10-15-representative/report.md) 与前述 Echo 报告分开保存，避免将 body 与应用帧数据混为同一口径。

[优化实施与验收](../../../docs/benchmarks/2026-10-15-optimized/RESULTS.md) 保存修复后五轮数据、小包回退检查、profile 摘要及独立 Go 库提交。Go 优化已随 gosproto v0.4.0 发布，GSR 正式依赖已升级；当前可直接用正式依赖复现，报告保留当时的临时 modfile 采样来源。采集器会记录实际模块覆盖及 Go/TS 源码哈希，避免把本地优化结果误标为正式版本。Profile 可设 `GSR_PROFILE_SCENARIO=game_large` 只采大型游戏消息，默认仍采两组。

### 原生 API、Protobuf 对照与解包形态

原套件有意分开调用 encode/pack，Echo frame 也分别编码头和 body 后拼接，因此不会体现 pencode/attach 内部减少复制的全部收益。补充脚本复用上述五组 body 和四组 Echo 数据，不更改 wire 签名：

```sh
npm --prefix examples/websocket-gate run build
node examples/websocket-gate/scripts/benchmark-sproto-api.mjs /absolute/output/api
# 可选：同进程交错对比上一版已编译的 Sproto 模块。
node examples/websocket-gate/scripts/benchmark-sproto-api.mjs /absolute/output/api /absolute/baseline/sproto.mjs
```

每操作独立热身至少约 50 ms，五轮各约 100 ms；同操作的 PB/SP 前后版本紧邻采样，并在奇数轮整体反转顺序，减少版本整组计时的阶段偏差。结果保存模块、脚本、夹具指纹、采样顺序及运行环境。代表性消息同时测 Encode/Decode、Pack/Unpack、原生 PEncode/PDecode 与分阶段组合；PB Encode/Decode 作为同进程控制项。可选基线参数只用于 Sproto 前后对照，不更换 PB 依赖。

Echo 文本在计时外构造一次，PB 与两个 SP 版本共享同一份输入。值相同但独立 repeat 生成的字符串可能具有不同 V8 内部表示，不能据此计算编解码器变化；历史记录保持原样，后续以共享输入的采样为主。

RPC 使用已有 attach/dispatch、真实 C2S schema 和固定 session=1，覆盖同一个等待项，不包含外层客户端安全预检、会话分配、网络或 MAC。原生收发与手工组帧/固定 schema 解码分别计时，不能混用两种入口计算优化比例。内部解包缓冲在热身时已建立，原生解码数据代表热缓冲；冷启动、超保留容量和失败复用由回归与缓冲观测单独验证。

另测 46 种公开 unpack 数据形态：32 B–64 KiB 的 ASCII、零块、稀疏 word，512 B–4 KiB FF 前缀后接稀疏数据，重复短零串，以及零前缀后接 ASCII；重复形态最大约 129 KiB。Pack 单独测 14 种形态，包括每 word 一至七个非零字节的旋转 mask、确定性随机数据、短包，以及 FF 六/七字节 word 的连续合并与分段。它们专门检查字节分布和段状态带来的回退，不与 PB 编解码排名混合。有基线时共 250 项、五轮 1250 个样本。

需要定位 Pack 热点或 public/manual 路径分配时，先构建 TS，再单独执行：

```sh
node examples/websocket-gate/scripts/analyze-sproto-pack.mjs tmp/sproto-pack-analysis
node examples/websocket-gate/scripts/analyze-sproto-pack.mjs tmp/sproto-pack-before /absolute/baseline/sproto.mjs
```

该脚本保存代表性消息的 mask/非零字节数分布、显式 Buffer 工厂与 subarray 调用次数，以及独立的 Pack/PEncode 和解包 CPU profile。分布计数使用包装后的实例，profile 使用新实例；不与计时 Benchmark 并发运行。Buffer 调用观测不等于完整堆分配统计，profile 百分比也不能直接换算为绝对加速。

### Go 解包形态

Go 单独覆盖 18 种 Unpack 形态：32 B/4 KiB/64 KiB 零字节与 ASCII、每 word 一至七个非零字节的旋转 mask、FF 前缀接稀疏尾部、重复短零段和确定性随机输入。它们用于发现只测业务消息会漏掉的回退，不作为 PB 直接比较项，也不冒充与 TS 形态逐字节相同的跨语言夹具。

```sh
GOMAXPROCS=1 go test ./examples/websocket-gate/benchmarks \
  -run '^$' -bench '^BenchmarkUnpackShapes$' -benchmem -benchtime=100ms -count=5 -timeout=2m
```

比较未发布库时使用显式临时 modfile；固定同一份 Benchmark 分别编译前后二进制，按操作配对并交替先后。source、二进制和实际模块覆盖一起保存；固定次数复核与自动迭代全量分别报告。

### 结果文件入库规则

测试代码、共用夹具、官方 wire 向量和复现脚本纳入版本管理。正式 Benchmark 基线在 docs/benchmarks/ 下保留最终报告、环境与源码信息、最终原始采样，以及有分析价值的精简 profile 摘要；不保存每轮调试快照。

普通测试输出、分阶段中间采样、完整 pprof/cpuprofile/heapprofile 和测试二进制放在已忽略的 tmp/。需要跨机器保留时使用 CI artifact。独立 gosproto 的源码改动在该库提交，GSR 报告引用提交或版本，不重复保存源码补丁。

### 生成与官方对照

Benchmark schema 位于 `schema/`，仅服务于测试，不进入 GSR 业务协议目录。重生成需要本机 protoc、protoc-gen-go v1.36.1、已安装的 protoc-gen-es v2.15.0，以及固定的 sprotodump：

```sh
SPROTODUMP_DIR=/path/to/lvzixun/sprotodump \
  npm --prefix examples/websocket-gate run generate:benchmark
```

sprotodump 固定 `3a45d3e49ea2bc8e2b607e2c8060b32281d730dd`，复用现有工具，不引入新 generator 或依赖。普通 `npm run generate` 不处理本套测试 schema。

可选官方验证需要固定源码目录及其编译的 Lua core 模块（本轮 Lua 5.3.1、cloudwu/sproto `e93fd5a0653b2bcab38d3b830958bf2e3a901b5f`）：

```sh
SPROTO_ORACLE_DIR=/path/to/pinned/sproto \
  python3 examples/websocket-gate/scripts/benchmark-oracle.py
# 另外检查 20 组正确性向量。
SPROTO_ORACLE_DIR=/path/to/pinned/sproto \
  python3 examples/websocket-gate/scripts/benchmark-oracle.py --correctness
```

脚本先验证 `oracle-source.json` 的六份源码哈希，再用官方 C/Lua 编码五组数据，与固定签名比较。可加 `--output /absolute/file.json` 导出独立证据；Go 侧可设 `GSR_BENCH_EXPORT_WIRE=/absolute/file.json` 运行夹具测试导出独立签名。两种导出都不自动覆盖 `representative.json`，更新夹具时应先比较独立结果再审查变更。

[原生整数 Decode 受控写入](../../../docs/benchmarks/2026-10-17-gosproto-decode-store/RESULTS.md) 记录私有 helper 封装后的五轮新采样：大型 Decode/PDecode 约 -7.4%/-5.9%，分配不变，完整矩阵/A/A 控制和 GC/布局验收一并保留。该切片已本地提交为 gosproto `fc3246a`、GSR `862fa8b`，正式依赖仍为 v0.4.0。

[Decode 入口与普通 mask 候选筛选](../../../docs/benchmarks/2026-10-17-gosproto-decode-binding/RESULTS.md) 保存三个入口写法与固定八字节恢复的 918 个筛选样本。四个候选全部未采用；新增基线 profile、mask 分布和独立 FF 后继回归，未改生产代码或正式依赖；该轮已本地提交为 gosproto `a09fb00`、GSR `d1beb28`。

[入口最终实验与 Decode 冻结](../../../docs/benchmarks/2026-10-17-gosproto-decode-freeze/RESULTS.md) 保存 90 个微基准与 645 个完整对照样本。入口约 -38.3%，大型 Decode/PDecode 仅 -2.25%/-1.28%，未达预定门槛，候选未采用。Decode当时冻结于 `a09fb00`，用户已于2026-10-19解除，见本页当前入口复核；正式依赖未变；阶段性冻结记录已本地提交为 GSR `0db5ed0`，后续仍可回头优化。

[Pack 普通 word 执行路径](../../../docs/benchmarks/2026-10-17-gosproto-pack-execution/RESULTS.md) 保存当前 137 项五轮对照及三轮筛选。大型/整数数组 PEncode 约 -6.0%/-5.7%，分配不变，Decode 等控制不变；所有代价与验证范围一并保留；该轮已本地提交为 gosproto `faa6a21` / GSR `3626b3d`，未推送/发布。

[连续零 word Pack 筛选](../../../docs/benchmarks/2026-10-17-gosproto-zero-pack/RESULTS.md) 保存三个候选各 191 项三轮的全部原值，共 1719 样本。零块有收益，但其他路径代价不合适，全部撤回；只保留精确字节回归和记录，已本地提交为 gosproto `bba3326` / GSR `a27ec4e`，未推送/发布。新增 27 种零段/尾块 benchmark 控制及完整复现脚本，不作为新的协议总体性能基线。

[稀疏 word 与 A/A 诊断](../../../docs/benchmarks/2026-10-17-gosproto-pack-sparse/RESULTS.md) 保存 191 项三轮候选筛选和 59 项三轮独立 A/A，共 750 个样本。候选组合回退未过门；同源码 A/A 也有明显差值，下一步应先校准测量，不能以约 6% 的纯 Pack 差值作为稳定收益。正式库未改，该轮诊断已本地提交为 GSR `8f0003f`，未推送。

2026-10-20：已完成 [a93bb0d 单模块 Go/PB 当前基线](../../../docs/benchmarks/2026-10-20-go-sp-pb-current/RESULTS.md)，72 项 × 5 轮，夹具及源码/模块/二进制哈希核对通过。大型 Encode −41.6%、PEncode −21.8%，PDecode +10.9%；保留全部原始波动，正式依赖不变。

2026-10-20：[同模块/同进程Unpack实验及采纳](../../../docs/benchmarks/2026-10-20-gosproto-unpack-inprocess/RESULTS.md)完成60进程/5760块。保留固定视图＋非零mask索引范围表达：整数数组Unpack两轮中位−1.03%/−2.50%，7非零字节word−3.00%/−3.87%；大型及组合路径不确定、局部小包成本与21项中位/累计反向保留。同源大型控制仍偏，不声称噪声消失。完整库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过，fuzz3310次后停增如实记录。生产代码及永久回归已提交独立库50c69e1、逐文件等于验收快照，原6项删除和正式依赖v0.4.0不变，未推送。

2026-10-20：[字符串数组受控写槽](../../../docs/benchmarks/2026-10-20-gosproto-string-slots/RESULTS.md)保留长数组收益及小数组代价。实际元素恰为原生string时同步借用新切片，命名元素反射回退、完整预检/复制/GC屏障保持；128/512项Decode两轮约−9%，512项PDecode约−7%，3项及现有大型Decode有局部代价，不宣称整体游戏提速。88进程/8448块、基线新增回归、完整库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过；Codec456532次，Decode13781次后停增如实记录。代码及永久回归已提交独立库aef6876且等于验收快照，原6项删除和正式依赖不变，未推送。

2026-10-20：[消息头顺序游标实验](../../../docs/benchmarks/2026-10-20-gosproto-header-cursor/RESULTS.md)未采用。大型Decode中位两轮+0.32%/+0.12%，PDecode−0.79%/−0.29%的小幅正向及所有反向保留；控制同阶，机器字行不变、栈320→336B。修正一次超出当前编码跨度的新增夹具，初次18个self样本与失败命令单独保留；完整v2为80进程/7680块/110条成功命令，全部20输入预检、头部上限/跳跃/预算等三parser race/checkptr通过。原库仅原6项删除，生产仍aef6876。下一Body候选考察整数记录数组入口的当前字段/类型证明复用，不改逐项wire/预算/对象独占，不跨调用缓存。


2026-10-20：[整数记录数组 Decode 证明复用](../../../docs/benchmarks/2026-10-20-gosproto-decode-record-proof/RESULTS.md)保留至少8项的短数组回退版。大型Decode两轮中位−9.40%/−9.50%、PDecode−7.76%/−6.22%，128/512项正文约快12%～13%；首版1/2项明显退化完整保留，第二版补3/4/7/8/9边界后消除该退化。当前可达字段/实际类型在本次数组入口核对，逐项wire/预算/独占对象与GC屏障保持，不跨调用缓存。两版150+200条命令、10752+14592块全部保留；新增回归先在基线通过，完整库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过，Decode fuzz2735次后停增如实记录。候选源码等于验收快照，独立库代码及永久回归已提交5360fe0，GSR报告同步提交；原6项删除和正式依赖v0.4.0不变。前轮GSR0c23252已提交；下一切片回到Packing成本诊断，Body继续开放，无新PB/TS排名。


2026-10-20：[Decode优化后Packing成本更新](../../../docs/benchmarks/2026-10-20-gosproto-packing-refresh/RESULTS.md)基于已提交5360fe0完成三消息Unpack/PDecode共12份独立CPU/全量allocation profile，56条流程及1条实际wire审计通过。大型/整数数组普通写出源码行约占完整Unpack样本25.1%/14.0%，实际386/321个word为3～6非零字节；ASCII几乎全走FF，kevent/GC上下文保留，不重算分母或当网络瓶颈。下一单一候选为按有效字节数range连续源窗口，先核对popcount/检查/栈成本再测，完整首遍和ownership保持；本轮尚未实现。无新生产改动或PB/TS排名，库5360fe0与GSR b0b1c4c已提交，新报告留审查，原6项删除及正式依赖不变，Body继续开放。


2026-10-20：[Unpack连续源窗口遍历](../../../docs/benchmarks/2026-10-20-gosproto-unpack-source-range/RESULTS.md)不采用通用替换。少一个循环源索引检查，但增加每word计数/窗口检查，机器字行388→396、栈不变；大型Unpack两轮+13.04%/+12.87%、整数数组+13.52%/+14.12%。固定4～7字节密度的重复收益、小包正向、21项累计反向和全部控制/长尾保留，不按最低收益比例舍弃。84进程8064块、115条命令、全部mask三入口race/checkptr、夹具/vet/预检/指纹复算通过。下一步区分固定密度和真实混合mask顺序，不直接按密度分流。本轮无新生产代码，库5360fe0、正式依赖和原6项删除不动；前轮诊断已提交63d8015，新报告留审查，Body与Packing继续。


2026-10-20：[连续源Unpack混合mask诊断](../../../docs/benchmarks/2026-10-20-gosproto-unpack-mixed-masks/RESULTS.md)沿用同一候选，20输入、80进程7680块及110条流程通过。真实流保持段集合/直方图/长度后分组或洗牌仍慢约11%～15%，不能把退化简单归因于原始顺序；固定4/7字节收益保留，4有效位变位置结果不稳定，不据密度直接分流。五组输入不变量、独立字节重建、三入口全部mask race/checkptr及来源核对通过。下一步以真实高频mask隔离位置/依赖，不宣称已定位全部根因。本轮无新生产代码，库5360fe0、正式依赖与原6项删除不动；前轮报告已提交c957a55，新诊断留审查，Body/Packing继续。


2026-10-20：[真实高频mask收尾诊断](../../../docs/benchmarks/2026-10-20-gosproto-unpack-frequent-masks/RESULTS.md)完成22入口88进程8448块/120条流程及另4份CPU profile/21条流程。实际4～6字节mask单独重复也有正向，不能把优势只归于低位连续位置；原始大型/整数数组Unpack仍慢约13%～15%，不采用连续源窗口。全部小收益、14项累计反向、控制与长尾保留，不按门槛舍弃，也不声称已定位全部退化根因。全mask三入口race/checkptr、夹具/vet/预检/指纹复算通过；无新生产代码或PB/TS排名。下一切片保留旧写出，单测首遍普通mask预检分支，完整预算/错误顺序/ownership保持，尚未实现。前轮报告已提交0a5eded，新诊断留审查；库5360fe0、正式依赖和原6项删除不变，Body继续开放。


2026-10-20：[Unpack普通mask预检两版实验](../../../docs/benchmarks/2026-10-20-gosproto-unpack-precheck-ordinary/RESULTS.md)暂不并入。首版拆普通路径导致机器字行388→420；精简版只合并分类，共用原检查，机器字行/栈不变。小型Unpack两轮约快3%保留，但大型无重复正向、整数数组不一致，组合/累计及23项反向完整披露，不按收益门槛舍弃。两版168进程16128块/230条流程、三入口全mask与新增边界/预算冲突错误顺序race/checkptr、夹具/vet/预检/来源复算通过。下一候选为一次packed产出上界证明及严格预算回退，尚未实现，所有边界/错误顺序/ownership保持。无新生产代码；前轮报告已提交6c40523，新报告留审查，库5360fe0、依赖和原6项删除不动，Body继续开放。


2026-10-20：[Unpack一次预算上界证明两版实验](../../../docs/benchmarks/2026-10-20-gosproto-unpack-budget-proof/RESULTS.md)证明成立但暂不采用。首版helper在固定普通mask有重复收益，却给单word约5%代价；共享版只加证明guard，无新helper，大型无一致收益、固定普通mask多数慢1%～3%。小收益、严格预算回退和39项累计反向保留，不按收益门槛舍弃。两版192进程18432块/260条流程＋1条汇编导出，三入口全mask/证明门槛/最大int/错误顺序race/checkptr、夹具/vet/预检/指纹复算通过；无新生产代码。下一Body候选看原生*string受控赋值，保持复制/独占/owner回退，尚未实现。前轮已提交7b24406，新报告留审查；库5360fe0、依赖和原6项删除不变，两条主线继续开放。


2026-10-20：[原生字符串指针受控写入](../../../docs/benchmarks/2026-10-20-gosproto-string-pointer/RESULTS.md)已保留。AddressBook Decode两轮−4.59%/−4.58%、PDecode−3.47%/−3.11%；64字符串Decode约快15%、128字符串记录约快9.5%，不因低于5%舍弃真实业务收益。实际类型/当前owner/Wire/Array证明后类型化赋值，复制/独立对象/GC屏障及命名/外来描述回退保持；全缺失约1%局部代价和全部28项累计反向保留，不宣称大型整数消息全面提速。104进程9984块/140条流程，新增回归先在原提交通过；10条完整验收含库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过，Decode10968次后停增如实记录。精确源码与测试等于验收快照，独立库已提交f5de803；GSR报告同步提交，未推送。依赖仍为v0.4.0，七项无关工作树删除保留，两主线继续。
