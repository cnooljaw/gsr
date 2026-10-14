# Protobuf / Sproto 接入计划

本计划按 [ADR 0013](adr/0013-client-protocol-boundaries.md) 推进。当前已实现 Protobuf；Sproto 尚未进入生产依赖或公开 API。所有发布标签及其原有协议保持不变。

## 边界与步骤

| 切片 | 内容 | 完成条件 | 当前状态 |
| --- | --- | --- | --- |
| 1 | Gate 安全模式命名与双协议边界 | SecurityMode 替换 Protocol，现有 Gate/Protobuf 接线测试通过，API/术语同步 | 已完成 |
| 2 | Go Sproto 与生成工具验证 | 固定可复现版本；类型与坏包测试；确认 `.sproto` 到 Go 类型/协议定义的产物，pack/unpack 和解码资源上限可落实 | 验证中 |
| 3 | 独立 Sproto Dispatcher | 构造时绑定强类型 Handler，逐请求 Sproto Context，响应关联与 Notify/Push，单连接状态隔离，错误契约及测试 | 待实施 |
| 4 | Go Server ↔ TypeScript Client Echo | 共用一份 schema，通过真实 WebSocket 验证 Request/Response、Notify/Push、session、错误、断线和已有 Protobuf 回归 | 待实施 |
| 5 | 接入指南与收尾 | 可复现生成/运行命令，支持类型与限制明确，按改动范围验证和审查 | 待实施 |

Go/TS 先验证已有工具；只有现有生成产物确实无法满足接线时再考虑 GSR generator。Sproto 新示例复用 Gate，不复制握手、MAC 或队列实现。

## 候选实现

- [cloudwu/sproto](https://github.com/cloudwu/sproto)：官方 C/Lua 实现与 wire 规范，作为兼容性参考。
- [xjdrew/gosproto](https://github.com/xjdrew/gosproto)：Go 编解码与 Protocol/RPC；标准库没有对应实现，候选库用于避免重写编码器。
- [lvzixun/sprotodump](https://github.com/lvzixun/sprotodump)：现有 Go 类型、协议定义及 schema 生成工具，需实际核对产物。
- [sealindx/sproto-ts](https://github.com/sealindx/sproto-ts)：候选 TypeScript 客户端，需核对整数精度、浏览器字节缓冲与坏包行为。

不能混用仓库主分支说明与已发布版本的能力。Go 候选库初步固定为 `v0.1.0` 做隔离验证，尚未写入本项目 go.mod；此版本的 RPC session 使用 int32。业务字段的 integer 为有符号 64 位，TS 端超过安全整数范围的表达方式需验证，不能静默丢精度。

## 必须保护的行为

- 共用同一份 `.sproto` 作为定义来源，分别生成/加载 C2S、S2C；不加入 Protobuf 风格元数据。
- 需要响应的请求带有效 session，响应沿该协议的关联返回；Notify/Push 不登记等待响应。
- 字符串、整数、布尔、binary、数组、嵌套结构、缺失字段与零值的编码解释一致；double、定点数和 map 必须有实际兼容证据才能列为支持。
- 每连接状态独立，重复响应、未知 session 和断线清理不会影响其它连接；会话不会回绕或无限积累。
- pack 后帧大小、unpack 后载荷、集合/结构深度有界，畸形输入返回可处置错误，不触发业务 Handler、失控分配或进程崩溃。
- Response 使用自身定义表达业务失败；框架失败和未知协议明确如何让客户端结束等待，不塞入另一种协议的错误消息。
- Agent 仍显式实现 Handle/HandleClientMessage，普通 Command 和客户端 Dispatcher 分别持有具名字段，并在同一 runner 中使用。

这些是接入验收要求，不表示候选库已经全部满足。

## 第一轮候选库证据（2026-10-14）

隔离 module 中以 `xjdrew/gosproto v0.1.0` 执行三条最小用例，均未达到接入要求：

- 有符号整数数组 `[-1, -2147483648, 42]` 编解码后得到 `[4294967295, 2147483648, 42]`，负值被改变。
- `double` 字段无法建立编码类型，返回 unknown wire type。
- 整数数组元素宽度为零的畸形字节触发 integer divide by zero panic。

同时，Unpack 没有配置解包后上限，递归结构解码没有深度预算。这些结论只针对已下载的 v0.1.0，不扩展为对其它提交或其它 Go Sproto 库的结论。正式引入前需比对后续提交/修复实现，并以这些可复现用例验证；不能用 recover 掩盖数值错误或当作完整资源限制。本轮未修改依赖缓存、未把候选库写入生产 go.mod。

TypeScript 实现与生成工具的首次 GitHub clone 长时间无进展，已停止该操作，尚未形成其生成产物或跨语言互通证据。
