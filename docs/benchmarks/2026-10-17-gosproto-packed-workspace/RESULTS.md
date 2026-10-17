# EncodePacked 私有共享工作区

2026-10-17。上一轮 Pack 组合已本地提交为 gosproto d1ab622 / GSR d3b3ef4，提交后两库干净，未推送。本轮以该版本为基线，保留 EncodePacked 单缓冲实现到独立库工作树，未提交或发布。正式 GSR 依赖仍为 v0.4.0，Decode 继续冻结。

## 修改与边界

原 EncodePacked 先 Encode 分配完整未压缩 body，再 Pack 分配独占输出。现有预检已经得到 body 的确切尺寸，因此本轮只分配原 Pack 容量的一块缓冲：在后部编码，再从前部压缩，最终返回独占输出。没有新增 public API、借用结果、池、缓存、unsafe 或依赖。

- 公开 Encode/EncodeWithLimits 仍单独分配编码结果；Pack 仍单独分配输出并保持输入不变。
- 将原 Pack 主循环提取为私有 packInto，缓冲容量计算集中为 packedBufferSize；mask、FF 搜索/合并、256-word 与尾块算法不变。
- EncodePacked 复用原预检、layout 和写出，只在本次调用的独占缓冲内共享空间；默认字节/深度/元素预算、错误和失败时 nil 输出不变。
- 输出长度、容量公式和跨调用独占不变。允许重叠只属于私有组合入口，不要求调用者管理工作区。

三个生产文件变化为 pack.go、limits.go、sproto.go，另扩展现有测试/fuzz 并更新 CHANGES。Decode、Unpack 实现和 RPC 源码均保持不变；普通 Encode 经过新的私有共用入口，所以本轮也测量其实际成本，没有把它当成完全不受实现变化影响的控制。

## 读写边界依据

设预检后的未压缩长度为 N，原 Pack 预留容量为：

```text
C = N + 2 * ceil(N / 2048) + 8
H = C - N
output[H : H+N] 存放未压缩编码
output[0:]      写入压缩结果
```

完整 FF 段每 256 word 增加两字节头。短 FF 段只有在遇到至多五个非零字节的完整 word 时才提前结束，该 word 至少节省两字节，抵消一次额外 FF 头；六/七字节 word 仍合并。因此在还有后续输入时，输出相对已消费输入的增长受两字节/2048 B 额度约束。H 再包含八字节余量，覆盖普通 word 固定位置写出的临时窗口。

普通完整 word 在写出前已读取为 uint64；FF 段先扫描确认消费长度，再复制。FF 头写在源起点之前，Go 内建 copy 允许重叠，复制不会破坏该段内容；非末尾段的输出不会触及下一段未消费输入。短尾块与补零只在末尾处理。该不变量由私有调用方构造，不新增运行时全局状态或业务校验责任。Go copy 文档的本地源码指纹保存于 validation.json。

独立负向检查将 H 故意设为零，新的 65535 B 负载用例立即发现 wire 不同，见 [overlap-negative.json](overlap-negative.json)。这说明长 FF 边界测试能够检出写坏未消费输入，而不仅是验证小包往返。

## 测试增量及修正

已有测试覆盖单独 Encode/Pack 的 wire、FF 和预算，但没有保护组合入口的共享缓冲。因此在既有 sproto_test.go 增加：

- 10240 组 mask/FF/尾块组合，加六种跨多段的长输入（含 64 KiB 邻域），比较 EncodePacked 与 Pack(Encode) 的精确字节、cap、源存储及旧输出独占。
- nil/非指针、unsigned 超界和默认字节预算错误，必须仍返回可判断错误与 nil 输出。
- 简单组合路径比分步编码少一次缓冲分配。初版在旧实现观察到两次分配并按预期失败，正常新实现为一次。
- 原 Pack fuzz 增加二进制消息的组合入口，原 Codec fuzz 增加嵌套/数组消息的组合字节与容量比较。

首次 race/checkptr 验收遇到测试断言失败：插桩构建在组合路径计得两次分配，不能使用普通构建的一次总分配数作为插桩上限。只将测试修正为比较同一构建中的组合与分步路径，要求严格少一次；没有修改生产代码或放松 wire 检查。修正后的用例在旧生产实现仍失败（2 对 2），见 [relative-negative.json](relative-negative.json)，新实现普通及插桩构建均通过。原失败记录保存在 [acceptance-instrumentation-failure.json](acceptance-instrumentation-failure.json)。

