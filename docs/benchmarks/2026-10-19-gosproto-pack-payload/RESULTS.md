# Pack固定八字节payload视图

日期：2026-10-19。库基线`7397804`，GSR基线`d1dceaa`。上一轮连续段实验已本地提交为GSR `d1dceaa`，未推送。正式依赖仍v0.4.0，Decode冻结。

## 候选与预检

上一轮增加形态分支的批量写出未保留。本轮[候选](candidate.patch)只调整已有普通word写出的位置表示：保留 `_ = dst[8]`，建立 `payload := dst[1:9]`，将 `dst[1+prefix]` 改为 `payload[prefix]`；mask继续写到原`dst[0]`，八次byte store、前缀乘法、最终计数和写入顺序均不变。稀疏、FF、尾块和Decode路径不改，无新表、池、缓存或unsafe。

现有检查证明`len(dst)>=9`，固定payload视图长8；每个位置仍取`&7`，范围0～7，严格别名到旧的`dst[1+i]`。包括临时覆盖零字节的scratch写入在内，每次写的地址和值完全相同，不扩展允许的输入或输出容量，也不改变组合工作区重叠约束、wire、ownership、预算或错误顺序。

反汇编预检：Pack符号指令296→288、帧112B不变，密集store没有新边界检查。七个索引的单独加1合并成一个payload基址加1；静态符号数量受布局等因素影响，不能理解为每次调用固定节省8条执行指令。基线的最终计数数据流保持不动，避免把两个不同优化混在一起。

计时前，现有全mask/高位字节/FF分段/短尾/零段、EncodePacked工作区/wire/cap/独占/分配/错误测试和17,984组旧新差分通过；没有给同一外部契约增加镜像测试。

## 固定采样

[计划](plan.json)沿用原70项内容与顺序，self/clone/两轮比较四组共280个逐项独立进程。每项完整构造夹具并初始化schema cache，精确选择一项后清空输出引用，在预热前GC。仍是Go1.24.13 darwin/arm64、GOMAXPROCS1、默认GOGC100和无限内存软上限，交错预热50ms，6轮×4quartet、目标块10ms、ABBA/BAAB；分配为每侧3×128次。四个指标快照在计时/分配区间外读取，离散堆值不是峰值。

所有构建、harness检查、编译与反汇编在第一项计时前完成；采样后不改代码/探针/计划或重跑。控制标记只描述本轮的不确定性，不减去控制中位数、不删除离群，不设最低收益比例。旧混合环境、上一轮独立进程和本轮数据不池化，不生成新PB/TS排名。

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-pack-payload/reproduce.py \
  --library /path/to/gosproto \
  --output tmp/gosproto-pack-payload/new-run
python3 -B docs/benchmarks/2026-10-19-gosproto-pack-payload/analyze.py \
  tmp/gosproto-pack-payload/new-run
