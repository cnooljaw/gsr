# Body结构数组受控指针槽写入（已保留，库提交 e43df35）

2026-10-20。前轮Body成本诊断已提交GSR `06adf22`，未推送。本轮从af9169d仅改decodeStructSlice的目标元素写入；**候选已通过完整验收并落到独立库，运行时代码与被测/验收快照一致，已提交为 e43df35**。没有混入countChunks/tag或Packing候选。正式依赖v0.4.0不变，Body与Packing继续向TS参考领先程度推进。

## 实现与安全边界

countChunks完整预检、MakeSlice、每个子对象reflect.New、通用decodeMessage、完整消费检查、错误/元素/深度/字节预算和最终value.Set均保持。只在count>0、当前字段Array/Wire为结构数组且**实际切片元素类型等于PointerTo(私有子schema.typ)**时，同步借用新切片的[]unsafe.Pointer槽；元素成功解码后使用Go指针赋值。否则继续原reflect.Index/Set；命名切片可匹配，命名指针走原可赋值语义。

新切片len/cap均count，目标元素已确认为单个Go指针，其尺寸/对齐/GC布局与槽一致，unsafe.Slice只覆盖实际分配的count个元素。reflect.New的对象类型与证明使用同一私有子schema；不缓存公开Fields、不借外来offset、不跨调用保留地址，不使用uintptr或指针memcpy。槽视图只在本次同步Decode内使用，result仍在成功时写回；失败时新数组/子对象被丢弃，外部原值不变。每个子对象和标量仍独占，没有池、arena、共享backing对象或构造工厂。

机器码保留runtime.gcWriteBarrier2；真实目标指针存储没有绕过GC。decodeStructSlice静态208→352条、栈224→256B，新增证明/回退和视图检查有维护/执行成本；不将代码变长自动判失败，也不因减少反射就预告提速。程序集原提取器对256B序言未识别，原raw保留，summary按SUB $256/RSP序言补充，见safety-review。

## 增量测试与评审

使用clean-coder-review检查该局部借用的owner、生命周期和失败路径，没有新增公共API或架构角色。计时前两库隔离公共测试覆盖0/1/3/64/128元素、命名切片/命名指针、相同输入指针在不同字段/元素/调用解码后独占、低GOGC与显式GC后存活、数组恢复之后坏尾部仍保留原值，以及Array/Wire公开形状变化的反射回退；原契约、70入口wire/完整值/消费长度、109形态525差分亦通过。新增测试相对旧标量指针GC用例的增量是结构数组目标槽、命名指针回退和容器存活，而非重复搭建GSR跨层夹具。

上述新路径在计时前单独通过race+严格checkptr。配对后补充永久回归并在未改基线先通过：外来子结构字段描述不能静默写出不兼容的目标指针；允许原反射拒绝形式，不固定panic文本或把它扩成新公开契约。永久测试作为新decode_struct_slots_test.go加入库，运行时代码未再修改；它不进入imported-library性能二进制。全库验收覆盖这些新增测试。

## 固定70项两轮结果

self/clone/两轮compare共280进程、26,880块、6720四元组、1680轮、1120快照；默认GOMAXPROCS1/GOGC100/无限GOMEMLIMIT，清sink/GC/50ms预热、10ms校准块、3×128分配计数和原sampleBlock/blockCalls不变。所有构建/测试/汇编在计时前，固定来源与顺序，不重采择优、删长尾或扣控制。

| 入口 | 第1轮 | 第2轮 | 第1轮累计ns | 第2轮累计ns | self / clone控制 |
| --- | ---: | ---: | ---: | ---: | --- |
| body/addressbook/decode | +0.96% | +0.38% | +104.5 | -298.3 | False / True |
| body/addressbook/pdecode | +2.56% | +2.07% | +45.3 | +36.6 | False / True |
| body/game_small/decode | -1.54% | -1.26% | +69.5 | -394.9 | True / True |
| body/game_small/pdecode | -0.79% | -1.85% | -2.7 | -39.2 | True / True |
| body/game_large/decode | -3.50% | -3.63% | -935.9 | -607.4 | True / True |
| body/game_large/pdecode | -3.05% | -2.66% | -600.1 | +994.4 | True / False |
| rpc/game_large/dispatch | -5.80% | -3.04% | -2883.1 | -956.2 | True / False |
| growth/flat/full/4096/decode | -4.41% | -4.21% | -16039.3 | -10324.9 | False / False |
| growth/flat/full/4096/pdecode | -0.01% | -1.90% | -5331.2 | -29154.4 | False / False |
| growth/flat/nil/4096/decode | -8.77% | -8.74% | -20710.5 | -10618.3 | True / True |
| growth/flat/nil/4096/pdecode | -9.27% | -7.54% | -31900.8 | -19567.4 | False / True |
| growth/flat/half/4096/decode | -6.21% | -5.85% | -25727.6 | -22277.5 | False / False |
| growth/flat/half/4096/pdecode | -3.54% | -4.65% | -13958.1 | -18800.4 | False / False |
| growth/nested/32x128/decode | -5.04% | -3.87% | -24048.6 | -16855.0 | False / False |
| growth/nested/32x128/pdecode | -2.21% | -2.39% | -18376.2 | -15207.9 | True / False |
| growth/array_record/4096/pdecode | -4.06% | -5.20% | -47252.2 | -67214.8 | True / True |
| rpc/int32_edges/dispatch | +2.07% | +1.24% | +143.4 | +84.3 | False / False |
| rpc/width8_len896/dispatch | +2.26% | +2.52% | +275.0 | +286.4 | True / False |

