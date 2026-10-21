# 最新提交的 Body / Packing 六入口成本复核

2026-10-21，gosproto `2e1d44b`、GSR `894297d`。前轮指针直接复用已提交，未推送。本轮采集AddressBook、大型游戏、整数数组的Encode/Pack/PEncode/Decode/Unpack/PDecode，共18份CPU与18份全量分配profile；152条流程和6条补充源码导出全部通过，源码、输入、二进制与原始profile哈希核对通过。**本轮仅诊断，无新生产改动，没有PB/TS新排名。**

## 当前热点

全部百分比使用完整CPU样本作分母；flat与累计分开，父子累计不能相加。

| 消息 / 入口 | 函数 | flat | 累计 |
| --- | --- | ---: | ---: |
| 大型Encode | encodedIntegerRecordSize | 12.56% | 18.39% |
| 大型Encode | encodeIntegerRecordInto | 13.45% | 18.83% |
| 大型PEncode | packInto | 17.90% | 18.34% |
| 大型PEncode | encodeIntegerRecordInto | 11.79% | 16.16% |
| 大型Decode | decodeIntegerRecord | 7.39% | 25.65% |
| 大型Decode | mallocgc | 7.83% | 24.35% |
| 大型Unpack | UnpackWithLimit | 48.90% | 57.27% |
| 大型PDecode | UnpackWithLimit | 12.04% | 16.20% |
| 数组PEncode | packInto | 27.63% | 27.63% |
| 数组Unpack | UnpackWithLimit | 63.45% | 77.91% |
| 数组PDecode | UnpackWithLimit | 24.34% | 30.97% |

AddressBook Encode仍在通用预检/写出：encodedSize累计32.46%、encodeMessageInto38.16%；不能把整数数组专用循环的结论推广到它。大型Decode的reflect.New累计10.87%，分配profile中decodeIntegerRecord对象数57.21%、reflect.New38.48%，反映presence与子对象构造成本仍重要。分配profile包括初始化与预热，tiny allocator对象计数不等于源码new次数；不除以5000伪造精确对象分布。

大型专用Encode写出源码中layout.next所在行采到累计70ms/2.23s（约3.14%），其余仍分散在字段读取、presence、tag处理和写出。行采样不是表达式精确计时，但支持先检查布局读取，避免回头重复已未确认的Tag/字段引用缓存实验。

## 下一项 Body 实验

限定**已证明的纯整数记录数组，批量/分段消费本次预检产生的header布局计数**。现有每条记录调用layout.next，重复加载并更新cursor，逐次判断栈内32项还是extra。纯整数记录不含嵌套子结构，布局项连续；可探索在数组入口按local/extra两段取得只读视图，把count传给记录写出，减少每记录的游标和分段判断。

这是待验证的局部实现方向，尚未实现，不能预先宣称更快。必须覆盖跨32项边界、数组前已有兄弟结构、nil记录（仍占布局项）、空数组、多个相邻数组、描述替换及命名指针回退；保留原布局顺序、全部预检与失败原子性。不得按未验证数组长度提前分配，不缓存可变标量值，不复活跨调用元信息证明。也不能把单次源码行采样当作潜在总收益上限。

对象分配不通过池化、合并presence指针存储或私有runtime分配器解决。每个返回子对象/字段仍独占。Decode继续开放，但本轮证据更支持先回到Encode布局处理。

## Packing 主线

大型与整数数组仍集中在普通word循环；FF搜索不是这两个消息的主成本。大型Pack源码样本分布在word读取、非零mask、稀疏判定、前缀位置计算和payload写入，不能把某一行全部叫作边界检查。

后续检查普通word的mask/前缀中间计算是否能局部共享，或减少依赖；仍以真实混合输入检验，而非固定高频mask赢了就替换。当前还没有新的已实现候选，不重试此前来源窗口、预算证明、固定mask特化等已经未确认/否定的写法。零段、FF段、短包、1～7有效字节和组合入口都必须保留，不能用新增分配缓存每word元信息换表面循环速度。

## 分配契约

各格为B/op / allocs/op，取自ResetTimer后的循环，而非全量分配profile总量。

| 消息 | Encode | Pack | PEncode | Decode | Unpack | PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| AddressBook | 144 / 1 | 144 / 1 | 144 / 1 | 528 / 29 | 144 / 1 | 672 / 30 |
| 大型 | 3968 / 2 | 3456 / 1 | 3968 / 2 | 10416 / 532 | 3456 / 1 | 13872 / 533 |
| 整数数组 | 4096 / 1 | 4096 / 1 | 4096 / 1 | 8288 / 4 | 4096 / 1 | 12384 / 5 |

组合Encode没有额外完整输出分配，组合Decode在这些消息中只多一个解包输出分配。大型Encode第二个分配来自layout外溢；其全量分配空间视图约12.77%落在layout.add，这不是删除有界预检的理由。各入口独立采样，不能用耗时直接相减推导精确CPU成本。

## 方法与限制

GOMAXPROCS1/GOGC100/GOMEMLIMIToff，50ms预热后CPU每入口2s，全量分配每入口5000次、memprofilerate=1；每个入口/kind独立进程。先通过固定代表消息语义/wire/消费长度/padding验证、harness vet与编译，然后采样。保留Go缓存。

runtime.kevent占各CPU profile的15.74%～81.40%；保留完整分母，不将其当网络协议成本，不删除重归一化。2s采样有误差；不能比较不同入口profile的百分比就断言前轮改动带来了多少加速，更不能把profile附带ns/op当普通基准排名。旧反向结果不会被本轮新profile覆盖。

152条流程包括36个采样进程及其文本导出；extra.py核对原始profile/二进制后补充6份编码函数源码视图。原始二进制和profile位于tmp/gosproto-dual-cost-20261021，文本/metadata/哈希保存在报告中。验证仅覆盖相关夹具、harness静态检查与证据一致性；无生产变化，不重复全库/race/fuzz/oracle/GoTS。

```sh
python3 docs/benchmarks/2026-10-21-gosproto-dual-cost/reproduce.py --library /private/tmp/gsr-gosproto-fork --output /absolute/new-output
python3 docs/benchmarks/2026-10-21-gosproto-dual-cost/extra.py --output /absolute/new-output
python3 docs/benchmarks/2026-10-21-gosproto-dual-cost/collect.py --output /absolute/new-output
```

输出目录须不存在。使用完整Git归档，不恢复真实库七项用户删除；正式依赖仍v0.4.0。新诊断与文档留工作树审查，未提交。
