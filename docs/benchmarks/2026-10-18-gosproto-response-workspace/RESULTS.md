# RPC 响应的单缓冲工作区

2026-10-18。上一轮私有header优化已本地提交为 gosproto `f9de3d2` / GSR `dbb681f`，提交后两库干净，未推送。本轮只融合声明了body的ResponseEncode，源码和记录未提交/发布；正式依赖仍v0.4.0，Decode冻结。

## 修改与不变量

原响应先Encode分配body，再packRpcFrame分配完整输出并复制body。本轮把已验证的私有header字节传给现有写出函数，body完整预检后，直接写入最终Pack工作区尾部，再从前部压缩。省掉中间body分配及复制。通用写出只增加私有字节prefix参数；公开Encode/EncodePacked传nil，不引入公共前缀API、消息包装或协议模型。

设body预检尺寸为B、header长度为P，S=P+B，C=packedBufferSize(S)，H=C-S。最终输出与原RPC相同容量C，out[H:H+P]复制header，out[H+P:]直接编码body，Pack读取out[H:]。原FF增长余量与八字节游标间隔按完整S计算，继续保护未消费输入。这个复用只发生在本次调用独占的缓冲里，不暴露借用结果。

- header预算独立验证，body仍经过原normalizeLimits/getbase/schema/encodedSize和完整字节、深度、元素预检；不把header作为body字段或嵌套层计入预算。body恰到8 MiB边界仍按原行为接受。
- 当前唯一非nil前缀来自固定私有header，长度至多14 B（响应只有session），源存储是调用内22 B栈缓冲；编译器确认prefix及encodedHeader不逃逸，输出不别名prefix。
- ResponseEncode的session/name/type判断顺序不变。合法内部header的类型和默认预算保证其准备不会失败，因此提前在栈上准备header不改变body错误的对外优先级；body失败仍返回nil，不暴露部分帧。
- 无body响应仍走原packRpcFrame，忽略response参数的行为保持；仅声明了body的响应使用融合入口。
- RequestEncode方法、session登记、锁和重复session返回已编码body的路径逐字节不变。其共用header函数被提取、普通body写出传nil前缀，实际成本也做测量。
- Decode/Dispatch、Pack/Unpack算法、wire/cap、输入/旧输出独占、元信息和TS生产源码不变；没有池、缓存、unsafe、generator或新依赖。

## 测试增量及纠正

原1008组RPC角色/header/长度、2016输出及39官方C/Lua头向量继续保护精确wire/cap与独占。复用原body预算边界及错误/session用例；补3种session下的typed nil响应，必须返回ErrNil与nil输出，避免先准备header后误发部分帧。

新增typed nil用例起初误认为会编码空结构，但现有getbase明确返回ErrNil。失败保存于checks.json，只纠正测试预期并移入既有非法输入用例，生产行为未改；修正后的用例在旧实现也通过。

既有分配用例将声明body响应与同构建EncodePacked对照：旧实现2对1按预期失败，候选普通/race/checkptr通过；通知与有响应请求仍按原两缓冲对照保护。大型消息保留布局记录，不承诺所有响应只有一次总分配。

原RPC旧新差分探针保持不变，固定3078种子/12312帧（type/session/全mask、尾块、FF邻域）通过，包含请求/通知控制与有/无body响应。通用Pack/Codec各10秒fuzz，分别136238/220094次，记录后段无新增计数的原始输出，没有为改善数字重试。

## 两轮真实RPC与共用写出成本

沿用原19项真实RPC探针：5个声明body响应是目标，14个请求/通知观测共用函数的间接成本。self、clone各一次控制，固定候选两次；另一次原19项普通body/Pack/手工帧，因nil-prefix共用写出也变更，不能把这些路径称为源码完全未变。六轮×四个ABBA/BAAB四元组、10 ms目标块、GC开启，Apple M2、Go1.24.13、GOMAXPROCS=1。

总计 **9120块、2280四元组、570配对轮次**，逐块独立复算到1e-12相对容差一致；没有重试或调参。负百分比为耗时减少，取六轮配对比值中位数，不是置信区间。

