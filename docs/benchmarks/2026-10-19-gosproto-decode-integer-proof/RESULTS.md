# 原生整数写入资格前移实验（未采用）

2026-10-19。上一轮结构数组schema实验报告已本地提交为GSR `26a5748`，gosproto仍为已验收的 `913c465`；两库提交后干净，未推送。本轮只验证一个保持全部守卫的局部候选，**未采用，主库从未应用补丁，全部18份Go源码仍等于913c465且工作树干净**。新实验与文档未提交，正式依赖v0.4.0，Decode继续开放。

## 机器码问题与唯一候选

当前decodeMessage先判断field.nativeInt64Pointer，再校验scalarBits并new(int64)，最后由decodePointer.setInteger重新核对pointer、nativeInteger中的类型标记、owner、Wire和Array。[基线机器码](preflight/before-assembly.txt)确认：原生标记在分配前后各加载一次，源码中的重复在这个工具链下没有自动消失。

候选没有删除检查，而是让私有setInteger接收bits，先计算pointer.value != nil && field.nativeInteger(schema)，再创建/赋值唯一的独占int64对象；资格成立时类型化store，最后返回同一个对象及written。调用点在false时继续使用原反射字段索引回退。所有类型/owner/Wire/Array验证仍在，helper没有“调用方自行保证合法”式的新免检入口。

判断只跨过new和标量赋值：没有外部回调或私有字段修改；未承诺并发修改公开元信息安全，不缓存跨调用事实。scalarBits仍在前，所有输入长度、标量、tag、深度/元素/字节预算与错误优先级不改。每字段一个独占对象，foreign owner或形状变化仍用该对象反射写入，成功才提交根临时结果；presence/独占/失败原子性、实际record类型证明和GC写屏障不变。无新unsafe、池、API、parser或分配模型。Encode/Pack/Unpack/RPC/meta及结构数组实现不改。

## 编译器取舍

[预检查输入](preflight/inputs.json)及四份实际配对二进制再次核对：setInteger成本39→56，仍在调用点内联；decodeMessage符号指令664不变、栈帧320B不变、字段标记加载2→1。owner指针、Wire字符串内容与长度、Array形状守卫和真实gcWriteBarrier2均保留，结构数组函数仍208条/224B。

减少一次标记加载没有令所有机器工作减少：资格布尔值先保存到栈上，分配后再读取；write barrier路径与fallback的寄存器/分支也重新布局。静态条数、内联或少一次加载都不是性能结论；保留源/汇编证据，但没有单变量消融，不把任何一项单独宣称为回退原因。

## 固定实验与验证范围

完整复用前两轮40项构造probe：五组代表消息×Decode/Unpack/PDecode/真实通知Rpc.Dispatch=20，四档Echo×payload/frame decode=8，五种增长形态×Decode/PDecode=10，大型Encode/PEncode控制2项。输入/wire、所有结果独占和每次新建目标不变；RPC计时仍是通知Dispatch，未测需要登记session的响应Dispatch。

基线与候选既有记录tag/预算、pointer ownership、padding/GC、公开Fields替换、未知字段/坏标量、schema演进、预算及RPC契约在计时前通过；harness统计/选择/GC/代表夹具/默认指标与vet、40入口wire/值/消费长度和109形态525项编码差分通过。编码差分是未改部分的控制，不冒称109种Decode全覆盖；未新增镜像实现的生产测试。

四模式各40个独立进程，一次采完；计划/patch/probe/driver在首样前固定，不穿插构建/测试/profile。GOMAXPROCS1、GOGC100、无限MemoryLimit；同组构造后只保留单项闭包、清三个sink并GC，预热50ms与有界校准。原sampleBlock/blockCalls逐字节不变，6轮×4组ABBA/BAAB×4块，每臂三次128调用分配观测，4快照在计时外。fresh归档在计时前另建，采样后核对全部输入。预检查和主矩阵命令全部成功，没有准备或计时重试；使用已有Go缓存，没有清缓存、profile或新增PB/TS排名。

## 结果与取舍

**不采用当前资格前移方案。** 大型Decode两轮+1.76%/+1.51%（多271.8/233.6ns），PDecode+0.90%/+3.64%（多185.6/777.6ns），累计同向更慢，部分或全部控制失败，不能唯一归因。更有约束力的是含数组记录4096：Decode+2.31%/+1.22%（多8,386.7/4,548.1ns）、PDecode+1.37%/+0.84%（多6,763.7/4,160.9ns），四条比较累计都更慢，两类控制均通过。half4096 Decode/PDecode也两轮观察到成本，但部分控制失败。

小型PDecode两轮快0.18%/0.89%，嵌套PDecode快1.51%/0.78%，收益保留；后者两类控制均失败。Echo纯body和组合有数ns改善，但echo_ascii4k PDecode两轮配对更快、累计却更慢。大型通知Dispatch方向相反，不能挑第二轮称整体改善。

