# 结构数组子 schema 局部复用实验（未采用）

2026-10-19。上一轮Decode入口优化已本地提交为gosproto `913c465`、GSR `b708108`，两库提交后干净，未推送。用户已解除Decode阶段性冻结；本轮只验证一个局部候选，**未采用，正式库全部18份Go源码仍与913c465相同且工作树干净**。新实验报告留待审查，正式依赖仍v0.4.0，Decode继续开放。

## 选点、补丁与代码生成

前轮大型消息CPU诊断显示decodeStructSlice是通用decodeMessage递归的一部分，独占子结构构造和反射写入仍可见。其累计份额与父函数重叠，不能相加或当作加速上限；这是此前基线诊断，本轮未新采CPU/分配profile。该函数本体在前轮入口优化中未改，本轮从913c465重建两侧，直接检查实际汇编。

旧循环每个元素使用两次field.st：一次为reflect.New提供typ，另一次交给decodeMessage。唯一候选在countChunks校验与MakeSlice之后声明schema := field.st，循环内改用schema.typ/schema。它只保存私有子schema链接，不缓存字段描述、解码值、地址或跨调用证明；每个元素仍reflect.New，parser仍即时读取公开Fields及当前描述。field.st没有公开setter或用户回调可以在本次同步调用内改变，未承诺并发修改元信息安全。

没有把schema.typ提前到循环外求值，空数组不会新增对schema的解引用；缺失/空数组、wire边界、元素/深度/字节预算、消费长度、错误顺序、临时结果提交、presence/独占/GC写屏障均保持。无新unsafe、池、API、解析器或分配模型，不改Encode/Pack/Unpack/RPC/meta。

[预检查](preflight/inputs.json)使用独立前后归档与同一真实harness二进制；[主矩阵汇编摘要](assembly-summary.json)再次确认：decodeStructSlice两侧均208条反汇编指令，field.st的循环两次加载变为循环前一次加载，局部schema仍要在调用前后保存/重载。**栈帧224→240B**，不是零代价；原内联入口和真实类型化赋值写屏障继续保留。条数相同、少一次字段定位都不能证明加速，栈帧变化也不能单独定因或否定候选。

## 固定范围与实际验证

复用前轮40项构造probe，内容逐字节相同：五类代表消息×Decode/Unpack/PDecode/真实通知Rpc.Dispatch=20，四档Echo×payload/frame decode=8，五种增长形态×Decode/PDecode=10，大型Encode/PEncode控制2项。继续每次创建新的独占结果，不复用目标或pool；RPC是通知Dispatch，未计时需要登记session的响应路径。

基线/候选先运行既有记录tag/预算、presence/ownership、padding/GC、公开Fields替换、未知字段/坏标量、schema演进、预算边界及RPC契约；harness统计/选择/GC/夹具/默认指标与vet、40项wire/语义/消费长度、109形态525项编码差分通过。后者是未改编码控制，不称为109种Decode全面覆盖。本次没有新增生产测试或镜像私有实现的用例。

计划、probe、patch、driver在首次计时前固定。原sampleBlock/blockCalls逐字节不变，GOMAXPROCS1、GOGC100、无限GOMEMLIMIT；同组构造后只保留所选闭包、清三个sink并GC，预热50ms及有界校准。六轮×四ABBA/BAAB quartet×四块，每臂三次128调用分配观测，四快照在计时外。

self/clone/两轮比较各40个独立进程，一次采完，共160进程；所有构建/测试/vet/汇编先于第一项计时，不穿插到比较过程中。fresh prepare-only归档在计时前另行重建，计时后核对两套全部输入与源码相同。首次预检查构建因用户Go缓存写权限失败，授权使用原缓存后成功，未清缓存；失败命令与后续输出完整留在preflight/commands.json，发生于任何正式计时之前，没有计时重试或输入修补。

## 结果与取舍

**此次不采用，是目标路径的收益/成本取舍，不是最低百分比门槛。** 含数组记录4096的纯Decode两轮快0.92%/1.05%（少4,482.9/3,973.2ns），两类控制通过；其PDecode约−0.01%/−1.49%。这些收益没有被抹去或因“小于某个比例”否认。

但大型消息Decode两轮+1.25%/+0.79%（多185.0/119.5ns）、通知Dispatch+0.48%/+0.65%（多96.8/132.0ns），累计方向也更慢，相关控制部分或全部失败，归因有局限。更有约束力的是nil4096：Decode+1.59%/+1.81%、PDecode+1.25%/+1.87%，四条比较的累计方向也更慢，两类控制均通过；小型纯Decode/通知同样两轮观察到成本。

