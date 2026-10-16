# gosproto 长 FF 段有界搜索

2026-10-16，上一轮已本地提交为 gosproto `7dc6713`、GSR `e9e1f74`，提交后两个工作树干净，未推送。该切片随后已本地提交为 gosproto `07350a9`、GSR `3a78dbd`，未发布；正式 Go 依赖仍为 v0.4.0，验收通过临时 modfile 使用独立库工作树。后续见 [连续零 mask 解包](../2026-10-16-gosproto-zero-unpack/RESULTS.md)。

## 实现与边界

从上一轮 Pack profile 出发，先试混合 word 的前缀位置计算，真实收益不足；最终保留长 FF 段搜索及主循环简化，未继续堆混合 word 位运算。

已确认第一个 word 全非零，且剩余输入至少 256 B 时，使用标准库 bytes.IndexByte 搜索接下来最多 255 个完整 word。只跳过确认全非零的完整 word；遇零所在的 word 留给既有六/七字节合并规则，不因出现一个零就提前结束 FF。FF 总长仍至多 256 word，部分尾块仍由原路径处理。同步调整 used、FF 计数及源切片位置后，沿用原批量复制和补零。

普通循环直接以 len(src) >= 8 区分完整 word 和尾块，移除中间 input 切片及重复长度判断。稀疏/混合 word 写出算法、Unpack、Encoder、Decoder、RPC、元信息与 TS 生产源码保持不变。

单次搜索最多 2040 B，切片上界只取有效长度，不取额外容量。输出仍按原公式预留，B/op、allocs/op、返回切片的容量、输出独占及源切片不变。没有新增 unsafe、池、缓存、依赖或公共 API。官方 Sproto 的 FF 合并/256-word/尾部规则是契约，有界搜索只是同样字节结果的执行方式。

本机 Go 1.24.13 的 internal/bytealg/indexbyte_arm64.s 使用向量搜索，具体源码指纹保存在 inputs.json；其他平台继续使用标准库实现，不能把本机收益当成跨平台结论。

## 候选与代价

- [prefix.json](prefix.json)：把混合 word 写出位置改为逐字节前缀和，纯 Pack 约快 2%～3%，实际组合没有稳定收益，撤回。
- [scan.json](scan.json)：在主循环内从 64 B 起直接搜索，长 ASCII 显著受益，但 AddressBook Pack +14.0%、小型游戏 Pack +10.7%，没有直接保留。
- [outlined.json](outlined.json)：搜索移入私有 helper，AddressBook Pack 仍 +7.8%；后续采用保守的 256 B 入口，减少短输入尝试。
- [final.json](final.json) 是中间版的五轮采样，名称沿用当时脚本；其 AddressBook Pack +11.6%、PEncode +3.9% 全部保留。最终版本另以 direct.json 记录，不能将这两组中位数混算或隐藏中间回退。
- [bounds.json](bounds.json)：临时收窄内部源切片容量，Pack 栈帧 320 → 304 B，但专项收益不足以单独保留该语句；最终改为更直接的源长度判断，不携带这条额外赋值。

[汇编概况](assembly-summary.json) 显示，基线 Pack 栈帧 256 B，直接在主循环中搜索的版本 336 B；最终 Pack 320 B，搜索 helper 另有 64 B 栈帧。它们不是 heap B/op 或长期保留缓存。新控制流及栈使用是小包成本线索，不能据此证明全部回退都由栈帧造成。当前保留长文本的实际收益，明确接受小包数纳秒成本，不以更复杂的循环分层追求全项归零。

## 最终五轮配对

Apple M2，Go 1.24.13 darwin/arm64，GOMAXPROCS=1，PB v1.36.1。归档 `7dc6713` 仅在忽略目录改 module 路径，与当前库同进程加载。共享原 JSON、生成类型与固定 wire 签名，公共 Pack/EncodePacked 入口不换；每项固定 60000 次，五轮交替顺序。

下表为 [direct.json](direct.json) 五轮中位数，单位 µs/op：

| 消息 | Pack 前 | Pack 后 | 变化 | PEncode 前 | PEncode 后 | 变化 | 同次 PB Encode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| AddressBook | 0.1432 | 0.1503 | +5.0% | 0.6746 | 0.6693 | -0.8% | 0.3379 |
| 小型游戏 | 0.3459 | 0.3630 | +4.9% | 1.086 | 1.113 | +2.5% | 0.6747 |
| 大型游戏 | 3.309 | 3.284 | -0.8% | 9.889 | 10.051 | +1.6% | 7.642 |
| 896 项整数数组 | 3.286 | 3.179 | -3.3% | 5.471 | 5.384 | -1.6% | 4.886 |
| 4 KiB ASCII body | 2.075 | 0.5851 | -71.8% | 2.682 | 1.142 | -57.4% | 0.5587 |

