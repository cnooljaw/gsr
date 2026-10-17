# Decode 入口与普通 mask 恢复候选筛选

2026-10-17。上一轮受控写入已本地提交为 gosproto `fc3246a`、GSR `862fa8b`，工作树干净后继续，未推送/发布。当前四个候选全部未采用，全部生产 Go 源码仍为 `fc3246a`；只保留 TestPack 的新增 FF 后继回归和本报告。正式 GSR 依赖仍为 v0.4.0。

后续状态：该切片已本地提交为 gosproto `a09fb00`、GSR `d1beb28`。最后一次独立入口实验也未达保留门槛，Decode 性能阶段已按用户决定冻结，见 [冻结报告](../2026-10-17-gosproto-decode-freeze/RESULTS.md)；不改写下文原始数据。

## 结论与范围

入口 helper 的编译器复杂度不是足够的性能决策依据。三种保持类型检查的写法没有带来值得保留的组合收益；随后根据新 profile 验证较密 word 固定八字节恢复，也因真实业务和稀疏控制回退撤回。本轮没有进入五轮正式验收，不替换上一轮正式基线或产生新的 PB/SP 排名。

Apple M2、Go 1.24.13 darwin/arm64、GOMAXPROCS=1、PB v1.36.1。每项固定 60000 次、三轮交替反转操作顺序：三个入口候选各 59 项 × 三轮（代表性 Decode/Unpack/PDecode/PB 与 Echo payload/frame），固定恢复候选另做完整 129 项 × 三轮，合计 **918 样本**。前者未跑 35 种 shapes，不能称为完整矩阵。

候选均从 `fc3246a` 归档，基线只改 module 名称以同进程加载；所有夹具、TS、Encode、RPC、元信息和资源语义不变。入口候选仅改 Decode helper/调用，dense 仅改普通 Unpack 恢复，完整预检保持原样。各候选源文件指纹和样本见 record.json / element.json / address.json / dense.json，原始输出见 [raw](raw/)。四组中 SP 分配字节数及次数与各自同场基线一致。

## 入口候选：全部未采用

1. `record`：parser 把已经取得的 record 与原指针值一起传入 helper，改为 record.Type() 与私有 schema.typ 比较。
2. `element`：helper 自行 value.Elem().Type()，避免 reflect.Type 接口的 Elem 查询，但重复取得一次结构体值。
3. `address`：helper 仅接收 record，核对类型后通过 record.Addr().UnsafePointer() 借用地址。

都保留实际类型检查、每次 store 的 native/owner/Wire/Array 检查、反射回落、独占分配及写屏障，不依据统计结果删检查。

三轮中位数相对各自同场基线，负数更快：

| 候选 | 大型 Decode | 大型 PDecode | 小型 PDecode | AddressBook PDecode |
| --- | ---: | ---: | ---: | ---: |
| 传入已取得的 record | -1.9% | -0.6% | -3.6% | -1.1% |
| helper 内 Elem().Type() | -0.9% | +3.1% | -0.3% | -0.9% |
| record.Addr() 取地址 | +1.7% | +2.1% | -2.9% | +4.9% |

record 最好的大型 Decode 只从 15.768 → 15.464 µs（-1.9%），PDecode 21.361 → 21.231 µs（-0.6%），4 KiB ASCII 组合约 +2.5%、整数数组纯 Decode 约 +2.5%。element 的大型组合约 +3.1%；address 的 AddressBook 组合约 +4.9%。不跨组拼接最佳中位数，也不将其他冻结项的小幅变化归为源码收益。

[compiler.json](compiler.json) 显示 record 的 messageDecodePointer 编译器成本由原 216 降到 152，仍不能内联；element 为 210，address 为 210。编译器成本不等于 ns/op，数据不支持为了这个数字改变现有封装。

## 新基线 profile

计时筛选完成后，复用完整源码和二进制指纹均与已提交 `fc3246a` 一致的 store.test，另采大型 Decode/PDecode 与整数数组 PDecode 各约三秒的 CPU / alloc_objects profile，见 [profiles.json](profiles.json)。profile 的 ns/op 不作为新性能基线；原始 pprof 和二进制留在忽略目录，保存命令、完整 Go 指纹、摘要和文件指纹。

- messageDecodePointer 累计占大型纯 Decode 总采样约 4.51%，大型 PDecode 约 2.41%。每字段 nativeInteger 与分配也仍可见。helper 的这部分累计不能与其调用者 decodeMessage 累计相加。
- 大型 PDecode 的 UnpackWithLimit 累计约 16.62%，整数数组 PDecode 约 29.94%；行级样本仍指向普通 mask 恢复写出和源切片推进，而不只是零块扫描。
- profile 下大型纯 Decode 仍为 10416 B / 532 次分配，主要对象归于 decodeMessage 的独占整数指针和 reflect.New。这里的 alloc_objects 是采样归属，不当作精确对象计数或承诺全部可消除。
- CPU 中 runtime.kevent 仍占约 18.6% / 29.8% / 53.7%，原样保留，不归入编解码工作，也不由这些百分比推算最多可提速比例。

## 较密 word 固定恢复：撤回

