# 整数记录头部固定两字节视图

前轮单项 profile 已本地提交为 GSR `7c91913`，库基线为 `d2f6027`，未推送。本轮只在 `encodeIntegerRecordInto` 的两处头部 store 中，将动态剩余切片改成固定 len/cap=2 的视图：

```go
writeUint16(output[headerOffset:headerOffset+2:headerOffset+2], skip)
// ...
writeUint16(output[headerOffset:headerOffset+2:headerOffset+2], header)
```

字段/tag/presence 遍历、当前整数读取、内容写出、预检预算、布局、Pack/Decode、公开 API 与输出 cap/独占均不改；没有新增分配、unsafe、缓存或池。旧 writeUint16 同样要求当前位置至少两个有效字节；完整预检已确定记录的头数与内容尺寸。临时视图只改变编译器的定位方式，store 地址与顺序相同。

## 计时前证据

未计时归档与实际四个采样二进制都确认：候选 216→212 条符号指令、32B 栈帧不变、panic 调用点均 14 个。两处头视图的动态指针掩码消失，NEG/AND 各从 5 个降到 3 个；没有宣称消除全部边界检查，也不按静态条数推断实际速度。BCE 与原始/整理后汇编见 preflight 和各模式 record-assembly 文本。

第一次准备在测试启动阶段失败：并存归档把 before 的模块改名，原外部测试仍导入正式模块名。失败发生在任何计时之前，原命令保留在 [失败记录](preflight/setup-failed-commands.json)。修正只为两侧已有测试创建独立 modfile，恢复其自身导入；不改采样源码或 go.mod。随后新目录一次运行正式矩阵，没有挑选或重跑计时样本。

基线与候选已有记录/tag/nil/当前值/元信息替换/预算/RPC 契约通过；复用 109 种形态 525 项 body/packed/RPC/预算 wire-cap-独占差分通过。harness 统计、选择、GC、代表夹具、默认指标、70 项名称顺序和 vet 均通过，没有新增重复测试。

## 测量方法

采样前固定 [计划](plan.json)、[唯一候选](candidate.patch)与[驱动](reproduce.py)。原 70 项 × self/clone/两轮比较，280 个独立进程；每进程构造原矩阵后只保留所选操作，清输出引用并 GC，再预热和有界校准。GOMAXPROCS=1、默认 GOGC=100、无限 GOMEMLIMIT；原 sampleBlock/blockCalls 代码不变，6 轮×4 ABBA/BAAB quartet×4 块。分配仍是每臂三次 128 调用观测。

CPU/GC 状态只在计时区间外读取，不剔除异常、不扣控制偏差，不设最低收益比例。所有构建、测试、vet、编译器输出和汇编均在首个计时进程前完成。两轮比较之后再做 fresh 归档及独立复算，不穿插到计时中。

## 结果与保留决定

保留两处视图改动。收益集中在较长的普通记录及部分真实请求：4096条full记录请求两轮配对快2.50%/1.19%（少5,418.2/2,677.4ns），累计也少18,484.1/2,464.2ns，两组控制通过。4096条half记录的Encode/PEncode/请求，以及32×128嵌套PEncode也两轮同向、累计更快；这些约0.5%～1.3%的收益照样计入，不用最低收益比例淘汰。

大型游戏消息本轮收益很小、部分入口翻转，不能说大型业务路径都稳定更快。小型请求只少10.5/14.3ns也保留为观察；是否采用依据是整个低复杂度候选的已见收益、代价及不确定性，不逐项保证每个数字都能复现。

负号表示更快。括号为轮级几何耗时差的中位数（ns），不是用百分比乘累计耗时；原始逐块、轮值、累计及分配均保留在各模式raw/summary和[独立分析](analysis.json)。

