# 基于 GSR 开发独立业务项目

本文面向依赖 GSR 的业务仓库，按已发布的 `v1.0.0` API 编写。业务项目负责游戏规则、协议、持久化和部署；GSR 负责 Service 调度、消息、生命周期及接入基础设施。框架仓库的 `AGENTS.md` 包含 Core 开发和 Skynet 对照流程，不应整份复制到业务项目。

## 1. 建立独立模块并固定依赖

在业务项目目录执行，模块路径替换成自己的仓库地址：

```sh
go mod init example.com/mygame/server
go get github.com/cnooljaw/gsr@v1.0.0
```

需要 Go 1.24 或更高版本。提交 `go.mod` 与 `go.sum`，CI 使用明确的 Go 工具链版本。生产依赖固定版本，不长期追随 `@main`；升级时记录版本、API/错误代码/参数类型变化，并验证业务自己的集成链路。当前标签不等于 `main` 最新提交：`v1.0.0` 定位发布基线，后续教程和测试补充在 `main`。

本地联调可以临时指定 GSR checkout：

```sh
go mod edit -replace=github.com/cnooljaw/gsr=../gsr
```

该相对路径只适用于业务项目与 GSR 目录同级的情况；路径按 `go.mod` 所在目录解析。联调结束执行以下命令，再确认实际依赖版本：

```sh
go mod edit -dropreplace=github.com/cnooljaw/gsr
go mod tidy
go list -m github.com/cnooljaw/gsr
```

不要把开发机绝对路径的 `replace` 提交进正式依赖。需要多模块联调时也可使用本地 `go.work`，但它不能代替正式 `go.mod` 的版本锁定，CI 不应依赖未声明的本机工作区。

## 2. 目录与文档职责

以下是起步布局，只创建当前功能实际需要的包：

```text
server/
├── AGENTS.md
├── go.mod / go.sum
├── cmd/server/main.go             # 配置、信号、外部资源与 gsr.Run
├── internal/application/         # MainService 和业务组装
├── internal/player/              # 玩家 Service、命令、调用方法与测试
├── internal/room/                # 房间 Service、规则与测试
├── internal/protocol/            # 业务 proto、生成目录与客户端协议
├── internal/storage/             # 确有需要的持久化适配
├── CONTEXT.md                    # 玩家/房间/请求/版本等业务术语
└── docs/
    ├── ARCHITECTURE.md           # 状态 owner、节点职责和依赖方向
    ├── GSR-INTEGRATION.md        # 固定版本、用到的能力和配置预算
    ├── PROTOCOL.md               # Command、客户端消息和错误契约
    └── GO-CODE-STYLE.md           # 项目采用的 Go 规则
```

`AGENTS.md` 只保留工作约束和文档入口；状态归属、协议和重大取舍写入项目自己的文档。尚未涉及的模块无需先建目录、通用接口或完整 Service 体系。

可复制 [业务项目 AGENTS.md 模板](templates/AGENTS.application.md) 到新仓库根目录，改名为 `AGENTS.md`。随后根据实际业务补齐文档和验证命令；模板中的 GSR 版本链接应随依赖升级同步调整。

## 3. 入口、Service 与状态归属

- `main` 负责配置加载、信号、外部资源和 `gsr.Run`；普通 MainService 负责创建、命名和发起业务。
- 普通业务实现 `gsr.Service`，使用 `gsr.ServiceContext` / `gsr.CommandContext`；`runtime.Service` 是底层集成接口。仍可导入 `runtime.CommandID` 和稳定错误，不需要直接持有 `*runtime.Runtime`。
- 创建成功的 Service 归 Runtime 管理，不随 MainService 退出而停止。`self.Exit()` 只请求当前 Service 退出，调用后立即返回；它不会结束 `gsr.Run`，也不会跳过后面的 Go 语句。
- 每份可变权威状态只有一个 Service owner。其它 Service 保存目标 ID/名称并发送 Command，不保存对方实现指针、不直接调用对方方法读写状态。
- Service 是运行单元。纯规则计算、数据转换、配置校验直接使用普通 Go 函数；不要为每个代码包创建 Service。
- ServiceID 只在本次 Runtime 生命周期有效；玩家 ID、房间 ID 等业务身份独立定义。持久化存业务 ID，不持久化 ServiceID。重建后重新解析名称或更新路由。

