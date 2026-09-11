# 统一 FIFO 日志服务

状态：已按用户最新裁决实现统一入口、单写入者及 Runtime/Cluster 直接接线。滚动文件、SLS 采集部署与源头限频尚未实现。

## 设计裁决

整个 Runtime 的业务和基础设施日志共用一个队列，以成功推入日志队列的顺序输出，不按 Service 分队列再汇总，也不按时间戳重排。不要求日志写完文件后业务才能继续。

记录处采集时间、文件行号、来源和内容快照；序号分配与成功入队在同一锁内完成。并发线程在采集时间后可能被调度挂起，因此时间戳不一定递增；seq 表示入队顺序，不宣称建立所有业务操作的全局因果顺序。

本地 Skynet 的 skynet_error 先格式化文本、附 Service 地址再推入 logger；skynet_mq_push 在锁内追加，logger 串行输出。文件时间戳是在 logger 消费时生成，普通 skynet.error 不自动附代码行号。GSR 沿用 FIFO 模型，改为记录处采集时间和调用位置，不照搬可扩容队列或 Lua/C 运行机制。

## 已实现使用方式

```go
logger, err := logging.New(logging.Config{})
// 处理 err；默认 stdout，每行一个 JSON 对象。

local, err := runtime.NewRuntime(runtime.Config{Logger: logger})
network, err := cluster.NewCluster(cluster.ClusterConfig{
    Codecs: codecs,
    Nodes: nodes,
    Logger: logger,
})
// 处理构造错误，按应用顺序启动与创建 Service。

log := logger.WithService(uint64(serviceID))
err = log.Info("开始结算", "battleID", battleID)
err = log.Error("结算失败", "battleID", battleID, "err", cause)
// 根据需要处理入队错误；不等待写盘。
```

logging.Logger 是专用日志服务的入口，不是普通业务 Service，也不依赖普通 Runtime Mailbox。所有绑定入口共享一个队列和一个串行 writer goroutine。应用创建一个 Logger 并给所属 Runtime、Cluster 和业务注入同一个实例，最后关闭；没有全局 logger 或 goroutine-local。业务可在 OnStart 使用 Self() 绑定并保存自己的日志入口，不必保存 ServiceContext。

Runtime.Config.Logger 与 ClusterConfig.Logger 都只借用该实例，不管理其生命周期。nil 保留原先仅 Diagnostics 的行为，不隐式启动后台写入者。logging.New 的 Writer 为 nil 时默认终端；可传入 io.Writer，输出所有权仍归应用。

## 顺序、时间与地点

每行输出 time、seq、level、service、file、line、message、fields。time 在 Info/Error 调用路径采集，file/line 指向调用者；字段在调用返回前 JSON 快照，后台不再读取 map、slice、error 或业务对象。调用期间并发修改传入字段仍由调用方避免。

服务标识通过 WithService 显式绑定；无绑定为零。框架自动日志的位置是 Runtime/Cluster 捕获并记录故障的边界，不伪造为业务错误源头。Handler panic 另带已有 stack；普通 error 的原始位置需要业务在发生处记录。

Diagnostics 保留独立观测用途：Runtime/Cluster 在产生诊断的同步路径直接写 Logger，然后投递观测事件。不得再消费 Diagnostics 转发到同一 Logger，否则会重复记录并扭曲顺序。此前“外层消费 Diagnostics 作为主日志”的方案已被此裁决替代。

## 有界性与失败

本轮采用非阻塞有界 admission：默认 Capacity=1024，负数无效，正数显式配置。满载返回 ErrQueueFull，并由 DroppedCount 计数；不会静默扩容或等待磁盘。序号仅分配给成功接纳记录，uint64 耗尽返回 ErrSequenceExhausted，不回绕。

每条编码记录最多 64 KiB（不含换行），超限或不可快照字段返回 ErrInvalidRecord；字段为 string 键值对，不允许重复键。字段格式化 panic 被捕获。该上限限制排队记录大小，不等同于限制任意自定义 MarshalJSON 的执行时间或临时分配，扩展实现必须协作返回。

Writer 错误、短写或 panic 记录为输出失败，WriteFailureCount 累计失败记录数，Close 返回首次输出错误；不为每次失败建立无限错误链，也不自动重试造成重复。失败可能导致日志缺失或部分行，不承诺可靠审计。框架自动记录处不把日志入队失败替换为业务错误，满队列由日志自身计数。

## 生命周期

正常顺序：停止业务日志生产 → Cluster.Quiesce → Runtime.Stop/Close → Cluster.Close → 应用退出记录 → Logger.Close(ctx) → 关闭应用拥有的输出文件。

Logger.Close 停止接纳，排空队列并调用输出器可选的 Flush()；不主动关闭借用的 Writer。ctx 只限制本次等待，超时后唯一 writer 继续，后续 Close 可以继续等待。成功不代表 fsync、云端接收或采集完成；底层 Writer 无限阻塞时 Go 不能强杀，不得提前关闭仍被写入的文件。

## 后续范围

默认终端、可选滚动文件，目的地不与前台/后台运行硬绑定。“静默”只是不输出终端，不等于禁用日志。SLS 由外部采集器采集文件或标准输出，不在 Core/Cluster 中加入云 SDK、凭据或上传重试。

滚动文件路径、滚动条件、保留上限及磁盘失败策略仍需收口，未在本轮实现。源头坏帧重复诊断限频仍归 Cluster，参数与抑制计数待定；现有 Diagnostics 丢弃数不与日志队列丢弃数混用。不先建设通用日志插件或业务限流框架。

## 验证

测试覆盖并发 FIFO 序号、来源绑定、调用处时间与行号、字段快照、满载非阻塞、关闭超时后继续等待、Writer 错误/panic，以及业务、Runtime、Cluster 共享序列且生产者关闭后仍可记录退出日志。
