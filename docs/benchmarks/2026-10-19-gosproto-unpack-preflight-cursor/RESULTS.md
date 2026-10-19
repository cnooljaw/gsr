# Unpack 完整预检输入游标（未采用）

2026-10-19。前轮源游标已本地提交为库 `af9169d`、GSR `6b47822`，未推送。本轮只有隔离候选：完整预检从remaining尾切片改成position输入索引。**业务收益保留为证据，但当前未采用通用替换**；主库从未应用补丁、全部18份Go源码仍等于af9169d且干净。新诊断/实验报告未提交，正式依赖v0.4.0，Decode继续开放。

## 动机与契约

先做[旧/当前8进程CPU及实际wire诊断](../2026-10-19-gosproto-unpack-cost-profile/RESULTS.md)。它没有确认FF接稀疏/短零段退化的唯一原因：短零段不执行普通mask used循环，FF接稀疏则主要为固定0x11 word。完整预检仍有可见成本，故只验证输入游标表示；不把profile的不同分母份额当作A/B基线。

原先remaining=remaining[consumed:]，新候选用position、available=len(src)−position及src[position]，仅零段helper仍构造受限区间src[position+2:position+end]。恢复阶段保持已提交的源游标、mask提取/写出、零/FF路径、分配/清零和ownership；没有新表、缓存、池、unsafe或公开API。

证明：迭代起点0<=position<len(src)，原remaining等于src[position:]，available就是原len(remaining)。所有相对读取与zeroHeaderPrefix的端点相同；先检查consumed>available返回ErrUnpack，再检查produced>maxBytes−size返回ErrLimit。只有两项通过才增加position，故增量不超过len(src)，零段run乘8前的预算保护仍在；size、错误优先级和完整预检后分配的精确长度不变。原始src未在预检中修改，恢复仍从相同开头读取。

实际ARM64普通预检不再每word重算尾切片len/cap/NEG/指针，但整个UnpackWithLimit384→396条，栈帧112B不变，index panic调用点5→8、slice仍8。静态指令不作为采用/拒绝门槛，见[assembly-summary](assembly-summary.json)。

## 验证与固定矩阵

复用已提交全mask独立预期、输入偏移/后继、坏输入/预算/ownership及整数数组/RPC用例；不新增镜像生产测试。新增隔离差分探针枚举全部mask截断前缀、普通/FF后继、0..4096零mask、最大FF段的所有前缀及20档预算，共138,500组：比较两包各自稳定错误class、结果bytes/len/cap/nil、输入不变与结果独占。它不依赖错误message文本；失败原子性与错误优先级在计时前验证通过。

固定70项与前轮相同：原52入口加全部18现有Unpack形态；self/clone和两轮compare共280进程、26,880块、6,720个ABBA/BAAB四元组、1,680轮、1,120快照。所有测试/构建/vet/机器码检查先于第一块计时，冻结plan/probe/patch/源码/工具/二进制；不重试择优或事后改矩阵。GOMAXPROCS1、GOGC100、无限GOMEMLIMIT、清三个sink/GC/50ms预热、原校准/10ms块及每侧3×128分配观察保持原样。

## 收益与代价

以下六轮配对时间比中位数负值更快；[完整70项表](ALL-CASES.md)并列累计差值及控制，[analysis](analysis.json)独立逐块复算：

