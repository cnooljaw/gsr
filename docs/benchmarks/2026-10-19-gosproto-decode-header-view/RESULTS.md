# 固定两字节 header 读取视图

2026-10-19。整数资格前移实验已本地提交为GSR `f7381dd`，gosproto基线仍为已验收的 `913c465`，未推送。本轮保留一个单行Decode优化，代码与新报告未提交，正式依赖v0.4.0，Decode继续开放。

## 改动与边界证明

只将decodeMessage的readUint16参数从开放尾切片改为固定len/cap=2视图：

```go
entry := int(readUint16(chunk[2+index*2 : 4+index*2 : 4+index*2]))
```

count由uint16读取，原代码在循环前验证total=2+2*count不超过len(chunk)。对0≤index<count，视图终点4+2*index≤total；即使32位int，最大终点131072也不溢出。count=0时不创建视图，短header在进循环前拒绝。即使输入cap大于len，视图仍由这份len校验约束，不利用隐藏容量读取新字节。新旧都只向同一个LittleEndian.Uint16提供相同的两个字节。

只缩小同步读取的临时视图，未改公开结果的len/cap/独占模型，没有新增分配、unsafe、池、缓存、parser或API。未知tag仍先验证其chunk边界；scalar、tag、header/元素/深度/字节预算、错误优先级、失败时保留原值、presence、公开Fields替换、实际Go类型和逐字段写入守卫、GC写屏障均不动。Encoder/Pack/Unpack/RPC/meta、整数分配与结构数组实现不改。

只读Skynet的sproto.c中toword直接读两个小端字节，Decode先检查完整header，再逐个读取；这支持保持固定宽度读取边界，不能借其C指针模型省掉Go的边界和资源检查。[源码引用指纹](c-reference.json)明确这是本地Skynet源码，不声称等同于固定官方提交；实际兼容验收仍用已固定C/Lua oracle。

## 计时前预检查

最初两索引写法只固定len，仍保留cap/指针NEG/AND调整；证据、补丁和原计划保存在two-index-preflight，未做任何计时。将实现意图补全为len/cap都固定2后，另建独立预检查归档与fresh输入，再固定最终计划/补丁开始采样。两份准备归档都保留，不把它描述为计时重试或在看过数据后改候选。

三索引视图的实际parser符号指令664→660，栈帧320B不变；header读取的尾长度/容量调整及NEG/AND消失，但仍保留端点边界检查，相关panic入口形状改变。原生标记两次加载、owner/Wire/Array守卫与gcWriteBarrier2保持；结构数组函数208条/224B不变。[实际二进制摘要](assembly-summary.json)再次验证，少四条指令不直接等于业务收益。

## 固定实验与覆盖

复用前两轮逐字节相同的40项构造probe：五类代表消息×Decode/Unpack/PDecode/真实通知Dispatch=20，四档Echo×payload/frame decode=8，五种增长形态×Decode/PDecode=10，大型Encode/PEncode控制2项。输入/wire与每次新建独占结果不变，RPC仍为真实通知Rpc.Dispatch，不测需要新登记session的响应Dispatch。

计时前完成两侧既有记录tag/预算、pointer ownership、padding/GC、Fields替换、未知字段/坏标量、schema演进、预算与RPC契约，harness统计/选择/GC/代表夹具/默认指标及vet，40入口wire/完整结果/消费长度和109形态525项编码控制差分。没有为相同契约新增镜像生产测试；编码差分不冒称109种Decode全面覆盖。

self/clone/两轮比较各40独立进程，一次采完；最终计划/patch/driver/probe在首样前固定。构造同组后只保留单项闭包、清三个sink并GC，预热50ms和有界校准。原sampleBlock/blockCalls逐字节不变，GOMAXPROCS1、GOGC100、无限MemoryLimit，6轮×4组ABBA/BAAB×4块；每臂三次128调用分配观测，4快照在计时外。所有构建、测试、vet、汇编先于首样，不穿插profile，不重跑或删除异常，不扣除控制偏差。最终fresh归档在计时前另建，计时后核对。

## 保留决定与成本

**保留低复杂度的局部收益，并接受明确成本；不宣称所有入口更快。** half4096 PDecode两轮快2.41%/2.54%（少11,141.8/11,794.6ns），累计少11,078.6/8,331.7ns，两类控制均通过。大型PDecode快1.72%/1.89%（少362.9/410.9ns），累计同向，但clone控制失败；嵌套Decode快5.03%/4.16%、PDecode快3.11%/3.94%，也有控制失败。这些重复信号和单行等价变更支持保留，不使用最低收益比例门槛。

大型纯Decode第一轮+0.33%、第二轮−1.26%，两类控制均失败，不能称它已稳定改善。含数组记录4096纯Decode第一轮+5.19%（+20,710.7ns）、第二轮+0.11%（+399.3ns），累计也更慢，两类控制均失败；PDecode方向混合。4KiB Echo真实Dispatch两轮+1.94%/+3.37%（+30.0/+52.9ns），小型游戏PDecode+1.38%/+0.64%（+32.4/+14.4ns）也保留。短UTF8 frame约多10.1/9.5ns，不称为无回退。

