// Package mysqlclient 为直接使用 database/sql 的 Service 建立有界 MySQL 连接池。
// 连接池由应用持有，并在 Runtime 停止后关闭；SQL 和数据模型仍由业务决定。
package mysqlclient
