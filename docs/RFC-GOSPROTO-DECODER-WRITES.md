# gosproto Decoder 原生整数指针的受控写入提案

状态：2026-10-17，范围已按用户“提交，继续”采纳并完成最小实现，已本地提交为 gosproto `fc3246a`、GSR `862fa8b`。此前安全 inline 与提案/回归已分别本地提交为 gosproto `70d985c` / `5be3b26`、GSR `d0328fd` / `2872d46`；均未推送/发布。正式依赖仍为 v0.4.0。下文保留提案依据，封装后的新数据见 [实施验收](benchmarks/2026-10-17-gosproto-decode-store/RESULTS.md)。

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
