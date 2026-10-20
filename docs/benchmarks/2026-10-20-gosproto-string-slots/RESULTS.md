# 字符串数组 Decode 的受控写槽优化

2026-10-20，基线为已提交gosproto `50c69e1`，包含上一轮Unpack优化。本轮回到Body Decode，保留字符串数组的局部优化：完整预检并构造新切片后，核对**目标实际Go元素类型恰为原生string**，同步借用其`[]string`元素槽写入，减少逐项reflect.Index/SetString。命名切片（元素仍是string）可走快路径；命名字符串元素走原反射路径。没有改变协议、公开API或消息模型。

## 结果与采纳范围

**保留长字符串数组的重复改善，同时接受并披露小数组代价，不宣称现有大型游戏消息提速。** 不设最小收益百分比，也不根据这轮数据增加未经测量的长度阈值。

| 场景 | pass1 中位 / 累计 | pass2 中位 / 累计 |
| --- | ---: | ---: |
| 原生128项 Decode | -9.46% / -5.68% | -9.21% / -5.55% |
| 原生128项 PDecode | -4.62% / -0.53% | -3.70% / +34.72% |
| 原生512项 Decode | -9.12% / -3.81% | -9.06% / -12.45% |
| 原生512项 PDecode | -6.75% / -2.55% | -7.34% / -10.74% |
| 命名切片128项 Decode | -9.20% / -8.83% | -9.34% / -9.23% |
| 命名切片128项 PDecode | -5.40% / -6.00% | -5.61% / -5.66% |
| 原生3项 Decode | +0.55% / +0.31% | +1.35% / +1.66% |
| 原生3项 PDecode | +1.86% / +6.68% | +0.15% / +6.05% |
| 现有game_large Decode | +0.42% / +3.89% | +1.59% / +2.55% |
| 现有game_large PDecode | +0.70% / -0.50% | +0.28% / +3.61% |

负值表示候选耗时更低。现有game_small/game_large都只有3个label；本轮不能拿128/512项数组的约9%替代游戏消息成绩。长数组是额外的单字段tag0对照，字符串由固定中文前缀和索引生成，原代表夹具及wire签名没有修改，也没有新增或改变Protobuf schema。本轮没有新PB/TS采样，不推算新的Go/PB排名。

全部22入口、控制与命名元素回退见 [ALL-CASES.md](ALL-CASES.md)；全部逐轮/块、分配和GC见analysis.json及原始JSON。累计按实际时间与调用数计算，不替代轮次几何比中位数。原生128项PDecode第二轮累计+34.72%等反向结果完整保留，不能将其描述为端到端稳定改善。88项模式/入口中26项中位/累计反向（含控制）。

## 代码、安全与静态代价

countChunks、元素预算、完整chunk边界预检、reflect.MakeSlice、逐项readChunk及错误处理都保持，最终仍value.Set(result)。每个string仍由`string(content)`独立复制，不能借用输入。新指针只借用本次新切片，长度正数且实际元素类型精确核对；不根据可变公开描述推断Go布局，不缓存uintptr、对象地址或slice，不增加池/arena。

赋值是Go类型化string赋值，编译器保留实际GC写屏障（汇编有gcWriteBarrier2），不能改为指针或字节memcpy。命名字符串元素不满足原生元素证明，保持反射写入。外来描述也先取得目标实际reflect.Value，再判断实际元素类型；目标为命名元素的外来原生描述回归验证走安全回退。输出仍是原reflect.MakeSlice创建的准确类型、长度及容量，包括空数组行为。

静态机器字行（含填充）176→268，栈帧208→240B；增加实际类型证明、unsafe.Slice构造及逐项快/慢分支，代码和栈成本不可忽略。长数组以减少逐项反射摊薄这些成本；3项小数组有实测代价。此次没有新增private元信息标记或生成器。

## 参考工程实现

只读核对官方Sproto `sproto.c:1059` 的decode_array_object：逐个检查长度后经callback交付元素，不规定Go对象构造方式。Go protobuf v1.36.1 `internal/impl/codec_gen.go:5228` 的consumeStringSlice使用已绑定的类型化字符串切片并追加`string(v)`；带UTF8验证的变体另保留验证。

本轮参考的是避免逐项通用反射的工程方式，没有移植Protobuf wire、取消校验或统一两种协议模型。GSR/gosproto继续先做完整有界预检，再一次分配准确结果；不照搬append增长。参考文件哈希见reference.json。

