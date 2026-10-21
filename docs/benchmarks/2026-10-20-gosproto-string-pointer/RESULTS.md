# 原生字符串指针受控写入

2026-10-20，基线 gosproto `5360fe0`。**保留候选：AddressBook Decode两轮中位改善4.59% / 4.58%，PDecode改善3.47% / 3.11%；64字符串结构Decode改善约15%。** 这些小于5%的业务收益有重复证据，不因比例门槛舍弃。大型整数消息未宣称提速，空字段结构的局部代价保留。

已将验收通过的3个运行时文件、永久回归和既有fuzz扩展提交为独立库 `f5de803`；GSR报告同步提交。上一轮预算证明报告已提交GSR `a1dc73b`，未推送。正式依赖仍为v0.4.0，原六项用户删除保持，未发布。

## 改动与安全证明

字段及schema构造时追加私有nativeStringPointer标志，识别实际Go类型恰为原生*string。解码每条消息仍即时核对临时结果的实际Go类型；每次字段存储还核对当前字段的owner、Wire和Array。成立时借用已有encOffset作类型化Go指针赋值，减少record.Field/reflect.ValueOf/Set；失败继续原字段索引反射语义，不使用外来owner的offset。

decodeMessage仍先做原wire、预算、字段查找和inline合法性检查，随后为每个出现的原生字符串独立new(string)，并用string(content)复制输入。没有字符串驻留、unsafe.String、借用输入、池或共享对象；nil与非nil空字符串presence保持。Go类型化赋值保留写屏障，不用uintptr或字节复制存指针。

命名字符串继续原decoder；当前Fields可在调用间替换，owner/Wire/Array变化时逐次回退，不缓存公开字段“已经证明”的状态。Wire/Array变化不会自动重新绑定原私有decoder，原来允许或拒绝的行为均保留；新增回归明确核对这些已有语义。encoder没有新增快路径，整数记录数组的专用循环保持。

## 测量

负值表示候选耗时更低；中位与实际总时间/调用数累计均列出，不扣控制、不挑选较好指标。

| 场景 | pass1 中位 / 累计 | pass2 中位 / 累计 |
| --- | ---: | ---: |
| AddressBook Decode | -4.59% / -4.57% | -4.58% / -4.46% |
| AddressBook PDecode | -3.47% / -2.85% | -3.11% / -2.84% |
| 单字符串 Decode | -3.43% / -4.07% | -3.07% / -2.42% |
| 8字符串 Decode | -12.38% / -13.56% | -11.53% / -5.94% |
| 8字符串 PDecode | -8.57% / -8.06% | -10.32% / -12.98% |
| 64字符串 Decode | -15.20% / -15.32% | -15.78% / -19.02% |
| 64字符串 PDecode | -11.13% / -6.29% | -11.47% / -14.78% |
| 64字段整数/字符串混合 Decode | -11.14% / -11.42% | -11.95% / -11.47% |
| 同上 PDecode | -8.92% / -7.87% | -7.81% / -10.31% |
| 128字符串记录 Decode | -9.58% / -8.04% | -9.40% / -9.61% |
| 128字符串记录 PDecode | -7.21% / -6.84% | -7.38% / -6.74% |
| game_large Decode | +0.17% / +1.46% | -0.25% / -18.78% |
| game_large PDecode | +0.15% / +1.33% | +0.07% / +3.61% |
| 64字符串全缺失 Decode | +0.16% / -1.12% | +1.11% / +1.20% |
| 同上 PDecode | +0.72% / +0.73% | +1.31% / +1.21% |

全部26入口及self/clone在 [ALL-CASES.md](ALL-CASES.md)。AddressBook Decode clone−0.23%/IQR1.33pp、PDecode clone+0.39%/1.12pp；64字符串Decode clone−0.09%/0.88pp，128字符串记录Decode clone+0.16%/1.34pp。目标字符串收益两轮重复，且对应累计也快，不用同阶控制相减修正。

非目标并非全部改善：空字段结构多出原生能力/实际类型入口工作，存在约1%的局部代价；命名字符串走反射，64字段Decode两轮−0.30%/+0.16%，没有稳定收益。大型PDecode两轮中位微慢，整数数组/ASCII结果也有翻转，不宣称全面提速或把大型Decode次轮累计−18.78%当稳定提速。

