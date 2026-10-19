# Decode 解冻与入口候选复核

2026-10-19，用户明确“decode不再冻结”，阶段性冻结已解除。Decode入口、parser、构造/写入可以按证据继续优化；公开API、官方wire、预算/错误优先级、失败原子性、presence、独占和GC写屏障契约保持。不会在本轮结束后自行恢复冻结。

上一轮非inline内容视图已本地提交为gosproto `e50dfef`、GSR `29c954b`，未推送，正式依赖仍为v0.4.0。本轮重新验证历史split入口候选，旧实验结论和当时门槛保留，本轮不使用最低收益百分比。

## 选点与当前证据

历史候选位于[旧冻结报告](../2026-10-17-gosproto-decode-freeze/RESULTS.md)：大型Decode/PDecode当时五轮同向、改善2.25%/1.28%，因预定3%/2%门槛未采用。这是重新测试的理由，不是直接采用的证据；代码/测量环境和矩阵以本轮归档为准，不与历史129项或近期70项控制分数混算。

当前基线CPU采样另起进程，构造新40项、只保留所选闭包、清结果sink并GC/预热50ms后手动启动，每项一次2秒；没有与memprofilerate=1混用。四项为大型Decode、PDecode、整数数组Decode、4KiB ASCII手工帧decode，全部先于第一项配对计时。

大型纯Decode中入口helper累计3.18%，通用decodeMessage累计46.36%、decodeStructSlice42.73%；父子累计重叠，不能相加。其余三个入口helper未进top40不能当作零。runtime.kevent在四项分别占51.36%、72.65%、73.97%、89.81%，原值保留，不能据此估算业务加速上限或断言其唯一成因。profile用于确认现有路径，耗时不是A/B基线。

## 唯一候选与边界

复用旧split补丁：parser已有 `record := value.Elem()`，即时核对 `schema.nativeInt64Pointer && record.Type() == schema.typ`，将same value与结果交给私有helper借用地址。对于parser传入的结构指针，value.Type().Elem()与value.Elem().Type()一致；核对仍针对同一对象，不缓存地址/事实，不扩展unsafe权限或去掉每字段owner/Wire/Array检查。

编译器确认旧helper成本216、不内联，新helper成本67、在调用点内联；decodePointer.setInteger仍内联且实际Go类型化赋值保留gcWriteBarrier2。parser符号指令616→664、320B栈帧不变；旧56条/64B独立helper消失。内联不等于parser变小，不能拿微基准或静态条数直接替代业务测量。Encode/Pack/Unpack/RPC/meta、字段分配与失败时先写临时结果的模型均不改。

## 新矩阵与验证方法

固定40项：五类既有代表消息×Decode/Unpack/PDecode/真实通知Dispatch=20；四档既有Echo×payload/frame decode=8；五种既有增长形态×Decode/PDecode=10；大型Encode/PEncode控制2项。RPC使用真实Rpc.Dispatch解通知帧，包含Unpack/header/body解析，不登记等待；本轮不测需要重新登记session的响应Dispatch耗时，既有RPC测试继续保护其行为。

代表输入/wire使用现有JSON与签名；增长选full/nil/half4096、32×128嵌套和含数组记录4096。nil结构元素既有解码语义是空结构，以两库解码结果、消费长度对照，不把它误判为输入不相等。所有计时Decode都新建独占结果，不复用目标对象、共享字段或删分配。既有presence/预算/GC/padding/Fields替换/RPC测试，40入口wire/语义检查和旧109形态525项编码wire/cap/预算/独占差分在采样前通过；后者作未改编码控制，不冒称109种Decode全覆盖。没有新增镜像生产测试。

40项×self/clone/两轮比较，共160个独立进程；同组构造后只保留所选操作、清所有结果sink再GC/预热/有界校准。GOMAXPROCS=1、GOGC=100、无限GOMEMLIMIT，6轮×4 ABBA/BAAB quartet×4块；分配每臂三次128调用观测。原sampleBlock/blockCalls不变，状态快照在计时外。全部构建、检查和profile/汇编在首个计时前完成，不穿插编译/profile到配对计时中。

