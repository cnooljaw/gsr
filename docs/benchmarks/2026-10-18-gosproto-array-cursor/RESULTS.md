# 原生整数数组的连续游标写出

2026-10-18。上一轮热点诊断已本地提交为GSR `d2be5d8`，gosproto保持`6d854d1`，提交后两库干净、未推送。本轮保留一个局部候选到库工作树，新增代码/记录未提交；正式依赖仍v0.4.0，Decode冻结。

## 变更与编译器证据

只改encodeInt64Slice：先写宽度字节，再用remaining切片连续写4/8字节并前移；返回长度仍为1+width*len(values)。生产Go净增三行，没有新API、依赖、unsafe、pool、cache或generator。预检/宽度选择、具名/unsigned回落、预算、Pack/Unpack、RPC和Decode均未改。

原写出每项使用output[1+index*4:]或*8，同时检查切片起点和固定宽度store。Go1.24.13的BCE报告显示旧272/276行各有IsSliceInBounds+IsInBounds；新273/278行只保留store的IsInBounds，前移切片检查由编译器证明冗余。没有手工删除检查或绕过验证。ARM64符号汇编也减少重复偏移计算；不能仅凭少几条指令承诺速度。

BCE/汇编使用单独诊断构建，编译命令、源码/二进制指纹和两版完整encodeFieldInto文本保留；正式计时用普通构建，不把诊断flag与普通二进制混用。循环只借用本次输出剩余区间，每次写4/8字节，已验证数组长度确保不会越过该字段；后继字段仍由调用方按同一返回长度定位，输出容量和独占规则不变。

## 增量验证

查阅既有native/named、unsigned超界、当前值变化、预算和官方数组用例后，新增一项独立wire回归：4/8字节各20种正长度（1～4096，含895/896/897），包含int32/int64极值、混合符号、后继字节字段；用标准库AppendUint32/64构造完整参考wire，不靠本库decode反推正确。40场景各检查Encode与EncodePacked的bytes/cap、源存储和后续调用不能改旧输出。空数组继续由既有用例保护。

新测试在基线先通过，候选也通过。另建独立负向归档，只把8字节步长故意改成7；新测试在width=8,length=2按预期失败，生产/采样归档未被修改。它验证测试能抓住游标写偏，不是另一个性能候选。

## 固定配对

沿用原19项body Encode/Pack/PEncode/手工帧与19项实际RPC，另加896个混合正负64位数（Min/MaxInt64、刚超int32、−1/0/1）的Encode/PEncode和三种RPC，共43项。wide64是新诊断负载，不能伪装成旧PB/TS正式夹具或官方性能结论。GameState含原生整数数组；AddressBook/Echo与纯Pack作为非目标观察。所有case在计时外核对旧新wire/cap，既有夹具还核对固定哈希。

四次固定运行：self、clone、pass1、pass2，所有43项都有A/A。Go1.24.13 darwin/arm64、Apple M2、GOMAXPROCS=1、GC开启，六轮×四个ABBA/BAAB四元组，目标块10 ms。**16512块、4128四元组、1032配对轮次**独立复算到1e-12一致；不重试、删样本或调整候选/参数。负值表示耗时减少，百分比为配对比值中位数，不是置信区间；ns为每轮两臂几何均值差的中位数，不从独立计时中位数相减。

### body 路径

| 项目 | 第一轮 | 第二轮 | 两轮配对差 ns |
| --- | ---: | ---: | ---: |
| body/game_small/sproto_encode | -2.04% | -1.57% | -15.5 / -11.8 |
| body/game_small/sproto_pencode | -2.88% | -0.29% | -27.8 / -2.8 |
| body/game_large/sproto_encode | -3.29% | -3.05% | -220.2 / -204.2 |
| body/game_large/sproto_pencode | -3.11% | -3.32% | -273.6 / -292.8 |
| body/int32_edges/sproto_encode | -24.98% | -24.89% | -597.6 / -583.9 |
| body/int32_edges/sproto_pencode | -13.89% | -14.05% | -612.7 / -614.4 |
| body/wide64/sproto_encode | -22.94% | -22.61% | -606.9 / -595.5 |
| body/wide64/sproto_pencode | -7.86% | -7.70% | -591.5 / -582.2 |

