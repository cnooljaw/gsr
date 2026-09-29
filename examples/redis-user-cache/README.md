# Redis Hash 用户信息缓存

这是一个 GSR 业务 Service 调用 Redis 的最小示例。`MainService` 创建 `userProfileService` 并向它 `Call`；`userProfileService.Handle` 在自己的 runner 中直接调用 `userHashCache`。缓存对象是普通依赖，不是另一个 Service。`userHashCache` 用 `user:<ID>` 作为键，以 Redis Hash 的 `name`、`level` 字段保存用户信息快照；缓存不是权威数据源，也不负责从数据库加载或自动回填。

从仓库根目录运行：

```sh
go test ./examples/redis-user-cache
```

实际 Redis 集成测试会启动本机的 `redis-server`，使用临时目录和回环端口，结束后回收进程；未安装时跳过集成用例。[`service.go`](service.go) 是 Service 内的 Redis 调用，[`service_test.go`](service_test.go) 展示 `MainService.CreateService/Call` 的完整运行路径。`cacheUserCommand` 保存快照，`loadUserCommand` 返回 `userLookup{Found: false}` 表示未命中；Redis 故障和损坏的 Hash 分别返回稳定错误，不当成未命中。

本仓库使用 Go 1.23.3，因此依赖固定在仍支持该版本的 `go-redis/v9.18.0`；不为示例升级全仓 Go 版本。示例以 Redis 6.0 及以上为运行基线，只使用该版本已有的 `DEL`、单字段 `HSET`、`EXPIRE` 和 `HGETALL`，不依赖 Redis 8 专属命令。

宿主先创建依赖，并在 `gsr.Run` 返回后关闭 Client：

```go
client := redis.NewClient(&redis.Options{
    Addr:                  "127.0.0.1:6379",
    ContextTimeoutEnabled: true,
    DialTimeout:           time.Second,
    ReadTimeout:           time.Second,
    WriteTimeout:          time.Second,
})

cache, err := newUserHashCache(client, 5*time.Minute, 300*time.Millisecond)
if err != nil {
    return err
}
// 将 cache 注入 MainService；gsr.Run 返回后处理 client.Close() 的结果。
```

在 `MainService.OnStart(ctx, self)` 中，调用形状如下（`cache` 由应用创建并注入）：

```go
id, err := self.CreateService(ctx, &userProfileService{cache: cache})
if err != nil {
    return err
}
_, err = self.Call(ctx, id, cacheUserCommand, userInfo{ID: 42, Name: "小明", Level: 7})
if err != nil {
    return err
}
value, err := self.Call(ctx, id, loadUserCommand, uint64(42))
if err != nil {
    return err // Redis 故障或缓存损坏，不是缓存未命中。
}
lookup := value.(userLookup) // lookup.Found 为 false 才表示未命中。
```

应用持有 `*redis.Client`，在 GSR 收尾后关闭它；Service 与缓存对象均不关闭借用的 Client。创建 Client 时须启用 `ContextTimeoutEnabled`，否则此版本的 `go-redis` 不会用每次调用的 context 截止时间约束连接 I/O。缓存的每次 `Put/Get` 还设置了更短的最长等待。`CommandContext` 不是 `context.Context`；调用方 `Call` 的截止时间不能强杀正在执行的 Handler，因此 Redis 自身的有限超时必不可少。等待期间只阻塞当前 `userProfileService` 的 runner，不阻塞其它 Service。

调用方根据自己的业务来源处理未命中和故障，不把“Redis 连不上”当成“用户不存在”。`Put` 用同一 Redis 事务替换整个 Hash 并设置 TTL；一次写入在超时或断线后可能已经由 Redis 执行，不能仅凭返回错误判断写入未发生。这里只有可重建的缓存快照，不承诺权威数据写入。示例不新增 `RedisService`、Runtime API、自动回填或跨节点一致性规则。

示例只存昵称和等级，不存密码、token 等敏感字段。正式部署的 Redis 地址、认证、TLS 和访问范围由应用配置管理；测试用的无认证回环 Redis 不代表生产部署方式。