## 单模块同进程方法与控制

在一份50c69e1归档增加实验decoder同源副本、候选和临时选择器；选择器在计时外切换现有schema的decoder指针，所有臂共享同一模块、同一份元信息、相同公开Decode/DecodePacked入口和消息构造器。每个case/mode一个进程；没有并行修改schema，切换接口不进入生产。实验结束还原基线decoder。clone函数体去除名称/空白后与基线相同。

5种原代表消息×Decode/PDecode，加0/3/128/512项数组、命名切片128项、命名元素128项×两入口，共22项；self/clone/pass1/pass2四模式。每臂预热50ms，基线校准约20ms调用数，双臂使用相同调用数；6轮×4个ABBA/BAAB四元组，GOMAXPROCS1/GOGC100/GOMEMLIMIT=off。共88进程、528轮、2112四元组、8448块，98条准备/测量流程命令成功。计时外记录每块MemStats，所有GC、暂停、长块和分配尾差保留，不强制两臂之间GC，也不扣除控制差值。

控制仍有局部异常：512项Decode clone中位−2.55%、IQR7.31pp；命名切片128项Decode clone中位−9.77%、IQR16.57pp；game_large PDecode self中位−7.07%、IQR13.85pp。不能声称所有噪声消失。原生128/512项Decode两轮的集中改善和机械原因支持局部采纳，但小数组/大型整体/非目标入口不能作确定加速结论，也不把回退自动解释成噪声后删除。

分配形态保持：3项Decode约216B/8次，128项4472B/133次，512项17784B/517次；现有大型Decode约10416B/532次。极小字节/对象尾差保留于块级telemetry，不宣称每一块精确相同。堆快照最大值不是峰值。ReadMemStats及计时外选择器仍可能影响后续块的GC相位，校准也不保证每块精确20ms。

## 测试增量与完整验收

计时前三种decoder同时通过原生string、命名外层切片和命名字符串元素的低GOGC、空/1/3/16/128项、嵌入零与非法UTF8字节、预算、坏尾部失败原子性、输入及跨调用独占回归的race/严格checkptr。夹具语义、wire、消费长度和padding检查后，每臂再次验证Decode/DecodePacked结果相等，再开始采样。

新增永久回归在完整基线首先通过；相对已有一般字符串/预算用例，增加实际元素类型分支、低GC下写屏障/所有权组合和外来原生描述映射到命名元素的回退。随后完整候选快照（不含实验选择器/副本）完成：

- 全库测试＋固定官方C/Lua oracle、全库race＋严格checkptr、vet。
- Codec fuzz限时10s、2 worker：456532次，持续推进。
- Decode fuzz限时10s、2 worker：13781次，3秒后停增，约11秒PASS退出；未延长或重跑，不以有限fuzz声称穷尽安全性。
- GSR protocol/sproto、debugserver、benchmarks三包测试；Go/TS集成7/7通过、零跳过。

包括新增回归的基线检查共8条验证命令成功，另2条gofmt准备命令。真实集成二进制build-info核对指向候选快照；候选核心函数与计时函数去除名称/注释/空白后相等。验收后decode.go和合并回归放入独立库，所有Go文件逐一等于验收快照，CHANGES同步记录小数组代价。

独立库实现和永久回归已提交为aef6876，原六项删除保持。GSR正式依赖仍v0.4.0，使用临时modfile验收。本轮实现未推送/发布。

## 复现及下一步

```sh
python3 docs/benchmarks/2026-10-20-gosproto-string-slots/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-string-slots/analyze.py --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-string-slots/accept.py \
  --experiment /absolute/new-output --output /absolute/new-acceptance-output
```

输出目录必须不存在，保留Go缓存。临时归档与二进制在tmp/gosproto-string-slots-20261020，完整验收在tmp/gosproto-string-slots-accept-20261020，不提交二进制。计时前冻结源码、生成输入、JSON夹具和二进制，结束后全部核对，Python独立复算全部块至1e-12。

下一Body方向仍是现有大型结构的字段解析和对象构造；本轮已确认长字符串数组受益，不因此把大型Decode判为完成。Packing保留50c69e1已提交成果并继续观察，不把本轮数组结果替代公开Unpack/PDecode结论。
