# Header 字节偏移遍历实验（未采用）

2026-10-19。上一轮固定len/cap2读取视图已本地提交为gosproto `503a725`、GSR `2104eca`，两库提交后干净，未推送。本轮以503a725为独立增量基线，只验证字节偏移遍历；**未采用，主库从未应用补丁，18份Go源码仍等于503a725且工作树干净**。新实验与文档未提交，正式依赖v0.4.0，Decode继续开放。

## 候选与语义证明

旧循环按index=0…count−1计算header起点2+index*2。候选在进循环前保存headerEnd := total，改为headerOffset从2起、每次加2、到headerEnd前结束；readUint16仍使用固定len/cap2视图。total随后会随payload消费增长，**不能把可变total用作循环上界**，候选显式保留初始末端。

初始total=2+2*count已验证≤len(chunk)，新offset序列恰为旧2+2*index，共count次；continue跳跃/未知tag也推进一次，count=0不读取。最大端点131072，即使32位int也不溢出；读取相同两个字节，不借输入隐藏cap越过len。header预算仍按原count计数，tag/chunk/scalar校验、错误优先级、递归深度、元素/字节预算、完整消费、失败时保留原值、presence/独占、实际Go类型/逐字段owner/Wire/Array验证和GC写屏障均不动。

没有添加切片游标、unsafe、分配、池、缓存、parser或公开API；Encoder/Pack/Unpack/RPC/meta、结构数组和标量构造/写入保持原源码。本轮不将前后独立实验百分比相加，也不把旧基线排名当成新结果。

## 预检查与测量范围

[预检查](preflight/inputs.json)从503a725归档前后版本，实际四份配对二进制再次确认：[parser符号](assembly-summary.json)660→656条，栈帧320B不变；每轮索引倍增LSL与部分起点运算消失。边界检查、原生标记两次加载、owner/Wire/Array守卫和真实gcWriteBarrier2保留，结构数组仍208条/224B。静态指令减少不直接作为采纳条件，也不能据此定位所有运行成本。

原40项构造probe逐字节复用：五类代表消息×Decode/Unpack/PDecode/真实通知Rpc.Dispatch=20，四档Echo×payload/frame decode=8，五种增长形态×Decode/PDecode=10，大型Encode/PEncode控制2项。每次创建独占结果，不复用调用方对象；RPC计时不包含需重新登记session的响应Dispatch。

基线/候选既有记录tag/预算、pointer ownership、padding/GC、Fields替换、坏标量/未知字段、schema演进、预算及RPC契约先通过；harness统计/选择/GC/夹具/默认指标与vet、40入口wire/值/消费长度和109形态525项编码差分通过。后者保护未改编码，不冒称109种Decode全覆盖；没有新增镜像实现测试。

固定self/clone/两轮各40个独立进程，一次采完。最终计划、patch、probe、driver在首样前冻结；构造同组后只保留选中闭包，清三个sink并GC，预热50ms和有界校准。原sampleBlock/blockCalls不变，GOMAXPROCS1、GOGC100、无限MemoryLimit；6轮×4组ABBA/BAAB×4块，每臂三次128调用分配观测，四快照在计时外。构建/测试/vet/汇编先于首样，fresh归档在计时前另建，不穿插profile、不改输入、不重跑或删除异常、不扣除控制漂移。

## 结果与取舍

**保留503a725，不采用当前字节偏移候选。** 大型Decode两轮配对−0.44%/−0.11%、PDecode−0.81%/−1.03%、通知Dispatch−0.94%/−0.17%，这些局部收益如实保留；但对应两类控制均失败，第二轮大型Decode/PDecode累计反而+681.7/+277.0ns，第一轮Dispatch累计+167.1ns，不能只挑配对中位数称整体改善。

小型通知Dispatch快1.21%/1.16%（少29.0/27.4ns）、小型PDecode快0.26%/1.97%，有控制失败；这些收益没有因比例小被舍弃。相反，整数数组PDecode两轮+1.10%/+1.24%（多84.7/97.3ns）、累计+620.0/+850.3ns，两类控制均通过。含数组记录4096 PDecode+1.68%/+1.57%（多8,340.2/7,889.7ns）、累计也更慢，控制各有一项失败；full4096纯Decode两轮也观察到成本。nil PDecode、零字节frame有重复成本，控制不完整。

当前证据未显示足以抵消这些组合路径成本的稳定增量，因此先保留已验收实现。没有使用最低百分比门槛，也不把少四条指令当成必须保留的理由。观察到的回退不唯一归因于循环源码：未改的Unpack与Encode/PEncode也漂移，全部保留且不相减校正。没有在采样后追加变体或重跑到控制通过。

