// Package logging 提供统一 FIFO 日志入口和单写入者。
// 记录处捕获时间、位置与字段快照，输出顺序只由成功入队顺序决定。
// 应用为一个 Runtime 及其 Cluster 注入同一 Logger，最后关闭 Logger；
// 不使用全局 logger、goroutine-local 或 Diagnostics 后台转发。
package logging
