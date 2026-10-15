# Profile 分析与 Sproto 优化方案

本文保留提交 `d942321` 时的基线分析和待实施方案。后续五步优化及正确性修复已完成，当前结果见 [优化实施与验收](../2026-10-15-optimized/RESULTS.md)；以下缺陷和性能数字描述基线状态。

## 本轮结论

官方 AddressBook 数据已完整复现：Alice/Bob、三个电话号码，Protobuf 为 69 字节，Sproto raw/packed 为 130/83 字节。五组夹具的 Sproto raw/packed 签名均与固定官方 C/Lua 编码器一致；Go/TS 均通过固定签名、语义往返和 padding 检查。官方来源为 [cloudwu/sproto 固定提交](https://github.com/cloudwu/sproto/tree/e93fd5a0653b2bcab38d3b830958bf2e3a901b5f)，六份本地参考源码已通过 GitHub API 与固定提交逐字节核对，哈希见 [oracle-source.json](oracle-source.json)。

| 场景 | Go PB Encode / Decode µs | Go SP Encode / Decode µs | TS PB Encode / Decode µs | TS SP Encode / Decode µs |
| --- | ---: | ---: | ---: | ---: |
| AddressBook | 0.331 / 0.852 | 1.887 / 1.724 | 1.169 / 1.400 | 1.617 / 1.024 |
| 大型游戏消息 | 7.567 / 17.475 | 43.212 / 41.346 | 22.575 / 31.095 | 34.600 / 16.466 |

大型游戏消息包括 256 个正负属性、64 个技能编号、128 个物品、三个字符串标签及标量状态。该消息 Sproto raw/packed 为 3336/2280 字节，Protobuf 为 2081 字节。数字数组使用合理的 Protobuf packed/sint32 定义，Sproto 使用原生 integer；没有以低效的 Protobuf 数组配置制造优势。

当前 Go Sproto 的成本主要在编码器和反射解码器，packing 只是其中一部分。大型游戏消息 Go Encode 为 43.212 µs，Pack 为 5.683 µs；TS Encode 为 34.600 µs，Pack 为 6.524 µs。相反，4 KiB ASCII 的 TS Encode 为 1.745 µs，Pack 为 14.448 µs，说明热点确实随数据形状改变。

TS 在本轮所有五组的纯 Decode 中位数均低于 PB；大型游戏消息即使加上 Unpack，PDecode 约 22.346 µs，仍低于 PB Decode 的 31.095 µs。不能用一种语言或一个方向概括协议优劣。全部大小、独立阶段和组合路径结果见 [报告](report.md)。

本轮只对照当前 Go/TS 实现，没有重测官方历史 C/Lua 对 pbc-lua 的速度排名。

## 采集口径与边界

对 AddressBook、game_large 两个场景的 PB Encode/Decode、SP Encode/Decode、Pack/Unpack 各采一次 CPU 和分配 profile，共 24 组“语言 × 场景 × 操作”。没有对另外三组消息做 profile，也没有对 PEncode/PDecode 再重复归因。

- Go CPU：独立进程，GOMAXPROCS=1，2 秒目标 benchtime；保留 self 与 cumulative top。profile 覆盖测试进程启动和校准，不把其运行时长当作 Benchmark 结果。
- Go 分配：另起进程，固定 1000 次目标操作、memprofilerate=1；使用 alloc_space/alloc_objects，按 BenchmarkRepresentativeCodec.func1 过滤操作调用栈，并按过滤后的子图计算百分比。正常计时的 B/op、allocs/op 仍取独立五轮 benchmem；profile 计数与 MemStats 尤其在 tiny allocation 上不应机械逐项相等。
- TS CPU：每个操作先热身，再开启 V8 CPU profiler，1 ms 采样间隔、约 2 秒；schema 构建与校验在 profiler 外。
- TS 分配：CPU 采集停止后，在单独约 2 秒时段开启 V8 sampling heap profiler，16 KiB 间隔，包含期间被 minor/major GC 回收的对象。得到的是采样估算的分配归属，包含短命对象，不是存活堆、泄漏证明或精确 B/op；Buffer 的外部分配不完整反映在其中，不能直接拿总字节与 Go 比较。

Go CPU 样本包含较高的 runtime.kevent/runtime 调度归因；V8 JIT 内联也会把部分分配/执行归到 batch、runFor 或匿名帧。CPU 百分比仅用于定位，再与独立阶段耗时、分配和源码交叉验证，不承诺跨平台精确比例。cumulative 指标相互重叠，不相加。

已保存 [Go top 汇总](profile-go-summary.json)、[TS CPU/heap 汇总](profile-typescript-summary.json) 和 [采集环境/命令](profile-environment.json)。原始二进制、pprof、cpuprofile、heapprofile 共约 55 MiB，留在仓库 tmp/protocol-profile/，不加入版本管理；可按下面命令重建。

## 已确认的热点

### Go 编码

game_large 正常计时为 16496 B/op、320 allocs/op。CPU profile 中 encodeMessage 累计约 63%，encodedSize 约 24%；后者包含 fieldSize，两者百分比不能与其子函数重复相加。保留资源预算检查有成本，但删除它也解释不了当前全部差距。

过滤操作后的 alloc_space 直接分配归属：encodeStructSlice 约 31.8%、encodeHeaders 30.9%、encodeIntSlice 24.3%、encodeMessage 12.4%。源码将各子结构先编码为独立字节片段，再组合 header/body，数组又有自己的临时缓冲。encodedSize 已经算出完整尺寸，EncodeWithLimits 当前仅用它校验，未用这个结果直接分配最终输出。

AddressBook 同样表现为 encodeMessage/encodeStructSlice 的编码与分配成本，说明不只是“大数组太长”。不能仅看到源码的 make(4096) 就把每个缓冲全部算成堆分配，应以 benchmem 和 profile 为准。

### Go 解码

game_large 为 22112 B/op、1143 allocs/op。过滤后的 alloc_objects 中 reflect.New 约 57.6%、reflect.Value.extendSlice 19.4%、decodeHeader 19.4%；alloc_space 中 decodeHeader 约占 28.8%。

源码对应三类动作：每个结构先分配 []Tag，内联值又通过 *uint16 保存；结构体/字符串数组用 reflect.Append 不断增长；optional 指针字段、子结构和失败原子性临时结果由 reflect.New 创建。后者有一部分是当前 Go 数据表示的真实成本，不能承诺在不改变表示的前提下全部消除。

### TS 编码与解码

game_large 编码 CPU self 中 encode_object_array 约 28.8%、lencode 27.2%、encode_integer_array 14.0%、checktype 8.0%。V8 采样分配字节约 60.7% 归到 lencode、37.0% 归到 encode_integer_array。具体子表达式可能受内联影响，不能仅凭采样认定每一次临时分配的来源；源码中可确认存在通用 Object.prototype.toString/substring/toLowerCase 类型判断、逐元素检查、临时参数对象和逐字节搬移。

TS 解码 self 中 ldecode 约 60.6%、decode_array 32.7%；heap 约 97.5% 归到 decode_array，主要对应输出对象/数组。这条路径当前已有实测优势，优化优先级低于编码；不能为了减少这些分配复用并泄漏上次消息的可变结果。

### Pack / Unpack

Go game_large Pack 的 packSegment self 约 62.5%；TS Pack 的 pack self 约 90.5%，write_ff 仅约 0.4%。所以只将 TS write_ff 改成批量复制，不应期待解决大型数值消息的大部分 packing 成本；ASCII 连续非零数据的收益需另测。

Go Unpack 为 9472 B/op、3 allocs/op，而 raw 仅 3336 字节。现实现按 packed 长度起步并 append 扩容，存在减少累计分配的空间。TS Unpack 热点在 lunpack，CPU self 约 88.2%。

## 优化实施顺序

以下按小切片实施，每步保留独立前后结果；不在这一轮直接改库，也不预先承诺倍数。

### 0. 先修复 TS 正确性缺口

新增探针已确认：

1. true 布尔值在 checkInteger 被 Number.isSafeInteger 拒绝；boolean 编码应有独立处理，将值写成原生内联 0/1，不再把布尔值交给整数校验。
2. integer 数组 [1, 4294967296, -1] 编解码不能往返。源码涉及 32→64 位数组升级、写入游标和 signed high word 解释；先建立官方 raw/packed 向量及 Go↔TS 回归，再分别修复，不能仅改到自身往返通过。

正式性能夹具的数组限定为 int32；上述两组仍保留失败诊断，没有从结果里伪装成已支持。探针只记录现状，不把“必须失败”固化成长期契约。超过 JavaScript 安全整数范围仍显式拒绝，BigInt 不混入本轮修复。

### 1. Go 解码：先消除临时 Tag 和数组增长

先用私有解码游标直接遍历已验证的头部，按值传递 inline 值和 presence，减少临时 []Tag 与每个 inline 的指针分配，不改变导出 API。随后给结构体/字符串数组做一次有界 chunk 计数和边界检查，一次分配输出 slice、逐索引填充，替代 reflect.Append。额外扫描可能有 CPU 代价，必须同时测小包、长数组和畸形输入。

保留临时目标、完整成功后提交的失败原子性；optional 字段仍保持 nil/零值区分，输出子对象仍由本次消息独占。暂不引入对象池或共享 arena。

### 2. Go 编码：利用尺寸检查结果直接写最终缓冲

将已验证的编码长度用于一次输出分配，按偏移写入 header、数据区和嵌套结构，回填必要长度；逐步替代 encodeHeaders、子结构片段拼接和整数数组临时缓冲。先覆盖平坦标量/整数数组，再扩展嵌套结构，避免一次重写全部编码器。

资源字节/深度/元素预算仍在分配和递归前验证；反射 metadata 继续复用，公开 API 和官方 wire 不变。第一阶段不增加 unsafe、默认代码生成或另一套 Fast Codec。

### 3. TS 编码：减少热循环中的通用类型转换与搬移

在 scalar/array 分支使用对应的直接类型与安全整数检查，减少通用字符串类型判断和重复参数对象；保留异类数组、非法值及整数范围检查。对结构体数组和 FF 连续块的实际搬移评估 Buffer.copy，保持重叠复制、尾块补零和 256-word 边界语义。

不要缓存“这个 JS 数组以后始终是整数”或跳过后续调用的检查；调用方对象可能变化。依旧使用原生 Sproto 模型，不增加通用 ProtocolCodec/Envelope。

### 4. 独立优化 Pack / Unpack

Go/TS 都单独比较八字节位掩码扫描、连续块批量复制；先测再决定是否保留位运算改写。Go Unpack 可试“先做有限长度扫描，再按确切输出大小分配并写入”的两遍方案，对照目前的 append 扩容；小包可能因额外扫描变慢，不能只看分配下降。保留最大输出、截断帧和 padding 检查，不根据未验证的长度分配。

除原有案例外，收尾复测零字节密集、FF 长块和 2 KiB/4 KiB 边界，避免只针对 game_large 调参。组合路径 PEncode/PDecode 必须一起验证。

## 验收和发布边界

每步先建立能暴露旧问题的相关测试，再做最小修改；复用 gosproto 当前官方向量、负整数/空数组、嵌套、资源预算和失败保留原值的回归，不在 GSR 重复建立整套底层夹具。新 TS 缺陷在参考客户端层加跨语言回归。Go 库优化在独立 gosproto 仓库完成，发布后再让 GSR 固定升级，不传播 replace。

优化验收看同一 fixture 的字节一致、错误契约、ns/op、B/op、allocs/op 和 profile 变化；完整保留基线，标出无改善或有回退的场景。CPU/分配采集不与正常计时同时运行。没有确认测量改善前不合并“看起来更快”的实现；不承诺赶上官方历史 C/Lua 的倍数。

复现：

```sh
npm --prefix examples/websocket-gate run bench:representative
npm --prefix examples/websocket-gate run profile:protocol
SPROTO_ORACLE_DIR=/path/to/pinned/sproto \
  python3 examples/websocket-gate/scripts/benchmark-oracle.py

# profile 以 game_large/sproto_encode 为例；原始文件由上一条 npm profile 命令产生。
go tool pprof -top tmp/protocol-profile/codec.test \
  tmp/protocol-profile/go/game_large/sproto_encode/cpu.pprof
go tool pprof -top -alloc_space -relative_percentages \
  -focus=BenchmarkRepresentativeCodec.func1 tmp/protocol-profile/codec.test \
  tmp/protocol-profile/go/game_large/sproto_encode/alloc.pprof
```

TS 原始 CPU 文件在 tmp/protocol-profile/typescript/场景/步骤/cpu.cpuprofile，可用 DevTools 加载；heap.heapprofile 为 V8 采样结果。所有完整执行参数见 profile-environment.json。
