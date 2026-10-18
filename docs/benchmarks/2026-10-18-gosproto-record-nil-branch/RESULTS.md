# 整数记录 nil 调用分支实验（未采用）

2026-10-18。上一轮指针入口优化与记录已本地提交为 gosproto `a3b3830` / GSR `f37fef0`，提交后两个工作树干净，未推送。本轮只实验两个调用点的 nil 分支前移；固定测量结束后撤回自身生产候选，最终 gosproto 全部 Go 源码与 a3b3830 相同、工作树干净。只保留 GSR 实验记录未提交，正式依赖仍 v0.4.0，Decode 冻结。

## 候选与编译结果

在 fieldSize / encodeFieldInto 的既有 integers 分支内，取得一次 item := value.Index(i)，声明零值 encodePointer，仅当 !item.IsNil() 才调用原 messageEncodePointer(item, childBinding)。之后无论 nil 与否仍调用原记录 helper，保留其布局登记/读取、深度、字段数、tag 间隔和字节预算。生产候选只改 encode.go、limits.go，12 行增加、2 行删除（包含两句中文原因注释）。

没有更改实际 Go 类型、owner/形状核对、两阶段当前值读取、root KeepAlive、wire/cap、失败原子性或独占结果；没有修改 helper、标量数组算法、Pack/Unpack、RPC、Decode，也没有新增 unsafe 能力、API、缓存或依赖。nil 指针不绕过预算，只省去提取；非 nil 记录则增加一次 IsNil 的通用 kind/flag 判断，仍需原 UnsafePointer 调用。

ARM64 普通编译确认 nil 分支可绕过 UnsafePointer。fieldSize 汇编 937→985 行，encodeFieldInto 865→909 行，两个调用点栈帧都为208 B。记录预检/写出 helper 未改，汇编仍253/217行、栈帧128/32 B。原构建指纹、普通汇编与命令全部保留；代码多了分支不能预先断言会变慢，少了 nil 调用也不能预先断言会更快。

## 验证与固定采样

已有六项相关回归先在基线、再在候选通过：native offset、公开字段/嵌套描述替换、形状变化、当前值与旧输出独占，以及乱序声明/忽略字段的记录数组用例（nil/空记录、稀疏tag、字节/深度/元素预算）。基线命中测试缓存，如实保留；没有使用 -count=1 绕过缓存，没有新增重复夹具。

主矩阵完全复用上一轮43项（19 body/手工帧、19实际RPC、5 wide64），nil补充矩阵完全复用8项（全nil1/32/256及混合nil/空/填充256，各Encode/PEncode）。两组计划、补丁、探针在测量前固定，各运行self、clone、两轮compare，不在采样期间修改Go源码，不重试或调参。两臂在计时前检查完整wire/cap；harness统计/选择/GC观测/代表夹具检查通过。

Go1.24.13 darwin/arm64 / Apple M2 / GOMAXPROCS=1 / GC开启；六轮×四个ABBA/BAAB四元组×四块、目标块10 ms。主16512块/4128四元组/1032配对轮，补充3072块/768四元组/192配对轮，独立逐块复算到1e-12一致。

下表百分比是六轮配对比值中位数，ns为每轮两臂几何均值差的中位数；末列是全部累计ns/call的B/A−1。它们口径不同，全部保留，不能把配对中位数称作累计吞吐或尾延迟保证；负值表示候选耗时减少。

### 正常游戏路径

