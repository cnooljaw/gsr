# Decode 整数记录候选与受控写入原型

2026-10-17，上一轮已本地提交为 gosproto `70d985c`、GSR `d0328fd`，工作树干净后继续，未推送。当前正式库的全部 Go 生产源码仍与 `70d985c` 逐字节相同，仅 limits_test.go 留有新回归。整数记录专用 parser 已撤回；类型化写入只在临时副本验证，尚未采用到正式库。正式 GSR 依赖仍为 v0.4.0。

后续状态：本轮回归/提案已本地提交为 gosproto `5be3b26`、GSR `2872d46`。用户要求继续后，按最小范围封装正式 Decode helper 并重新计时；新结果与当前状态见 [实施验收](../2026-10-17-gosproto-decode-store/RESULTS.md)，不改写下文原始实验数据。

## 整数叶子数组候选：撤回

数组入口检查所有原 tag 表可命中的字段是否属于当前 schema，且仍是原生 *int64/整数/非数组；失败则回落原通用 decoder。成功时集中建立一次叶子深度，使用专用 parser 逐记录恢复整数，仍保留逐 header 和字节校验、预算、独占输出与原子替换根结果。

这需要另维护约百行解析流程。[record.json](record.json) 的 129 项三轮、387 样本中，大型 Decode 17.548 → 17.000 µs（-3.1%），PDecode 22.421 → 22.134 µs（-1.3%）；小型 Decode/PDecode 反而各约 +1.5%/+1.4%。分配不变。没有足够的真实组合收益来承担双 parser 的维护成本，已撤回，未进入五轮验收。

仅扩展相关回归：

- 既有整数记录数组用例增加 Decode，区分 Encode 实际值计数与 Decode wire header 计数；两个稀疏跳跃头也算预算，9 恰好通过，8 拒绝，深度 1 拒绝，失败保持原数组指针。
- 新增 Fields 替换用例：用别的类型的字段描述替换当前字段，验证原反射索引语义仍成立。它保护新的入口绑定风险，不把编码路径的测试当作解码绑定证据。

初版用例先在基线、再在 record 候选通过。撤回后把 Fields 用例加强为相同反射索引、不同 offset（64 与 8）的描述，已再次在正式基线通过；同样的强用例在外部原型测试执行。计时副本保留初版测试指纹，最终正式库测试另存 retained-tests.patch，未改计时的生产源码。

## 类型化写入：仅隔离原型

为区分反射赋值成本，在第二个 `70d985c` 归档副本中保持完整通用 parser，仅替换原生整数指针的构造/赋值段：

1. 当前实际 Go 结构类型与私有 schema.typ 相同，且 schema 确有原生整数指针，才在本次同步调用借用 Go 对象地址。
2. 仍完整读取和检查 header、chunk、scalar 宽度、未知字段及资源预算。inline 沿用上轮直接取值方式，非 inline 仍由 scalarBits 校验 4/8 字节。
3. 仅当字段的原生类型、owner、WireInteger 和非数组形状均成立时，使用 schema 构造时的 offset 做 `**int64` 类型化赋值；不满足时按原反射字段索引 Set。
4. 每个字段仍 new(int64)。指针不转成 uintptr 跨调用保存，不把地址放入缓存，不借用消息输入，也不复用调用方旧对象。
5. 仍在临时根对象中解码，整条成功后才替换调用方输出。保留 Go 编译器生成的写屏障，不使用原始字节复制替代指针赋值。

该副本没有固定 Item/业务 tag，也没有修改生成器、元信息模型、资源模型、Pack/Unpack 或协议格式。它覆盖通用流程中的原生 *int64，不是上一轮固定三字段的诊断 parser。当前仅为隔离原型，正式集成建议见 [受控写入提案](../../RFC-GOSPROTO-DECODER-WRITES.md)。

## 五轮同进程对照

Apple M2、Go 1.24.13 darwin/arm64、GOMAXPROCS=1、PB v1.36.1。与 `70d985c` 同进程加载，每项固定 60000 次、五轮反转操作先后。仍使用五组代表性消息、四档 Echo 和 35 种 Unpack 控制形态，共 129 项、645 样本，见 [typed-store.json](typed-store.json)。不把前面失败候选的三轮样本混入本组。

µs/op，中位数：

