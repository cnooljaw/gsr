# 整数记录 Decode：独立 header 游标

基线为 gosproto `9048cfa`。完整 Git 归档建立同模块实验，不恢复真实工作树的七项无关删除；正式 GSR 依赖仍为 v0.4.0。

候选只改变 decodeIntegerRecord 的 header 遍历：在 count、header总长与字段预算检查之后，取得 `chunk[2:total:total]`，每次读取两字节再前移游标，替代按 index 反复计算两端位置。内容游标 total 仍在原 chunk 上独立前移。header 区域长度由 uint16 count×2 构造，必为偶数，`len(headers)>=2` 恰好遍历 count 项，不静默放过非法尾部。

内容区长度/4与8字节验证、未知字段跳过、tag、字段查找、预算扣减和错误顺序保持；每个已知字段仍单独 new(int64)，指针写入保留 Go 写屏障，没有对象合并、池化、新 unsafe 或接口变更。header 游标不跨调用保存。

arm64 parser 符号从388条降到384条，栈帧176B不变；静态差值不当成每次 Decode 的固定加速比例。

## 方法与验证

三个 parser 位于同一模块与同一二进制，外层设置模式；公共 decodeIntegerRecord 通过实验选择器调用，每种模式有相同的间接入口成本。源码不作为正常入口绝对排名。AddressBook、小型/大型游戏、整数数组、4KiB ASCII Echo均测Decode/PDecode；主要目标为大型整数记录数组，其余作为非目标对照，不声称每条消息都经过候选。

self、clone、两遍候选；每进程六轮×四个ABBA/BAAB四元组，两侧各预热50ms，基线256次校准目标20ms，等调用数。GOMAXPROCS=1、GOGC=100、GOMEMLIMIT=off。所有GC、暂停、分配、长尾保留，无控制相减或摆放轮换。

复用现有 parser 差分夹具，比较合法值/未知字段、截断、错误值、预算状态、已解码字段、输入快照及可变描述；结合既有budget/native/schema演进用例，开启race/checkptr。harness fixture、10项wire/消费与结果预检、vet/build通过。clone等于基线，候选逆变换还原基线；源码/fixture/生成文件/二进制哈希复核。没有新增重复行为夹具。

```sh
python3 docs/benchmarks/2026-10-21-gosproto-decode-header-cursor/run.py --library /private/tmp/gsr-gosproto-fork --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-decode-header-cursor/analyze.py --output tmp/新的空目录
```

输出目录须不存在，分析会写入本报告目录，复测前另存原始记录。全部控制、两遍结果及逐块telemetry见ALL-CASES.md和各JSON。

## 结果：保留候选，暂不应用

| 场景 | 第一遍中位/累计 | 第二遍中位/累计 |
|---|---:|---:|
| 大型 Decode | −0.72% / −3.12% | +1.44% / +7.78% |
| 大型 PDecode | −1.53% / −3.61% | −0.16% / −1.41% |
| 小型 Decode | 约0% / +0.55% | −0.43% / −10.87% |
| 小型 PDecode | −0.28% / −2.28% | +0.96% / +1.23% |

负数表示B耗时更少。大型PDecode两遍中位和累计均改善，这是保留候选的局部证据，不因幅度小就删除；大型Decode反转尚未解释，因此暂不修改生产实现。第一遍小型Decode的极小正中位保留在analysis.json，不用舍入后的 +0.00% 宣称持平。

大型Decode clone中位+1.03%、IQR4.82pp，self −1.17%、IQR3.68pp；大型PDecode clone −0.20%、IQR2.22pp。控制不相减。纯整数数组等不走记录parser的消息也出现约1%变化，说明不能把所有差异归因于此改动。Echo PDecode第一遍+10.11%/累计+20.31%，第二遍−2.90%/累计−31.76%，原样保留；没有采集系统trace，不将波动擅自归因于GC或调度，也不据此宣称候选改变了Echo解码逻辑。

40进程、240轮、960四元组、3840块，60条实验流程成功。大型正常分配保持Decode 10416B/532alloc、PDecode 13872B/533alloc；所有原始遥测尾数和GC保留。未改动实际库或GSR依赖，上一轮inline编码9048cfa继续保持，七项无关删除未动。没有运行额外全库、官方oracle、fuzz或Go/TS生产验收，因为候选尚未落地。

后续若复核，应限定大型Decode/PDecode并控制块耗时和实际摆放，不把非目标消息的波动当作候选效果；也不为追求更少分配合并独立presence指针或牺牲GC写屏障。