唯一补丁、构造probe、profile probe、driver和[计划](plan.json)在采样前冻结。首次准备脚本辅助复制列表留下一个字符串元素，发生在任何Go构建/profile/计时之前；修复后用新目录一次采完固定矩阵，失败driver与零采样证据见[准备记录](preflight/setup-failure.json)。不重跑、删除异常或扣除控制偏差，也不恢复旧最低收益线。

## 结果与保留决定

**保留入口候选，Decode继续开放优化。** 大型纯Decode两轮分别快2.37%/2.02%，每次少369.2/314.6ns；full4096纯Decode快3.04%/2.83%，全nil4096 Decode快6.19%/7.06%、PDecode快6.40%/6.94%，对应累计方向也更快。half和嵌套长数组同样两轮改善。入口证明与借用分离是局部等价变化，不新增解析器或状态；重复收益支持保留，不因低于旧3%/2%门槛而舍弃。

这不是所有业务入口获益：大型通知Dispatch两轮为+0.63%/+0.03%（+123.3/+6.9ns），累计+103.8/+522.8ns，self控制失败；大型PDecode虽配对−0.51%/−3.36%，两类控制都失败，第一轮累计反而+492.9ns。int32纯Decode与Dispatch方向也不理想，4KiB/UTF8短路径多数变化只在几ns且方向混合。未改大型Encode/PEncode控制仍有约+15～50ns漂移，不扣除它们“校正”候选。接受这些成本及归因限制，不宣称全部组合入口或PB排名改善。

全部40项如下：负号为更快；比例是六轮配对比值中位数，ns为六轮两臂几何耗时差中位数，并非独立中位数相减。self/clone标记继续沿用测量控制要求，既不把失败项删掉，也不把通过当作无回归证明。

