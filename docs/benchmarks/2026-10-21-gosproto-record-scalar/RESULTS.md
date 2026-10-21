# 整数记录直接消费标量内容

2026-10-21，基线gosproto `f5de803`。**保留候选**：整数记录将inline和非inline分开消费，已知整数直接读取4/8字节，不再先构造内容切片再进入scalarBits。候选已通过完整生产入口验收，独立库已提交为 `ab35ee3`；前轮诊断已提交GSR `e38c350`，均未推送。正式依赖仍v0.4.0，七项无关删除全部保留。

## 收益与限制

负数表示耗时降低；控制不相减，不删GC/长尾，不设置5%门槛。

| 实际路径 | 原20ms目标块第1轮 中位/累计 | 第2轮 中位/累计 | 加长60ms第1轮 中位/累计 | 第2轮 中位/累计 |
| --- | ---: | ---: | ---: | ---: |
| 大型Decode | −1.82% / +1.28% | −1.63% / +0.63% | −1.67% / −2.69% | −0.53% / −0.68% |
| 大型PDecode | −1.84% / −2.18% | −1.07% / −0.54% | −0.71% / −11.84% | −1.18% / −0.84% |
| 小型Decode | −2.09% / −0.54% | −0.94% / −5.65% | −1.10% / −1.25% | +1.05% / +17.11% |
| 小型PDecode | −0.89% / −4.22% | −0.89% / +0.58% | −0.45% / −0.03% | −0.36% / −1.74% |

大型四轮中位方向一致，加长后两轮累计也一致；指令与栈更小，没有增加分配，因此保留这一小幅局部改进。**不能据此声称稳定精确提升某个百分比，也不能声称所有消息变快。** 原大型Decode两轮累计反向，加长小型Decode第二轮反向17.11%都保留。加长大型PDecode首轮累计−11.84%含长尾影响，不当成常态提速。

原大型Decode clone中位−0.05%、IQR2.81pp，PDecode clone−1.05%/0.51pp；加长大型Decode clone−0.41%/1.14pp，PDecode clone+0.00%/3.57pp。候选的小收益接近部分控制波动，未从候选中扣除控制值。接受理由是重复方向、加长复核和实现缩小的综合判断，不是宣称每轮都显著超过噪声。

原10项覆盖5条代表消息的Decode/PDecode，4种模式40进程；加长仅针对大小游戏消息4项、16进程。合计56进程、336轮、1344组ABBA/BAAB、5376块，计时流程90条命令通过。首轮全表见[ALL-CASES](ALL-CASES.md)，加长完整表见[补充测量](../2026-10-21-gosproto-record-scalar-long/ALL-CASES.md)。原10项有10项中位/累计方向不同，补充4项没有；均完整保存。

目标20/60ms来自baseline256次调用的一次校准，不保证实际块长。候选比较块原测量0.604～76.394ms，加长15.174～1008.084ms；未因短块或长尾删除样本。源码/夹具/生成物/二进制指纹与逐块独立复算通过。

## 实现与契约

已知整数：inline直接取值；非inline先检查4字节长度头和完整载荷，再按4/8字节读取，4字节仍做int32符号扩展。未知字段仍接受并跳过任意合法长度，不把integer的4/8字节限制施加给未知字段。原深度、集合预算、tag上限、错误返回和公开入口失败原子性不变。

只移动已证明整数记录中的读取逻辑；数组入口仍先证明当前描述可达字段，外来/变更元信息继续回退。每个presence字段仍new(int64)，每个子对象仍独立，类型化指针写入及GC屏障不变。没有借用输入、池化或合并字段存储。

ARM64静态指令行数（含padding）baseline/clone各408，candidate388；栈帧192→176B。静态panicIndex调用2→2、panicSlice调用4→5：不能把改善描述为全面删除边界检查。实际业务大型Decode保持10416B/532allocs、PDecode13872B/533allocs；小型与全部控制保留原始分配数据。

参考只读官方sproto.c:1190–1268：先验证并消费长度，对已知integer按4/8字节解释。protobuf v1.36.1 codec_gen.go:1932–1954有标量指针专用消费器和常见值短路径。借鉴的是按已知类型直接消费；不照搬C临时指针/callback，也不照搬PB复用原presence指针，因为本库保持新结果独立和失败原子性。参考源码哈希见references.json。

## 验证

先做固定头/长度/截断/符号边界与5000组固定种子随机wire差分，逐一比较baseline/clone/candidate的返回值、错误、预算状态、部分对象和输入未修改。另用单独副本在三种模式下执行既有整数记录ownership/GC/元信息/坏帧，以及外部包原生指针、预算/tag和描述替换用例，race/checkptr通过。新内容是parser差分适配，不重复建立永久业务夹具。

恢复正常私有生产入口后8条验收全部通过：gofmt、完整库+固定官方oracle、全库race/严格checkptr、vet、Codec和Decode各10s fuzz、GSR相关三包、Go↔TS集成7/7（零跳过）。Codec450110次；Decode23035次后从第3秒起停增，11.217s正常退出，不把停增解释为持续覆盖，也不盲目重试。Go源验收哈希保持，实际库只复制已验收的decode_integer_record.go；CHANGES补充说明。

准备阶段有两次构建错误，均保留日志：实验wrapper残留unused unsafe import；内部测试适配器试图调用外部包测试。前者移除无用import，后者分开internal/external适配器后通过。没有丢弃失败记录，也不是产品运行测试失败。

## 复现

```sh
python3 docs/benchmarks/2026-10-21-gosproto-record-scalar/reproduce.py --library /private/tmp/gsr-gosproto-fork --output /absolute/new-output
python3 docs/benchmarks/2026-10-21-gosproto-record-scalar/analyze.py --output /absolute/new-output
```

加长实验使用同名long目录脚本；contracts.py和accept.py记录额外契约与实际入口验收。原始工作目录分别为tmp/gosproto-record-scalar-20261021-run2、tmp/gosproto-record-scalar-20261021-long、tmp/gosproto-record-scalar-accept-20261021。二进制不入库。本轮没有PB/TS重新排名，不拼接旧PB绝对值和新百分比。

后续Body可观察整数记录分配和内容消费的剩余成本；Packing继续真实混合mask主线，不重复已否定的窗口/预算方案。当前没有第二项生产改动。