此前位置/计数表和游标候选已失败，见 [普通 Unpack 筛选](../2026-10-16-gosproto-unpack-candidates/RESULTS.md)。本次只验证一种不同的工作方式：零/FF 与至多两个有效位沿用原路径，普通 word 至少三位且剩余有效输入至少八字节时，用固定八次读取/写出恢复一个目标 word；零位显式写零。每次源索引不超过七，输出区间为已预检的八字节，不读取切片长度之外。未选中位置可能读取后继输入字节，但会被掩码清零，不消费它们。

没有新增 unsafe、表或分配；没有改变完整预检、错误优先级、输出预算/容量或独占规则。先通过既有 TestPack / TestUnpackOutputBudget，再做 129 项三轮筛选，单位 µs/op：

| 消息 | Unpack 前 → 后 | 变化 | PDecode 前 → 后 | 变化 |
| --- | ---: | ---: | ---: | ---: |
| 大型游戏 | 5.0210 → 6.0230 | +20.0% | 21.3650 → 22.1850 | +3.8% |
| 小型游戏 | 0.4919 → 0.5832 | +18.6% | 2.1260 → 2.2190 | +4.4% |
| 整数数组 | 5.4250 → 6.0790 | +12.1% | 7.9100 → 8.3690 | +5.8% |
| AddressBook | 0.1734 → 0.1945 | +12.2% | 1.2190 → 1.2020 | -1.4% |
| 4 KiB ASCII | 0.5338 → 0.5140 | -3.7% | 1.1950 → 1.4320 | +19.8% |

真实数值场景已出现明确回退，无需继续五轮，也没有靠追加更多密度分支修补。冻结 Decode 项和全部 shapes 原值均保留在 dense.json；例如 ASCII 纯 Unpack 与组合变化方向不同，不用某个分段中位数相加解释组合时间。

另用独立诊断统计固定 body wire 的 mask 分布，见 [histogram.json](histogram.json) 和 [探针源码](histogram.go.txt)：大型游戏 415 个普通 word 中，三/四/五/六有效字节分别 29/177/87/93；只有 2 个 FF word。整数数组的普通 word 中，一/二/三/四/五/六有效字节为 1/128/1/64/128/128。固定八次恢复在大量四字节 word 上增加了零位工作；结合回退结果，这不足以抵消省去的逐位循环。此处是从代码工作量和分布作出的解释，不作为硬件瓶颈的独立证明。

这份分布只代表当前五组 body 夹具，不代表线上负载或 RPC 完整帧。它也说明，不应根据长字符串 FF 快路径的收益推断游戏数值恢复一定能使用同一策略。

## 保留回归与验证范围

扩展原 TestPack 的独立 mask 字节用例：原来 mask 后接 0x81 word，现在另加一个具有不同字节的完整 FF word，并枚举八种源偏移，新增 **2048 组**。它验证普通 word 到 FF 的消费/位序；三/四字节 mask 后也存在至少八字节输入，可保护未来批量恢复不会把后继头当成当前数据。

新回归先在未改生产实现 `fc3246a` 通过，再在另建的 dense-retained-tests 副本通过；没有改动已经计时的 dense 副本。计时 Go 源码指纹与最终保留 pack_test.go 分开记录，见 [inputs.json](inputs.json) 和 [retained-tests.patch](patches/retained-tests.patch)。

[validation.json](validation.json) 区分本轮执行和未执行项：候选筛选每次运行 TestRepresentativeFixtures，dense 通过原相关用例；新回归分别通过基线及独立候选副本。四个归档/补丁 prepare-only 副本均重建出完全相同的 Go 源码、七份 harness 与三份 JSON 指纹。

由于未采用生产改动，没有重复完整库/race/checkptr/vet/fuzz、GSR 三包/GoTS 接线或 TS 性能；上一轮已提交的验收没有冒充本轮重新执行。没有新的跨平台执行结论。

## 后续判断

保留 `fc3246a` 的通用 parser 与受控写入，不再为了入口内联预算或某种合成 mask 追加分支。若继续缩小完整 PDecode 的差距，先针对普通 mask 的 word 级恢复提出新的执行方式，并使用本轮真实分布和完整控制矩阵验证；既有表、游标、八次固定恢复都已有失败证据，不机械重试。

减少完整 Unpack 中间缓冲的方案属于更大的执行模型实验，必须先说明随机 header/body 访问、未知字段跳过、完整预检/错误优先级、预算及输出独占如何保持；不能把它当作一个局部循环改写直接落地。本轮没有实施或采纳这种方案。

## 复现

需要包含 `fc3246a` 的独立库与当前 GSR 固定 harness/夹具：

```sh
python3 docs/benchmarks/2026-10-17-gosproto-decode-binding/reproduce-pairs.py \
  --library /path/to/gosproto --candidate dense --output /tmp/gsr-dense-reproduce
```

candidate 可选 record / element / address / dense。前三者默认重跑 59 项 × 三轮，dense 重跑 129 项 × 三轮；`--prepare-only` 只建立副本并核对指纹。只对副本应用补丁，不修改传入库。不会复现完全相同的 ns/op。新回归通过 retained-tests.patch 单独保留，不混入已计时快照。
