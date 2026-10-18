# 整数记录 Fields 局部复用实验（未采用）

2026-10-18。上一轮数组游标优化已本地提交为 gosproto `0c1f6cf` / GSR `22eb772`，提交后两库干净、未推送。本轮完成一个两行生产候选的固定评估，随后只撤回自身生产改动；最终库生产源码与0c1f6cf逐字节相同，保留测试增强及实验文档未提交。正式依赖仍v0.4.0，Decode冻结。

## 候选与编译结果

encodedIntegerRecordSize、encodeIntegerRecordInto原本逐字段读取schema.Fields[index]；候选各在循环前声明fields := schema.Fields，循环用fields[index]。它只在每条记录的同步调用内复制切片头，不复制字段或消息值，不保存到schema或跨调用缓存。仍按schema.order索引，使用当前字段指针和既有owner/形状验证，完整保留nil/ignored/tag/深度/元素/字节预算。

先行检查读取上一轮已验收二进制，其记录函数源文件与0c1f6cf一致，来源指纹见precheck-inputs.json。随后从0c1f6cf与candidate.patch独立构建前后版本：ARM64汇编显示旧循环每字段都有Fields长度/数据指针加载，新版移到循环前，field索引边界检查保留。该指令变化成立，不能据此推定实际收益，编译器文本和普通构建指纹全部保留。

候选净增两行，无新增API、类型、依赖、unsafe、池或缓存；Pack/Unpack、原生数组游标、RPC、Decode及通用body路径未改。

## 测试增量

扩展既有TestIntegerRecordArrayBudgetsAndTags：原生record把tag40000声明在首位，夹带无协议tag的Ignored string，再声明tag0/tag20000；具名整数参考类型维持原Go顺序。这样Go布局、声明顺序和tag顺序不同，还必须忽略非协议字段。原有稀疏间隙、nil/空记录、精确byte/depth/element预算、缺失bridge错误和解码行为全部继续保护，不新增整套重复夹具。

增强后的既有用例与元信息替换/形状变化/当前值测试先在基线通过，候选也通过。独立负向归档将两处for _, index := range schema.order故意改为for index := range schema.order，错误使用位置而非tag排序索引：新用例因tag gap按预期失败；同一个错误实现换回旧夹具仍通过，证明本次覆盖有增量。负向测试没有修改测量归档或主库。

## 固定43项配对

完全复用上一轮原19项body/手工帧、19项实际RPC及5项混合64位诊断，探针与夹具内容不变；10项小型/大型游戏的Encode/PEncode/RPC是目标，其他33项作观察。self/clone/两次候选比较固定运行，不重试、删样本或调整候选。Go1.24.13 darwin/arm64、Apple M2、GOMAXPROCS=1、GC开启；六轮×四组ABBA/BAAB、目标块10 ms。

**16512块、4128四元组、1032配对轮次**独立复算到1e-12一致。负值为耗时减少，百分比为六轮配对比值中位数，ns为每轮两臂几何均值差的中位数；不是置信区间，不用独立中位数相减。

### 目标路径

| 项目 | 第一轮 | 第二轮 | 两轮配对差 ns |
| --- | ---: | ---: | ---: |
| body/game_small/sproto_encode | -0.73% | -0.03% | -5.4 / -0.2 |
| body/game_small/sproto_pencode | +0.07% | +1.15% | +0.7 / +10.9 |
| body/game_large/sproto_encode | -0.75% | -1.17% | -47.7 / -75.7 |
| body/game_large/sproto_pencode | +0.05% | +0.18% | +4.5 / +16.1 |
| rpc/game_small/notification | -0.83% | -0.09% | -8.4 / -0.9 |
| rpc/game_small/request_cancel | +0.12% | -0.86% | +1.3 / -10.0 |
| rpc/game_small/response | -0.60% | -1.11% | -6.0 / -11.3 |
| rpc/game_large/notification | +1.07% | -0.16% | +93.8 / -14.2 |
| rpc/game_large/request_cancel | +1.29% | +0.32% | +114.0 / +27.5 |
| rpc/game_large/response | -0.32% | +0.52% | -29.2 / +45.5 |

大型纯Encode约−0.75%/−1.17%（−47.7/−75.7 ns），PEncode却为+0.05%/+0.18%（+4.5/+16.1 ns）。大型RPC通知+1.07%/−0.16%、请求+1.29%/+0.32%、响应−0.32%/+0.52%，方向不一致。小型纯Encode的−0.73%/−0.03%也未稳定转化到组合入口。

大型纯Encode的self/clone中位差约−0.69%/−0.72%，self IQR4.89pp未通过；与候选纯Encode变化同阶。这不是可相减的“偏差修正”，但限制了对数十纳秒变化的归因。当前证据不能确认业务组合路径获益，故撤回生产候选；没有使用最低5%/2%等淘汰线。

### 其他33项

