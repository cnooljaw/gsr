# 整数记录预算：返回计数、唯一调用点提交

基线gosproto `9048cfa`。从[已有数据审计](../2026-10-22-gosproto-evidence-audit/RESULTS.md)选择局部预算PEncode的轮级小收益信号继续研究；旧数据只用于探索，不作为本候选的独立确认。没有合并其他候选。

## 与前轮的区别

前轮局部预算在helper的多个错误出口手工写回，维护时容易遗漏。本变体让私有encodedIntegerRecordSize返回`(size,elements,error)`，由真实唯一调用点fieldSize在判断err之前无条件提交一次`budget.elements=elements`。helper仍逐字段检查剩余元素预算，通过后计数，再执行原tag/字节检查；每个返回都携带本次已消费计数，不提前扣除整条记录。

nil、深度/字节早拒绝也返回原始计数。layout仍原样占位/回填，错误后布局和元素数不变。没有新接口、选项、缓存或wrapper层进入生产；这是两处私有函数间传值。通用结构、输出容量、预算优先级和所有权保持。

helper本体指令仍256条，栈帧112→128B，fieldSize符号均996条。额外返回值并非免费；不因为写回位置更整洁就预先宣称提速。

## 验证

复用并适配10368组完整budget/layout直接对照，测试适配器执行与真实调用点相同的一次提交。三模式公开Encode预算/错误/wire/cap契约同时验证真实fieldSize集成。错误文本和ErrLimit/ErrEncode分类、大小、nil/空/全/部分字段、当前tag变大间隔/负数、初始元素计数和跨32项前缀均覆盖；相关测试开启race/checkptr。不是只检验成功wire。

正常入口分配parity、fixture、所有测量项预检、vet/build通过。Size/FieldSize/RecordSize与写出树分别静态绑定，选择仅在外层，防止layout逃逸。候选limits源单独保存和还原核对；所有源码/输入/二进制哈希检查。

## 方法

大型与prefix31，Encode/PEncode；self/clone/两遍候选，固定一种摆放。沿用七批基线中位+三批pilot，目标60ms/pilot下限40ms，最多三阶段、256～65536次。六轮×四组ABBA/BAAB，双侧50ms预热、等调用数、全量GC/分配/长尾保留，不扣控制。GOMAXPROCS1/GOGC100/GOMEMLIMIToff。

```sh
python3 docs/benchmarks/2026-10-22-gosproto-record-budget-return/run.py --library /private/tmp/gsr-gosproto-fork --output tmp/新的空目录
python3 docs/benchmarks/2026-10-22-gosproto-record-budget-return/analyze.py --output tmp/新的空目录
python3 docs/benchmarks/2026-10-22-gosproto-record-budget-return/check-calibration.py
```

目录须不存在，分析将写入本报告目录，重测前另存全部原始数据。

## 结果与决定

| 场景 | 第一遍中位/累计 | 第二遍中位/累计 |
|---|---:|---:|
| 大型Encode | +0.58% / +1.06% | −4.63% / −4.41% |
| 大型PEncode | −0.12% / +10.67% | −0.95% / −1.23% |
| prefix31 Encode | +0.85% / −0.53% | −0.99% / −1.66% |
| prefix31 PEncode | −4.46% / −5.40% | +3.76% / +5.89% |

负数为更快。集中提交改善了写回位置的维护性，但尚未确认稳定整体收益，暂不应用。大型PEncode双遍中位的局部小收益保留；第一遍累计+10.67%和prefix31反转不能省略，也不能只挑大型Encode第二遍−4.63%宣布成功。不设最小收益比例，不将维护性改善自动换算成性能收益。

大型Encode clone−0.39%、IQR1.27pp，PEncode clone−0.49%、IQR4.09pp；大型Encode self+2.44%、IQR5.49pp。没有控制扣减、离群删除或系统根因猜测。一次额外int返回、caller提交与16B栈增长的具体贡献没有被独立定量，不能把变化全归因于其中某项。

16进程、96轮、384四元组、1536块，30条流程通过。163个校准批次和决策独立复算通过，所有最终pilot达40ms；正式块最短46.063ms、中位59.327ms、最长335.961ms，低于40ms为0。正常分配parity保持，大型Encode/PEncode3968B/2alloc。

状态验证与所有原始数据保留，实际库生产源码仍为9048cfa，正式依赖不改。收尾观察到README.md、pack_test.go、sproto_test.go另有删除，连同先前七项共十项删除均未触碰；验收/计时使用完整Git归档，不恢复实际工作树。因不应用生产，未重复全库、官方oracle、fuzz、Go/TS生产验收；三模式公开错误预算/完整内部状态/race/checkptr、fixture/vet与源码hash检查已完成。
