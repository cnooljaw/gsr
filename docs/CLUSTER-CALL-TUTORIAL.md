# Cluster 节点间调用教程

本教程面向 GSR 1.0 的默认 ValueCodec，使用应用层 `self.Cluster().Send/Call`。先运行现成双节点示例，再看不同参数如何编码、远端 Handler 如何声明，以及返回值如何读取。

## 1. 先跑通两个节点

从仓库根目录构建：

```sh
go build -o /tmp/gsr-twonode ./examples/twonode
```

在两个终端分别执行：

```sh
# 终端 A：接收节点，创建并注册 echo Service，开放 battle 监听。
/tmp/gsr-twonode -config examples/twonode/server.json

# 终端 B：调用节点，通过节点表找到 battle；不需要开放自己的监听。
/tmp/gsr-twonode -config examples/twonode/caller.json
```

配置中的 `battle` 是节点路由名，`echo` 是目标节点内的 Service 名称，两者用途不同。示例把 `battle` 映射到 `127.0.0.1:19091`；跨机器时调用方填写接收端可访问的地址。应用层 `Open("battle")` 取该配置的端口并绑定 `0.0.0.0`，不是只绑定配置中的回环 IP。Cluster 用于可信网络，按部署配置控制访问范围。

调用节点每轮向 `echo` 发一次 Send 和 Call。查看日志：

```sh
tail -f log/battle-01/gsr.log
tail -f log/caller-01/gsr.log
```

`Send admission` 的空错误表示本地接纳，`Call result` 才包含远端回复或错误。两个进程用 Ctrl+C 有序退出。配置、信号、停机和恢复说明见 [双节点示例](../examples/twonode/README.md)。现成示例只演示 `uint64` 操作编号，不是全部参数类型的演示程序。

## 2. 接收端注册，调用端按名访问

接收端 MainService 在 `OnStart(ctx, self)` 中执行：

```go
id, err := self.CreateService(ctx, newEchoService())
if err != nil {
    return err
}
if err := self.Register("echo", id); err != nil {
    return err
}
if err := self.Cluster().Open("battle"); err != nil {
    return err
}
self.Exit()
return nil
```

先创建成功，再发布名称。MainService 退出后 Echo 和监听继续由运行环境管理。不要保存跨进程重启的 ServiceID；调用方按目标节点内的稳定名称访问。

调用端在自己的 `OnStart` 或 Handler 中使用当前 `self`：

```go
// 无需结果时使用 Send；参数必须符合目标命令的 Handler 签名。
if err := self.Cluster().Send("battle", "echo", notifyCommand, uint64(7)); err != nil {
    return err
}

wait, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()
value, err := self.Cluster().Call(wait, "battle", "echo", echoCommand, uint64(7))
if err != nil {
    return err
}
operation, ok := value.(uint64)
if !ok {
    return fmt.Errorf("unexpected Echo result: %T", value)
}
fmt.Printf("operation=%d\n", operation)
```

参数顺序是节点名、Service 名、CommandID、业务参数。`notifyCommand` 和 `echoCommand` 是双节点示例已有的命令编号。业务通过 `self.Cluster()` 取得来源绑定能力，不手工创建 ClusterClient，不持有 Runtime，也不在 Service 内启动 goroutine。

## 3. 参数类型如何变化

Send 与 Call 使用同一个参数编码规则；Call 的成功返回值也经过同一 Codec。下表中的“远端类型”同时适用于远端收到的参数和调用方收到的返回值。

