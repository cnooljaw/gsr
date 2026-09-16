# 统一 FIFO 日志服务

状态：已实现统一入口、单写入者、Runtime/Cluster 直接接线、滚动文件及可查询的输出失败状态。Cluster 同连接同类坏帧首条保留与重复抑制已实现；SLS 采集部署尚未实现。

## 设计裁决

整个 Runtime 的业务和基础设施日志共用一个队列，以成功推入日志队列的顺序输出，不按 Service 分队列再汇总，也不按时间戳重排。不要求日志写完文件后业务才能继续。

记录处采集时间、文件行号、来源和内容快照；序号分配与成功入队在同一锁内完成。并发线程在采集时间后可能被调度挂起，因此时间戳不一定递增；seq 表示入队顺序，不宣称建立所有业务操作的全局因果顺序。

本地 Skynet 的 skynet_error 先格式化文本、附 Service 地址再推入 logger；skynet_mq_push 在锁内追加，logger 串行输出。文件时间戳是在 logger 消费时生成，普通 skynet.error 不自动附代码行号。GSR 沿用 FIFO 模型，改为记录处采集时间和调用位置，不照搬可扩容队列或 Lua/C 运行机制。

## 已实现使用方式

```go
// main：默认 stdout；Run 负责共享日志接线与关闭。
err := gsr.Run(ctx, gsr.Config{}, &MainService{})
// 处理运行结果 err。

// Service 回调：以下是另一个调用位置的片段，框架已绑定当前 Self。
log := self.Logger()
err = log.Info("开始结算", "battleID", battleID)
err = log.Error("结算失败", "battleID", battleID, "err", cause)
// 根据需要处理入队错误；不等待写盘。
```

logging.Logger 是专用日志服务的入口，不是普通业务 Service，也不依赖普通 Runtime Mailbox。所有绑定入口共享一个队列和一个串行 writer goroutine。gsr.Run 创建 Logger 并给所属 Runtime、Cluster 和业务接入同一个实例，最后关闭；没有全局 logger 或 goroutine-local。业务可在 OnStart 取得并保存自身 Logger，用于 OnStop 等场景，不必保存 ServiceContext。

底层 Runtime.Config.Logger 与 ClusterConfig.Logger 都只借用该实例，不管理其生命周期；底层直接构造时 nil 保留仅 Diagnostics 的行为，不隐式启动后台写入者。应用使用 gsr.Run 时必须通过 Config.Logging 配置，不能自行设置这两个 Logger 字段。logging.New 的 Directory 和 Writer 都未设置时默认终端；可传入 io.Writer，输出所有权仍归应用。

文件输出由配置显式选择，内部文件由 Logger 创建并关闭；Directory 与 Writer 同时设置返回错误：

```go
err := gsr.Run(ctx, gsr.Config{
    Logging: logging.Config{Directory: "./log/battle-01"},
}, &MainService{})
// 处理 err；另一个进程从自己的配置传入 ./log/battle-02。
// 业务可用 self.Logger().Err() 查询首次输出故障，Run 收尾时等待日志关闭。
```

## 顺序、时间与地点

每行输出 time、seq、level、service、file、line、message、fields。time 在 Info/Error 调用路径采集，file/line 指向调用者；字段在调用返回前 JSON 快照，后台不再读取 map、slice、error 或业务对象。调用期间并发修改传入字段仍由调用方避免。

服务标识由框架通过 WithService 绑定当前 Self；无绑定为零。框架自动日志的位置是 Runtime/Cluster 捕获并记录故障的边界，不伪造为业务错误源头。Handler panic 另带已有 stack；普通 error 的原始位置需要业务在发生处记录。

Diagnostics 保留独立观测用途：Runtime/Cluster 在产生诊断的同步路径直接写 Logger，然后投递观测事件。不得再消费 Diagnostics 转发到同一 Logger，否则会重复记录并扭曲顺序。此前“外层消费 Diagnostics 作为主日志”的方案已被此裁决替代。

## 有界性与失败

本轮采用非阻塞有界 admission：默认 Capacity=1024，负数无效，正数显式配置。满载返回 ErrQueueFull，并由 DroppedCount 计数；不会静默扩容或等待磁盘。序号仅分配给成功接纳记录，uint64 耗尽返回 ErrSequenceExhausted，不回绕。

每条编码记录最多 64 KiB（不含换行），超限或不可快照字段返回 ErrInvalidRecord；字段为 string 键值对，不允许重复键。字段格式化 panic 被捕获。该上限限制排队记录大小，不等同于限制任意自定义 MarshalJSON 的执行时间或临时分配，扩展实现必须协作返回。

Writer 错误、短写或 panic 使输出进入失败状态，拒绝后续日志并停止写入；WriteFailureCount 累计失败记录及队列残留数，Err() 和 Close 返回首次输出错误。不为每次失败建立无限错误链，也不自动重试造成重复。失败可能导致日志缺失或部分行，不承诺可靠审计。框架自动记录处不把日志入队失败替换为业务错误，满队列由日志自身计数。

## 生命周期

正常顺序：停止业务日志生产 → Cluster.Quiesce → Runtime.Stop/Close → Cluster.Close → 应用退出记录 → Logger.Close(ctx) → 关闭应用拥有的输出文件。

Logger.Close 停止接纳，正常时排空队列并调用输出器可选的 Flush()，失败时将残留计入失败；最终关闭内部文件，不主动关闭借用的 Writer。ctx 只限制本次等待，超时后唯一 writer 继续，后续 Close 可以继续等待。成功不代表 fsync、云端接收或采集完成；底层 Writer 无限阻塞时 Go 不能强杀，不得提前关闭仍被写入的文件。

