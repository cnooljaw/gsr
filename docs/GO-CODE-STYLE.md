# GSR Go 编码规范

本规范补充 Go 官方约定，目标是让代码简单、直白、统一、可读。它不追求代码越短越好，而追求熟悉业务但不是原作者的开发者能够快速理解行为、边界和失败方式。

## 1. 规则层级

规范分为两类：

- **强制规则**：可以在评审或工具中明确判断，新增和修改代码必须遵守。
- **评审启发**：用于发现复杂度，不以固定行数或层数机械裁决。

架构与公开 API 以 `API-DESIGN.md` 为准。本规范不得被用来绕过 Service 状态所有权、Mailbox 或生命周期边界。

## 2. 强制规则

### 2.1 格式与布局

- 所有 Go 代码必须经过 `gofmt`。
- 具名函数和方法使用正常多行函数体，不把赋值、清理和返回压缩在一行。
- 两个函数或方法之间保留一个空行。
- 长签名、调用和复合字面量按语义换行；约 120 字符可作为软提示，不是硬门禁。
- 每行表达一个主要动作，不使用分号压缩控制流程。

```go
func (service *PlayerService) Close() error {
	service.context = nil
	service.phase = phaseClosed
	return nil
}
```

### 2.2 命名

- 名称表达领域含义和所有权，避免 `data`、`obj`、`tmp`、`ret` 等空泛名称。
- 缩写按 Go 习惯统一为 `ID`、`URL`、`HTTP`、`RPC`、`GSR`。
- 方法接收者必须命名；同一类型保持一致，不使用 `this` 或 `self`。
- 使用中的参数必须表达用途；为满足接口而未使用的参数显式命名为 `_`。
- `gsr.ServiceContext` 参数统一命名为 `self`，表示当前 Service 的身份与能力，不使用 `scoped` 或 `ctx`；普通 `context.Context` 仍命名为 `ctx`。未使用的 ServiceContext 参数保持 `_`。此规则针对上下文参数，不把业务 Service 的方法接收者改名为 self。
- 沿用项目稳定术语，不为相同概念创造近义词。

### 2.3 控制流

- 错误、无效输入和终止分支优先提前返回，让主路径保持左对齐。
- 已经 `return`、`continue` 或 `break` 的分支后不写多余 `else`。
- 允许只完成一个动作的 `if` initializer：

```go
if err := config.Validate(); err != nil {
	return err
}
```

- 当初始化值还参与后续业务判断，或条件混合错误、nil 和业务状态时，拆成具名局部变量。

```go
player, err := repository.LoadPlayer(ctx, playerID)
if err != nil {
	return err
}
if player.Disabled {
	return ErrPlayerDisabled
}
```

### 2.4 Error

- 错误必须处理，不使用 panic 表达预期的业务失败或外部失败。
- 使用 `%w` 保留错误链；包装必须增加操作、关键身份或边界信息。
- 不在多层重复添加 `failed`、`service failed` 等没有定位价值的文字。
- `errors.Is`、`errors.As` 和稳定的 sentinel/type error 用于程序判断，不依赖错误字符串分支。
- 多阶段清理需要同时保留多个错误时使用明确的聚合策略，例如 `errors.Join`。
- 日志通常只在能够决定处置方式的边界记录；不要每层既记录又返回同一个错误。

```go
player, err := repository.LoadPlayer(ctx, playerID)
if err != nil {
	return fmt.Errorf("load player %d: %w", playerID, err)
}
```

### 2.5 Context

- `context.Context` 只承载 deadline、cancellation 和请求范围 metadata，不作为依赖容器。
- `context.Context` 通常作为第一个参数，不存入长期对象；只有明确生命周期 owner 可以持有用于关闭的根 context/cancel。
- `ServiceContext` 是 Runtime 授予当前 Service 的能力集合，不等同于 `context.Context`。
- 不在 Handler 外或非 Runtime 管理的 goroutine 中保存、复用 `ServiceContext` 或 `CommandContext`。
- 数据库、配置、日志器等长期依赖通过明确字段或窄接口注入。

### 2.6 Goroutine 与 Channel

- Service 和业务层禁止直接使用 `go`，异步状态推进使用 Command、Timer 或独立 Service。
- 不提供公共 `Runtime.Go`、`Fork` 或相似通用入口。
- 允许启动 goroutine 的基础设施必须有明确 owner，并负责取消、等待、并发上限、panic 和错误归属。
- 新增并发 owner 必须覆盖正常关闭、取消、超时和泄漏测试。
- Channel 只用于真实的 goroutine 通信、队列或事件循环，不作为默认解耦工具。
- 能用普通调用、mutex 或 atomic 清楚表达的同步，不强行改成 channel。

### 2.7 接口、Struct 与构造

- 接口由使用者定义，并保持为完成该用途所需的最小能力集合。
- 只有多实现、测试隔离、模块边界、插件化或明确依赖倒置需求时才引入接口。
- Struct 显式声明依赖，谨慎使用让方法来源不清楚的 embedding。
- 配置项较多时默认使用 Config Struct；Functional Option 只在可选项多、组合频繁且确实更清楚时使用。
- 构造函数完成默认值填充和不变量建立；可能失败的构造返回 error。
- `MustXxx` 只用于静态、由程序员定义且失败代表编程错误的构造，名称必须明确 panic 语义。

