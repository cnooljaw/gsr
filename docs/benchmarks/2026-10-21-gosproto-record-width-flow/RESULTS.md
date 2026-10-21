# 整数记录外置字段：4/8 字节直接分支

基线：gosproto `9048cfa`（上一轮 inline 直接继续已提交），完整 Git 归档、隔离实验模块，GSR 正式依赖仍为 v0.4.0。

## 源码与编译器证据

最初检查 encodeIntegerInto 中看似重复的 inline 判断；上一轮汇编显示它已被内联及消除，因此没有为不存在的机器成本另造优化。

本候选只改 encodeIntegerRecordInto 的外置路径：先按原有 `int64(bits)` 的 MinInt32/MaxInt32 范围判断，直接分别完成 4/8 字节内容和常量长度头写出、offset 常量累加。不再先通过 integerWidth 产生 width，再比较 width 和计算 `4 + width`。保留通用 helper 供其他调用点使用，没有新增 API。

候选的 header 写在内容之前；全部尺寸及预算仍在写出前完整预检，不改变可观察失败顺序。字段值读取、presence、nil、tag 跳过、布局消费和独占输出均保持。没有缓存标量、引入 unsafe、删除检查或扩大预算。

arm64 三个记录写出符号均为188条指令。源码中消失的中间状态不等于机器总指令减少；具体控制流与常量化后的代价由计时验证，不预先宣称加速。

## 测量

同一二进制三个静态调用树，选择仅发生在外层；不在 encodeLayout 链添加函数选择器。game_small/game_large/int32_edges 与 records7/prefix31，均测 Encode/PEncode；self、clone、两遍候选，六轮×四组ABBA/BAAB。双侧50ms预热，基线256次校准目标20ms，等调用数，GOMAXPROCS=1/GOGC=100/GOMEMLIMIT=off。所有GC、分配、长尾保留，不扣控制，没有摆放轮换。

三模式既有整数记录、预算、tag、描述替换/回退、nil及跨布局契约在race/checkptr下验证。harness fixture与正常入口分配parity、10项wire/cap预检、vet/build同时通过。源码重命名可逆，gofmt后独立还原；源码、fixture、生成文件与二进制哈希复核。没有为相同底层契约重复建立夹具。

当前合成记录的非inline值主要覆盖4字节内容；没有单独构造全8字节记录的性能排名。8字节的正确性由既有整数记录边界契约保护，不能从本次计时泛化其性能。

## 复现

```sh
python3 docs/benchmarks/2026-10-21-gosproto-record-width-flow/run.py --library /private/tmp/gsr-gosproto-fork --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-record-width-flow/analyze.py --output tmp/新的空目录
```

脚本固定基线，输出目录须不存在；分析覆盖报告内样本，复测前另存。完整控制、两遍结果与原始telemetry见ALL-CASES.md、analysis.json和各项JSON。

## 结果与决定

| 场景 | 第一遍中位/累计 | 第二遍中位/累计 |
|---|---:|---:|
| 小型 Encode | −0.46% / −0.84% | +0.59% / +3.89% |
| 小型 PEncode | −0.19% / −3.85% | +1.12% / +4.43% |
| 大型 Encode | 约0% / −3.26% | +0.22% / +2.55% |
| 大型 PEncode | +0.63% / +4.58% | −0.67% / −4.69% |
| 7项记录 Encode | +0.06% / −1.26% | +1.02% / +3.27% |
| 7项记录 PEncode | −0.88% / −0.28% | −0.47% / +5.70% |
| prefix31 Encode | −0.18% / −1.14% | +0.02% / +5.13% |
| prefix31 PEncode | −0.29% / −0.40% | −1.84% / −0.41% |

正数表示更慢。第一遍大型 Encode 中位实际为极小负值，精确值保留在 analysis.json，不用四舍五入的 −0.00% 冒充有意义收益。

暂不应用生产，保留候选源码和 prefix31 PEncode 的局部正面信号，不按收益门槛舍弃。当前没有确认大型 Encode/PEncode 稳定改善，7项记录 Encode 第二遍慢1.02%，prefix31 Encode 亦未同步改善。该写法还将通用 helper 的内容写出复制到记录路径，增加维护点；在收益未确认、静态指令未减少时，暂不承担这份重复代码。不能将结论扩大为“4/8字节专门写出没有优化可能”。

同源码clone大型PEncode −1.38%/IQR0.79pp，prefix31 PEncode −0.30%/IQR0.90pp；小型Encode self中位+4.18%、IQR13.00pp。控制提示噪声/布局影响，不作相减或硬件归因。所有累计反向和长尾保留。

40进程、240轮、960四元组、3840块，60条记录流程均成功。正常入口分配parity保持，大型Encode/PEncode仍为3968B/2alloc。实际库没有修改，上一轮9048cfa保持；七项无关删除与GSR正式依赖不变。仅执行本轮三模式契约/race/checkptr、fixture、分配parity、vet与计时，没有重复全库、官方oracle、fuzz或Go/TS生产验收。
