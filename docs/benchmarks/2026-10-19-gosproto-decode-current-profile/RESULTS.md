# 当前 Decode / PDecode 热点与分配诊断

2026-10-19。前轮header字节偏移实验已本地提交为GSR `9699583`，未推送；正式库保持已验收的 `503a725`。本轮重新定位热点，**不修改生产代码、不采用新候选、不采A/B，也不形成新的PB/TS排名**。新诊断报告未提交，正式依赖v0.4.0，Decode继续开放。

## 方法与数据边界

从503a725独立归档self前后模块，18份Go源码等于主库和提交；沿用当前40项的同一构造/wire/语义检查，精确选择大型Decode/PDecode与896元素整数数组Decode/PDecode四入口。每次解码创建新的独占结果，其他操作闭包释放，清encoded/representative/decoded三个sink再GC，预热50ms。

四入口各起两个独立进程：CPU手动在上述初始化后启动，采约2秒；allocation另用memprofilerate=1、固定20,000次。GOMAXPROCS1、GOGC100、无限MemoryLimit，每进程三个计时外快照，共8进程/24快照。limiter全部零，最大采样堆4,048,168B，约3.86MiB；这是快照，不是峰值。

CPU profile只覆盖预热后的循环。Go的memprofile包含初始化、40项夹具构造与预热；因此大量before库、layoutRecords、reflect.New出现在整数数组内存摘要中，不能据此说每次整数数组调用都构造完整大型记录。准确循环B/op和allocs/op以b.ResetTimer后的benchmark输出为依据；对象profile也不等于每个new表达式的精确计数，不将其除以20,000或当作可消除对象总量。

所有原始profile的SHA、二进制/源码/工具/夹具/生成输入和命令保留。profile含明显测量开销：大型纯Decode在CPU采样下16,440ns/op，而memprofilerate=1下835,008ns/op；后者不能作为新性能基线或拿来与前者/PB对比。完整耗时原文保留于各benchmark.txt；本页只用它们诊断路径和精确循环分配。

## 精确循环分配

分开的CPU与allocation进程在以下B/op、allocs/op上一致，不把CPU采样与内存采样混成一个计时结果：

| 入口 | B/op | allocs/op |
| --- | ---: | ---: |
| 大型 Decode | 10,416 | 532 |
| 大型 PDecode | 13,872 | 533 |
| 整数数组 Decode | 8,288 | 4 |
| 整数数组 PDecode | 12,384 | 5 |

大型路径仍包含独占整数指针、子结构、字符串等构造。当前普通Go struct与presence/独占契约并不允许直接把这532次都当作冗余，更不能从pprof对象比例推导全都可省。整数数组已经是4次分配，下一步更适合先改逐元素执行；PDecode多出的4,096B和1次分配与Unpack归属一致，但不是移除输入/输出边界的依据，也不把分段耗时相加解释组合耗时。

## CPU 样本

以下为各自**完整CPU样本总数**中的累计比例；kevent为其flat比例。父子累计重叠，尤其decodeMessage与decodeStructSlice/decodeIntSlice不能相加。未列出的函数不能当成零。

| 入口 | decodeMessage | 相关子路径 | 其他主要路径 | runtime.kevent flat |
| --- | ---: | --- | --- | ---: |
| 大型 Decode | 60.70% | decodeStructSlice 54.15% | reflect.New 10.92%，setInteger 5.24% | 34.06% |
| 大型 PDecode | 47.34% | decodeStructSlice 40.58% | reflect.New 7.25%，UnpackWithLimit 15.46% | 33.82% |
| 整数数组 Decode | 35.44% | decodeIntSlice 34.60% | readUint32 8.44% | 60.76% |
| 整数数组 PDecode | 13.30% | decodeIntSlice 12.88% | UnpackWithLimit 30.90%，readUint32 3.00% | 52.79% |

大型路径仍集中在通用parser递归、子结构构造及独占整数对象，不支持继续只围绕循环计数方式推演收益。整数数组纯Decode的slice读取循环有可见执行成本，且分配数量很低。组合路径的Unpack比数组decoder累计样本更多，说明即使body改进也必须回看实际PDecode，不能只报告纯Decode胜出。

kevent保留原值，不从分母中剔除、不归一化“业务占比”，也不单凭这些样本断言其唯一原因或理论提速上限。这些profile不是配对计时，没有控制分数、置信区间或新的性能胜负结论。

## 源码与机器码确认

当前native []int64分支先检查非空数组宽度只能4/8、长度整除和元素预算，再一次性分配结果；随后每个元素调用readUint32(data[index*4:])或readUint64(data[index*8:])。实际[反汇编](decode-assembly.txt)在两个循环均保留开放尾切片的剩余len/cap、NEG/ASR/AND调整，而不只有固定宽度载入。这里是下一轮值得单独验证的局部执行成本；本轮没有生成或计时固定元素视图候选，不预言收益。

同时阅读本地只读Skynet的sproto.c和锁定版本Protobuf Go源码，文件指纹/行号见[source-references.json](source-references.json)：

- Sproto C在确认4/8字节宽度及整除后按固定步长读取，对4字节有符号值做扩展。可以沿用先校验宽度与总边界、再执行固定读取的思想；C指针访问不能代替Go边界/预算和输出独占要求。此处引用本地Skynet源码，不冒称它等同于某个固定官方提交。
- Protobuf Go使用原生类型访问、短varint路径，并先计数packed数组元素再增长；其wire与现有目标对象/切片模型不同。可参考将已验证信息转化为短执行路径，不能直接照搬指针复用或append策略而改变GSR当前失败原子性/独占语义。本轮没有运行Protobuf计时或把源码相似性当作速度保证。

## 下一切片

优先只验证native []int64的**固定4/8字节元素读取视图**，继续沿用现有预检、符号扩展、精确分配、空数组和命名类型反射回退；不同时改数组游标、分配、字段守卫或packing。当前896元素代表夹具是4字节路径，因此下一轮计时须补8字节大数组观察，再看小数组、真实游戏、PDecode与未改Unpack控制，不能仅用4字节收益决定全部路径。

正确性先复用已有SignedIntegerArray、NativeIntegerArrayWire、MalformedIntegerArray和预算/ownership用例；只有发现明确覆盖缺口才扩展测试。不设置统一最低收益比例，保留局部成本、失败控制和累计翻转。若body有效而组合路径不改善，先解释Unpack边界成本，再决定是否单独重访Unpack，不将两个候选混在同一轮。

## 验证与复现

本轮实际完成harness统计/选择/GC/代表夹具/默认指标测试与vet、当前40项名称顺序及构造wire/值/消费长度检查、构建、8次独立profile、20份pprof摘要/源码文本、当前decoder反汇编、fresh重建和源码/二进制/rawprofile/元数据指纹核对。35条记录命令均成功。源、对象和计时范围的限制如上，不冒称新跑109形态525差分或160项配对。

生产未改且主库干净，未重复库全测/官方/race/checkptr/fuzz、GSR三包/GoTS、PB/TS性能或跨平台；503a725完整验收仍见[固定header视图报告](../2026-10-19-gosproto-decode-header-view/RESULTS.md)。正式go.mod/go.sum等于HEAD、仍为v0.4.0。没有推送、发布或打tag。

从GSR根目录复现，输出目录须不存在，库须包含503a725：

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-decode-current-profile/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-decode-current-profile-new
```

--prepare-only只建输入。文本/元数据随报告保存，原始CPU/内存profile、二进制和源码副本保留在忽略的tmp目录，通过SHA连接。新诊断未提交，Decode继续开放，不新增阶段冻结。
