# 整数记录数组 Decode 入口证明复用

2026-10-20，基线 gosproto `aef6876`。**保留短数组回退版：大型消息正文 Decode 两轮中位改善 9.40% / 9.50%，PDecode 改善 7.76% / 6.22%。** 新实现及永久回归已提交独立库 `5360fe0`；正式 GSR 依赖仍为 v0.4.0，未推送或发布。本轮只比较 Go Sproto 候选与基线，没有重新采样 PB/TS，也不把本轮比例乘到旧 PB 绝对耗时上。

## 改动与安全边界

此前每个子对象都进入通用 decodeMessage，逐字段核对当前字段的原生整数、owner、形状及实际对象类型。本次只在同构整数记录数组入口完成一次证明，然后复用到数组内各个独占子对象。至少 8 项才尝试证明；短数组走原通用循环。这是局部实现选择，不是新 API 或资源预算。

实际数组元素必须恰为私有 schema.typ 的指针类型；命名外层切片可用，命名指针元素回退。schema 必须在构造时已确认所有协议字段为原生 *int64，再逐项核对当前 tagMap 可达的 Fields 描述。dense 是构造时从同一 tagMap 生成的索引，因而也在覆盖范围内。当前 Tag<0 不能跳过：Decoder 的私有索引仍可能访问该描述。nil 描述仍作未知字段；短 Fields 或 owner/Wire/Array 变化只让证明失败，保留通用路径，不因提前扫描对原本未访问的字段引入 panic 或拒绝。

证明只在当前数组内使用，不跨调用缓存；调用间元信息变化继续生效，并发修改元信息仍不受支持。所有头长、跳跃/tag 上限、未知字段 payload 边界、4/8 字节标量宽度、深度/元素预算、子项完整消费、失败原子性继续执行。countChunks 的完整首遍不变。每个子对象仍 reflect.New，每个已出现的整数仍 new(int64)，不引入 arena、池或共享指针。unsafe 只在已证明布局内作 Go 类型化指针赋值，写屏障保留。

官方 Sproto C 的 sproto_decode 先检查完整头部，未知字段也先验证并推进非内联 payload；这些规则保持。Go Protobuf 的 makeCoderMethods 在初始化时选择字段 coder、保存 offset 和类型信息，说明将已知类型工作移出热循环具有工程依据；其描述模型不能直接代替本库可在调用间改变的 Fields，因此本次没有永久缓存公开字段证明。参考源码及哈希见 [reference.json](reference.json)。

## 首版代价与短数组回退

首版不限制数组长度，完整 28 项、112 进程、10,752 块、150 条命令均成功，原始数据和当时源码在 [initial/](initial/ALL-CASES.md)。1 项 Decode 中位慢 20.76% / 19.43%，2 项慢 7.14% / 8.20%；16/128/512 项重复改善。大型 Decode 首轮中位快 4.33%，累计却慢 18.59%，含候选约 15ms 长尾；全部保留，没有筛掉后重算。

据此第二版只让至少 8 项尝试证明，并补 3、4、7、8、9 项，重新完整测量，不拼接两版结果。8 是本次测得可摊销的保守切换点，不声称是所有字段数/机器的数学最优点；3～7 项保留通用路径，1/2 项的明显回退已消失。后续若有不同分布证据仍可调整。

## 第二版结果

负值为候选耗时更低；中位和实际总时间/调用数累计并列，不择优替换。

| 场景 | pass1 中位 / 累计 | pass2 中位 / 累计 |
| --- | ---: | ---: |
| game_large Decode | -9.40% / -6.20% | -9.50% / -5.39% |
| game_large PDecode | -7.76% / -11.54% | -6.22% / -2.35% |
| game_small Decode | -2.14% / -6.94% | -0.77% / +0.65% |
| game_small PDecode | +0.41% / -5.84% | -2.14% / -3.32% |
| 1 项 Decode | -0.37% / +0.54% | -0.21% / -14.69% |
| 2 项 Decode | -0.95% / -23.67% | -0.19% / +4.60% |
| 7 项 Decode | +0.25% / +5.03% | -0.67% / -3.75% |
| 8 项 Decode | -5.87% / -6.35% | -5.70% / -6.58% |
| 8 项 PDecode | -3.67% / -2.96% | -4.56% / +4.87% |
| 9 项 Decode | -6.20% / -1.54% | -7.02% / -7.07% |
| 16 项 Decode | -9.61% / -11.30% | -9.71% / -6.69% |
| 128 项 Decode | -12.93% / -5.67% | -12.45% / -12.47% |
| 512 项 Decode | -12.92% / -12.10% | -11.67% / -9.28% |
| 命名外层切片 128 项 Decode | -13.05% / -18.12% | -13.08% / -16.17% |
| 混合字段回退 128 项 Decode | +2.33% / +3.95% | +0.46% / +1.03% |
| AddressBook Decode | +0.92% / +0.98% | +0.97% / +2.86% |
| ASCII4KiB Decode（无结构数组） | +1.89% / +4.00% | +6.15% / +10.30% |

