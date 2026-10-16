# gosproto Pack 混合数值 word 优化

2026-10-16，上一轮已本地提交为 gosproto `5e491ff`、GSR `70c84af`，提交后两个工作树干净，未推送。本轮只改 Pack 热循环及对应测试，Encoder、Decoder、RPC 与 TS 生产源码冻结；该切片随后已本地提交为 gosproto `7dc6713`、GSR `e9e1f74`，未发布，正式 Go 依赖仍为 v0.4.0。后续见 [长 FF 搜索](../2026-10-16-gosproto-ffscan/RESULTS.md)。

## 诊断与实现

固定官方 cloudwu/sproto `e93fd5a0653b2bcab38d3b830958bf2e3a901b5f` 的 `pack_seg/sproto_pack`：每八字节构造 mask；FF 段允许后续六/七个非零字节的 word 合并，最多 256 word，最后批量复制并补齐尾部。其 C 指针和缓冲管理不能直接替代 Go 的边界与所有权检查。PB 没有对应的 Sproto Pack 阶段，参考其固定 wire 操作的展开写法，不拿 PB Encode 与 SP Pack 当同一操作。

[基线 CPU profile](baseline-profiles.json) 中，packWord 占纯 Pack 约 45%–56% flat；Pack 在整数数组 PEncode 中约占 61% cumulative，大型消息约占 39%。这些是实测组合路径的样本，不用独立 Pack 时间除以 PEncode 时间推算占比。

[实际 body 分布](distribution.json) 显示：大型消息 417 个完整 word 中，4/5/6 个非零字节分别为 177/87/93 个；整数数组 449 个完整 word 中，2/4/5/6 个非零字节分别为 128/64/128/128 个，没有 FF word。主攻对象是混合 word，不能把数值消息一概当成稀疏输入或全零输入。

最终保留：

- 零至两个非零字节继续沿 mask 提取有效字节；三至七个非零字节按八个固定位置写出，目标游标只在对应 mask 位为一时递增。临时写入的零会被下一有效字节覆盖，末尾临时字节位于既有预留区，不进入返回切片。
- 用清除至多两个置位判断稀疏程度；FF 内用其补码判断零字节是否至多两个，常规写出不再求完整 popcount。
- 普通 word 不再进入多层 FF 收尾判断；只有连续段内才处理合并、256-word 收尾或退出。完整非零 word 的原有快判断及批量复制保留，部分尾块仍按实际长度读取。

仍使用安全 Go 切片和原 `len + ceil(len/2048)*2 + 8` 输出预留；没有增加 unsafe、池、表缓存或保留缓冲。固定位置写出前显式确认九字节窗口，编译器能够据有限 mask 增量推导后续索引范围；没有关闭 bounds check 或删减输入检查。公开 API、精确 wire、源切片不变、输出独占与所有资源预算不变。

## 未保留的候选

[unrolled.json](unrolled.json)：所有普通 word 用八个条件分支展开，整数数组 Pack +8.5%，全零/单字节形态约 +29%～+47%，大型 Pack 基本持平，撤回。

[branchless.json](branchless.json)：按稀疏程度选择循环/固定位置写出，整数数组 Pack -11.4%、PEncode -6.6%，大型 Pack -3.5%，组合没有明确收益。随后加入位判断和 FF 状态简化，见 [state.json](state.json)；该三轮筛选曾有 FF 2046 B 项 +12.2%，最终五轮保留同一边界复核，未重现该幅度。各候选原始值全部保留，不与最终五轮混算。

## 最终五轮配对

Apple M2，Go 1.24.13 darwin/arm64，GOMAXPROCS=1，PB v1.36.1。`5e491ff` 归档仅在忽略目录改 module 名称，与当前库同进程加载；使用原 JSON、生成类型、公共 Pack/EncodePacked 入口和固定 wire 签名。每项固定 60000 次，五轮交替顺序，中位数如下，单位 µs/op：

| 消息 | Pack 前 | Pack 后 | 变化 | PEncode 前 | PEncode 后 | 变化 | 同次 PB Encode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| AddressBook | 0.1556 | 0.1476 | -5.1% | 0.6662 | 0.6737 | +1.1% | 0.3346 |
| 小型游戏 | 0.3564 | 0.3407 | -4.4% | 1.109 | 1.064 | -4.1% | 0.6696 |
| 大型游戏 | 3.473 | 3.301 | -5.0% | 10.045 | 9.917 | -1.3% | 7.601 |
| 896 项整数数组 | 3.664 | 3.287 | -10.3% | 5.902 | 5.498 | -6.8% | 4.879 |
| 4 KiB ASCII body | 2.077 | 2.056 | -1.0% | 2.662 | 2.674 | +0.5% | 0.5736 |