## 文件输出

### 按大小滚动

- 当前文件固定名为 gsr.log，所在目录由各进程配置显式指定。
- 单文件默认上限 100 MiB；写下一条会超限时先滚动，不拆开一条日志。
- 默认保留 10 个历史文件，不包含当前文件；超过后删除最旧历史文件。
- 滚动由现有单写入者执行，不增加后台任务；切文件不重置 seq，不改变 FIFO 顺序。
- 序号仅在当前 Logger 生命周期内递增；进程重启重新创建 Logger 后从头开始，不声称跨重启全局唯一。
- 第一版不按日期切分、不压缩。

历史文件命名为 gsr.log.1 至 gsr.log.10，.1 最新、.10 最旧。滚动时删除最旧文件，从旧到新依次移动 .9 → .10 直至 .1 → .2，再将当前文件移到 .1，创建新的 gsr.log。进程重启追加已有当前文件，不清空，写下一条前照常检查大小。自动删除仅针对该日志输出器管理的历史文件，不能清理目录内其它文件。

### 启动配置与目录

应用通过不同启动参数选择配置文件，各配置显式指定独立相对日志目录。例如两个战斗进程分别读取 battle-01 和 battle-02 的配置，对应 ./log/battle-01 与 ./log/battle-02；同一工作目录也可运行多个进程。目录名不由日志组件拼接 NodeID、PID 或推断业务身份。

相对目录以进程启动工作目录为基准解析并固定，不相对配置文件目录，也不相对可执行文件位置。应用负责解析命令行、读取配置并传入最终目录；日志组件不选择配置文件，不绑定 YAML 或某个命令行参数名。文件模式显式创建缺失目录，目录创建或文件打开失败直接返回错误，不静默回退终端。不启用文件输出时仍默认终端，不创建日志目录。

每个进程独占其配置目录内的当前和历史日志文件；配置不同进程使用同一目录属于错误部署，不据此引入多进程共同滚动协议。多个 Logger 也不得同时滚动同一组文件。

默认终端、可选滚动文件，目的地不与前台/后台运行硬绑定。“静默”只是不输出终端，不等于禁用日志。SLS 由外部采集器采集文件或标准输出，不在 Core/Cluster 中加入云 SDK、凭据或上传重试。

滚动条件、默认保留上限及路径规则已实现，第一版不暴露大小与历史数量配置。坏帧重复诊断由 Cluster 在源头抑制：每连接每固定类别只保留首条，重复项只计数，连接结束补一条数量汇总；没有每秒重复输出或定时任务。SuppressedDiagnosticCount 不与 Diagnostics 满载丢弃数、日志队列丢弃数混用。业务日志不去重，不先建设通用日志插件或业务限流框架。完整类别、汇总字段及 best-effort 边界见 API-DESIGN 的诊断契约。

### 输出失败状态

初始化创建目录或打开文件失败，直接返回错误，由应用决定是否启动。运行中写入、短写、重命名或删除失败后，文件日志输出进入失败状态，停止继续写入和滚动，不自动恢复、不转终端、不无限重试，也不自动退出业务进程。

失败状态可由应用查询，保存首次输出错误，最终 Close 返回该故障。失败后新日志调用返回输出故障，不再声称已接纳；已经接纳但未能输出的记录计入 WriteFailureCount，与满载 DroppedCount 分开。日志故障不覆盖 Runtime/Cluster 的业务结果，由应用决定告警或重启。

停止文件操作指停止继续写入和滚动，不禁止最终安全释放已持有句柄；关闭失败也不得掩盖首次故障。滚动中途失败不通过反复移动文件尝试恢复，也不宣称文件集合已自动回滚。

文件和借用 Writer 使用同一失败状态；Err() 可在运行中查询，nil 表示尚未观察到输出故障，不保证后续输出成功。Flush 或最终文件关闭错误同样保留首次故障，但不虚构逐条失败计数。失败写入者自动停止并释放内部文件，不依赖应用先调用 Close。

## 验证

Runtime 集成回归 TestRuntimeLoggingFailureIsolation 覆盖 Writer 阻塞且队列满、Writer 已失败两种状态：Service 内记录返回日志错误，但业务成功值与领域错误不被替换；框架自动日志失败不阻断独立 Diagnostics。Runtime.Stop/Close 不等待借用的 Logger 写入完成，也不关闭它；日志满载/输出失败与 Runtime 诊断丢弃计数分别核对。测试 Service 显式不将日志错误作为业务返回值；框架不会替业务代码吞掉它自己返回的错误。

输出器生命周期回归覆盖：短写返回 io.ErrShortWrite 并终止输出；写入故障后不再 Flush；正常关闭只 Flush 一次；Flush 错误或 panic 保留在 Err()/Close 中但不虚构逐条失败数；重复 Close 保持同一结果；无论成功或失败都不调用借用 Writer 的 Close。

测试覆盖并发 FIFO 序号、来源绑定、调用处时间与行号、字段快照、满载非阻塞、关闭超时后继续等待、Writer 错误/panic，以及业务、Runtime、Cluster 共享序列且生产者关闭后仍可记录退出日志。文件测试覆盖相对目录、冲突配置、初始化错误、完整记录滚动、10 份历史保留、重开追加、跨文件序号连续、异常目录项保护和失败后停止接纳及残留计数。