```

驱动引用已提交的fixture/telemetry，`--prepare-only`只重建输入、不计时。逐项JSON/二进制留在忽略tmp目录，Git保存无删减同序合并原值、各项SHA、命令/指纹和独立复算。编译诊断同时保留stdout/stderr，不依赖空stdout文件。

## 结果与保留决定

**保留候选，应用了与归档完全相同的pack.go，暂未提交。** 普通路径少了重复索引加法，代码体积下降、栈与容量不增，目标Pack、PEncode与主要真实RPC在两轮配对中同向改善。没有设最低收益比例，仍把小收益和成本一起考虑。

26,880块、6,720个quartet、1,680轮、1,120个指标快照已独立复算，与工具值在1e−12容差内一致。下表负数表示候选更快；纳秒是每轮几何成本差的中位数，不是将汇总比例乘一个基线推得。

| 入口 | pass1差值 | pass2差值 | pass1 / pass2 配对纳秒差 | self / clone控制 |
|---|---:|---:|---:|---|
| `body/game_small/sproto_pack` | -6.94% | -6.57% | -18.0ns / -17.0ns | 通过 / 通过 |
| `body/game_small/sproto_pencode` | -3.00% | -3.59% | -26.2ns / -31.7ns | 通过 / 未通过 |
| `body/game_large/sproto_pack` | -8.87% | -9.13% | -209.1ns / -216.3ns | 通过 / 通过 |
| `body/game_large/sproto_pencode` | -2.70% | -2.05% | -203.7ns / -153.2ns | 通过 / 通过 |
| `rpc/game_large/request_cancel` | -2.07% | -2.78% | -154.1ns / -206.5ns | 通过 / 未通过 |
| `rpc/game_large/response` | -3.08% | -1.60% | -229.7ns / -117.9ns | 通过 / 通过 |
| `body/int32_edges/sproto_pack` | -7.05% | -4.80% | -157.2ns / -106.1ns | 通过 / 通过 |
| `body/int32_edges/sproto_pencode` | -4.70% | -4.19% | -164.1ns / -144.8ns | 通过 / 通过 |
| `rpc/int32_edges/response` | -3.77% | -3.97% | -131.2ns / -136.7ns | 通过 / 通过 |
| `growth/flat/full/512/request_cancel` | -1.60% | -1.46% | -458.1ns / -413.6ns | 通过 / 通过 |
| `growth/array_record/4096/request_cancel` | -1.13% | -0.83% | -3681.8ns / -2704.8ns | 未通过 / 通过 |
| `body/wide64/sproto_pencode` | +0.63% | +0.06% | +39.0ns / +4.0ns | 通过 / 未通过 |
| `rpc/wide64/response` | +0.02% | +0.08% | +1.4ns / +5.3ns | 通过 / 通过 |

例如4096条含数组记录的真实请求只改善0.83%～1.13%，但两轮配对少2,704.8～3,681.8ns、累计少2,565.1～3,653.1ns，也保留为有效观察，不因低于2%而舍弃。wide64 PEncode两轮多39.0/4.0ns，累计多63.3/7.4ns，clone控制失败；这些非目标成本不隐藏。

### 累计口径与异常块

大型纯Pack累计少211.0/221.7ns、整数数组PEncode少163.4/152.4ns，大型请求少344.0/570.0ns，和配对方向一致。但以下项目有方向差异，不能只公布配对比例：

- 大型PEncode pass2配对少153.2ns，累计多490.0ns；最大B块31.679ms，1,372调用，期间完成2次GC。
- 小型Pack pass2配对少17.0ns，累计多18.1ns；最大B块84.716ms，46,449调用，期间完成6次GC。
- 4096条full记录请求pass2配对少5,174.5ns，累计多124,317.1ns。第2轮第3 quartet的B块耗时233.678ms，仅43调用，期间完成2次GC；本项累计A/B为265,881.7/390,198.8ns。该块及其他块全部保留，不能认定唯一原因或忽略长尾风险。
- 未改纯Encode也有漂移：1024条记录纯Encode两轮配对多287.3/438.3ns，累计却少149.3/161.2ns；没有把此类变化归功或归咎于未改函数。

完整70项的配对轮值、累计ns、GC数、分配观测与全部控制见[独立分析](analysis.json)及四组合并原始JSON。不减去控制中位数，不用上一轮控制替代本轮控制，不删离群或重跑。

### 控制与分配

self/clone各56/70控制通过；最大单轮离群分别为嵌套纯Encode +69.62%和小型Pack +39.63%。失败只表示相应测量仍有不确定性，不单独否决已有小收益。280个进程四阶段limiter均为零；最大堆快照约19.47MB（pass2），不是峰值，也不能用limiter解释所有波动。

分配中位数self69/70、clone69/70、pass1 69/70、pass2 70/70相同。尾差仅是128次观测中的16B：self zero手工帧A14,616.125/B14,616B每次，clone该项相反；pass1 ASCII4KiB手工帧A14,616.125/B14,616B每次，均为5次分配。保留非整数尾差，不宣称测量中的每一项分配完全相同；候选没有增加缓冲或中间对象。

## 应用与验收

只把已采样的pack.go应用到独立gosproto工作树，其他Go源码仍与7397804相同；新库代码和GSR文档均未提交，未推送/发布，不升级正式依赖。原Decode、FF、稀疏与尾块代码不变。库全测、race + checkptr=2、vet、两项有限fuzz、GSR三个相关包和Go/TS接线全部通过，集成7/7、零跳过，见[验收命令](acceptance.json)。使用临时modfile替换到本地库，并读取服务器二进制module信息确认绑定，未修改正式go.mod/go.sum。

有限fuzz：Pack36,026次，3s后计数停止增长、到11s正常退出；不延长重跑，不把有限覆盖当完备证明。Codec442,757次，新增1个interesting输入，正常通过。现有黄金向量与差分测试继续保护全mask/高位/FF/短尾/工作区重叠/独占/cap/错误契约，未新增同义用例。

官方C/Lua对照复用先前e93fd5a记录的core.so，SHA完全相同；sproto.lua从只读Skynet取得且与该历史官方SHA一致，sprotoparser.lua取只读Skynet并单独记录SHA，不声称已核对其上游revision。C模块未重编译，没有拿不同C源码冒充同一版本。原oracle临时目录的Lua源码已缺失，本轮在GSR忽略tmp目录组装运行环境，未改参考目录，见[oracle来源](oracle-provenance.json)。正常与race/checkptr验收均设置oracle目录，相关测试未跳过。

fresh prepare-only重建四组输入与固定指纹一致，逐项/合并记录、源码/生成输入/fixture/driver/probe/二进制SHA复核通过。保存的反汇编仅去掉行尾空白，原始文本留在tmp并记录双份SHA；未修改任何计时输入或样本。没有新PB/TS排名，也没有将旧混合环境数据回填或合并。下一轮先分析剩余切片定位与最终计数的数据流；本次不同时叠加第二个优化。
