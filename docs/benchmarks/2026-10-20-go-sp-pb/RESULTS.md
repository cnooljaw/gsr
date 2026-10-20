# 当前Go gosproto / Protobuf同场对比

2026-10-20（用户时区Asia/Shanghai）。Apple M2、Go1.24.13 darwin/arm64，Protobuf Go v1.36.1，对比gosproto已提交优化版af9169d。正式依赖仍v0.4.0，采样用独立git归档和临时modfile，不修改任何生产代码或依赖文件；没有测试未采用的候选。独立库工作区已有go.mod等删除，未恢复或纳入采样，详细状态见environment。

本轮只比较Go：原代表消息40项＋Echo32项，各5轮200ms，共72项360个样本，中位数；GOMAXPROCS1/GOGC100/无限GOMEMLIMIT。完整语义/wire SHA/消费长度/padding检查通过后采样，每个解码新建结果，计时包含输出分配，不使用复用对象捷径。原始五轮全部保留，不剔除长尾。

## 消息体与packing（µs/op）

| 消息 | PB Encode | SP Encode | Pack | SP PEncode | PB Decode | SP Decode | Unpack | SP PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 0.335 | 0.593 | 0.110 | 0.622 | 0.886 | 1.028 | 0.173 | 1.232 |
| game_small | 0.687 | 0.691 | 0.235 | 0.850 | 1.733 | 1.612 | 0.472 | 2.103 |
| game_large | 7.665 | 5.555 | 2.087 | 7.046 | 17.807 | 15.489 | 4.783 | 20.463 |
| int32_edges | 5.007 | 1.726 | 2.017 | 3.180 | 4.294 | 1.739 | 5.131 | 6.967 |
| echo_ascii4k | 0.619 | 0.686 | 0.620 | 0.762 | 0.696 | 0.674 | 0.577 | 1.327 |

SP Encode/Decode为未packed正文；PEncode/PDecode为库公开组合路径。**与PB最终发送格式相比时，应同时查看packed结果，不能只用未packed正文宣布胜负。** Pack/Unpack分开预建输入测量，各项中位数不要求算术相加等于组合结果（组合可有内存融合，且分别采样）。这些表不含网络/MAC、Dispatcher、真实Rpc.Request/Dispatch或schema构造。

## packed组合相对PB

负值表示Sproto更快；差值按完整精度中位数计算。

| 消息 | PEncode 相对 PB Encode | PDecode 相对 PB Decode |
| --- | ---: | ---: |
| addressbook | +85.5% | +39.0% |
| game_small | +23.6% | +21.4% |
| game_large | -8.1% | +14.9% |
| int32_edges | -36.5% | +62.2% |
| echo_ascii4k | +23.2% | +90.6% |

## 包大小（字节）

| 消息 | PB | SP raw | SP packed |
| --- | ---: | ---: | ---: |
| addressbook | 69 | 130 | 83 |
| game_small | 191 | 328 | 216 |
| game_large | 2081 | 3336 | 2280 |
| int32_edges | 2435 | 3593 | 2374 |
| echo_ascii4k | 4099 | 4104 | 4103 |

两者wire不同，体积与校验契约也是取舍。Protobuf数组使用packed sint32，AddressBook保留proto2 presence；Sproto保留资源预算和失败原子性。

## Go分配（B/op / allocs/op）

| 消息 | PB Encode | SP Encode | Pack | SP PEncode | PB Decode | SP Decode | Unpack | SP PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| addressbook | 80 / 1 | 144 / 1 | 144 / 1 | 144 / 1 | 648 / 26 | 528 / 29 | 144 / 1 | 672 / 30 |
| game_small | 192 / 1 | 352 / 1 | 352 / 1 | 352 / 1 | 1208 / 53 | 1200 / 52 | 352 / 1 | 1552 / 53 |
| game_large | 2304 / 1 | 3968 / 2 | 3456 / 1 | 3968 / 2 | 13560 / 537 | 10416 / 532 | 3456 / 1 | 13872 / 533 |
| int32_edges | 2688 / 1 | 4096 / 1 | 4096 / 1 | 4096 / 1 | 4160 / 2 | 8288 / 4 | 4096 / 1 | 12384 / 5 |
| echo_ascii4k | 4864 / 1 | 4864 / 1 | 4864 / 1 | 4864 / 1 | 4160 / 3 | 4176 / 5 | 4864 / 1 | 9040 / 6 |