| 发送的 Go 值 | 远端类型 | 示例 |
| --- | --- | --- |
| `nil` | `nil` | 一个明确的空参数 |
| `bool` | `bool` | `true`、`false` |
| `string` | `string` | `"hello"`、`"你好"`、`""` |
| `int`、`int8`、`int16`、`int32`、`int64` | `int64` | `int32(-7)` → `int64(-7)` |
| `uint`、`uint8`、`uint16`、`uint32`、`uint64` | `uint64` | `uint16(9)` → `uint64(9)` |
| 有限的 `float32`、`float64` | `float64` | `float32(1.5)` → `float64(1.5)` |
| `[]byte` | `[]byte` | 原始二进制，不变成整数列表 |
| 普通切片 | `[]any` | `[]int{1, 2}` → `[]any{int64(1), int64(2)}` |
| Go 定长数组 | `[]any` | `[2]string{"a", "b"}` → `[]any{"a", "b"}` |
| 字符串键 map | `map[string]any` | `map[string]int{"count": 3}` → `map[string]any{"count": int64(3)}` |
| 嵌套切片、数组、map | 递归应用上述规则 | map 内的 `[]int` 也变成 `[]any` |

需要特别注意：

- `[N]byte` 是数组，按列表解码，元素为 `uint64`；只有字节切片走二进制分支。
- 自定义具名标量和集合类型按底层类型编码，接收端不恢复类型名。例如 `type UserID uint64` 的值解码为普通 `uint64`。
- 整数保留有符号/无符号类别及完整 64 位范围，不先转换为浮点数。`float32` 扩为 `float64` 时保留其原有精度，不获得额外精度。
- nil 切片/map 不编码为独立的 `nil`：普通 nil 切片解码为空 `[]any`，nil map 解码为空 map。零长度 `[]byte`（无论发送端是否为 nil）在当前实现中解码为 nil `[]byte`。集合的 nil/空状态不能作为远程业务语义；需要区分时增加显式字段。

默认 Codec 的集合深度上限为 64，payload 上限为 4 MiB。Call 的整个参数列表本身也是一层集合。Cluster 默认完整 frame 上限也是 4 MiB，但它还包含帧头和路由等字段，因此业务 payload 应留出余量，不能把完整 4 MiB 当作可发送的业务字节数。

## 4. 多参数与“一个列表参数”

下面是三种不同的调用，`someCommand` 的 Handler 签名必须与对应形式匹配：

```go
// 两个位置参数；Handler 可以声明 (self, text string, count int64)。
value, err := self.Cluster().Call(wait, "battle", "params", someCommand, "hello", int32(3))

// 一个位置参数；Handler 声明 (self, items []any)。
value, err := self.Cluster().Call(wait, "battle", "params", someCommand, []int{1, 2})

// ... 展开后是两个位置参数，不再是一个列表参数。
args := []any{"hello", int64(3)}
value, err := self.Cluster().Call(wait, "battle", "params", someCommand, args...)
```

这些片段互为替代，不是连续调用同一个签名的命令。不能把 `[]int` 用 `args...` 展开到 `...any`；若确实要发送多个动态位置参数，先显式构造 `[]any`。应用 Dispatcher 的业务 Handler 不支持 variadic；使用固定参数签名，动态集合由一个 `[]any` 参数承载。

Call 可以没有业务参数：Handler 只声明 `gsr.CommandContext`。`Call(..., nil)` 则是一个参数，不能把它当成无参数调用。

## 5. 声明适合远程调用的 Handler

以下业务 Service 接收字符串、数字、列表和 map，并返回一个 map。它使用现有应用 Dispatcher，没有额外接口或包装层。可把它放入业务项目，复用双节点示例的配置与启动结构：

```go
package main

import (
    "context"

    "github.com/cnooljaw/gsr"
    "github.com/cnooljaw/gsr/runtime"
)

const inspectCommand runtime.CommandID = 0x04100308

type parameterService struct {
    dispatcher *gsr.Dispatcher
}

var _ gsr.Service = (*parameterService)(nil)

func newParameterService() *parameterService {
    service := &parameterService{}
    service.dispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{
        inspectCommand: service.inspect,
    })
    return service
}

func (service *parameterService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
    return nil
}

func (service *parameterService) Handle(self gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
    return service.dispatcher.Handle(self, command, args...)
}

func (service *parameterService) inspect(
    _ gsr.CommandContext,
    text string,
    count int64,
    items []any,
    meta map[string]any,
) (map[string]any, error) {
    // 参数由消息独占；这里仅返回它们，不暴露 Service 内部的可变状态。
    return map[string]any{
        "text": text, "count": count, "items": items, "meta": meta,
    }, nil
}

func (service *parameterService) OnStop(_ context.Context) error {
    return nil
}
```