full/half/嵌套的PDecode有改善，但多个控制失败，纯Decode不一致；不能用其中一项组合收益抵消其他入口成本。未改的大型Encode第二轮+4.93%、PEncode第一轮+6.88%显示漂移，不能归因于这个Decode候选，也不能扣掉它们校正结果。当前证据不支持把它作为通用结构数组解码优化纳入库；这不否认局部获益，也不证明静态栈帧变化就是原因。未在采样后加变体或重跑到控制通过。

负号更快。比例为六轮配对比值中位数，ns为轮级两臂几何耗时差中位数，不是独立中位数相减；累计耗时、逐轮值及所有失败控制另见[独立分析](analysis.json)。

| 入口 | 第一轮：比例（差值） | 第二轮：比例（差值） | self / clone控制 |
| --- | ---: | ---: | --- |
| `body/addressbook/decode` | +0.22%（+2.2ns） | -1.54%（-15.5ns） | 通过 / 通过 |
| `body/addressbook/unpack` | -1.73%（-3.4ns） | -0.13%（-0.2ns） | 通过 / 通过 |
| `body/addressbook/pdecode` | +0.10%（+1.1ns） | +0.56%（+6.5ns） | 未通过 / 通过 |
| `rpc/addressbook/dispatch` | -0.02%（-0.3ns） | -1.78%（-26.4ns） | 通过 / 通过 |
| `body/game_small/decode` | +0.91%（+14.3ns） | +0.53%（+8.5ns） | 通过 / 通过 |
| `body/game_small/unpack` | +0.38%（+1.9ns） | +0.21%（+1.0ns） | 通过 / 通过 |
| `body/game_small/pdecode` | -0.17%（-3.5ns） | -1.43%（-28.6ns） | 通过 / 通过 |
| `rpc/game_small/dispatch` | +0.20%（+4.4ns） | +1.10%（+24.9ns） | 通过 / 未通过 |
| `body/game_large/decode` | +1.25%（+185.0ns） | +0.79%（+119.5ns） | 未通过 / 未通过 |
| `body/game_large/unpack` | -0.05%（-2.4ns） | +0.56%（+26.8ns） | 通过 / 未通过 |
| `body/game_large/pdecode` | +1.04%（+208.2ns） | -0.09%（-17.5ns） | 未通过 / 未通过 |
| `rpc/game_large/dispatch` | +0.48%（+96.8ns） | +0.65%（+132.0ns） | 未通过 / 通过 |
| `body/int32_edges/decode` | +0.11%（+2.5ns） | +0.18%（+4.2ns） | 通过 / 通过 |
| `body/int32_edges/unpack` | -0.20%（-10.7ns） | +0.64%（+33.7ns） | 通过 / 通过 |
| `body/int32_edges/pdecode` | -0.04%（-2.8ns） | -0.24%（-18.4ns） | 通过 / 通过 |
| `rpc/int32_edges/dispatch` | +0.08%（+6.0ns） | +0.41%（+32.4ns） | 未通过 / 通过 |
| `body/echo_ascii4k/decode` | -2.57%（-15.8ns） | -0.63%（-4.3ns） | 通过 / 未通过 |
| `body/echo_ascii4k/unpack` | +0.24%（+1.1ns） | +0.62%（+3.2ns） | 通过 / 通过 |
| `body/echo_ascii4k/pdecode` | +1.94%（+24.1ns） | +0.20%（+2.3ns） | 通过 / 通过 |
| `rpc/echo_ascii4k/dispatch` | -0.63%（-8.8ns） | -1.10%（-15.6ns） | 通过 / 未通过 |
| `echo/ascii32/payload_decode` | -0.20%（-0.3ns） | +0.14%（+0.2ns） | 未通过 / 通过 |
| `echo/ascii32/frame_decode` | -3.06%（-3.2ns） | -0.03%（-0.1ns） | 通过 / 通过 |
| `echo/utf8_1k/payload_decode` | +0.36%（+1.0ns） | +1.02%（+2.8ns） | 通过 / 通过 |
| `echo/utf8_1k/frame_decode` | +0.34%（+2.2ns） | -0.49%（-3.0ns） | 未通过 / 通过 |
| `echo/ascii4k/payload_decode` | +0.29%（+1.8ns） | +0.89%（+5.6ns） | 通过 / 通过 |
| `echo/ascii4k/frame_decode` | -0.08%（-1.1ns） | -0.05%（-0.7ns） | 通过 / 通过 |
| `echo/zeros4k/payload_decode` | -0.03%（-0.2ns） | -0.20%（-1.3ns） | 通过 / 通过 |
| `echo/zeros4k/frame_decode` | +0.20%（+2.7ns） | +0.17%（+2.3ns） | 未通过 / 通过 |
| `growth/flat/full/4096/decode` | -0.02%（-115.1ns） | +0.24%（+1108.6ns） | 未通过 / 通过 |
| `growth/flat/full/4096/pdecode` | -0.05%（-331.1ns） | -0.74%（-4703.8ns） | 未通过 / 未通过 |
| `growth/flat/nil/4096/decode` | +1.59%（+3189.5ns） | +1.81%（+3666.4ns） | 通过 / 通过 |
| `growth/flat/nil/4096/pdecode` | +1.25%（+2969.0ns） | +1.87%（+4239.4ns） | 通过 / 通过 |
| `growth/flat/half/4096/decode` | +0.35%（+1226.0ns） | +0.66%（+2290.2ns） | 未通过 / 通过 |
| `growth/flat/half/4096/pdecode` | -0.36%（-1579.8ns） | -1.53%（-7150.6ns） | 未通过 / 未通过 |
| `growth/nested/32x128/decode` | -0.27%（-1289.5ns） | +0.20%（+964.2ns） | 通过 / 未通过 |
| `growth/nested/32x128/pdecode` | -1.27%（-8275.3ns） | -1.48%（-9776.1ns） | 未通过 / 未通过 |
| `growth/array_record/4096/decode` | -0.92%（-4482.9ns） | -1.05%（-3973.2ns） | 通过 / 通过 |
| `growth/array_record/4096/pdecode` | -0.01%（-77.4ns） | -1.49%（-7380.9ns） | 通过 / 通过 |
| `control/game_large/encode` | +0.64%（+34.7ns） | +4.93%（+274.1ns） | 通过 / 通过 |
| `control/game_large/pencode` | +6.88%（+455.6ns） | +1.09%（+75.9ns） | 通过 / 通过 |