性能采样后仅调整该测试。最终三个生产文件与采样候选逐字节一致；[candidate.patch](candidate.patch) 加 [final-test-adjustment.patch](final-test-adjustment.patch) 可重建全部最终 Go 源码，不把测试修正冒充为同一个完整文件快照。

## 两轮业务测量

按预先记录的 [plan.json](plan.json)，self、clone 控制各一次，候选各两轮；原 19 项、10 ms、六轮 ABBA/BAAB 不变。共 **7296 块、1824 个四元组、456 个配对轮次**，全部独立复算一致，没有重试或改变参数。Apple M2、Go 1.24.13、GOMAXPROCS=1、GC 开启。

| PEncode 路径 | 第一轮耗时变化 | 第二轮耗时变化 | B/op 前 → 后 | allocs/op 前 → 后 |
| --- | ---: | ---: | ---: | ---: |
| AddressBook | −7.12% | −6.20% | 288 → 144 | 2 → 1 |
| 小型游戏 | −6.62% | −6.86% | 704 → 352 | 2 → 1 |
| 大型游戏 | −4.82% | −5.52% | 7424 → 3968 | 3 → 2 |
| 896 项整数数组 | −9.46% | −9.70% | 8192 → 4096 | 2 → 1 |
| ASCII 4 KiB | −40.03% | −40.71% | 9728 → 4864 | 2 → 1 |

分配值在两轮一致；大型消息仍保留 layout 的额外分配，未承诺所有结构都只有一次总分配。self/clone 原工具总控制分别通过 15/19、16/19；全部失败标记、IQR 和逐轮原值保留在各运行目录及 [joined.json](joined.json)。表中是配对轮次中位数，不是置信区间。

## 独立入口和完整帧的代价

本轮没有修改手工组帧或 RPC，完整帧仍分别编码 header/body、拼接后调用 Pack，不能把 PEncode 的降幅直接用于网络完整帧。

| 完整帧 | 第一轮 | 第二轮 |
| --- | ---: | ---: |
| ASCII 32 B | +3.07% | +2.25% |
| UTF-8 | +0.39% | +0.15% |
| ASCII 4 KiB | +0.75% | −1.33% |
| 零字节 4 KiB | +0.02% | +1.48% |

32 B 帧有一致的约 7～9 ns 代价，原值保留，不能全部归为噪声。结合 PEncode 的一次分配下降与五种真实消息改善，本轮接受该权衡。其余完整帧小幅变化视为基本持平，不宣称获得组合入口的收益。

独立 Encode 各项两轮约在 −1.83%～+0.90%，独立 Pack 约在 −1.39%～+1.85%，没有观察到持续的明显回退；源码边界拆分仍可能有小包调用成本。完整帧分配数和容量不变，第二轮零字节帧出现原有 0.125 B/op 的 tiny/GC 窗口差，原计数单列保留。

## 最终验收与复现

[acceptance.json](acceptance.json) 保存最终普通库测试、完整 race/checkptr、固定官方 C/Lua 对照、vet、两项有限 fuzz、GSR 三包及 Go/TS Plain/MAC 集成 7/7、零跳过。Pack/Codec fuzz 分别执行 44243 / 228585 次；Pack 后段零新增执行并正常退出，未延时或重跑。没有 GSR 全仓/race、跨平台运行或新 PB/TS 性能结论。

四个 prepare-only 归档重建精确匹配计时源、harness、夹具、工具和生成类型；应用最终测试补丁后又精确匹配实际工作树。正式 go.mod、TS、夹具和 Decode 阶段基线未变。

```sh
python3 docs/benchmarks/2026-10-17-gosproto-packed-workspace/reproduce.py \
  --library /path/to/gosproto --output /tmp/gsr-packed-workspace
```

可加 --prepare-only 只核对归档生成；输出目录必须不存在。使用本报告版本的工具和夹具，指纹见 inputs.json、driver-inputs.json 与 validation.json。重建的是计时候选，完整最终验收前需应用 final-test-adjustment.patch。脚本不修改传入库，不保证相同耗时。新实现、测试和记录留待审查，未提交、推送或发布。
