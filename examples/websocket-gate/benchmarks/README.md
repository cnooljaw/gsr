# 双协议编解码 Benchmark

从仓库根目录运行：

```sh
npm --prefix examples/websocket-gate ci
npm --prefix examples/websocket-gate run bench:protocol
```

已安装锁定依赖时不必重复 `npm ci`。脚本先构建 TS，再顺序运行 Go 和 Node 基准，默认产物在 `tmp/protocol-benchmark/`。指定输出目录：

```sh
npm --prefix examples/websocket-gate run bench:protocol -- /absolute/output/directory
```

Go 基准也可以单独运行：

```sh
GOMAXPROCS=1 go test ./examples/websocket-gate/benchmarks \
  -run '^$' -bench '^BenchmarkProtocolCodec$' -benchmem -benchtime=200ms -count=5 -timeout=2m
```

这里的 `-count=5` 用于性能采样；不改变日常测试保留缓存的规则。Node 基准在构建后运行 `node examples/websocket-gate/client/dist/protocol_benchmark.js`，标准输出为五轮原始数据及中位数 JSON。追加 `--case ascii32` 可单独复测 32 字节小包。

## 比较口径

`cases.json` 是两种语言共用的消息内容：32 字节 ASCII、1000 字节 UTF-8、4096 字节 ASCII 和 4096 字节零字节文本。每条消息均是现有 EchoRequest 的一个 text 字段，复用现有 Protobuf 生成物与 Go Sproto 生成物。`echo.sproto` 仅把现有 echo.request 的同一字段布局命名为 EchoRequest，便于 TS 单独测量 payload；TS 在计时前核对其应用帧与真实 SprotoEchoClient 输出一致。

每条消息测八项：Protobuf/Sproto × payload/frame × encode/decode。payload 不 pack；frame 按示例现有格式封装 type/session 或 msg_id/request_id，Sproto 包含 pack/unpack。输入 payload 对象复用，帧头对象每次构造；包含输出分配和新对象解码。Go 记录 ns/op、B/op、allocs/op；TS 记录 ns/op，不估算内存分配。

`cases.json` 的字节数和 SHA-256 固定编码结果。Go 和 TS 在计时前均验证签名及语义往返，遇到不一致立即失败。这些签名首先由 Go 现有生成类型编码建立，再由 TS 验证；它们是跨语言基准夹具，不替代独立官方 Sproto 对照。协议变更后应核对变更原因，不要为了通过基准直接重写签名。

不含网络、Gate、MAC、Dispatcher、业务 Handler、安全预检和 RPC 会话表。TS 的固定内联 package 头长 6 字节，解码后按这个已知长度切分 body；没有测通用路由。Go Sproto 含库本身的反射、资源预算与失败原子性成本；两种库也保留各自的字符串校验语义。不要把结果解释为协议格式的理论性能。

## 报告

[Go 当前基线与第三轮筛选](../../../docs/benchmarks/2026-10-16-gosproto-round3/RESULTS.md) 从 gosproto `6012b06` 重新采样；保留布局记录的内存收窄，解包展开/窗口和原生指针赋值候选未采用。完整主套件、配对复核、profile 与失败值分别记录，TS 实现冻结，正式 Go 依赖仍为 v0.4.0。

[数值 packing、字段查询与类型化构造](../../../docs/benchmarks/2026-10-15-sproto-round2/RESULTS.md) 是第二轮结果；保留与首轮及正式 v0.4.0 的对照。原有两套基准不改变调用方式，实际 pencode/attach 的缓冲融合收益单独测量。

[TS 长块与 Go 反射路径优化验收](../../../docs/benchmarks/2026-10-15-sproto-next/RESULTS.md) 保存相对 v0.4.0 的前后对照、两套完整五轮采样、分配代价、profile 和撤回的尝试。TS 修改在当前工作树；Go 使用独立库未发布优化源码及临时 modfile，正式依赖仍为 v0.4.0，复现时需按该报告指定源码。

[v0.4.0 正式依赖完整复测](../../../docs/benchmarks/2026-10-15-v0.4.0/report.md) 汇总 Go/TS × Protobuf/Sproto 的两套基准：每种语言 72 项、每项五轮，同时列出纯 body、packing、应用帧、wire 大小、Go 分配和采样波动。

[2026-10-15 本机结果](../../../docs/benchmarks/2026-10-15/report.md) 包含完整耗时、大小、分配、实现版本和原始数据。此基线仅覆盖 Echo 字符串，后续比较整数数组、嵌套结构或真实游戏消息时，应新增相同语义的夹具并分别报告。

## AddressBook、游戏数值与数组

新增独立消息体套件，保留上面的 Echo 应用帧套件：

```sh
npm --prefix examples/websocket-gate run bench:representative
npm --prefix examples/websocket-gate run profile:protocol
```