| 项目 | 第一轮 | 第二轮 | 配对差 ns（两轮） | 累计耗时比（两轮） |
| --- | ---: | ---: | ---: | ---: |
| body/game_small/sproto_encode | +8.51% | +8.42% | +59.1 / +58.3 | +8.24% / +14.50% |
| body/game_small/sproto_pencode | +7.17% | +6.93% | +65.9 / +63.1 | +13.76% / +3.14% |
| body/game_large/sproto_encode | +9.09% | +8.85% | +537.7 / +519.0 | +2.76% / +9.06% |
| body/game_large/sproto_pencode | +4.15% | +4.84% | +336.1 / +383.4 | -4.00% / +4.71% |
| rpc/game_small/notification | +7.70% | +6.86% | +77.6 / +64.7 | +8.09% / +6.85% |
| rpc/game_small/request_cancel | +7.32% | +6.73% | +72.4 / +66.6 | +6.94% / +20.07% |
| rpc/game_small/response | +6.15% | +7.92% | +58.9 / +74.5 | +11.48% / +7.88% |
| rpc/game_large/notification | +4.39% | +4.82% | +361.1 / +405.6 | +4.32% / +5.77% |
| rpc/game_large/request_cancel | +3.46% | +5.27% | +283.0 / +425.1 | -2.09% / +5.39% |
| rpc/game_large/response | +4.89% | +5.46% | +395.8 / +435.4 | +4.29% / +13.94% |

正常记录没有保住上一轮收益：大型Encode增加约519～538 ns（8.85%～9.09%），PEncode增加约336～383 ns（4.15%～4.84%），三种RPC增加约283～435 ns（3.46%～5.46%）。小型Encode增加约58～59 ns、PEncode约63～66 ns，RPC增加约59～78 ns；两轮方向一致。

### nil 密集数组

| 项目 | 第一轮 | 第二轮 | 配对差 ns（两轮） | 累计耗时比（两轮） |
| --- | ---: | ---: | ---: | ---: |
| nil1/encode | +1.74% | -1.88% | +2.0 / -2.1 | +0.78% / -2.89% |
| nil1/pencode | -1.83% | -4.52% | -2.7 / -6.5 | -3.12% / -3.34% |
| nil32/encode | -4.37% | -4.86% | -30.0 / -33.6 | -4.34% / -2.27% |
| nil32/pencode | -4.64% | -4.84% | -35.9 / -37.4 | -4.92% / -4.88% |
| nil256/encode | -9.13% | -9.82% | -401.3 / -426.2 | -9.56% / -6.84% |
| nil256/pencode | -8.45% | -6.54% | -420.6 / -321.9 | -9.12% / -5.67% |
| mixed256/encode | +6.55% | +4.24% | +591.9 / +394.0 | +0.25% / +4.44% |
| mixed256/pencode | +4.49% | +5.42% | +531.4 / +640.2 | +4.64% / +5.43% |

全nil256记录确有收益：Encode约减少401～426 ns，PEncode约减少322～421 ns（约6.5%～9.8%）；32项减少约30～37 ns。单元素方向混合、只有数纳秒。混合256项却回退：Encode增加394～592 ns，PEncode增加531～640 ns。

因此撤回本候选：这是正常业务与混合数组的重复回退，与全nil数组真实收益之间的取舍，不是因为收益没有达到某个最低百分比。已提交的a3b3830及其约0.2μs全nil长数组代价继续保留，上一轮指针入口整体优化不回退。

### 其他33项与未解释观察