接收端 MainService 用 `newParameterService()` 创建实例，将它注册为 `"params"`，再 `Open("battle")`；顺序与第 2 节相同。若在双节点示例中练习，可以添加该 Service，并在 server 分支替换 Echo 的创建和注册名称。

调用端可把原 `callerService.handleTick` 换成下面的一次调用；不再登记下一次 Timer，因此只演示一轮。它使用示例已经导入的 `context`、`fmt` 和 `time`：

```go
func (service *callerService) handleTick(self gsr.CommandContext) error {
    wait, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    value, err := self.Cluster().Call(
        wait, service.target, "params", inspectCommand,
        "hello", int32(3), []int{10, 20}, map[string]bool{"active": true},
    )
    if err != nil {
        return err
    }
    reply, ok := value.(map[string]any)
    if !ok {
        return fmt.Errorf("unexpected inspect result: %T", value)
    }
    fmt.Printf("text=%v count=%v items=%v meta=%v\n",
        reply["text"], reply["count"], reply["items"], reply["meta"])
    return nil
}
```

输出为 `text=hello count=3 items=[10 20] meta=map[active:true]`；`reply["count"]` 的动态类型为 `int64`，`reply["items"]` 为 `[]any`，`reply["meta"]` 为 `map[string]any`。调用仍发生在 Service runner 中，不在 Go main 里绕过 Service 来源。单次流程完成后运行环境继续等待退出信号。

`[]any` 的元素需要按业务规则检查，例如遍历时用 `item.(int64)` 的带 `ok` 形式验证，再显式组装业务所需的 `[]int64`；Dispatcher 只检查最外层参数类型，不替业务校验集合元素、范围或 map 字段。

### 本地和远程混用的注意事项

本地 Send/Call 不编码，所以本地发送 `int32(3)` 仍是 `int32`，无法匹配上面 Handler 的 `int64` 参数；远程发送它则会先规范化，能够匹配。普通 `[]int` 和 `map[string]bool` 同样只在远程变成规范集合。

若同一 Handler 同时供本地和远程使用，调用方统一使用规范类型：

```go
// 以下参数既能匹配本地 CallName，也能匹配远程 Cluster.Call。
count := int64(3)
items := []any{int64(10), int64(20)}
meta := map[string]any{"active": true}
```

不要将远程 Handler 的参数声明为 `int32`、`[]int`、`[2]string`、`map[string]int` 或具名 `UserID`，期待 Dispatcher 自动转换。它按可赋值关系绑定参数，不做数值转换或业务结构恢复；类型不匹配返回 `runtime.ErrInvalidCommandArguments`。

## 6. 结构体、返回值与错误

默认 ValueCodec 不直接支持 struct、指针（包括 typed nil 指针）、函数、channel、非字符串键 map、complex、uintptr，以及 NaN/Infinity。集合内部包含这些值也会被拒绝。

业务结构体可展开成位置参数，或转换成 `map[string]any`，或自行编码为 `[]byte`。例如 `time.Time` 不能直接传，约定使用 Unix 时间 `int64`；单位、范围、字段校验和幂等含义由业务定义。不要为了保留 Go 类型名而给默认 Cluster 增加命令级 Codec 注册机制。

返回值同样必须能编码：远端返回 `int(3)` 后调用方得到 `int64(3)`；返回业务 struct 不会自动变成 JSON，需先转换。错误返回走结构化 Runtime/Domain error 路径，不作为成功返回值编码。

