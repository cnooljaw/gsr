# Go Sproto双主线目标

2026-10-20，用户明确裁决：**同时推进Body Encode/Decode和Pack/Unpack，最终相对各自Protobuf的领先程度向TS结果靠近。**已领先PB不构成停止优化的理由；部分路径若已超过TS参考幅度，保持优势，不强行追同一个比例。

## 评价方式

分开跟踪纯Encode/Decode、单独Pack/Unpack、PEncode/PDecode和真实Request/Attach/Dispatch。每个入口看相对同语言同场PB的时间比、绝对ns和分配，业务组合结果是最终验收，同时保留正文独立进展，不能用一条主线的进展替代另一条。

TS为2026-10-15第五轮历史基线，Go为2026-10-20新五轮；共享代表消息输入哈希相同，但不是同场跨语言配对。PB分别为@bufbuild/protobuf2.15.0和Go protobuf1.36.1，运行时、对象/整数模型、资源校验和分配机制不同。下列比例是目标参照，不是Go必然可达的保证，也不拿跨语言绝对耗时直接排实现优劣。UTF8字符串校验差异、原生与manual API差异继续说明。

负值表示SP耗时低于各自PB。TS未重新采样。

## Body主线参考

| 消息 | Go Encode | TS Encode | Go Decode | TS Decode |
| --- | ---: | ---: | ---: | ---: |
| addressbook | +77.0% | -34.9% | +16.0% | -41.3% |
| game_small | +0.5% | -39.9% | -7.0% | -53.2% |
| game_large | -27.5% | -35.9% | -13.0% | -57.2% |
| int32_edges | -65.5% | -76.7% | -59.5% | -58.3% |
| echo_ascii4k | +10.9% | -77.8% | -3.2% | -27.0% |

## Packed组合参考

| 消息 | Go PEncode | TS PEncode | Go PDecode | TS PDecode |
| --- | ---: | ---: | ---: | ---: |
| addressbook | +85.5% | -11.1% | +39.0% | -28.1% |
| game_small | +23.6% | -18.3% | +21.4% | -39.8% |
| game_large | -8.1% | -15.4% | +14.9% | -44.8% |
| int32_edges | -36.5% | -37.1% | +62.2% | -18.4% |
| echo_ascii4k | +23.2% | -72.8% | +90.6% | -10.6% |

## 推进顺序与约束

- **Body主线保持独立投入**：大型结构的Encode和Decode都继续开放。先定位schema/字段遍历、实际字段访问、对象构造和指针字段分配成本，再提出单一候选；不因大型Encode已赢PB约28%、Decode已赢约13%而降为“基本完成”。数组作为已验证的高效路径和回归控制。countChunks是候选之一，不代表全部Body潜力。
- **Packing主线同步推进**：当前重点Unpack与packed Decode，优先验证整数数组和大型消息，同时保留普通word、零段、FF、小包和不同容量/所有权形态；Pack仍跟踪其对PEncode与真实RPC的影响。不得为了正文领先而省略packing，也不得因为packing落后就冻结正文。
- 两条主线各保持明确候选与验收证据，按热点/工程价值交替推进；一次实验只改变可解释的一处，避免在一个补丁中混入两类改写。不按机械轮数分配工作，不只追容易修改的小点。
- 参考官方Sproto的wire与数据流，参考Go Protobuf的工程实现；保持资源预算、失败原子性、presence、ownership、输出cap与公开契约，不靠删检查或换消息分布制造优势。较大重构可以讨论，但公开API/所有权等变化须按项目裁决流程另行确认。
- 真实可解释的小收益也保留，不设固定最低比例；同时公开非目标退化、控制失败、分配、累计与中位数分歧，不用TS目标为不可靠结果背书。

数据来源：[Go当前对照](benchmarks/2026-10-20-go-sp-pb/RESULTS.md)、[TS第五轮](benchmarks/2026-10-15-sproto-round5/RESULTS.md)、[迁移经验](SPROTO-OPTIMIZATION-LESSONS.md)。本文件只确认目标和对照，不代表新优化已实现，不修改历史实测数据。