全部 38 项及 self/clone 见 [ALL-CASES.md](ALL-CASES.md)。大型 Decode self 中位 +0.90%/IQR1.10pp，clone +1.91%/10.18pp；PDecode clone -0.23%/3.46pp。控制并非全部稳定，尤其累计与长尾仍明显。ASCII 消息没有结构数组，选择器没有更换其 decoder，不能把该项变化解释成候选新增的逐数组执行成本。混合字段、AddressBook 等实际回退路径的局部代价仍需承担和跟踪，不宣称全面提速，也不扣控制来制造“净收益”。

本版共有 35 项中位/累计方向不同，原始块全部保留；比较中位分配次数全相同，大型 Decode/PDecode 均为 532/533 次。累计分配有少量尾差，不能说每个块总分配都完全一致。最大堆快照 85,744,352B，不是峰值；实际计时块约 0.132～286ms，虽然校准目标是 20ms，单次校准会受长尾影响，未以目标值冒充实际块长。本轮收益判断依赖重复的目标数组和业务组合方向，仍有环境及实验适配器限制。

## 静态成本与测量方式

数组入口机器字行（含填充）352→444，栈256→368B；专用整数 parser 为408行/192B栈，另有8行清理 helper。完整数组入口增加分支/调用参数，不是“总指令必然减少”。专用 parser 的类型化标量写入和外层切片写入均保留 GC 屏障调用。静态统计见 [assembly-summary.json](assembly-summary.json)，不是动态执行次数或性能证明。复制一份私有 wire 循环带来维护成本，未来修改相关校验必须同步两条路径和差分回归。

同一模块、同一进程、共享 schema 与公共 Decode/DecodePacked 入口；仅在计时外更换结构数组 decoder。clone 函数体去除名称/空白后等于基线；candidate 专用递归仅用于已证明整数记录，其余结构继续通用路径。实验选择器不进入生产库，绝对值不作为新 PB 排名。

每臂预热50ms，基线256次校准约20ms，两臂相同固定调用数。6轮×4个ABBA/BAAB四元组，GOMAXPROCS1/GOGC100/GOMEMLIMIT=off，计时外记录 MemStats，保留 GC 与长尾。第二版152进程、912轮、3648四元组、14592块；38项全部预检后才计时，200条准备/预检/测量命令成功。两版分别分析，未跨二进制合并。源码、生成代码、输入、唯一SP模块与二进制指纹前后相同，逐块比值独立复算至1e-12。

## 正确性与落地

新增永久回归补充相对现有测试的覆盖：0/1/2/3/4/7/8/9/16/128规模与命名形状、低GOGC和跨调用/记录独占；空child时深度错误顺序、晚发标量错误的原子性；负Tag仍可达、负Tag+Array形状变化、nil/短Fields和外来owner回退。另有648个合法/截断/逐bit变异child输入，在3组预算下与命名整数通用路径执行1944对公共API对照，核对值、错误和消费长度，覆盖未知payload及跳跃上界。不是为同一断言重复搭建跨层夹具。

新增回归先在未改基线通过，随后完成10条验收流程（含2次gofmt）：完整库与固定官方oracle、完整race+严格checkptr、vet、两个10秒fuzz、GSR受影响三包、Go/TS集成7/7且零skip。Codec438902次；Decode2735次后计数停增，11秒正常退出，保留原文，不把有限fuzz当作完整证明。集成服务器buildinfo确认replace到本次验收候选。实际库所有Go源逐文件等于完整验收快照；原六项用户删除不动，go.mod未恢复。

前轮GSR报告已提交 `0c23252`，本轮独立库的decode.go、两个新文件及CHANGES已提交 `5360fe0`，GSR报告及文档随本轮提交。正式依赖未升级。下一切片回到 Packing 主线：重新核对当前整数数组/大型消息 Unpack 的成本分布，再挑选单一热点候选；Body Decode 保持开放。

```sh
python3 docs/benchmarks/2026-10-20-gosproto-decode-record-proof/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output /absolute/new-output \
  --minimum-records 8 --extra-boundaries
python3 docs/benchmarks/2026-10-20-gosproto-decode-record-proof/analyze.py --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-decode-record-proof/accept.py \
  --experiment /absolute/new-output --output /absolute/new-acceptance
```

目录须不存在，保留Go缓存。省略两个可选参数可复现首版算法与28项矩阵；当时的测试/harness源码另保留在initial，不保证新构建的机器码地址或时序逐位复现。原测量目录为tmp/gosproto-decode-record-proof-20261020及tmp/gosproto-decode-record-proof-v2-20261020，完整验收为tmp/gosproto-decode-record-proof-accept-20261020；归档不提交二进制。
