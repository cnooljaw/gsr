# Protobuf / Sproto 接入指南

当前Body与测量诊断（2026-10-20）：[字段Tag局部复用](benchmarks/2026-10-20-gosproto-record-tag/RESULTS.md)确认两记录函数Tag加载2→1、指令/栈不变，但净收益未确认，未采用。原37项大型Encode+6.22%/+7.18%，同源跨模块clone本身+6.66%，不能直接归因于候选。追加单模块独立二进制96worker，Encode+1.95%、PEncode−0.86%，控制/离散仍明显，全部小收益与反向保留，不扣控制差值。原178条流程＋补充102条及两侧描述替换race/checkptr通过；后续关键Go候选补单模块复核，旧数据不回填。前轮440f436已提交，库a93bb0d及原6项删除保持，新报告未提交，双主线继续。

当前Body实验（2026-10-20）：[数组局部字段引用](benchmarks/2026-10-20-gosproto-record-fields/RESULTS.md)固定37项及补充17/32字段4项两轮完成，未采用。大型Encode+9.03%/+15.75%、PEncode+6.47%/+7.34%，回退17/32字段Encode约慢10%～12%；全部正收益与累计反向保留，不设比例门槛。无新增堆分配但fieldSize栈256→432B、记录循环指令与检查增多。178+46条测量命令、原契约/109形态525差分、两侧字段容量/乱序tag/描述替换race/checkptr回归通过；库a93bb0d和原6项删除保持，前轮28c494f已提交，新报告未提交，双主线继续。

当前Packing实验（2026-10-20）：[普通Pack共享目标窗口](benchmarks/2026-10-20-gosproto-pack-shared-window/RESULTS.md)固定55项两轮完成，静态指令296→284但大型Pack+0.50%/−0.39%、5非零word+3.37%/+4.11%，不采用通用替换。2非零word−1.63%/−0.13%、数值PEncode−1.34%/−0.03%及全部小收益/66项累计反向保留；大型PEncode中位慢、累计两轮快一并披露。250条测量命令、109形态525差分与两侧41472次重叠窗口race/checkptr回归通过；原库a93bb0d及原6项删除不动。前轮诊断125bc91已提交，新实验未提交，依赖和双主线保持。

当前双主线诊断（2026-10-20）：[指针优化后成本](benchmarks/2026-10-20-gosproto-post-pointer-cost/RESULTS.md)基于已提交a93bb0d完成9CPU+9allocation及独立实际wire审计。大型Encode的整数记录预检/写出仍累计21.50%/17.00%；大型/数值数组分别386/417与321/449个word为3～7非零字节，下一Packing候选优先普通word目标视图与扫描，不误投FF。ASCII512个FF word另看扫描/分配。80条profile命令+1条审计、fresh/源码/输入/二进制/raw核对通过，无新生产改动或PB/TS排名。前轮库a93bb0d/GSR3f23e02已提交，新诊断未提交；正式依赖、用户原6项删除及两条主线保持。

当前Body实现（2026-10-20）：[整数记录数组受控指针读取](benchmarks/2026-10-20-gosproto-record-pointers/RESULTS.md)仅在当前字段证明及实际元素指针类型核对后，同步读切片指针槽，减少两阶段逐元素反射，命名指针回退。固定37项两轮，大型Encode−18.63%/−19.05%、PEncode−15.87%/−14.34%；AddressBook PEncode+6.43%/+0.43%、数值Encode+1.61%/+2.01%及31项累计反向保留。178条测量流程、基线新回归及7条全库/oracle/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过，候选和永久回归已提交独立库a93bb0d。前轮库7b05b78/GSR2c5c653已提交，正式依赖及原6项删除不动，Body与Packing继续。

当前Body实现（2026-10-20）：[原生字段分布诊断](benchmarks/2026-10-20-gosproto-inline-shapes/RESULTS.md)沿用同一延迟内容切片候选，固定8/64字段×8分布的Encode/PEncode共32项、128进程两轮。8字段混合Encode−0.57%/−0.77%、64字段半缺失PEncode−2.15%/−2.58%；64字段负数PEncode+1.40%/+0.50%及29项累计反向全部保留。决定保留小改动、接受局部代价，不宣称业务全面提速；158条测量流程及7条全库/oracle/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7验收通过，精确源码已提交独立库7b05b78。前轮19项报告已提交ffc591f，正式依赖v0.4.0及用户原6项删除不动，Body与Packing继续。

当前Body实验（2026-10-20）：[通用Encode延迟内容切片](benchmarks/2026-10-20-gosproto-encode-inline-body/RESULTS.md)完成固定原19项、76进程两轮比较。大型Encode中位−0.36%/−1.36%，首轮累计反慢214.9ns；大型PEncode+2.23%/−0.67%方向翻转。UTF8手工帧−2.33%/−1.32%、AddressBook PEncode−0.06%/−2.11%等小收益完整保留；候选待确认、不应用生产，不设收益比例门槛。102条流程命令、既有契约/夹具wire/来源与逐块复算通过；self13/19、clone15/19，17项累计反向及3项分配尾差保留。参考文档已提交7e48d4c，生产库e43df35、正式依赖及原6项删除不变；新报告未提交，双主线继续。

当前目标（2026-10-20，用户裁决）：同时推进Body Encode/Decode与Pack/Unpack，最终相对各自PB的领先幅度向TS结果靠近。已小幅领先不等于Body完成；Unpack组合缺口也不构成冻结正文的理由。Go/TS逐入口参考、边界与执行规则见[双主线目标](GOSPROTO-DUAL-TRACK.md)。

当前Body实现（2026-10-20）：[结构数组受控指针槽](benchmarks/2026-10-20-gosproto-decode-struct-slots/RESULTS.md)已保留并提交独立库 e43df35。相同对象构造下大型Decode−3.50%/−3.63%、Dispatch−5.80%/−3.04%，长结构重复改善、对象数中位不变；AddressBook PDecode+2.56%/+2.07%及全部控制/累计反向保留。新低GOGC/命名类型/元数据回退/异构指针拒绝回归、全库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过。新增unsafe仅同步借用已核对类型的新切片槽，GC屏障与反射回退保留。前轮成本诊断已提交06adf22，正式依赖v0.4.0不变，原库6项已有删除不动；Body与Packing继续。

本计划按 [ADR 0013](adr/0013-client-protocol-boundaries.md) 推进。当前工作树已实现 Protobuf 与独立 `protocol/sproto`，通过 Go/TypeScript Echo 验证。Go 编解码依赖独立维护的 `github.com/cnooljaw/gosproto v0.4.0`。新增 GSR API 尚未发布；已有 GSR 标签及原有协议保持不变。

当前优化切片（2026-10-19）：前轮word预算报告已提交GSR `8e9880f`，未推送。新[生产版阶段成本诊断](benchmarks/2026-10-19-gosproto-unpack-phase-profile/RESULTS.md)只读采af9169d的8个CPU入口：大型纯Unpack预检/恢复占完整样本30.40%/32.16%，大型PDecode中Unpack累计17.67%、正文Decode累计53.88%（嵌套项不相加）。countChunks累计4.31%，inline的未用content视图cap守卫仍在；下一候选为仅计数路径消除不使用的视图，尚未实施。55条流程命令＋1条静态objdump、24快照/1827采样权重、fresh/来源核对通过，无新A/B结论。主库仍af9169d干净，报告未提交，依赖v0.4.0、Decode继续开放。

## 边界与步骤

