# Pack 普通 word 执行路径收敛

2026-10-17。Decode 阶段性冻结记录已本地提交为 GSR `0db5ed0`，独立库基线为 `a09fb00`，其 Decode 生产源码与 `fc3246a` 相同；该轮 Pack 实现已本地提交为 gosproto `faa6a21`，本报告为 GSR `3626b3d`，未推送/发布，正式 GSR 依赖仍为 v0.4.0。Decode 保持阶段性冻结，后续仍可回头优化。

## 问题与实施范围

先重新采当前 Pack/PEncode profile，见 [baseline-profiles.json](baseline-profiles.json)。数值 Pack 仍主要执行 packWord；编译器估算该函数成本 270、无法内联。flat 样本还包含真实的 mask 和写出工作，不能把整个函数的耗时都当成调用开销。

此前的前缀位置、查表、固定恢复等候选已有失败证据，本轮没有重做这些算法。只把原来完整 word 的 mask/稀疏提取/混合字节写出放入 Pack 主循环，去掉 packWord 调用，保留同一份写出逻辑；FF 可合并性仍由一个可内联的小型位判断表达。

- 普通稀疏 word 仍只提取有效字节；混合 word 仍用原八位置写出与有限游标。
- FF 的六/七字节合并、256-word 边界、长非零搜索、尾块实际长度和补零全部保留。
- 输出仍按原公式预留，返回长度/容量、独占和源输入不变；不增加 unsafe、表、池、缓存、依赖或公开 API。
- 仅 pack.go 生产代码变化，另更新库 CHANGES；Encode、Decode、Unpack、RPC、meta、TS 与夹具均冻结。

这是执行位置和编译器优化边界调整，没有更换 Sproto wire 算法。Pack 主循环变长、嵌套更深是维护代价；没有复制两套普通写出，库 Go 源码净增四行。用明确组合收益来承担这一代价，不把“手工内嵌”作为所有函数的默认写法。

## 筛选与最终测量

Apple M2、Go 1.24.13 darwin/arm64、GOMAXPROCS=1、PB v1.36.1。基线从 `a09fb00` 归档，仅改 module 路径以同进程加载。每项固定 60000 次、按轮反转先后。使用原五组消息、四档 Echo 和 39 种密度/FF/尾块形态，137 项。

三轮筛选 [inline.json](inline.json) 共 411 样本：大型 Pack/PEncode 约 -14.9%/-5.7%，整数数组约 -16.7%/-9.3%；4 KiB ASCII PEncode 约 +4.3%。随后只补充 Go doc 与 helper 注释，使用真实独立库工作树另做五轮，源码完整指纹重新记录；不混用三轮最佳值。

[final.json](final.json) 五轮共 685 样本；[raw](raw/) 保存两阶段逐轮输出。以下 µs/op 为五轮中位数：

| 消息 | Pack 前 → 后 | 变化 | PEncode 前 → 后 | 变化 | 同场 PB Encode |
| --- | ---: | ---: | ---: | ---: | ---: |
| AddressBook | 0.1505 → 0.1336 | -11.2% | 0.6854 → 0.6596 | -3.8% | 0.3309 |
| 小型游戏 | 0.3617 → 0.3105 | -14.2% | 1.1030 → 1.0560 | -4.3% | 0.6701 |
| 大型游戏 | 3.3160 → 2.8650 | -13.6% | 9.9210 → 9.3210 | -6.0% | 7.5870 |
| 896 项整数数组 | 3.1630 → 2.5420 | -19.6% | 5.3340 → 5.0310 | -5.7% | 4.9420 |
| 4 KiB ASCII | 0.5826 → 0.5665 | -2.8% | 1.1660 → 1.1600 | -0.5% | 0.5631 |

全部 SP 项分配字节数与次数不变。大型 Pack 为 3456 B / 1 次，PEncode 为 7424 B / 3 次；整数数组分别为 4096 B / 1 次、8192 B / 2 次。大型 PEncode 仍比 PB 慢约 22.9%，整数数组慢约 1.8%，不宣称完整路径全面胜 PB。

四档 Echo 的完整帧仍走原有两次 Encode、拼接、Pack，未引入融合 API：

