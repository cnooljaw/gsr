# gosproto 连续零 mask 的有界解包

2026-10-16，上一轮已本地提交为 gosproto `07350a9`、GSR `3a78dbd`，提交后两个工作树干净，未推送。本轮只改 Unpack 与相关回归，Pack、Encoder、Decoder、RPC、元信息和 TS 生产源码冻结。该切片随后已本地提交为 gosproto `22e287e`、GSR `1af93f4`，未发布，正式 Go 依赖仍为 v0.4.0，接线使用临时 modfile。

## 诊断与实现

参考固定官方 Sproto/只读 Skynet 的 sproto_unpack：零 mask 表示八个零字节，FF 是连续原始块，其他 mask 按位恢复。C 写入调用方缓冲并逐字节补零；当前 Go 的新输出由 make 清零，旧版本也已只写非零位，不能把本轮描述成首次省掉零字节写入。

[当前基线 profile](baseline-profiles.json) 显示，4 KiB 零字节帧解码中 Unpack 累计样本约 71%；纯零块主要花在逐 mask 的预检、计数及恢复循环。大型结构 PDecode 中 Unpack 约占 22%，仍有大量真实反射及对象构造成本，不能从零块推断所有消息都能同幅改善。

当前实现保留完整两遍：

1. 两个连续零 mask 才进入分组；按有效切片长度以 uint64 计数，首个非零机器字用 TrailingZeros64 定位第一个非零字节，不跨过该字节。这个范围中的每个零字节都是独立 mask，不是某个非零 mask 的 payload。
2. 预检用剩余输出预算计算可接纳 word 数。固定入口检查后，传给前缀 helper 的范围截止在该额度再加一个 mask，足够判定超额；不会为小预算扫描整段大输入。run 在乘八前先验证额度，避免长度乘法溢出。
3. 完整预检成功后按原确切长度 make 独占输出。零段批量推进，其他位与 FF 仍恢复原字节；输出不是输入视图、共享池或保留缓冲。
4. 合并零/FF/普通分类：零及 FF 不求无用的 popcount；复制阶段先处理零和 FF，普通循环明确具有至少一个有效字节，避免重复检查同一零状态。输入长度验证、FF 边界和有限预算没有删除，bounds check 保持启用。

拒绝仍在分配输出前完成，格式错误与额度错误按原输入顺序决定。例如额度恰好接纳零前缀时，后续截断 FF 返回 ErrUnpack；额度少一字节则先返回 ErrLimit。MaxBytes 非正仍返回 ErrInvalidLimits，失败输出仍为 nil。

## 候选与回退

[zero.json](zero.json) 从至少八个零 mask 起跳，4 KiB 零块约 -64%，但七零接 FF 的交替形态 +28.9%：它反复尝试八字节确认而无法分组，已撤回。

[grouped.json](grouped.json) 改为两零分组，短零段回退消失，但普通双非零字节形态仍约 +10%。[grouped-final.json](grouped-final.json) 是该中间版的五轮数据，保留 +10.8% 的普通形态损失；它不是最终代码。

随后合并分类，先以 [classified.json](classified.json) 三轮筛选，再用同一二进制/源码完成最终五轮 [classified-final.json](classified-final.json)。筛选中的 AddressBook Unpack +14.3% 和单零 mask 的数纳秒损失同样保留，不挑最好一轮，也不混用不同组中位数。最终 AddressBook 仍约 +10%，不能拿临界值小于 10.0% 当成无回退。

## 最终同进程配对

Apple M2，Go 1.24.13 darwin/arm64，GOMAXPROCS=1，PB v1.36.1。归档 `07350a9` 仅在忽略目录更改 module 名称，与当前库同进程加载。原代表性 JSON、Go 类型与 wire 签名冻结，Encode/Pack 仅用于准备输入。每项固定 60000 次，五轮反转版本顺序；计时、profile 与验证分开。

代表性消息的中位数，单位 µs/op：

| 消息 | Unpack 前 | Unpack 后 | 变化 | PDecode 前 | PDecode 后 | 变化 | 同场 PB Decode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| AddressBook | 0.1605 | 0.1765 | +10.0% | 1.187 | 1.206 | +1.6% | 0.8281 |
| 小型游戏 | 0.4773 | 0.4843 | +1.5% | 2.333 | 2.336 | +0.1% | 1.653 |
| 大型游戏 | 5.091 | 5.003 | -1.7% | 23.269 | 23.625 | +1.5% | 17.711 |
| 896 项整数数组 | 5.397 | 5.393 | -0.1% | 7.686 | 7.678 | -0.1% | 4.185 |
| 4 KiB ASCII body | 0.5313 | 0.5340 | +0.5% | 1.199 | 1.198 | -0.1% | 0.6318 |