保留依据是相同对象构造下，大型Decode两轮−3.50%/−3.63%、累计少935.9/607.4ns，真实Dispatch−5.80%/−3.04%、累计少2883.1/956.2ns；多种长数组Decode/PDecode重复改善，nil4096约快9%、half4096约快6%。full4096 PDecode首轮仅−0.01%、累计少5331.2ns也列出，不用固定收益比例筛掉。

同时接受并记录局部成本：AddressBook PDecode+2.56%/+2.07%、累计增加45.3/36.6ns；它的子数组较短，入口类型/形状证明的代价不能忽略。int32和8字节大数组的Dispatch也分别约+1%～2.5%，这些非结构数组目标变化不能自动归功或归罪槽写入，也不隐藏。

大型PDecode两轮中位快3.05%/2.66%，但第二轮累计慢994.4ns；含数组记录Decode第二轮中位快4.52%、累计慢8387.8ns，全部保留。若只公布有利统计量会高估组合收益。本轮不是新PB/TS排名，不能把旧PB绝对耗时乘配对比例构造新的领先数字。

## 控制与验收

self39/70、clone37/70，所有对象数分配中位相同，29项字节尾差、21入口中位/累计反向与全部GC/最差轮保留。最大堆快照32,841,888B非峰值，limiter零；四模式最长块 170.3/260.9/233.5/127.0ms。多项控制失败，未扣控制制造校正收益，不宣称每个小差值已证明统计显著。

314条测量流程命令成功（含计时前新安全用例race/checkptr）；fresh/18份冻结Go源/工具/生成输入/夹具/harness/原采样函数/二进制/raw及顺序核对。另在未改af9169d基线上验证永久回归，再执行7条完整验收：库全测＋固定官方C/Lua oracle、全库race/严格checkptr、vet、Codec/Decode各10秒有限fuzz、临时modfile的GSR三包、真实GoTS7/7且零skip。

Codec fuzz472819次，Decode9614次；Decode在3秒后无新增执行，有限时间正常退出，原文保留，不盲目重试或描述成持续高吞吐fuzz。所有验收在含新增测试的完整候选快照执行，实际运行时代码与原库最终文件逐字节核对；GoTS二进制的实际replace来源保存。原库go.mod等6项删除不恢复、不纳入本次改动，所以不直接在缺文件工作区跑全库测试。独立库本轮只改decode.go、CHANGES.md并新增安全测试；GSR正式go.mod/go.sum保持HEAD。

运行时/公开API/所有权不变，但增加了受证明约束的unsafe切片借用，有限验证不等于任意平台绝对安全承诺。后续修改必须保留类型回退、边界与GC屏障。无推送/发布/tag；库已提交 e43df35，报告随本次 GSR 提交归档，双主线继续。

[安全评审与编译证据](safety-review.json)、[完整验收](acceptance.json)、[基线回归](baseline-regression-check.json)、[实际GoTS来源](integration-build-info.txt)。

```sh
python3 -B docs/benchmarks/2026-10-20-gosproto-decode-struct-slots/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-decode-struct-slots-new
python3 -B docs/benchmarks/2026-10-20-gosproto-decode-struct-slots/analyze.py \
  tmp/gosproto-decode-struct-slots-new
```

输出目录必须不存在。性能脚本从af9169d归档应用精确patch，不依赖库工作树。accept.py要求完整候选快照，并加入与permanent-test.go.txt一致的新测试；不能对缺go.mod的原库工作树直接执行。原始源码/二进制/逐项进程在忽略tmp，聚合raw/复算/机器码/来源随报告保留。
