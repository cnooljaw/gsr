# gosproto 大型 Decode 的构造与赋值诊断

2026-10-17，上一轮已本地提交为 gosproto `d66644a`、GSR `acfb070`；GSR 后续补丁格式属性修正为 `ae2e1bc`，均未推送。诊断固定 `d66644a` 的生产源码，正式依赖仍为 v0.4.0；所有固定 Item 探针只在忽略目录生成的独立副本中运行，不作为通用生产 decoder。

## 先修正“532 次分配”的含义

同一大型夹具、同场五轮固定次数采样：SP Decode 为 10416 B / 532 allocs，PB Decode 为 13560 B / 537 allocs。SP 仍有执行成本，但这份数据不支持“SP 的分配次数/字节明显比 PB 多，所以先减少对象数量”的判断。

Go 生成类型以 `*int64` 表示 optional presence，每个 Item 本身也是独立对象；新建这些对象不是自动可删除的浪费。既有失败原子性需要临时根结果；缺字段、存在的零值、不同字段与调用之间的独占都必须保持。当前对照没有合并对象到 arena、复用消息、引入池、缓存输出或修改生成物。

## 官方 Sproto 与 PB 的实际执行模型

已阅读固定官方 oracle（cloudwu/sproto `e93fd5a0653b2bcab38d3b830958bf2e3a901b5f`）及只读 Skynet 的 `lualib-src/sproto/{sproto.c,lsproto.c}`。Lua decode callback 对整数使用 lua_pushinteger，再写入 table；子结构创建 table 并递归解码。它依赖 Lua 栈、值槽和 table 的表示，不为每个整数建立 Go 的独立 optional 指针对象。可借鉴预解析字段与短执行路径，不能用其分配模型直接衡量普通 Go struct。

本机固定 protobuf-go v1.36.1 的 `internal/impl/codec_message.go` 初始化字段 offset 与 coder，`codec_gen.go` 的 consumeInt64Ptr 使用类型化指针；首次无值时仍 new(int64)，随后直接写目标。它的优势也包含访问、分派及赋值成本，不能简单归为“消灭所有分配”。

## 同分配、同 wire 的隔离探针

从 `d66644a` 归档生成通用、fixed_reflect、fixed_typed 三份副本。诊断类型与原 GameState/Item 的字段、tag、optional 形状一致，沿用原 JSON 和固定 wire 签名。三份都恢复完整 root、数组、Item 和字段值；根流程、数组构造和非 Item 字段保持通用实现。

- generic：不修改解码执行，作为诊断类型与原生成类型的控制。
- fixed_reflect：只对固定三整数 Item 缩短 schema 查找/通用分派，仍通过 reflect.Value.Field/Set 写入相同的独占 *int64。
- fixed_typed：使用同一个固定 Item parser，改成类型化字段赋值。仍为每个整数 new(int64)，结构体仍走原 reflect.New。

三个 Item parser 都保留字段/深度预算、inline 与 4/8 字节 scalar 校验、未知 tag 跳过及完整消费检查。它们固定三整数模型，没有验证全部 schema/元信息替换和通用错误行为，不能直接复制到生产库或作为新的 generator/runtime hook。

Apple M2、Go 1.24.13 darwin/arm64、GOMAXPROCS=1、PB v1.36.1；每项 60000 次、五轮反转操作次序，共 26 项、130 样本。计时不与 profile/测试并行；所有原值和控制保留。

大型消息中位数，µs/op：

| 入口 | Decode | PDecode | Decode B/allocs | PDecode B/allocs |
| --- | ---: | ---: | ---: | ---: |
| 原生成类型通用 SP | 18.230 | 23.136 | 10416 / 532 | 13872 / 533 |
| 诊断类型通用 SP | 18.208 | 23.362 | 10416 / 532 | 13872 / 533 |
| fixed_reflect | 17.261 | 22.771 | 10416 / 532 | 13872 / 533 |
| fixed_typed | 15.390 | 20.492 | 10416 / 532 | 13872 / 533 |
| 同场 PB | 17.605 | — | 13560 / 537 | — |

