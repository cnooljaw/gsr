# 整数记录 inline 分支延后内容视图

前轮已本地提交：gosproto `9067933`、GSR `ca02105`，均未推送。本轮只检查私有 `encodeIntegerRecordInto` 是否为 inline 整数提前构造不用的内容切片。

## 候选与编译证据

原调用 `encodeIntegerInto(bits, output[bodyOffset:])` 必须先求切片参数，再判断整数是否 inline。候选显式区分同一范围：0～32766 直接使用原头部公式 `(bits+1)*2`、内容长度为零；其余 uint64 位模式（包括有符号负数）仍调用原 `encodeIntegerInto` 决定4/8字节并写出。内容切片只在后者构造。

字段/tag/presence 顺序、实际值重读、头部/内容 store 次序、预检/布局/预算、Pack/Decode、wire/cap/独占、API 均不改。没有扩大 unsafe、缓存字段值或引入池。完整预检保证有效输出区间；inline 不会写内容，因此延后该切片不改变有效调用的结果。

唯一候选的未计时归档显示：212→216条符号指令、32B帧不变、panic调用点14→15。inline前的切片运算被推迟、重复inline判断被编译器折叠，但非inline长度写出又构造一次切片。保留这个静态代价进入实测，不按总指令数自动否决，也不提前宣称加速。未叠加内容视图复用或其他候选，原始/整理汇编及BCE见preflight。

## 新增覆盖与验证方法

先用rg核对既有用例。根/普通嵌套指针已有0、32766、32767与整数极值；原同构记录数组用例的值主要为-1、0、MaxInt64。扩展既有 `TestIntegerRecordArrayBudgetsAndTags`，增加10次当前值变换：0、32766、32767、-1、MinInt32、MinInt32−1、MaxInt32、MaxInt32+1、MinInt64、MaxInt64，继续与命名整数反射路径比较完整wire/cap，并保留后继字段及稀疏tag。

新增部分先在未优化的基线通过，见[基线记录](preflight/baseline-test.json)。独立负向归档故意将inline上限放宽一位：新测试在32767失败、旧测试仍通过，见[负向命令](preflight/negative-commands.json)。它补的是专用记录数组的inline/宽度边界，未另起重复夹具或新测试函数。准备时有一次报告输出相对路径错误，测试源码已写入、基线测试仍正常完成；改为绝对报告路径，未重复插入测试或采计时，见[准备记录](preflight/preparation.json)。

基线与候选既有记录/tag/nil/当前值/元信息/预算/RPC契约、109形态525项body/packed/RPC/预算wire-cap-独占差分及harness/vet在正式计时前完成。原70项×self/clone/两轮比较，共280个独立进程；每进程构造原矩阵、只保留所选操作，GC后预热/有界校准，GOMAXPROCS=1、GOGC=100、无限GOMEMLIMIT。6轮×4个ABBA/BAAB quartet×4块，计时区间外读状态；每臂三次128调用分配观测。sampleBlock/blockCalls不变。

采样前冻结[计划](plan.json)、[候选与测试补丁](candidate.patch)、[驱动](reproduce.py)。全部构建/测试/vet/汇编在首项计时前完成；无计时重试、异常剔除或控制偏差扣除，不设最低收益比例。fresh归档与独立复算在固定矩阵完成后执行。

## 结果与取舍

保留这个单一候选，并接受输入形态带来的局部代价。大型游戏消息Encode两轮快2.57%/2.34%、PEncode快1.72%/3.26%、真实请求快3.18%/2.44%、响应快2.29%/2.44%，上述入口两组控制都通过，分配不变。小型游戏Encode/PEncode/请求也两轮同向改善。逻辑只增加一处分支并复用原非inline写出，不新增运行时模型、分配或持久状态；这些收益不因低于旧百分比门槛而丢弃。

代价同样具有重复性：普通full记录的256～4096形态大多慢0.5%～1.5%，部分PEncode达到2.32%；half记录请求两轮慢1.07%/0.82%。布局夹具每个非nil记录的三个值为index、−index−1、1<<33，在这些长度下只有第一个是inline，后两个走非inline。新增切片工作与这个取舍相符，但没有用分支计数或新的CPU profile排除全部其他因素，不能说已精确解释每个纳秒。

决定优先保留真实游戏消息路径的改善，同时明确接受下表成本。不是宣称所有记录编码都更快，也不是只报道赢家。负号更快，括号为轮级几何耗时差中位数；它不能与累计耗时混算。所有70项均在raw/summary和[独立分析](analysis.json)中保留。