ASCII 的收益来自连续非零范围搜索；该消息 packing 后字节数几乎不缩小，仍承担真实的格式工作，PEncode 仍高于 PB Encode。数值消息没有相同长 FF 结构，大型组合甚至有小幅损失，不称为所有业务路径都受益。

四档 Echo 的完整帧沿用原两次 Encode、拼接、Pack 路径，未替换为另一套融合 API；单位 µs/op：

| 消息 | SP 前 | SP 后 | 变化 | 同次 PB |
| --- | ---: | ---: | ---: | ---: |
| 32 B ASCII | 0.2925 | 0.2883 | -1.4% | 0.1856 |
| UTF-8 | 1.035 | 0.6813 | -34.2% | 1.575 |
| 4 KiB ASCII | 3.352 | 1.906 | -43.1% | 1.606 |
| 4 KiB 零字节 | 4.234 | 4.221 | -0.3% | 1.589 |

137 项 × 五轮，共 685 个最终样本：五种真实消息、四档 Echo，以及 39 种合成 Pack 形态。新补 63/64/65、127/128/129、255/256/257 B 阈值附近和早遇零形态，保留原 FF 六/七字节、随机、全部密度与 256-word 尾块邻域。最终 SP 中位数无 >10% 回退，最差为 4 KiB 双非零字节形态 +5.7%；AddressBook 与小型游戏 Pack 各有约 5% 代价。输入不足 256 B 时没有调用搜索，相关小幅变化不能解释为搜索加速。

另保留三个三轮候选各 345 样本、容量专项 60 样本及中间版 685 样本，共 1780 个非最终样本。原始值全部保存，不删除失败轮次，也不将前后组的差值强行归因于单个清理动作。冻结的 body 控制仍有约 0%～3% 波动，不计为 Encoder 优化。

所有前后项 B/op、allocs/op 均相同。4 KiB ASCII Pack 仍为 4864 B / 1 次，PEncode 为 9728 B / 2 次；大型消息仍为 3456 B / 1 次、7424 B / 3 次。输出预留及所有权没有换成复用输入或共享缓冲。

## Profile

[同一最终二进制的前后 CPU profile](profiles.json) 另测 ASCII Pack/PEncode，不与计时并行。旧纯 Pack 的 flat 样本约 68% 归于主循环；新热点更多落在分配/GC、memmove 与标准库字节搜索。PEncode 中 Pack 累计样本约 71% → 37%，这只是热点位置变化，不把占比差值直接当成提速，也不相加累计百分比。

## 验证

扩展既有 TestPack 的全部 256 mask × 四种非零字节：原来的短 FF 合并/退出输入后追加 256 个零，使输入实际进入搜索路径，并独立追加 32 个零 mask 作为预期。它保护“搜索遇零所在的 word 必须交回原合并规则”，不是只验证自身往返；既有 256-word 部分尾块和额外容量毒化用例继续复用。

旧新 Pack 差分扩为 17984 组，覆盖短/长输入的全部 mask、FF 255/256/257-word、阈值邻域、偏移、额外容量与尾块，并检查输入/边界和旧输出独占。完整独立库 race + 严格 checkptr（启用固定官方 C/Lua oracle）、vet 通过。FuzzPackRoundTrip 约 11.6 秒、131507 次，FuzzCodecRoundTrip 约 10.4 秒、558991 次，通过；有限 fuzz 不代表穷尽。GSR 临时 modfile 下 Sproto、Echo debugserver、Benchmark 三包测试、TypeScript 构建及 Go/TS Plain/MAC 集成 7/7、零跳过。命令与范围见 [validation.json](validation.json)。未重复 GSR 全仓测试、全仓 race 或新 TS 性能采样；仅在 darwin/arm64 运行，不声称跨平台运行。

## 复现与当前状态

```sh
python3 docs/benchmarks/2026-10-16-gosproto-ffscan/reproduce-pairs.py \
  --library /absolute/gosproto \
  --output tmp/gosproto-ffscan-reproduce
```

需要 Go 1.24、git、gofmt 及包含 `7dc6713` 的独立库。输出目录必须不存在；可加 --prepare-only。已核对五份 Go harness 与三份 JSON 和实测输入一致。新库取指定工作树，必须核对实际 replace 与源码指纹，不能以 require 中的 v0.4.0 标识未发布源码。

来源与冻结代码见 [inputs.json](inputs.json)，计时文本见 [raw](raw)，差分代码见 [pack-differential.go.txt](pack-differential.go.txt)。原始二进制、完整反汇编及 pprof 留在忽略的 tmp/gosproto-pack-prefix/，不提交可重建产物。当前新增实现和文档留在工作树供审查；发布与正式依赖升级分开处理。