相对诊断类型通用流程，fixed_reflect Decode 约 -5.2%，fixed_typed 约 -15.5%；后者相对 fixed_reflect 约 -10.8%。这区分了缩短固定流程与类型化赋值的收益，不把全部差值都归给某一个 reflect API。

小型游戏的 generic/fixed_reflect/fixed_typed Decode 为 1.818 / 1.756 / 1.620 µs，分配均为 1200 B / 52 次。AddressBook 与整数数组保留原通用入口和同场 PB 控制，没有用固定 Item parser 改变这些模型。

fixed_typed 的大型纯 Decode 低于同场 PB，但其 PDecode 20.492 µs 仍高于 PB 17.605 µs。它证明固定执行模型有空间，不能据此宣布通用 Go Sproto 已赢 PB；packing 的额外成本仍存在。

## 构造微基准

只构造 128 个 Item 及各三个 *int64，保留所有结果，字段值一致：反射构造约 12.478 µs，类型化构造约 7.064 µs；两者均为 7320 B / 514 allocs。反射流程包括 MakeSlice、New、Field/Set、数组元素 Set，类型化流程直接 new 和赋值；它不是对单个 API 的隔离对照。

这说明相同对象数量也能有显著构造执行差异。微基准没有 wire/完整 schema/预算工作，不能从完整 Decode 中减去它来计算“wire 必要成本”，也不能与其他分段耗时相加。

## 落地的安全 inline 改动

探针之后，仅在正式 decoder 加入七行快捷处理：原流程已完成字段/深度预算和 header/类型校验；hasInline 且元信息确认是原生 *int64 时，直接 new(int64)、赋 inline 值，再用安全 reflect.Value.Set 写入临时结果。原 scalarBits 在此只会返回 inline 值，省掉其重复判定和 decoder 间接分派。

没有使用 encOffset 写字段，没有新增 unsafe。非 inline 整数、命名类型、其他标量/数组及错误处理继续走原 decoder；完整 root 成功后才替换调用方值，optional presence 与独占对象不变。Pack/Unpack、Encoder、元信息、limits、RPC 和 TS 生产源码保持冻结。

先做 129 项三轮筛选（387 样本），原始数据保存在 [inline/inline.json](inline/inline.json)。该批次有明显波动：Echo ASCII PDecode 中位数约 +74.7%，未改的零帧路径也显示约 -32.9%；原始逐轮值保留，未删除或替换。随后对同一源码、同一二进制做完整五轮复核（645 样本），作为以下结果；两批不合并计算中位数。

最终同进程中位数，µs/op：

| 消息 | Decode 前 → 后 | 变化 | PDecode 前 → 后 | 变化 | 同场 PB Decode |
| --- | ---: | ---: | ---: | ---: | ---: |
| AddressBook | 1.0130 → 0.9969 | -1.6% | 1.2140 → 1.1780 | -3.0% | 0.8232 |
| 小型游戏 | 1.8210 → 1.7380 | -4.6% | 2.3310 → 2.2580 | -3.1% | 1.6620 |
| 大型游戏 | 18.2250 → 17.2530 | -5.3% | 23.4880 → 22.5150 | -4.1% | 17.8400 |
| 896 项整数数组 | 2.2500 → 2.2620 | +0.5% | 7.6800 → 7.7070 | +0.4% | 4.1820 |
| 4 KiB ASCII body | 0.6494 → 0.6343 | -2.3% | 1.2000 → 1.1830 | -1.4% | 0.6126 |

所有配对项 B/op、allocs/op 不变。大型纯 Decode 约比同场 PB 少 3.3% 耗时，但 PDecode 仍多约 26.2%；不把纯 body 胜出扩大为组合路径或总体胜出。整数数组源码和正常入口未变，其小幅差异作为控制保留。

