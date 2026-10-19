# Pack分支内固定len/cap视图

日期：2026-10-19。上一轮已本地提交：gosproto `0b38019`、GSR `31b3f99`，均未推送。本轮基线从该提交增量，不回到更早的prefix实现；正式依赖仍v0.4.0，Decode冻结。

## 先检验具体假设

三份草稿均只在归档执行既有Pack/EncodePacked契约测试和17,984组旧新差分，全部通过。前两份没有进行CPU采样或基准计时，不能根据静态指令数宣称实测快慢；补丁、命令与反汇编保留在[preflight](preflight/summary.json)。

| 未计时预检 | 符号指令 | 观察与决定 |
|---|---:|---|
| 基线 | 288 | 已提交的固定payload视图 |
| 分支内两索引固定长度 | 304 | cap仍动态，NEG/AND指针掩码未去掉，且增加检查；未达到定位工作减少的目标 |
| 前缀最高字节计入头字节 | 288 | 实际仍是MUL+ADD，没有预期MADD融合；计数草稿不测量、不应用 |
| 三索引固定len/cap | 296 | 目标分支去掉动态cap的SUB/NEG/AND定位序列，总检查/错误布局变大，选为唯一实测候选 |

本地Go1.24.13 ARM64规则存在乘加合并且要求MUL单一使用，实际第二份草稿没有MADD；没有读取SSA证明具体未融合原因，不凭规则猜测归因。三份栈帧均112B。

## 实测候选与边界

[候选](candidate.patch)不再共用`packed[offset:]`，而是在稀疏分支使用`packed[offset:offset+3:offset+3]`，普通分支使用`packed[offset:offset+9:offset+9]`。已有dst[2]/dst[8]检查保留在源码中，由编译器证明冗余；payload、前缀计数与最终offset计数均沿用已提交实现。没有新增形态分支、池、表、缓存、unsafe或API。

所有生产调用者用packedBufferSize创建完整输出，`len(packed)==cap(packed)`由构造保证。原稀疏/普通分支已分别要求当前位置可用3/9字节，因此新视图覆盖完全相同的位置；临时视图限制cap不进入返回值，公共结果仍从原packed切出，cap不变。Go三索引检查高端cap与低端/溢出，没手删检查；不把内部任意短len/大cap缓冲当新的公开调用契约。

稀疏、普通写出的字节、地址和顺序相同，FF仍使用原packed和扫描规则，尾块路径不变；输出增长额度、八字节scratch、工作区重叠隔离、独占、预算与错误顺序不变。静态指令总数增加并不是自动否决条件：被定位的动态计算消失了，但其他检查和布局有代价，需要真实API测量。

## 固定测量

[计划](plan.json)冻结后，原70项内容/顺序不变，self/clone/两轮compare，共280个逐项进程。Go1.24.13 darwin/arm64、GOMAXPROCS1、默认GC100和无内存软上限；每项仍构造所有夹具、初始化schema、精确选择一项并清空输出引用，在预热前GC。50ms交错预热，6轮×4 quartet，10ms目标块，ABBA/BAAB交错，分配每侧3×128次，4个快照只在计时/分配区间外读取。

所有构建、正确性检查、编译与反汇编在计时前完成。两份草稿没有混入比较组，采样后不改候选/计划/探针或重跑，不删除GC块/失败控制/分配尾差，不减去控制中位数，不设最低收益比例。旧混合环境和此前独立进程数据不池化，不生成PB/TS新排名。

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-pack-window/reproduce.py \
  --library /path/to/gosproto \
  --output tmp/gosproto-pack-window/new-run
python3 -B docs/benchmarks/2026-10-19-gosproto-pack-window/analyze.py \
  tmp/gosproto-pack-window/new-run
