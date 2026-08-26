# ADR 0006: 运行期 ID 与结构化 Reply error

## Status

Accepted

## Context

长时间运行的服务器会持续创建 Call 和 Timer。Skynet 的较小 Session 空间需要处理回绕危险区与活跃 Session 冲突；GSR 使用 Go uint64，可以用更简单的单调分配换取几乎不可耗尽的空间。

Domain Error 还必须在本地和 Cluster Call 之间保持可判定语义。只传错误文本会让业务依赖文案，直接传任意 Go error 又无法稳定编码，并可能泄漏内部错误或 panic stack。

## Decision

- ServiceID、SessionID、TimerID 和 ClusterSessionID 都使用 uint64。
- 零值保留；SessionID/ClusterSessionID 的零值表示 Send，其余零值无效。
- 每种 ID 在 owner 生命周期内从 1 单调分配，不扫描、不回绕、不复用；到达 MaxUint64 后永久返回 `ErrIDExhausted`。
- Runtime 或 Cluster 实例重启后可以从 1 重新开始，因为旧对象和连接已经全部失效。
- Reply error 使用 `StatusError`，直接包含 ErrorClass、ErrorCode 和安全 Message；wire 只编码这三个字段，不在 Core 另设重复的 ErrorStatus 类型。第一版 ErrorClass 为 Domain 或 Runtime，ErrorCode 使用稳定字符串。
- StatusError 按 Class + Code 实现 `errors.Is`；Message 不参与程序判断。
- 未分类 Handler error 对外转换为 `runtime.handler_failed`，panic 转换为 `runtime.service_panicked`；原始错误、panic value 和 stack 只保留在本地诊断。
- 第一版不提供任意 error details；需要结构化业务信息时使用 typed result。

## Consequences

- 分配器无需维护空洞、活跃 ID 扫描或 Skynet 式回绕危险区，代码和并发测试更简单。
- 理论耗尽仍有明确失败语义，不会因整数回绕意外关联旧对象。
- 同一个 Domain Error 跨 Cluster 后仍可由 typed client 通过 `errors.Is` 判断。
- ErrorCode 成为长期兼容契约，发布后不能因文案或重构随意改变。