Pack 变快不意味着每项组合都同比例变快。大型 PEncode 收益有限；整数数组 PEncode 仍比同次 PB Encode 多约 12.7% 耗时，大型消息多约 30.5%。原始 body 大小不同，packing 还承担实际格式工作，本轮不声称 Go 全面胜出 PB。

[final.json](final.json) 共 115 项 × 五轮，575 个样本：五组真实消息的 Pack/PEncode/Encode 与 PB 控制、四档 Echo payload/frame，以及 28 种合成 Pack 形态。后者覆盖每 word 零至八个非零字节的 32 B/4 KiB 输入、随机字节、FF 接六/七字节混合 word，以及 2046/2047/2048/2049/4094/4095/4096 B 尾块边界。另有三个候选各 345 个样本，共 1035 个筛选样本，全部保存。

没有 SP 项中位数回退超过 10%，但并非全部加速：4 KiB 单非零字节形态 3.121 → 3.220 µs（+3.2%），全零形态 +0.8%，32 B FF +2.2%。Echo 32 B 完整帧 -0.9%、4 KiB ASCII 帧 -2.2%、UTF-8 帧 +0.6%、零字节帧 +0.8%。纯 body 代码冻结，控制项仍有约 0%～5% 波动，不把它们算成本轮 Encoder 改进。

所有前后项的 B/op、allocs/op 相同。大型 Pack 仍为 3456 B / 1 次、PEncode 7424 B / 3 次；整数数组分别为 4096 B / 1 次、8192 B / 2 次。源码与采样绑定，计时期间未改 Go 源码；测试、计时和最终 profile 分开执行，没有挑选最好一轮。

## 最终 profile

[最终 CPU profile](final-profiles.json) 中，原来的 OnesCount8 热点消失，纯 Pack 仍主要花在 packWord；整数数组/大型消息的实际 PEncode 中 Pack 分别约占 54% / 30% cumulative。占比下降不直接等于绝对加速，最终耗时仍以配对表为准。当前密集路径仍有真实的 mask 构造、字节写出和游标依赖，不把全部剩余成本称为分支开销。

## 验证

扩展现有 TestPack：全部 256 mask × 4 种非零值（含 0x80/0xff），直接构造 FF 后合并/退出及五字节尾 word 的官方预期字节。原用例只覆盖独立 mask 和少量 FF 例子，本增量保护密度分派与 FF 状态简化的接点。既有 256-word 与部分尾块用例复用。

另执行 9216 组旧新 Pack 差分：FF 255/256/257-word 邻域、全部 mask、高位字节、不同源切片偏移/额外容量与短尾块，并检查输入、切片边界和旧输出不被后续调用修改。差分不能替代规范，故与独立预期字节、固定官方 C/Lua 对照共同使用。

完整独立库 race + 严格 checkptr（启用固定官方 C/Lua oracle）、vet 通过。FuzzPackRoundTrip 约 11.6 秒、2686 次，FuzzCodecRoundTrip 约 10.4 秒、532983 次，通过；有限 fuzz 不代表穷尽。GSR 临时 modfile 下 Sproto、Echo debugserver、Benchmark 三包测试通过；TypeScript 构建与 Go/TS Plain/MAC 集成 7/7、零跳过。命令及范围见 [validation.json](validation.json)。未运行 GSR 全仓测试、全仓 race 或重新测量 TS；仅在 darwin/arm64 运行，不把旧切片的交叉编译当成本轮运行验收。正式依赖及发布与本轮优化分开处理。

## 复现与后续

```sh
python3 docs/benchmarks/2026-10-16-gosproto-pack/reproduce-pairs.py \
  --library /absolute/gosproto \
  --output tmp/gosproto-pack-reproduce
```

需要本机 Go 1.24、git、gofmt，以及包含 `5e491ff` 的独立库。输出目录必须不存在；可加 `--prepare-only`。已核对生成的五份 Go harness、三份 JSON 与实测输入一致。当前版本取指定库工作树，不以 require 中的 v0.4.0 冒充未发布源码。源码、实际模块覆盖与官方来源见 [inputs.json](inputs.json)，原始计时见 [raw](raw)；二进制和 pprof 留在忽略的 tmp/gosproto-pack/。

差分探针见 [pack-differential.go.txt](pack-differential.go.txt)，放入使用同样新旧模块覆盖的独立测试包运行；不要与 package benchmarks 混在同一个目录。

当前保留的是中等幅度且不增加内存的 Pack 改进。后续应由最终 profile 选择 FF/全零扫描、写出或组合复制中的真实热点，不默认继续给热循环叠加分支，更不通过取消预算或改变输出所有权换速度。