| 切片 | 内容 | 完成条件 | 当前状态 |
| --- | --- | --- | --- |
| 1 | Gate 安全模式命名与双协议边界 | SecurityMode 替换 Protocol，现有 Gate/Protobuf 接线测试通过，API/术语同步 | 已完成 |
| 2 | Go Sproto 与生成工具验证 | 固定可复现版本；类型与坏包测试；确认 `.sproto` 到 Go 类型/协议定义的产物，pack/unpack 和解码资源上限可落实 | 已完成 |
| 3 | 独立 Sproto Dispatcher | 构造时绑定强类型 Handler，逐请求 Sproto Context，响应关联与 Notify/Push，单连接状态隔离，错误契约及测试 | 已完成 |
| 4 | Go Server ↔ TypeScript Client Echo | 共用一份 schema，通过真实 WebSocket 验证 Request/Response、Notify/Push、session、错误、断线和已有 Protobuf 回归 | 已完成 |
| 5 | 接入指南与收尾 | 可复现生成/运行命令，支持类型与限制明确，按改动范围验证和审查 | 已完成 |

Go/TS 已复用并验证现有工具；本轮没有新增 GSR generator。Sproto 新示例复用 Gate，不复制握手、MAC 或队列实现。

## 候选实现

- [cloudwu/sproto](https://github.com/cloudwu/sproto)：官方 C/Lua 实现与 wire 规范，作为兼容性参考。
- [xjdrew/gosproto](https://github.com/xjdrew/gosproto)：Go 编解码与 Protocol/RPC；标准库没有对应实现，候选库用于避免重写编码器。
- [davyxu/gosproto](https://github.com/davyxu/gosproto)：基于 xjdrew 代码的编解码器与 sprotogen，提供扩展 `.sp` 描述及生成工具；候选实现仍以官方 `.sproto` 与 wire 为准，不采用改变官方解释的扩展来解决兼容问题。
- [lvzixun/sprotodump](https://github.com/lvzixun/sprotodump)：现有 Go 类型、协议定义及 schema 生成工具，需实际核对产物。
- [sealindx/sproto-ts](https://github.com/sealindx/sproto-ts)：候选 TypeScript 客户端，需核对整数精度、浏览器字节缓冲与坏包行为。

不能混用仓库主分支说明与已发布版本的能力。历史候选验证固定 `v0.1.0`；当前采用自己的修复版本 `v0.4.0`，正常 require，不使用本地 replace。RPC session 使用正 int32。业务字段的 integer 为有符号 64 位，TS 端超过安全整数范围的表达方式需验证，不能静默丢精度。

## 必须保护的行为

- 共用同一份 `.sproto` 作为定义来源，分别生成/加载 C2S、S2C；不加入 Protobuf 风格元数据。
- 需要响应的请求带有效 session，响应沿该协议的关联返回；Notify/Push 不登记等待响应。
- 字符串、整数、布尔、binary、数组、嵌套结构、缺失字段与零值的编码解释一致；double、定点数和 map 必须有实际兼容证据才能列为支持。
- 每连接状态独立，重复响应、未知 session 和断线清理不会影响其它连接；会话不会回绕或无限积累。
- pack 后帧大小、unpack 后载荷、集合/结构深度有界，畸形输入返回可处置错误，不触发业务 Handler、失控分配或进程崩溃。
- Response 使用自身定义表达业务失败；框架失败和未知协议明确如何让客户端结束等待，不塞入另一种协议的错误消息。
- Agent 仍显式实现 Handle/HandleClientMessage，普通 Command 和客户端 Dispatcher 分别持有具名字段，并在同一 runner 中使用。

这些是接入验收要求；已落地范围与未覆盖能力见下一节。

## 当前接入方式

1. 从当前 GSR 工作树开发，业务项目在新 GSR 版本发布后固定对应版本；不要把 v1.0.1 标签当作已包含 Sproto。
2. 业务 `.sproto` 分别定义 C2S/S2C，通过现有 sprotodump 生成 Go struct 与 `gosproto.Protocol` 表；不添加 Protobuf 元数据。
3. Agent 具名持有 `sproto.Dispatcher[gsr.CommandContext]`，构造时绑定强类型 Handler；在 `HandleClientMessage` 调用 `HandleFrame(self, frame)`。
4. `Context.Response` 保留原 session，`Context.Push` 和 `Dispatcher.Push` 使用 S2C 定义；普通业务仍经 Send/Call 路由到 Service。
5. Handler、坏包、未知协议或遗漏响应错误由 Agent 关闭连接并记录诊断；业务拒绝使用自身 Response，客户端断线清理等待。

修复库可以在独立项目直接使用：

```sh
go get github.com/cnooljaw/gosproto@v0.4.0
```

本轮已在独立 module 验证普通安装与 64 位整数数组、double 的 Pack/Unpack 往返，无 replace。GSR 新 API 的外部项目接入需等待对应 GSR 发布版本；当前本地开发以该工作树为准。

完整接线见 [Sproto 包](../protocol/sproto/README.md)，可复制运行命令见 [Echo 示例](../examples/websocket-gate/README.md#sproto-联调)。Gate 的 SecurityMode 独立选择 MAC/Plain，客户端协议由 NewAgent 选择。现有 Protobuf Envelope 字段不改名。

| 能力 | Go 修复库 | 本轮 TS Echo 客户端 |
| --- | --- | --- |
| string / binary / boolean | 支持，沿用原有类型回归；binary 是 []byte，不新增 binary 数组 | Echo 使用字符串；参考编码器的 boolean/数组已有官方向量，不扩展为所有 binary 能力结论 |
| signed integer / 数组 | 修复负值、空数组与宽度检查，保留 64 位位模式 | 只接受 JavaScript 安全整数，超范围报错 |
| 原生 double / 数组 | 新增 IEEE-754 支持，官方编码向量对照 | 不作为已验收能力 |
| 嵌套结构与数组 | 沿用原有类型回归，增加深度/元素预算 | 当前预检限于扁平 Echo schema |
| map / 自动定点数缩放 / BigInt 客户端 | 未新增或验收 | 未实现 |
| Request/Response、Notify/Push | 已实现 | Plain/MAC 真正跨语言验证 |
| 服务端请求客户端并等待 Response | 首版 Dispatcher 不提供 | 未提供 |

默认 packed 输入/输出上限 1 MiB，解包和编码 8 MiB，深度 64，字段/集合元素合计 100000。TS Echo 最多等待 32 个请求，字符串最多 64 KiB，关闭时清理会话；它是 Node.js 联调参考，不是通用浏览器生产 SDK。有限 fuzz、race 和固定向量不能证明所有官方类型/输入组合已穷尽。

编解码速度使用共用消息夹具分别测 Go/TS 的 payload 和应用帧，不将网络或业务调度混入；命令及比较口径见 [Benchmark 使用说明](../examples/websocket-gate/benchmarks/README.md)，本机大小、耗时和分配结果见 [2026-10-15 报告](benchmarks/2026-10-15/report.md)。当前性能基线只覆盖 Echo 字符串，未覆盖全部类型。测量发现并删除 TS `write_ff` 中未使用的逐字节十六进制调试字符串，保留优化前后数据；wire 不变。

随后补充 [官方 AddressBook 与游戏数值/数组基线](benchmarks/2026-10-15-representative/report.md)，Go/TS 分别测纯 Encode/Decode、Pack/Unpack 及组合路径，Sproto 固定 wire 与官方 C/Lua 对照通过。基线探针发现 TS boolean 与混合正负 int64 数组缺陷；这两项现已修复，并由 20 组独立官方 raw/packed 向量保护，覆盖布尔、整数宽度与符号、缺失字段及 packing 边界。TS 仍只接受安全整数，正式性能数组继续使用相同 int32 夹具以便比较。

五步优化已完成，结果、原始采样、profile 与撤回的回退方案见 [优化实施与验收](benchmarks/2026-10-15-optimized/RESULTS.md)。TS 改动直接位于示例参考实现；Go 优化已随 [gosproto v0.4.0](https://github.com/cnooljaw/gosproto/releases/tag/v0.4.0) 发布，GSR 正式依赖已升级，GSR 不保存源码补丁副本。报告中的性能测量使用发布前同一源码与临时 modfile；正式接入不使用 replace。资源预算、官方 wire 和解码失败原子性保持不变。

随后使用正式 v0.4.0 重跑两套完整基准，Go/TS 各 72 项、每项五轮；纯 body、含 packing、应用帧、大小与分配的四种实现对照见 [正式依赖完整复测](benchmarks/2026-10-15-v0.4.0/report.md)。

随后先优化 TS 长 FF/零块的 Pack/Unpack，再参考 Protobuf v1.36.1 完成安全 Go 路线，减少重复反射与遍历。实现范围见 [gosproto 性能方案](RFC-GOSPROTO-PERFORMANCE.md)，完整前后对照见 [本轮验收](benchmarks/2026-10-15-sproto-next/RESULTS.md)。TS 修改位于示例参考实现；Go 修改位于独立库未发布工作树，验收通过临时 modfile 接入，GSR 正式依赖仍为 v0.4.0。没有引入 unsafe、代码生成或新的公开 API。

第二轮继续优化数值 packing、TS 字段查询、Go 标量写出与原生 optional 值构造，以及 TS 实际 RPC 的中间复制。修复 TS 未知 body 字段导致后续字段错位的问题，并保护合并缓冲后的独立 body 编码预算。原有分阶段基准保持口径，pencode/attach 的融合收益另行测量，结果见 [第二轮验收](benchmarks/2026-10-15-sproto-round2/RESULTS.md)。正式 Go 依赖尚未改变。

第三轮按 [TS 性能方案](RFC-SPROTO-TS-PERFORMANCE.md) 完成 FF 扫描门槛、数值数组 DataView 写出、Pack 尾部分配和有界解包缓冲优化。原生 pdecode/dispatch 同步借用实例私有缓冲，最多保留 16 KiB；公开 unpack 和编码输出仍独占。公开解包只在 FF 占优时采用精确分配，数值与高压缩率路径保留原策略；完整 PB/SP 同进程采样、29 种解包形态及内存代价见 [第三轮验收](benchmarks/2026-10-15-sproto-round3/RESULTS.md)。未更换 Go 依赖。

第三轮已提交为 `72fc11a`。第四轮继续按四个零头展开解包计数，保持资源与所有权检查；4 KiB 零字节原生 RPC 解码耗时约减少 21%，公开 4–64 KiB 零块解包约减少 30%–45%。同时修正 Echo 基准为 PB/SP 共享同一份预构造文本，避免 V8 字符串内部表示差异混入编码对照；历史数据保留，早期 Echo 编码百分比需谨慎解读。新增短零段混合形态后，完整同进程 222 项、1110 个样本及验证结果见 [第四轮验收](benchmarks/2026-10-15-sproto-round4/RESULTS.md)。未修改 Go 或生产 Pack。

第四轮已提交为 `d9655b8`。第五轮先分析 Pack 的 mask/非零字节分布，再集中 FF 合并状态、缩短普通混合 word 路径；大型游戏和整数数组 Pack 约快 7.3%/4.9%。公开/私有解包省略视图的候选因形态回退未保留，输出与预算契约不变。完整 250 项逐操作配对采样、14 种 Pack/46 种 Unpack 形态及独立 profile 见 [第五轮验收](benchmarks/2026-10-15-sproto-round5/RESULTS.md)。本轮未修改 Go 依赖，未推送或发布。

2026-10-16，TS 性能阶段以提交 `9801808` 收尾，后续重点转向 gosproto。经验与适用边界见 [TS → Go 优化经验](SPROTO-OPTIMIZATION-LESSONS.md)。Go 下一轮从独立库已提交的 `6012b06` 做增量，先重建当前基线/profile，不将正式 v0.4.0 或 TS 数据冒充当前 Go 结果；执行入口见 [Go 性能方案](RFC-GOSPROTO-PERFORMANCE.md#7-ts-阶段收尾后的-go-增量入口)。

Go 第三轮已重建基线并筛选候选：仅保留布局记录内存收窄，大型消息 Encode/PEncode 各减少 512 B/op，未取得明显 CPU 加速。解包与原生指针赋值候选未合入；正式依赖未升级，独立库已本地提交为 `a579920`、GSR 记录为 `fdbc736`，均未推送/发布。验证、数据与复现见 [第三轮报告](benchmarks/2026-10-16-gosproto-round3/RESULTS.md)。

大型 Encode 优化随后按用户同意实施：原生 `*int64` 使用受控只读字段绑定，命名类型等保留反射；预算、wire 与所有权不变。同进程大型 Encode 14.345 → 8.456 µs，仍比同次 PB 慢约 7.5%。该切片已本地提交为 gosproto `819047e`、GSR `bf3ed8c`，见 [实施结果](benchmarks/2026-10-16-gosproto-encoder/RESULTS.md)。

后续修复父字段描述替换导致的嵌套 Go 布局不匹配，并将重复校验收敛到同构整数结构数组入口。大型 Encode 相对上一提交 8.382 → 6.528 µs，同场 PB 7.606 µs；AddressBook 有类型安全修复成本，未宣称全部消息胜出。完整独立库 race/checkptr/官方对照、有限 fuzz 及 GSR Go/TS 7/7 接线通过，该切片已本地提交为 gosproto `5e491ff`、GSR `70c84af`。见 [结果与复现](benchmarks/2026-10-16-gosproto-bindings/RESULTS.md)。

随后 Pack 切片调整混合 word 写出和 FF 状态，资源及 wire 契约不变。整数数组 Pack/PEncode 约少 10.3%/6.8% 耗时，大型消息约少 5.0%/1.3%，仍不代表全部组合胜出 PB；已本地提交为 gosproto `7dc6713`、GSR `e9e1f74`，见 [Pack 验收](benchmarks/2026-10-16-gosproto-pack/RESULTS.md)。

长 FF 搜索切片：只跳过有界范围内确认全非零的完整 word，保留原合并/尾块规则，主循环直接判断源长度。4 KiB ASCII Pack/PEncode 约少 71.8%/57.4%，完整帧约少 43.1%；数值消息没有同等收益，部分小包 Pack 有约 5% 代价。已本地提交为 gosproto `07350a9`、GSR `3a78dbd`，未发布，正式依赖仍为 v0.4.0；见 [FF 搜索验收](benchmarks/2026-10-16-gosproto-ffscan/RESULTS.md)。

当前新增连续零 mask 的有界解包，保留完整预检、错误优先级和独占输出。4 KiB 零字节帧解码约 -51.2%，同场比 PB 低约 11.5%；数值业务组合基本持平，AddressBook Unpack 约 +10%、PDecode +1.6% 的代价保留。45824 组差分、官方对照与库 race/checkptr/vet/fuzz、GSR 三包和 Go/TS 集成通过，见 [解包验收](benchmarks/2026-10-16-gosproto-zero-unpack/RESULTS.md)。已本地提交为 gosproto `22e287e`、GSR `1af93f4`，未发布，正式依赖仍为 v0.4.0。

普通 Unpack 后续验证五个位置/计数表及游标候选，各做 129 项三轮配对，全部因稀疏回退或真实组合收益不足撤回。该切片只保留 2048 组独立 mask 字节回归与 [筛选记录](benchmarks/2026-10-16-gosproto-unpack-candidates/RESULTS.md)，已提交为 gosproto `d66644a`、GSR `acfb070`（格式属性修正 `ae2e1bc`），正式依赖仍为 v0.4.0。

2026-10-17，同分配探针确认大型 Decode 仍有反射执行成本；落地已确认 inline 原生整数的安全快捷处理。大型 Decode/PDecode 约 -5.3%/-4.1%，分配、wire、预算及独占不变，未新增 unsafe。完整库/官方对照、差分、有限 fuzz 与 GSR/GoTS 集成通过，控制中的波动和代价分别保留，见 [Decode 诊断与验收](benchmarks/2026-10-17-gosproto-decode-analysis/RESULTS.md)。已本地提交为 gosproto `70d985c`、GSR `d0328fd`，未发布，正式依赖仍为 v0.4.0。

后续整数记录专用 parser 已因组合收益不足撤回。原生整数指针类型化写入在隔离副本的五轮数据中带来大型 Decode/PDecode 约 -11.2%/-9.2%，并通过官方/GC/差分及 GoTS 集成；正式生产代码仍为 `70d985c`。数据与局限见 [实验记录](benchmarks/2026-10-17-gosproto-decode-writes/RESULTS.md)，正式化范围见 [受控写入提案](RFC-GOSPROTO-DECODER-WRITES.md)。该轮已本地提交为 gosproto `5be3b26`、GSR `2872d46`，正式依赖仍为 v0.4.0。

## 独立修复库与官方对照（2026-10-15）

[cnooljaw/gosproto v0.2.0](https://github.com/cnooljaw/gosproto/releases/tag/v0.2.0) 基于 xjdrew v0.1.0，保留上游历史与 MIT 许可证，自己的 module 路径和标签避免给业务项目传播 replace。标准库没有 Sproto 实现，使用这个依赖专门解决原生编解码与 pack/unpack，不引入通用协议框架或运行时 C/Lua。

修复覆盖负整数数组、官方空数组、畸形宽度、原生 double、整数溢出、schema 缓存失败、有限资源预算、解码失败保留原值、有界 Unpack，以及 RPC 等待上限和显式清理。生成工具固定 `lvzixun/sprotodump 3a45d3e49ea2bc8e2b607e2c8060b32281d730dd`；TS 参考固定 `sealindx/sproto-ts bbee46fd76e2c9a7401ff9efd1dcadb538908130`，保留其 BSD 许可证并修正实例间 host/会话共享、整数安全范围、包边界和高压缩率输入的解包扩容。

官方参考仍为 `cloudwu/sproto e93fd5a0653b2bcab38d3b830958bf2e3a901b5f`，未修改其编解码源码。实际扩大边界验证时发现：`sproto.c` 的 `todword` 在 signed int 中计算后返回 size_t，本机 64 位 C/Lua 构建会对低 32 位最高位为 1 的部分 64 位值错误扩展，例如数组里的 MaxInt64 解为 -1。这是固定实现的缺陷，不应改变 Go wire 去复制错误。

因此完整边界向量从独立 Lua 输入由官方编码，Go 编码与其逐字节一致并正确解码；另以官方解码核对四字节负整数数组。固定十六进制向量加入修复库默认测试，无需 Lua；开启 `SPROTO_ORACLE_DIR` 则实际运行 C/Lua 对照。该官方解码限制被显式记录，没有通过只做 Go 自身往返隐藏它。

### 修复库 v0.3.0 职责收敛与 GSR 升级

用户确认 gosproto 对齐官方 Sproto 的协议职责：删除上游额外的 Service 网络 helper、阻塞 Call/异步 Go、Done channel、两字节 TCP 分帧、反射方法注册及其旧网络示例；保留结构体编解码、Pack/Unpack 和原生 type/session RPC。官方 host:dispatch/attach 处理协议消息，连接、调度、Handler 和超时由外部负责。GSR 一直使用自己的 Dispatcher，没有调用被删的 helper。

同时删除只为 Go 方法绑定服务的 Protocol.MethodName、MatchMethod 和 GetProtocolByMethod。此变更已随 [gosproto v0.3.0](https://github.com/cnooljaw/gosproto/releases/tag/v0.3.0) 发布，独立模块的正常安装、RPC 与打包边界验证通过。GSR 当时正常 require v0.3.0，go.sum 固定发布版本校验和，不使用 replace。sprotodump 包装脚本在生成时剔除 MethodName，C2S/S2C 产物已重新生成；后续重生成会保持相同格式。

### 修复库 v0.4.0 性能优化与 GSR 升级

[gosproto v0.4.0](https://github.com/cnooljaw/gosproto/releases/tag/v0.4.0) 发布提交为 `3f2491f7b2701c710fdfc9dfa7952c8e7002dd1d`，包含优化提交 `931e2a6`。本版本减少编解码临时分配，直接写入最终输出，并优化 Pack/Unpack；公开 API、官方 wire 和默认资源预算不变，协议类型无需重生成。GSR 正常 require v0.4.0，go.sum 固定发布校验和，不使用本地 replace 或临时 modfile。

以下历史记录保留候选版本的失败证据；“未引入/尚未验证”描述当时状态，不覆盖上述实施结果。

## 第一轮候选库证据（2026-10-14）

隔离 module 中以 `xjdrew/gosproto v0.1.0` 执行三条最小用例，均未达到接入要求：

- 有符号整数数组 `[-1, -2147483648, 42]` 编解码后得到 `[4294967295, 2147483648, 42]`，负值被改变。
- `double` 字段无法建立编码类型，返回 unknown wire type。
- 整数数组元素宽度为零的输入触发 integer divide by zero panic。后续官方对照更正：无元素的零宽度输入被官方接受为空数组，不能直接称为坏包；非空零宽度输入则由官方拒绝，两种输入都不应触发 Go panic。

同时，Unpack 没有配置解包后上限，递归结构解码没有深度预算。这些结论只针对已下载的 v0.1.0，不扩展为对其它提交或其它 Go Sproto 库的结论。正式引入前需比对后续提交/修复实现，并以这些可复现用例验证；不能用 recover 掩盖数值错误或当作完整资源限制。本轮未修改依赖缓存、未把候选库写入生产 go.mod。

TypeScript 实现与生成工具的首次 GitHub clone 长时间无进展，已停止该操作，尚未形成其生成产物或跨语言互通证据。

## 官方对照与 davyxu 候选验证（2026-10-14）

用户要求以官方仓库为准，并补充 davyxu 作为候选。本轮通过 GitHub API 下载并固定源码，所有编译、隔离 module 与输入用例均在临时目录，不修改生产依赖或 Envelope：

- 官方 `cloudwu/sproto`：`e93fd5a0653b2bcab38d3b830958bf2e3a901b5f`。
- `davyxu/gosproto`：`001cf85c891ced746b27a36a75a735025d7fcc79`。
- 对照 `xjdrew/gosproto`：继续使用 `v0.1.0`。

官方 C/Lua 实现已编译并实际执行；本机 Lua 5.3 的 x86_64 模块构建额外定义 `luai_unlikely(x)=(x)`，只补齐分支提示宏，不改变官方编解码代码。官方成功往返负整数数组、负整数标量和 double；空整数数组的规范编码为 `0100000000000000`；零宽度但无元素的非规范编码被接受，非空零宽度输入返回解码错误。

| 行为 / 官方结果 | xjdrew v0.1.0 | davyxu 固定提交 |
| --- | --- | --- |
| 数组 `[-1, -2147483648, 42]` 保留符号 | 变为 `[4294967295, 2147483648, 42]` | 相同失真 |
| 官方四字节 `-1` 标量解码为 `-1` | 通过 | 得到 `4294967295` |
| 官方空整数数组规范编码可解码 | 返回 Decode error | 返回 Decode error |
| 原生 `double` 可以建立类型 | 不支持 | 不支持 |
| 非空零宽度整数数组由解码器拒绝 | 除零 panic | 除零 panic |

davyxu 的五条最小 Go 用例均未通过；xjdrew 的标量用例通过，其余四条未通过。官方基准验证成功；这些失败属于候选库兼容性检查，不是 GSR 现有测试失败。规范输入的十六进制向量来自上述官方实现，后续实现须使用它们做回归。

davyxu 的 `float32/float64` 是生成器扩展：生成整数字段，用精度因子缩放，默认因子为 1000；它不是官方 double 的 IEEE-754 wire，不能当作 double 支持。源码的顶层解析分支与生成模板主要处理结构体/enum，未覆盖原生 `echo 1 { request ... response ... }` 的完整 RPC 定义生成，不能直接替代原生协议工具链；本轮没有运行其生成器。

结论：这两个候选版本均不能直接作为满足接入门禁的依赖。优先验证修复实现，并以官方 schema、编码向量和错误行为验收；若采用某库的修复分支，明确锁定提交和修复范围。不改官方协议来迁就第三方扩展，不把临时 recover 当作兼容性或资源上限的解决方案。

受控写入现按提案的最小范围封装到私有 decodePointer；新五轮大型 Decode/PDecode 约 -7.4%/-5.9%，分配不变，AddressBook 与控制代价保留。完整库/官方/GC/差分/fuzz 及 GSR 三包/GoTS 7/7 验收通过，见 [实施验收](benchmarks/2026-10-17-gosproto-decode-store/RESULTS.md)。该切片已本地提交为 gosproto `fc3246a`、GSR `862fa8b`，未推送/发布，正式依赖未升级。

后续三个 Decode 入口写法与固定八字节普通 Unpack 恢复均因收益不足/回退未采用，全部生产代码保持 `fc3246a`。基线新 profile、mask 分布、918 个筛选样本和新增 FF 后继回归见 [局部候选筛选](benchmarks/2026-10-17-gosproto-decode-binding/RESULTS.md)。该轮未重复集成验收，回归与记录已本地提交为 gosproto `a09fb00`、GSR `d1beb28`。

历史记录（2026-10-17；2026-10-19已解除冻结）：Decode 性能阶段当时按用户决定冻结于独立库 `a09fb00`（Decode 生产源码同 `fc3246a`）。最后的独立入口实验微基准约 -38.3%，完整大型 Decode/PDecode 仅 -2.25%/-1.28%，未达预设门槛，候选未采用；详见 [冻结报告](benchmarks/2026-10-17-gosproto-decode-freeze/RESULTS.md)。正式依赖仍为 v0.4.0，实验与阶段性冻结文档已提交为 GSR `0db5ed0`，未发布。

后续只调整 Pack 普通 word 的执行路径，Decode 保持当前阶段基线。五轮大型/整数数组 PEncode 约 -6.0%/-5.7%，分配不变，完整库/官方/差分/fuzz 与 GoTS 7/7 验收通过，见 [Pack 执行路径验收](benchmarks/2026-10-17-gosproto-pack-execution/RESULTS.md)。该轮已本地提交为 gosproto `faa6a21` / GSR `3626b3d`，未推送/发布，正式依赖仍为 v0.4.0。

后续连续零 word Pack 的三个候选虽改善零块，但因 ASCII、数值组合或短包代价全部未采用，正式生产代码保持 `faa6a21`。仅新增精确零段/FF/尾块回归和 1719 个筛选样本，未重复集成验收，见 [筛选记录](benchmarks/2026-10-17-gosproto-zero-pack/RESULTS.md)。该轮已本地提交为 gosproto `bba3326` / GSR `a27ec4e`，未推送/发布；Decode 阶段基线与正式依赖保持不变。

后续稀疏 word 写出候选未通过组合路径门槛，未采用；额外 A/A 发现同源码计时差值可大于候选收益，下一步先校准测量方法。750 个配对样本、四项 CPU 摘要和汇编核对见 [诊断记录](benchmarks/2026-10-17-gosproto-pack-sparse/RESULTS.md)。独立库保持 `bba3326` 且工作树干净，正式依赖与 Decode 阶段基线不变；该轮未重复接线验收，诊断已本地提交为 GSR `8f0003f`。

后续新增同函数/同源码 A/A 与 ABBA/BAAB 配对工具，9/19 项同时通过两类控制。原候选复测仍未达保留门槛，正式库保持 `bba3326`；该轮工具和记录已提交为 GSR `f064584`，未重复接线验收，见 [校准记录](benchmarks/2026-10-17-gosproto-pair-calibration/RESULTS.md)。该工具只服务局部优化，不替代完整 Go/TS 对照或发布验收。

后续仅扩展工具的筛选和 GC 观测：隔离探针解释了 16 B 的 tiny 块窗口差值，40 ms 聚焦实验未使控制整体稳定，默认与总门槛不变。原始数据与验证见 [GC 边界诊断](benchmarks/2026-10-17-gosproto-pair-resolution/RESULTS.md)，已本地提交为 GSR `3a88564`，未推送。没有比较或采用生产候选；独立库、正式依赖和 Decode 阶段基线保持不变。

再做三项同函数墙钟/进程 CPU 隔离诊断，CPU 配对结果明显比墙钟稳定，支持存在执行之外等待；具体原因仍未分离。CPU 只作交叉检查，不替代墙钟或分配验收，未采用生产候选；已本地提交为 GSR `b2b3dd7`，见 [CPU 诊断](benchmarks/2026-10-17-gosproto-pair-cpu/RESULTS.md)。

后续 FF 段集中处理候选通过既有字节/边界与 17984 组差分，但未达到预设性能及控制要求，已撤回。固定三模式共 1728 块，见 [FF 段实验](benchmarks/2026-10-17-gosproto-pack-runs/RESULTS.md)，新记录待审查。正式库、依赖与 Decode 阶段基线不变，未重复接线验收。

用户随后修正统一收益淘汰线，稀疏展开与 FF 段分离重新进行两轮独立业务复测、前后控制及固定形态检查，组合保留到 gosproto 工作树。大型/整数数组 PEncode 两轮分别约快 3.72%～4.24% / 2.83%～3.04%，零字节完整帧约快 20%；合成四非零字节形态的约 3.33% 代价单列披露。最终库官方对照、race/checkptr/vet/fuzz、临时 modfile 三包及 Go/TS Plain/MAC 7/7 通过，见 [复评记录](benchmarks/2026-10-17-gosproto-pack-reassessment/RESULTS.md)。实现与记录未提交，正式依赖仍为 v0.4.0，Decode 冻结。

上述切片已本地提交为 gosproto d1ab622 / GSR d3b3ef4，未推送。后续仅优化 EncodePacked 私有工作区，省去中间字节分配；两轮 body PEncode 改善及 32 B 手工帧约 7～9 ns 代价分别披露，不将组合入口收益当成 RPC 帧收益。最终库官方对照、普通/race/checkptr/vet/fuzz、GSR 三包与 Go/TS 7/7 通过，见 [工作区报告](benchmarks/2026-10-17-gosproto-packed-workspace/RESULTS.md)。该切片已提交为 gosproto 56f5c21 / GSR 0f0f779，未推送，正式依赖和 Decode 基线不变。


后续 RPC 编码切片只将 header 编码与拼接复用到 Pack 工作区，保留 body 校验、session 登记及锁范围。真实通知/请求加取消/响应共 19 项，两轮全部更快且少两次分配；大型消息约快 8%～10%，整数数组约快 18%～20%。默认预算、错误顺序、wire/cap/独占的增量回归及库普通/race/checkptr/官方 C-Lua/vet/fuzz、GSR 三包、Go/TS 7/7 通过；该切片已提交为 gosproto dc4948d / GSR c4622bf，未推送，见 [RPC 工作区报告](benchmarks/2026-10-17-gosproto-rpc-workspace/RESULTS.md)。这次是实际 RPC API 的测量，区别于原手工帧与 body PEncode；没有新增网络或 PB/TS 排名结论，正式依赖不变。


后续只调整 RequestEncode 的局部 session 存储：通知不再因另一分支取参数地址而多分配。两轮9种通知都少一次逻辑分配、约4 B/op；32 B/AddressBook小包有小幅CPU改善，大型通知约48～68 ns及ASCII请求约18～20 ns的代价与控制波动一并披露。库普通/官方C-Lua/race/checkptr/vet、GSR三包及Go/TS 7/7通过，见 [session逃逸报告](benchmarks/2026-10-17-gosproto-rpc-session/RESULTS.md)。该切片已提交为 gosproto 69cae73 / GSR 9ecc811，未推送；wire、session/锁语义、Decode和正式依赖不变。


后续按RPC profile仅优化库私有的type/session头：复用已有整数与tag写出、保留预算检查，用22 B调用内栈缓冲消除反射遍历和header/session堆对象。真实RPC19项两轮均更快，32 B通知约快37%/38%，大型消息约快2%～4%；通知少一次、请求/响应少两次分配。1008组API边界、39个官方C/Lua头向量、3078组固定旧新差分及库/race/checkptr/官方/vet、GSR三包、Go/TS 7/7通过。首次采样干扰、首版分配失败、仅58次有限随机fuzz和控制未全过的事实均保留，见 [RPC profile与header报告](benchmarks/2026-10-17-gosproto-rpc-profile/RESULTS.md)。该切片已提交为 gosproto f9de3d2 / GSR dbb681f，未推送；当时Decode、普通body codec、Pack/Unpack、TS及正式依赖不变。


后续仅融合声明body的ResponseEncode：以私有header字节前缀复用现有预检/写出，省一次body分配及复制，不新增公共前缀API或协议模型。五项响应两轮均改善，大型约快6%～8%、整数数组约快13%～15%、ASCII4KiB约快43%～44%，容量/独占/预算保持；请求方法与session/错误返回未改，共用nil前缀写出成本与通知约9/7 ns代价独立披露。库/官方/race/checkptr/vet、有限fuzz、3078固定RPC差分、GSR三包和Go/TS 7/7通过，见 [响应工作区报告](benchmarks/2026-10-18-gosproto-response-workspace/RESULTS.md)。该切片已本地提交为 gosproto b626637 / GSR b2825cf，未推送；正式依赖、Decode、Pack/Unpack和TS不变。


后续仅融合有Request、无Response的通知，复用上一轮私有前缀写出，省一次body分配和复制；有响应请求的校验、锁、session登记和重复错误raw body返回保持。9项通知两轮均改善：大型约快6.5%～7.3%、整数数组约快11.7%～13.5%、ASCII4KiB约快43%～45%；10项非目标分配不变，整数数组请求约14～20 ns代价与self13/19、clone7/19控制失败完整披露。库/官方/race/checkptr/vet、3078固定RPC差分、GSR三包和Go/TS 7/7、零跳过通过，源码/归档一致，见 [通知工作区报告](benchmarks/2026-10-18-gosproto-notification-workspace/RESULTS.md)。该切片已本地提交为 gosproto 0988af0 / GSR 9876a5b，未推送；正式依赖/Decode/Pack/Unpack/TS不变，未重新测PB/TS排名。


后续融合声明Request且有Response的请求：锁外完成body预检和写出，原session接纳后Pack，重复session仍返回限长限容raw body。五项请求两轮均少一次分配，大型约快5.8%～6.2%、整数数组约快9.5%～11.6%、ASCII4KiB约快41%～42%。共用writer成本（ASCII纯Encode约+21 ns）、失败workspace分配（32 B invalid/full多16 B，MaxInt32重复缓冲级别多32 B）及self17/19、clone15/19控制失败全部披露。扩展504组重复body/cap/独占回归，库/官方/race/checkptr/vet、固定RPC差分、有限fuzz、GSR三包和Go/TS 7/7通过，源码/归档一致，见 [请求工作区报告](benchmarks/2026-10-18-gosproto-request-workspace/RESULTS.md)。该切片已本地提交为 gosproto 6d854d1 / GSR f715e6b，未推送；正式依赖与Decode/Pack/Unpack/TS不变；后续先重采profile再决定热点，不新增PB/TS排名。


有响应请求切片已本地提交为 gosproto 6d854d1 / GSR f715e6b，未推送。随后只刷新四种既有消息的body EncodePacked与真实RequestEncode+CancelSession八条profile，CPU与逐次内存采样分开；大型消息集中在整数记录预检/写出，整数数组集中在Pack和连续写出，大型仍有512 B布局外溢。4 KiB ASCII的kevent/GC归因明显，完整保留而不据此排名。独立harness夹具/统计/选择/vet、16次采样及归档/指纹核对通过；库源码干净，未重复生产验收，见 [融合后热点报告](benchmarks/2026-10-18-gosproto-post-fusion-profile/RESULTS.md)。下一轮先单独实验整数数组写出，预算/wire/cap/独占不变；该诊断已本地提交为GSR d2be5d8，库保持6d854d1，未推送；正式依赖与Decode继续冻结。


后续只调整原生[]int64的4/8字节输出游标，保留预检/宽度/预算/wire/cap/独占。编译器可消除每元素重复切片检查；43项两轮中，32/64位纯Encode约快25%/23%，PEncode约快14%/8%，大型约快3%，实际RPC也改善。对象数中位数不变，两个手工帧±0.125 B/op尾差、小包约5～8 ns及未改纯Pack第二轮+9.45%（245 ns）均披露；Pack各臂两轮符号指令一致，原因仍未确定。库/官方/race/checkptr/vet、474047次有限Codec fuzz、GSR三包和Go/TS 7/7通过；40组精确wire及故意错步长负向回归保护游标。见 [连续游标报告](benchmarks/2026-10-18-gosproto-array-cursor/RESULTS.md)，该切片已本地提交为gosproto 0c1f6cf / GSR 22eb772，未推送；正式依赖与Decode不变，不新增PB/TS排名。


后续完成Fields局部复用实验：两个记录helper各捕获一次切片头，编译器确实把两次每字段描述加载移出循环。固定43项两轮中，大型纯Encode约快0.75%/1.17%，与控制偏差同阶；PEncode持平、RPC方向混合，未确认业务组合收益，生产候选撤回。保留乱序Go声明/忽略字段的预算tag回归；错误使用声明顺序时，新夹具失败、旧夹具仍通过，覆盖增量明确。最终生产代码与0c1f6cf一致，相关基线/候选测试、固定采样及归档核对通过，未重复生产全验收，见 [Fields实验](benchmarks/2026-10-18-gosproto-record-fields/RESULTS.md)。测试与记录已本地提交为gosproto bd1976a / GSR 4bf1b32，未推送；下一步单独检查记录入口反射值/指针借用成本，不扩大unsafe范围，Decode和正式依赖不变。


整数记录指针入口改用已验证数组分支借用现有encodePointer，helper避免三字反射值与重复nil类型分支，生产净减3行，不扩大unsafe或改变预算/wire/所有权。固定43项两轮中，大型Encode约快7.9%～8.3%、PEncode快5.6%～7.3%、三种RPC快5.2%～7.2%，小型也改善，目标分配中位数相同。补充8项nil矩阵确认全nil256记录约慢193～217ns（4%～5%），混合数组更快；明确接受该局部代价，保留候选。主self35/43、clone34/43及全部失败/累计方向相反/离群块保留，第二轮未改int32请求+4.74%未解释；旧Pack+9.45%仍不覆盖。库/官方/race/checkptr/vet、441809次有限Codec fuzz、GSR三包及Go/TS7/7通过。见 [指针入口报告](benchmarks/2026-10-18-gosproto-record-pointer/RESULTS.md)，该切片已本地提交为gosproto a3b3830 / GSR f37fef0，未推送；Decode和正式依赖继续冻结。


仅在两个整数记录数组调用点用IsNil跳过nil指针提取，helper仍处理布局/预算，固定43项及8项nil补充、各两轮。全nil256记录约快322～426ns，但大型Encode回退8.85%～9.09%（519～538ns）、PEncode4.15%～4.84%、RPC3.46%～5.46%，混合数组也回退，故撤回候选，不以最低百分比裁决。未进入该分支的int32纯Encode也回退约13%；内联4字节核心15条指令相同、具体原因未定，完整保留。主控制35/43、37/43与nil5/8、6/8、累计方向不同和字节尾差均披露。相关基线/候选测试、独立复算/编译/归档核对通过；最终所有Go源码与a3b3830相同、库工作树干净，没有重复未变生产的全验收。见 [nil分支报告](benchmarks/2026-10-18-gosproto-record-nil-branch/RESULTS.md)，该实验记录已本地提交为GSR a391e42，库保持a3b3830，未推送；Decode及正式依赖不变。


从已提交a3b3830归档刷新四种输入的纯Encode/EncodePacked/RequestEncode+CancelSession，CPU各2秒、逐次分配另起进程各20000次，共24份profile；不作为A/B或PB/TS排名。整数数组Pack约34%/39%，大型记录预检/写出与Pack仍各自可见；父子累计不相加。大型三个入口3968B/2次，其中512B布局外溢，其余输入一次结果分配。4KiB ASCII及整数数组纯Encode的kevent/GC栈占比保留，不归因协议或据此排名。harness检查/vet/构建、选择/指纹及归档复现通过，库保持a3b3830干净，未重复生产全验收。见 [优化后热点报告](benchmarks/2026-10-18-gosproto-post-pointer-profile/RESULTS.md)，该诊断已本地提交为GSR 17996e0，未推送；下一轮先检查普通word Pack游标/BCE，再做单一候选，Decode和正式依赖不变。


只改packInto目标游标：推进剩余dst并从原packed起点返回，保留cap及FF/尾部/重叠规则。BCE总检查点未减少，ARM64汇编301→309行、栈帧112B不变；既有相关回归及17984组wire/cap/独占差分通过。固定原43项+原19形态，23808块独立复算一致。大型纯Pack+1.33%/+1.68%，大型和整数PEncode方向翻转，四/六非零长word约慢0.1μs；部分RPC/零场景改善与控制限制完整保留，综合撤回候选，不设最小收益比例。self49/62、clone38/62、累计方向不同/离群/字节尾差保留；最终全部Go源码与a3b3830相同、库干净，未重复未变生产的全验收。见 [Pack游标实验](benchmarks/2026-10-18-gosproto-pack-cursor/RESULTS.md)，该实验记录已本地提交为GSR 13d4609，库保持a3b3830，未推送；Decode及正式依赖不变。


核对历史前缀源码/二进制后，仅将当前Pack密集word串行index改为字节前缀和，写入位置/顺序、FF/稀疏/尾部、offset及重叠/cap/独占不变。ARM64汇编301→297行，栈帧112B、检查调用点7/6不变，生产净减6行。固定原62项两轮：大型Pack快8.6%～9.5%、PEncode1.9%～4.6%、三种RPC2.7%～5.0%；整数数组Pack快7.2%～9.1%、PEncode4.9%～5.8%、RPC4.6%～6.3%，保留候选。小型稀疏约1～4ns代价、未改路径+21ns、累计方向不同、self52/62/clone46/62失败及0.125B尾差均保留。相关回归/17984差分、库/官方/race/checkptr/vet、有限Pack3995与Codec478204次fuzz、GSR三包/GoTS7/7通过；Pack计数停增的覆盖限制披露。见 [主循环前缀报告](benchmarks/2026-10-18-gosproto-pack-prefix-main/RESULTS.md)，已本地提交为库a161158、GSR d6c672c，未推送，正式依赖和Decode不变。


Pack前缀已提交为库a161158、GSR d6c672c，均未推送。后续只读布局诊断覆盖109形态/317入口，game_large的99项外溢512B一次容纳，初始64会增至768B/2次；长数组累计增长才是可实验项。48种项数的分配模型保留倍增的中间长度/最终容量代价，不推断CPU收益，也不按数组长度提前预分配。诊断/模型/vet、既有fixture、独立复算和fresh归档核对通过，库保持a161158，生产/Decode/正式依赖不变；该诊断已本地提交为GSR c201237，未推送。见 [布局增长诊断](benchmarks/2026-10-18-gosproto-layout-audit/RESULTS.md)。


仅在布局外溢满载时倍增，固定原43项+27个长数组入口。4096个含整数数组记录的真实请求快8.54%/8.89%，分配230784B/12次→155136B/8次；full/nil/half长数组同向改善。中间嵌套多3456B、常规追加成本、预检帧各增16B及非目标大幅翻转均保留，按综合收益保留候选，不设最低比例。既有扩展/525差分/官方/race/checkptr/vet/有限fuzz/GSR/GoTS验收通过，Pack fuzz计数停增限制如实披露。见 [布局按需增长报告](benchmarks/2026-10-18-gosproto-layout-growth/RESULTS.md)，该轮已本地提交为库bdfe1d0、GSR bf828b0，未推送，Decode与正式依赖不变。


布局追加仅复用连续索引写入，保持容量与预算；固定原70项两轮，大型纯Encode/PEncode和三种RPC同向改善，实际请求快3.13%/3.48%，所有分配中位数相同。整数记录预检帧144→112B，通用帧不变；int32/手工帧成本、未改纯Pack波动及全部失败控制/累计原值保留。既有回归/525差分、官方/race/checkptr/vet/有限fuzz/GSR/GoTS7/7通过；Pack9053次且3s后停增的限制披露。见 [连续索引报告](benchmarks/2026-10-18-gosproto-layout-index/RESULTS.md)，该轮已本地提交为库7397804、GSR 25b0b0d，未推送，不改Decode或正式依赖。


7397804后只读刷新26份profile及4KiB同函数上下文，发现混合夹具环境会大幅改变堆/GC；另起无计时诊断确认GC CPU limiter在full启用（0→1279）、isolated未启用（0）。这是已测到的机制，不回填历史比较或把隔离数字当协议加速。整数数组组合Pack仍占37%～41%，24项形态统计支持后续普通word研究；先补单项独立进程采样。诊断/vet、384块/24形态/194事件点和源/归档/profile核对通过，库干净，生产/正式依赖/Decode不变；该轮已本地提交为 GSR `a125ab4`，未推送。见 [当前热点与GC上下文](benchmarks/2026-10-18-gosproto-post-layout-profile/RESULTS.md)。


原70项逐项独立进程控制已完成：self/clone 各61/70通过，分配中位数全部相同；140个进程 limiter 观测均为零，最大堆快照约4.20MB。13,440块与560快照独立复算、harness测试/vet及fresh指纹通过；仍有17个不同入口至少一组失败，离群全保留，不回填混合历史或宣称codec加速。见[逐项进程报告](benchmarks/2026-10-19-gosproto-isolated-cases/RESULTS.md)。该轮已本地提交为 GSR `4ff411f`，未推送，生产源码/正式依赖/Decode不变；下一轮只实验普通word连续非零段批量写出。


连续非零段批量写出已做固定原70项/四组280进程实验，目标整数数组Pack与大型Pack两轮回退，主要RPC也变慢；增加32条Pack符号指令、帧不变。候选仅留归档，未应用到库；小收益、所有控制失败/离群/累计原值保留，不按最低比例裁决。17,984差分、既有Pack/工作区契约、harness/vet、26,880块独立复算与fresh指纹通过。见[连续段实验](benchmarks/2026-10-19-gosproto-pack-contiguous/RESULTS.md)。self/clone13/70、42/70且limiter全零，测量仍有不确定性；库干净，Decode/正式依赖不变；该轮已本地提交为 GSR `d1dceaa`，未推送。


普通Pack固定payload视图已保留：不增分支，写入地址/顺序不变，符号指令296→288、帧112B不变。原70项四组280进程两轮，大型Pack快约8.9%/9.1%，整数数组PEncode快4.7%/4.2%，主要RPC改善；低于2%的小收益照常计入。控制各56/70、所有累计翻转/异常块/尾差及wide64小成本保留。既有契约/17,984差分、独立复算/fresh指纹、库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7/7通过，Pack计数停增和oracle来源限制明确记录。见[固定payload报告](benchmarks/2026-10-19-gosproto-pack-payload/RESULTS.md)。该轮已本地提交为库 `0b38019`、GSR `31b3f99`，未推送，不变更正式依赖或Decode。


三索引固定输出视图已保留到工作树：临时len/cap为3/9、公开cap不变，无新分支/分配/unsafe。原70项两轮整数数组PEncode/真实通知/请求同向改善，低比例收益、ASCII请求和大记录成本、累计翻转与全部失败/尾差保留。两个未达预检目标的草稿不计时。既有契约/17,984差分、26,880块复算/fresh指纹、库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7/7通过，Pack计数停增限制如实记录。见[固定len/cap报告](benchmarks/2026-10-19-gosproto-pack-window/RESULTS.md)。该轮已本地提交为库 `d2f6027`、GSR `175028b`，未推送，Decode/正式依赖不变。


后续只读[当前编码热点诊断](benchmarks/2026-10-19-gosproto-current-profile/RESULTS.md)已完成16入口/32份CPU与分配profile，原70项构造后逐项保活、默认GC、96快照limiter均零。大型记录预检/写出、整数Pack仍有可见成本；4KiB runtime.kevent占比与未确认成因完整披露，不产生新PB/TS排名。harness/vet、独立选择/次数/源/profile/fresh指纹通过，生产未改/库干净，不重复生产验收；该轮已本地提交为GSR `7c91913`，未推送。


后续[记录头部视图](benchmarks/2026-10-19-gosproto-record-header/RESULTS.md)只替换两处固定len/cap2切片，216→212条指令、帧/检查调用点不变。原70项两轮4096条full请求少5.42/2.68µs，half/嵌套及小收益保留，含数组记录请求成本与27项累计翻转不删。self63/70、clone62/70，目标分配不变；既有契约/525差分、26,880块复算/fresh、库/官方/race/checkptr/vet/有限Codec fuzz/GSR三包/GoTS7/7通过。该轮已本地提交为库 `9067933`、GSR `ca02105`，未推送，正式依赖/Decode不变。


后续[inline内容视图](benchmarks/2026-10-19-gosproto-record-inline/RESULTS.md)只把记录写出的内容切片延到非inline分支；扩展既有数组测试10个边界并用上限+1负向归档证明新覆盖。原70项两轮大型真实请求快3.18%/2.44%、响应快2.29%/2.44%，混合长记录/未改整数数组成本与28项累计翻转全留。self60/70、clone64/70、目标分配不变；既有契约/525差分/复算/fresh、库/官方/race/checkptr/vet/有限Codec fuzz/GSR/GoTS7/7通过。该轮已本地提交为库 `2ea804e`、GSR `b004da7`，未推送，Decode/正式依赖不变。


后续[非inline内容视图复用](benchmarks/2026-10-19-gosproto-record-body-view/RESULTS.md)只复用本次内容切片，216条/32B帧不变、panic调用点15→14；inline新增两条清零指令的代价一并测量。原70项两轮大型PEncode快0.51%/0.77%、half4096请求少1.39/0.98µs，分配不变；full4096请求与含数组记录成本、27项累计翻转全留，不宣称全面解决。self50/70、clone56/70，既有契约/525差分/复算/fresh、库/官方/race/checkptr/vet/有限Codec fuzz/GSR/GoTS7/7通过。该轮已本地提交为库 `e50dfef`、GSR `29c954b`，未推送，正式依赖不变。


用户于2026-10-19明确解除Decode阶段性冻结，入口、解析和构造/写入可继续按证据优化。当前[入口复核](benchmarks/2026-10-19-gosproto-decode-resume/RESULTS.md)保留即时类型证明与地址借用分离：大型Decode两轮快2.37%/2.02%、nil4096 Decode快6.19%/7.06%，大型通知Dispatch小成本/PDecode控制失败及累计翻转完整保留。新40项160进程/15,360块独立复算、fresh与来源核对，库全测/官方/race/checkptr/vet、有限Codec/Decode fuzz497,350/1,217次、GSR三包及GoTS7/7零跳过通过；Decode fuzz后段计数停止保留原输出，未重复PB/TS排名。该轮已本地提交为库 `913c465`、GSR `b708108`，未推送，正式依赖仍v0.4.0，Decode继续开放。


后续[结构数组schema局部复用](benchmarks/2026-10-19-gosproto-decode-array-schema/RESULTS.md)只保存同步调用内子schema链接，208条指令不变、栈帧224→240B。固定40项两轮中，含数组记录Decode快0.92%/1.05%，但大型Decode+1.25%/+0.79%、nil长数组Decode+1.59%/+1.81%，综合取舍未采用，不设最低收益比例。160进程/15,360块复算，self27/40、clone30/40；对象数不变，字节尾差/累计翻转/全部离群保留。目标契约/harness/vet/差分/fresh与指纹通过；主库从未应用补丁，源码等于913c465、工作树干净，未重复生产完整验收。该轮报告已本地提交为GSR `26a5748`，未推送；库保持913c465，依赖v0.4.0、Decode继续开放。


后续[原生整数资格前移](benchmarks/2026-10-19-gosproto-decode-integer-proof/RESULTS.md)保留全部guard，在独占分配前计算写入资格，返回同一对象用于原生或反射回退。helper仍内联、parser664条/320B不变，重复标记加载2→1；固定40项两轮大型Decode+1.76%/+1.51%，含数组记录Decode+2.31%/+1.22%，主要目标成本超过当前局部收益，未采用，不设最低收益比例。160进程/15,360块复算，self28/40、clone25/40，对象数不变，全部尾差/翻转/异常保留；目标测试/harness/vet/差分/fresh和源码/二进制核对通过。主库未应用候选、保持913c465且干净，未重复生产完整验收。该轮报告已本地提交为GSR `f7381dd`，未推送；库保持913c465，依赖v0.4.0、Decode继续开放。


后续[固定header视图](benchmarks/2026-10-19-gosproto-decode-header-view/RESULTS.md)只收窄readUint16的len/cap为2，原len/预算/错误/独占/全部字段guard不变。parser664→660条、320B不变；计时前两索引初稿证据另存，未计时。原40项两轮half4096 PDecode快2.41%/2.54%且双控制通过，大型PDecode快1.72%/1.89%；保留单行收益并接受短Echo/含数组记录等成本，self21/40、clone13/40及全部翻转/离群不删。160进程/15,360块复算、fresh/全部指纹、库全测/官方/race/checkptr/vet、有限Codec/Decode fuzz432,127/38,718次、GSR三包/GoTS7/7零跳过通过，Decode fuzz后段计数停止如实保留。该轮已本地提交为库 `503a725`、GSR `2104eca`，未推送，依赖v0.4.0、Decode继续开放。


后续[header字节偏移](benchmarks/2026-10-19-gosproto-decode-header-offset/RESULTS.md)用初始headerEnd与步进2替代index倍增，原字节/次数/预算/错误/独占/写屏障不变，parser660→656条/320B。40项两轮大型PDecode快0.81%/1.03%，但双控制失败且有累计翻转；整数数组PDecode+1.10%/+1.24%双控制通过、含数组记录PDecode+1.68%/+1.57%，整体取舍未采用，无最低收益比例。160进程/15,360块复算，self20/40、clone19/40，尾差/全部异常保留；目标测试/harness/vet/差分/fresh和指纹通过。主库从未应用候选，18份Go源码等于503a725且干净，未重复生产完整验收。该轮报告已本地提交为GSR `9699583`，未推送；库保持503a725，依赖v0.4.0、Decode继续开放。


后续[当前Decode诊断](benchmarks/2026-10-19-gosproto-decode-current-profile/RESULTS.md)在503a725只读刷新四入口CPU/独立allocation，8进程/24快照，limiter零。大型Decode/PDecode为10416B/532、13872B/533次；整数数组为8288B/4、12384B/5次。CPU/分配范围分离，初始化/kevent/递归累计限制保留，耗时不作新排名。源码与机器码确认native数组逐元素开放尾切片仍有调整，下一候选为固定4/8字节视图并补8字节大数组观察，本轮未实施。harness/vet/40顺序/构建/profile文本/fresh及全部指纹通过，主库18份Go源码等于503a725且干净，未重复生产完整验收。该诊断已本地提交为GSR `16d1491`，未推送；依赖v0.4.0、Decode继续开放。