## 控制、异常与分配限制

15,360块、3,840 quartet、960轮、640快照独立复算到1e-12一致。self27/40、clone30/40满足原控制标记；limiter始终零，默认GC与MemoryLimit在所有快照核对。最大采样堆13,710,752B，约13.08MiB，是快照而非峰值。

160组对象数中位数相同，分配字节/对象数同时相同为self35、clone34、两轮各33项。剩余25项B/op差值落在−0.5～+0.625B/op，对应128次调用中−64～+80B窗口差，完整见[分配尾差](allocation-tails.json)。不截断计数、不称为新增对象；本轮没有重新定因这些tiny/GC窗口尾差。

[累计翻转](cumulative-reversals.json)涉及22入口、25个比较轮次。第二轮AddressBook PDecode配对+6.5ns，累计却−8,207.6ns；第一轮含数组记录Decode配对−4,482.9ns，累计−313,401.7ns，两种统计差异明显，不能只挑最有利的数字。

[最大块](long-blocks.json)分别为：self ASCII4KiB payload71.454ms（16,284次/22GC），clone ASCII4KiB frame75.406ms（7,488次/23GC），第一轮含数组记录Decode342.113ms（27次/2GC），第二轮AddressBook PDecode1,858.944ms（5,117次/1GC）。[最大轮偏移](worst-rounds.json)分别约−21.28%、+32.15%、+40.79%、+130.31%。所有原值保留，不移除GC/暂停块，不把累计极值当作候选收益。观察到的暂停不能唯一归因于调度、GC或本次源码变化。

## 最终状态与复现

主库从未应用该候选；当前18份Go源码与913c465逐字节相同，工作树干净。candidate.patch与独立副本保留，不对用户文件执行恢复/清理。正式GSR go.mod/go.sum等于HEAD，依赖仍v0.4.0。新GSR文档/实验未提交，未推送、发布或打tag。

本轮实际执行的是前述目标契约、harness/vet、结构差分、四组构建/汇编、固定160项采样、独立复算、fresh和全部源码/二进制/单项SHA核对。因候选未纳入主库，未重复库全测/官方/race/checkptr/fuzz、GSR三包/GoTS、PB/TS性能或跨平台验证；正式实现的完整验收仍见上轮913c465[入口报告](../2026-10-19-gosproto-decode-resume/RESULTS.md)，不把旧测试称为本轮新执行。

源码、工具、夹具、生成输入与实际二进制指纹见inputs/validation，聚合raw、摘要、编译器、汇编随报告保留；单项JSON、二进制与源码副本在忽略的tmp目录，以SHA连接。反汇编仅去行末空白，原文与整理后SHA均保留。

从GSR根目录复现，输出目录须不存在，库须含913c465，脚本从提交和固定补丁构建：

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-decode-array-schema/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-decode-array-schema-new
python3 -B docs/benchmarks/2026-10-19-gosproto-decode-array-schema/analyze.py \
  tmp/gosproto-decode-array-schema-new
```

--prepare-only只建输入，不采样。Decode继续开放；下一步先核对parser中原生整数标记、owner/形状验证是否在实际机器码中重复，再决定有无不扩大unsafe、不放松元信息替换与写屏障契约的候选。不会因一次局部失败冻结Decode，也不直接转向目标复用、对象池或第二套parser。