完整矩阵中的极小零段 zeroWords16 为 57.64 → 65.12 ns（+13.0%），FF/稀疏混合为 4.336 → 3.698 µs（-14.7%）；两项 Unpack 源码均未改。为核对这两个差异，另构建完全恢复原 Decode 的 A/A 二进制，与候选二进制按轮交替运行五次：

| 形态 | A/A 旧/当前副本 | inline 旧/当前副本 |
| --- | ---: | ---: |
| 16 个零 mask | 54.61 / 53.04 ns | 54.18 / 54.89 ns |
| FF512 + sparse4096 | 4.216 / 4.192 µs | 4.200 / 3.606 µs |

专项未支持零段持续约 13% 的回退；FF/稀疏差异仍存在，不能把它归为 Unpack 算法改进。源码相同不保证不同二进制布局及采样上下文的耗时相同，具体机器层原因未进一步证明。主矩阵和 [A/A 控制](inline/controls.json) 分别保留，不用专项替换主矩阵数据，也不宣称“全项没有超过 10% 的回退”。

## 实现验收

扩展原 TestNativePointerDecodeOwnership，保留 -42，再增加 0、1、32766、32767、MaxInt64，覆盖存在的零值、inline 上边界及非 inline 值。用例先在基线实现通过，再用于保护新路径；不同字段和不同调用仍保持独占。

[131068 组差分](inline/differential.go.txt) 枚举全部 0…32766 inline 值，分别验证成功、低元素预算拒绝、后续坏帧及 packed 解码，比较稳定错误类、消费长度、完整输出、失败时原指针和成功字段独占。完整独立库 race/严格 checkptr（启用固定官方 C/Lua oracle）、vet、两项各约十秒 fuzz，以及 GSR 临时 modfile 三包和 Go/TS Plain/MAC 集成 7/7、零跳过通过。

命令、执行数量与未验证范围见 [validation.json](validation.json)。没有重跑 GSR 全仓/race、TS 性能或跨平台运行。该切片已本地提交为 gosproto `70d985c`、GSR `d0328fd`，未发布，GSR 正式依赖仍为 v0.4.0。后续见 [记录候选与受控写入原型](../2026-10-17-gosproto-decode-writes/RESULTS.md)。

inline 实现可用独立副本复现：

```sh
python3 docs/benchmarks/2026-10-17-gosproto-decode-analysis/inline/reproduce-pairs.py \
  --library /absolute/gosproto \
  --output tmp/gosproto-inline-decode-reproduce
```

脚本从 d66644a 归档并在新输出目录应用测试和实现补丁，不修改传入库；七份 harness、三份 JSON 和全部 Go 源码均核对到实测指纹。最终配对数据见 [inline/inline-final.json](inline/inline-final.json)，资源与冻结源码见 [inline/inputs.json](inline/inputs.json)。这部分是通用生产改动的验收，与上面的固定 Item 诊断分开。

## 复现与验证范围

```sh
python3 docs/benchmarks/2026-10-17-gosproto-decode-analysis/reproduce.py \
  --library /absolute/gosproto \
  --output tmp/gosproto-decode-diagnosis-reproduce
```

需要含 `d66644a` 的独立库、Go 1.24、Python、git 和 gofmt。脚本从归档建立四份副本（含原生成类型控制使用的 normal），不修改传入库；输出目录必须不存在，可用 --prepare-only。已核对四份完整 Go 源码、三份 harness 和三份 JSON 与实测指纹全部一致。

固定 wire/完整值/消费长度校验通过。这些固定 Item parser 没有通用正确性验收，不能借用正式库或安全 inline 的验证作为它们的通用正确性背书。完整采样与指纹见 [diagnosis.json](diagnosis.json)，原值见 [raw](raw)。二进制留在忽略目录 tmp/gosproto-decode-analysis/。
