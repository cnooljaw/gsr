# Pack 普通 word 与 FF 段分离实验

2026-10-17。上一轮墙钟/CPU 诊断已本地提交为 GSR `b2b3dd7`，未推送。本轮只在隔离归档实现一个候选，**未通过预设收益和控制要求，已撤回，不进入正式库**。gosproto 工作树干净且保持 `bba3326`，GSR 正式依赖仍为 v0.4.0，Decode 继续阶段性冻结。

## 依据与候选

复核与当前 bba3326 同源码的 [profile](../2026-10-17-gosproto-pack-sparse/baseline-profiles.json) 及 [业务 word 分布](../2026-10-16-gosproto-pack/distribution.json)：普通 word 的 mask、稀疏/混合写出仍是可见成本；896 项整数数组没有 FF word，却也会逐 word 检查 FF 状态。本轮不重做已撤回的稀疏循环展开、零段批量处理或查表。

阅读固定官方 e93fd5a 的 sproto.c：pack_seg 将完整八字节非零 word 作为 FF 起点，FF 内后续六/七个非零字节也可合并，最多 256 个 word，最后批量复制并补齐尾部。官方用 C 指针推进和延迟写出，本轮沿用其 wire 规则，将 Go 内部执行位置改为先扫描当前 FF 段，再一次写出；不照搬 C 缓冲边界处理。

[候选补丁](patches/ff-runs.patch) 只修改 pack.go：

- 普通完整 word 循环不再维护 ffS/ffN/ffP；沿用原 mask、稀疏提取与混合字节写出。
- 遇到完整 FF 起点才调用私有 ffRunBytes，先复用原长非零搜索，再按六/七字节规则扫描，最多接纳 256 个 word。
- 不足八字节的末尾按实际长度计数；允许合并的六/七字节尾块由 writeFF 补零，其余仍走原 packSegment。
- 沿用原输出容量公式、安全切片、输入不变和独占输出，没有新增表、池、unsafe、依赖或公开 API。Encode/Decode/Unpack/RPC 与 TS 均未改。

FF 与普通路径分开后，普通主循环更短，但增加了独立扫描函数，FF 退出时可能重新分类下一 word；不能只把它描述成删除分支而没有代价。[静态汇编记录](assembly.json) 中 Pack 主体从 640 条降至 324 条，另有 116 条 ffRunBytes；这不是动态执行数或完整调用图的代码大小，也不是性能收益证明。

## 固定筛选与结果

计时前写入 [plan.json](plan.json)：只试一个候选；self、clone、compare 各一次，10 ms、六轮、每轮四组 ABBA/BAAB。选大型/整数数组 Pack 与 PEncode、小型 PEncode、ASCII 4 KiB 完整帧六项，共 **1728 个计时块、432 个四元组、108 个配对轮次**。没有调整默认参数、删除样本或重复到通过。

Apple M2、Go 1.24.13、GOMAXPROCS=1、GC 开启、PB v1.36.1。三模式约 9.08/8.45/8.41 秒，各自在原 90 秒限制内完成。每次由原工具先检查固定 wire 和夹具，所有时长、轮次、分配与 GC 原值保存在各模式目录。

保留要求沿用上一候选：大型或整数数组 Pack 至少快 5%，对应 PEncode 至少快 2%；相应 self/clone 总控制均通过。compare 还要求轮次 IQR ≤3 pp、至少五轮距其中位比值 ≤5%。只有这一步成立，才进入其他业务/短包回退、完整正确性和集成验收。没有以新规则放宽原门槛。

