// Package runtime 实现 Core Service Runtime。
//
// Core 只负责本地 Runtime、Service、Message、Mailbox、Scheduler 和 Timer
// 的运行模型；Cluster 和其它外层能力不属于本包的第一阶段实现。
// Runtime.Diagnostics 提供有界 Handler 故障事件，宿主负责日志消费；
// Stop 超时不关闭事件流，仅资源释放并进入 Closed 后关闭，不承诺可靠审计。
package runtime
