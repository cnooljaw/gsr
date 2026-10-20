# gosproto Decoder 原生整数指针的受控写入提案

当前目标（2026-10-20，用户裁决）：同时推进Body Encode/Decode与Pack/Unpack，最终相对各自PB的领先幅度向TS结果靠近。已小幅领先不等于Body完成；Unpack组合缺口也不构成冻结正文的理由。Go/TS逐入口参考、边界与执行规则见[双主线目标](GOSPROTO-DUAL-TRACK.md)。

当前Body实现（2026-10-20）：[结构数组受控指针槽](benchmarks/2026-10-20-gosproto-decode-struct-slots/RESULTS.md)已保留并提交独立库 e43df35。相同对象构造下大型Decode−3.50%/−3.63%、Dispatch−5.80%/−3.04%，长结构重复改善、对象数中位不变；AddressBook PDecode+2.56%/+2.07%及全部控制/累计反向保留。新低GOGC/命名类型/元数据回退/异构指针拒绝回归、全库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过。新增unsafe仅同步借用已核对类型的新切片槽，GC屏障与反射回退保留。前轮成本诊断已提交06adf22，正式依赖v0.4.0不变，原库6项已有删除不动；Body与Packing继续。

当前Body诊断（2026-10-20）：[正文构造与执行成本](benchmarks/2026-10-20-gosproto-body-cost/RESULTS.md)完成PB/SP三消息的12CPU＋12allocation独立profile。大型SP Decode仍10416B/532次、PB13560B/537次，不能默认删对象；PB子消息数组也用reflect.New。下一项隔离同构造/同对象数量的结构数组目标槽写入，保留类型证明、GC屏障、错误/预算/ownership，不新建公共factory或池；原型本轮未实现。Encode的记录预检/写出继续跟踪，Packing不冻结。前轮tag报告已提交c4f06ee，新诊断未提交，生产源af9169d及用户已有删除保持。

当前Body实验（2026-10-20）：[非负tag计数表示](benchmarks/2026-10-20-gosproto-decode-tag-counter/RESULTS.md)完成固定70项两轮，去掉查找负数检查但总指令仍660。大型Decode−0.08%/−4.51%，首轮累计却慢139.8ns；大型PDecode/Dispatch两轮略慢，小型Dispatch−0.83%/−0.17%及全部小收益保留。候选待确认、暂不应用，不设最低收益门槛。新增10组最大tag/跳跃链公共API对照及既有契约/差分/来源核对通过。前轮Packing报告已提交36f03f2，新报告未提交；生产Go源仍af9169d、用户原有删除不动，双主线继续。

当前Body实验（2026-10-20）：[countChunks直接计数](benchmarks/2026-10-20-gosproto-chunk-count/RESULTS.md)已完成固定70项两轮比较。大型Decode观察−0.72%/−0.10%，部分结构数组有小收益；self8/70、clone11/70，组合/累计多有翻转，收益尚不能可靠区分。候选保留待确认、未应用生产，不因低于比例门槛丢弃。既有契约/差分、fresh与来源核对通过；库原有删除保持，正式依赖不变。上一轮双主线目标/对照已提交386727c，新实验未提交，Body与Packing继续开放。

状态：2026-10-17，范围已按用户“提交，继续”采纳并完成最小实现，已本地提交为 gosproto `fc3246a`、GSR `862fa8b`。此前安全 inline 与提案/回归已分别本地提交为 gosproto `70d985c` / `5be3b26`、GSR `d0328fd` / `2872d46`；均未推送/发布。Decode 性能阶段已按用户决定冻结于 `a09fb00`（生产源码同 `fc3246a`），最终入口候选未达保留门槛，见 [冻结报告](benchmarks/2026-10-17-gosproto-decode-freeze/RESULTS.md)。正式依赖仍为 v0.4.0。下文保留提案依据，封装后的新数据见 [实施验收](benchmarks/2026-10-17-gosproto-decode-store/RESULTS.md)。

