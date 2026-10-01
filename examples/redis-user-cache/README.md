# GSR Service 调用 Redis Hash

这个可运行示例展示：`mainService` 用 `CreateService/Call` 访问业务 `userProfileService`，后者用 `gsr.Dispatcher` 绑定强类型 Handler，并在自身 Handler 中同步调用普通的 `userHashCache`。缓存对象不是另一个 Service，也不是权威用户数据源。

从仓库根目录启动一个隔离的测试 Redis（终端一）：

```sh
redis_test_dir=$(mktemp -d)
LC_ALL=C redis-server --bind 127.0.0.1 --port 6380 --save '' --appendonly no --dir "$redis_test_dir"
```

然后运行示例（终端二）：

```sh
redis-cli -h 127.0.0.1 -p 6380 PING
# PONG
go run ./examples/redis-user-cache -redis-addr 127.0.0.1:6380
# user 42: 小明 (level 7)

redis-cli -h 127.0.0.1 -p 6380 TYPE gsr:example:user:42
# hash
redis-cli -h 127.0.0.1 -p 6380 HGETALL gsr:example:user:42
# 包含 name=小明、level=7；字段顺序不保证固定
redis-cli -h 127.0.0.1 -p 6380 TTL gsr:example:user:42
# 剩余秒数；过期后为 -2
```

终端一按 Ctrl+C 停止测试 Redis。示例只写入 `gsr:example:user:42`，TTL 为一分钟；过期后 `TYPE` 返回 `none`，`HGETALL` 返回空结果。若端口 6380 已被使用，以上命令中的端口要一起换成其它空闲端口。不要将无认证的测试启动方式用于生产部署。

[`main.go`](main.go) 使用框架的 `redisclient.New` 创建带有限超时的原生 `*redis.Client`，并在 `gsr.Run` 返回后关闭它；[`main_service.go`](main_service.go) 是普通应用入口；[`service.go`](service.go) 在业务 Handler 中调用 [`cache.go`](cache.go)。Client 由宿主持有，Service 和缓存对象都不关闭它。无需额外的 `RedisService`、Runtime Redis API 或 worker。

`cacheUserCommand` 保存可重建的快照；`loadUserCommand` 用 `userLookup{Found: false}` 表示未命中。Redis 故障与损坏的 Hash 分别返回稳定错误，不伪装成未命中，也不向调用者暴露底层错误文本。示例没有数据库加载、自动回填或跨节点一致性规则。

Client 启用 `ContextTimeoutEnabled`，缓存每次操作另设 300ms 最长等待。调用方 `Call` 的截止时间不能强杀已运行的 Handler；Redis 等待期间只阻塞当前 Service 的 runner，不阻塞其它 Service。示例只使用 Redis 6.0 起已有的 `DEL`、单字段 `HSET`、`EXPIRE` 和 `HGETALL`，不依赖 Redis 8 专属命令。

测试会自行启动、关闭隔离的临时 Redis，不依赖上面的 6380 端口：

```sh
go test ./examples/redis-user-cache
```
