# 固定原生整数数组读取视图

2026-10-19。前轮Decode热点诊断已本地提交为GSR `16d1491`，未推送。本轮从库 `503a725` 继续，只收窄原生 `[]int64` 两处元素读取，**保留候选**。实现与新报告尚未提交；正式依赖仍为v0.4.0，Decode继续开放。

## 实现与边界

将 `readUint32(data[index*4:])` 和 `readUint64(data[index*8:])` 改成长度/容量固定为4或8的元素切片。仍先校验宽度、整除和元素预算，再一次性精确分配；4字节保持int32符号扩展，8字节保持原位模式。空数组、命名类型/其他数值类型反射回退、错误顺序、消费长度、失败原子性和结果独占均不变。不增加unsafe、池、parser、生成器或公开API；Pack/Unpack及分配方式不改。

证明依赖现有边界：width只能4/8；count=len(data)/width且整除；index<count，因此(index+1)*width<=len(data)，固定窗口访问同一元素，不借用后继字段。ARM64实际decodeIntSlice从360条降至348条，栈帧160B、写屏障保留；开放尾切片NEG/ASR/AND调整消失，边界/溢出保护仍在。parser仍660条/320B。不以静态条数推演速度。源码依据沿用[上轮固定来源和哈希](../2026-10-19-gosproto-decode-current-profile/source-references.json)，本轮不冒称重新跑官方性能或PB。

扩展既有TestNativeIntegerArrayWire：原来的4/8宽度×20长度共40组合已检查编码；本轮增加同一fixture的Decode、DecodePacked、消费长度、值和各结果/wire独占。测试在改动生产实现前先通过。已有原生/命名短数组、畸形宽度/长度、预算、presence/GC/Fields回落仍复用原测试，不新增重复夹具。

## 固定实验与主要结果

原40入口加3元素4字节、3元素8字节、896元素8字节三种夹具，各包含Decode/Unpack/PDecode/真实Rpc.Dispatch，合计52项。样本先验证wire/值/消费长度；每次新建独占结果，不复用目标。四模式self/clone/pass1/pass2，共208独立进程、19,968块、4,992个ABBA/BAAB四元组、1,248轮、832个计时外快照。全部构建/检查先于采样，不重试、删样、扣控制或事后改矩阵。

GOMAXPROCS1、GOGC100、无限GOMEMLIMIT；清三个sink、GC、50ms预热、原校准与10ms目标块、每侧3×128次分配观察均不变。全部原始结果与独立复算见[analysis](analysis.json)和[完整52项表](ALL-CASES.md)。下表是六轮配对时间比中位数，负值更快，不是旧profile耗时或PB比较：

| 入口 | 第1轮变化 | 第2轮变化 |
| --- | ---: | ---: |
| 4字节×896 Decode | -25.88% | -26.84% |
| 4字节×896 PDecode | -6.86% | -8.44% |
| 4字节×896 Dispatch | -8.77% | -8.76% |
| 8字节×896 Decode | -28.09% | -28.50% |
| 8字节×896 PDecode | -1.17% | -7.52% |
| 8字节×896 Dispatch | -4.25% | -7.03% |
| 4字节×3 Decode | -1.46% | +0.11% |
| 4字节×3 PDecode | -0.36% | -2.92% |
| 8字节×3 Decode | -1.31% | +0.90% |
| 8字节×3 PDecode | +1.95% | +0.81% |
| 大型游戏 Decode | -1.77% | +2.47% |
| 大型游戏 PDecode | -1.37% | +0.12% |
| 小型游戏 Dispatch | -1.19% | -1.68% |

两种大数组纯Decode各12/12轮更快。4字节组合PDecode/Dispatch也各12/12轮更快，支持采用这两处局部收窄。收益不依赖最低比例门槛；小型Dispatch约1%收益也保留。大型游戏并无稳定收益，不把其第一轮改善归结为已解决大型解码。短8字节PDecode的+1.95%/+0.81%约+5.6/+2.4ns，短4字节Dispatch−2.02%/+2.46%等代价也不隐藏。

