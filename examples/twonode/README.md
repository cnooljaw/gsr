# 最小双进程示例

使用标准库 JSON 配置和公开 GSR API，不依赖发现服务。两个进程各有自己的 Runtime、Cluster 和 Logger；配置中的目标地址是回环地址，Open 按节点表取端口并绑定 0.0.0.0。只用于可信网络，需按部署要求配置防火墙，不应直接暴露公网。

main 只调用 `gsr.Run(ctx, config, mainService)`。MainService 创建 Echo 或 Caller；接收端通过 `self.Register("echo", id)` 发布名称，再用 `self.Cluster().Open("battle")` 开放监听，最后 Exit。普通 Handler 通过 `ctx.Cluster().Send/Call` 通信、`ctx.Logger()` 写入来源绑定的日志，不持有 Host、ClusterClient 或全局单例。框架负责初始化回滚和最终关闭；入口退出不停止其它 Service。

Cluster 内建统一 ValueCodec，无需 Command 清单。Echo 创建后即可工作，Caller 用 Timer 驱动业务，不强制 Start Command。

Echo 和 Caller 均显式实现 `Handle`，普通命令交给各自的具名 Dispatcher：Notify 接收一个 `uint64` 操作编号且不返回结果，Echo 返回该编号；多余或错误类型的参数由 Dispatcher 拒绝。

从仓库根目录构建一次：

```sh
go build -o /tmp/gsr-twonode ./examples/twonode
```

终端一启动接收端：

```sh
/tmp/gsr-twonode -config examples/twonode/server.json
```

终端二启动调用端：

```sh
/tmp/gsr-twonode -config examples/twonode/caller.json
```

两者通过 Ctrl+C 或 SIGTERM 有序退出。日志路径相对于进程启动工作目录，不相对于配置文件位置：

```sh
tail -f log/battle-01/gsr.log
tail -f log/caller-01/gsr.log
```

`process ready` 包含实际 `listen_address`，调用端为空；接收端的 listen 配置是节点名，地址位于共同 nodes 表中。测试可在表中配置端口 0，按实际绑定结果构造回环拨号地址；端口零条目不是有效出站路由，不用于正式部署。

## 自动化跨进程门禁

```sh
GSR_PROCESS_TEST=1 go test ./examples/twonode -run '^TestTwoNodeProcess.*Recovery$' -v -timeout=1m
GSR_PROCESS_TEST=1 go test -race ./examples/twonode -run '^TestTwoNodeProcess.*Recovery$' -v -timeout=1m
```

测试使用两个独立测试子进程调用同一 `runFile` 启动入口，保留真实配置解析、信号处理、TCP 和日志文件；不在每次测试时重新构建可执行文件。它不覆盖 main 的命令行 flag 解析。测试使用临时目录和系统分配的回环端口，等待日志中的成功/失败/恢复证据；并验证两端退出、Send 到达及演练中没有重复投递。重启复用原端口，若被其它进程占用则测试失败，不静默改用其它端口。

默认 `go test ./...` 跳过此跨进程演练；开启后各等待阶段和子进程均有超时，断言失败也会取消并回收子进程。当前使用 Unix 中断信号，面向本项目的 macOS/Linux 开发环境，不代表 Windows 信号行为已验证。

`TestTwoNodeProcessRecovery` 使用中断信号正常停机；`TestTwoNodeProcessCrashRecovery` 使用 SIGKILL 强杀测试自己启动的接收端，确认它没有正常退出记录，再验证重启后恢复。强杀发生在首轮 Call 完成且接收日志已输出后，覆盖既有连接突然消失；不代表在途业务执行结果、排队日志或持久化数据已获得崩溃安全保证。断电、网络分区和跨机器故障不在这两个测试的覆盖范围。

## 观察什么

- 接收端创建名为 `echo` 的示例 Service，名称映射由应用闭包提供；不把跨重启无效的 ServiceID 写进配置。
- 调用端不监听，也没有本地 NodeID；配置中的 `battle` 只是远端路由名。
- 调用端使用 Timer Command 发起一轮 Send 和 Call；框架通过 ServiceContext 自动绑定当前 Service，业务不创建 Client 或 goroutine。
- `Send admission` 的 error 为空只表示本地接受，不能据此认定对端已经处理。接纳后的网络故障由同一个 Logger 中的 Cluster failure 记录。
- `Call result` 记录本轮 OperationID、回复或错误。等待只阻塞当前 Service，不阻塞整个 Runtime。单次 Call 上限为 2 秒，结束后等待 3 秒再发起新的操作。
- 接收端 `received` 记录实际处理的命令与参数；本示例不消费 Diagnostics 再转写日志，避免重复。

## 手动故障演练

1. 保持调用端运行，停止接收端，观察后续调用错误。调用端应继续运行。
2. 使用原配置重新启动接收端，观察后续新操作恢复成功；旧请求不会自动重放。
3. 先停止调用端，再停止接收端，检查两份日志末尾的 `process stopped`。

每次运行的 OperationID 只是示例 Service 内的序号，不保证跨进程重启唯一，不用于可靠重试或幂等协议。接收端端口占用时启动直接失败；换端口需同步修改两端 nodes 表中的 battle 地址，listen 名称不变。

配置解析和启动信号属于应用层。日志目录须由该进程独占；不要让两个示例进程使用同一个目录。所有初始化与退出错误会写入 stderr 并产生非零退出码；文件模式下正常业务日志不输出终端。

关闭顺序为 Cluster.Quiesce → Runtime.Close → Cluster.Close → 退出日志 → Logger.Close。样例每轮调用有限等待；它不提供强杀不返回的自定义 Handler/Writer、生产守护、认证或 SLS 采集配置。