| 入口 | 第一轮：比例（差值） | 第二轮：比例（差值） | self / clone控制 |
| --- | ---: | ---: | --- |
| `body/game_small/sproto_encode` | -2.18%（-15.1ns） | -2.21%（-15.1ns） | 通过 / 通过 |
| `body/game_small/sproto_pencode` | -1.57%（-13.5ns） | -2.02%（-17.2ns） | 通过 / 通过 |
| `rpc/game_small/request_cancel` | -0.71%（-6.6ns） | -1.99%（-18.3ns） | 通过 / 通过 |
| `body/game_large/sproto_encode` | -2.57%（-144.6ns） | -2.34%（-128.8ns） | 通过 / 通过 |
| `body/game_large/sproto_pencode` | -1.72%（-122.4ns） | -3.26%（-233.5ns） | 通过 / 通过 |
| `rpc/game_large/notification` | -2.07%（-147.5ns） | -0.91%（-61.8ns） | 通过 / 通过 |
| `rpc/game_large/request_cancel` | -3.18%（-224.3ns） | -2.44%（-174.5ns） | 通过 / 通过 |
| `rpc/game_large/response` | -2.29%（-162.1ns） | -2.44%（-173.8ns） | 通过 / 通过 |
| `body/addressbook/sproto_encode` | -0.53%（-2.9ns） | -0.47%（-2.6ns） | 通过 / 通过 |
| `rpc/addressbook/request_cancel` | -1.21%（-8.5ns） | -0.22%（-1.5ns） | 通过 / 通过 |
| `rpc/ascii32/notification` | -3.13%（-4.8ns） | -1.43%（-2.2ns） | 未通过 / 通过 |
| `body/echo_ascii4k/sproto_pencode` | -0.10%（-0.7ns） | -0.09%（-0.7ns） | 通过 / 通过 |
| `body/int32_edges/sproto_pencode` | +0.67%（+21.0ns） | +2.60%（+78.9ns） | 通过 / 通过 |
| `rpc/int32_edges/request_cancel` | +0.31%（+10.2ns） | +0.36%（+11.8ns） | 通过 / 通过 |
| `body/wide64/sproto_pencode` | +0.04%（+2.2ns） | +0.48%（+29.7ns） | 通过 / 通过 |
| `growth/flat/full/256/encode` | +0.68%（+68.5ns） | +0.50%（+51.3ns） | 通过 / 通过 |
| `growth/flat/full/256/pencode` | +2.32%（+304.4ns） | +1.44%（+190.3ns） | 通过 / 通过 |
| `growth/flat/full/256/request_cancel` | +1.68%（+225.5ns） | +0.50%（+68.7ns） | 通过 / 通过 |
| `growth/flat/full/512/request_cancel` | +0.98%（+264.9ns） | +1.24%（+336.3ns） | 通过 / 通过 |
| `growth/flat/full/1024/request_cancel` | +1.05%（+573.8ns） | +0.51%（+295.9ns） | 未通过 / 通过 |
| `growth/flat/full/4096/encode` | +1.28%（+2143.8ns） | +0.67%（+1116.1ns） | 通过 / 通过 |
| `growth/flat/full/4096/pencode` | +1.15%（+2487.6ns） | +0.49%（+1075.6ns） | 通过 / 通过 |
| `growth/flat/full/4096/request_cancel` | +0.54%（+1192.0ns） | +0.54%（+1205.3ns） | 通过 / 通过 |
| `growth/flat/half/4096/request_cancel` | +1.07%（+1551.7ns） | +0.82%（+1213.8ns） | 通过 / 通过 |
| `growth/nested/32x128/request_cancel` | +0.23%（+515.0ns） | +1.18%（+2679.7ns） | 通过 / 通过 |
| `growth/array_record/4096/pencode` | -0.20%（-619.9ns） | -0.15%（-492.1ns） | 通过 / 通过 |

例如32B通知只少4.8/2.2ns（self未通过、clone通过），带不确定性保留；AddressBook真实请求少8.5/1.5ns、4096条含数组记录PEncode只快0.20%/0.15%，也完整记入观察，不用最低比例淘汰。

### 累计方向、未改路径与噪声

至少一轮配对/累计方向相反共28项，完整见[方向翻转](cumulative-reversals.json)。大型PEncode第一轮配对少122.4ns但累计多33.5ns；大型通知第二轮配对少61.8ns但累计多234.9ns。4096条full请求第一轮配对多1,192.0ns，累计却少8,875.2ns；4096条full PEncode第二轮配对多1,075.6ns，累计少8,149.5ns。half PEncode两轮配对慢0.03%/0.53%，累计分别少1,691.5/2,984.8ns。这些不按有利方向挑选解释。