| 声明body的响应 | 第一轮 | 第二轮 | B/op 前 → 后 | allocs/op 前 → 后 |
| --- | ---: | ---: | ---: | ---: |
| addressbook/response | -7.45% | -8.32% | 304 → 160 | 2 → 1 |
| game_small/response | -6.40% | -7.78% | 704 → 352 | 2 → 1 |
| game_large/response | -7.58% | -6.04% | 7424 → 3968 | 3 → 2 |
| int32_edges/response | -14.79% | -12.54% | 8192 → 4096 | 2 → 1 |
| echo_ascii4k/response | -43.90% | -43.04% | 9728 → 4864 | 2 → 1 |

两轮5项响应都更快且少一次分配；大型保留一次布局记录分配。ASCII4KiB约快43%～44%，整数数组约快13%～15%，大型游戏约快6%～8%；这些是ResponseEncode收益，不能推广成RequestEncode或完整业务往返收益。

| 其他真实RPC路径 | 第一轮 | 第二轮 |
| --- | ---: | ---: |
| addressbook/notification | +1.30% | +0.98% |
| addressbook/request_cancel | +0.21% | -0.22% |
| game_small/notification | +0.10% | +1.10% |
| game_small/request_cancel | +0.51% | -0.47% |
| game_large/notification | -0.91% | -0.69% |
| game_large/request_cancel | -1.18% | -0.45% |
| int32_edges/notification | +0.29% | +0.89% |
| int32_edges/request_cancel | -0.50% | +0.68% |
| echo_ascii4k/notification | -0.91% | +0.73% |
| echo_ascii4k/request_cancel | -0.66% | -1.51% |
| ascii32/notification | +1.10% | -0.18% |
| utf8_1k/notification | -0.52% | +0.31% |
| ascii4k/notification | -0.75% | -0.99% |
| zeros4k/notification | +0.24% | +0.15% |

这些14项分配不变。AddressBook通知两轮约慢1.30%/0.98%，每轮两臂ns/op几何均值之差的中位数约9/7 ns，作为可见代价保留；其余计时存在正负波动，不把未针对的路径小幅下降解释成新算法收益。

原19项nil-prefix普通body/Pack/手工帧中位差范围为−2.86%～+1.33%，分配次数不变；整数数组PEncode约+1.03%、ASCII body Encode约+1.23%，Pack源码未变也出现约+1.3%，这些细微差值保留但不单独归因。全部数据见business/及joined.json。

## 控制与取舍

self 原工具总控制通过 17/19；未通过：game_large/response（中位差 +0.38%，IQR 5.00pp）、ascii4k/notification（中位差 -0.60%，IQR 3.18pp）。

clone 原工具总控制通过 16/19；未通过：game_large/notification（中位差 +0.64%，IQR 3.23pp）、int32_edges/request_cancel（中位差 -0.44%，IQR 3.37pp）、echo_ascii4k/response（中位差 +1.57%，IQR 4.47pp）。

保留实现：两轮5项真实响应均改善，确定少一次分配和整body复制；共用路径分配不增加，约9/7 ns的通知代价与控制波动透明披露，维护成本只增加私有字节前缀和提取已有header写出。没有采用统一最低加速比例，也不将AA未全过当成自动否决。请求路径不继续融合，后续若处理必须保护body错误与session/锁的原有语义。

## 验收和复现

最终库普通全测（启用固定官方C/Lua）、race+checkptr=2、vet、两项有限fuzz、3078固定RPC差分、临时modfile下GSR protocol/sproto/debugserver/benchmarks三包及Go/TS Plain/MAC 7/7、零跳过全部通过。最终库Go源码与采样候选逐字节一致，四份归档/探针/夹具/工具重建一致。未运行GSR全仓/race、跨平台或新的PB/TS性能矩阵；没有推送/发布/正式依赖升级。

```sh
python3 -B docs/benchmarks/2026-10-18-gosproto-response-workspace/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/rpc-response-new
python3 -B docs/benchmarks/2026-10-18-gosproto-response-workspace/analyze.py tmp/rpc-response-new
```

输出目录必须不存在；--prepare-only仅重建四份源码而不计时。基线f9de3d2，candidate.patch包含全部库Go改动，CHANGES只记录发布事项。固定差分向prepare-only的pass1/harness复制rpc-fuzz.go.txt为rpc_fuzz_test.go后执行 go test -mod=mod -run=^FuzzRPCHeader$ . -timeout=1m。全部原始块、控制失败、测试纠正、命令及指纹保留；归档和二进制放忽略目录。
