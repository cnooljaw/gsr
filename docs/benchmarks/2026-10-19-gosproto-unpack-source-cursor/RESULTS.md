# 普通 Unpack 仅改源游标

2026-10-19。前轮已本地提交为gosproto `f7022ac`、GSR `b3d4517`，未推送。本轮保留普通mask逐字节读取的源游标候选，**不设最低收益比例**；约1%的真实组合收益与稀疏形态代价同时记载。新实现和本报告未提交，正式依赖v0.4.0，Decode继续开放。

## 定位、历史与实现

先在f7022ac单独采[两种整数数组Unpack/PDecode CPU](../2026-10-19-gosproto-unpack-current-profile/RESULTS.md)，4个进程/12快照。8字节PDecode中Unpack累计43.23%、数组decoder7.42%；4字节为35.00%/13.75%。这只支持选题，不把父子累计相加、不剔除kevent或按份额预测提速。

核对过旧[2026-10-16 cursor候选](../2026-10-16-gosproto-unpack-candidates/RESULTS.md)：它同时建立固定输出word、给TrailingZeros8结果加&7并改源游标。本轮没有恢复这组组合。只在原目标计算 `unpacked[offset+index]` 不变的前提下，以 `used` 读取 `src[used]`，每个word结束后一次 `src=src[used:]`，替代逐有效字节切片；不改目标视图、mask提取/清位或新增密度分支。官方C与锁定Protobuf来源、采用思想及限制见[固定源码参考](../2026-10-19-gosproto-unpack-current-profile/source-references.json)。

普通header为1..254、有效字节数1..7。完整预检已确认对应payload和输出范围；used顺序为0..popcount−1，恢复顺序与原来相同，最后恰好消费popcount字节。源读取/目标写入边界检查保留。零段、FF、完整预检、错误优先级、预算、精确输出分配/清零、输入/输出独占、Pack和Decode均不改；没有新unsafe、表、池、缓存或API。

实际ARM64普通循环回跳由−22到−15条指令，源切片len/cap/指针调整移到word外；整个UnpackWithLimit仍384条，栈帧128→112B。边界panic调用点仍5个index/8个slice；静态计数不是速度承诺，见[assembly-summary](assembly-summary.json)。

## 固定70项配对

原52项（含4/8字节大/小数组、游戏/AddressBook/Echo、PDecode及真实Rpc.Dispatch、growth与Encode控制），追加当前全部18种Unpack形态：三档零/FF、1..7有效字节word、FF接稀疏、三种短零段交替及随机输入。用于保护旧cursor暴露的稀疏问题，不按某个fixture mask特判。

self/clone与两轮compare共280独立进程、26,880块、6,720个ABBA/BAAB四元组、1,680轮、1,120快照。所有构建/测试/vet/机器码检查先于第一块计时；冻结plan/probe/patch/工具/源码/二进制，不修改或重采择优。GOMAXPROCS1、GOGC100、无限GOMEMLIMIT，释放构造闭包、清三个sink、GC/50ms预热、原校准/10ms块、每侧3×128分配观察不变。

下表为六轮配对时间比中位数，负值更快；完整结果包含累计差值与每项控制，见[70项完整表](ALL-CASES.md)及[独立复算](analysis.json)：

| 入口 | 第1轮变化 | 第2轮变化 |
| --- | ---: | ---: |
| AddressBook PDecode | -0.89% | -0.34% |
| 小型游戏 Unpack | -6.33% | -6.08% |
| 小型游戏 PDecode | -1.67% | -1.82% |
| 小型游戏 Dispatch | -1.73% | -1.20% |
| 大型游戏 Unpack | -6.33% | -6.15% |
| 大型游戏 PDecode | -1.16% | -1.42% |
| 大型游戏 Dispatch | -1.23% | -1.74% |
| 4字节×896 Unpack | -6.59% | -6.48% |
| 4字节×896 PDecode | -4.69% | -3.34% |
| 4字节×896 Dispatch | -4.48% | -4.90% |
| 8字节×896 Unpack | -5.44% | -4.92% |
| 8字节×896 PDecode | -4.16% | -4.00% |
| 8字节×896 Dispatch | -1.49% | -0.52% |
| 4字节×3 PDecode | +2.44% | +0.25% |
| 8字节×3 PDecode | +0.11% | +0.89% |
| ASCII4KiB PDecode | -0.11% | +0.11% |
| 单字节word×256 | -0.51% | +1.73% |
| 双字节word×256 | -10.30% | -8.49% |
| 七字节word×256 | -19.11% | -21.51% |
| FF512接稀疏4096 | +11.47% | +10.55% |
| 短零段8word×64 | -0.65% | +14.28% |