## Echo应用帧（µs/op）

| 输入 | PB frame Encode | SP frame Encode | SP变化 | PB frame Decode | SP frame Decode | SP变化 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| ascii32 | 0.189 | 0.277 | +46.6% | 0.259 | 0.367 | +41.9% |
| utf8_1k | 1.596 | 0.627 | -60.7% | 1.704 | 0.605 | -64.5% |
| ascii4k | 1.469 | 1.665 | +13.3% | 1.561 | 1.318 | -15.6% |
| zeros4k | 1.501 | 2.852 | +90.0% | 1.577 | 1.347 | -14.6% |

此套件SP帧仍是手工Encode(package)+Encode(body)+append+Pack；它不覆盖已优化的Rpc.Request/Attach融合路径。不能据此认定真实RPC编码同样慢，也不能用RPC历史数字替换本表。两套Echo正文类型/构造/采样位置独立，不能把相近字段在不同套件的结果强拼成加速曲线。全部payload项目见analysis.json。

UTF-8结果包含契约差异：Protobuf string编码会校验UTF-8，Sproto字符串不要求同样的UTF-8语义校验。SP在该输入上的优势不能全部解释为编解码内核优劣，不能通过关闭PB校验来造同口径。

## 改进方向（按2026-10-20用户裁决修正）

保持Body Encode/Decode与Pack/Unpack双主线，最终领先程度向TS参考靠近；详细比例及约束见[双主线目标](../../GOSPROTO-DUAL-TRACK.md)。大型正文已领先PB不等于优化完成，不能据此将Body整体降为次要方向。Body继续定位大型结构遍历/字段访问/对象构造成本；Packing侧优先Unpack→PDecode，以大数组和大型消息为目标，并保护全部字节形态。countChunks是一个局部机会，不代表Body全部潜力。小包固定成本与真实RPC继续独立观察，不以手工帧替代融合API。

## 波动与验证限制

这是开发机的五轮总览，不是ABBA交错的细小收益验收，也没有硬件计数器/置信区间；百分之几的差距不作统计显著性保证。完整min/median/max、所有五轮及分配见analysis.json，不能跨历史不同轮次拼接最佳值。此次无新profile、不跑TS，不作跨语言排名。

| 波动最大的10项 | min µs | median µs | max µs |
| --- | ---: | ---: | ---: |
| protocol/ascii4k/protobuf_payload/decode | 0.953 | 0.996 | 2.824 |
| representative/game_small/sproto_unpack | 0.464 | 0.472 | 1.246 |
| representative/addressbook/sproto_encode | 0.558 | 0.593 | 0.986 |
| representative/addressbook/protobuf_decode | 0.822 | 0.886 | 1.333 |
| representative/echo_ascii4k/sproto_pdecode | 1.218 | 1.327 | 1.898 |
| protocol/utf8_1k/sproto_payload/decode | 0.272 | 0.287 | 0.405 |
| representative/addressbook/sproto_pack | 0.107 | 0.110 | 0.152 |
| protocol/ascii4k/sproto_frame/decode | 1.246 | 1.318 | 1.725 |
| representative/echo_ascii4k/sproto_pack | 0.601 | 0.620 | 0.818 |
| representative/int32_edges/sproto_pack | 2.005 | 2.017 | 2.693 |

快照18份Go源码逐一等于af9169d；夹具、测试/生成输入、二进制、模块覆盖和正式依赖不变核对通过。只运行相关fixture测试和两套基准，没有重跑无改动库的全测/race/fuzz/官方oracle/GoTS。既有未提交阶段诊断及用户库工作区改动保持；本轮不提交、不推送。

```sh
python3 -B docs/benchmarks/2026-10-20-go-sp-pb/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output /absolute/new-output
```

输出目录必须不存在。复现脚本取af9169d git归档，不使用库工作区删除状态；实际模块解析另保存modules.json。独立复算脚本读取随报告保存的原始五轮。