两条命令顺序执行，不要同时采集性能计时与 profile。无需启动服务器、访问网络或安装 Lua；普通运行使用已保存的生成类型和 wire 签名。

`representative.json` 为两端共用数据：官方 AddressBook；16 属性/8 技能/8 物品和 256 属性/64 技能/128 物品的两档游戏状态；896 个 int32 边界值；4 KiB ASCII 对照。游戏状态还包含整数状态位、正负坐标和字符串标签。Protobuf 的游戏数值数组显式 packed，负值采用 sint32；AddressBook 保留官方 proto2 presence。TS 参考 parser 使用顶层 PhoneNumber，Sproto 字段布局不变。

每组八项：PB Encode/Decode、SP Encode/Decode、单独 Pack/Unpack、Encode+Pack 与 Unpack+Decode。没有 Envelope/package，以分离编解码器与 packing 成本。两端先核对完整语义、raw/packed 签名与 padding，再热身计时。默认输出在 `tmp/protocol-representative/`；可同旧套件一样追加绝对输出目录。

Go 单独运行：

```sh
GOMAXPROCS=1 go test ./examples/websocket-gate/benchmarks \
  -run '^TestRepresentativeFixtures$' -bench '^BenchmarkRepresentativeCodec$' \
  -benchmem -benchtime=200ms -count=5 -timeout=2m
```

TS 构建后可单独做一致性检查：

```sh
node examples/websocket-gate/client/dist/representative_benchmark.js --mode check
```

额外能力探针记录 boolean 与混合正负 int64 数组的现状，失败时输出诊断，性能排名不纳入失败输入。两项在基线失败，当前修复后均通过；`correctness.json` 保存 20 组独立官方 raw/packed 向量，由现有 Sproto 客户端测试验证布尔、整数宽度与符号、稀疏字段及 packing 边界。游戏性能夹具继续使用原有整数状态和 int32 数组，不因修复而更换比较数据。

Profile 默认对 AddressBook/game_large 的六个独立步骤逐个采集，Go CPU 和全量分配用不同进程，TS CPU 和采样分配用不同时段。完整文件位于 `tmp/protocol-profile/`，汇总 JSON 保存 self/cumulative CPU、Go alloc_space/alloc_objects 及 V8 sampled heap 归属。TS 堆采样包含已回收对象，但不完整计入 Buffer 外部内存，不能等同精确 B/op。开销与采样归因限制见 [分析和优化方案](../../../docs/benchmarks/2026-10-15-representative/ANALYSIS-AND-PLAN.md)。

[新套件结果](../../../docs/benchmarks/2026-10-15-representative/report.md) 与前述 Echo 报告分开保存，避免将 body 与应用帧数据混为同一口径。

[优化实施与验收](../../../docs/benchmarks/2026-10-15-optimized/RESULTS.md) 保存修复后五轮数据、小包回退检查、profile 摘要及独立 Go 库提交。Go 优化已随 gosproto v0.4.0 发布，GSR 正式依赖已升级；当前可直接用正式依赖复现，报告保留当时的临时 modfile 采样来源。采集器会记录实际模块覆盖及 Go/TS 源码哈希，避免把本地优化结果误标为正式版本。Profile 可设 `GSR_PROFILE_SCENARIO=game_large` 只采大型游戏消息，默认仍采两组。

### 原生 API、Protobuf 对照与解包形态

原套件有意分开调用 encode/pack，Echo frame 也分别编码头和 body 后拼接，因此不会体现 pencode/attach 内部减少复制的全部收益。补充脚本复用上述五组 body 和四组 Echo 数据，不更改 wire 签名：

```sh
npm --prefix examples/websocket-gate run build
node examples/websocket-gate/scripts/benchmark-sproto-api.mjs /absolute/output/api
# 可选：同进程交错对比上一版已编译的 Sproto 模块。
node examples/websocket-gate/scripts/benchmark-sproto-api.mjs /absolute/output/api /absolute/baseline/sproto.mjs
```

每操作独立热身至少约 50 ms，五轮各约 100 ms；同操作的 PB/SP 前后版本紧邻采样，并在奇数轮整体反转顺序，减少版本整组计时的阶段偏差。结果保存模块、脚本、夹具指纹、采样顺序及运行环境。代表性消息同时测 Encode/Decode、Pack/Unpack、原生 PEncode/PDecode 与分阶段组合；PB Encode/Decode 作为同进程控制项。可选基线参数只用于 Sproto 前后对照，不更换 PB 依赖。

Echo 文本在计时外构造一次，PB 与两个 SP 版本共享同一份输入。值相同但独立 repeat 生成的字符串可能具有不同 V8 内部表示，不能据此计算编解码器变化；历史记录保持原样，后续以共享输入的采样为主。

