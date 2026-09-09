// Package cluster 提供建立在 Core 之上的跨节点 Send/Call。
//
// 应用通过 NewCluster 配置静态目标和命令 codec，再由受信任组装层调用
// Cluster.Client 绑定 ServiceID。网络连接按请求需要建立，不要求本地身份或监听。
// Send 成功只表示本地接纳；投递异常从有界 Diagnostics 出口读取。
//
// 停机先 Quiesce，停止 Core 后再 Close；Close 的 context 只限制调用方等待。
// 本包当前提供公开出站组装，入站 Open、认证和动态目标配置尚未公开。
package cluster