| 情况 | 应如何处理 |
| --- | --- |
| 请求含不支持的值 | Send/Call 在请求入队前返回 Codec 错误，可用 `errors.Is(err, cluster.ErrUnsupportedValue)` 判断 |
| 位置参数数量或规范类型不匹配 | 远端 Dispatcher 拒绝；Call 返回 `runtime.ErrInvalidCommandArguments`，Send 的 Handler 失败归接收端诊断 |
| 业务拒绝 | 返回稳定的 Domain `StatusError`，Call 用 `errors.Is` 或 Class/Code 分支 |
| Call 等待超时或连接断开 | 处理调用错误；不要据此推断远端一定未执行，不自动重试有副作用的操作 |
| Send 入队后的网络失败 | 进入调用端基础设施诊断；Send 已返回的 nil 不会变成异步业务回复 |

Cluster 配置 `MaxCallWait` 默认 5 秒，调用方 context 只能缩短。等待期间当前 Service 不处理下一条消息；回复由 Runtime 自动完成，业务不手工 Reply。业务在 B 节点得到的 Core Source 是零，不能用它当跨节点玩家或 Service 身份；需要业务身份时显式携带并校验业务字段。Source 的完整边界见 [API 契约](API-DESIGN.md)。

## 7. 当前测试覆盖到哪里

GSR 1.0 已实现上述参数集合。原有 Codec 测试覆盖字符串、整数、浮点数、字节串、混合列表、嵌套 map、非法类型以及深度/payload 限制；原有真实 TCP 往返用例主要是 `"hello" + int32(3)`。本教程切片在这些现有测试中补充了集合和边界参数，没有改动生产实现。

| 验证层级 | 用例 | 覆盖范围 |
| --- | --- | --- |
| 默认 Codec | [TestValueCodecRoundTripsSupportedValues](../cluster/value_codec_test.go) | 标量、Unicode/空字符串、64 位整数极值、类型化切片、定长数组、字节数组、typed map、嵌套集合、nil/空集合 |
| 错误与输入限制 | [ValueCodec 拒绝与限制用例](../cluster/value_codec_test.go) | 不支持的值、非有限浮点、深度与 payload 上限、坏载荷与过大 map 数量 |
| 真实 TCP + Core Service | [TestTCPClusterRoundTrip](../cluster/tcp_integration_test.go) | 多位置参数、无参数、独立列表参数、标量混合、64 位整数极值、字节串/数组/map/嵌套集合、nil/空集合的请求和结果往返，以及领域错误 |
| 应用 Dispatcher | [Dispatcher 测试](../dispatcher_test.go) | 强类型绑定、参数数量/类型错误、nil 绑定、非法 variadic Handler 和返回值/error 行为 |
| 应用启动与命名路由 | [Run 测试](../run_test.go) | 同进程两节点的来源绑定、名称隔离、Service 重建与停止；不重复全部 Codec 参数矩阵 |
| 双进程 | [双节点恢复测试](../examples/twonode/process_test.go) | 独立进程的正常停机/强杀与重启恢复，示例业务参数为 `uint64` |

真实 TCP 的参数矩阵使用现有 Core Echo 测试 Service，按原样回传解码后的 Args；它不代表每种业务 Handler 都已验收，也没有把所有参数组合重复到应用层和双进程层。

从仓库根目录运行参数专项验证：

```sh
go test ./cluster -run '^(TestValueCodecRoundTripsSupportedValues|TestValueCodecRejectsUnsupportedValues|TestValueCodecEnforcesDepthAndPayloadLimits|TestTCPClusterRoundTrip)$' -v -timeout=2m
```

2026-10-12 本轮 `go test -race ./cluster -timeout=2m` 已通过。第 5 节的 Service 和调用代码也已与双节点启动结构一起编译，在两个独立进程间通过真实 TCP 得到上述输出，并有序退出；这是教程示例的实际演练，没有新增一套重复的持久测试夹具。

需要独立进程恢复演练时，再使用双节点 README 中的显式门禁。上述验证不替代业务 schema 校验、目标部署容量或跨机器故障验收。
