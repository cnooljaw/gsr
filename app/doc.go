// Package app 提供 Core 之外的应用宿主，统一组装并关闭 Runtime、Cluster 和 Logger。
// 业务入口仍是普通 Service；宿主不提供业务 Send/Call，也不使用全局 Runtime。
package app
