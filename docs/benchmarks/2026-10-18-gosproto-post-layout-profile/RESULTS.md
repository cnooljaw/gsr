# 布局优化后的热点与 4 KiB 测量环境诊断

采集：2026-10-18；整理与 Git 恢复：2026-10-19。库 `7397804`、GSR `25b0b0d` 已本地提交，未推送。本轮只读诊断，生产源码不变、库工作树干净；新 GSR 记录未提交，Decode 冻结，正式依赖仍 v0.4.0。

主要发现：**混合矩阵保留的其他夹具会显著改变 GC 环境；补充事件诊断确认 full 环境触发了 Go GC CPU limiter，isolated 没有。** 这是一个已测到的干扰机制，不能据此回填每个历史块或断言所有回退都已解释。下一轮先补充单项独立进程采样，再继续优化数值消息的普通 Pack word。

## 1. 固定诊断范围

[计划](plan.json) 与 [驱动](reproduce.py) 在采集前固定：4 个输入（ascii32、game_large、int32_edges、echo_ascii4k）× 纯 Encode / EncodePacked / RequestEncode+CancelSession。

- 每入口 CPU 2s 一次；逐次分配另起进程 20000 次、memprofilerate=1。12 份 CPU +12 份内存 profile。
- 同一二进制、同一个实际 API 函数，另做纯 Pack ASCII4KiB、手工 zero4KiB frame 的 full/isolated 两种存活环境。full 保留原 70 个 operation；isolated 构造相同 70 项后只保留目标，清空 setup 输出并 GC。两边全局 schema 缓存均已建立，避免把少建类型当成差异。
- 每条件一次：6 轮 ×4 个 ABBA/BAAB quartet ×4 块 ×16384 次，共 **384 块 /96 quartet /24 配对轮 /6291456 次定时调用**，不含 50ms 同函数热身。A/B 标签调用完全相同的函数，是 A/A，不是两个实现。
- runtime/metrics 在计时循环外记录堆、扫描量、分配、GC 轮次、GC/assist/user CPU 估计；统计调用本身及计数更新时间仍可能影响观测，因此这是带插桩的诊断，不能替换旧 sampler。
- 纯 Pack 两种环境另各采一份 2s CPU profile。合计 **26 份 profile**。Go 1.24.13、darwin/arm64、Apple M2，GOMAXPROCS=1，GC 默认开启。

其后仅补两个确定性诊断，另有独立计划/输入/命令：实际 body/RPC 的 [byte-pattern 形态](byte-pattern-plan.json)，以及发现堆增长后记录的 [limiter 事件](limiter-plan.json)。后者不记录 ns 或 A/A 比率，不是重跑到控制通过。

## 2. 当前算法热点

下表来自单入口 CPU profile 的累计份额，分母包括 runtime；**父子累计份额不能相加，不能由此直接推算加速上限**。

| 输入/入口 | Pack | 记录预检 helper | 记录写出 helper | runtime.kevent flat |
|---|---:|---:|---:|---:|
| game_large PEncode | 18.22% | 18.22% | 17.33% | 16.44% |
| game_large 实际请求/取消 | 22.91% | 17.18% | 19.38% | 12.33% |
| int32_edges PEncode | **40.69%** | 不适用 | 不适用 | 24.68% |
| int32_edges 实际请求/取消 | **37.10%** | 不适用 | 不适用 | 29.03% |

大型消息的 body 预检/写出仍有工作，布局 add 本身仅约 2.2%，不应继续仅围绕它堆技巧。整数数组的组合入口仍以 Pack 为主要库内成本。纯 Encode 中原生整数写出与宽度扫描仍可见，但没有证据说明它们应优先于 Pack。

Pack source 和 PC 归因已保留：int32 PEncode 的 packInto 为 940ms，循环比较 PC 被采到 330ms，word load 150ms，header 归因约120ms。**行/PC 样本不是某条 CMP 指令自身耗时的证明**；下一步应看完整 word 循环的数据依赖/分支，而不是按单行样本删除检查。不能把粗粒度源码行归因当作绝对成本分解。

ASCII4KiB 三入口的 kevent 归因仍高（约62%～72%），位于 GC/startTheWorld 路径中；纯 Pack 上下文 CPU profile 也保留其变化。不能把它算成 Sproto 字节算法成本或声称等比例可消除；本轮没有重算 PB/TS 排名或从 profile 的 ns/op 宣布加速。

