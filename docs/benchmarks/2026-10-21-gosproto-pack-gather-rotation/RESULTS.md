# Pack 直接收集位图：整数数组与组合路径的实际摆放轮换

基线为 gosproto `451998ea0dcf6262bdd4ca0ab6c686d0fb738fac`。候选仍然只把 header 的计算改成 `uint8((nonzero * 0x0002040810204081) >> 56)`；没有新算法或生产代码改动。上一轮整数数组 Pack 两遍约快 1.85%，但 PEncode 未同步改善，因此本轮只复核 game_large/int32_edges 的 Pack/PEncode。

## 控制与范围

三个同模块二进制各包含 baseline/clone/candidate，三者仅按 012、120、201 轮换定义顺序。为了真正轮换基线，将隔离归档里的原 packInto 定义移至实验文件；生产源文件不修改。所有定义内容跨布局一致，candidate 逆变换还原 clone、clone 等于 baseline。三函数仍各 296 条 arm64 指令。

实际相对地址已在任何计时之前验证：

| 布局 | baseline | clone | candidate |
|---|---:|---:|---:|
| 012 | 0 | 1184 | 2368 |
| 120 | 2368 | 0 | 1184 |
| 201 | 1184 | 2368 | 0 |

具体地址来自 assembly.txt，偏移见 offsets.json。它确认了代码位置变化，不等于证明某种硬件根因。没有插入 NOP 或修改输入、分配策略。PEncode 共用正常 encodeBufferWithLimits，三个外层 wrapper 静态调用各自的 Pack。其余库函数、调用点位置未做独立轮换，不能把该实验称为排除全部布局影响。

三个布局的构建、race/checkptr、wire/cap/共享缓冲和 fixture/vet 预检全部完成后，按 mode、case 交错布局计时，每条 case 改变布局起始顺序。每组保留 self、clone、两遍候选。各进程六轮、每轮四个 ABBA/BAAB 四元组；两侧各 50ms 预热，基线 256 次校准目标 20ms，两侧等调用数。GOMAXPROCS=1、GOGC=100、GOMEMLIMIT=off。没有在计时期间编译其他布局。

全部块、GC、暂停、分配、长尾及中位/累计反向保留；不扣控制，不把三种布局池化为校正后的收益。逐布局完整 self 和所有结果见各目录 ALL-CASES.md，汇总见 ALL-LAYOUTS.md。

## 验证与复现

每布局运行已有 TestPack/TestUnpack 与 TestGatherEquivalence（256 mask × 14 长度 = 3584 组），含 wire、输入快照以及真实共享缓冲末端源区域写入，开启 race/checkptr。每个计时进程也先对照 Pack/PEncode wire 和返回容量。源码、fixture、二进制哈希在计时后复核。

本轮无生产变更，未重复全库、官方 oracle、fuzz 或 Go/TS 集成。复现前确认独立库 HEAD 等于上述基线，所有输出目录须不存在：

```sh
python3 docs/benchmarks/2026-10-21-gosproto-pack-gather-rotation/run.py tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-pack-gather-rotation/summarize.py
```

脚本从 `/private/tmp/gsr-gosproto-fork` 导出完整 Git 归档，不读取或恢复实际工作树中的七项无关删除。样本写入本报告目录的 012/120/201，重测前需另存旧输出。prepare.py 与 analyze.py 可单独运行，具体命令见 commands.json。

## 结论

候选继续保留为实验源码，本轮不进入生产。不是因为收益不足某个百分比，而是尚未确认真实组合路径的收益：

- 整数数组 Pack：六次中位中五次下降（−0.24% 至 −1.51%），一次 +0.05%；但累计只有三次下降。上一轮两次约 −1.85% 未在三布局下原幅复现。
- 整数数组 PEncode：六次中位/累计各三次下降；中位 −1.10% 至 +0.41%，方向并不一致。
- 大型 Pack：六次中位中五次下降，累计只有两次下降，不能从中位直接宣称常态吞吐改善。
- 大型 PEncode：012 两次中位改善 −1.55%/−0.77%，120 为 +0.10%/+1.68%，201 为 +0.82%/+1.47%。分离出的 Pack 收益尚未形成稳定的组合收益。

201 的大型 Pack/pass2 累计 −40.38% 全部保留。该组 A 最大块 345.503ms、B 最大块 42.705ms，中位仅 −2.23%；累计大幅变化受到长尾影响。原始块未做剔除，也没有将其当成日常加速。没有采集足以定位这次长尾根因的系统 trace，不擅自归因于 GC、调度或缓存。

上述现象支持“公式等价且有局部正面信号”，不足以支持“纯公式替换会稳定提升业务 PEncode”。仍存在调用点/返回路径位置、机器负载等未单独控制因素，不把所有差异归因于本轮轮换的三个函数。后续不再重复同一公式的全面密度扫描，优先研究普通 word 的实际计算/写出工作量。

共 48 个计时进程、288 轮、1152 个四元组、4608 个计时块；27 条准备命令、48 条测量命令与 3 条独立分析命令全部成功。驱动另记录了准备脚本调用，不能再将其重复算作底层准备数量。正常分配保持：大型 Pack 3456B/1alloc、PEncode 3968B/2alloc；整数数组二者均 4096B/1alloc。所有精确分配遥测在 summary.json 中保留。