全部28项中位/累计反向、GC、暂停及长尾保留。块级分配次数中位全相同，未用少对象换速度；累计分配仍有尾差。实际块约3.68～147.72ms，校准目标20ms不等于每块实际长度；最大堆快照87,134,872B不是峰值。

## 工程代价与比较边界

ARM64通用parser机器字行（含填充）660→768、栈320→352B；defer清理helper仍8行，静态GC写屏障调用点1→3。字符串路径少了反射，但整个parser增加分支和代码体积，这些代价真实保留。调用点不是动态执行次数，静态条数也不是提速证明。空字段/命名类型和大型整数消息正是用于观察非目标成本。

同一5360fe0归档内复制完整parser为baseline/clone/candidate，共享同一模块、schema、分配器和公共Decode/DecodePacked。临时decodeMessage通过共同函数变量分派，递归子结构也使用被选parser；已有整数记录专用循环没有字符串字段，未改动。切换发生在计时外，适配器不进入生产。共有的元信息增加了私有字符串标志，baseline parser函数体仍等于原提交；这是一次候选比较，不把绝对值当作未修改元信息的生产API排名。

5种原代表消息，加1/8/64字符串、命名64、混合64、全缺失64、原生/命名字符串记录128项，共13消息×Decode/PDecode。额外动态结构不替换原夹具。每臂预热50ms，基线256次校准约20ms、两臂固定相同调用数；6轮×4个ABBA/BAAB四元组，GOMAXPROCS1/GOGC100/GOMEMLIMIT=off。104进程、624轮、2496四元组、9984块，140条准备/预检/测量日志命令均成功。

没有重新采样PB/TS，不将本轮百分比乘旧PB耗时生成新排名；AddressBook改善也不等于已经追平PB。Packing算法与正式依赖均未改。

## 增量回归与完整验收

新增永久回归相对已有原生指针测试补充：纯字符串schema的类型绑定、空字符串presence、混合字段/非零padding、字符串记录数组、命名类型回退、低GOGC、不同字段/记录/调用间独立指针；修改后续结果或原输入、执行GC后先前结果仍不变。坏尾部、非法inline保持失败原子性；Wire/Array变化、nil描述及外来owner+不同offset仍按原反射索引处理。

新增回归先在原5360fe0完整归档上通过，再对去除实验分派的完整候选验收。扩展既有fuzzDocument的原生*string字段，让Codec/Decode fuzz覆盖新写入路径及缺失presence，不另建重复fuzzer。

10条完整验收流程（含2次gofmt）全部通过：基线新回归、完整库及固定官方oracle、全库race+严格checkptr、vet、两个10秒fuzz、GSR受影响三包、Go/TS集成7/7零skip。Codec477124次；Decode10968次后计数停增，11秒正常退出，保留原文，不将有限fuzz当完整证明。oracle文件哈希与集成服务器buildinfo均核对，后者确实replace到本次验收候选。

验收将实际parser/helper恢复到正常私有位置，没有实验全局选择器。仅修正计时归档中新增字段旁误附的旧类型注释，并扩展测试，运行逻辑与被测候选相同。实际库所有Go源逐文件等于验收快照；复制前确认只有原六项删除，复制后未恢复go.mod或用户删除。production-source.json记录当前未提交状态。

计时源码、harness、生成输入、夹具、唯一SP模块与二进制指纹前后不变，逐块比例独立复算至1e-12。报告、完整验收日志、精确候选源和原始块一并保留。下一步基于这份实现核对Body剩余成本，再按热点选择一项；Body与Packing继续开放。

```sh
python3 docs/benchmarks/2026-10-20-gosproto-string-pointer/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-string-pointer/analyze.py --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-string-pointer/accept.py \
  --experiment /absolute/new-output --library /private/tmp/gsr-gosproto-fork --output /absolute/new-acceptance
```

输出目录须不存在，保留Go缓存。原始计时在tmp/gosproto-string-pointer-20261020，完整验收在tmp/gosproto-string-pointer-accept-20261020；二进制不提交。库代码已提交为 `f5de803`，未推送。

2026-10-21提交核对：本次5个Go文件哈希仍与验收一致；另外发现 `encode_test.go` 的工作树删除，连同原六项删除全部保留，均未纳入提交。上述完整验收使用的是包含该测试的完整快照。
