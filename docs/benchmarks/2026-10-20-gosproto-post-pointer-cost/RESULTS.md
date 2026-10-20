# 数组指针优化后的Body与Packing成本诊断

2026-10-20。库基线为已提交 `a93bb0d`，前轮报告已提交GSR `3f23e02`。本轮只读采样，没有新生产候选、依赖升级或PB/TS性能排名。目的在于更新热点与实际byte pattern，避免沿用优化前的比例继续猜测。

## 方法与口径

从提交fresh归档，三个相同代表消息（game_large、int32_edges、echo_ascii4k），分别走公开Encode、Pack、EncodePacked。9个CPU进程每个约2秒、9个全量allocation进程每个5000次、memprofilerate=1；GOMAXPROCS1/GOGC100/无限内存软上限。独立测试先验证原fixture/wire，harness vet与编译通过。源码、生成输入、夹具、profile helper、二进制及raw profile哈希固定，80条流程命令均成功。

CPU表以各自完整profile样本为分母，父子累计不相加。带profile的ns/op不当作新速度排名，跨进程/跨轮百分比也不相减来算提速。allocation profile包含初始化/预热；表中的loop B/op和allocs/op取ResetTimer后的5000次循环，不把alloc_objects物理块计数等同MemStats逻辑小对象计数。

本次runtime.kevent样本很高，例如大型Encode31%、大型Pack45.05%、ASCII Pack73.04%；相关调用栈包含GC start/stop world。完整原文保留，不删除后重算codec占比，也不据此把GSR网络实现认定为瓶颈。短CPU profile与行级PC归属只能帮助缩小候选范围，不能证明某一源码行就是独立耗时。

## Body剩余成本

| 入口 | 符号 | 完整样本累计占比 |
| --- | --- | ---: |
| 大型Encode | encodedSize | 28.50% |
| 大型Encode | encodedIntegerRecordSize（上项子调用） | 21.50% |
| 大型Encode | encodeMessageInto | 30.00% |
| 大型Encode | encodeIntegerRecordInto（上项子调用） | 17.00% |
| 大型PEncode | encodedSize | 25.22% |
| 大型PEncode | encodeMessageInto | 29.13% |
| 大型PEncode | packInto | 17.39% |
| 整数数组PEncode | encodedSize | 8.23% |
| 整数数组PEncode | encodeMessageInto | 16.88% |
| 整数数组PEncode | packInto | 29.00% |

数组指针提取已不是下一轮主攻点，整数记录的字段执行仍在两遍中发生。Body继续检查当前schema.order→Fields的字段索引、tag/presence处理与当前标量读取；不因已经领先旧PB而停止，也不通过缓存标量或跳过预算改变契约。

下一Body候选可以在已证明的同构整数记录数组入口，构造仅属于本次遍历的当前字段引用访问计划，减少每个元素的重复双层索引；数值仍每次读取，公开描述在每次Encode重新绑定。先核对实际收益与额外拷贝、栈帧及分配成本，超出局部容量的字段集可保留原路径。尚未实施，不新增公共plan/API，不把它写成已接受的架构。

## Packing：两种不同的热路径

另在独立harness副本上核对实际编码wire，逐word统计非零字节数，并从packed流独立重建、与公开Unpack比较；没有修改profile输入/源码/二进制。1条审计命令通过，完整分布与wire SHA见wire-audit.json。

| 消息 | 原始字节 | 完整word | 非零0～2字节 | 非零3～7字节 | 非零8字节 | 实际FF段/消费字节 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 大型状态 | 3336 | 417 | 29 | 386 | 2 | 2 / 16 |
| 整数数组 | 3593 | 449（另1字节尾部） | 128 | 321 | 0 | 0 / 0 |
| ASCII4KiB | 4104 | 513 | 1 | 0 | 512 | 2 / 4096 |

大型状态与整数数组的普通word占主导，不应优先优化FF搜索。大型Pack的packInto累计46.04%，整数数组Pack28.43%；行级样本出现在主循环、word读取、mask分类、固定目标视图和dense前缀位置写入，不能简单等同“某个if占多少”。

下一Packing候选只隔离**FF分支之后，普通word的稀疏/密集目标视图是否可共享一次固定窗口构造**，核对它是否减少长度/cap/address计算；现有两分支分别构造3/9字节视图。必须先证明packedBufferSize的完整增长额度/8字节余量以及EncodePacked重叠读写间隔仍成立，再做机器码、全部mask/长度/密度和小包配对。统一9字节视图可能让稀疏路径变差，不能预告收益或去掉必要检查；FF、输出所有权、cap和wire均不变。尚未实施。

ASCII几乎全为FF：packInto累计6.86%，其中ffRunBytes/followingFFWords各4.41%为父子关系；mallocgc累计18.63%。它的主要约束与普通word不同。结果是独占字节输出，不能把池化、借用输入或取消清零当成不改变API的免费优化。优先级放在普通word候选之后，保留ASCII作回退对照。

## 循环分配

| 消息 | Encode B/次 | Encode分配/次 | Pack B/次 | Pack分配/次 | PEncode B/次 | PEncode分配/次 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 大型状态 | 3968 | 2 | 3456 | 1 | 3968 | 2 |
| 整数数组 | 4096 | 1 | 4096 | 1 | 4096 | 1 |
| ASCII4KiB | 4864 | 1 | 4864 | 1 | 4864 | 1 |

PEncode不是两个公开操作简单相加的分配；它使用既有融合缓冲。当前不存在“每个记录创建输出对象”的分配问题，不应为减少对象数而引入arena或改变独立所有权。长期优化容量/缓冲策略仍须遵守既有cap契约，此轮不调整。

## 来源、复现与工作树

18份profile、loop分配、flat/cum/source/objects/space文本及复算analysis均保留。raw二进制profile、归档和可执行文件留在忽略tmp，随inputs保存精确哈希。独立wire审计沿用相同fixture和库模块，source SHA及命令记录另存；原profile harness保持不变。fresh源码/输入/二进制/raw/库工作树核对全部通过。没有新运行时代码，不重复前轮完整oracle/race/fuzz/GoTS验收，也不声称本轮新测过这些。

复现profile：在GSR根执行 `python3 -B docs/benchmarks/2026-10-20-gosproto-post-pointer-cost/reproduce.py --library /private/tmp/gsr-gosproto-fork --output tmp/新的空目录`，再以同目录analyze.py复算。wire审计用同目录audit.py传入profile输出及另一个不存在的输出目录。保留Go缓存；环境与所有命令在inputs/commands中记录。

原库只有既有6项用户删除，Go源与提交a93bb0d一致；没有暂存/恢复/提交这些删除。GSR新诊断及文档未提交，未推送/发布/tag，正式依赖v0.4.0不变。Body与Packing保持两条主线，不冻结Decode，不用一条主线的收益替代另一条。