未改的Unpack、纯整数数组及Encode控制也漂移，不能都归因于此候选；也不能拿它们相减校正结果。这次不采用是主要目标成本与有限局部收益的整体取舍，没有最低收益比例门槛。不能据此推广成“重复检查优化总是无效”，也不会在同批采样后追加变体或重跑到通过。

负号更快；百分比为六轮配对比值中位数，ns为轮级两臂几何耗时差中位数，不是独立中位数相减；全部累计/逐轮/控制/分配见[独立分析](analysis.json)。

| 入口 | 第一轮：比例（差值） | 第二轮：比例（差值） | self / clone控制 |
| --- | ---: | ---: | --- |
| `body/addressbook/decode` | +0.53%（+5.3ns） | +0.66%（+8.7ns） | 通过 / 通过 |
| `body/addressbook/unpack` | -1.43%（-2.6ns） | +2.55%（+4.5ns） | 通过 / 通过 |
| `body/addressbook/pdecode` | +0.32%（+3.9ns） | +0.35%（+4.1ns） | 通过 / 通过 |
| `rpc/addressbook/dispatch` | +0.83%（+11.6ns） | -0.22%（-3.0ns） | 通过 / 通过 |
| `body/game_small/decode` | +0.12%（+2.0ns） | +0.16%（+2.6ns） | 未通过 / 通过 |
| `body/game_small/unpack` | +1.01%（+5.0ns） | +1.20%（+5.8ns） | 通过 / 未通过 |
| `body/game_small/pdecode` | -0.18%（-3.9ns） | -0.89%（-18.4ns） | 通过 / 通过 |
| `rpc/game_small/dispatch` | +0.75%（+17.2ns） | +0.17%（+3.8ns） | 未通过 / 通过 |
| `body/game_large/decode` | +1.76%（+271.8ns） | +1.51%（+233.6ns） | 通过 / 未通过 |
| `body/game_large/unpack` | +0.32%（+16.2ns） | -0.23%（-11.8ns） | 通过 / 通过 |
| `body/game_large/pdecode` | +0.90%（+185.6ns） | +3.64%（+777.6ns） | 未通过 / 未通过 |
| `rpc/game_large/dispatch` | +0.29%（+60.0ns） | -1.34%（-285.5ns） | 未通过 / 未通过 |
| `body/int32_edges/decode` | -0.38%（-8.9ns） | +0.66%（+14.9ns） | 通过 / 通过 |
| `body/int32_edges/unpack` | -0.10%（-5.3ns） | +2.47%（+136.9ns） | 通过 / 通过 |
| `body/int32_edges/pdecode` | +0.10%（+7.9ns） | +1.25%（+93.7ns） | 通过 / 通过 |
| `rpc/int32_edges/dispatch` | +0.56%（+42.2ns） | +0.30%（+23.3ns） | 未通过 / 通过 |
| `body/echo_ascii4k/decode` | -0.33%（-2.1ns） | -0.78%（-4.7ns） | 通过 / 未通过 |
| `body/echo_ascii4k/unpack` | +2.95%（+16.5ns） | +0.66%（+3.4ns） | 通过 / 通过 |
| `body/echo_ascii4k/pdecode` | -0.51%（-6.1ns） | -0.29%（-3.4ns） | 通过 / 通过 |
| `rpc/echo_ascii4k/dispatch` | +0.23%（+3.2ns） | +0.26%（+3.6ns） | 通过 / 未通过 |
| `echo/ascii32/payload_decode` | +1.09%（+1.8ns） | +1.38%（+2.3ns） | 通过 / 通过 |
| `echo/ascii32/frame_decode` | -0.01%（-0.0ns） | -6.23%（-24.0ns） | 未通过 / 通过 |
| `echo/utf8_1k/payload_decode` | +0.13%（+0.4ns） | +0.39%（+1.1ns） | 通过 / 通过 |
| `echo/utf8_1k/frame_decode` | +0.22%（+1.4ns） | +0.00%（+0.0ns） | 通过 / 未通过 |
| `echo/ascii4k/payload_decode` | +1.27%（+7.9ns） | +0.43%（+2.7ns） | 通过 / 通过 |
| `echo/ascii4k/frame_decode` | +0.04%（+0.6ns） | +1.26%（+17.2ns） | 通过 / 通过 |
| `echo/zeros4k/payload_decode` | -0.25%（-1.7ns） | +0.49%（+3.0ns） | 未通过 / 通过 |
| `echo/zeros4k/frame_decode` | +0.33%（+4.8ns） | -0.13%（-1.8ns） | 通过 / 未通过 |
| `growth/flat/full/4096/decode` | +0.57%（+2641.8ns） | -0.07%（-317.8ns） | 未通过 / 未通过 |
| `growth/flat/full/4096/pdecode` | +0.10%（+665.2ns） | +1.03%（+6509.7ns） | 未通过 / 未通过 |
| `growth/flat/nil/4096/decode` | +1.17%（+2521.1ns） | +2.61%（+5357.9ns） | 通过 / 未通过 |
| `growth/flat/nil/4096/pdecode` | +1.79%（+4023.3ns） | +2.35%（+5298.0ns） | 未通过 / 通过 |
| `growth/flat/half/4096/decode` | +1.27%（+4464.8ns） | +0.34%（+1194.3ns） | 通过 / 未通过 |
| `growth/flat/half/4096/pdecode` | +0.31%（+1423.0ns） | +0.99%（+4406.8ns） | 通过 / 未通过 |
| `growth/nested/32x128/decode` | -0.02%（-112.0ns） | +0.24%（+1122.0ns） | 未通过 / 未通过 |
| `growth/nested/32x128/pdecode` | -1.51%（-9903.2ns） | -0.78%（-5069.7ns） | 未通过 / 未通过 |
| `growth/array_record/4096/decode` | +2.31%（+8386.7ns） | +1.22%（+4548.1ns） | 通过 / 通过 |
| `growth/array_record/4096/pdecode` | +1.37%（+6763.7ns） | +0.84%（+4160.9ns） | 通过 / 通过 |
| `control/game_large/encode` | +0.43%（+24.0ns） | +0.69%（+37.2ns） | 通过 / 通过 |
| `control/game_large/pencode` | +1.48%（+101.1ns） | +0.51%（+35.7ns） | 通过 / 通过 |