`ServiceContext` 和 `CommandContext` 仅在当前生命周期回调或 Handler 内使用，不能存到字段、闭包后台任务或全局“当前 Service”中。`self.Logger()` 和 `self.Cluster()` 已绑定当前来源，业务无需组装 Host/ClusterClient。

## 4. 最小可运行骨架

先把以下内容存为新项目的 `cmd/server/main.go`，执行 `go mod tidy`、`go run ./cmd/server`。它创建一个拥有计数状态的 Service，经消息写入和查询，输出 `count=7` 后继续等待 Ctrl+C。

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/runtime"
)

const (
	setCountCommand runtime.CommandID = 1
	getCountCommand runtime.CommandID = 2
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return gsr.Run(ctx, gsr.Config{}, &mainService{})
}

type mainService struct{}

var _ gsr.Service = (*mainService)(nil)

func (service *mainService) OnStart(ctx context.Context, self gsr.ServiceContext) error {
	id, err := self.CreateService(ctx, newCounterService())
	if err != nil {
		return err
	}
	if err := self.Register("counter", id); err != nil {
		return err
	}
	if err := self.SendName("counter", setCountCommand, int64(7)); err != nil {
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	value, err := self.CallName(wait, "counter", getCountCommand)
	if err != nil {
		return err
	}
	count, ok := value.(int64)
	if !ok {
		return fmt.Errorf("unexpected count result: %T", value)
	}
	if _, err := fmt.Printf("count=%d\n", count); err != nil {
		return err
	}
	self.Exit()
	return nil
}

func (service *mainService) Handle(_ gsr.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	return nil, runtime.ErrUnknownCommand
}

func (service *mainService) OnStop(_ context.Context) error {
	return nil
}

type counterService struct {
	dispatcher *gsr.Dispatcher
	count      int64
}

var _ gsr.Service = (*counterService)(nil)

func newCounterService() *counterService {
	service := &counterService{}
	service.dispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{
		setCountCommand: service.setCount,
		getCountCommand: service.getCount,
	})
	return service
}

