# gosproto 第三轮：当前基线、失败候选与布局记录内存收窄

2026-10-16，TS 经验文档已提交为 GSR `f45a8c6`。本轮从独立 gosproto `6012b06` 重建 Go 基线与 profile，未从 v0.4.0 重做前两轮。TS 生产源码冻结，GSR 正式依赖仍为 v0.4.0；新 Go 改动通过临时 modfile 接入，尚未提交、发布或升级正式依赖。

## 本轮保留的改动

只将当前调用的 encodeLayout 记录从 int 改为 uint32。记录只存结构头项数和非空整数数组宽度 4/8；合法 tag 范围 0–65535、字段 tag 唯一，头项数不超过 131072，uint32 保留完整数值范围。不是改为可能截断头项数的 uint16。记录数量、栈内 32 项和溢出增长策略均不变，没有新增跨调用状态。

64 位环境下，栈内数值区从 256 B 缩为 128 B，首份 128 项额外记录从 1024 B 缩为 512 B。本机大型消息 Encode 总分配 4480 → 3968 B/op，PEncode 7936 → 7424 B/op，均少 512 B；分别仍为 2/3 allocs/op。32 位环境的原 int 已为四字节，不宣称有相同内存节省。

公开 API、官方 wire、默认/自定义预算、预检与遍历、optional presence、失败原子性及输出独占均不变。Decode/Meta/Limits/Pack 的最终生产源码与起点逐字节相同。没有采用 unsafe、池或新 generator。

## 当前 Go 差距

工具链：go version go1.24.13 darwin/arm64；GOMAXPROCS=1；PB 为 google.golang.org/protobuf v1.36.1。继续使用原 AddressBook、两档游戏、896 项数组和四种 Echo 夹具；原有 wire 签名和调用入口保持不变。

下表为最终同一 Go 二进制的五轮中位数，单位 µs/op。SP 纯编码/解码与含 packing 的组合分别列出，不能用前者替代后者评价实际 wire 成本。

| 消息 | PB Encode | SP Encode | SP PEncode | PB Decode | SP Decode | SP PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| AddressBook | 0.330 | 0.541 | 0.704 | 0.829 | 1.023 | 1.213 |
| 小型游戏状态 | 0.672 | 1.377 | 1.747 | 1.663 | 1.813 | 2.342 |
| 大型游戏状态 | 7.558 | 14.498 | 18.057 | 17.396 | 17.901 | 23.251 |
| 896 项整数数组 | 4.868 | 2.184 | 6.713 | 4.171 | 2.279 | 7.631 |
| 4 KiB ASCII body | 0.575 | 0.624 | 2.753 | 0.605 | 0.708 | 1.237 |

Go 并非所有路径都落后：整数数组纯编解码已快于 PB，但 packing 后仍有差距；大型结构体的编码差距仍明显。不能从 TS 10/10 胜出推断 Go 会自动重复同样曲线。本轮没有声称全面追平 PB。

## 保留改动的前后对照

主套件前后各 72 项、每项五轮，共 720 个计时样本；200 ms 自动迭代。另有最终 18 种解包形态 × 五轮（90 个样本），只用于回退观测，不算作 PB 可比项目。计时、profile 和测试顺序进行。

| 消息 | 操作 | 前 µs | 后 µs | 耗时变化 | B/op 前→后 | allocs/op 前→后 |
| --- | --- | ---: | ---: | ---: | --- | --- |
| AddressBook | sproto_encode | 0.545 | 0.541 | -0.8% | 144 → 144 | 1 → 1 |
| AddressBook | sproto_pencode | 0.711 | 0.704 | -1.0% | 288 → 288 | 2 → 2 |
| AddressBook | sproto_decode | 1.021 | 1.023 | +0.2% | 528 → 528 | 29 → 29 |
| AddressBook | sproto_pdecode | 1.228 | 1.213 | -1.2% | 672 → 672 | 30 → 30 |
| 小型游戏状态 | sproto_encode | 1.387 | 1.377 | -0.7% | 352 → 352 | 1 → 1 |
| 小型游戏状态 | sproto_pencode | 1.747 | 1.747 | +0.0% | 704 → 704 | 2 → 2 |
| 小型游戏状态 | sproto_decode | 1.836 | 1.813 | -1.3% | 1200 → 1200 | 52 → 52 |
| 小型游戏状态 | sproto_pdecode | 2.330 | 2.342 | +0.5% | 1552 → 1552 | 53 → 53 |
| 大型游戏状态 | sproto_encode | 14.542 | 14.498 | -0.3% | 4480 → 3968 | 2 → 2 |
| 大型游戏状态 | sproto_pencode | 18.080 | 18.057 | -0.1% | 7936 → 7424 | 3 → 3 |
| 大型游戏状态 | sproto_decode | 18.066 | 17.901 | -0.9% | 10416 → 10416 | 532 → 532 |
| 大型游戏状态 | sproto_pdecode | 23.542 | 23.251 | -1.2% | 13872 → 13872 | 533 → 533 |
| 896 项整数数组 | sproto_encode | 2.216 | 2.184 | -1.4% | 4096 → 4096 | 1 → 1 |
| 896 项整数数组 | sproto_pencode | 5.947 | 6.713 | +12.9% | 8192 → 8192 | 2 → 2 |
| 896 项整数数组 | sproto_decode | 2.280 | 2.279 | -0.0% | 8288 → 8288 | 4 → 4 |
| 896 项整数数组 | sproto_pdecode | 7.688 | 7.631 | -0.7% | 12384 → 12384 | 5 → 5 |
| 4 KiB ASCII body | sproto_encode | 0.607 | 0.624 | +2.7% | 4864 → 4864 | 1 → 1 |
| 4 KiB ASCII body | sproto_pencode | 2.695 | 2.753 | +2.2% | 9728 → 9728 | 2 → 2 |
| 4 KiB ASCII body | sproto_decode | 0.646 | 0.708 | +9.6% | 4176 → 4176 | 5 → 5 |
| 4 KiB ASCII body | sproto_pdecode | 1.186 | 1.237 | +4.3% | 9040 → 9040 | 6 → 6 |

