// Package redisclient 为直接使用 go-redis 的 Service 提供有界的默认连接配置。
// 客户端由应用持有，并在 Runtime 停止后关闭；缓存键、结构和策略仍由业务决定。
package redisclient