| 消息 | Decode 前 → 后 | 变化 | PDecode 前 → 后 | 变化 | 同场 PB Decode |
| --- | ---: | ---: | ---: | ---: | ---: |
| AddressBook | 1.0010 → 1.0090 | +0.8% | 1.1860 → 1.2000 | +1.2% | 0.8343 |
| 小型游戏 | 1.7500 → 1.5650 | -10.6% | 2.2740 → 2.0950 | -7.9% | 1.6710 |
| 大型游戏 | 17.5350 → 15.5650 | -11.2% | 22.7150 → 20.6360 | -9.2% | 17.4040 |
| 896 项整数数组 | 2.2720 → 2.2680 | -0.2% | 7.7200 → 7.7070 | -0.2% | 4.1890 |
| 4 KiB ASCII body | 0.6080 → 0.6378 | +4.9% | 1.1630 → 1.1820 | +1.6% | 0.5971 |

所有配对 B/op、allocs/op 不变；大型 Decode 10416 B / 532 次，PDecode 13872 B / 533 次。大型纯 Decode 已明显低于本组 PB，但 PDecode 仍高于 PB，packing 成本没有消失；不能据此称全部 Go 业务路径都胜出 PB。

代价与控制原值保留：4 KiB ASCII body Decode 约 +4.9%、AddressBook PDecode +1.2%；两者不以原生整数指针为主。未改源码的极小 zeroWords16 为 61.23 → 68.35 ns（+11.6%），单非零字节形态为 1.286 → 1.124 µs（-12.6%）。没有把这些未改 Unpack 的差异称为算法收益，也没有宣称全部中位数回退小于 10%。本轮未继续做该控制的 A/A 复核；提案正式实施时仍须处理可重复代价。

## 原型验证与边界

[131068 组差分](inline_test.go.txt) 与 `70d985c` 比较全部 inline 值、低预算、迟到坏帧和 packed 路径，验证错误类、消费长度、完整输出、失败保留原指针和字段独占。[GC/offset 探针](gc_test.go.txt) 使用带 127 字节前置 padding 及中间 padding 的记录、稀疏 tag、嵌套数组和多份保留结果，反复 GC 后检查指针值及不同字段/调用互不影响。

外部验证还执行增强的 Fields 用例：当前字段的反射索引仍兼容，但外来 owner 的 offset 为 8，当前字段 offset 为 64。原型正常回落通过；[仅删除 owner 守卫的负面对照](negative-owner.json) 则让该回归失败，确认用例能捕获布局误用，而非只碰巧采用相同 offset。

隔离原型的完整库 race/严格 checkptr（固定官方 C/Lua oracle）、vet、Decode/Codec 各约十秒 fuzz 通过；上述外部差分与 GC 探针也启用 race/checkptr。通过单独临时 modfile 的 GSR 三包及 Go/TS Plain/MAC 集成 7/7、零跳过通过。命令、数量、scope 见 [validation.json](validation.json)。这些结论属于临时原型，不代表正式依赖已升级或原型已发布。

[本机反汇编](barrier.json) 在类型化 store 处保留条件检查和 runtime.gcWriteBarrier2；这是本编译器的事实，不是所有平台/工具链的运行验证。没有读取 reflect.Value 私有布局；地址只由 Go 值取得，offset 只取自私有构造元信息。尚未进行跨架构运行或新增 32 位测试二进制验收。

## 复现

```sh
python3 docs/benchmarks/2026-10-17-gosproto-decode-writes/reproduce-pairs.py \
  --library /absolute/gosproto \
  --candidate typed-store \
  --output tmp/gosproto-typed-store-reproduce
```

需要包含 `70d985c` 的独立库、Go 1.24、Python、git、gofmt、patch；输出目录必须不存在。也可指定 --candidate record，按原三轮复现失败候选；可用 --prepare-only。脚本只在新输出目录的归档副本应用测试及指定候选补丁，不修改传入库。两种副本的全部 Go 源码、七份 harness、三份 JSON 已核对到实测指纹。

准备 typed-store 副本后，可用下列命令重跑外部 GC/offset 与差分检查；脚本只在该输出目录创建 external-validation：

```sh
python3 docs/benchmarks/2026-10-17-gosproto-decode-writes/verify-probe.py \
  --prepared tmp/gosproto-typed-store-reproduce
```

来源与正式库保留源码见 [inputs.json](inputs.json)，补丁见 [patches](patches)，原始样本见 [raw](raw)。二进制与外部测试副本位于忽略目录 tmp/gosproto-record-decode/。本轮保留的测试、诊断记录和提案未提交；未改变正式库的 unsafe 边界或 GSR 依赖。