大型真实请求累计两轮少2,508.4/169.8ns，但第一轮A臂存在119.294ms长块（1,443调用/2次GC），单轮比值−40.56%；不能只拿累计值强调收益。第二轮配对和累计仍同向，原值/长块都保留。

未进入本helper的整数数组PEncode两轮多21.0/78.9ns，真实请求多10.2/11.8ns；wide64 PEncode多2.2/29.7ns，观察成本不能隐藏或全归于inline分支。未改Pack在四个二进制均296条且符号指令序列一致，整数数组纯Pack却+1.86%/−0.65%方向不同。encodeFieldInto源码也未改，四个二进制均864条；初次整函数指令相同断言失败，核对现有输出发现反射Float类型错误构造路径的一条ADD立即数从3720变为3752，因此不宣称整函数每条指令完全一致。差异与断言记录见[指令核对](unchanged-functions.json)，没有为此重新采样，也不据此扣除噪声或确认漂移的唯一成因。

### 控制与分配

self60/70、clone64/70通过，全部失败与轮值保留；分配中位数68/69/70/69项相同。差异均为128调用观测中的16B尾差、mallocs仍5：self ASCII手工帧A高、zero手工帧B高；clone ASCII帧A高；pass2 ASCII帧B高（14,616与14,616.125B/次）。比较第一轮70/70相同，所有目标消息分配中位数不变，不说每项观测都完全一样。

1,120个状态快照limiter全部为零，最大堆对象快照4,486,400B（约4.28MiB），不是峰值，也不能排除GC或运行时成本。最大单轮self为zero手工帧+33.04%、clone为int32纯Encode+41.23%、pass1为大型请求−40.56%、pass2为nil4096 PEncode+33.54%。最长块分别为self half4096请求A臂105.292ms（68调用/2次GC）、clone ASCII手工帧A臂105.431ms（5,541调用/27次GC）、pass1大型请求A臂119.294ms、pass2 nil4096请求A臂140.721ms（130调用/3次GC），见[最长块](long-blocks.json)。没有重跑、删除或用GC完成数修正。

## 验收、范围与下一步

26,880块、6,720 quartet、1,680轮、1,120快照独立复算到1e-12，280份逐项原值与合并数据/顺序/次数一致；冻结driver/probe/plan/源码/生成输入/fixture/二进制、sampleBlock/blockCalls未变、fresh归档均核对。计时矩阵没有失败命令或重试，准备阶段的报告路径错误和采样后的过强指令相同断言已分别保留。

库仅 `encode_integer_record.go` 实现和 `limits_test.go` 既有用例增强，18个Go文件与冻结候选逐字一致，gofmt通过。库全测/官方C-Lua、race+checkptr=2、vet、10秒Codec fuzz470,086次（没有新增interesting）、GSR三相关包及Go/TS7/7零跳过通过，见[验收命令](acceptance.json)。未改Pack算法，不重复Pack随机fuzz；没有新PB/TS排名。官方core/facade沿用历史SHA，parser来自只读Skynet并独立归属，见[来源](oracle-provenance.json)。临时modfile和服务器module-info核对本地库接线，正式go.mod/go.sum不变。

新实现/测试和GSR报告均未提交；未推送、发布或升级依赖，Decode继续冻结。下一轮只检查非inline分支能否复用同一内容视图来避免再次定位，保持inline路径与store次序；先核对编译结果，不同时改预算、预检、布局或Pack，不把本轮成本当作已解决。

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-record-inline/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/record-inline-repro
python3 -B docs/benchmarks/2026-10-19-gosproto-record-inline/analyze.py tmp/record-inline-repro
python3 -B docs/benchmarks/2026-10-19-gosproto-record-inline/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/record-inline-fresh --prepare-only
```

可审查的合并原值/摘要/命令/汇编与指纹入报告目录；280份逐项JSON、二进制和补充objdump原文在忽略目录 `tmp/gosproto-record-inline/`，各模式inputs保留逐项SHA。初始静态检查只含实现补丁，随后加入边界用例再冻结正式计划；前者原文为preflight/implementation.patch，其SHA与初始归档一致，生产实现与正式候选一致。汇编只移除行尾空白，原始与整理后SHA分开；核对见[validation.json](validation.json)。
