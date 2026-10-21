# 字符串优化后的 Decode / Unpack 成本复核

2026-10-21，gosproto `f5de803`，GSR `f81a60d`。完成独立进程9份CPU与9份全量分配profile，80条命令全部通过；源码、输入、二进制和原始profile哈希核对通过。本轮只诊断，没有新增生产改动，也没有重测PB或TS排名。

## 结果

| 入口 | 热点 flat / cumulative | 说明 |
| --- | ---: | --- |
| AddressBook Decode | decodeMessage 9.46% / 70.27% | mallocgc累计34.68%，reflect.New累计16.22% |
| 大型 Decode | decodeIntegerRecord 10.80% / 38.03% | mallocgc累计34.27%；整数记录仍为主要正文路径 |
| 大型 Unpack | UnpackWithLimit 47.16% / 53.28% | 普通word写出仍值得关注 |
| 整数数组 Unpack | UnpackWithLimit 64.85% / 75.73% | 组合解码的关键成本 |
| 大型 PDecode | UnpackWithLimit 10.34% / 11.64% | 整数记录累计22.41% |
| 整数数组 PDecode | UnpackWithLimit 17.98% / 25.88% | decodeIntSlice累计7.02% |

大型整数记录源码行中，`new(int64)`累计440ms，记录循环累计810ms；`readChunk`所在行flat100ms。行级采样只用于提出候选，不能把整行都归为边界检查，也不能相加父子累计值。大型全量分配视图中，decodeIntegerRecord占对象数57.21%、reflect.New占38.48%；这包含预热/初始化，并受tiny allocator影响，不等于每次源码new的精确计数。

| 消息 | Decode B/op / allocs | Unpack | PDecode |
| --- | ---: | ---: | ---: |
| AddressBook | 528 / 29 | 144 / 1 | 672 / 30 |
| 大型 | 10416 / 532 | 3456 / 1 | 13872 / 533 |
| 整数数组 | 8288 / 4 | 4096 / 1 | 12384 / 5 |

字符串写入优化保持分配契约；这些消息的PDecode仍只比Decode多一个解包输出分配。不能由此宣称存在第二份完整正文复制。

## 下一切片

Body优先实验**整数记录的非inline内容读取**：检查当前`readChunk → scalarBits`能否用更直接的4/8字节消费路径减少中间切片和重复判断。未知字段仍须按原规则跳过任意合法长度内容；已知字段仍只接收4/8字节，保持符号扩展、错误顺序、预算、深度、失败原子性和描述替换语义。先核对生成代码，再以单模块self/clone和两轮交错测量检验实际大型消息；小收益也保留，不设比例门槛。该候选尚未实现，没有收益结论。

对象构造确实占成本，但本轮不据此引入对象池、共享字段存储或运行时私有分配器。每个presence指针和子对象的独立可变性必须保留。继续堆字段反射技巧也缺乏证据：大型消息已在整数专用parser中，不能把剩余差距笼统称为reflect开销。

Packing继续保留真实普通mask混合输入作为评估中心。前几轮已否定通用来源窗口、频繁mask特化和预算证明的当前写法；这次profile没有推翻这些测量，不重复同一实验，也不凭固定mask胜出就应用生产。

## 方法与验证边界

复用前轮诊断harness：GOMAXPROCS=1、GOGC=100、GOMEMLIMIT=off，50ms预热；CPU每入口2s，全量分配5000次、memprofilerate=1。先通过代表夹具语义/wire/消费长度/padding与harness vet，再采集。保留完整CPU分母；runtime.kevent占比18.92%～71.50%，不删去或归一化，也不解释为协议网络瓶颈。该短采样用于定位，profile附带ns/op不用于跨轮排名。

原始profile与二进制位于`tmp/gosproto-decode-refresh-20261021`，报告保留文本、metadata和哈希。复现：

```sh
python3 docs/benchmarks/2026-10-21-gosproto-decode-refresh/reproduce.py --library /private/tmp/gsr-gosproto-fork --output /absolute/new-output
python3 docs/benchmarks/2026-10-21-gosproto-decode-refresh/collect.py --output /absolute/new-output
```

使用完整提交归档，不恢复独立库七项工作树删除。正式依赖仍v0.4.0；本次无生产修改，不重复全库/race/fuzz/GoTS验收。前轮完整验收已随字符串优化报告提交；本轮新诊断留工作树审查。
