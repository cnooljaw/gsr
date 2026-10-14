# ADR 0012：节点组装能力与 Service 日志入口

## 状态

Accepted，2026-10-14 用户确认。接口调整随 v1.0.1 发布；已发布的 v1.0.0 标签不移动。本裁决更新 ADR 0011 的组装入口和日志视图，不改变资源 owner 或 Core 边界。

## 依据与问题

只读参考 Skynet 的 lualib/skynet/cluster.lua、service/clusterd.lua、examples/main.lua 和 examples/watchdog.lua：入口可以发起节点监听和独立 Service 创建后退出，真正监听的组件继续运行。Lua/C API 从 Lua registry 取得绑定的 skynet_context，隐式提供来源与组件取得机制；Go 使用具体能力类型表达作用范围。

原 ServiceContext 混合通信和节点监听，Cluster 视图也提供 Open/ListenAddress；Logger 绑定视图仍能 Close 共享日志、重新 WithService。底层配置暴露 Logger，但 Run 禁止调用者填写。

## 裁决

- MainService 继续顺序组装后 Exit，不把监听自动启动搬到 Config，不增加 Manager、特殊 MainService 接口或通用生命周期容器。
- ServiceContext 增加 Node() *Node；Node 复用所属运行环境，限当前回调使用，不借给异步任务，不提供 Close、业务通信或内部组件 getter。
- Node 提供 RegisterService(name, id)、OpenCluster(name)、ClusterListenAddress()、OpenHTTP(address, handler)、CreateGate(ctx, config)。CreateGate 创建独立 GateService，返回实际监听地址，生命周期归 Runtime。
- 名称仍归目标 ServiceID，目标停止时自动清理，注册者退出不撤销；创建成功的 Service 不随创建者退出而停止。
- self.Cluster() 只提供来源绑定的 Send/Call，移除 Open/ListenAddress；ServiceContext 移除 Register/OpenHTTP/OpenGate，不保留兼容别名。
- 节点表 host 是访问地址，port 同时用于本机监听；OpenCluster(name) 仍取端口绑定 0.0.0.0，部署访问限制归网络/ACL/防火墙，不拆分地址配置。端口零只供动态监听测试，不进入出站路由。
- logging.Logger 保留原始记录、来源绑定、观测和关闭；WithService 返回仅有 Info/Error 的具体 ServiceLogger。ServiceLogger 不嵌入 Logger，不暴露 Close、WithService 或原始入口；self.Logger() 返回 ServiceLogger。共享队列、计数、记录位置和关闭行为不变。
- gsr.Config 使用应用层 RuntimeConfig/ClusterConfig，保留当前可用字段但不包含 Logger，不嵌入或别名到底层类型；Run 转换并注入唯一 Logger，底层基础设施配置仍可显式注入 Logger。
- MainService 启动失败由 Run 收尾；普通 Service 启动失败只回滚自身，不扩大为节点关闭。CreateService 的 context 是启动操作协作取消，不是新 Service 的长期生命周期。

## 代价与验证

公开 API 源码不兼容，调用方、示例、教程和工作模板一起迁移；文档明确区别于 v1.0.0。不改变消息、wire、Source、队列、等待或清理顺序。

扩展现有日志和接线测试，保护窄日志方法集合、记录来源/位置、入口退出后名称与监听继续有效、失败资源释放、访问 host 不改变通配监听。收尾执行全仓 test/vet/race 与双节点恢复，新版发布前保持旧标签。

## 后续收敛：统一节点运行对象（2026-10-14）

用户确认将内部 application 合并进 Node。Node 保存节点运行状态，Run 创建唯一对象并管理其生命周期，业务通过 self.Node() 借用同一个对象的公开能力；同一 Run 共享 Node，不同 Run 保持隔离。删除包装与注册转发，将内部地址表命名为 nodeAddresses；字段、close 和内部服务适配方法仍未导出。不增加公开方法、构造器、资源 getter 或 Manager，不改变关闭顺序和通配监听。