控制通过率本轮偏低：self21/40、clone13/40。未改大型Encode/PEncode、Unpack也有漂移，不把它们都归因于这一行，也不相减校正。保留决定以half组合入口的双控制和重复收益为主要证据，其他收益/成本有不确定性；不把40项当成无噪声排名，不合并旧基线百分比。

负号更快；比例是六轮配对比值中位数，ns是轮级两臂几何耗时差中位数，不是独立中位数相减。逐轮、累计和全部控制见[独立分析](analysis.json)。

| 入口 | 第一轮：比例（差值） | 第二轮：比例（差值） | self / clone控制 |
| --- | ---: | ---: | --- |
| `body/addressbook/decode` | -0.59%（-6.2ns） | -0.54%（-5.6ns） | 通过 / 未通过 |
| `body/addressbook/unpack` | -4.10%（-10.3ns） | -1.81%（-3.4ns） | 通过 / 未通过 |
| `body/addressbook/pdecode` | -1.05%（-13.2ns） | -2.17%（-27.3ns） | 通过 / 未通过 |
| `rpc/addressbook/dispatch` | +0.15%（+2.1ns） | +0.69%（+10.6ns） | 未通过 / 通过 |
| `body/game_small/decode` | -2.72%（-47.4ns） | -3.61%（-60.3ns） | 通过 / 未通过 |
| `body/game_small/unpack` | +0.07%（+0.3ns） | +0.91%（+4.6ns） | 未通过 / 通过 |
| `body/game_small/pdecode` | +1.38%（+32.4ns） | +0.64%（+14.4ns） | 未通过 / 未通过 |
| `rpc/game_small/dispatch` | +0.30%（+7.3ns） | -1.31%（-30.9ns） | 通过 / 通过 |
| `body/game_large/decode` | +0.33%（+51.5ns） | -1.26%（-191.0ns） | 未通过 / 未通过 |
| `body/game_large/unpack` | -0.53%（-27.4ns） | +0.93%（+47.5ns） | 通过 / 通过 |
| `body/game_large/pdecode` | -1.72%（-362.9ns） | -1.89%（-410.9ns） | 通过 / 未通过 |
| `rpc/game_large/dispatch` | -1.56%（-338.6ns） | -1.18%（-245.7ns） | 未通过 / 未通过 |
| `body/int32_edges/decode` | -0.62%（-14.9ns） | +0.49%（+11.7ns） | 未通过 / 未通过 |
| `body/int32_edges/unpack` | +0.70%（+37.6ns） | +1.24%（+68.4ns） | 通过 / 未通过 |
| `body/int32_edges/pdecode` | +0.58%（+46.1ns） | -0.30%（-23.4ns） | 未通过 / 未通过 |
| `rpc/int32_edges/dispatch` | +0.76%（+63.0ns） | -0.07%（-6.2ns） | 未通过 / 未通过 |
| `body/echo_ascii4k/decode` | -0.44%（-3.3ns） | -0.72%（-5.2ns） | 通过 / 未通过 |
| `body/echo_ascii4k/unpack` | +1.09%（+6.1ns） | -1.63%（-11.8ns） | 通过 / 未通过 |
| `body/echo_ascii4k/pdecode` | +1.27%（+16.2ns） | -0.84%（-11.4ns） | 未通过 / 通过 |
| `rpc/echo_ascii4k/dispatch` | +1.94%（+30.0ns） | +3.37%（+52.9ns） | 通过 / 未通过 |
| `echo/ascii32/payload_decode` | -1.28%（-2.2ns） | +0.23%（+0.4ns） | 通过 / 未通过 |
| `echo/ascii32/frame_decode` | +0.31%（+1.1ns） | -0.40%（-1.5ns） | 未通过 / 通过 |
| `echo/utf8_1k/payload_decode` | -1.66%（-5.0ns） | -0.59%（-1.7ns） | 通过 / 未通过 |
| `echo/utf8_1k/frame_decode` | +1.55%（+10.1ns） | +1.51%（+9.5ns） | 未通过 / 未通过 |
| `echo/ascii4k/payload_decode` | +0.02%（+0.6ns） | +1.52%（+12.0ns） | 未通过 / 通过 |
| `echo/ascii4k/frame_decode` | -2.40%（-37.3ns） | -1.79%（-29.0ns） | 未通过 / 通过 |
| `echo/zeros4k/payload_decode` | -1.09%（-8.1ns） | +1.66%（+12.2ns） | 通过 / 未通过 |
| `echo/zeros4k/frame_decode` | -1.46%（-24.5ns） | +0.68%（+10.3ns） | 未通过 / 通过 |
| `growth/flat/full/4096/decode` | -2.70%（-12795.7ns） | -0.33%（-1593.8ns） | 通过 / 未通过 |
| `growth/flat/full/4096/pdecode` | -2.08%（-13602.5ns） | -1.69%（-11451.6ns） | 未通过 / 未通过 |
| `growth/flat/nil/4096/decode` | -0.66%（-1458.1ns） | -2.07%（-4571.0ns） | 通过 / 未通过 |
| `growth/flat/nil/4096/pdecode` | +0.61%（+1494.2ns） | -1.16%（-2765.7ns） | 未通过 / 未通过 |
| `growth/flat/half/4096/decode` | -2.38%（-9106.6ns） | -2.24%（-8379.2ns） | 未通过 / 未通过 |
| `growth/flat/half/4096/pdecode` | -2.41%（-11141.8ns） | -2.54%（-11794.6ns） | 通过 / 通过 |
| `growth/nested/32x128/decode` | -5.03%（-25302.5ns） | -4.16%（-20987.1ns） | 通过 / 未通过 |
| `growth/nested/32x128/pdecode` | -3.11%（-21146.1ns） | -3.94%（-27183.3ns） | 通过 / 未通过 |
| `growth/array_record/4096/decode` | +5.19%（+20710.7ns） | +0.11%（+399.3ns） | 未通过 / 未通过 |
| `growth/array_record/4096/pdecode` | -1.53%（-7938.5ns） | +0.11%（+513.1ns） | 未通过 / 通过 |
| `control/game_large/encode` | +0.53%（+30.1ns） | -2.55%（-144.8ns） | 通过 / 通过 |
| `control/game_large/pencode` | +3.36%（+233.1ns） | +0.06%（+3.7ns） | 通过 / 通过 |