| 项目 | self 差值 / IQR | self 总控制 | clone 差值 / IQR | clone 总控制 | 候选差值 / IQR |
| --- | ---: | --- | ---: | --- | ---: |
| 小型 PEncode | +0.20% / 1.69 pp | 通过 | +0.15% / 7.61 pp | 未通过 | −3.91% / 0.85 pp |
| 大型 Pack | +0.05% / 7.17 pp | 未通过 | +0.28% / 0.84 pp | 通过 | −4.59% / 1.54 pp |
| 大型 PEncode | +0.35% / 1.08 pp | 通过 | −0.72% / 0.80 pp | 通过 | −3.99% / 0.67 pp |
| 整数数组 Pack | −1.12% / 6.62 pp | 未通过 | +0.65% / 0.34 pp | 通过 | −2.16% / 2.60 pp |
| 整数数组 PEncode | +0.24% / 0.65 pp | 通过 | −0.33% / 0.78 pp | 通过 | −1.02% / 0.76 pp |
| ASCII 4 KiB 完整帧 | +1.78% / 4.40 pp | 未通过 | −0.43% / 2.68 pp | 未通过 | −1.66% / 7.31 pp |

候选差值为配对 B/A−1 的六轮中位数，IQR 为轮次离散描述，不能视为置信区间。所有 compare 项分配统计相同。clone 的 ASCII 帧计时通过，但 A/B 分配字节中位数分别为 14616.125/14616 B/op，仍按原规则拒绝总控制，没有截断小数过门。

self 总控制 3/6、clone 4/6，同时通过的仅大型和整数数组 PEncode 两项。大型 Pack compare 的六轮比值为 0.9515、0.9476、0.7722、0.9567、1.0179、0.9665，只有四轮在其中位比值的 5% 内，虽 IQR 小仍拒绝其稳定性。整数数组 Pack compare 离散要求通过，但 self 控制失败。全部逐轮值与判定见 [joined.json](joined.json)。

**大型 Pack 的 −4.59% 未达 5%，且控制不足；整数数组 Pack/PEncode 的 −2.16%/−1.02% 未达 5%/2%。** 大型 PEncode 的 −3.99% 是局部改善信号，但不能独自替整版过门。按计划结束该候选，不扩展为多版本参数搜索，也没有进入全矩阵五轮验收或产生新的 PB/SP 排名。

## 验证与复现

[validation.json](validation.json) 保存命令、来源与指纹：

- 候选现有 TestPack、TestPackZeroRuns、TestPackPartialRunAtBlockLimit、TestUnpackOutputBudget 通过，复用全部 mask、高位字节、FF 256-word、短尾块和零段回归，无新增行为测试。
- 复用既有差分探针，只增加旧新 cap 相等断言。17984 组通过，覆盖输入偏移/额外容量、FF 边界、尾块、输入不变和跨调用输出独占，业务夹具同时通过。开发副本与计时候选所有根目录 Go 文件指纹一致。
- 原工具每模式的统计、选择、GC 观测和代表夹具测试通过。三个 prepare-only 复现重新生成相同库、harness、夹具、生成类型、工具和参数指纹；未重复采样。
- 候选没有采用，因此未重复完整库/race/checkptr/官方 C/Lua/fuzz、GSR/GoTS 或 TS 性能验收；正式库与 Decode 阶段基线未变。

```sh
python3 docs/benchmarks/2026-10-17-gosproto-pack-runs/reproduce.py \
  --library /path/to/gosproto --output /tmp/gsr-pack-runs
```

可加 --prepare-only，仅重建并核对输入。输出目录必须不存在；脚本使用归档和补丁，不修改传入库，也不保证相同的性能结果。输入来自各模式 inputs.json，原始二进制和汇编留在忽略目录。新报告与补丁未提交，留待审查。

## 下一方向

本实验没有改变实际 mask 计算和字节写出，只省掉逐 word 的 FF 状态维护；它的收益不足以承担本次重排。停止继续围绕这些分支做局部变体。

当前 sproto.go 的 EncodePacked 直接 Encode 后再 Pack，limits.go 预检得到确切 body 尺寸后才分配编码缓冲；组合路径存在两个独占字节缓冲。下一轮先分析能否在私有实现中减少中间分配，保持公开 Encode/Pack 输入不变、EncodePacked 输出长度/容量、预算和失败行为。任何共享工作区方案都须先证明读写不会覆盖未消费输入，建立独立字节与所有权回归，再谈性能；本轮没有实施该方案或引入新 API。