负号更快；比例为六轮配对比值中位数，ns为轮级两臂几何耗时差中位数，不是独立中位数相减。逐轮、累计、失败控制与分配见[分析](analysis.json)。

| 入口 | 第一轮：比例（差值） | 第二轮：比例（差值） | self / clone控制 |
| --- | ---: | ---: | --- |
| `body/addressbook/decode` | +0.23%（+2.4ns） | -0.49%（-5.0ns） | 通过 / 未通过 |
| `body/addressbook/unpack` | -0.19%（-0.4ns） | -2.12%（-3.7ns） | 未通过 / 通过 |
| `body/addressbook/pdecode` | +0.21%（+2.4ns） | -0.14%（-1.6ns） | 通过 / 通过 |
| `rpc/addressbook/dispatch` | -0.77%（-10.9ns） | +1.24%（+17.3ns） | 未通过 / 通过 |
| `body/game_small/decode` | -1.21%（-20.0ns） | +1.43%（+23.0ns） | 通过 / 通过 |
| `body/game_small/unpack` | +1.04%（+5.2ns） | -0.67%（-3.4ns） | 通过 / 通过 |
| `body/game_small/pdecode` | -0.26%（-4.5ns） | -1.97%（-42.4ns） | 通过 / 未通过 |
| `rpc/game_small/dispatch` | -1.21%（-29.0ns） | -1.16%（-27.4ns） | 通过 / 未通过 |
| `body/game_large/decode` | -0.44%（-68.1ns） | -0.11%（-17.0ns） | 未通过 / 未通过 |
| `body/game_large/unpack` | -0.79%（-39.9ns） | +0.36%（+18.2ns） | 通过 / 通过 |
| `body/game_large/pdecode` | -0.81%（-169.2ns） | -1.03%（-213.1ns） | 未通过 / 未通过 |
| `rpc/game_large/dispatch` | -0.94%（-200.7ns） | -0.17%（-36.2ns） | 未通过 / 未通过 |
| `body/int32_edges/decode` | +0.81%（+18.8ns） | -0.33%（-7.8ns） | 通过 / 未通过 |
| `body/int32_edges/unpack` | -1.42%（-80.7ns） | -0.07%（-3.8ns） | 通过 / 通过 |
| `body/int32_edges/pdecode` | +1.10%（+84.7ns） | +1.24%（+97.3ns） | 通过 / 通过 |
| `rpc/int32_edges/dispatch` | +0.37%（+30.3ns） | -0.95%（-82.7ns） | 通过 / 未通过 |
| `body/echo_ascii4k/decode` | +1.94%（+13.2ns） | -1.76%（-12.2ns） | 未通过 / 通过 |
| `body/echo_ascii4k/unpack` | -1.29%（-7.1ns） | +0.77%（+4.3ns） | 未通过 / 未通过 |
| `body/echo_ascii4k/pdecode` | -0.47%（-6.6ns） | +0.10%（+1.3ns） | 未通过 / 未通过 |
| `rpc/echo_ascii4k/dispatch` | -2.21%（-34.9ns） | +1.96%（+32.0ns） | 未通过 / 通过 |
| `echo/ascii32/payload_decode` | +1.31%（+2.2ns） | -1.89%（-3.2ns） | 未通过 / 通过 |
| `echo/ascii32/frame_decode` | -0.39%（-1.5ns） | +0.26%（+1.0ns） | 通过 / 通过 |
| `echo/utf8_1k/payload_decode` | +0.61%（+1.8ns） | +1.59%（+4.6ns） | 未通过 / 未通过 |
| `echo/utf8_1k/frame_decode` | +0.12%（+0.7ns） | -0.78%（-5.0ns） | 通过 / 未通过 |
| `echo/ascii4k/payload_decode` | -2.34%（-16.8ns） | +0.76%（+5.2ns） | 未通过 / 通过 |
| `echo/ascii4k/frame_decode` | -0.96%（-14.9ns） | -3.40%（-51.8ns） | 通过 / 未通过 |
| `echo/zeros4k/payload_decode` | -2.38%（-16.8ns） | +3.88%（+28.0ns） | 通过 / 未通过 |
| `echo/zeros4k/frame_decode` | +1.34%（+19.8ns） | +5.08%（+74.5ns） | 未通过 / 通过 |
| `growth/flat/full/4096/decode` | +0.38%（+1739.0ns） | +0.95%（+4355.3ns） | 未通过 / 通过 |
| `growth/flat/full/4096/pdecode` | +0.89%（+5559.3ns） | -1.54%（-9961.9ns） | 未通过 / 未通过 |
| `growth/flat/nil/4096/decode` | -0.41%（-853.5ns） | -0.37%（-772.3ns） | 通过 / 通过 |
| `growth/flat/nil/4096/pdecode` | +1.87%（+4303.9ns） | +1.28%（+2982.3ns） | 通过 / 未通过 |
| `growth/flat/half/4096/decode` | -0.15%（-526.1ns） | -0.25%（-945.2ns） | 通过 / 未通过 |
| `growth/flat/half/4096/pdecode` | +0.26%（+1206.4ns） | -0.96%（-4324.3ns） | 未通过 / 通过 |
| `growth/nested/32x128/decode` | +1.25%（+5783.7ns） | +0.39%（+1815.4ns） | 未通过 / 未通过 |
| `growth/nested/32x128/pdecode` | -2.13%（-14267.8ns） | +0.74%（+7210.6ns） | 未通过 / 未通过 |
| `growth/array_record/4096/decode` | +0.79%（+3009.9ns） | -0.35%（-1412.4ns） | 通过 / 未通过 |
| `growth/array_record/4096/pdecode` | +1.68%（+8340.2ns） | +1.57%（+7889.7ns） | 未通过 / 通过 |
| `control/game_large/encode` | +1.36%（+78.6ns） | +5.17%（+281.0ns） | 通过 / 未通过 |
| `control/game_large/pencode` | +3.82%（+267.4ns） | +1.55%（+111.3ns） | 未通过 / 通过 |