## 分配、异常和来源

160进程的15,360块、3,840 quartet、960轮、640快照独立复算到1e-12一致。所有快照GOGC100、无限MemoryLimit、limiter零；最大采样堆4,113,304B，约3.92MiB，是快照而非峰值。

160组对象数中位数全部相同；分配字节和对象数同时相同为self36、clone33、两轮33/36项。22项字节尾差范围−0.25～+0.375B/op，即128次调用中−32～+48B，见[原值](allocation-tails.json)。不截断尾差使控制通过，也不虚构新增对象或强行定因。

[累计翻转](cumulative-reversals.json)涉及15入口、16个比较轮次。大型通知Dispatch第一轮配对−338.6ns、累计却+1,461.4ns；nil4096 Decode第二轮配对−4,571.0ns、累计却+58,397.8ns。即使配对中位数更快，也不能称累计吞吐一定改善。

[最大块](long-blocks.json)：self大型Dispatch114.415ms（530次/3GC），clone echo_ascii4k Decode116.621ms（14,230次/20GC），第一轮echo_ascii4k PDecode75.496ms（8,050次/25GC），第二轮nil4096 Decode302.335ms（45次/2GC）。[最大轮偏移](worst-rounds.json)分别约+35.90%、+61.40%、+100.10%、−27.14%。所有原值保留，不唯一归因于调度/GC/本次源码，也不删块或重试到控制通过。

全部18份Go源码等于两轮冻结候选，只变decode.go这一行；各模式fresh输入、工具/生成代码/夹具/源码/实际二进制、160个单项raw SHA与命令顺序核对。聚合raw、摘要、编译器、汇编和来源随报告保存；单项raw/源码副本/可执行文件保留于忽略的tmp目录。反汇编仅去行末空白，原文和整理后SHA均在验证清单。

## 收尾验收与复现

应用精确补丁后完成[7项验收](acceptance.json)：独立库全测（启用固定官方C/Lua oracle）、race+严格checkptr=2全测、vet、10秒Codec fuzz432,127次、10秒Decode fuzz38,718次、临时modfile下GSR三包、Go/TS官方向量/ownership/session/预算及Plain/MAC集成7/7、零跳过。两项fuzz各新增1个interesting，无失败；Decode计数从3秒起未增长，原输出保留，不盲目延长或重跑，不称为广泛随机覆盖。

[官方来源](oracle-provenance.json)复用固定C core与Lua facade；parser来源单列，不冒称与固定官方提交相同。[集成构建信息](integration-build-info.txt)确认实际替换到本地候选。正式go.mod/go.sum逐字节等于HEAD、仍为v0.4.0，没有发布/推送/tag，也未重测未改Pack随机fuzz、GSR全仓/race、PB/TS排名或跨平台。

从GSR根目录复现，输出目录须不存在，库须包含913c465，脚本从提交和最终固定patch构建：

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-decode-header-view/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-decode-header-view-new
python3 -B docs/benchmarks/2026-10-19-gosproto-decode-header-view/analyze.py \
  tmp/gosproto-decode-header-view-new
```

--prepare-only只建输入，不计时；two-index-preflight只保存计时前的初稿证据，不参与结果。新库单行实现及GSR文档未提交，Decode继续开放。后续先检查header遍历与输入切片推进的实际成本，保留同样的边界/预算/错误/独占约束，不同时改字段守卫或分配模型。