| 入口 | 第1轮变化 | 第2轮变化 |
| --- | ---: | ---: |
| AddressBook PDecode | -2.92% | -2.11% |
| 小型游戏 PDecode | -4.07% | -1.85% |
| 大型游戏 Unpack | -12.20% | -12.89% |
| 大型游戏 PDecode | -3.20% | -1.85% |
| 大型游戏 Dispatch | -2.96% | -3.17% |
| 4字节×896 PDecode | -11.85% | -9.45% |
| 4字节×896 Dispatch | -9.26% | -11.32% |
| 8字节×896 PDecode | -8.72% | -8.09% |
| 8字节×896 Dispatch | -11.38% | -10.88% |
| 4字节×3 PDecode | +0.59% | +0.44% |
| 8字节×3 PDecode | +2.06% | +2.84% |
| 单字节word×256 | +33.30% | +29.95% |
| 双字节word×256 | +20.59% | +19.86% |
| 三字节word×256 | +10.42% | +9.62% |
| 四字节word×256 | +14.09% | +12.41% |
| 五字节word×256 | +18.52% | +19.20% |
| 六字节word×256 | +17.11% | +15.82% |
| 七字节word×256 | +4.26% | +5.28% |
| FF512接稀疏4096 | +10.39% | +6.42% |
| 短零段8word×64 | +2.49% | +0.65% |

4字节数组PDecode约快838/650ns，8字节约快841/798ns，大型PDecode约快646/368ns；这些业务收益没有被删除，也不按“低于某比例”判为无价值。两种大数组PDecode的12轮均更快。8字节真实Dispatch两类控制均通过、两轮快11.38%/10.88%，是重要的正面证据。

未采用的原因是通用预检需要面对尚未限定的合法byte pattern：1..7有效字节word从约4%到33%的重复代价，绝大部分跨两轮/各轮同方向，FF接稀疏仍+10.39%/+6.42%，短零段亦+2.49%/+0.65%；本轮未解决原选题的代价，也未解释为何真实组合与这些基础形态方向相反。短8字节PDecode还有+2.06%/+2.84%约6/8ns成本。当前不在主库切换这一通用扫描，候选补丁和正面结果均保留，便于后续分解或重新裁决；没有再添加密度/具体mask/fixture分支。

self39/70、clone33/70。普通形态clone约−2%～−8%漂移，FF接稀疏clone+27.33%，失败标记均保留；不扣除它们来得出“校正后”收益或成本。业务目标亦有失败控制，因此本轮数值是当前机器/二进制的工程观察，不是唯一因果或跨平台承诺。

对象数中位数全部相同，27项字节窗口尾差、21个入口中位/累计反向，均保存在[allocation-tails](allocation-tails.json)和[cumulative-reversals](cumulative-reversals.json)。例如小型PDecode第二轮中位−1.85%而累计+782.6ns，nested PDecode第二轮中位−2.92%而累计−190294ns；不以离群累计美化典型收益。最大堆快照14,180,264B（约13.52MiB），不是峰值，limiter全零；最长块82.0/306.2/221.3/213.5ms和全部GC块/最差轮原样保留。

## 收尾与复现

313条测量命令均成功，包括两侧已有目标契约、harness/vet、70入口语义检查、109形态既有差分、138,500组新预检差分、构建/汇编与280项独立采样。fresh最终输入、基线全部18源、候选补丁、工具/夹具/生成输入、二进制、raw指纹、顺序和原采样函数逐字节核对。首次计时前完善过差分结果独占检查，最终fresh与实测输入一致，早期准备副本未用于样本或混入核对。

主库未应用补丁，保持af9169d且干净；因此未重复生产完整库/oracle/race/checkptr/fuzz、GSR三包或Go/TS验收，也未重测PB/TS排名。accept.py仅为若未来采用时的验收配方，本轮未执行，不作成功声明。上一轮af9169d的完整验收仍见其报告。正式go.mod/go.sum保持HEAD，require仍v0.4.0；没有推送、发布或打tag。

后续先将相同packed字节的纯Unpack与真实组合入口分开诊断，并核对shape的固定/旋转mask分布和编译执行路径；未建立原因前不把这次通用预检替换推入生产。Decode继续开放，不新增阶段冻结。本轮诊断与实验报告未提交。

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-unpack-preflight-cursor/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-unpack-preflight-cursor-new
```

输出目录须不存在，--prepare-only仅建输入。聚合raw、独立复算、来源/汇编/验证文本随报告保存；逐项进程JSON、二进制和源码副本仍在忽略tmp目录，通过SHA连接。