## 3. 分配没有出现新对象来源

| 输入 | Encode B/次数 | PEncode B/次数 | 实际请求 B/次数 |
|---|---:|---:|---:|
| ascii32 | 48/1 | 64/1 | 64/1 |
| game_large | 3968/2 | 3968/2 | 3968/2 |
| int32_edges | 4096/1 | 4096/1 | 4096/1 |
| echo_ascii4k | 4864/1 | 4864/1 | 4864/1 |

这是 benchmark 计时区间的 B/op、allocs/op，**不把整个内存 profile（含启动、schema、RPC 构造及热身）直接除以20000**。大型消息仍为3456B输出尺寸级别+512B布局外溢；本轮没有新的中间 body/header/session 对象需要再次融合。

逐次分配 profile 的初始化对象、完整 alloc_space/objects/source 都保留。CPU 与 memprofilerate=1 的耗时不作比较基线。

## 4. 同函数为何也能“差很多”

下表全部为 A/A，负数不代表优化。独立复算与六轮原值见 [context-summary.json](context-summary.json)。MB 按1e6B，原始字节值在 trace 中。

| 场景 | 存活环境 | A/A 中位差 | IQR(pp) | 5%窗口轮数 | 诊断 timing 标志 | 块均值 ns/call | 堆峰值 MB |
|---|---|---:|---:|---:|---|---:|---:|
| ASCII4KiB Pack | full | −3.35% | 11.76 | 2/6 | 未通过 | 1179 | **637.3** |
| ASCII4KiB Pack | isolated | −0.72% | 1.04 | 5/6 | 通过 | 625 | 4.1 |
| zero4KiB 手工 frame | full | −4.79% | 4.73 | 2/6 | 未通过 | 4975 | 原值见 trace |
| zero4KiB 手工 frame | isolated | +0.96% | 3.32 | 4/6 | **未通过** | 3086 | 原值见 trace |

isolated Pack 仍有一轮 B/A=0.75493（−24.5%），并非每轮都稳定；isolated zero frame 也未通过规则。未删该轮、未再次采样或把门槛改到通过。

全量与隔离的初始堆约1.37MB/0.255MB，扫描堆约0.777MB/0.201MB；初始 goal 均约4.19MB。Pack full 的每块完成 GC 为0～32次（25块为零），isolated 为25～27次；full 的时长与块 GC 完成次数相关系数约0.956，isolated约0.048。完整 GC CPU 估计/定时墙钟约56.2%/35.4%。手工 frame 对应 full 的相关系数约0.908。

**“次数更少”不等于 GC 成本更低。** full 的堆和 goal 大幅变化，辅助扫描及阶段行为不同。相关性不能证明所有块耗时的唯一原因；CPU 计数是累计估计，读取区间包围定时循环，不能从 ns/op 中减去它“修正”结果。分配计数也有4864.010～4864.014、14616.022～14616.034 B/call的细小尾差，原样保留，不用来宣称库多分配了对象。

## 5. 补充事件诊断：CPU limiter 确实参与

在另一个相同源码归档中，每环境固定1572864次同一 Pack 调用，每16384次读取官方 `/gc/limiter/last-enabled:gc-cycle`，无耗时/比率采集。Go 当前本地官方指标说明：零表示从未启用；非零记录最近启用时的 GC 轮次；limiter 在 GC CPU 过高时用更多内存换 CPU 时间。

| 环境 | 观测GC轮次 | limiter最近启用轮次 | 堆峰值 MB | loop中启用 |
|---|---:|---:|---:|---|
| full | 3→1279 | **0→1279** | 459.3 | 是 |
| isolated | 3→2568 | **0→0** | 13.1 | 否 |

数据见 [limiter-summary.json](limiter-summary.json) 及两个97点原始 trace。full 的最后启用轮次晚于开始时GC轮次，直接确认在此补充诊断 loop 内触发。原 A/A/历史比较没有记录这个事件，不能把该值回填到它们；两个进程的峰值也不能互换。isolated 的补充进程峰值13.1MB，不能只保留先前4.1MB的较小数字。

这说明当前混合夹具可以引入真实的 GC 相位/limiter 干扰，足以提醒我们：**先校准存活工作集，不能把未变 Pack 的十几个百分点直接当成算法回退。** 这并不自动撤销已保留的业务收益，也不证明所有历史异常只由 GC 造成。

