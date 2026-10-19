# 非 inline 整数内容视图复用

前轮已本地提交：gosproto `2ea804e`、GSR `b004da7`，均未推送。本轮只检查私有 `encodeIntegerRecordInto` 的非inline分支能否复用内容视图，减少长度写出的第二次定位。

## 候选与静态取舍

原先 `encodeIntegerInto(bits, output[bodyOffset:])` 写出整数后，写长度时又求一次 `output[bodyOffset:]`。候选增加调用内 `body []byte`，仅在非inline分支赋值，供原整数写出和后续长度写出共用。

两次原切片之间没有改变output描述或bodyOffset；头部写出位于预检确定的独立头部区间。因此新旧store的地址、字节和顺序相同。局部借用不逃出本次同步调用，结果cap/独占不变；inline仍用原条件/公式，header不为零，不读取零值body。nil记录、空字段、当前值重读、tag、预算、预检、布局、Pack/Decode和公开API均不改，无新分配/unsafe/池。

未计时归档：符号指令216→216、32B帧不变、panic调用点15→14。非inline长度写出复用了已求出的地址/长度，第二次SUB/NEG/AND定位序列消失；inline分支却多了两条寄存器清零指令。逻辑条件不变不等于机器路径完全不变，必须同时测游戏消息和混合记录成本，不按检查点减少就宣称提速。唯一候选原文、BCE和前后汇编在[preflight](preflight/summary.json)。

## 方法与覆盖

复用前轮已提交的记录数组用例：10个inline/4vs8字节边界、后继字段、nil、稀疏tag、预算、命名整数参考wire/cap；不为相同契约再建测试。原inline判断未改，不重做同一个上限+1负向探针。基线与候选既有记录/元信息/当前值/预算/RPC契约、109形态525项差分和harness统计/选择/GC/fixture/默认指标/vet均在计时前完成。

采样前固定[计划](plan.json)、[补丁](candidate.patch)、[驱动](reproduce.py)。原70项×self/clone/两轮比较，共280个独立进程，构造原夹具后只保留所选闭包、清输出引用并GC，再预热与有界校准。GOMAXPROCS=1、GOGC=100、无限GOMEMLIMIT；原sampleBlock/blockCalls不变，6轮×4个ABBA/BAAB quartet×4块。每臂三次128调用分配观测，状态快照在计时外。

所有构建/测试/vet/汇编在首个计时进程前完成，计时期间不改输入。没有重试、删异常、扣控制偏差或设置最低收益比例。固定矩阵结束后再独立复算、重建fresh归档核对。

## 结果与保留决定

保留小幅、局部收益，并接受相反成本。大型游戏PEncode两轮快0.51%/0.77%（少35.3/54.2ns），累计少336.3/198.0ns；half4096记录真实请求快0.94%/0.67%（少1,390.3/982.1ns），累计少1,496.1/759.4ns，两组控制均通过。256条full记录纯Encode也两轮快0.87%，累计同向；这些收益不因比例低而丢弃。

大型游戏真实请求基本持平且方向不同，纯Encode也翻转，不能称所有游戏路径加速。full4096请求两轮多1,810.3/593.0ns，两组控制均失败，保留为有不确定性的成本；含数组记录的Encode/PEncode/请求也观察到配对变慢。低复杂度局部变量复用、目标分配不变和部分真实路径的重复收益支持保留，不能据此宣称上一轮混合记录代价已全面解决，更不能把不同轮次/基线的比例直接相加成净收益。

负号更快。括号为轮级几何耗时差中位数（ns），不是百分比乘累计耗时。所有70项原值/控制/分配/累计保留于各模式raw/summary及[独立分析](analysis.json)。