| 入口 | 第一轮：比例（差值） | 第二轮：比例（差值） | self / clone控制 |
| --- | ---: | ---: | --- |
| `body/game_large/sproto_encode` | -0.25%（-14.3ns） | -0.97%（-54.8ns） | 通过 / 未通过 |
| `body/game_large/sproto_pencode` | -0.81%（-58.6ns） | +0.12%（+8.9ns） | 通过 / 未通过 |
| `rpc/game_large/notification` | -0.53%（-37.8ns） | +0.07%（+5.0ns） | 未通过 / 通过 |
| `rpc/game_large/request_cancel` | -0.33%（-23.4ns） | -0.09%（-6.6ns） | 通过 / 通过 |
| `rpc/game_large/response` | -0.16%（-11.4ns） | +0.04%（+3.0ns） | 通过 / 通过 |
| `rpc/game_small/request_cancel` | -0.82%（-10.5ns） | -1.57%（-14.3ns） | 通过 / 通过 |
| `growth/flat/full/256/encode` | -1.88%（-185.8ns） | -1.25%（-127.9ns） | 通过 / 通过 |
| `growth/flat/full/256/request_cancel` | -0.67%（-87.3ns） | -1.58%（-218.4ns） | 通过 / 通过 |
| `growth/flat/full/512/encode` | -1.32%（-268.9ns） | -0.90%（-183.3ns） | 通过 / 通过 |
| `growth/flat/full/512/request_cancel` | -0.89%（-241.2ns） | -1.50%（-414.1ns） | 通过 / 通过 |
| `growth/flat/full/1024/pencode` | -0.65%（-341.1ns） | -1.64%（-881.7ns） | 通过 / 通过 |
| `growth/flat/full/1024/request_cancel` | -0.44%（-236.5ns） | -0.75%（-415.3ns） | 通过 / 通过 |
| `growth/flat/full/4096/encode` | -2.05%（-3598.0ns） | -1.10%（-1860.6ns） | 未通过 / 通过 |
| `growth/flat/full/4096/pencode` | -0.88%（-1923.0ns） | -0.76%（-1669.8ns） | 通过 / 通过 |
| `growth/flat/full/4096/request_cancel` | -2.50%（-5418.2ns） | -1.19%（-2677.4ns） | 通过 / 通过 |
| `growth/flat/half/4096/encode` | -0.96%（-1135.3ns） | -1.06%（-1246.5ns） | 通过 / 通过 |
| `growth/flat/half/4096/pencode` | -0.94%（-1521.5ns） | -0.57%（-845.7ns） | 通过 / 通过 |
| `growth/flat/half/4096/request_cancel` | -0.88%（-1316.1ns） | -0.77%（-1135.4ns） | 通过 / 通过 |
| `growth/nested/32x128/pencode` | -1.21%（-2768.0ns） | -1.28%（-3037.6ns） | 通过 / 通过 |
| `growth/nested/32x128/request_cancel` | -1.26%（-2921.5ns） | -0.52%（-1192.4ns） | 通过 / 通过 |
| `growth/nested/32x32/pencode` | -0.13%（-72.6ns） | -0.11%（-66.8ns） | 通过 / 通过 |
| `growth/array_record/4096/request_cancel` | +0.47%（+1509.3ns） | +0.10%（+315.8ns） | 通过 / 通过 |
| `body/game_small/sproto_pencode` | +0.58%（+5.0ns） | +0.70%（+6.0ns） | 通过 / 通过 |
| `rpc/game_small/response` | +0.54%（+4.7ns） | +0.22%（+2.0ns） | 通过 / 通过 |

### 代价、累计翻转和未改路径

含整数数组记录的4096条请求两轮多1,509.3/315.8ns，累计多1,801.0/41,890.2ns，控制都通过；这是接受的局部成本，不因候选只有两行就忽略。小型PEncode多5.0/6.0ns，小型响应多4.7/2.0ns，也全部披露。

