# 整数叶子 Decode：局部深度检查

基线gosproto `9048cfa`，完整Git归档、隔离实验模块。本轮不合并header游标或字段查找候选。

## 不变量与候选

canDecodeIntegerRecords在数组入口即时证明可达字段均为当前原生整数标量，专用decodeIntegerRecord不调用任何子解码器。它内部调用的budget.add只读取/更新elements，不读取depth；FieldByTag也没有回调或递归。在这个同步私有叶子路径内，临时修改共享depth不会被其他合法操作观察到。

因此将：

```go
budget.depth++
defer func() { budget.depth-- }()
if budget.depth > budget.limits.MaxDepth { /* 原错误 */ }
```

改为同位置的 `if budget.depth+1 > budget.limits.MaxDepth`，不写共享depth。保留本层深度检查、检查先于chunk长度的顺序，所有返回/错误后的budget.depth与原defer恢复结果相同。使用同一int加法表达式，不以 `depth >= limit` 替代而偷偷改变非典型私有整数边界行为。

预算元素计数、wire/长度、未知字段、独立对象/presence分配和Go写屏障均不变。通用递归decodeMessage完全不改；未来若此专用parser引入子解码器，必须重新维护共享深度，不得将叶子不变量推广到递归路径。

ARM64 parser符号388→292条，栈帧176→128B；原defer清理在多个返回点展开，静态差值不是每条记录固定节省96条执行指令，也不是整体加速比例。

## 验证方法

扩展既有实验差分的初始depth集合，覆盖−1/0/1/63/64/MaxInt；比较成功/失败后的完整budget、错误顺序、字段结果、输入快照。旧的wire/未知字段/截断/形状变化测试继续复用；没有另建重复业务夹具。相关契约开启race/checkptr，真实fixture/入口预检与vet/build通过。clone等于基线，候选逆变换还原基线，源码/fixture/生成文件/二进制哈希复核。

大型Decode/PDecode为目标，AddressBook为非目标对照。固定一种摆放，同二进制三parser通过共同实验入口调用。基线七批中位+三批pilot（60ms目标、40ms下限、最多三阶段、256～65536次调用），所有校准原始批次记录并独立复算。正式六轮×四组ABBA/BAAB，双侧预热50ms、等调用数、全部GC/分配/长尾保留，不扣控制。GOMAXPROCS=1/GOGC=100/GOMEMLIMIT=off。

```sh
python3 docs/benchmarks/2026-10-21-gosproto-decode-leaf-depth/run.py --library /private/tmp/gsr-gosproto-fork --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-decode-leaf-depth/analyze.py --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-decode-leaf-depth/check-calibration.py
```

输出目录须不存在，重测前另存本报告原始样本。完整控制、两遍与遥测见ALL-CASES.md、analysis.json、各JSON。

## 结果与决定

| 入口 | 第一遍中位/累计 | 第二遍中位/累计 |
|---|---:|---:|
| 大型Decode | +5.63% / +3.66% | −1.71% / +0.58% |
| 大型PDecode | −0.79% / +0.52% | +1.61% / +6.24% |
| AddressBook Decode（非目标） | +1.88% / +19.92% | +1.63% / +4.15% |
| AddressBook PDecode（非目标） | −1.35% / −4.70% | −0.32% / −0.04% |

正数表示B更慢。暂不应用生产，候选源码与叶子等价性证明保留。两次大型Decode中位反向，累计均慢；PDecode中位反向，累计均慢。第二遍Decode−1.71%与第一遍PDecode−0.79%未被删除，但不能仅挑局部正面值替代整体观察。

本次删除的是叶子私有路径无可观察作用的深度写回/defer，不是深度限制检查；正确性差分支持这项等价性。性能却未重复改善，说明大量静态清理代码减少不等同于热路径固定节省同样数量的执行指令。没有CPU trace足以进一步归因，不把变化全归结于defer、栈或缓存。

大型Decode clone +1.79%、IQR4.93pp，PDecode clone−0.51%、IQR4.11pp；控制不扣减。AddressBook不经过该叶子parser，其变化作为非目标噪声保留，不宣称候选改变了它的逻辑。

16进程、96轮、384四元组、1536正式块，30条实验流程成功。169个校准批次和调用数决策全部独立复算；本轮触发了额外pilot阶段，最终pilot均达40ms。正式块最短29.061ms、中位59.118ms、最长538.260ms；低于20ms为0，低于40ms为152。校准目标不是每个正式块的下限，运行状态变化仍能使部分块更快或更慢，全部保留，不补跑或剔除。

正常大型分配仍Decode10416B/532alloc、PDecode13872B/533alloc。实际库9048cfa、正式依赖和七项无关删除保持。无生产变化，未重复全库、官方oracle、fuzz和Go/TS生产验收；三模式错误/状态差分及相关race/checkptr已覆盖本轮局部风险。