| 入口 | 第一轮：比例（差值） | 第二轮：比例（差值） | self / clone控制 |
| --- | ---: | ---: | --- |
| `body/addressbook/decode` | -1.86%（-18.6ns） | -2.54%（-25.3ns） | 未通过 / 未通过 |
| `body/addressbook/unpack` | -0.26%（-0.5ns） | -0.02%（-0.0ns） | 通过 / 通过 |
| `body/addressbook/pdecode` | -1.61%（-19.3ns） | -1.30%（-15.5ns） | 通过 / 通过 |
| `rpc/addressbook/dispatch` | -1.07%（-14.7ns） | -1.54%（-21.5ns） | 通过 / 通过 |
| `body/game_small/decode` | -0.02%（-0.3ns） | -1.92%（-30.7ns） | 未通过 / 通过 |
| `body/game_small/unpack` | +0.15%（+0.7ns） | -0.95%（-4.7ns） | 通过 / 通过 |
| `body/game_small/pdecode` | -1.69%（-35.9ns） | -1.76%（-37.4ns） | 通过 / 通过 |
| `rpc/game_small/dispatch` | -1.14%（-26.3ns） | -1.19%（-27.3ns） | 通过 / 通过 |
| `body/game_large/decode` | -2.37%（-369.2ns） | -2.02%（-314.6ns） | 未通过 / 通过 |
| `body/game_large/unpack` | -0.03%（-1.5ns） | +0.01%（+0.7ns） | 通过 / 通过 |
| `body/game_large/pdecode` | -0.51%（-98.5ns） | -3.36%（-700.1ns） | 未通过 / 未通过 |
| `rpc/game_large/dispatch` | +0.63%（+123.3ns） | +0.03%（+6.9ns） | 未通过 / 通过 |
| `body/int32_edges/decode` | -0.25%（-5.7ns） | +0.55%（+12.6ns） | 未通过 / 通过 |
| `body/int32_edges/unpack` | +0.09%（+4.7ns） | -0.27%（-14.9ns） | 通过 / 通过 |
| `body/int32_edges/pdecode` | -0.68%（-52.0ns） | -0.33%（-25.7ns） | 通过 / 通过 |
| `rpc/int32_edges/dispatch` | +0.05%（+3.7ns） | +0.27%（+21.2ns） | 未通过 / 未通过 |
| `body/echo_ascii4k/decode` | +0.17%（+1.0ns） | +0.54%（+3.1ns） | 通过 / 通过 |
| `body/echo_ascii4k/unpack` | +0.39%（+1.9ns） | -0.11%（-0.6ns） | 通过 / 通过 |
| `body/echo_ascii4k/pdecode` | -0.94%（-10.8ns） | -0.27%（-3.2ns） | 通过 / 通过 |
| `rpc/echo_ascii4k/dispatch` | -0.13%（-1.8ns） | -0.36%（-4.9ns） | 通过 / 通过 |
| `echo/ascii32/payload_decode` | -2.81%（-4.7ns） | +0.02%（+0.0ns） | 通过 / 通过 |
| `echo/ascii32/frame_decode` | -0.62%（-2.3ns） | -0.69%（-2.5ns） | 通过 / 未通过 |
| `echo/utf8_1k/payload_decode` | +0.51%（+1.4ns） | +0.46%（+1.2ns） | 未通过 / 通过 |
| `echo/utf8_1k/frame_decode` | -0.21%（-1.3ns） | -0.14%（-0.8ns） | 通过 / 通过 |
| `echo/ascii4k/payload_decode` | +0.42%（+2.6ns） | -0.30%（-1.9ns） | 通过 / 通过 |
| `echo/ascii4k/frame_decode` | -0.73%（-10.1ns） | -0.01%（-0.1ns） | 通过 / 通过 |
| `echo/zeros4k/payload_decode` | +0.47%（+2.8ns） | -0.03%（-0.2ns） | 通过 / 通过 |
| `echo/zeros4k/frame_decode` | -0.51%（-7.1ns） | +0.42%（+5.8ns） | 通过 / 通过 |
| `growth/flat/full/4096/decode` | -3.04%（-14179.3ns） | -2.83%（-13233.4ns） | 未通过 / 通过 |
| `growth/flat/full/4096/pdecode` | -1.67%（-10780.0ns） | -1.92%（-12305.6ns） | 未通过 / 通过 |
| `growth/flat/nil/4096/decode` | -6.19%（-13365.6ns） | -7.06%（-15407.1ns） | 通过 / 通过 |
| `growth/flat/nil/4096/pdecode` | -6.40%（-15515.2ns） | -6.94%（-16819.6ns） | 通过 / 通过 |
| `growth/flat/half/4096/decode` | -3.22%（-11449.0ns） | -4.17%（-14635.9ns） | 通过 / 通过 |
| `growth/flat/half/4096/pdecode` | -2.92%（-13285.6ns） | -3.24%（-14843.1ns） | 未通过 / 通过 |
| `growth/nested/32x128/decode` | -2.02%（-9657.2ns） | -3.14%（-15131.0ns） | 未通过 / 通过 |
| `growth/nested/32x128/pdecode` | -2.25%（-14599.9ns） | -2.90%（-18986.5ns） | 未通过 / 通过 |
| `growth/array_record/4096/decode` | -1.84%（-6766.2ns） | -0.86%（-3228.7ns） | 通过 / 未通过 |
| `growth/array_record/4096/pdecode` | -0.85%（-4207.5ns） | -0.78%（-3866.5ns） | 通过 / 通过 |
| `control/game_large/encode` | +0.27%（+14.7ns） | +0.50%（+27.2ns） | 通过 / 通过 |
| `control/game_large/pencode` | +0.72%（+50.3ns） | +0.58%（+41.0ns） | 通过 / 通过 |

## 控制、分配和异常

160进程的**15,360块、3,840 quartet、960轮、640快照**从逐块数据独立复算到1e-12一致。self27/40、clone35/40满足原控制标记，失败项与IQR/逐轮方向见[独立分析](analysis.json)。所有快照GOGC100、无限MemoryLimit、limiter为零；最大采样堆7,655,800B，约7.30MiB，这是快照，不是峰值。