| 入口 | 第一轮：比例（差值） | 第二轮：比例（差值） | self / clone控制 |
| --- | ---: | ---: | --- |
| `body/game_large/sproto_encode` | -0.81%（-44.9ns） | +0.44%（+23.8ns） | 通过 / 通过 |
| `body/game_large/sproto_pencode` | -0.51%（-35.3ns） | -0.77%（-54.2ns） | 通过 / 通过 |
| `rpc/game_large/notification` | +0.96%（+66.5ns） | -0.14%（-9.9ns） | 通过 / 未通过 |
| `rpc/game_large/request_cancel` | -0.06%（-3.9ns） | +0.17%（+12.0ns） | 通过 / 通过 |
| `rpc/game_large/response` | -0.49%（-33.4ns） | -0.92%（-65.0ns） | 通过 / 通过 |
| `body/game_small/sproto_pencode` | -0.30%（-2.5ns） | +1.00%（+8.4ns） | 通过 / 通过 |
| `rpc/game_small/request_cancel` | -0.97%（-8.8ns） | +0.01%（+0.2ns） | 通过 / 通过 |
| `rpc/addressbook/request_cancel` | -2.13%（-14.0ns） | -0.84%（-5.8ns） | 通过 / 通过 |
| `growth/flat/full/256/encode` | -0.87%（-89.7ns） | -0.87%（-88.8ns） | 通过 / 通过 |
| `growth/flat/full/256/pencode` | -1.22%（-162.8ns） | -0.03%（-4.1ns） | 未通过 / 通过 |
| `growth/flat/full/256/request_cancel` | -0.15%（-21.4ns） | -1.02%（-140.9ns） | 未通过 / 通过 |
| `growth/flat/full/512/request_cancel` | -0.10%（-26.3ns） | -5.31%（-1723.4ns） | 未通过 / 通过 |
| `growth/flat/full/1024/pencode` | -0.25%（-132.9ns） | -1.21%（-772.9ns） | 未通过 / 通过 |
| `growth/flat/full/4096/encode` | -6.60%（-12449.5ns） | -1.97%（-3294.6ns） | 通过 / 通过 |
| `growth/flat/full/4096/pencode` | -0.88%（-1859.0ns） | -0.08%（-191.0ns） | 通过 / 通过 |
| `growth/flat/full/4096/request_cancel` | +0.81%（+1810.3ns） | +0.34%（+593.0ns） | 未通过 / 未通过 |
| `growth/flat/nil/4096/pencode` | +1.18%（+900.1ns） | +0.64%（+478.7ns） | 通过 / 通过 |
| `growth/flat/half/4096/pencode` | +2.04%（+2920.8ns） | -0.41%（-596.6ns） | 通过 / 通过 |
| `growth/flat/half/4096/request_cancel` | -0.94%（-1390.3ns） | -0.67%（-982.1ns） | 通过 / 通过 |
| `growth/nested/32x32/encode` | +0.15%（+69.3ns） | +0.77%（+337.5ns） | 通过 / 通过 |
| `growth/nested/32x32/request_cancel` | -0.14%（-81.0ns） | -0.32%（-185.7ns） | 通过 / 通过 |
| `growth/nested/32x128/request_cancel` | -0.45%（-1036.1ns） | -0.72%（-1652.5ns） | 通过 / 未通过 |
| `growth/array_record/4096/encode` | +0.58%（+1630.2ns） | +1.47%（+4128.8ns） | 通过 / 通过 |
| `growth/array_record/4096/pencode` | +0.58%（+1858.9ns） | +0.37%（+1172.0ns） | 通过 / 通过 |
| `growth/array_record/4096/request_cancel` | +0.15%（+469.6ns） | +0.38%（+1216.2ns） | 通过 / 通过 |
| `body/ascii32/frame` | +0.48%（+1.3ns） | +0.23%（+0.7ns） | 通过 / 通过 |
| `body/int32_edges/sproto_pencode` | +0.53%（+16.6ns） | -0.71%（-22.9ns） | 通过 / 未通过 |
| `rpc/int32_edges/request_cancel` | +3.00%（+96.3ns） | -0.00%（-0.0ns） | 通过 / 通过 |

例如256条full PEncode第二轮只快0.03%、少4.1ns（self失败、clone通过），仍作为带不确定性的观察保留。32×32嵌套请求只快0.14%/0.32%，累计也更快、两组控制通过；不设置最低收益线。反方向同样保留：nil4096 PEncode多900.1/478.7ns、32×32纯Encode多69.3/337.5ns、AddressBook响应多6.0/4.3ns（self失败），不以候选简单就忽略。

### 累计值、控制与离群