32位896数组Encode两轮约−25%，PEncode约−14%；混合64位Encode约−23%，PEncode约−8%。大型消息Encode/PEncode也约−3%。小型PEncode第二轮只有−0.29%，clone控制未通过，不能把这项微小变化单独当确定收益。

### 实际 RPC

| 项目 | 第一轮 | 第二轮 | 两轮配对差 ns |
| --- | ---: | ---: | ---: |
| rpc/game_small/notification | -3.26% | -0.98% | -32.8 / -9.8 |
| rpc/game_small/request_cancel | -1.02% | +0.01% | -10.6 / +0.1 |
| rpc/game_small/response | -0.51% | -1.02% | -5.2 / -10.3 |
| rpc/game_large/notification | -2.91% | -2.11% | -259.6 / -186.9 |
| rpc/game_large/request_cancel | -2.16% | -1.68% | -184.1 / -150.5 |
| rpc/game_large/response | -2.29% | -1.31% | -202.3 / -115.8 |
| rpc/int32_edges/notification | -14.23% | -12.39% | -635.6 / -553.9 |
| rpc/int32_edges/request_cancel | -13.34% | -13.03% | -609.7 / -592.4 |
| rpc/int32_edges/response | -13.36% | -12.63% | -599.1 / -567.9 |
| rpc/wide64/notification | -8.25% | -9.77% | -651.7 / -785.9 |
| rpc/wide64/request_cancel | -7.45% | -7.72% | -599.2 / -623.1 |
| rpc/wide64/response | -8.16% | -7.60% | -658.6 / -604.7 |

RPC包含真实header、组帧及有响应请求的登记/CancelSession搭档，区别于手工frame；不含网络、Dispatch或业务Handler。整数数组三种RPC两轮约快12%～14%，wide64约快7%～10%，大型约快1%～3%。小型request第二轮+0.01%基本持平，不概括为20项目项全赢。

### 非目标与代价

| 项目 | 第一轮 | 第二轮 | 两轮配对差 ns |
| --- | ---: | ---: | ---: |
| body/addressbook/sproto_encode | -0.09% | +0.21% | -0.5 / +1.1 |
| body/addressbook/sproto_pack | +0.20% | +0.78% | +0.2 / +1.0 |
| body/addressbook/sproto_pencode | -0.42% | +0.15% | -2.7 / +1.0 |
| body/game_small/sproto_pack | +0.17% | -0.88% | +0.5 / -2.6 |
| body/game_large/sproto_pack | +0.20% | +0.84% | +5.4 / +22.8 |
| body/int32_edges/sproto_pack | -0.16% | +9.45% | -4.1 / +245.2 |
| body/echo_ascii4k/sproto_encode | +0.08% | -0.56% | +0.6 / -4.4 |
| body/echo_ascii4k/sproto_pack | -0.50% | -0.22% | -3.5 / -1.6 |
| body/echo_ascii4k/sproto_pencode | +0.77% | +0.36% | +7.0 / +3.3 |
| body/ascii32/frame | +1.87% | +2.74% | +5.2 / +7.9 |
| body/utf8_1k/frame | -0.07% | +0.03% | -0.6 / +0.2 |
| body/ascii4k/frame | +0.24% | -1.51% | +5.7 / -35.2 |
| body/zeros4k/frame | +0.02% | +1.83% | +0.8 / +65.0 |
| rpc/addressbook/notification | -0.34% | -0.25% | -2.3 / -1.7 |
| rpc/addressbook/request_cancel | -0.19% | -0.92% | -1.4 / -6.6 |
| rpc/addressbook/response | -0.01% | -0.21% | -0.1 / -1.4 |
| rpc/echo_ascii4k/notification | +0.11% | +0.41% | +1.2 / +4.2 |
| rpc/echo_ascii4k/request_cancel | +0.84% | +0.59% | +8.9 / +6.4 |
| rpc/echo_ascii4k/response | +0.35% | -0.00% | +3.6 / -0.0 |
| rpc/ascii32/notification | +2.25% | +0.46% | +3.5 / +0.7 |
| rpc/utf8_1k/notification | -0.18% | -1.78% | -0.7 / -6.6 |
| rpc/ascii4k/notification | +0.93% | -2.44% | +9.6 / -25.4 |
| rpc/zeros4k/notification | -0.62% | +0.01% | -13.4 / +0.3 |