8字节大数组PDecode首轮中位仅−1.17%，累计反而+7,157.1ns；其最差轮1.519875，第二轮中位−7.52%、累计−1,430.5ns。它不支持“8字节组合路径稳定快7%”的结论。4字节Decode首轮中位−25.88%而累计只−141.4ns；大型Decode第二轮中位+2.47%、累计−54,806.3ns。差异原样保留，不用累计异常或去尾均值美化。

## 控制、分配与异常

self21/52、clone23/52通过既有标记；多项目标控制失败，尤其两种大数组纯Decode两类控制都失败，4字节PDecode/Dispatch亦失败。未改Unpack/Encode也有波动。因此本轮是当前darwin/arm64机器上的局部工程证据，没有总体精度承诺或跨平台推断。采用依据为两个数组宽度重复出现的逐轮收益、真实4字节PDecode/Dispatch收益，以及两行改动没有新增状态/分配模型成本；不是把控制失败忽略成全面无回退。

所有对象数中位相同，存在29项字节窗口尾差，全部列在[allocation-tails](allocation-tails.json)。例如4字节Dispatch候选多0.125B/op、8字节Decode首轮候选少0.125B/op，均不是省/增一个逻辑对象。4字节Decode仍8288B/4次，PDecode12384B/5次；大型Decode10416B/532次。没有宣称分配优化。

limiter初值/增量均零。最大堆快照19,332,056B（约18.44MiB）出现在clone，是快照不是峰值。19个入口有某轮中位/累计符号翻转。各模式最长块约142.9/158.9/254.2/212.5ms，全部GC块与离群保留；最差轮及长块见[worst-rounds](worst-rounds.json)、[long-blocks](long-blocks.json)。不将其统一归因为GC或外部调度。

## 测试补丁归档偏差与纠正

冻结驱动原计划把相同contract-test.patch叠加于两侧；实际在GSR工作树内的子目录调用git apply时静默跳过，退出0。第一轮验收的全源码指纹核对发现了差异。**测量归档中的生产候选正确、测试文件仍为原版**；当时两侧通过的是原有目标测试，不声称扩展40组合在配对前双侧执行。

未修改计时归档、其输入指纹或已采样数据，也没有以修复为由重采择优。扩展测试已在正式基线实现上先通过；随后另建基线副本，仅叠加完全相同的扩展测试，再次通过。当前候选的完整库/官方/race/checkptr验收均使用扩展测试。accept.py强制检查当前生产源码逐字节等于被测候选，只有limits_test.go按所存补丁不同；见[source-verification](source-verification.json)。最初失败只是验收前的指纹断言，尚未启动测试，不伪称已完成验证。

保留冻结reproduce.py用于追溯原执行；新增reproduce-checked.py使用patch并立即校验预期测试SHA，绑定原驱动与自身指纹。本轮只对checked入口做prepare-only及四模式源核对，未用它替换测量或宣称新计时。它修正的是复现工具，不改变生产结论。

## 收尾验收与复现

8条收尾命令全部成功：强化基线40组合；候选库全测并启用固定官方oracle；race+严格checkptr；vet；CodecRoundTrip/Decode各10秒有限fuzz；临时modfile的GSR三包；Go/TS真实Plain/MAC等7/7、零skip。fuzz分别436400/75091次，Decode在3秒后未新增执行，有限时间正常结束；保留该限制、不盲目重试，不宣称长时间持续吞吐。未重测未改Pack随机fuzz、PB/TS总体排名或其他架构。

[验收命令](acceptance.json)、[oracle来源](oracle-provenance.json)、[服务端实际构建信息](integration-build-info.txt)及[validation](validation.json)保存指纹。正式go.mod/go.sum与HEAD相同、依赖v0.4.0；Go/TS仅用临时replace。实现与本轮文档未提交，不推送、不发布、不打tag。

建议从GSR根目录使用带测试补丁检查的入口，输出目录须不存在：

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-decode-array-view/reproduce-checked.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-decode-array-view-new
```

加--prepare-only只准备输入。原采样使用冻结reproduce.py；原版/fresh的生产及原测试源码、二进制/raw/工具/夹具/生成输入SHA已核对，checked-fresh另存。原始逐项进程、二进制和源码副本仍在忽略tmp目录。Decode保持开放；下一轮若继续组合路径，应单独profile当前8字节密集数组Unpack，而非同时更改元素读取和packing。
