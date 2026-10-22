# 三步稳定字节展开：通用版与密集版

本轮完成算法候选、正常入口筛选、同模块同进程复核，以及密集mask收窄版。**两版均不应用到生产库**。6/7有效字节合成流有重复明显收益，但真实混合消息没有确认净收益；保留所有正面信号，不按固定百分比门槛舍弃。

上一轮已提交gosproto `9b2ecfb`、GSR `1ebd141`，均未推送。本轮报告与复现工具留审查；生产pack.go保持9b2ecfb，正式依赖v0.4.0，十项用户删除保持。

## 算法

普通mask的有效字节在packed输入中连续排列，恢复时需按mask散布到八字节word。按第k个有效字节的目标位置p计算位移d=p−k，d随k单调不减。预先分别记录d的4/2/1字节位移阶段，在高位阶段先移动，保持稳定次序；每阶段执行清除源位置并左移合并。其余位置为零。

每个header保存一个有效输入掩码和三个移动掩码，共256×4×8=8192B固定规则表；初始化后只读。运行时从合法的八字节源窗口读word，去掉未来段字节，只做三步移位/掩码，再一次PutUint64。该读取始终在src有效长度内，不按capacity越界读取；可能读到的后续段字节被有效输入掩码清除。

通用版仅在源剩余≥8B、普通mask至少三个有效字节时进入；尾部与稀疏mask沿用逐位恢复。密集版使用既有packFFMergeable判定至少六个有效字节，只改变通用版的准入条件。零/FF分支和完整首遍长度/预算/坏帧校验不变，仍一次分配精确容量的独占结果；没有新增unsafe、池、公共API或消息特判。

[通用源码](candidate-pack.go.txt)、[密集源码](dense/candidate-pack.go.txt)。这与旧位置表逐字节查找不同：表保存三阶段位移动规则，运行时不再逐有效字节循环。ARM64通用函数机器指令行388→448、静态panic调用点14→17；静态代码增长和8KiB表是真实代价，不代表动态执行必然更快。初始化/冷缓存成本未作为独立指标测量。

## 正常入口筛选与测量限制

两份完整9b2ecfb归档，仅B替换pack.go；独立正常二进制，无计时内选择器。self同一个A二进制，pass1/pass2为A/B；各三轮ABBA/BAAB，20项×36进程=720行，200ms Go自适应目标，GOMAXPROCS1/GOGC100/GOMEMLIMIToff。

[全部正常入口结果](ALL-CASES.md)保留逐轮、中位、近似累计与分配计数；51条流程命令成功。样本出现极端离散：未修改的FF/零段也大幅波动，整数数组Unpack某轮+2819.98%，因此这批数据仅作筛选，不用其极端值作算法或硬件归因。没有删除任何样本或扣减控制。

## 同进程复核

新快照中将9b2ecfb完整旧UnpackWithLimit复制为实验函数，与正常候选共享同一模块、schema和分配器；旧函数除名称/空白外与Git源码相同，[基线证明](inprocess/baseline-proof.json)。每臂通过同一个闭包/函数指针形式调用，避免为一侧引入额外的分派层。

Unpack直接比较字节结果/cap，PDecode两侧均执行所选Unpack后调用同一个Decode，新建独立消息。该PDecode是用于隔离Unpack差异的等价组合，不是直接计时库公共DecodePacked，不能作为新的PB排名或绝对延迟基线。真实fixture先通过既有wire和语义检查。

每个用例独立进程，7批baseline seed中位+3批pilot校准，目标60ms，最多三轮pilot、65536调用上限。每臂预热50ms；六轮×四组ABBA/BAAB×四块=96块，每进程两臂调用数固定且相同；记录全部GC、分配与时间块。每版8项×self/pass1/pass2=24进程2304块，两版共4608块。self不扣减，所有累计反向保留。

