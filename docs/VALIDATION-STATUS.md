# 当前阶段验证清单

本清单整理 Core、Cluster 与日志当前已有的验证证据，不替代 API 契约，也不表示生产发布或兼容性冻结。实施路线图保留各轮历史切片；判断当前覆盖范围时先看这里，再进入对应测试。

## 已覆盖的运行链

| 能力 | 主要测试证据 | 证据边界 |
| --- | --- | --- |
| 本地 Service 生命周期、Send、Call、Timer | [Service](../runtime/service_test.go)、[Call](../runtime/call_test.go)、[Timer](../runtime/timer_test.go) | 本地行为与关闭契约，不代表真实业务负载 |
| 规模与关闭后的资源状态 | [规模测试](../runtime/scale_test.go) | 包含 10,000 个空闲 Service 和 10,000 个 Timer；不是吞吐、尾延迟或长期内存稳定性承诺 |
| 公开 Cluster 调用、Open 与关闭 | [公开出站](../cluster/cluster_test.go)、[Open](../cluster/open_test.go)、[关闭边界](../cluster/open_boundary_test.go) | 同一测试进程内的真实 TCP 回环，不是独立进程部署验收 |
| 断线、按需恢复与不重放 | [出站连接](../cluster/outbound_peer_test.go)、[TCP 集成](../cluster/tcp_integration_test.go) | 覆盖半写、断线和下一请求恢复；不承诺结果不确定请求可安全重试 |
| 坏帧、超限与残帧隔离 | [协议故障](../cluster/open_protocol_test.go)、[响应读取](../cluster/response_reader_test.go) | 合法后续调用继续运行；不可恢复帧边界关闭连接，不把残帧送入 Handler |
| 重复坏帧诊断与日志 | [双连接日志](../cluster/open_logging_test.go)、[入站连接](../cluster/inbound_connection_test.go) | 每连接同类首条、抑制计数和结束汇总；不是全局限流或可靠审计 |
| FIFO 日志、文件滚动及失败收尾 | [日志顺序](../logging/logger_test.go)、[文件](../logging/file_test.go)、[输出器生命周期](../logging/writer_lifecycle_test.go) | 覆盖短写、Flush 故障、保留历史和借用所有权；不保证 fsync、磁盘崩溃恢复或云端收到 |

## 当前检查结果

- 全仓 `go test ./... -timeout=5m` 与 `go vet ./...` 在前一测试切片通过；本轮仅整理文档，没有改动 Go 代码。
- 本轮补跑 `go test -race ./... -timeout=5m` 通过，关闭了前一轮权限审核超时导致的未验证项；部分未变化 package 使用 Go 测试缓存。
- `git diff --check` 用于文档与补丁格式检查。
- 未在本轮运行 benchmark，不把普通测试耗时当作性能结果。

## 仍需单独验证的范围

1. **独立进程运行链**：当前主要是单进程内两个节点的回环测试。还需两个独立进程分别加载配置、打开独立日志目录、完成 Send/Call，并验证停掉目标进程后的错误与重启后的按需恢复。
2. **持续负载和容量**：已有空闲规模基线，但持续热点调用、日志慢盘、Timer 集中到期等组合负载还需目标机器上的容量测算。不能据此决定预先更换 Scheduler 或 Timer 架构。
3. **部署与运维**：进程退出信号、外部日志采集器、磁盘配额与告警还没有完整部署验收；SLS 属于外层采集，不给 Core 添加云 SDK。
4. **安全与发现**：认证协议、动态发现仍按已有设计边界留待实际需求。当前可信内网模式不提供对端身份保证，不能直接当成公网服务使用。

## 下一步建议

优先完成一个最小双进程运行示例及启动说明：应用用启动参数选配置，每个进程配置独立日志目录，调用端无需 Open，接收端显式 Open；业务只演示 Send 与 Call，资源按既有顺序关闭。

这一步先证明框架能被应用清楚地组装和运行，不引入通用配置框架、发现服务、网关或新 Runtime 生命周期 API。新增可执行示例后再补自动化跨进程故障验证；上述事项未完成前，不宣称生产验收完成。