当前状态（2026-10-19）：用户明确解除Decode阶段性冻结，允许按证据继续入口、解析和构造/写入优化；本页保留历史提案/验收，入口复核已本地提交为库 `913c465`、GSR `b708108`，见[Decode解冻报告](benchmarks/2026-10-19-gosproto-decode-resume/RESULTS.md)；后续[结构数组schema实验](benchmarks/2026-10-19-gosproto-decode-array-schema/RESULTS.md)未采用且报告已提交为GSR `26a5748`；当前[整数资格前移实验](benchmarks/2026-10-19-gosproto-decode-integer-proof/RESULTS.md)也未采用，报告已提交为GSR `f7381dd`；后续保留[固定header视图](benchmarks/2026-10-19-gosproto-decode-header-view/RESULTS.md)，已本地提交为库 `503a725`、GSR `2104eca`；后续[字节偏移实验](benchmarks/2026-10-19-gosproto-decode-header-offset/RESULTS.md)未采用且报告已提交为GSR `9699583`；[刷新Decode诊断](benchmarks/2026-10-19-gosproto-decode-current-profile/RESULTS.md)已本地提交为GSR `16d1491`；随后保留[原生整数数组读取视图](benchmarks/2026-10-19-gosproto-decode-array-view/RESULTS.md)，52项两轮大数组Decode快26%～29%、4字节组合快7%～9%，短包代价/控制失败/工具测试补丁偏差与纠正单列。该轮已本地提交为库 `f7022ac`、GSR `b3d4517`，完整兼容/接线验收通过，Decode继续开放。官方wire、预算、失败原子性、独占与写屏障约束不变。

最新切片（2026-10-19）：前轮word预算报告已提交GSR `8e9880f`，未推送。新[生产版阶段成本诊断](benchmarks/2026-10-19-gosproto-unpack-phase-profile/RESULTS.md)只读采af9169d的8个CPU入口：大型纯Unpack预检/恢复占完整样本30.40%/32.16%，大型PDecode中Unpack累计17.67%、正文Decode累计53.88%（嵌套项不相加）。countChunks累计4.31%，inline的未用content视图cap守卫仍在；下一候选为仅计数路径消除不使用的视图，尚未实施。55条流程命令＋1条静态objdump、24快照/1827采样权重、fresh/来源核对通过，无新A/B结论。主库仍af9169d干净，报告未提交，依赖v0.4.0、Decode继续开放。

## 问题与本轮证据

[前轮同分配诊断](benchmarks/2026-10-17-gosproto-decode-analysis/RESULTS.md) 显示，大型消息 SP/PB 分配次数相近，固定 Item 类型化赋值仍能减少执行成本。安全 inline 已有约 5% Decode 收益；本轮另建整数叶子数组专用 parser 只得到约 3.1% Decode、1.3% PDecode 收益，不值得维护重复解析器，已撤回。

随后在完整通用 parser 中建立原生 `*int64` 类型化写入隔离原型。对照 `70d985c`，五轮 129 项固定次数采样：

| 消息 | Decode 前 → 后 | PDecode 前 → 后 |
| --- | ---: | ---: |
| 大型游戏 | 17.535 → 15.565 µs（-11.2%） | 22.715 → 20.636 µs（-9.2%） |
| 小型游戏 | 1.750 → 1.565 µs（-10.6%） | 2.274 → 2.095 µs（-7.9%） |

分配次数与字节数不变。AddressBook/字符串代价、未改 Unpack 控制的波动均保留；这不是总体 PB/SP 排名。原始输入、失败候选、临时原型、GC/差分验证及复现见 [实验记录](benchmarks/2026-10-17-gosproto-decode-writes/RESULTS.md)。

## 官方 Sproto / Skynet 的实际做法

已读固定官方 oracle 的 sproto.c，以及只读 `../skynet/lualib-src/sproto/{sproto.c,lsproto.c}`：sproto_decode 校验头及数据长度，查找字段后把数值交给 callback；Lua callback 将整数推入 Lua 栈，再写 table，子结构另建 table 并递归。

它依赖 C 的指针、栈上 scalar 与 Lua 的值槽和 GC。可直接沿用的是 wire 校验、稳定字段元信息与短执行路径；不宜照搬 C 对任意地址写入，也不能用 Lua 的整数值槽替代 Go optional 指针而不改变生成类型/API。

本轮不改官方 wire，也不把 Go decoder 改成 C callback 模型。

## Protobuf 工程参考与 Go 约束

固定 protobuf-go v1.36.1：codec_message.go 初始化字段 offset 与 coder；codec_gen.go 的 consumeInt64Ptr 取得 `**int64`，必要时 new(int64) 后赋值。普通 Go 类型化指针 store 由编译器处理写屏障。这里可借鉴的是绑定后直接赋值，而不是依赖 `reflect.Value.Set` 解释赋值。