## 分配、控制与异常

160进程的15,360块、3,840 quartet、960轮、640快照从逐块数据独立复算到1e-12一致。self20/40、clone19/40满足原控制标记，失败项、IQR和全部逐轮值保留；所有快照GOGC100、无限MemoryLimit、limiter零。最大采样堆21,141,688B，约20.16MiB，是快照不是峰值。

160组对象数中位数均相同；字节和对象数同时相同为self35、clone33、两轮34/36。22项字节尾差范围−0.25～+0.625B/op（128次中−32～+80B），见[原值](allocation-tails.json)，不截断以令控制通过，也不强行定因。

[累计翻转](cumulative-reversals.json)涉及16入口、18个比较轮次。嵌套PDecode第一轮配对−14,267.8ns、累计+4,522.5ns，第二轮配对+7,210.6ns、累计−358,160.1ns；nil Decode第一轮配对−853.5ns、累计+62,383.2ns。极端累计差不能替代配对统计，配对更快也不代表累计一定更快。

[最大块](long-blocks.json)：self零4KiB frame546.188ms（6,894次/22GC），clone ASCII4KiB frame71.829ms（7,170次/22GC），第一轮nil4096 Decode181.930ms（48次/2GC），第二轮嵌套PDecode395.300ms（15次/3GC）。[最大轮偏移](worst-rounds.json)分别约+41.68%、+92.71%、+31.33%、+84.93%。所有暂停/GC块保留，不因其不利而删除，也不唯一归因于某个调度或GC因素。

## 最终源码与复现

主库未应用候选，18份Go源码与503a725逐字节相同，工作树干净；独立候选/patch/二进制指纹保留，不恢复或清理用户文件。正式go.mod/go.sum等于HEAD、仍依赖v0.4.0。新GSR文档未提交，未推送/发布/tag，Decode继续开放。

本轮完成前述目标契约、harness/vet、差分、四组构建/汇编、160项固定采样、独立复算、fresh和全部源码/工具/夹具/生成输入/二进制/单项raw SHA核对。因未采用候选，没有重复库全测/官方C-Lua/race/checkptr/fuzz、GSR三包/GoTS、PB/TS排名、CPU/分配profile或跨平台；正式实现完整验收仍见503a725[固定视图报告](../2026-10-19-gosproto-decode-header-view/RESULTS.md)，不把旧测试称为本轮新执行。

聚合raw、编译器/汇编、摘要与来源随报告保留；可执行文件、源码副本及单项raw在忽略的tmp目录，通过SHA连接。反汇编仅去行末空白，原文/整理后指纹均保留。

从GSR根目录复现，输出目录须不存在，库须包含503a725：

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-decode-header-offset/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-decode-header-offset-new
python3 -B docs/benchmarks/2026-10-19-gosproto-decode-header-offset/analyze.py \
  tmp/gosproto-decode-header-offset-new
```

--prepare-only只建输入。后续以503a725为基线继续按profile/源码证据选择热点，不继续围绕同一循环计数形式追加变体；资源、元信息、失败原子性、独占与写屏障约束不变，不重新冻结Decode。