| 同进程用例 | 通用pass1中位/累计 | 通用pass2中位/累计 | 密集pass1中位/累计 | 密集pass2中位/累计 |
| --- | ---: | ---: | ---: | ---: |
| 整数数组 Unpack | +3.68% / +9.68% | +0.28% / −2.32% | +1.35% / +1.72% | +2.33% / +3.55% |
| 整数数组 PDecode | −9.87% / −15.96% | −0.15% / −3.36% | +2.09% / −0.48% | +5.18% / +14.73% |
| 大型游戏 Unpack | +8.71% / +4.15% | +9.66% / +10.43% | +2.73% / +3.76% | +3.23% / +2.03% |
| 大型游戏 PDecode | −2.11% / −5.00% | +0.84% / −0.16% | −0.61% / −1.96% | −2.36% / −2.90% |
| 6有效字节×256 | −18.73% / −17.85% | −20.52% / −21.84% | −25.48% / −29.02% | −21.46% / −21.57% |
| 7有效字节×256 | −34.75% / −35.72% | −33.79% / −35.80% | −30.42% / −29.90% | −22.46% / −22.57% |
| 1有效字节×256 | +8.83% / +8.36% | +14.48% / +15.41% | +2.85% / +1.75% | +6.13% / +8.23% |

通用版数组PDecode和密集版大型PDecode的正向信号完整保留，但不能覆盖它们各自Unpack与其他实际目标的代价。密集数据的固定流收益不能按比例乘回混合消息，也不能据此增加业务消息开关。

完整self/全部六轮/分配计数见[通用复核](inprocess/SUMMARY.md)、[密集复核](dense/SUMMARY.md)及各analysis.json。每次解包分配次数及字节数未增加，固定表仍有8KiB进程内存成本。

## 校准与正确性边界

[通用校准复算](inprocess/calibration-validation.json)：252批决策复算，块最短1.489ms、中位57.496ms、最长750.755ms；287块低于20ms，475块低于40ms。

[密集校准复算](dense/calibration-validation.json)：243批决策复算，块最短1.538ms、中位58.460ms、最长241.834ms；288块低于20ms，350块低于40ms。

zero32控制受到65536调用上限限制，不能达到目标块长；其他块也有低于目标的情况，全部保留。校准独立复算不是证明测量完全稳定；不将这些数据包装成统计显著性或硬件因果结论。

正常A/B及密集版复用全部256mask的TestUnpackWordViewBounds，覆盖重复word/零前缀、精确容量/预算、坏尾部、独占输出，race+严格checkptr通过；通用正常入口另跑相关官方/格式错误/limits测试，fixture与vet通过。同进程两侧在计时前核对输出，真实消息共享语义fixture。生产未应用，未额外跑全库fuzz或发布集成，不声称这些未跑项成功。

同进程辅助文件初次因未使用encoding/binary import编译失败，发生在任何计时之前；修复后在新输出目录准备，日志[preparation-failed.json](inprocess/preparation-failed.json)保留，没有废弃计时样本。初次正常筛选驱动未显式gofmt pack.go，后续复现脚本已补该步骤；同进程两版均在构建前gofmt，语义和源码指纹均验证。

## 结论

本轮证明三步展开对密集word有效，却不能作为当前通用Unpack的替换。最终保留9b2ecfb生产实现，规则表没有进入实际库。表查询、分流、稀疏/尾部路径和静态代码增长均是后续必须计入的工程成本；本轮未单独隔离各项机器成本，不将退化简单归咎于缓存或分支预测。

常规逐位恢复在高度混合的实际流中仍有价值。若研究下一种执行模型，应直接检验整段批处理或架构向量字节重排对混合流的端到端收益，而不是再用单一密度推断业务性能；该方向本轮未实现，未引入架构汇编。

## 复现

```sh
python3 docs/benchmarks/2026-10-22-gosproto-unpack-spread/run.py tmp/spread-normal-new
python3 docs/benchmarks/2026-10-22-gosproto-unpack-spread/analyze.py
python3 docs/benchmarks/2026-10-22-gosproto-unpack-spread/inprocess/run.py tmp/spread-pairs-new
python3 docs/benchmarks/2026-10-22-gosproto-unpack-spread/inprocess/analyze.py
python3 docs/benchmarks/2026-10-22-gosproto-unpack-spread/inprocess/check-calibration.py
python3 docs/benchmarks/2026-10-22-gosproto-unpack-spread/dense/run.py tmp/spread-dense-new
python3 docs/benchmarks/2026-10-22-gosproto-unpack-spread/dense/analyze.py
python3 docs/benchmarks/2026-10-22-gosproto-unpack-spread/dense/check-calibration.py
```

所有库构建来自完整Git归档，不恢复工作树用户删除。文本数据/源码和指纹归档，二进制留忽略的tmp。