RPC 使用已有 attach/dispatch、真实 C2S schema 和固定 session=1，覆盖同一个等待项，不包含外层客户端安全预检、会话分配、网络或 MAC。原生收发与手工组帧/固定 schema 解码分别计时，不能混用两种入口计算优化比例。内部解包缓冲在热身时已建立，原生解码数据代表热缓冲；冷启动、超保留容量和失败复用由回归与缓冲观测单独验证。

另测 46 种公开 unpack 数据形态：32 B–64 KiB 的 ASCII、零块、稀疏 word，512 B–4 KiB FF 前缀后接稀疏数据，重复短零串，以及零前缀后接 ASCII；重复形态最大约 129 KiB。Pack 单独测 14 种形态，包括每 word 一至七个非零字节的旋转 mask、确定性随机数据、短包，以及 FF 六/七字节 word 的连续合并与分段。它们专门检查字节分布和段状态带来的回退，不与 PB 编解码排名混合。有基线时共 250 项、五轮 1250 个样本。

需要定位 Pack 热点或 public/manual 路径分配时，先构建 TS，再单独执行：

```sh
node examples/websocket-gate/scripts/analyze-sproto-pack.mjs tmp/sproto-pack-analysis
node examples/websocket-gate/scripts/analyze-sproto-pack.mjs tmp/sproto-pack-before /absolute/baseline/sproto.mjs
```

该脚本保存代表性消息的 mask/非零字节数分布、显式 Buffer 工厂与 subarray 调用次数，以及独立的 Pack/PEncode 和解包 CPU profile。分布计数使用包装后的实例，profile 使用新实例；不与计时 Benchmark 并发运行。Buffer 调用观测不等于完整堆分配统计，profile 百分比也不能直接换算为绝对加速。

### Go 解包形态

Go 单独覆盖 18 种 Unpack 形态：32 B/4 KiB/64 KiB 零字节与 ASCII、每 word 一至七个非零字节的旋转 mask、FF 前缀接稀疏尾部、重复短零段和确定性随机输入。它们用于发现只测业务消息会漏掉的回退，不作为 PB 直接比较项，也不冒充与 TS 形态逐字节相同的跨语言夹具。

```sh
GOMAXPROCS=1 go test ./examples/websocket-gate/benchmarks \
  -run '^$' -bench '^BenchmarkUnpackShapes$' -benchmem -benchtime=100ms -count=5 -timeout=2m
```

比较未发布库时使用显式临时 modfile；固定同一份 Benchmark 分别编译前后二进制，按操作配对并交替先后。source、二进制和实际模块覆盖一起保存；固定次数复核与自动迭代全量分别报告。

### 结果文件入库规则

测试代码、共用夹具、官方 wire 向量和复现脚本纳入版本管理。正式 Benchmark 基线在 docs/benchmarks/ 下保留最终报告、环境与源码信息、最终原始采样，以及有分析价值的精简 profile 摘要；不保存每轮调试快照。

普通测试输出、分阶段中间采样、完整 pprof/cpuprofile/heapprofile 和测试二进制放在已忽略的 tmp/。需要跨机器保留时使用 CI artifact。独立 gosproto 的源码改动在该库提交，GSR 报告引用提交或版本，不重复保存源码补丁。

### 生成与官方对照

Benchmark schema 位于 `schema/`，仅服务于测试，不进入 GSR 业务协议目录。重生成需要本机 protoc、protoc-gen-go v1.36.1、已安装的 protoc-gen-es v2.15.0，以及固定的 sprotodump：

```sh
SPROTODUMP_DIR=/path/to/lvzixun/sprotodump \
  npm --prefix examples/websocket-gate run generate:benchmark
```

sprotodump 固定 `3a45d3e49ea2bc8e2b607e2c8060b32281d730dd`，复用现有工具，不引入新 generator 或依赖。普通 `npm run generate` 不处理本套测试 schema。

可选官方验证需要固定源码目录及其编译的 Lua core 模块（本轮 Lua 5.3.1、cloudwu/sproto `e93fd5a0653b2bcab38d3b830958bf2e3a901b5f`）：

```sh
SPROTO_ORACLE_DIR=/path/to/pinned/sproto \
  python3 examples/websocket-gate/scripts/benchmark-oracle.py
# 另外检查 20 组正确性向量。
SPROTO_ORACLE_DIR=/path/to/pinned/sproto \
  python3 examples/websocket-gate/scripts/benchmark-oracle.py --correctness
```

脚本先验证 `oracle-source.json` 的六份源码哈希，再用官方 C/Lua 编码五组数据，与固定签名比较。可加 `--output /absolute/file.json` 导出独立证据；Go 侧可设 `GSR_BENCH_EXPORT_WIRE=/absolute/file.json` 运行夹具测试导出独立签名。两种导出都不自动覆盖 `representative.json`，更新夹具时应先比较独立结果再审查变更。