| 项目 | 第一轮 | 第二轮 | 配对差 ns（两轮） | 累计耗时比（两轮） |
| --- | ---: | ---: | ---: | ---: |
| body/addressbook/sproto_encode | +0.94% | -1.70% | +5.2 / -9.5 | +10.76% / -1.91% |
| body/addressbook/sproto_pack | +1.51% | +1.33% | +1.9 / +1.6 | +1.61% / +6.88% |
| body/addressbook/sproto_pencode | +0.47% | -0.95% | +3.0 / -6.2 | +2.52% / -1.65% |
| body/game_small/sproto_pack | +0.29% | +0.16% | +0.9 / +0.5 | +1.29% / +0.14% |
| body/game_large/sproto_pack | -0.29% | +0.83% | -8.0 / +22.8 | -0.04% / +2.79% |
| body/int32_edges/sproto_encode | +13.71% | +12.98% | +243.6 / +230.7 | +13.78% / +12.59% |
| body/int32_edges/sproto_pack | +1.63% | -0.24% | +42.1 / -6.3 | +5.56% / -0.35% |
| body/int32_edges/sproto_pencode | +4.86% | +6.52% | +187.4 / +245.3 | +4.74% / +10.19% |
| body/echo_ascii4k/sproto_encode | -1.38% | +0.77% | -12.1 / +6.6 | +0.06% / -0.19% |
| body/echo_ascii4k/sproto_pack | +0.62% | -1.70% | +4.7 / -13.8 | +0.28% / -9.69% |
| body/echo_ascii4k/sproto_pencode | +0.06% | +0.09% | +0.6 / +0.9 | -4.32% / +8.10% |
| body/ascii32/frame | +2.32% | +0.87% | +6.7 / +2.5 | +2.44% / +3.53% |
| body/utf8_1k/frame | -0.29% | +0.81% | -2.4 / +7.2 | +4.74% / +8.62% |
| body/ascii4k/frame | +0.77% | +0.66% | +19.8 / +15.7 | +0.55% / +4.71% |
| body/zeros4k/frame | +0.09% | +0.93% | +3.6 / +34.8 | +2.69% / +1.26% |
| rpc/addressbook/notification | +0.84% | -0.66% | +5.7 / -4.5 | +0.89% / -5.60% |
| rpc/addressbook/request_cancel | -1.39% | -0.06% | -11.3 / -0.5 | -3.16% / -9.26% |
| rpc/addressbook/response | -0.07% | -0.79% | -0.4 / -5.4 | +0.18% / +1.24% |
| rpc/int32_edges/notification | +5.94% | +6.19% | +230.3 / +240.9 | +10.86% / +6.37% |
| rpc/int32_edges/request_cancel | +5.90% | +5.69% | +231.2 / +225.3 | +5.74% / +1.84% |
| rpc/int32_edges/response | +6.63% | +6.34% | +256.9 / +246.1 | +10.27% / +6.35% |
| rpc/echo_ascii4k/notification | +0.20% | -0.54% | +2.0 / -5.5 | +16.62% / -0.85% |
| rpc/echo_ascii4k/request_cancel | +1.00% | -0.00% | +12.3 / -0.0 | +53.70% / +0.72% |
| rpc/echo_ascii4k/response | -1.09% | +0.35% | -11.4 / +3.7 | +0.01% / +11.47% |
| rpc/ascii32/notification | +2.06% | +0.72% | +3.3 / +1.1 | +1.44% / -1.35% |
| rpc/utf8_1k/notification | -1.06% | -1.17% | -3.9 / -4.4 | +4.56% / +8.76% |
| rpc/ascii4k/notification | +0.16% | -0.20% | +1.8 / -2.1 | +0.07% / +3.02% |
| rpc/zeros4k/notification | -0.61% | -0.18% | -13.2 / -3.9 | +7.18% / +0.21% |
| body/wide64/sproto_encode | -0.96% | +0.71% | -20.0 / +14.5 | -0.82% / +0.18% |
| body/wide64/sproto_pencode | +0.18% | +0.29% | +12.8 / +20.3 | -4.69% / +0.21% |
| rpc/wide64/notification | -0.23% | +0.03% | -16.8 / +2.2 | +3.93% / +1.46% |
| rpc/wide64/request_cancel | +0.66% | +0.85% | +49.6 / +63.9 | +0.92% / +5.40% |
| rpc/wide64/response | +0.16% | +0.26% | +11.7 / +19.1 | -2.15% / +0.21% |

未经过nil记录分支的int32数组纯Encode也连续两轮回退13.71%/12.98%（约244/231 ns），PEncode与RPC同向变慢；wide64纯Encode却为−0.96%/+0.71%。该项self中位约−0.38%、clone约−0.04%，clone IQR3.02pp略未过原标记。控制限制保留，不能把约13%的重复变化自动归为噪声，也不能归因于执行了并未进入的nil分支。