## 6. 实际 byte-pattern 支持的下一候选

24 项实际 body/notification/request/response 的原始8字节 mask 计数，通过实际 Encode/RPC API 获得；RPC 用公开 Unpack 恢复、保留至多7字节尾 padding。只读使用冻结的 Unpack，未修改它。

| 输入 | 普通3～7非零字节word数 | 其中连续非零段word数 | 比例 |
|---|---:|---:|---:|
| int32 body | 321 | 192 | 59.8% |
| int32 request | 322 | 192 | 59.6% |
| game_large body | 386 | 30 | **7.8%** |
| game_large request | 416 | 72 | **17.3%** |

整数body常见11/1f/21/7e/c7/e2/fc，真实请求则变成1f/47/7f/84/88/f1/f8；协议头会改变 word 对齐。大型消息也有不同分布，不能只测 body 或硬编码几个 fixture mask。

原始 word 形态**不是实际分支执行次数**：FF scanner 可能吸收6/7非零字节word。完整 mask 与非零字节计数在 [patterns/patterns.json](patterns/patterns.json)，独立检查24项总数相符。本次不据形态统计宣称候选一定更快。

## 7. 下一步顺序与验证

1. **先补单项独立进程采样。** 保留原70项输入/真实入口与self/clone，明确控制时只保留目标夹具，并记录limiter/堆状态；旧混合环境保留为独立观察，不静默替换默认工具、旧基线或把隔离结果当作生产承诺。一次固定计划，不重跑到控制通过。
2. 数据可靠后，再单独实验普通word中“连续非零字节段”的批量写出；这是可解释的一般形态，不硬编码fixture mask。约60%的整数密集word可能适用，但大型body仅约8%，必须验证通用分支的额外成本。FF、稀疏、尾部、读写重叠间隔、输出cap和独占均保持；沿用既有全mask/边界与旧新差分，观察真实RPC与游戏状态回退。没有最低收益百分比。
3. Decode继续冻结，不因本轮诊断扩大unsafe、改资源预算、关闭GC或引入池来美化数字。

本轮完成 harness统计/选择/fixture检查、vet、固定profile/上下文检查，独立复算384块、24形态计数、2组194个limiter点、源/生成输入/二进制/26份profile SHA 和 fresh prepare-only 归档核对；补充两类诊断分别用独立归档，没有修改原采样harness。profile二进制的296条Pack符号指令与上一已提交比较完全相同。

独立分析器初版误用了大写round/index键，修正分析器后复算通过；原始数据、探针、计划和采集均未改。此轮无生产变更，未重复库全测/官方/race/checkptr/fuzz/GSR/GoTS，也未运行PB/TS新Benchmark。库所有Go源码与7397804一致、工作树干净。新GSR文档留待审查，未提交。

文本profile、完整命令、原始context/limiter/pattern JSON及指纹保存到仓库。原始.cpu/.mem、二进制和源码归档保留在忽略的tmp目录，inputs记录其SHA；不把插桩/采样耗时或诊断环境差异写成协议加速比。

复现（分别使用新输出路径）：

```sh
python3 -B docs/benchmarks/2026-10-18-gosproto-post-layout-profile/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/post-layout-profile-reproduce
python3 -B docs/benchmarks/2026-10-18-gosproto-post-layout-profile/byte-patterns.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/post-layout-patterns-reproduce
python3 -B docs/benchmarks/2026-10-18-gosproto-post-layout-profile/limiter.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/post-layout-limiter-reproduce
```

`analyze.py`读取主输出中的context JSON与patterns.json；先把补充patterns输出复制到主输出，再运行分析。`profile-summary.json`汇总profile文本与分配列，原命令输出均保留，不用汇总覆盖原值。

收尾另发现临时库的.git/HEAD、config缺失，具体原因未证实。已先完整复制到工作区忽略目录，备份恢复HEAD后fsck通过；按现存分支及发布记录恢复原目录的HEAD和最小origin/tracking配置，未修改源码/对象/refs。原自定义配置无法完整还原。当前status干净、完整fsck及18份Go SHA通过，并在稳定工作区保存可独立验证的全refs bundle；未推送。详见 [恢复记录](git-recovery.json)。后续工作前优先将独立库迁出临时目录，避免再次依赖临时元数据。