### 2.8 配置

- 配置使用明确 Struct，并在进入业务代码前完成默认值填充和完整校验。
- 文件、环境变量和命令行只在配置加载边界读取。
- 业务模块只接收最终配置对象，不在运行路径中重复解析配置源。
- 稳定的业务规则和内部实现参数保留在代码中，不为了“灵活”过度配置化。
- 敏感信息不提交到仓库，也不写进普通配置文件。

### 2.9 注释和文档

- 注释解释契约、原因、并发保证和非显而易见的限制，不复述代码。
- 导出符号提供 Go doc；文档重点说明调用者需要知道的行为和失败语义。
- TODO 必须说明未完成事项；需要跨版本追踪时附任务编号或设计文档链接。

### 2.10 包文档文件

- `doc.go` 是集中放置 package comment 的惯例文件名，不是 Go 的特殊入口，也不会因为文件名自动执行逻辑。
- 包级说明放在 `doc.go`，让 `go doc`、IDE 和 pkgsite 能展示统一的包契约；不要在其中放业务初始化或隐藏副作用。

### 2.11 测试可读性

- 每个 `Test...` 函数前写一句注释，说明它保护的可观察行为；注释不只重复函数名。

## 3. 评审启发

以下指标提示需要重新审视代码，但不单独构成拒绝理由：

- 匿名函数已经表达独立业务概念、需要单独测试，或跨越多个抽象层级；此时提取成具名函数。
- 函数同时处理策略、I/O、持久化、日志和状态修改；按不同变化原因拆分。
- 控制流出现连续深层嵌套；优先使用提前返回、提取决策或拆分阶段。
- 调用者需要了解目标模块的内部表、锁、dirty bit、session 或清理顺序；将意图操作收回目标模块。
- 三五行相似业务代码不必急于抽象；已经稳定重复的机械流程可以封装。
- 封装 `StartAll`、`StopAll`、`CloseAll` 等流程时，必须保留执行顺序、失败阶段、对象身份、多错误和回滚语义。
- 一个抽象需要大量泛型参数、回调或 option 才能使用；先检查是否把几个不同概念错误合并了。

## 4. Runtime 与业务错误边界

Runtime 可以统一提供：

- panic recovery；
- Handler 和 RPC 错误转换边界；
- tracing/metrics 的基础挂接；
- deadline/cancellation 传播；
- 生命周期阶段和失败对象的诊断信息。

Runtime 不应隐式决定：

- 哪些业务操作可以 retry；
- retry 次数与退避；
- 幂等键和重复执行语义；
- 业务 timeout；
- 部分成功后的补偿或回滚。

这些策略由拥有操作语义的模块决定，Runtime 只提供实现机制和可观测边界。

## 5. 验证门禁

验证采用分层策略。下一层只在上一层通过、风险需要或功能切片收尾时执行。

### 5.1 L0：格式与编译

每次编辑后先处理修改文件，并编译受影响 package：

```text
gofmt -w <modified-go-files>
go test ./path/to/package -run '^$'
```

目标耗时不超过 30 秒。纯文档修改只检查文档结构和链接，不运行无关 Go 测试。

### 5.2 L1：相关行为

开发循环只运行当前行为相关的测试：

```text
go test ./runtime -run 'Test(Call|Reply|Timeout)' -timeout=2m
```

相关测试默认在 2 分钟内完成。不要在每次小编辑后运行全仓验证。

### 5.3 L2：受影响 package

功能在局部完成后运行受影响 package 的完整测试和静态检查：

```text
go test ./runtime -timeout=5m
go vet ./runtime
```

默认预算为 5 分钟。`go test -timeout` 限制的是各测试二进制的执行时间；总体命令仍需观察实际进度。

### 5.4 L3：功能切片收尾

功能切片完成、准备提交或公开契约变化后执行：

```text
go test ./...
go vet ./...
```

全仓验证只在收尾执行一次，不在 L1/L2 每一步重复执行。

### 5.5 L4：高风险验证

涉及 goroutine、生命周期、锁、channel、Timer、Scheduler、Transport 或共享状态时，在收尾执行：

```text
go test -race ./...
```

压力、泄漏、跨进程和外部依赖测试可以独立为 slow/integration 测试集，由 CI、阶段收尾或明确需求触发；不得为了缩短日常验证而把普通正确性测试移出默认测试集。

### 5.6 耗时与重试规则

- 命令超过预算或长时间无输出时，先定位具体 package 和测试，不盲目等待。
- 同一命令超时后不得未经分析连续重试，也不得只通过增大 timeout 掩盖死锁或泄漏。
- 日常验证保留 Go cache；仅在排查缓存、随机性或 flaky test 时使用 `-count=1`。
- 不习惯性执行 `go clean -testcache`。
- CPU、内存和 I/O 互不争抢的检查可以并行；全仓测试与全仓 race 默认串行。
- 建立 package 正常耗时基线；明显变慢应作为潜在性能或资源泄漏回归处理。
- 如果预算内无法完成必要验证，交付时列出已通过、未完成、耗时位置和剩余风险。

工具通过不代表代码自动符合本规范；职责、命名、依赖方向和 API 阅读成本仍需要评审。