gosproto 已为 Encode 建立受控 offset 读取。本提案将能力扩展到 Decode 写入，新增的库维护责任是：字段实际类型与 owner 证明、GC 可见性、写屏障和错误回落必须同时成立。这不能只看 ns/op 后直接扩大到任意类型。

## 推荐的正式实现范围

只推广原生 `*int64` 标量，inline 和合法 4/8 字节 data 共用；不覆盖命名指针、结构体、切片、字符串或 double，不引入 generator、池或 arena。

1. 在同步调用内，根据实际 Go 类型与私有 schema 类型建立临时 `decodePointer` 能力；没有原生整数指针的结构体不建立地址能力。
2. 将地址获取与类型化 store 封装到私有文件，wire parser 不直接散布 unsafe 操作。地址使用 unsafe.Pointer，不持有跨调用 uintptr，不读取 reflect.Value 的内部表示。
3. 每次使用都要求字段来自当前 schema，原生类型及整数/非数组形状仍成立。公开 Fields 被替换、形状变化或实际类型不匹配时，保留原反射字段索引语义。
4. 校验当前标量成功后才建立独占 `*int64`。使用 Go 类型化指针赋值，让编译器保留写屏障；不得用字节复制写 Go 指针，不复用调用方旧指针。
5. 根结果仍先临时构造，完整成功后替换；字段/深度/字节预算、未知字段、消费长度、稳定错误和失败原子性保持原模型。
6. 不把实验中的 helper 命名与 Encode 的 encOwner 耦合扩展成公共能力。是否统一私有 owner/offset 名称只服务本切片，不顺带重构其他 schema 机制。

这是一种可保留现有普通 Go struct API 的内部实现选择。其代价是库需要持续验证 Go 布局与 GC 假设；它没有把这些成本转交业务调用方。

## 实施门槛与替代方案

正式实现先封装最小原生整数写入，再重测完整配对矩阵。封装后不能直接复用内嵌原型的计时；需要重新检查内联、失败路径和小包成本。原型出现的极小 Unpack 控制约 +11.6% 与字符串 Decode 约 +4.9% 保留为待解释项，不宣称全项无回退。

当前原型已通过完整库官方对照/race/严格 checkptr/vet、两项有限 fuzz、131068 组基线差分、带 padding/GC 的独占探针、不同 offset 回落及缺失 owner 守卫的负面对照，以及 GSR 三包和 Go/TS Plain/MAC 7/7；尚无跨架构运行结果。正式实现应把必要的 GC/布局/回落用例落入独立库测试，按目标工具链继续验证，不把临时探针当作长期门禁。

替代方案是继续使用安全反射，保持已提交的 inline 改进；已有数据不支持再维护一份整数专用 parser。生成 typed decoder 能避开 unsafe，但会引入生成链与运行库版本约束，需要单独设计，不在本提案中默认采用。

## 当前实施结果

已按最小范围新增私有 decodePointer；每字段 setInteger 可内联，每结构体入口仍显式检查实际类型，未改公共 API 或 wire。新 GC/padding 回归进入库内，既有 foreign owner 回落通过，移除 Decode owner 守卫的副本按预期失败。

封装后另做 129 项五轮：大型 Decode 17.484 → 16.189 µs（-7.4%），PDecode 22.904 → 21.548 µs（-5.9%），分配不变。收益低于原型，AddressBook 约 +2%，未改极小 Unpack 的 +16.6% 与 A/A +2.6% 分开保留，不据此宣称全面无回退。完整库/官方对照、race/checkptr/vet、差分、有限 fuzz 与 GSR/GoTS 验收通过；跨架构仅有 Linux 三种架构构建。该切片已本地提交为 gosproto `fc3246a`、GSR `862fa8b`；后续入口写法和固定恢复候选均未采用，见 [候选筛选](benchmarks/2026-10-17-gosproto-decode-binding/RESULTS.md)。原始验收详见 [实施验收](benchmarks/2026-10-17-gosproto-decode-store/RESULTS.md)。

## 性能阶段冻结

最后一次独立入口实验显示微基准明显收益，但大型完整 Decode/PDecode 仅 -2.25%/-1.28%，未达预定 3%/2% 门槛，候选未采用。现有私有 decodePointer、即时类型核对和逐字段 store 守卫全部保留，基线为 `a09fb00`。本阶段先收尾，后续仍可回头优化；本文件记录已实施的边界，不能把旧提案当作新优化的直接证据。正确性修复仍按现有契约处理，阶段范围见 [冻结报告](benchmarks/2026-10-17-gosproto-decode-freeze/RESULTS.md)。