func (service *counterService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

func (service *counterService) Handle(self gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return service.dispatcher.Handle(self, command, args...)
}

func (service *counterService) setCount(_ gsr.CommandContext, count int64) error {
	service.count = count
	return nil
}

func (service *counterService) getCount(_ gsr.CommandContext) (int64, error) {
	return service.count, nil
}

func (service *counterService) OnStop(_ context.Context) error {
	return nil
}
```

这是单文件入门骨架，后续按实际职责拆包。MainService 不提供业务命令，因此显式拒绝未知命令且无需 Dispatcher。Counter 的两个消息由同一调用方顺序投递，查询排在已接纳的写入之后；不需要 sleep，也不依赖 Send 返回时写入已经完成。`gsr.Config{}` 使用有限默认值，部署前按实际负载确定容量和超时预算。

2026-10-13 已将此代码放入临时的独立 Go module，以本地 replace 指向从 `v1.0.0` 标签导出的源码；gofmt、依赖整理与编译通过，实际输出 `count=7`，入口退出后节点继续运行，SIGTERM 后退出码为零。该演练复用现有依赖缓存，未验证从远端首次下载模块，也未运行全仓 Go 测试。

## 5. Command、参数与调用约束

普通业务 Service 在构造时用具名字段持有 `*gsr.Dispatcher`，用静态 `CommandID -> typed Handler` 表绑定已实现命令；必须显式实现 `Handle`，不匿名嵌入 Dispatcher。Handler 第一参数是精确的 `gsr.CommandContext`，后续参数固定；返回 `error` 或 `(结果, error)`，不使用 variadic Handler。静态表可用 `MustDispatcher`，外部动态输入的构造错误应返回 error。

核心角色在类型定义后紧跟接口断言。客户端 Agent 通常同时声明：

```go
var (
	_ gsr.Service              = (*playerAgent)(nil)
	_ gsr.ClientMessageHandler = (*playerAgent)(nil)
)
```

接口断言按真实方法集使用指针/值形式，不为通过断言修改 receiver，不集中到文件末尾或 `interfaces.go`；不机械补齐所有工具接口。

### 本地参数所有权

本地消息只浅拷贝 `Args` 外层，不自动深拷贝 map、slice、pointer。发送前建立本次消息专用快照，嵌套数据按实际结构复制；成功接纳后不再修改该快照。接收方将 Args 当只读输入，需要保存/修改时复制到自己的状态。Call 结果同样不能暴露 Service 内部可变状态，返回值应是标量、不可变值或独立快照。

常用跨模块调用可提供普通 typed 调用函数/小 client，集中参数装配、快照和结果校验。持有 `self` 的调用对象只在当前回调中使用，不能长期缓存上下文；不为一次调用建立通用 RPC 框架。

### 远程参数边界

| 发送值 | 默认 Cluster 接收类型 |
| --- | --- |
| 有符号整数 | `int64` |
| 无符号整数 | `uint64` |
| 有限浮点数 | `float64` |
| string / bool / nil / `[]byte` | 对应原类型 |
| 普通切片、定长数组 | `[]any` |
| 字符串键 map | `map[string]any` |

本地调用不编码，Dispatcher 不自动转换。准备同时供本地和远程调用的 Handler，调用方和接收方统一用规范类型；`type PlayerID uint64` 的类型名不会跨网保留。业务 struct 需展开、转换成规范 map 或自行编码为字节，结果也须可编码。集合元素和字段仍由业务校验。

完整参数、nil/空集合、大小限制与双节点演练见 [Cluster 教程](CLUSTER-CALL-TUTORIAL.md)。节点路由与 Service 名由业务配置；固定 Command 的参数顺序、类型和错误代码属于业务协议，升级时不得静默改变。

### Send、Call、Timer

- Send 的 nil 只表示接纳；需要业务成功结果时用 Call。队列满要显式拒绝、降载或按业务决定重试，不无限扩容或循环重试。
- Call 设置业务等待预算；实际等待受 Runtime/Cluster 上限约束。等待时当前 Service 不处理下一条消息，避免 A 等 B、B 再等 A 的依赖环。self Call 被拒绝，不假设框架能发现所有跨 Service/跨节点环。
- 超时/断线不等于目标未执行。写操作的幂等键、事务、重试和补偿属于业务；Cluster 恢复连接不自动重放请求。Handler 通过 return 完成 Reply，不手工或异步回复。
- 周期任务使用一次性 `self.After` 投递 Command，在 Handler 完成后按需求重新登记。Timer 遵循普通 admission，Source 为零；取消、重复调度、过期与失败处理由业务明确规定，不在 Timer 回调或 goroutine 中改状态。

## 6. I/O、资源和接入边界

| 资源/能力 | 推荐 owner 与约束 |
| --- | --- |
| Runtime、Cluster、框架 Logger | `gsr.Run` 统一收尾；业务 Logger 从 `self.Logger()` 取得，不能自行 Close |
| 玩家/房间状态 | 对应业务 Service；跨 Service 只能消息访问 |
| Redis/MySQL 原生客户端 | 宿主或明确资源 owner 创建，作为借用依赖注入；先结束使用者再关闭，不能多个 Service 重复 Close |
| Service 私有资源 | 明确 OnStart/OnStop 契约；部分启动失败也要能清理，借用资源不关闭 |
| HTTP | `self.OpenHTTP` + `*gsr.HTTPIngress`；HTTP Handler 可以保存该窄入口，不能保存 ServiceContext，鉴权/参数/错误映射由业务负责 |
| Gate / Protocol / Agent | Gate 管字节连接，Protocol 管 Envelope 编解码，Agent 组合二者；权威玩家/房间状态由业务 Service 管理 |

Service 内同步 I/O 会阻塞该 Service，必须使用有限 context 和客户端 I/O 超时。不要从 OnStart 保存 context 供未来 Handler 使用。真正需要外部并行工作时，建立有固定上限的基础设施 worker owner，完成后通过受控入口投递 Command；任务只携带输入快照和关联信息，不持有/调用 ServiceContext、不直接写业务状态。状态 owner 验证业务 ID、RequestID/Revision 等关联条件，丢弃迟到或失效结果。没有实际并发需求时不要先建 worker 框架。

Go 不能强杀不返回的 Handler、OnStart、OnStop；退出超时不是资源已经释放的证明。启动失败只回滚当前 Service，已经创建成功的子 Service 不自动撤销；一次业务组装需回滚时由组装 owner 明确补偿，整体进程启动失败则由宿主进入停机。

客户端 Agent 的普通命令与客户端入口分别显式实现 `Handle` / `HandleClientMessage`，使用具名 `commandDispatcher` / `protocolDispatcher`。业务 `.proto` 和生成目录归业务项目；`protocol.NewRegistry` 已包含框架通用目录，不能占用通用 ErrorResponse 编号 9001。客户端 `msg_id`、`request_id` 与内部 `CommandID`、Call SessionID 是不同协议空间。

Gate 的 MAC/seq 不代替玩家 token 鉴权。Plain 只用于回环/私有网络；公网使用 WSS，配置连接数、帧上限及每连接出站预算。Cluster 当前用于可信网络，NodeID 与远程 Source 都不能作为认证依据；跨节点 Source 为零，需要身份时显式携带并校验业务字段。具体接线见 [WebSocket 示例](../examples/websocket-gate/README.md)、[HTTP 示例](../examples/http-inbound/README.md)、[Redis 示例](../examples/redis-user-cache/README.md)和 [MySQL 示例](../examples/mysql-product/README.md)。

## 7. 错误、日志和 Go 编码

- 正常业务拒绝使用稳定 Domain `runtime.StatusError`，例如 `room.full`；不要使用框架保留的 `runtime.` 前缀。调用方用 `errors.Is` 或 Class/Code 判断，不比较 Message。未分类的普通 error 对调用方会转成安全的 HandlerFailed，内部原因留在本地诊断。
- 预期失败不 panic；静态接线的编程错误可使用明确命名的 Must 构造。错误包装用 `%w` 增加定位信息，多个清理错误按明确策略聚合，不到处重复记录同一个错误。
- 业务日志用当前 `self.Logger()`；不要记录 token、密钥或完整敏感载荷。日志接纳也可能满载/失败，按操作语义决定处置，不能把关键业务审计只寄托于有界诊断流。
- `gofmt`；receiver 名表达自身类型，常用 `service` / `agent`，不叫 `self`。`context.Context` 用 `ctx`，Service/Command 上下文用 `self`，未使用参数用 `_`。
- 构造器建立依赖不变量；接口由使用者定义且保持窄，不为抽象本身增加 interface、泛型层或包装 Service。
- Go 自然语言注释尽量中文，导出符号写有契约价值的 Go doc；测试前注释解释保护的可观察行为。机器指令、许可证和必要协议术语保持原义。

完整参考见 [Go 编码规范](GO-CODE-STYLE.md)和 [API 契约](API-DESIGN.md)。采用其中的规则时在业务项目自己的编码文档中记录，框架内部的组件实现规则无需搬进业务仓库。

## 8. 测试和上线前验证

先覆盖业务增量：状态变化和拒绝条件、消息快照、命令接线、结果/错误转换、超时后的幂等或补偿，以及自己拥有的资源关闭。纯规则测试可以直接调规则函数；验证 Service 的路由、来源或生命周期时走真实消息入口，不以直接调用 Service 方法代替集成检查。

优先扩展已有用例，按改动选择验证：定向测试 → 受影响 package → 跨包/发布时全仓 test/vet；并发、生命周期或共享资源切片收尾跑相关 race。不在每次小编辑后重复全仓测试，也不习惯性用 `-count=1` 清空缓存。纯文档只检查 diff 和链接，实际可运行代码片段应编译/演练。

发布前另行验证实际网络、Redis/MySQL、WSS、日志磁盘、队列拒绝、进程停机与恢复，记录环境、负载和容量。GSR 开发机测试通过不能替代业务项目的目标部署验收。

新项目完成第一个纵向切片时，应能回答：

1. 每份权威状态归谁，节点/Service 重建后如何找到它？
2. Command 的固定参数、结果和错误代码是什么，本地与远程是否一致？
3. 队列满、Call 超时、迟到结果和重复请求如何处理？
4. 每个资源谁创建、谁关闭，部分启动失败怎样清理？
5. 入口鉴权、日志与业务审计在哪里，哪些数据不应进入日志？
6. 已运行哪些测试和部署演练，哪些能力尚未验证？

## 9. AGENTS.md 的使用方式

模板用于约束后续 Agent 的默认行为，不替代业务契约。进入新项目先读本地文档、确定实际依赖版本，再核对对应版本的 GSR 文档/代码。涉及业务协议、状态所有权或生命周期的改变先明确预期并更新契约和测试；普通局部实现细节无需逐条上升为架构裁决。

业务项目把 GSR 当依赖；发现框架问题时整理最小复现和版本信息。在业务任务中不顺手修改依赖缓存、vendor 或旁边的 GSR checkout；需要框架修复时明确范围，单独验证和升级。提交与推送按项目授权规则处理，写指南不代表获得后续提交或推送授权。