只读核对普通二进制：encodeInt64Slice被内联至encodeFieldInto；4字节store/cursor/load/边界检查的15条核心指令，两臂符号与机器码完全相同，但函数其余代码和地址布局变化。int32-loop-inspection.json保存两臂地址和指令。该局部一致不证明整条路径等价，布局、预检或环境等具体贡献仍未确定；不改采样二进制、不再跑一次求好结果。

累计方向也不总与配对方向相同：例如第一轮大型PEncode累计−4.00%、request_cancel累计−2.09%，配对中位却回退；第二轮大型response累计+13.94%、小型request_cancel累计+20.07%。两种口径都在表和原值中，不只挑支持结论的一列。唯一大于100ms的比较块属于第一轮Echo4KiB request_cancel的B，217.755ms、同块完成17次GC；只能确认包含GC，不能证明全部延迟由GC造成。long-blocks.json索引该块，其余所有块及GC也完整保留。

主self35/43、clone37/43，nil组self5/8、clone6/8满足原控制标记；全部失败项、逐轮比值和IQR保留在joined.json及nil/joined.json。控制不是最小收益门槛，没有相减“校正”或过滤异常轮次。两组所有对象数中位数相同；主ASCII4KiB手工帧第二轮、零字节帧第一轮B/op均14616.125→14616，对象数仍5，其余B/op相同。保留−0.125B/op尾差，不截断计数或强行解释机制。

历史6d→数组游标的纯Pack+9.45%及bd→指针入口的未改int32请求+4.74%仍是原比较中的未解释观察；本轮基线a3b3830不同，新数据不覆盖它们，见[数组游标报告](../2026-10-18-gosproto-array-cursor/RESULTS.md)与[指针入口报告](../2026-10-18-gosproto-record-pointer/RESULTS.md)。

## 撤回与后续

测量后核对主库全部Go源码与两组compare、编译归档candidate一致，才逐文件以HEAD内容恢复本轮encode.go/limits.go；没有清理其它文件或用户改动。restoration.json保存撤回前后指纹和仅恢复的两个文件，最终全部Go源码逐个与a3b3830比对一致、工作树干净。重做两组各四份prepare-only归档，核对补丁、源码、工具、夹具、探针、参数，与测量输入一致，不重跑性能。

本轮没有保留生产或测试改动，因此不重复全库/官方/race/checkptr/vet/fuzz、GSR三包或Go/TS验收。最终生产仍使用a3b3830已经通过的官方兼容、race/checkptr/vet、441809次fuzz和Go/TS7/7验收；这些是上一轮结果，不计作本轮新执行。本轮新增验证是基线/候选相关测试、harness检查、固定两组采样、编译核对、源码恢复及归档复现。

下一步从已提交a3b3830刷新大型消息Encode/PEncode及实际RPC profile，确认两轮优化后的当前成本分布，再选择下一处局部改动。nil分支位置实验到此结束，不扩大unsafe范围，不修改Decode或正式依赖。

复现（输出目录必须不存在）：

```sh
python3 -B docs/benchmarks/2026-10-18-gosproto-record-nil-branch/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/record-nil-new
python3 -B docs/benchmarks/2026-10-18-gosproto-record-nil-branch/analyze.py tmp/record-nil-new
python3 -B docs/benchmarks/2026-10-18-gosproto-record-nil-branch/nil-reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/record-nil-supplement-new
python3 -B docs/benchmarks/2026-10-18-gosproto-record-nil-branch/nil-analyze.py tmp/record-nil-supplement-new
```

只将复现输入、完整原值、普通汇编和验证记录保留到Git；二进制和完整源码归档在忽略的tmp/。当前GSR文档/记录未提交，gosproto干净，没有推送、发布或升级正式依赖。
