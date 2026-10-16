# Protobuf / Sproto 接入指南

本计划按 [ADR 0013](adr/0013-client-protocol-boundaries.md) 推进。当前工作树已实现 Protobuf 与独立 `protocol/sproto`，通过 Go/TypeScript Echo 验证。Go 编解码依赖独立维护的 `github.com/cnooljaw/gosproto v0.4.0`。新增 GSR API 尚未发布；已有 GSR 标签及原有协议保持不变。

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

当前新增长 FF 搜索：只跳过有界范围内确认全非零的完整 word，保留原合并/尾块规则，主循环直接判断源长度。4 KiB ASCII Pack/PEncode 约少 71.8%/57.4%，完整帧约少 43.1%；数值消息没有同等收益，部分小包 Pack 有约 5% 代价。新增改动未提交/发布，正式依赖仍为 v0.4.0；见 [FF 搜索验收](benchmarks/2026-10-16-gosproto-ffscan/RESULTS.md)。

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