160组对象数中位数全部相同；B/op与对象数同时相同的组数self34、clone39、第一轮34、第二轮36。其余多为16B/128次即0.125B/op的尾差，第二轮full/half PDecode有32B/128次差值，全部列入[分配尾差](allocation-tails.json)。两臂包含的微小GC/tiny窗口差异保留，不据此虚构新对象或强行定因，也不截断字节数使控制通过。

[累计翻转](cumulative-reversals.json)共7个入口、8个轮次：AddressBook PDecode、小型Unpack、大型PDecode、echo_ascii4k纯Decode/PDecode、手工ASCII4KiB payload和zero4KiB payload。尤其echo_ascii4k PDecode两轮配对−10.8/−3.2ns，累计却+61.6/+61.5ns；所以不能只挑百分比称其稳定提速。

[长块记录](long-blocks.json)逐模式保留最大块，轮级偏移见analysis.json：self full4096 Decode 106.436ms（22调用/2次GC）、clone echo_ascii4k Decode39.701ms（15,806调用/22次GC）、第一轮AddressBook PDecode32.527ms（8,613调用/2次GC）、第二轮UTF8 payload Decode45.427ms（36,289调用/13次GC）。最大轮偏移分别约+23.92%、−19.31%、+18.63%、−18.66%；不删除、不重试到控制通过，累计数据与配对统计同时保留。

## 验收、源码与范围

应用补丁前后核对18份Go源码，当前库等于两轮采样候选；只改decode.go和decode_pointer.go。四组prepare-only重建的源码/夹具/生成输入/工具/计划相同，二进制与160个单项raw的SHA、命令顺序、原采样函数和4份profile/12快照独立核对通过。汇编仅去行末空白归档，原文/整理后SHA在[验证清单](validation.json)。原始可执行文件、单项JSON、源码副本和CPU二进制保留在忽略的tmp目录；聚合raw、摘要、编译器、CPU文本及元数据随报告存档。

实际完成的7项验收见[命令与完整输出](acceptance.json)：

1. 独立库全测，启用固定官方C/Lua oracle。
2. 同样官方对照下的race与严格checkptr=2全测。
3. 独立库vet。
4. 10秒CodecRoundTrip fuzz，497,350次，1个新增interesting，无失败。
5. 10秒Decode坏输入fuzz，1,217次，无失败；计数从3秒起未再增长，原输出保留，不盲目延长/重跑、不称为广泛随机覆盖。
6. 临时modfile替换下的GSR Sproto/示例服务器/benchmark三包测试。
7. Go/TS Plain/MAC及官方向量、ownership/session/预算集成7/7、零跳过。

oracle来源和文件SHA见[官方来源](oracle-provenance.json)，复用已构建的固定oracle；[集成二进制构建信息](integration-build-info.txt)确认实际替换到本地候选。正式go.mod/go.sum逐字节等于GSR HEAD，仍依赖v0.4.0。本轮没有重测PB/TS性能排名、未改Pack随机fuzz、GSR全仓/race或跨平台，不把旧矩阵/旧差分次数当成本轮新增测试。

新生产变化不改变行为，复用既有完整契约，不新增镜像实现测试；保留字段owner/形状验证、真实Go类型赋值/GC写屏障、默认资源上限、错误优先级、失败原子性和每次新建独占结果。

## 复现与后续

从GSR根目录运行，输出目录须不存在；库须包含e50dfef，脚本从提交和固定patch重建，不依赖主工作树是否应用补丁：

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-decode-resume/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-decode-resume-new
python3 -B docs/benchmarks/2026-10-19-gosproto-decode-resume/analyze.py \
  tmp/gosproto-decode-resume-new
```

--prepare-only只重建归档，不计时。验收脚本accept.py记录本次环境下的独立命令，需已有固定oracle及主库候选；不要把正式依赖修改成临时replace。

下一轮优先检查大型decodeMessage/decodeStructSlice的重复字段定位和类型/构造成本，先以当前证据定位，再选一个低复杂度候选；不会通过共享结果、复用调用方目标、跳过budget/presence或复制第二套parser换速度。此次不同时改这些路径。Decode已按用户指令解冻，后续仍可优化，本轮不会自行再次冻结。当前新库实现及报告未提交，未推送、发布或升级依赖。