| 消息 | SP 前 → 后 | 变化 | 同场 PB |
| --- | ---: | ---: | ---: |
| 32 B ASCII | 0.2902 → 0.2846 | -1.9% | 0.1858 |
| UTF-8 | 0.6744 → 0.6555 | -2.8% | 1.5610 |
| 4 KiB ASCII | 1.7760 → 1.7370 | -2.2% | 1.4740 |
| 4 KiB 零字节 | 4.4680 → 3.7310 | -16.5% | 1.5440 |

## 代价与控制

最终没有 SP 项中位数回退超过 5%。最明显的三个正向差值：冻结的零字节纯 payload 编码 590.0 → 612.9 ns（+3.9%）、FF 2047 B Pack 311.3 → 319.7 ns（+2.7%）、冻结的 ASCII payload +1.6%。这些原值全部保留，纯 body 代码未变，不把其波动算成 Pack 算法变化。

三轮 ASCII PEncode +4.3% 在五轮中为 -0.5%，两组都保留，不以正式结果覆盖筛选回退。独立分段不能相加推导组合时间：整数数组纯 Pack -19.6% 对应实际 PEncode -5.7%，收益直接以组合路径测量为准。

## 编译器与 profile

[compiler.json](compiler.json) / [assembly.json](assembly.json) 确认原 packWord 调用点消失，packFFMergeable 被内联。Pack 自身的汇编列表由 549 行增为 641 行；原 packWord 的 137 行也被删除，不能只看 Pack 自身长度就声称整体代码增大。静态列表不是运行指令计数，也不能单独证明收益全部来自函数调用。

最终二进制另采同组数值 Pack/PEncode profile，见 [final-profiles.json](final-profiles.json)。真实字节写出继续可见；本轮最终 CPU profile 的 runtime.kevent 占比高达约 80%–89%，基线约 36%–58%，保留原始摘要但不从这组份额变化推算加速。性能结论以固定次数配对计时为准，调用边界以编译器/汇编为证据。原始 pprof 与二进制留在忽略目录。

## 验证

[validation.json](validation.json) 保存本轮命令、环境、输出与耗时：

- 复用既有 TestPack：全部 mask 与不同高位字节、FF 合并/退出、长搜索、后继 FF、八种输入偏移和尾块；没有为同样外部契约再新建一套测试夹具。
- 17984 组基线差分，覆盖全部 mask、FF 255/256/257-word、短长阈值、切片偏移/额外容量/尾块；检查精确字节、源输入和旧输出独占。探针沿用 [固定源码](../2026-10-16-gosproto-ffscan/pack-differential.go.txt)，独立模块仅替换新旧库路径。
- 完整库 race/严格 checkptr/固定官方 C/Lua 对照、vet 通过，官方源码及 core.so 指纹核对。Pack/Codec 有限 fuzz 各约十秒，分别 41581 / 585361 次通过；Pack 后段报告 0 新执行但正常退出 PASS，未延长超时或重试。
- GSR 临时 modfile 指向真实库工作树，三包测试、TS 构建和 Go/TS Plain/MAC 7/7、零跳过通过。正式 go.mod 没有变化。
- Linux amd64/arm64/386 生产构建通过，没有跨架构运行结论；未重复 GSR 全仓/race 或 TS 性能矩阵。

最终五轮采样、final profile 与最终验证绑定相同 Go 源码指纹；筛选及基线分别记录各自快照，测试未修改。Decode/Unpack/Encode/meta/RPC 逐字节匹配 `a09fb00`，TS 与三份夹具匹配既有指纹。筛选/最终的两个归档加补丁副本均重建出完整 Go 源码、七份 harness 和三份 JSON 的精确指纹。

## 复现与当前状态

```sh
python3 docs/benchmarks/2026-10-17-gosproto-pack-execution/reproduce-pairs.py \
  --library /path/to/gosproto --output /tmp/gsr-pack-execution
```

默认重跑 final 的 137 项 × 五轮；--candidate inline 重跑筛选三轮；--prepare-only 只建归档/补丁副本并核对输入。不会修改传入独立库。原始源码、模块覆盖与官方指纹见 [inputs.json](inputs.json)，候选补丁见 [patches](patches/)。本轮共保留 1096 个配对样本，不保证复现相同 ns/op。

目前保留的是无额外分配的数值 Pack 中等幅度改进；完整数值路径仍有差距，后续以这份组合测量为基线。Decode 当前阶段维持冻结。实现与记录已本地提交为 gosproto `faa6a21` / GSR `3626b3d`，未推送/发布，GSR 依赖仍为 v0.4.0。
