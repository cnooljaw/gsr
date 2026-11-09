# 验证覆盖与边界

发布基线为 **GSR v1.0.2**，当前开发分支另外包含有界 Service 池。本页记录覆盖范围、已有验证证据与复现入口；发布说明保留各版本当时的验收，不维护逐轮执行流水。

## 测试覆盖

| 层级 | 主要保护的行为 | 入口 |
| --- | --- | --- |
| Core | ID 不复用、串行与来源、Args 所有权、满载、Call 自动回复/超时/停止、Timer、panic、生命周期与 Inspection | [runtime](../../runtime/) |
| 应用组装 | Run/MainService、Node 隔离、来源绑定、名称退出/重建、HTTP/Gate 组装 | [run_test.go](../../run_test.go)、[service_test.go](../../service_test.go) |
| Service 池 | 多调用者并行、成员队列、来源、超时不重放、名称预留、无效工厂、失败/取消回滚、成员退出 | [池测试](../../service_pool_test.go)、[Runtime 配置与回收](../../runtime/service_config_test.go)、[可运行示例](../../examples/service-pool/) |
| Cluster | 统一编码、帧限制、Session/连接归属、入站有界 Call、按需恢复、不重放、Quiesce/Close、诊断与抑制 | [cluster](../../cluster/)、[双节点测试](../../examples/twonode/) |
| Gate | Plain/MAC、TLS/握手/seq、容量与慢客户端、Agent 投递、断开通知、迟到写回、重复关闭 | [gate](../../gate/) |
| Protocol | Protobuf / Sproto 各自的分发、参数与协议校验、响应/推送、会话和业务错误 | [protocol](../../protocol/)、[Sproto](../../protocol/sproto/) |
| Go ↔ TS | 认证固定向量、Protobuf Echo、Sproto Plain/MAC 真实链路与边界 | [WebSocket 示例](../../examples/websocket-gate/README.md) |
| 外部资源 | HTTP、Redis/MySQL Service 的业务接线与替身测试；真实环境单独启用 | [示例索引](../README.md#1-运行与接入) |

服务池跨节点测试复用既有双节点夹具；示例测试只保护创建、调用、结果收集和退出接线。PendingCall、codec 与底层 Mailbox 不在每层重复搭建完整夹具。Sproto 官方兼容与通用编解码矩阵由独立 gosproto 项目维护。

## 最近验证基线

以下是已有执行记录，不表示每次文档修改都重新执行，也不能替代对后续代码变更的验证。

| 基线 | 已通过的检查 | 限制 |
| --- | --- | --- |
| v1.0.2，2026-10-23 | 全仓 race（含双进程恢复）、vet、Sproto Go↔TS 四项、TS 认证五项 | 见 [发布验收](../releases/v1.0.2.md#发布验证)；未重跑完整负载或真实 MySQL |
| Service 池，2026-11-06 | `GSR_PROCESS_TEST=1 go test -race ./... -timeout=2m`、`go vet ./...`；零大小成员增量后根包 race 与根包/Runtime vet | 服务池尚未发布；全仓结果在零大小增量前取得 |
| 池示例，2026-11-09 | `go test -race ./examples/service-pool -timeout=2m`、`go run ./examples/service-pool` | 两个调用者、两个成员的接线与正常退出，不是容量测试 |

## 复现入口

从仓库根目录运行；日常按改动范围选用，具体门禁见 [开发规范](../development/GO-CODE-STYLE.md#5-验证门禁)。

```sh
# 跨 package 或发布验收；不与另一套全仓测试同时运行。
GSR_PROCESS_TEST=1 go test -race ./... -timeout=2m
go vet ./...

# 客户端接线，在 examples/websocket-gate 目录运行。
npm ci
npm run test:sproto:integration
```

双进程恢复、混合负载与 Gate 压力由各自显式环境开关控制，普通测试通过不能替代这些专项验收。命令与参数分别见 [双节点示例](../../examples/twonode/README.md)、[混合负载](MIXED-LOAD.md) 和 [Gate 设计](../design/GATE-PROTOCOL.md#验证与部署边界)。

## 尚未完成的验收

- 目标机器上的真实 Handler、数据库、日志慢盘、Timer 与网络组合负载，长期容量及泄漏观察。
- 跨机器网络分区、在途执行时强杀与业务一致性；已有进程恢复测试在首轮 Call 完成后强杀，不能证明在途执行结果。
- 部署的信号处理、外部日志采集、磁盘配额和告警；真实 MySQL/Redis 的运行环境。
- Cluster 认证与动态发现尚未实现；可信内网连接不提供对端身份保证。
- Gate 的独立密码协议审查及目标部署的长期失败组合；浏览器/Laya/Cocos 和 TS Sproto 扩展类型兼容。

性能数字统一见 [历史性能测量](PERFORMANCE.md) 和 [混合负载样本](MIXED-LOAD.md)。不同代码、环境、负载生成器及日志模式的数据不作为可互换的当前容量结论。
