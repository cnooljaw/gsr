# 整数叶子预检：直接追加最终布局项

基线gosproto `9048cfa`。本轮只改encodedIntegerRecordSize，不合并近期Decode/Pack候选。

## 等价性和资源约束

纯整数记录预检不产生子结构/整数数组布局项，其自身布局在本次函数内连续且唯一。原流程先`layout.add(0)`、成功时`layout.set(index,headers)`，候选在所有该记录字段验证成功后`layout.add(headers)`；nil仍追加0并返回2字节空结构大小。没有子布局，因此成功情况下布局项数量、顺序和值完全相同。

失败前不再为当前失败叶子留一个零占位，私有临时layout的count/内容可能与旧失败快照不同；不能宣称失败私有布局逐字相同。该layout只属于本次Encode，调用链遇错即返回nil输出，不被后续写出/复用；错误/预算检查的执行顺序和budget状态不改。分配被延后，失败时可能更少而不是更早/更多；成功的栈内32、初次外溢128、倍增序列与输出容量均保持。没有根据未验证数组长度预分配。

通用结构仍必须先占位再遍历子结构，本轮不修改它。没有删除预检、缓存标量、使用池或增加unsafe/API。

## 测量方法

扩展既有静态调用树，使encodedSize、fieldSize及encodedIntegerRecordSize也分别属于baseline/clone/candidate树；避免共享旧预检函数导致候选实际未执行。原写出Message/Field/Record/Array树也各自静态绑定。选择只在外层，encodeLayout指针不会跨内部函数选择器逃逸。正常入口分配parity单独验证，重命名逆变换/gofmt后还原都检查。

大型、7项记录、prefix31三场景，均测Encode/PEncode；self/clone/两遍候选。固定一种摆放，六轮×四组ABBA/BAAB。使用七批基线中位+三批pilot预定校准，60ms目标/40ms pilot下限，全部校准批次及正式GC/暂停/分配/长尾保留，双侧等调用数，不扣控制。

三模式既有nil/短数组/跨32项/相邻数组/命名指针/当前描述变化/预算失败契约在race/checkptr下验证；fixture、正常分配parity和全部测量项预检wire/cap、vet/build通过。没有新增同底层重复夹具。源码/fixture/生成文件/二进制哈希复核。

记录预检符号256→304条：nil和非nil路径分别内联追加逻辑使静态代码增多，不等于每次多执行48条指令；有无收益必须实测。

```sh
python3 docs/benchmarks/2026-10-21-gosproto-layout-final-append/run.py --library /private/tmp/gsr-gosproto-fork --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-layout-final-append/analyze.py --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-layout-final-append/check-calibration.py
```

输出目录须不存在，重测前另存报告原始样本。完整控制与每块数据保留在ALL-CASES.md和各JSON。正式依赖仍v0.4.0。

## 结果与决定

| 场景 | 第一遍中位/累计 | 第二遍中位/累计 |
|---|---:|---:|
| 大型Encode | +0.86% / +1.35% | +0.79% / +0.48% |
| 大型PEncode | +4.28% / +2.98% | +7.88% / +9.77% |
| 7项记录Encode | −1.07% / −2.24% | −0.52% / +1.15% |
| 7项记录PEncode | −3.76% / −2.94% | −1.07% / −0.71% |
| prefix31 Encode | −3.92% / −6.58% | +1.62% / +0.57% |
| prefix31 PEncode | −2.28% / −1.90% | −1.19% / −1.22% |

正数为耗时增加。暂不应用：短场景的真实小收益和失败私有占位减少均保留，但主目标大型Encode/PEncode两遍中位及累计都慢。不是因为收益小于门槛，也不能拿短场景的正向代替大型结果。

控制亦有波动：大型PEncode clone中位+0.25%、IQR7.65pp；prefix31 PEncode clone−7.25%、IQR9.18pp。没有作相减、删除长尾或指定缓存/GC等根因。源码可确定少了一次占位/回填，却未产生大型收益，不能按减少一次写入就宣称优化成功；额外nil路径与代码布局代价未被独立定量分解。

24进程、144轮、576四元组、2304块，40条流程通过。240个校准批次完整保留，决策独立复算通过。三个7项记录Encode进程达到65536调用上限，最终pilot中位约37ms，未达到40ms，按预定规则停止而非延长或重跑。全部正式块最短30.405ms、中位54.971ms、最长157.951ms；低于20ms为0，低于40ms为428。没有伪称所有pilot/块都达40ms。

正常入口分配parity保持，大型Encode/PEncode3968B/2alloc，公开输出len/cap不变。失败时布局少占位属于私有诊断状态差异，未将其写成已有公开契约改变。实际库9048cfa、正式依赖和七项无关删除不动。

因不应用生产，本轮不重复全库、官方oracle、fuzz和Go/TS生产验收；三模式预算/错误/wire/cap/race/checkptr及正常入口分配检查已完成。候选与全部数据保留作为后续布局优化的取舍证据，不继续在此改法上叠加新分支掩盖大型退化。