| 项目 | 第一轮 | 第二轮 | 两轮配对差 ns |
| --- | ---: | ---: | ---: |
| body/addressbook/sproto_encode | -0.28% | +0.64% | -1.5 / +3.5 |
| body/addressbook/sproto_pack | +1.04% | +1.20% | +1.2 / +1.5 |
| body/addressbook/sproto_pencode | +0.34% | +0.36% | +2.1 / +2.3 |
| body/game_small/sproto_pack | -0.01% | +0.97% | -0.0 / +2.9 |
| body/game_large/sproto_pack | +0.84% | -0.53% | +23.5 / -14.6 |
| body/int32_edges/sproto_encode | -0.14% | -1.42% | -2.4 / -25.7 |
| body/int32_edges/sproto_pack | -1.04% | +0.46% | -27.5 / +11.8 |
| body/int32_edges/sproto_pencode | -0.04% | +0.87% | -1.4 / +32.5 |
| body/echo_ascii4k/sproto_encode | -0.35% | +0.87% | -2.9 / +7.6 |
| body/echo_ascii4k/sproto_pack | +1.07% | -4.92% | +8.6 / -43.1 |
| body/echo_ascii4k/sproto_pencode | -1.16% | +0.17% | -10.6 / +1.5 |
| body/ascii32/frame | +0.15% | +0.34% | +0.4 / +1.0 |
| body/utf8_1k/frame | -0.96% | -0.15% | -7.7 / -1.3 |
| body/ascii4k/frame | -0.92% | -0.71% | -21.8 / -17.8 |
| body/zeros4k/frame | -0.45% | -1.93% | -16.8 / -74.5 |
| rpc/addressbook/notification | -1.39% | +1.09% | -9.4 / +7.4 |
| rpc/addressbook/request_cancel | +0.23% | +0.42% | +1.7 / +3.0 |
| rpc/addressbook/response | -0.07% | +0.72% | -0.5 / +4.8 |
| rpc/int32_edges/notification | +0.02% | -0.40% | +0.6 / -15.4 |
| rpc/int32_edges/request_cancel | -0.20% | +0.33% | -8.0 / +13.1 |
| rpc/int32_edges/response | -0.06% | -0.68% | -2.6 / -26.4 |
| rpc/echo_ascii4k/notification | +1.23% | +0.88% | +12.7 / +9.5 |
| rpc/echo_ascii4k/request_cancel | -0.81% | -0.01% | -9.4 / -0.0 |
| rpc/echo_ascii4k/response | -1.02% | -0.32% | -10.9 / -3.5 |
| rpc/ascii32/notification | -1.06% | +1.03% | -1.7 / +1.6 |
| rpc/utf8_1k/notification | +0.47% | -0.58% | +1.8 / -2.2 |
| rpc/ascii4k/notification | -0.32% | -1.36% | -3.4 / -14.3 |
| rpc/zeros4k/notification | +0.14% | +0.82% | +3.0 / +17.5 |
| body/wide64/sproto_encode | +0.23% | +0.95% | +4.8 / +19.1 |
| body/wide64/sproto_pencode | +0.55% | -0.41% | +38.6 / -28.0 |
| rpc/wide64/notification | +0.49% | +1.48% | +35.6 / +107.0 |
| rpc/wide64/request_cancel | -1.01% | +0.74% | -76.2 / +54.5 |
| rpc/wide64/response | +0.08% | -0.86% | +6.2 / -65.1 |

43项对象数中位数均相同；两轮零字节4KiB手工frame的B/op为14616→14616.125、对象数仍5，其余B/op相同。16B/128调用的尾差保留，与已有tiny/GC窗口现象同量级，本轮不重做机制探针或强行确定来源，不截断计数使控制过门。

本轮int32纯Pack−1.04%/+0.46%，其参照是0c1f6cf与Fields候选；上一轮未解释的Pack+9.45%属于6d854d1→数组游标候选，**比较基线不同**。本轮数值不用于宣称旧差值已解决，旧记录与原二进制指令核对保留于[游标报告](../2026-10-18-gosproto-array-cursor/RESULTS.md)。

self **32/43**、clone **35/43**满足原控制标记，全部失败项、IQR、逐轮比值与GC/分配窗口保留在joined.json/原始记录。控制通过不证明无回归，未通过也不自动否定小收益；此次决定基于纯Encode信号与控制同阶、PEncode无改善和RPC方向混合的整体证据。

## 最终工作树与验证范围

在固定采样结束后，先核对主库全部Go源码与采样/编译归档candidate一致，再只把encode_integer_record.go恢复为0c1f6cf。candidate.patch、全部原值及撤回前源码指纹仍保留；未丢弃用户改动，最终只剩limits_test.go中的测试增强。最终全部生产Go源码与基线一致、测试内容与已在基线通过的版本相同。

本轮实际执行：增强后的相关测试在基线/候选各一次、独立harness夹具/统计/选择检查与编译、四次43项采样、负向新旧夹具对照，以及四份prepare-only归档重建核对。因生产候选未采用，没有重复库全测/官方/race/checkptr/fuzz、GSR三包或Go/TS验收；正式实现仍以上一轮0c1f6cf验收为依据，不把旧验证次数称作本轮新执行。

从GSR根目录重建实验，输出目录须不存在：

```sh
python3 -B docs/benchmarks/2026-10-18-gosproto-record-fields/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-record-fields-new
python3 -B docs/benchmarks/2026-10-18-gosproto-record-fields/analyze.py \
  tmp/gosproto-record-fields-new
```

库须含0c1f6cf；patch固定被撤回候选，脚本从归档重建，不依赖主库是否保留它。--prepare-only只建输入。汇编/源码/二进制/工具与夹具指纹完整保留，二进制和归档源码不提交。

下一候选先检查记录入口：目前helper接收reflect.Value，先IsNil，再经messageEncodePointer调用UnsafePointer。可以在已验证的数组入口借用现有encodePointer，再传入记录helper，以一次指针提取承担nil判断并减少反射值传递；每个阶段仍读取当前对象，不缓存标量、不扩大unsafe范围，保留nil/tag/预算/元信息替换与失败原子性。它须独立测试和计时，本轮尚未实现，不预言收益，Decode继续冻结。
