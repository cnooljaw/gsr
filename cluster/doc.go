// Package cluster 提供建立在 Core 之上的跨节点 Send/Call。
//
// 普通应用通过 gsr.Run 配置节点表，再用 self.Cluster() 发起通信或按名称 Open。
// 本包的 NewCluster、Cluster.Client 和显式 Runtime 接线供受信任基础设施使用。
// 所有 Command 共用 PayloadCodec，不维护命令编码或准入清单。
// 网络按需建连，不要求本地身份或监听；Send 成功只表示本地接纳。
// 传输故障直接进入共享 Logger，并可从有界 Diagnostics 出口观察。
//
// 停机先 Quiesce，停止 Core 后再 Close；Close 的 context 只限制调用方等待。
// 本包提供公开出站组装及可选入站 Open；认证和动态目标配置尚未公开。
package cluster