采用依据为结构/数值真实PDecode的重复收益、来源不变的RPC组合，以及局部改动没有新增状态或分配模型：大型PDecode约−231/−283ns，即便只有约1%也保留；小型游戏PDecode约−35/−38ns；4/8字节大数组PDecode均有数百ns改善。AddressBook约−10/−4ns也列出，不因比例小抹掉。大型PDecode为12/12轮更快，4/8字节数组PDecode分别10/12、11/12轮更快；大型PDecode两类控制失败（含字节窗口尾差），不把它写成所有控制都通过。

## 代价、控制和分配

FF接稀疏的+11.47%/+10.55%约+430/+400ns，不能隐藏。该项clone同源码为+14.14%、控制失败，但不扣除它来制造“校正后收益”；两个compare各六轮均更慢。短零段8word第二轮+14.28%约+180ns、两类控制均通过，源代码路径未改也不自动把候选整体机器码影响归零，具体原因未定位。没有通过夹具分支修补这些代价，采用的取舍是保留结构/数值业务组合收益，同时明确这些输入分布需要后续独立观察。

单字节word第一轮−0.51%、第二轮+1.73%，第二轮累计+103.4ns且有1.2808最差轮，clone控制失败；没有恢复旧实验约12%的同一结论。2..7字节word两轮约快8.5%～21.5%，这支持缩短内循环，但这些合成形态不代表线上分布。ASCII body组合基本持平；小数组与短帧数ns代价保留。未改pure Decode也有成本，例如含数组记录+0.62%/+0.46%、短整数Decode约+1%～2%；不将它们删掉或强行归因到源游标。

self59/70、clone52/70；多项控制失败和所有轮次保留。对象数中位全部相同，27项字节窗口尾差完整列在[allocation-tails](allocation-tails.json)；例如4字节Dispatch有0.125B/op尾差，不是增加一个逻辑对象。大型PDecode仍13872B/533次，4字节PDecode12384B/5次；不宣称减少分配。

limiter初值/增量均零，最大堆快照21,678,592B（约20.67MiB），不是峰值。17个入口有中位/累计反向；8字节Dispatch首轮中位−1.49%而累计+73.8ns，第二轮两者更快。含数组记录PDecode首轮中位−1.57%、累计−134146ns，不能把累计离群当典型收益。最长块分别107.9/40.8/137.1/37.6ms，最差轮与全部GC块见[long-blocks](long-blocks.json)、[worst-rounds](worst-rounds.json)。不删除、归一化或将外部调度/GC当作唯一解释。

## 正确性与收尾

复用已提交的TestPack全mask独立预期、不同高位字节、八种输入偏移、普通word/FF后继，以及UnpackOutputBudget、坏输入、ownership/limits和RPC用例；没有新增镜像生产测试。f7022ac已含上一轮强化40组数组Decode测试，两侧归档原封相同；本轮不再叠加测试补丁，消除了上轮归档git apply跳过的前提。

八条收尾命令成功：库全测并启用固定官方oracle；race+严格checkptr；vet；PackRoundTrip、CodecRoundTrip、Decode各10秒有限fuzz；临时modfile的GSR三包；真实Go/TS7/7、零skip。三项fuzz分别3498/501185/15324次，Pack与Decode均在三秒后无新增执行，有限时间正常结束；不盲目重试、不把它说成持续高吞吐或长期fuzz保证。完整原文见[acceptance](acceptance.json)。

冻结被测候选的全部18份Go源码与当前主库逐字节相同；baseline全部等于f7022ac，fresh/生成输入/工具/夹具/采样函数/顺序/raw/二进制已核对。正式go.mod/go.sum保持HEAD、require v0.4.0，Go/TS仅使用临时replace，实际[二进制来源](integration-build-info.txt)已保存。不重测PB/TS总体排名或跨架构性能。

新实现与本轮诊断/报告未提交；没有推送、发布或打tag，Decode保持开放。若继续，应先针对FF接稀疏/短零段代价做独立执行诊断，避免把同源码控制或未改语句自动等同于零成本，也不向当前实现添加按fixture优化的补丁。

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-unpack-source-cursor/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-unpack-source-cursor-new
```

输出目录须不存在，--prepare-only仅建归档。原始逐项进程文件、二进制和源副本在忽略tmp目录；聚合raw、独立统计、输入/工具/汇编/验证指纹随报告保存。
