// Package runtime 实现 Core Service Runtime。
//
// Core 只负责本地 Runtime、Service、Message、Mailbox、Scheduler 和 Timer
// 的运行模型；Cluster 和其它外层能力不属于本包。
// 普通应用从 gsr.Run 进入；本包的 Runtime.Send/Call 是受信任基础设施入口。
// Runtime 直接向配置的共享 Logger 记录故障；Diagnostics 另提供有界观察事件，
// 不需要宿主将其转发为日志。Stop 超时不关闭事件流，仅进入 Closed 后关闭。
package runtime