## 控制、异常和分配

160进程的15,360块、3,840 quartet、960轮、640快照独立复算到1e-12一致。self28/40、clone25/40满足原控制标记；所有快照GOGC100、无限MemoryLimit、limiter零。最大采样堆25,092,232B，约23.93MiB，是快照不是峰值。

所有160组对象数中位数相同；分配字节/对象数同时相同为self34、clone33、两轮33/34。26项字节尾差范围−0.25～+0.5B/op（128次调用中−32～+64B），[原值](allocation-tails.json)完整保留，不强行定因或截断数字使控制通过。

[累计翻转](cumulative-reversals.json)涉及18入口、21个比较轮次；echo_ascii4k PDecode配对−6.1/−3.4ns，累计却+41.4/+41.0ns。未改的大型PEncode第一轮配对+101.1ns、累计+3,315.3ns，不能算成Decode源码成本，也不删除它对总体解释的限制。

[最大块](long-blocks.json)：self小型Dispatch171.835ms（4,817次/2GC）、clone含数组记录Decode265.308ms（27次/2GC）、第一轮未改大型PEncode218.205ms（1,511次/2GC）、第二轮AddressBook Decode105.473ms（10,799次/2GC）。[最大轮偏移](worst-rounds.json)分别约−32.59%、+36.10%、+72.65%、−40.39%。保留所有GC/暂停块、累计与配对数据，不以某一项极值推断真实收益或唯一暂停原因。

## 最终源码、验收边界和复现

库未应用该补丁，仍是上轮已验收的913c465；两轮候选与固定patch只保留在独立归档。当前18份Go源码、主库干净状态、两套fresh输入、生成代码/夹具/工具/二进制、160个单项raw SHA与计时顺序全部独立核对；反汇编仅去行末空白，原/整理后SHA在[验证清单](validation.json)。聚合raw、编译器/汇编、摘要和来源随报告保留，单项raw/可执行文件/源码副本留在忽略的tmp目录。

实际验证仅为前述目标契约、harness/vet、差分、四组构建/汇编、160项固定采样、独立复算与fresh指纹。候选未采用，未重复完整库/官方C-Lua/race/checkptr/fuzz、GSR三包/GoTS、CPU/内存profile、PB/TS性能或跨平台验证。正式实现完整验收仍对应913c465[入口报告](../2026-10-19-gosproto-decode-resume/RESULTS.md)，不把历史测试计作本轮执行。正式go.mod/go.sum等于HEAD，依赖v0.4.0，无推送/发布/tag。

从GSR根目录运行，输出目录须不存在；库须包含913c465，重建不依赖主工作树是否应用补丁：

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-decode-integer-proof/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-decode-integer-proof-new
python3 -B docs/benchmarks/2026-10-19-gosproto-decode-integer-proof/analyze.py \
  tmp/gosproto-decode-integer-proof-new
```

--prepare-only只建输入，不计时。下一步回到标量读取、独占构造与写入的整体执行布局，先定位实际热路径，避免继续仅围绕布尔CSE推演收益；不预设新parser、目标复用、对象池或删除守卫。新文档未提交，Decode继续开放，不因本次未采用再次冻结。
