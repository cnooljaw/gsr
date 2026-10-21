# Pack 普通 word 提前写 header：真实消息与同进程控制

基线：gosproto `451998ea0dcf6262bdd4ca0ab6c686d0fb738fac`，从完整 Git archive 建立隔离模块。生产库未修改，GSR 正式依赖仍为 v0.4.0。

候选仅把普通 3–7 非零字节分支的 `dst[0] = header` 移到八个 payload 写出之前，尝试缩短 header 的活跃范围。没有更改位图、prefix ranks、FF 扫描或缓冲策略。candidate 逆变换可还原 clone；本二进制三入口均 296 条汇编指令。

## 结果与决定

暂不应用：本次没有确认稳定的真实消息收益，而非因为改善幅度未达到某个门槛。

| 场景 | 第一遍中位/累计 | 第二遍中位/累计 |
|---|---:|---:|
| 大型 Pack | +1.78% / +5.13% | +0.01% / −5.58% |
| 大型 PEncode | +0.51% / +5.30% | +0.91% / +1.94% |
| 整数数组 Pack | +0.25% / −1.76% | +0.26% / +3.66% |
| 整数数组 PEncode | +1.10% / −0.66% | +0.43% / −3.49% |
| 七非零字节 Pack | −6.85% / −6.61% | +0.69% / +0.46% |

正数表示 B 耗时更高。全部八项与控制见 ALL-CASES.md。

同源码 clone 的整数数组 Pack 中位为 −10.02%、固定四字节 Pack 为 −6.48%，足以提醒函数摆放等实验偏差仍然存在；不把这些差值从候选中扣除，也不将所有变化归因于代码。没有进行摆放轮换。本实验不构成“提前写 header 一定更慢”的证明；它只不足以支持生产替换。第一遍的正面样本和中位/累计反向全部保留。

## 方法与验证

- 同一模块、同一二进制，三个静态 Pack 入口；选择仅在外层发生。PEncode 三入口共用正常 encodeBufferWithLimits，各自静态调用 Pack，避免在 encodeLayout 调用链内引入选择器。
- 八项：game_small/game_large/int32_edges 的 Pack/PEncode，固定四/七非零字节的 Pack。没有声称覆盖 ASCII、零块、RPC 或所有业务。
- self/clone/pass1/pass2 各一组，32 进程、192 轮、768 个 ABBA/BAAB 四元组、3072 个计时块。每进程两侧各预热 50ms，基线 256 次校准到目标 20ms，两侧等调用数。GOMAXPROCS=1、GOGC=100、GOMEMLIMIT=off，全部 GC、暂停、分配、长尾保留。
- 3584 组 mask/长度检查：wire 相同、输入不变，以及真实共享缓冲后端源写入。既有 TestPack/TestUnpack 与新增 TestHeaderEquivalence 通过 race/checkptr。
- 真实 fixture 校验、逐进程 wire/cap 对照、harness vet/build 通过；源码及二进制哈希测量前后不变。共 41 条记录命令全部通过，详见 commands.json。
- 正常大型 Pack 为 3456B/1alloc、PEncode 为 3968B/2alloc；整数数组两者均 4096B/1alloc，小型两者均 352B/1alloc。完整计时分配数据保留在 analysis.json，不以四舍五入隐藏尾数。
- 无生产改动，所以未重复全库、官方 oracle、fuzz 和 Go/TS 集成验收。

## 复现

从 GSR 根目录运行：

```sh
python3 docs/benchmarks/2026-10-21-gosproto-pack-header-store/run.py tmp/新的空实验目录
python3 docs/benchmarks/2026-10-21-gosproto-pack-header-store/analyze.py
```

run.py 默认从 `/private/tmp/gsr-gosproto-fork` 的 HEAD 归档，复现应先确认它等于上述基线。目录必须尚不存在。原始样本会写入本报告目录，复测前自行另存现有样本。

下一方向：减少普通 3–7 非零字节的位图/前缀计算或写出工作，而不是继续单独调整 header 写入位置；需要先作等价证明，再用真实消息和控制验证。
