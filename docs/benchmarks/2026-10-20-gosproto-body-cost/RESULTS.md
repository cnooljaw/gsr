# 当前Body Encode/Decode成本诊断

2026-10-20。上一轮tag候选已提交GSR `c4f06ee`，未推送；本轮只读使用生产提交af9169d，不携带countChunks/tag/word写出等未采用候选。**没有生产修改或新性能排名，诊断与文档未提交。**保持Body与Packing双主线，目标向TS相对PB领先程度靠近。

## 方法与验证

AddressBook、大型游戏、896整数数组，分别采PB/SP纯Encode/Decode的CPU与全量分配，12个CPU进程＋12个allocation进程。输入/生成类型与当前72项基准相同，计时前验证两协议完整语义、wire SHA/消费长度/padding。每次Decode新建独占结果，不复用对象，不删presence/预算/失败原子性。

CPU每项约2秒，手动在构造/GC/50ms预热之后开启；allocation每项固定5000次、memprofilerate1，**其profile含构造/预热，loop B/op及allocs/op是ResetTimer之后的计数**。不能把带profile的ns/op放进原性能表，尤其全量分配跟踪开销很大；也不能按profile对象占比直接分摊每次逻辑分配数。

所有CPU份额保留各自完整样本分母，包含GC/kevent/systemstack；父子累计不相加，不剔除运行时后重新归一化。不同进程份额不是同场速度比较或硬件根因证明。

## 稳定的loop分配形态

| 消息 | PB Encode B/次 | SP Encode B/次 | PB Decode B/次 | SP Decode B/次 |
| --- | ---: | ---: | ---: | ---: |
| addressbook | 80 / 1 | 144 / 1 | 648 / 26 | 528 / 29 |
| game_large | 2304 / 1 | 3968 / 2 | 13560 / 537 | 10416 / 532 |
| int32_edges | 2688 / 1 | 4096 / 1 | 4160 / 2 | 8288 / 4 |

表中B/次表示B/op / allocs/op。与前轮无profile基准一致。大型SP Decode并没有比PB分配更多对象或字节，因此不应以“先删几百个对象”为默认方案。optional标量指针、各Item对象、独占数组和失败原子性的临时根都有契约意义。将Item或标量合并进一个共享backing可能改变持有单个对象时的存活范围，不能悄悄作为无行为变化优化。

数组的Go表示也不同：SP生成类型为[]int64，PB为[]int32，虽然896个数值语义相同，底层数组宽度和分配size class不同。不能把SP的8288B vs PB4160B全部解释为编码器写得不好，也不改基准类型来隐藏差异。Body执行效率仍可独立优化。

## 大型Decode：构造与执行

SP完整CPU样本中decodeMessage累计61.40%、decodeStructSlice57.02%、reflect.New13.16%、reflect.Value.Set5.70%、runtime.mallocgc25.00%；父子不能相加。PB同轮独立profile中reflect.New累计9.71%、mallocgc29.13%，不能用这两个进程的比例差计算纳秒差或认定SP堆分配必然更慢。

SP allocation profile的flat对象记录主要落在decodeMessage58.40%、reflect.New38.48%；这不是“532次逻辑分配中58%可删除”。profile包含预热/初始化，且Go tiny allocator的逻辑小对象计数与profile分配块口径有区别；本机固定Go runtime mstats.go明确在MemStats里额外计入tiny allocations。精确的每次分配计数使用上表benchmark，profile只辅助定位归属。

对照固定protobuf-go v1.36.1的consumeMessageSliceInfo，PB同样reflect.New子消息，然后走pointer unmarshal与AppendPointerSlice；consumeUint32Ptr仍在无值时new标量并进行类型化指针写入。故**不能把消除reflect.New本身当作唯一方向**。更值得隔离的是构造后保留相同对象数量，如何减少SP的反射取值/赋值、重复类型处理与通用字段执行成本。

## 大型Encode：预检与写出都仍在热路径

SP完整CPU样本中encodeMessageInto累计32.60%、encodeFieldInto30.40%、encodedSize25.55%、encodedIntegerRecordSize15.42%、encodeIntegerRecordInto17.62%。预检/写出父子关系仍有重叠，不把这些数全部相加。reflect.Value.Index和UnsafePointer也有样本；不是因为Body已快PB约28%就无可改进。

后续Encode候选应围绕整数记录的字段定位、受控地址借用与布局执行，保持完整预算/预检和即时元信息证明，不缓存业务字段值或跳过失败前检查。不能把最终缓冲已融合等同于正文执行已经最优。

## 下一项隔离实验

优先为Body Decode验证**同构造、同对象数量，只缩短结构数组目标槽写入**：比较当前reflect.Index/Set与有实际Go元素类型证明的指针槽写入。先做私有隔离原型，不建立公共factory/注册层，不引入池、arena或共享对象；继续使用通用parser与现有字段元信息契约。

若原型借用底层切片，必须先证明实际目标确为相应指针元素类型、长度/cap和元素尺寸正确，公开字段描述变化时回退，地址仅在本次同步调用借用；类型化指针写入须保留GC写屏障，禁止uintptr缓存或memcpy指针。正确性、GC/race/checkptr和真实Decode/PDecode/Dispatch都要验证后再考虑生产采用。**本轮没有实现该原型，也不预告收益。**若目标类型或安全证明成本抵消收益，保留结果，不用固定比例门槛裁剪小收益。

Packing主线继续面对Unpack组合缺口，此诊断不冻结它；也不根据单个局部候选的失败宣布Go难以接近TS。较大公开API/对象所有权变化仍须另行裁决。

## 来源、失败记录与限制

104条完整流程命令成功：fixture测试、vet、构建/模块解析、24个profile和各pprof摘要。另保存首次提取失败：SP/reflect专用正则在PB Encode profile没有匹配，导致源码摘要提取返回非零。修正为包括两端操作闭包，重新完成独立诊断；原失败命令、源码、profile摘要/元信息/raw哈希保留在setup-failure，不将失败采样混入主分析，不是为选更快结果重测。

fresh源/探针/夹具和完整18份Go源码、生成依赖、二进制/raw哈希核对；模块解析确认本地af9169d归档和PB v1.36.1。正式go.mod/go.sum不变，原库6个已有删除保持，不称库工作区干净。没有新实现，不重复完整库/oracle/race/fuzz/GSR/GoTS验收；不重新测TS或替换72项基线。

[源码参考](source-references.json)、[全部符号与loop复算](analysis.json)、[验证指纹](validation.json)。原始profile/二进制在忽略tmp，文本摘要和来源随报告保存。不提交新报告、不推送/发布/tag，双主线继续。

```sh
python3 -B docs/benchmarks/2026-10-20-gosproto-body-cost/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-body-cost-new
python3 -B docs/benchmarks/2026-10-20-gosproto-body-cost/analyze.py \
  tmp/gosproto-body-cost-new
```

输出目录必须不存在；--prepare-only只准备输入。默认24个独立profile按固定顺序采集，不同时运行计时基准或其他重型检查。
