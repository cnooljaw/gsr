// Package runtime implements the Core Service Runtime.
//
// Core 只负责本地 Runtime、Service、Message、Mailbox、Scheduler 和 Timer
// 的运行模型；Cluster 和其它外层能力不属于本包的第一阶段实现。
package runtime