至少一轮配对/累计方向相反的入口共27项，见[全部翻转](cumulative-reversals.json)。大型响应第二轮配对少65.0ns、累计多259.9ns；256条full请求第一轮配对少21.4ns、累计多1,088.9ns；4096条full请求第一轮配对多1,810.3ns，累计却少10,128.5ns。half4096 PEncode第一轮配对多2,920.8ns、累计多22,489.9ns；第二轮配对少596.6ns、累计仍多1,830.0ns。含数组记录4096纯Encode两轮配对多1,630.2/4,128.8ns，累计却少44.9/12,556.5ns；两组控制通过也不消除这种差异。

self50/70、clone56/70控制通过，失败/轮值全留，不能沿用上一轮60/70与64/70。最大单轮self为AddressBook纯Encode+40.04%，clone为小型响应+45.80%，pass1为32B通知−45.04%，pass2为4KiB ASCII响应+44.79%。最长块分别是self 256条full请求A臂158.714ms（813调用/3次GC）、clone AddressBook请求A臂260.261ms（15,096调用/1次GC）、pass1 32B通知A臂202.921ms（75,815调用/1次GC）、pass2 1024条full PEncode A臂270.225ms（159调用/3次GC），见[最长块](long-blocks.json)。没有重跑、删除或按GC次数修正。

所有1,120快照limiter为零，最大堆对象快照15,291,392B（约14.58MiB），不是峰值，也不能证明没有GC/其他运行时成本。分配中位数self69/70、clone69/70、pass1 69/70、pass2 68/70相同；不同均为128调用观测中的16B尾差，mallocs仍5：self ASCII手工帧A高；clone与pass1 zero手工帧B高；pass2 ASCII帧A高、zero帧B高（14,616与14,616.125B/次）。目标分配中位数不变，但不说所有观测完全一致或把堆快照增加归因于新增对象。

### 未改路径

整数数组纯Pack两轮快1.19%/2.76%，组合PEncode却+0.53%/−0.71%，实际请求第一轮多96.3ns/第二轮几乎持平；这些路径没有执行本次原生整数记录helper，不能把其变化归功于body视图复用或用来扣除目标耗时。

采样后只读核对四个实际二进制：未改Pack均296条、encodeFieldInto均864条，规范化指令序列分别相同，见[指令清单](unchanged-functions.json)。这不排除布局、缓存、运行时或其他因素，也未找到离群的唯一成因；没有把结果说成“已解释为噪声”。

## 验收与下一步

独立复算280进程、26,880块、6,720 quartet、1,680轮和1,120快照到1e-12，逐项/合并原值、顺序/次数一致；冻结driver/probe/plan/源码/生成输入/fixture/二进制和fresh归档、原sampleBlock/blockCalls均核对。没有准备失败或计时重试。

生产库只改 `encode_integer_record.go`（局部body视图），18个Go文件与冻结候选逐字一致、gofmt通过，测试文件未改。库全测/官方C-Lua、race+checkptr=2、vet、10秒Codec fuzz512,102次（新增5个interesting、没有失败）、GSR三相关包及Go/TS7/7零跳过全部通过，见[验收命令](acceptance.json)。未改Pack算法，不重复Pack随机fuzz；没有新PB/TS排名。oracle沿用历史SHA一致的core/facade，parser来自只读Skynet且单独归属，见[来源](oracle-provenance.json)。临时modfile和服务器module-info确认本地库接线，正式go.mod/go.sum不变。

新库代码与GSR文档均未提交；未推送、发布或升级依赖，Decode继续冻结。下一轮先静态检查合流条件是否导致inline路径物化零值body，只有能减少重复工作且不改变store顺序的低复杂度候选才计时；不同时改预检、布局、宽度算法或Pack，也不承诺能消除所有本轮成本。

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-record-body-view/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/record-body-view-repro
python3 -B docs/benchmarks/2026-10-19-gosproto-record-body-view/analyze.py tmp/record-body-view-repro
python3 -B docs/benchmarks/2026-10-19-gosproto-record-body-view/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/record-body-view-fresh --prepare-only
```

合并原值、摘要、命令和整理后汇编入报告；280份逐项原值、二进制、补充objdump原文在忽略目录 `tmp/gosproto-record-body-view/`，各模式inputs保留逐项SHA。文本汇编只去行尾空白，原始/整理后SHA分别保留，核对见[validation.json](validation.json)。