至少一轮配对/累计方向相反的入口共27项，完整见[方向翻转](cumulative-reversals.json)。例如256条full请求第一轮配对少87.3ns、累计多1,346.2ns；512条full请求第二轮配对少414.1ns、累计多444.2ns；1024条full请求第一轮配对少236.5ns、累计多7,373.1ns。4096条full PEncode第二轮配对少1,669.8ns、累计多7.5ns，32×128嵌套请求第二轮配对少1,192.4ns、累计多12,099.7ns。大型请求第二轮配对少6.6ns但累计多0.2ns，不能把微小比例当作确定结论。

纯Pack未改仍有波动：AddressBook Pack两轮慢3.75%/4.05%，绝对多4.0/4.3ns，控制均通过；整数数组Pack却快3.40%/0.69%。四个实际二进制的Pack均296条符号指令且规范化指令序列相同，见[指令核对](unchanged-pack.json)。这只能证明函数指令未变，不能排除代码布局、运行时或其他因素，也不能把这些变化归功于头部优化或从目标结果中扣除校正。

### 控制、分配与离群

self63/70、clone62/70通过；失败名单与轮值全部保存，不用上一轮控制数替代。1,120个快照的limiter均零，最大堆对象快照8,646,048B（约8.25MiB），不是峰值，也不能证明没有GC或其他长尾。

分配中位数self70/70、clone69/70、pass1 68/70、pass2 69/70相同；不一致都是128调用观测中16B尾差，mallocs仍5：clone ASCII手工帧A为14,616.125/B为14,616；pass1 ASCII帧B高、zero帧A高；pass2 ASCII帧A高。所有目标分配不变，不把尾差解释成新增缓冲，也不说所有观测完全一样。

最大单轮self偏差为wide64 PEncode +88.47%，clone为AddressBook响应+36.58%，pass1为32B通知+67.20%，pass2为zero通知+48.93%。[最长块](long-blocks.json)分别为self wide64 PEncode B块262.349ms（1,657调用/5次GC）、clone wide64通知A块172.010ms（1,588调用/5次GC）、pass1 512条full请求A块155.901ms（389调用/3次GC）、pass2 zero通知B块254.571ms（5,773调用/9次GC）。没有删除、重跑或按GC数修正这些块，GC完成次数不能解释唯一原因。

## 验收与复现

独立复算全部280进程、26,880块、6,720 quartet、1,680轮及1,120快照，误差约束1e-12；逐项JSON与合并数组、计时次数/顺序、sampleBlock/blockCalls未变、源码/生成输入/fixture/driver/probe/二进制指纹及fresh归档均核对。全部正式计时进程各运行一次；第一次模块导入失败的旧准备目录没有计时调用，原失败也保留。

生产库仅 `encode_integer_record.go` 两行替换；gofmt无变化，18个Go文件与采样候选逐字一致。库全测/官方C-Lua、race+checkptr=2、vet、10秒Codec fuzz（436,240次、没有新增interesting）、GSR三相关包、Go/TS集成7/7零跳过全部通过，见[验收命令](acceptance.json)。没有改Pack算法，不重复Pack随机fuzz；没有新PB/TS排名。官方oracle复用历史SHA一致的core/facade，parser来自只读Skynet且单列来源，未冒称重新获取官方源码，见[来源](oracle-provenance.json)。临时modfile与服务器module-info确认本地库接线，正式go.mod/go.sum不变。

库新实现与GSR新文档均未提交；未推送、发布或升级依赖。Decode继续冻结。下一轮只静态检查inline整数是否仍提前构造内容切片，确有重复定位时再做单一写出候选；不叠加预检、布局或Pack改动。

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-record-header/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/record-header-repro
python3 -B docs/benchmarks/2026-10-19-gosproto-record-header/analyze.py tmp/record-header-repro
python3 -B docs/benchmarks/2026-10-19-gosproto-record-header/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/record-header-fresh --prepare-only
```

可审查的原值/汇编/命令及清单保留在本报告；280份逐项原值和二进制留在忽略目录 `tmp/gosproto-record-header-v2/`，逐项SHA在各模式inputs中。文本汇编只去行尾空白，原始/整理后哈希分开记录在validation；独立复算、fresh和正式源码核对见 [validation.json](validation.json)。
