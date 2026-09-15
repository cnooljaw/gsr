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

2026-09-15 规模切片：已实际运行现有空闲 Service/Timer 规模测试及固定次数 benchmark，结果见 [本地规模基线](PERFORMANCE-BASELINE.md)。这是单机小样本，不替代持续负载或生产验收；该切片未修改 Go 代码。

后续热点切片新增 TestRuntimeHotMailboxLoad（GSR_LOAD_TEST=1 显式启用）：4 个发送者、10 万次固定尝试、容量 64/1024，逐条核对接纳与处理结果并记录到达延迟分位数。它是短突发测试，不替代下文的持续负载验收；未修改生产实现。

Timer 集中到期切片新增 TestRuntimeTimerBurstLoad，沿用 GSR_LOAD_TEST=1：10,000 个 Timer 同轮到期，校验接纳/拒绝计数、消息唯一性和零 Source。实测见规模基线；使用 fake clock，不代表真实时钟精度或定时业务必达保证。

日志测量切片新增 TestLoggingBatchAccounting 与 BenchmarkLoggingBatch：比较禁用、io.Discard、临时文件输出，分别报告成功接纳、满载丢弃及关闭排空成本。本次固定小字段单发送者样本无丢弃；多生产者、慢盘和真实业务日志频率仍未测量。

双进程示例切片补充：已在临时工作目录启动两个独立进程，观察 Send 接收和 Call 成功；停止接收端后调用端继续运行并记录 connection refused，重启接收端后后续新操作恢复成功；最后两个进程均以退出码 0 结束并写出 process stopped。此为本机手动演练，不等同于自动化跨进程门禁或长期负载测试。

- 全仓 `go test ./... -timeout=5m` 与 `go vet ./...` 在前一测试切片通过；本轮仅整理文档，没有改动 Go 代码。
- 本轮补跑 `go test -race ./... -timeout=5m` 通过，关闭了前一轮权限审核超时导致的未验证项；部分未变化 package 使用 Go 测试缓存。
- `git diff --check` 用于文档与补丁格式检查。
- 前述故障验证切片未运行 benchmark；后续 2026-09-15 规模切片的独立测量见本节链接，不把普通测试总耗时当作性能结果。

## 仍需单独验证的范围

1. **独立进程运行链**：已有 [双进程示例](../examples/twonode/README.md) 和显式启用的 TestTwoNodeProcessRecovery、TestTwoNodeProcessCrashRecovery 门禁，覆盖独立配置、日志目录、Send/Call、正常停机或 SIGKILL 后调用端继续运行与重启后的新请求恢复。强杀发生在首轮 Call 完成后，不覆盖在途执行或日志持久性。默认集成测试仍主要在单进程内运行；跨进程测试不覆盖命令行参数解析、跨机器部署或网络分区。
2. **持续负载和容量**：已有空闲规模基线，但持续热点调用、日志慢盘、Timer 集中到期等组合负载还需目标机器上的容量测算。不能据此决定预先更换 Scheduler 或 Timer 架构。
3. **部署与运维**：进程退出信号、外部日志采集器、磁盘配额与告警还没有完整部署验收；SLS 属于外层采集，不给 Core 添加云 SDK。
4. **安全与发现**：认证协议、动态发现仍按已有设计边界留待实际需求。当前可信内网模式不提供对端身份保证，不能直接当成公网服务使用。

## 下一步建议

最小双进程运行示例及启动说明已提供：应用用启动参数选配置，每个进程配置独立日志目录，调用端无需 Open，接收端显式 Open；业务只演示 Send 与 Call，资源按既有顺序关闭。

这一步先证明框架能被应用清楚地组装和运行，不引入通用配置框架、发现服务、网关或新 Runtime 生命周期 API。自动化跨进程正常停机、强杀与重启恢复门禁已补充；持续负载、实际部署和其它故障场景仍需独立验收，不宣称生产验收完成。