本轮确定收益是内存，CPU 耗时基本持平，不能将几项较低的中位数包装为新的速度突破。专门的十项 Encode/PEncode 前后配对五轮另存 [pairs.json](pairs.json)；大型消息 Encode 约 +1.6%、PEncode 约 -0.1%，内存节省在所有轮次一致。

## 全量波动与复核

自动迭代全量中，整数数组 PEncode、4 KiB ASCII 帧编码/解码、零字节帧编码四项出现约 10%–13% 的中位数回退；分配次数不变，原始值全部保留。没有删除失败样本或直接判定是噪声。

随后生产源码不变，按操作配对、交替顺序，每轮固定 60000 次，五轮并列 PB 控制。配对用的基线由同一 6012b06 源码快照及新增形态 Benchmark 重新编译，最终候选二进制保持不变；全量最初的基线编译早于形态 Benchmark，两个基线二进制的指纹分别保留。原有业务 Benchmark 和夹具未改动，不混合两种采样取中位数。

四项 SP 变化分别约 -0.1%、+2.3%、+2.9%、-1.0%，未支持持续 >10% 回退。固定次数复核不替代全量统计，也不是显著性证明。完整复核见 [pairs.json](pairs.json)。

## 候选筛选：未合入的改写

| 候选 | 观察 | 结论 |
| --- | --- | --- |
| 普通 word 固定八分支展开 | 大型/整数 Unpack 约 -21%/-12%，AddressBook 约 +38% | 小包回退，撤回 |
| 只改游标和输出区间 | 纯 Unpack 部分改善，组合采样明显波动 | 不据专项局部值合入 |
| 固定八字节输入/输出窗口 | 四/七非零字节密度约 -17%，单字节约 +24% | 稀疏数据回退，撤回 |
| 窗口加单字节直接路径 | 单字节恢复，但双字节约 +21%、FF 前缀接稀疏约 +11% | 覆盖扩大后失败，撤回 |
| 窗口加一/二字节直接路径 | 形态较稳定；大型/整数真实 PDecode 仅约 -1.6%/-3.1%，仍有约 +5%/+7% 的形态损失 | 支路增加而业务收益有限，不合入 |
| 原生指针通过 Addr/Interface 直接赋值 | 大型 Decode/PDecode 约慢 19%，小型游戏约慢 16%–18% | 不能根据少一次 Set 推断收益，恢复原路径 |

候选均保持预算与 wire，但正确不等于值得合入。稀疏与密集输入的成本相反，不能只把 TS 的写法搬到 Go。公开结构的反射、optional 对象和失败原子性仍有真实成本；本轮未移除它们。候选全部原始采样和源码/二进制指纹见 [experiments.json](experiments.json)，开发期源码与完整临时日志保留在忽略的 tmp/gosproto-round3/。没有把库源码复制进 GSR 生产目录。

## Profile 与验证

当前基线对 AddressBook/game_large/int32_edges 的 Encode/Decode 分别采 CPU 与分配，另采两档 Pack、PEncode、Unpack；最终改动再采大型 Encode/PEncode。self/cumulative 不能相加，profile 不当作计时结果。原始 CPU 有明显 runtime.kevent 样本，不能将其分配为业务编码成本，结论只用于定位字段访问、反射赋值与普通解包循环。摘要见 [profile-summary.json](profile-summary.json)，完整二进制和 .pprof 留在 tmp/。

- 独立库 race 全测、vet、固定官方 C/Lua 对照通过；FuzzDecode / FuzzPackRoundTrip / FuzzCodecRoundTrip 各约 10 秒通过，有限 fuzz 不证明穷尽。
- 扩展已有全 mask × 四种非零值测试，Unpack 直接消费独立构造的规范字节；新增原生指针字段与跨调用结果互不影响的回归，补足旧 DeepEqual 往返未明确检查的别名风险。
- 临时 modfile 下，GSR protocol/sproto、Echo 服务端和 Benchmark 三包测试通过；Go/TS Plain/MAC 集成 7/7、零跳过。TS 源码没有修改。
- 仅修改独立库编码内部记录，不重复 GSR 全仓测试；正式 go.mod/go.sum 未变。完整命令与范围见 [validation.json](validation.json)。

## 复现与下一步

以独立库 6012b06 构建基线，以本轮未发布工作树（之后用对应提交）构建最终版本；临时 modfile 的实际模块覆盖和源码哈希必须与 [inputs.json](inputs.json) 对齐。主命令见 [Benchmark 说明](../../../examples/websocket-gate/benchmarks/README.md)。新解包形态可单独运行：

```sh
GOMAXPROCS=1 GOFLAGS=-modfile=/absolute/perf.mod go test ./examples/websocket-gate/benchmarks \
  -run '^$' -bench '^BenchmarkUnpackShapes$' -benchmem -benchtime=100ms -count=5 -timeout=2m
```

下一步优先分析大型结构体 Encode 的重复字段访问、scalar 判定和预检/写出的配合，再考虑其他变化；不要重复已有两轮，也不要为包装一个胜出结论扩大 Go 语义或引入未裁决的 unsafe/generator。本轮结果支持“先测再筛选”，还没有证明 Go 可在全部业务路径追平 PB。
