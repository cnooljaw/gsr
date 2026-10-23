# Protobuf / Sproto 接入指南

Gate只处理不透明字节、连接和认证，Agent选择业务协议。Protobuf和Sproto保留各自模型，不增加公共Envelope或万能Codec。架构边界见 [ADR 0013](adr/0013-client-protocol-boundaries.md)。

## 依赖与职责

Go通过 `github.com/cnooljaw/gosproto v0.4.0` 正式依赖接入，不在GSR维护codec副本、源补丁或永久本机replace。后续优化版本发布后再显式升级；新目录迁移不等于正式版本已变更。

`protocol/sproto` 负责GSR Dispatcher、Context、业务错误与字节发送窄接口。Agent组合Gate连接与Dispatcher；连接归Gate，协议层不拥有网络生命周期。

TypeScript单文件权威源码、官方兼容测试、通用Go/TS/PB基准和优化文档已迁至 [cnooljaw/gosproto](https://github.com/cnooljaw/gosproto)。本地工作目录为同级gosproto；根README和 `benchmarks/README.md` 是维护入口。GSR只保留Node客户端示例及消费副本。独立仓库迁移已本地提交为 `ae956fe`，本示例消费副本已记录该提交，后续协议库工作从独立项目继续。

## Echo 接线

完整实现见 [Sproto包](../protocol/sproto/README.md) 和 [WebSocket示例](../examples/websocket-gate/README.md#sproto-联调)。MainService创建Gate，Agent持有客户端连接和Sproto Dispatcher；MainService退出不停止Gate或Agent。SecurityMode独立选择MAC/Plain。

接入支持Request/Response、Notify/Push、session关联及业务错误；服务端主动请求客户端并等待Response不在首版Dispatcher范围。

## TS 副本同步

在WebSocket示例目录执行：

```sh
npm run sync:sproto -- /path/to/gosproto
npm run check:sproto-source
npm run test:sproto:integration
```

同步工具复制权威 `typescript/src/sproto.ts` 和BSD许可证，并记录仓库HEAD、是否含未提交源码、文件路径和SHA256。离线校验不要求本机存在gosproto源码；副本需要修改时回独立仓库验证再同步。

## 资源与支持边界

默认packed输入/输出1MiB、编码与解包8MiB、深度64、字段/集合元素合计100000。Node Echo限制32个等待请求、64KiB字符串，关闭清理会话；业务安全检查与会话管理仍在GSR客户端。

TS仅接受JavaScript安全整数，超范围拒绝；完整int64的BigInt、double及所有binary组合不作为已验收能力。当前实现依赖 `node:buffer`，浏览器、Laya、Cocos兼容性作为gosproto `docs/TODO.md` 的待办，不能由复制文件推导支持。

## 验证归属

独立库保护官方wire、类型/数组、Pack/Unpack、预算、失败原子性和输出独占；GSR验证Dispatcher/Agent接线、会话、边界安全以及Plain/MAC真实Go↔TS链路。不在GSR重复维护完整codec夹具。

历史候选库研究、各轮性能数据和profile移至gosproto，旧GSR提交仍可追溯；本指南不重复维护codec优化过程。
