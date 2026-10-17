# 连续零 word Pack 候选筛选

2026-10-17。上一轮已本地提交：gosproto `faa6a21`、GSR `3626b3d`，未推送/发布。**本轮三个候选全部撤回，正式生产代码保持 `faa6a21`；只保留 TestPackZeroRuns 和实验记录，已本地提交为 gosproto `bba3326` / GSR `a27ec4e`，未推送/发布。** Decode 仍按当前阶段冻结，后续可以再优化；正式 GSR 依赖仍为 v0.4.0。

## 范围与候选

连续完整零 word 在 wire 中各占一个零 mask。本轮试验批量扫描与清零输出 mask，仍是一 word 一 mask，不能改成另一种 RLE。只在 FF 段外开始批量处理，FF 退出和不足八字节的尾块沿用原状态机；输出预留公式、独占、分配次数与源输入不变。

1. `scan`：主循环先识别至少两个完整零 word，直接扫描并 clear；孤立零 word 仍走原 mask 路径。
2. `outlined`：同样先识别零 word，但批量扫描/clear 移到带 go:noinline 的私有 helper，孤立零 word 直接输出。编译器确认为独立调用，见 [compiler.json](compiler.json)。这是隔离实验，不把此指令纳入正式库。
3. `sparse`：把识别移入已有稀疏 mask 分支，避免每个普通 word 前增加零段分支；批量 helper 与第二版相同。

候选将原 zeroHeaderPrefix 私有 helper 改名为 zeroBytePrefix 并复用，算法未变；Unpack 两处调用名字随之改变，预检/恢复逻辑未改。不能声称候选的 Unpack 源码逐字节不变。正式库没有采用这次改名，所有生产 Go 文件均与 `faa6a21` 完全相同。

## 同进程筛选数据

Apple M2、Go 1.24.13 darwin/arm64、GOMAXPROCS=1、PB v1.36.1。基线从 `faa6a21` 归档，仅调整 module 路径以同进程加载。七份 harness、三份业务 JSON 固定；在原 39 种 Pack 形态上增加 21 种零块长度及 6 种零段/混合 word 交替形态，共 66 种。连同五组真实消息和四档 Echo，共 191 项，每项固定 60000 次，每轮反转先后顺序。

各候选三轮 573 个样本，总计 **1719**，未进入五轮验收。[scan.json](scan.json)、[outlined.json](outlined.json)、[sparse.json](sparse.json) 与 [raw](raw/) 保存全部中位数和逐轮原值。下表为各自同场旧 → 新，单位 µs/op；不同列的旧值不能混为一个基线：

| 项目 | scan | outlined | sparse |
| --- | ---: | ---: | ---: |
| 大型 Pack | 2.8240 → 2.8810 (+2.0%) | 2.8630 → 2.9160 (+1.9%) | 2.8270 → 2.8820 (+1.9%) |
| 大型 PEncode | 10.1420 → 10.0580 (-0.8%) | 9.4040 → 10.1190 (+7.6%) | 9.4230 → 9.4950 (+0.8%) |
| 整数数组 Pack | 2.6700 → 2.7720 (+3.8%) | 2.6520 → 2.7890 (+5.2%) | 2.6490 → 2.7980 (+5.6%) |
| 整数数组 PEncode | 4.8440 → 4.9690 (+2.6%) | 4.8350 → 4.9600 (+2.6%) | 4.9560 → 5.0160 (+1.2%) |
| 4 KiB ASCII Pack | 0.6286 → 0.7071 (+12.5%) | 0.6571 → 0.6459 (-1.7%) | 0.6187 → 0.6884 (+11.3%) |
| 4 KiB ASCII PEncode | 1.3840 → 1.4060 (+1.6%) | 1.4180 → 1.2540 (-11.6%) | 1.2760 → 1.3670 (+7.1%) |
| 4 KiB 零字节完整帧 | 4.1230 → 2.7210 (-34.0%) | 4.3720 → 2.6050 (-40.4%) | 4.1490 → 2.7330 (-34.1%) |
| 4 KiB 纯零 Pack | 2.5860 → 1.0080 (-61.0%) | 2.5420 → 0.9717 (-61.8%) | 2.5780 → 0.9781 (-62.1%) |
| 孤立零 word × 64 | 0.9233 → 1.0580 (+14.6%) | 0.9577 → 0.7717 (-19.4%) | 0.9350 → 0.9298 (-0.6%) |

三个候选的所有 SP 项分配字节数和次数均与各自旧库相同。零块确实有明显潜力，但当前实现代价不合适：第一版 ASCII Pack +12.5%、孤立零 word +14.6%；第二版大型 PEncode +7.6%，部分 32 B 密度形态约 +7%～9%；第三版 ASCII Pack/PEncode +11.3%/+7.1%，整数数组 Pack +5.6%。没有通过挑选各列最好值拼出一个“最终版本”。

控制路径也有波动，例如第三版未改的 32 B payload +10.1%、ASCII 纯 Encode +6.9%。这些值原样保留，不把所有变化断言为新增分支的必然成本，也不将编译器结构变化当成因果证明。当前证据不足以保留候选，因此不再扩大同类变体或重复筛选来追求过门；没有新的 PB/SP 总体排名。

## 回归与验证

原有全部 mask、长 FF 与后继 word 测试未完整覆盖零段批量入口及末尾余数。本轮新增 TestPackZeroRuns，先在未改生产实现上通过（0.609 秒），再在各候选上验证：

- 空、稀疏和较密前缀，以及 1/255/256/257-word FF 前缀。
- 15 种零 word 数量、尾部长度 0～8、全零或末字节非零、八种切片偏移和额外毒化容量。
- 独立构造预期 wire，检查精确 mask、FF 分段和整个输入存储不变。循环执行 17280 次检查；tail=0 的两种标记会重复，不将其宣传成 17280 个独立向量。

[validation.json](validation.json) 记录三个候选最终快照的 TestPack / TestPackZeroRuns / TestUnpackOutputBudget 通过，以及三个归档补丁副本的完整 Go 源码、七份 harness、三份夹具指纹核对通过。第一次受限环境访问 Go 构建缓存失败，随后取得缓存访问权限完成有界检查；没有测试断言失败或超时重试。

所有候选均未采用，因此没有重复完整库/race/checkptr/官方 C/Lua/fuzz、GSR/GoTS 或 TS 性能验收，不将上一轮通过当作本轮重跑。测试补丁已在基线通过，正式生产文件指纹见 validation.json；TS 与固定夹具也核对未变。

## 复现与下一步

```sh
python3 docs/benchmarks/2026-10-17-gosproto-zero-pack/reproduce-pairs.py \
  --library /path/to/gosproto --output /tmp/gsr-zero-pack --candidate sparse
```

可选择 scan、outlined、sparse，默认 sparse；均为三轮筛选。--prepare-only 仅重建归档、应用 tests.patch 和候选补丁、核对指纹，不修改传入库。完整扩展 harness 保存在 [packing-probe.go.txt](packing-probe.go.txt)，输入与模块记录见 [inputs.json](inputs.json)，补丁见 [patches](patches/)。不保证复现同样 ns/op。

当前保留已提交的 Pack 基线及新增边界回归。后续若再做零段优化，应先找到降低普通/短包额外成本的明确办法，再单独立项验证；本轮不增加额外 API、池或 buffer 容量变化来换取孤立零块收益。Decode 的阶段基线继续保持。
