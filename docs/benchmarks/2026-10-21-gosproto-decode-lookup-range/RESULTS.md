# Decode 字段查找：单次无符号范围比较

基线为 gosproto `9048cfa`，完整Git归档，同模块同进程实验。没有合并前轮header游标候选，header仍按原index读取。

## 候选与边界

公开FieldByTag的dense路径当前判断 `tag >= 0 && tag < len(st.dense)`。候选在实验私有helper中用 `uint(tag) < uint(len(st.dense))` 表达同一范围，再保持原dense索引、当前Fields读取和map回退。Go切片长度非负，对任意int该范围判断等价，负tag仍进入map回退，不私自把它拒绝。

本实验只让整数记录parser调用此helper，未修改公开FieldByTag或其他生产调用点。未合并私有描述、缓存字段/指针、弱化FieldByTag的Fields边界检查。budget、wire、错误顺序、presence与独立对象分配保持。

基线ARM64内联查找仍有tag负数位TBNZ检查；候选该位置改为无符号范围比较，helper已内联，没有新增helper调用。整个parser符号两侧仍388条（包括符号填充），不能声称总指令减少。保留汇编证据，不用源码行数推断加速。

## 验证与测量

新增实验内查找对照，覆盖MinInt/MaxInt、负tag映射、dense命中/空洞、稀疏map回退和当前Fields变nil；旧parser差分继续比较合法/未知字段/截断/错误/预算状态/已解码字段/输入快照。新增查找用例覆盖旧wire用例无法直接访问的负tag查找契约，没有重复搭建业务夹具。相关契约开启race/checkptr，真实fixture、入口预检及vet/build同时验证。

大型Decode/PDecode为目标，AddressBook两入口为非目标对照（不走整数记录parser），固定一种函数摆放。沿用上轮基线七批中位+三批pilot规则：60ms目标、40ms pilot下限、最多三个pilot阶段、256～65536次调用。全部校准批次记录，独立重算调用数决策。正式比较每进程六轮×四个ABBA/BAAB，双侧预热50ms，等调用数，保留所有GC/分配/长尾，不扣控制。

GOMAXPROCS=1、GOGC=100、GOMEMLIMIT=off。源码/fixture/生成文件/二进制哈希检查，clone等于基线，候选parser逆变换还原基线。没有修改正式Benchmark工具默认值。

```sh
python3 docs/benchmarks/2026-10-21-gosproto-decode-lookup-range/run.py --library /private/tmp/gsr-gosproto-fork --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-decode-lookup-range/analyze.py --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-decode-lookup-range/check-calibration.py
```

目录须不存在，重测前另存本报告原始记录。正式依赖、实际库和七项无关删除保持原状。

## 实测及决定

| 入口 | 第一遍中位/累计 | 第二遍中位/累计 |
|---|---:|---:|
| AddressBook Decode（非目标） | +0.77% / +0.02% | +0.93% / +1.40% |
| AddressBook PDecode（非目标） | −0.82% / −1.02% | +0.67% / −5.25% |
| 大型Decode | +0.15% / −0.43% | −0.93% / −0.72% |
| 大型PDecode | +0.77% / +0.93% | +0.19% / −1.91% |

正数表示B耗时更多。暂不应用：大型Decode有第二遍−0.93%和两遍累计改善的局部信号，完整保留；PDecode两遍中位却都略慢，尚未形成稳定组合收益。不是因为改善未达到某个比例门槛。没有把公开FieldByTag改成候选，也没有为私有路径增加一份生产路由实现。

大型Decode clone +0.73%、IQR1.89pp，PDecode clone +3.76%、IQR3.34pp；PDecode self −3.19%、IQR3.06pp。未扣控制、删除长尾或按其中一个方向选结论。非目标AddressBook的差异也保留，不能宣称改变了它的业务解码算法。

16进程、96轮、384四元组、1536块，30条实验命令成功。160个校准批次全部保留，所有调用数/停止决策独立复算通过；本轮全部首轮pilot达标。正式块最短44.595ms、中位58.951ms、最长534.264ms，低于40ms为0。方法消除了本次短块风险，但更长块仍有长尾和控制波动，不能宣称消除了噪声。

大型正常分配保持Decode10416B/532alloc、PDecode13872B/533alloc。生产9048cfa、正式依赖和七项无关删除不变；本轮只验证实验，不重复全库、官方oracle、fuzz或Go/TS生产验收。后续不以本局部样本直接推广到所有FieldByTag调用点。
