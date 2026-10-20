# 消息头顺序游标 Decode 实验

2026-10-20，基线gosproto `aef6876`，已包含字符串数组写槽及Unpack改动。单一候选只改decodeMessage头部读取：预算和完整头长校验后取得`headers := chunk[2:total:total]`，逐项读uint16并推进两字节，替代每项`2+index*2 : 4+index*2`索引表达式。payload的total游标独立保留；字段、tag、当前元信息、指针证明、对象数量、失败原子性和全部预算/错误检查不变。没有新增生产API、unsafe、缓存或复用输出。

## 结论

**暂不采用，目标大型正文Decode没有改善。** 不是因收益小于某个百分比而舍弃：大型Decode两轮中位/累计均略慢，64字段内联对照也没有重复改善；PDecode的小幅正向观察完整保留，但不足以确认来自头部表示变更，且候选增加了栈成本。不能用单轮混合字段的正向替代主目标或宣布小收益都无价值。

| 场景 | pass1 中位 / 累计 | pass2 中位 / 累计 |
| --- | ---: | ---: |
| game_large Decode | +0.32% / +2.07% | +0.12% / +0.65% |
| game_large PDecode | -0.79% / -2.05% | -0.29% / -4.61% |
| 64字段内联 Decode | -0.16% / +2.04% | +0.39% / -1.72% |
| 64字段混合 Decode | +0.71% / +0.62% | -4.38% / -8.27% |
| 64字段混合 PDecode | +1.12% / +4.24% | -6.16% / -9.73% |
| 空头部 Decode | -0.28% / +5.48% | +1.74% / +3.11% |

负值为候选耗时更低。全部20入口、self/clone及两轮见 [ALL-CASES.md](ALL-CASES.md)，全部块/GC/分配、轮次和长尾保留，不相减控制、不删离群值。中位与实际时间/调用数累计分别列出，不择优替换。

大型Decode self中位−1.71%、IQR0.92pp，同源clone−1.20%、IQR3.43pp；大型PDecode clone−0.67%、IQR2.07pp。控制表明当前亚百分比观察仍有噪声，不能把候选PDecode的−0.79%/−0.29%直接归因于头部游标；也不以“没有达到5%”为拒绝理由。

## 静态代价与参考

ARM64完整parser机器字行（含填充）660→660，panicIndex静态调用点4→5、panicSlice* 4→2；调用点不是实际执行次数。栈帧320→336B，保留了额外的头部切片状态；defer清理helper不变。减少索引表达式没有自动减少总指令或栈。

只读核对官方Sproto `sproto.c` 的sproto_decode：同样先校验完整头部，分开datastream，头项仍按`stream+i*SIZEOF_FIELD`读取。顺序游标只是本次Go实现候选，不是官方wire要求，也不能凭更像某种写法预测效率。参考源码位置及哈希见reference.json。

## 同模块、全递归比较

一份aef6876归档内复制完整parser为baseline/clone/candidate，临时decodeMessage入口经共同函数指针分派，子对象递归同样进入选中的parser；切换在计时外进行。所有臂共享模块、schema、分配器和公共Decode/DecodePacked路径。共同分派带来实验开销，不将本轮绝对值作为生产API排名；分派与实验副本不进入生产。clone函数体去除名称/空白后与基线相等。

原5种代表消息×Decode/PDecode，加8/64字段内联、64字段混合非内联、64字段全缺失、tag65535跳跃对照×两入口，共20项。动态结构只用于额外对照，不更改原夹具。每臂预热50ms；按基线256次校准约20ms，两臂相同固定调用数；6轮×4个ABBA/BAAB四元组。GOMAXPROCS1/GOGC100/GOMEMLIMIT=off。完整v2矩阵80进程、480轮、1920四元组、7680块；逐块在计时外记录MemStats，保留GC、暂停、堆快照与分配尾差。快照最大值不是峰值。

## 初次夹具失败与修正

初次运行完成18个self入口后，新增header_jump对照在调用Encode准备输入时失败，错误为`invalid encode value: tag gap`，未进入该项计时或任何候选比较。当前编码器在limits.go/encode_integer_record.go拒绝gap>32768；元信息和解码器允许的tag范围不等于编码器可一次跳跃的跨度。本轮没有修改这一行为。

这是实验准备错误，不是候选造成的生产回归。29条初次命令中1条失败；18份已完成self样本、命令输出、计划及当时harness保留在initial/，源码指纹重新核对未变。没有将这些样本混入完整矩阵，也不将它们当作噪声删掉。

修正对照为手工合法解码wire：头项`ffff / fffd / 0010`，最终tag65535、值7；通过baseline/candidate的Decode和DecodePacked语义检查。给全部20入口增加计时前预检，全部通过后才启动新的完整矩阵。v2保留完整计划与源码/二进制指纹，不覆盖初次目录。二进制重建后不跨版本拼接结果。原代表输入未改，也未为消除失败而降低tag边界。

## 验证范围

三种全递归parser通过新增头部回归与相关已有预算/未知字段/native pointer/SchemaEvolution等测试的race＋严格checkptr；harness fixture语义、固定wire、消费长度与padding、vet和编译通过。全部新增对照在采样前核对baseline/candidate结果相等。v2准备/预检/测量110条命令均成功。

新增回归保护0/1/2/7/8/9/16/255/256/257/65535项头部的完整消费与未知字段、恰好/不足预算、截断、未知非内联载荷仍须验证、失败原值不变，以及纯跳跃链允许结束但越界实际字段拒绝。这是对最大头项数量及循环推进方式的增量覆盖；未向生产库加入与未采纳候选绑定的测试。

计时前冻结全部Go源码、生成输入、JSON夹具和二进制，完成后核对；独立复算全部块比值至1e-12。未采纳，不重复全库/oracle/fuzz/GoTS完整验收，不声称本轮覆盖这些。独立库仍aef6876，仅原六项用户删除，正式GSR依赖v0.4.0不变。报告和文档留审查，尚未提交；未推送或发布。

## 下一方向

停止围绕头部游标表示继续微调。当前大型Decode还包含原生整数记录的逐字段owner/形状证明与对象构造成本；下一候选借鉴已采用的Encode整数记录数组：在数组入口核对当前字段集合与实际子对象类型，再评估将证明摊到多个子对象，保留通用回退。

不能直接照搬Encode的nativeIntegerFields：它会跳过当前Tag<0的字段，而Decoder仍按原dense/tagMap索引访问当前Fields。Decoder证明须覆盖这些实际可达项；遇到nil、短Fields或形状不符先安全回退，不能因提前扫描而增加原本未访问字段的panic或拒绝。

这仍是待验证方案：每项wire、标量宽度、预算、深度、未知字段、消费长度、失败原子性、独立对象分配和Go写屏障继续保留；公开Fields可在调用间变化，不能跨调用缓存“已经证明”的状态。先用现有Fields替换/命名类型/GC测试确认安全，再测现有大型消息，不以新合成数组替代业务夹具。尚未实现或形成新公共架构。

```sh
python3 docs/benchmarks/2026-10-20-gosproto-header-cursor/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-header-cursor/analyze.py --output /absolute/new-output
```

输出目录须不存在，保留Go缓存。本轮完整输出为tmp/gosproto-header-cursor-v2-20261020，初次中断输出为tmp/gosproto-header-cursor-20261020；二进制不提交。生产算法维持aef6876，不把本轮负结果回填到既有PB/TS历史基线。
