# 整数记录预检：局部元素预算计数

基线gosproto `9048cfa`，完整Git归档。没有合并布局最终追加或Decode候选；layout仍按原顺序占位/回填。

## 候选

encodedIntegerRecordSize内的每个出现字段仍先检查`1 > MaxElements-elements`，通过后增加计数，再检查tag间隔与字节预算。只把elements暂存在局部变量，成功和预算/tag/字节错误出口显式写回budget.elements，减少正常循环每字段的共享状态写入。

该纯整数叶子不递归；内部layout.add/set不读取budget，字段读取不触发业务回调。所有可观察返回点及后续错误构造之前写回，深度、nil、预算检查位置与错误顺序保持。仍使用相同的减法/自增，不用可能在边界改变溢出行为的简化比较。不删除逐字段检查、不预先扣整条记录或按未验证数组长度接纳预算。

没有缓存业务值、扩大unsafe、增加API、合并presence指针或改变输出容量。新写法需要各错误出口维持写回，测试必须覆盖这些出口；不能只用成功wire证明等价。

## 直接状态差分

在既有三模式契约外增加专门的预检状态对照，10368组：三种当前tag（正常/大间隔/负数）×四种值形态（nil/空/全字段/部分字段）×两种深度×六档元素上限×六档字节上限×四个初始元素计数×三个布局前缀。对baseline/clone/candidate比较大小、错误文本与ErrLimit/ErrEncode分类、完整budget及layout。它验证的是旧wire成功测试未直接观测的错误后内部计数增量，未重复搭建业务夹具。

状态对照及既有nil/描述替换/跨32项/相邻数组/命名指针/预算用例运行race/checkptr。正常分配parity、fixture、每个测量项wire/cap预检和vet/build也验证。源码/gofmt还原与二进制/输入哈希复核。

## 性能方法

预检Size/FieldSize/RecordSize与写出Message/Field/Record/Array各自有静态baseline/clone/candidate树，只在外层选择，避免layout指针在内部选择器处逃逸。固定一种摆放；大型、records7、prefix31的Encode/PEncode，self/clone/两遍候选。使用七批基线中位+三批pilot校准（60ms目标/40ms下限/最多三阶段/256～65536次），全部校准和正式块保留。正式六轮×四组ABBA/BAAB，双侧50ms预热、等调用数，GOMAXPROCS1/GOGC100/GOMEMLIMIToff，不扣控制或删GC/长尾。

记录预检符号256→252条，不据此承诺整体加速。

```sh
python3 docs/benchmarks/2026-10-21-gosproto-record-local-budget/run.py --library /private/tmp/gsr-gosproto-fork --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-record-local-budget/analyze.py --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-record-local-budget/check-calibration.py
```

输出目录须不存在，重测前另存报告内原始数据。

## 结果与决定

| 场景 | 第一遍中位/累计 | 第二遍中位/累计 |
|---|---:|---:|
| 大型Encode | −3.06% / −2.11% | +0.71% / +9.84% |
| 大型PEncode | −1.29% / −2.77% | −0.97% / +1.90% |
| 7项记录Encode | −0.32% / +2.19% | +0.40% / +3.73% |
| 7项记录PEncode | −0.22% / −4.50% | +2.30% / +3.69% |
| prefix31 Encode | −1.22% / +1.23% | −1.13% / −4.54% |
| prefix31 PEncode | −2.79% / −6.28% | −0.04% / +2.22% |

负数为耗时下降。保留大型PEncode双遍中位改善及prefix31 Encode的局部信号，但暂不应用。不是因为改善小于某门槛：大型Encode方向反转，PEncode第二遍累计反向；错误出口手动写回也增加维护点。在未确认稳定整体收益时，不将复杂度搬进生产，候选和10368组状态验证作为后续证据保留。

大型Encode clone中位−1.25%、IQR7.04pp，PEncode clone−0.12%、IQR5.33pp；PEncode self+3.91%、IQR6.83pp。没有相减控制或把所有变化归因于共享写入减少。第一遍正面、第二遍不利和全部长尾同样保留。

24进程、144轮、576四元组、2304块，40条流程成功。243个校准批次完整记录并独立复算通过，四个records7 Encode进程达到65536调用上限后未达40ms；其余最终pilot达标。正式块最短32.837ms、中位56.768ms、最长432.399ms，低于20ms为0，低于40ms为459。目标时长不是所有块的保证，不选择性延长或重跑。

正常入口分配parity保持，大型Encode/PEncode3968B/2alloc，返回wire/cap保持。实际库9048cfa、正式依赖及七项无关删除不变。本轮不应用生产，因此未重复全库、官方oracle、fuzz或Go/TS生产验收；局部预算状态/race/checkptr和既有三模式契约已通过。
