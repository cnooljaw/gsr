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

[优化实施与验收](../../../docs/benchmarks/2026-10-15-optimized/RESULTS.md) 保存修复后五轮数据、小包回退检查、profile 摘要及独立 Go 库提交。Go 优化尚未发布，GSR 正式依赖仍为 v0.3.0；按该报告的临时 modfile 命令复现优化版。采集器会记录实际模块覆盖及 Go/TS 源码哈希，避免把本地优化结果误标为正式版本。Profile 可设 `GSR_PROFILE_SCENARIO=game_large` 只采大型游戏消息，默认仍采两组。

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