```

`--prepare-only`只重建归档。逐项JSON和二进制留忽略tmp目录，Git保存无删减同序合并原值、逐项SHA、输入/源/工具/二进制指纹及完整命令；反汇编只规范行尾空白，原始SHA另存。

## 结果与保留决定

**保留唯一实测的三索引版本**，将与归档完全相同的pack.go应用到库工作树，新代码和报告未提交。虽然符号总数多8条，普通分支中的动态定位序列已经消失，整数数组组合/真实RPC在两轮配对和多个累计口径中改善；没有按静态指令数或最低收益比例自动否决。前两份未计时草稿均不应用。

26,880块、6,720 quartet、1,680轮、1,120快照独立复算，与工具值在1e−12容差内一致。下表负数表示候选更快，纳秒为每轮几何成本差中位数，不是用比例乘汇总基线。

| 入口 | pass1差值 | pass2差值 | pass1 / pass2 配对纳秒差 | self / clone控制 |
|---|---:|---:|---:|---|
| `body/game_large/sproto_pack` | -7.62% | -1.38% | -164.8ns / -35.4ns | 通过 / 通过 |
| `body/game_large/sproto_pencode` | -2.81% | -0.15% | -205.2ns / -18.5ns | 通过 / 通过 |
| `rpc/game_large/request_cancel` | -1.29% | -5.54% | -93.1ns / -472.2ns | 通过 / 通过 |
| `rpc/game_large/response` | -1.86% | -1.36% | -134.8ns / -119.1ns | 通过 / 通过 |
| `body/int32_edges/sproto_pack` | -6.18% | -5.39% | -133.2ns / -112.5ns | 未通过 / 未通过 |
| `body/int32_edges/sproto_pencode` | -6.07% | -3.73% | -202.5ns / -122.3ns | 通过 / 通过 |
| `rpc/int32_edges/notification` | -5.68% | -5.17% | -191.6ns / -189.6ns | 通过 / 通过 |
| `rpc/int32_edges/request_cancel` | -3.92% | -5.08% | -133.3ns / -179.9ns | 通过 / 通过 |
| `rpc/int32_edges/response` | -7.79% | -6.65% | -264.2ns / -232.3ns | 通过 / 通过 |
| `rpc/zeros4k/notification` | -7.28% | -8.30% | -139.2ns / -168.1ns | 未通过 / 通过 |
| `rpc/echo_ascii4k/request_cancel` | +0.59% | +1.74% | +5.1ns / +16.5ns | 通过 / 通过 |
| `growth/flat/full/4096/request_cancel` | -1.93% | -0.87% | -4419.3ns / -1997.8ns | 未通过 / 通过 |
| `growth/flat/nil/4096/pencode` | -0.47% | -0.60% | -361.5ns / -568.1ns | 未通过 / 通过 |
| `growth/array_record/4096/request_cancel` | +0.71% | +0.43% | +2284.1ns / +1291.7ns | 通过 / 未通过 |

例如nil4096 PEncode只快0.47%/0.60%，配对少361.5/568.1ns（self失败、clone通过），也完整保留为带不确定性的观察。反向成本同样披露：4KiB ASCII真实请求多5.1/16.5ns、累计多91.3/146.0ns，两组控制通过；4096条含整数数组记录的请求多2,284.1/1,291.7ns、累计多243.2/16,442.9ns，clone失败。AddressBook PEncode两轮多3.6/8.9ns、累计也多，UTF8手工帧配对多4.9/21.0ns。这些是本次保留时接受的观测代价，不宣称全面提速。

### 累计值与离群

整数数组PEncode累计少201.0/295.3ns，真实通知少170.7/184.6ns，真实请求少129.9/355.3ns，与配对方向一致；大型纯Pack累计少331.5/127.3ns。以下翻转全部保留：

- 整数数组响应pass2配对少232.3ns，累计多168.4ns；最大B块44.994ms、3,177调用，完成5次GC。
- wide64响应pass2配对少237.3ns，累计多1,234.0ns；最大B块206.939ms、1,559调用，完成5次GC。
- AddressBook纯Pack pass2配对少6.2ns，累计多94.1ns；最大B块227.795ms、81,793调用，完成5次GC。
- 大型PEncode pass2配对少18.5ns（−0.15%），累计多204.4ns；大型请求pass1、notification/response pass2等也有累计方向不同。完整数据见[逐项分析](analysis.json)。

未改纯Encode也有漂移，例如小型/大型纯Encode两轮约−1%左右；不把这些变化归功于Pack，也不从Pack比例中扣除“校正”。没有读取调度跟踪或CPU profile确认离群的唯一原因，GC完成数不能直接解释全部墙钟成本，不删除/修正异常块。

### 控制与分配

self46/70、clone51/70控制通过，不能拿上一轮56/70替代；失败和轮值全保留。最大单轮self偏差为half4096 PEncode +49.30%，clone为AddressBook Pack −57.47%；compare pass1 Echo响应+63.74%、pass2 AddressBook Pack +76.44%。280个进程四阶段limiter均为零，仍不能消除其他噪声或保证长尾；最大堆快照约27.88MB（clone），不是峰值。

分配中位数self70/70、clone69/70、pass1和pass2各68/70相同。差异均为128次观测的16B尾差（14,616与14,616.125B/次，mallocs均5）：clone的zero手工帧B高；pass1 ASCII4KiB和zero手工帧B高；pass2这两项A高。原始非整数值保留，不据此断言候选新增缓冲或把全部观测说成完全相同。公共cap、独占与工作区测试通过。

## 验收与范围

库只改pack.go（3行增加/1行删除），源码等于最终采样归档；其他Go源码仍与0b38019相同。既有全mask/高位/FF/尾块/零段、EncodePacked重叠/wire/cap/独占/错误/分配用例与17,984差分通过，未新增重复测试。库全测、官方C/Lua、race+checkptr=2、vet、有限fuzz、GSR三个相关包、Go/TS集成7/7零跳过全部通过，见[验收命令](acceptance.json)。

有限fuzz：Pack5,487次，3s后停止增长，约11s正常退出；Codec416,938次，正常通过。没有为增加计数延长或重跑，覆盖有限。官方oracle复用上一轮忽略目录：core.so与先前e93fd5a历史SHA一致，Lua facade同SHA；parser来源只读Skynet单独记录，不冒称已核对其上游revision，详见[来源](oracle-provenance.json)。正常与race/checkptr均启用oracle；参考目录未修改。

fresh prepare-only四组归档、逐项/合并原值和所有源/fixture/生成输入/driver/probe/二进制指纹复核一致。GSR验收用临时modfile绑定本地库，并核对服务器二进制module信息，正式go.mod/go.sum不变。前两份草稿的指纹及未计时证据一起保留，不能将它们的预检当作性能结论。

新库代码和GSR报告保持未提交，未推送/发布/升级依赖。Decode、FF规则、容量预算和公开接口不变。下一轮宜先刷新当前版本的真实组合路径profile，再决定是否继续逐指令实验；本轮不再叠加计数或输入游标改动。