全部43项两轮allocs/op中位数相同，20项目标的B/op也相同。手工ASCII4KiB frame第一轮14616.125→14616 B/op，零字节4KiB frame第二轮14616→14616.125 B/op，两项对象数仍5；其余B/op精确相同。0.125 B/op等于128次窗口的16 B，与已有[tiny/GC边界探针](../2026-10-17-gosproto-pair-resolution/RESULTS.md)现象一致，但本轮未重复机制探针，不能自动把全部尾差归为同一原因，也没有截断计数来过控制。初次报告校验假设B/op全部精确相等而失败，修正为逐项记录尾差，对象计数仍精确核对，见checks.json；没有重采样。32位数组4096 B/1次，wide648192 B/1次；大型仍3968 B/2次（含布局记录），这些是分配计数/尺寸级别，不是slice cap。

32 B手工frame两轮+1.87%/+2.74%（5.2/7.9 ns）代价保留。更显著的是未改的int32_edges纯Pack：第一轮−0.16%，第二轮+9.45%（245.2 ns）；第二轮六个比值均约+7.6%～+9.9%，不能只凭“源码未改”删掉它或称为已解决噪声。

为此只读取两轮原有二进制，未补采样：before/candidate的packInto各300条符号指令，分别在pass1/pass2完全一致（仅移除源位置、绝对PC和机器编码列，符号操作数未改写）。证据见pack-inspection.json和四份完整汇编。它排除两轮Pack发生不同源码/符号指令序列的解释，但不排除绝对布局、输入地址、GC/调度等影响，**原因未确定**；不能归因为新的Pack算法回退，也不能声称所有非目标路径都不变。完整PEncode两轮仍约−14%，不靠选择纯Pack有利轮次作决策。

self/clone均40/43满足原controlAccepted标记，失败如下，所有原值/IQR/GC/分配计数均保留：

- self：body/echo_ascii4k/sproto_pack, rpc/int32_edges/request_cancel, rpc/echo_ascii4k/notification。
- clone：body/game_small/sproto_pencode, body/ascii4k/frame, rpc/ascii32/notification。

按两轮显著数组收益、大型组合小幅改善、不增加分配和极低维护成本保留，同时接受已披露小包代价、控制失败和待解释的纯Pack差值。不使用最低CPU百分比淘汰线，也不将A/A通过标记当作无回归证明。

## 收尾验收与复现

- 库全测、固定官方C/Lua（含既有RPC头/数组向量）、race/checkptr=2、vet通过；官方e93fd5a归档源码/core.so指纹一致。
- Codec10秒有限fuzz通过，474047次，无重试；新长数组用例补足原fuzz仅短Values数组的长度覆盖。Pack算法未改，没有重复Pack随机fuzz或不涉及数值数组的3078条byte-body RPC差分。

- 临时modfile下GSR三包、TS构建和Go/TS Plain/MAC **7/7、零跳过**通过。未运行GSR全仓/race、跨平台或新PB/TS性能矩阵。
- 四份prepare-only归档的全部Go源码/harness/夹具/工具/探针/参数一致，最终库全部Go源码与测量candidate及编译器诊断归档一致。

从GSR根目录复现（输出目录必须不存在）：

```sh
python3 -B docs/benchmarks/2026-10-18-gosproto-array-cursor/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-array-cursor-new
python3 -B docs/benchmarks/2026-10-18-gosproto-array-cursor/analyze.py \
  tmp/gosproto-array-cursor-new
```

库须含6d854d1；candidate.patch固定本轮Go改动，驱动从归档构建，不依赖未提交源码。--prepare-only仅重建输入。compiler-commands.json记录BCE/汇编诊断；pack-inspection-commands.json只分析原已测二进制。二进制/归档源码留在忽略目录，仓库保留原始计时块、摘要、指纹及完整诊断文本。

下一轮保持Pack差值为未解释观察，另行针对大型整数记录数组遍历建立局部候选；不同时混改Pack、不删除预算或跨调用缓存可变schema事实，Decode继续冻结。
