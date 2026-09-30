# ADR 0010: 每 Service 不可变 Dispatcher

## Status

Accepted

## Context

Runtime 需要保持一个清晰的业务入口：它只把本地 Message 投递到目标 Service，再调用一次 `Service.Handle`。如果 Runtime 维护每个 Service 的命令白名单、注册表或预检查逻辑，命令能力、路由和 Mailbox admission 就会耦合，Timer 和普通消息也容易产生两套行为。

旧 GameServiceRuntime 已经验证了一种有价值的形状：每个 Service 可以在构造时嵌入不可变的分发器，用 `CommandID -> typed function` 表减少重复的 `switch`、参数数量检查和类型断言；Runtime 不维护 per-Service 命令表，未知命令仍进入 `Handle`。旧项目的 D-021、`command_dispatcher.go` 以及对应 API/conformance 测试共同证明了这一点。

旧实现的 Reply 方式不能直接迁移。旧 Dispatcher 会调用 `CommandContext.Reply`，Runtime 还会在 Handler 返回错误时兜底 Reply，因而存在手工 Reply、Dispatcher Reply 和 Runtime Reply 的双重完成路径。Skynet 的 `CMD` 表也依赖 Lua 的动态参数、PTYPE 和显式 response，这些机制不应进入 GSR Core。

最初把标准分发器放在 `runtime` 包，但它要求 `runtime.CommandContext`，而应用 Service 的 `Handle` 使用带 Cluster/Logger 能力的 `gsr.CommandContext`。Go 的方法签名不能据此嵌入复用，业务仍要写转发适配。2026-09-30 的裁决将唯一标准分发器移至应用层，不保留两套并存的分发 API。

## Decision

- `Service.Handle(CommandContext, CommandID, ...any) (any, error)` 是 Runtime 唯一调用的业务入口。
- `gsr` 应用层提供唯一可选的 `Dispatcher`；Core `runtime` 不维护对应分发表。Dispatcher 属于单个应用 Service，不是全局 Registry；普通业务 Service 默认嵌入它。基础设施的特殊入口可以直接实现 `Handle`；业务只在真实压测证实分发瓶颈、手写版本有收益时，对个别热点 Command 手写 `Handle` 快速分支，并把其它命令交回 Dispatcher。
- `NewDispatcher(map[runtime.CommandID]any)` 在构造阶段复制命令表、校验 Handler 签名并准备参数绑定；`MustDispatcher` 仅为静态定义的命令表提供 fail-fast。运行期不提供 `Register`、`Unregister` 或其它可变注册入口，也不引入额外的 `CommandHandlers` 类型。
- `CommandID` 固定为 `uint32`，与协议命令字段同宽以保留现有命令值兼容性；它由业务/协议包维护静态常量或生成代码，Runtime 不分配。
- 标准 typed Handler 只允许：

  ```go
  func(gsr.CommandContext, A, B, ...) error
  func(gsr.CommandContext, A, B, ...) (R, error)
  ```

  Handler 不使用 Go variadic 签名；一个稳定 CommandID 的参数数量和类型在构造时固定。外部 `Send/Call(...args)` 的 variadic 只是消息装配形式。
- 构造时用反射校验签名并保存绑定信息，运行时仍使用 `reflect.Call`，不得在每条消息上重新解析签名。真实压测若证明个别 Command 受此限制，优先由所属 Service 手写该命令的快速分支并对照验证收益；其余命令仍走 Dispatcher，不要求全项目使用生成代码。
- Dispatcher 负责参数数量和类型校验，返回 `ErrInvalidCommandArguments`；非法 Handler 定义返回 `ErrInvalidCommandHandler`；命令不存在返回 `ErrUnknownCommand`。这些错误使用稳定的 ErrorClass/ErrorCode，Message 只用于安全诊断。
- Runtime 不做 per-Service Command 预检查。未知 Command、Timer Command 与其它 Message 一样进入目标 Mailbox，再由 Dispatcher 或 Service.Handle 返回 `ErrUnknownCommand`。
- Dispatcher 只返回 `(any, error)`，不得调用 `CommandContext.Reply()`。Runtime 根据 SessionID 自动完成 Reply：Send 忽略结果，Call 获取结果或结构化错误。`CommandContext` 不提供手工或异步 Reply API。
- Runtime 不从命令表推断 Service 能力；CommandID 只标识消息入口，不承担 Service 地址、协议帧格式或远端路由语义。

## Consequences

- 普通 Service 获得统一的 typed 参数绑定，底层仍只有一个 Handle/Message/Mailbox 路径。
- Runtime 不知道 Service 支持哪些命令，因而不会把命令表变成第二个路由或 admission 系统。
- 直接实现 `Handle` 仍保留为扩展口，供基础设施特殊入口和经实测确认的业务热点使用；业务热点仅手写需要的分支，普通命令仍委托 Dispatcher，它不是普通业务的并列默认写法。
- 返回值 Reply 路径只有 Runtime 一条，避免手工 Reply、重复 Reply 和异步 Reply 的状态竞争。
- 反射实现有热路径成本；保留分发基准用于观察，不因单个合成基准引入全局代码生成框架。
- `ErrUnknownCommand` 等命令错误会成为新的稳定 Core ErrorCode；发布后不能仅因文案或实现重构而改变其程序身份。

## 验收测试

- 构造时拒绝 nil、非函数、缺少 `CommandContext`、错误返回值和 variadic Handler。
- typed Handler 能收到固定类型参数，并通过返回值产生结果；error-only Handler 的成功结果为 nil。
- 参数数量或类型错误被拒绝，且不会调用业务函数。
- 未知命令返回 `ErrUnknownCommand`；Runtime 不在 Mailbox admission 前预检查命令。
- 修改构造输入 `map[runtime.CommandID]any` 不会改变已创建 Dispatcher 的行为。
- Dispatcher 不依赖或调用 `CommandContext.Reply`；Runtime 集成测试验证 Send/Call 的自动 completion。
- 每个 `Test...` 函数前提供一句中文注释，说明它保护的可观察行为。
