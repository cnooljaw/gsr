// Package logging 提供统一 FIFO 日志入口和单写入者。
// 记录处捕获时间、位置与字段快照，输出顺序只由成功入队顺序决定。
// 应用持有 Logger owner，为 Runtime/Cluster 注入同一入口，最后关闭；
// ServiceLogger 只记录已绑定来源的日志，不能关闭或重新绑定。
// 不使用全局 logger、goroutine-local 或 Diagnostics 后台转发。
package logging