数值业务组合基本持平，AddressBook Unpack 增加约 16 ns，实际 PDecode 增加约 19 ns（1.6%）。它与零块收益分别报告，不称为普通数值解包取得新突破。PDecode 也不能换成纯 Decode 来和 PB 比较后宣称总体胜出。

原 Echo 手工完整帧解码仍包含 Unpack、package/body 解码，未换成另一种 API；单位 µs/op：

| 消息 | SP 前 | SP 后 | 变化 | 同场 PB |
| --- | ---: | ---: | ---: | ---: |
| 32 B ASCII | 0.3757 | 0.3789 | +0.9% | 0.2458 |
| UTF-8 | 0.6362 | 0.6229 | -2.1% | 1.618 |
| 4 KiB ASCII | 1.351 | 1.396 | +3.3% | 1.714 |
| 4 KiB 零字节 | 2.891 | 1.411 | -51.2% | 1.595 |

当前固定零字节帧约比同场 PB 少 11.5% 耗时；这是同一 Go 夹具与路径的结论，不扩大到 TS、网络吞吐或全部协议。相同字符串的纯 payload Decode 源码冻结，仍有少量时序波动。

独立形态显示，4 KiB 零块 1.620 → 0.5839 µs（-64.0%），64 KiB 23.940 → 7.593 µs（-68.3%）；七零/FF 交替 3.707 → 1.365 µs，一零/FF 交替 0.9104 → 0.6880 µs。普通双非零字节形态 +5.4%、七字节形态 +4.7%、64 KiB ASCII +5.6% 原样保留，没有为了数字删除它们。

最终 129 项 × 五轮、645 个样本，包括五组消息 Unpack/PDecode/冻结 Decode 与 PB 控制、四档 Echo，以及 35 种合成解包形态。新补一至三和七/八/九零 mask 的单段、交替段邻域，沿用 18 种原有稀疏/混合/FF/随机形态。另有首个候选 351 样本、两份三轮筛选各 387、中间五轮 645，共 1770 个非最终样本；全部保存。

所有前后项 B/op、allocs/op 不变；4/64 KiB 零块仍为对应字节数的一次独占输出分配，AddressBook Unpack 144 B / 1 次。算法未改为边扩容边校验，未扩大 scratch、引入缓存或新 unsafe。

## Profile 与验证

[最终 profile](profiles.json) 中，零段热点转向批量前缀计数、分配与清零；零字节帧的 Unpack 累计样本约 48%，基线约 71%。百分比不能直接换算为绝对提速或相加。大型结构普通 Unpack 的 bit 提取、popcount 与复制仍是后续独立问题；本轮不重做已失败的全量展开。

扩展既有 TestUnpackOutputBudget，覆盖一至三、七/八/九、十五/十六/十七、511/512/513 个零 mask，验证零段后非零字节的正确位置、恰好预算及少一字节、迟到截断 FF 的错误优先级，以及失败输出 nil。新增用例先在未改生产实现时通过，再用于保护优化。

[45824 组输入/预算对照](unpack-differential.go.txt) 比较新旧输出、nil 状态与稳定错误类别，不比较错误文本；含额外容量/偏移、全部 mask 与高位字节、合法/截断/随机 packed、非正预算及最大 int 预算，并检查输入、边界与旧输出独占。完整独立库 race + 严格 checkptr（启用固定官方 C/Lua oracle）、vet 通过。FuzzDecode / FuzzPackRoundTrip 各约十秒通过，分别执行 101542 / 179388 次。GSR 临时覆盖三包测试、TS 构建与 Go/TS Plain/MAC 集成 7/7、零跳过通过。命令及未验证范围见 [validation.json](validation.json)；仅在本机 darwin/arm64 执行。

## 复现与边界

```sh
python3 docs/benchmarks/2026-10-16-gosproto-zero-unpack/reproduce-pairs.py \
  --library /absolute/gosproto \
  --output tmp/gosproto-zero-unpack-reproduce
```

需要 Go 1.24、git、gofmt 与含 `07350a9` 的独立库。输出目录必须不存在；可加 --prepare-only。已核对七份 Go harness、三份 JSON 与实测输入逐字节一致；其中上一轮 packing 探针保留在构建输入中，本轮不执行其 Benchmark。候选取指定库工作树，必须核对实际 replace 和源码指纹，不把 require 行的 v0.4.0 当作未发布源码。

来源与冻结代码见 [inputs.json](inputs.json)，原始值见 [raw](raw)；二进制和 pprof 留在忽略的 tmp/gosproto-zero-unpack/。该切片已本地提交为 gosproto `22e287e`、GSR `1af93f4`，未发布。后续见 [普通 Unpack 候选筛选](../2026-10-16-gosproto-unpack-candidates/RESULTS.md)，正式依赖与发布分开处理。
