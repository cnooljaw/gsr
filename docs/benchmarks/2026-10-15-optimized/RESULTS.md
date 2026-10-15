# Sproto 优化实施与验收

基线已提交为 `d942321`，TS 实现和本报告提交为 `77bbbaa`。Go 优化提交为 `931e2a62b48d5f12ce670a233cfd701ed1c87410`，已随 [gosproto v0.4.0](https://github.com/cnooljaw/gosproto/releases/tag/v0.4.0) 发布，发布提交为 `3f2491f7b2701c710fdfc9dfa7952c8e7002dd1d`。

## 实际接入范围

TS 实现位于 GSR 示例内的 vendor/sproto.ts。Go 实现基于 v0.3.0 提交 `23bdb69608490c5c25955a529aee03459451baf5`；采样时用临时 modfile 覆盖依赖进行接线、Benchmark 和 profile 验证。后续发布的 v0.4.0 与本报告采样的五份 Go 源文件哈希一致，GSR 正式 go.mod/go.sum 已升级为 v0.4.0，无本地 replace。

完整 Go 改动由独立 gosproto 仓库管理，GSR 不存放源码补丁副本。实际参与采样的五份 Go 文件、TS 文件哈希与当时模块覆盖记录见 [环境](environment.json)；原始环境保留当时状态，不改写为使用已发布版本测得的数据。

## TS 编码结果

单位 µs/op；相同 fixture、五轮中位数。PEncode 包含 Encode+Pack，与 PB Encode 的工作量不同，不将纯 Encode 的胜负直接扩大为完整打包路径。

| 场景 | SP Encode 基线 | 优化后 | 原耗时/现耗时 | SP PEncode 基线 | 优化后 | 当前 PB Encode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 1.617 | 0.757 | 2.14 | 2.046 | 1.175 | 1.153 |
| game_small | 3.458 | 1.710 | 2.02 | 4.221 | 2.467 | 2.439 |
| game_large | 34.600 | 17.998 | 1.92 | 41.286 | 24.582 | 22.527 |
| int32_edges | 30.521 | 9.748 | 3.13 | 37.445 | 16.724 | 12.633 |
| echo_ascii4k | 1.745 | 1.249 | 1.40 | 16.201 | 6.758 | 5.596 |

大型游戏消息的 TS 纯 Encode 约快 1.9 倍，int32 数组约快 3.1 倍；加入 Pack 后仍有部分场景慢于当前 PB。TS Decode 基本保持原来的表现，没有声称所有阶段均胜过 PB。完整分阶段/大小/分配数据见 [报告](report.md)。

## Go 结果

| 场景/步骤 | 基线 µs | 优化后 µs | 基线 B/op / allocs | 优化后 B/op / allocs |
| --- | ---: | ---: | ---: | ---: |
| addressbook/sproto_encode | 1.887 | 0.868 | 784 / 23 | 144 / 1 |
| addressbook/sproto_decode | 1.724 | 1.279 | 920 / 47 | 528 / 29 |
| game_large/sproto_encode | 43.212 | 27.045 | 16496 / 320 | 3456 / 1 |
| game_large/sproto_decode | 41.346 | 29.261 | 22112 / 1143 | 10464 / 534 |
| game_large/sproto_unpack | 5.746 | 5.080 | 9472 / 3 | 3456 / 1 |
| int32_edges/sproto_encode | 15.259 | 13.102 | 16388 / 4 | 4096 / 1 |
| echo_ascii4k/sproto_pack | 6.217 | 2.099 | 4864 / 1 | 4864 / 1 |

Go 编码在这五组消息中均为一次输出分配。大型游戏消息 Decode 从 1143 次分配降到 534 次，保留每条消息独占的子对象和 optional 指针；没有通过复用上次消息结果减少分配。

## 五步实施内容

1. **TS 正确性**：布尔值独立编码；整数数组先决定合法宽度再写入；64 位高字按 signed 解释，保留 JavaScript 安全整数范围检查。20 组独立官方 raw/packed 向量覆盖 true/false、布尔数组、缺失/零值、正负安全 int64、32/64 位数组混合、稀疏字段、FF 边界和零字节密集输入。原来的两个失败探针现在均通过。

2. **Go 解码**：直接遍历已检查的头部，inline 按值传递；结构体/字符串数组先有界计数与边界检查，再一次分配 slice 并按索引写入。保留失败后目标原值不变。

3. **Go 编码**：使用尺寸预检得到的长度，一次分配最终缓冲；嵌套结构、数组直接写入子区间。整数数组写入过程中在首次遇到宽值时只升级一次，从后向前扩展，避免重复反射扫描。

4. **TS 编码**：常见类型直接判断，减少 Object.prototype.toString 字符串转换；缓存数组元素类型和结构 schema，取消热路径参数对象；字符串直接写入 scratch，重叠压紧使用 Buffer.copy。每次 Encode 仍返回独占字节快照。

5. **Pack/Unpack**：Go FF 连续块避免重复写入，保留热函数内联；Go Unpack 完整检查边界和输出预算后按确切长度分配。TS FF 块用批量复制，保留数值消息原有的单遍扫描；Unpack 保留有界预测缓冲及必要时按确切长度重分配，逐 word 检查代替重复的逐字节边界分支。

## 回退检查与撤回的尝试

- TS 双遍精确分配 Unpack 在大型数值消息由 5.746 µs 升到约 6.821 µs，已撤回；最终为约 5.333 µs。第一版 TS Pack 的掩码/复制改写也让数值数据变慢，最终保留单次扫描，只优化 FF 路径。
- Go 编码第一次直写版本对 int32 长数组额外扫描，约 15.259 → 17.634 µs；改为首次宽值才升级后，最终为约 13.102 µs。
- Go Pack 第一版使 packSegment 无法内联（编译器报告 cost 95 > 80），数值路径出现回退；快路径移到调用方后恢复内联。部分小差异仍处于本机噪声范围，没有据此声称稳定优势。

32 字节 ASCII 另作五轮专项，测的是旧 Echo 的 payload 和应用帧，与上面的纯 body 套件分开。

| 项目 | 基线 Go µs | 当前 Go µs | 基线 TS µs | 当前 TS µs |
| --- | ---: | ---: | ---: | ---: |
| sproto_payload/encode | 0.231 | 0.102 | 0.220 | 0.123 |
| sproto_payload/decode | 0.199 | 0.177 | 0.114 | 0.092 |
| sproto_frame/encode | 1.007 | 0.366 | 0.952 | 0.593 |
| sproto_frame/decode | 0.562 | 0.437 | 0.327 | 0.326 |

这组小包没有观察到明显回退；TS frame Decode 约 0.326 µs 与基线约 0.327 µs 基本相同。基线部分项目本来就有较大波动，原始值与每轮数据都保留，不把微小差值当作优化收益。

## Profile 验收

最终只复采 game_large 的六个独立步骤，共 12 组“语言 × 操作”的 CPU/分配 profile，未重复采 AddressBook。方法与基线相同：Go CPU/分配分进程，TS CPU/V8 分配分时段，普通计时不与 profile 并行。

- Go Encode 的 alloc_space 现在全部归于 EncodeWithLimits 的最终输出分配，原先 encodeHeaders/encodeStructSlice/encodeIntSlice 中间缓冲的分配已消失。
- Go Decode 的临时 Tag 和 reflect.Append/extendSlice 不再出现在主要分配归因中；reflect.New 约占过滤后 alloc_objects 的 96%，主要对应仍需保留的独占结果对象。
- TS Encode 仍以 lencode/encode_object_array 为主；checktype 的 CPU self 从约 8.0% 降到约 5.4%，不能将其与其他累计指标直接相加。V8 采样分配归因会随内联改变，当前集中到 encode_array；不把这一帧名称当作精确分配原因。
- TS Pack/Unpack 仍是位扫描热点。没有通过移除资源检查、共享 mutable 输出、BigInt 静默转换或换成另一套 wire 绕过成本。

Go profile 仍含 runtime.kevent 等运行时归因；V8 sampling heap 不完整计入 Buffer 外部内存，不作为精确 B/op 或泄漏证据。见 [Go top](profile-go-summary.json)、[TS top](profile-typescript-summary.json)、[采集环境](profile-environment.json)。原始 profile 留在 tmp/protocol-profile-optimized/。

## 验证与数据来源

- TS 20 组官方向量、错误整数拒绝和输出快照检查通过；真实 Plain/MAC Go↔TS Echo 六项测试全通过、零跳过。
- 独立 Go 库完整回归、固定 C/Lua 对照、race、vet 通过。有限 FuzzDecode 10 秒、FuzzPackRoundTrip 15 秒、FuzzCodecRoundTrip 15 秒通过；最后的数组升级改动另跑了一轮 CodecRoundTrip。有限测试不证明穷尽所有输入。
- GSR 默认依赖的全仓 race/vet 也通过；这部分验证的是正式 v0.3.0，不冒充本地优化库验证。本地优化库的 GSR 接线和 Benchmark 均明确使用临时 modfile。
- Go 最终整套 40 项重测，数据见 go.txt；TS 复用原始五轮且源码哈希一致的数据，采样环境见 environment-ts-capture.json。两者分时采集，最终源文件哈希和 Go 重测说明见 environment.json。

分阶段中间采样留在已忽略的 tmp/protocol-optimized-intermediate/，不纳入版本管理；仓库只保留最终采样及必要分析摘要。最终小包数据：[Go](small-go.txt)、[TS](small-typescript.json)。

## 使用正式依赖复现

当前工作树正常依赖已发布的 v0.4.0，从 GSR 根目录执行，不需要临时 modfile：

```sh
go mod download github.com/cnooljaw/gosproto
npm --prefix examples/websocket-gate run bench:representative
GSR_PROFILE_SCENARIO=game_large npm --prefix examples/websocket-gate run profile:protocol
```

采样时使用的临时 modfile 仅用于选择同一优化源码，未传播到正式依赖；发布后验证也使用正式 v0.4.0。这里没有重新执行五轮性能采样，表中的原始结果仍来自发布前测量。公开 API、官方 wire、Go 8 MiB/64 层/100000 元素默认预算与失败原子性保持不变；TS 原有字节与安全整数边界保持。
