# 向量恢复后的解码成本诊断

2026-10-22。前轮已提交：gosproto `92a155a`、GSR `feb7c31`，均未推送。本轮只诊断当前实现，生产源码不变，未升级正式依赖，未触碰库中用户十项删除。

## 范围与复现

以 `git archive 92a155a` 建立完整独立库及独立harness，运行当前共享代表消息。大型游戏、896项整数数组、ASCII4KiB分别采Decode/Unpack/PDecode：9份CPU及9份全量分配profile。CPU每项2秒，分配每项5000次，均GOMAXPROCS=1、GOGC=100；所有解码产生独占结果，不复用调用方状态。

[reproduce.py](reproduce.py)、[inputs.json](inputs.json)、[commands.json](commands.json)保留来源和80条成功命令。源码指纹包含Go和汇编，harness、共享生成代码、夹具、二进制及18份原始profile哈希均在结束后核对。原始二进制profile保留在忽略的tmp目录；仓库保存文本视图、元信息、SHA及复现脚本。首次沙箱读取Go缓存失败，未进入采样；[失败记录](sandbox-first-attempt.json)保留，取得缓存访问权限后换全新目录完成，没有覆盖失败记录。

本轮没有新行为、没有修改测试契约；只运行夹具验证、harness vet和诊断构建，不重复全库发布验收。

## 分配：先确认不能凭空消除的部分

以下为ResetTimer后循环B/op与allocs/op，不使用包含初始化/预热的累计分配profile计算逐调用值。

| 消息 | Decode B / alloc | Unpack B / alloc | PDecode B / alloc |
| --- | ---: | ---: | ---: |
| 大型游戏 | 10416 / 532 | 3456 / 1 | 13872 / 533 |
| 整数数组 | 8288 / 4 | 4096 / 1 | 12384 / 5 |
| ASCII4KiB | 4176 / 5 | 4864 / 1 | 9040 / 6 |

Unpack只分配一次精确长度缓冲；B/op是分配器记账尺寸，不能据此断言返回cap=4864。整数数组PDecode的累计alloc_space中decodeIntSlice约65.83%、Unpack约32.91%，包含预热，不等于逐调用CPU比例。正文原生[]int64与独占展开缓冲都有真实存储需求；不能靠共享输入、复用全局缓冲或缩窄元素类型来“优化”。

## CPU：明确采样限制后再选题

默认GC下runtime.kevent占各profile约33.62%～87.44%，位于GC启动/结束的runtime调用栈。这是采样观察，不足以断定业务在执行同等比例的系统调用，也不将它简单扣掉重算“真实业务占比”。带profile的ns/op不作为新的PB排名。

为检查方向是否受GC配置影响，用同一二进制补整数数组三份CPU profile，GOGC=1000、GOMEMLIMIT=128MiB，仍每项2秒，见 [sensitivity.py](sensitivity.py) 与 [12条补充命令](sensitivity-commands.json)。这是诊断，不替换正式GOGC100数据，不扩大为生产调参建议。

补充整数数组PDecode中：Unpack累计1.63s（完整采样75.12%）、向量恢复0.53s（24.42%）、正文Decode累计0.27s（12.44%）。父子项不能相加。单独Unpack中，外层Go函数flat0.77s，OnesCount8另归属0.20s，向量恢复0.95s；Go预检与恢复两边都有实质成本。GOGC改变了采样构成，但“继续处理Unpack而非重写已领先的整数正文”方向一致。

大型PDecode默认GC下正文Decode累计39.83%、Unpack累计12.55%；说明大型结构和整数数组的瓶颈不同，仍保留Body/Packing两条主线。ASCII的FF恢复占比小，分配与GC影响更突出，不应拿高密度普通mask的收益外推ASCII。

## 下一步实验边界

1. **优先整数数组Unpack预检普通mask循环**：完整输入和预算检查逐条保留，尝试减少游标/切片状态维护和重复控制依赖。不是合并预检与写出，不提前分配或延迟坏尾部检查；先看机器码，再用同模块基线/候选交替复核Unpack与真实PDecode。
2. `bits.OnesCount8`在当前ARM64机器码已是256项常量表读取，见 [objdump](unpack-objdump.txt)。不再凭函数名误判为逐位计数，不另造一张相同用途的表。
3. 若改恢复循环，重点是普通word的连续地址依赖与分支；FF已经64B批量复制，不再把整数消息误称为FF问题。小型AddressBook/ASCII和purego是必要对照。
4. Body侧保留大型结构对象构建/字段写入调查；本轮未证明某种新对象布局能保持API及独占语义，不为压532次分配引入共享子对象。

候选需要覆盖全部mask、FF/零段、短尾、预算不足与截断同时存在时的错误优先级；保持精确cap、只读源及失败无部分结果。门槛是重复可解释的收益及可接受代价，不是固定5%。本轮尚未生成候选补丁，不能将诊断写成已实现提速。
