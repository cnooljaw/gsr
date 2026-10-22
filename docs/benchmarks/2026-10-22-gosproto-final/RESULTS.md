# Go Sproto 优化阶段收尾

2026-10-22，Apple M2、Go 1.24.13 darwin/arm64，Protobuf Go固定v1.36.1。本轮从正常库 `9e47281` 出发，完成最新基线、三个独立候选/收窄对照、最终组合验收和72项PB/SP对照，结束逐轮微改写循环。

保留两处实现：较长原生整数数组编码使用受控视图/复制；较长8字节整数数组正文解码复制到独立结果。源码已放入独立库工作树，尚未提交；上一轮报告已提交GSR `d6d4106`，未推送。正式GSR依赖仍v0.4.0。本轮测试使用完整Git快照和临时modfile，工作树十项用户删除保持。

## 最终实现

- `encodeInt64Slice`：仅386/amd64/arm64、至少32项时启用。4字节数组先固定已分配输出区间，以int32视图逐项写出；8字节数组复制本次数值的字节视图。短数组和其他架构继续原LittleEndian循环。
- `decodeIntSlice`：4字节保持已保留的四项循环；8字节仅在上述架构且至少32项时，复制到新make的[]int64。短数组和其他架构保持原循环。
- 架构条件是 `runtime.GOARCH` 编译期常量，不推断主机字节序，不把大端输入当成本机整数。原生视图仅作用于已经核对的[]int64；命名/窄整数、double和unsigned继续原有路径。
- 宽度、长度、元素数/字节预算和完整输入预检保持；32项阈值只是内部执行策略，预算校验在它之前。无新增堆分配，输出len/cap和跨调用独占不变；不缓存消息地址，视图不逃逸到全局，未引入uintptr中转。

[已验收decode.go](selected-decode.go.txt)、[encode.go](selected-encode.go.txt)、[测试增量](selected-limits_test.go.txt)、[实际工作树SHA](worktree-source.json)。[accepted.patch](accepted.patch)为相对9e47281的零上下文补丁（应用需 `git apply --unidiff-zero`），只含本次四个文件，不含用户删除。

## 最终同数据PB/SP结果

单位µs/op，各三轮中位；每个decode新建独立对象，包含结果分配。完整216个最终样本、每轮范围和32项Echo payload/frame见[COMPARISON.md](COMPARISON.md)与[final-analysis.json](final-analysis.json)。

| 消息 | PB Encode | SP Encode | SP PEncode | PB Decode | SP Decode | SP PDecode |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| AddressBook | 0.330 | 0.536 | 0.609 | 0.808 | 0.944 | 1.138 |
| 小型游戏 | 0.718 | 0.602 | 0.786 | 1.636 | 1.467 | 1.945 |
| 大型游戏 | 7.552 | 4.032 | 5.561 | 17.057 | 12.196 | 16.636 |
| 896整数数组 | 5.362 | 1.352 | 2.906 | 4.242 | 1.369 | 6.413 |
| 4KiB ASCII Echo正文 | 0.588 | 0.619 | 0.725 | 0.625 | 0.623 | 1.108 |

原始正文和packed组合必须分别评估：大型游戏正文Encode/Decode比PB约低46.6%/28.5%，PEncode低26.4%，PDecode接近PB（本次中位低2.5%，不能宣布稳定领先）。整数数组正文Encode/Decode约低74.8%/67.7%，PEncode低45.8%，PDecode仍高51.2%。AddressBook及小消息组合仍有差距。

PB/SP均使用当前固定语义fixture；PB数组为packed sint32，AddressBook保留presence。wire格式、额外资源校验和分配模型不同。frame测量沿用既有手工封装benchmark，不代表库实际Rpc.Request/Dispatch，更不含Gate、网络和MAC。TS源码未修改、未重跑TS性能，本轮Go数据不能替代TS排名。

## 相对本轮起点

正常9e47281基线40项×三轮120样本，最终72项×三轮216样本；前后时段不同，以下是整体复核，不能替代下面的配对归因实验。全部原始数据保留，无离群删除。PB Encode整数数组在两时段也有约11%波动，因此不把每个小变化都归因于优化。

| 实际整数消息 | 起点 | 最终 | 变化 |
| --- | ---: | ---: | ---: |
| Encode | 1.624 | 1.352 | −16.75% |
| PEncode | 3.159 | 2.906 | −8.01% |
| Decode | 1.424 | 1.369 | −3.86% |
| PDecode | 6.303 | 6.413 | +1.75% |

本轮保留的4字节编码快路径确实作用于该消息；4字节解码未修改，不能把Decode的−3.86%当作新解码算法收益。PDecode的反向同样保留。所有40项代表消息前后B/op与allocs/op完全一致，[before-after.json](before-after.json)包含全部数据。

## 候选、收窄与取舍

三个对照各36进程/self+pass1+pass2，各模式三轮ABBA/BAAB。均为正常独立二进制、同模块名；没有计时内选择器。每项Go benchmark自行校准调用数；两次B/A耗时取几何配对比，保留三轮及中位，近似累计用打印ns/op×调用数复算，有文本舍入限制。self不相减，未设收益百分比门槛。

1. 广义解码视图：32项用例、1152行测量，200ms目标。4字节改int32读取视图，8字节改复制。4字节结果与短数组不稳定，最终未保留该通用替换；长8字节正文有重复收益。[完整结果](ALL-CASES.md)。
2. 独立编码候选：20项、720行，200ms目标。该轮对所有非空原生数组启用，真实整数消息Encode中位−19.80%/−26.12%，PEncode−9.41%/−10.08%；896项8字节数组Encode−33.12%/−35.68%。局部短包与大型非目标反向保留，[完整结果](encode-review/ALL-CASES.md)。最终增加32项阈值，不能把这些百分比标成阈值版精确配对收益；阈值版由最终组合验收及上表正常整体复核确认。
3. 收窄解码：16项、576行，300ms目标，覆盖1/8/31/32/33/896项与真实消息。8字节896项正文Decode两遍中位−11.29%/−7.91%，累计−11.91%/−8.95%；对应PDecode中位+1.63%/+3.66%、累计+2.70%/+1.62%。保留正文收益并接受该局部组合代价，不宣称PDecode全面提速。32/33项与所有短包/非目标波动见[完整结果](selected-review/ALL-CASES.md)。

共108个进程、2448行候选测量、159条准备/验证/测量命令通过。控制和个别轮次存在较大离散；这些不是显著性证明，不跨harness挑选最好的数值。广义解码候选的指令440→340仅是未采用版本的静态证据，不能当作最终收窄版的指令数。

上一轮普通Unpack双字节循环仍不采用。当前整数数组450word全为普通mask，完整预检和普通字节恢复仍是组合瓶颈；旧位置/计数表、源窗口、密度分流、word写出、循环展开均未确认真实组合收益。此次不继续叠加同类改写，也不以删除预算/预检、借用输出或改变wire换速度。

## 最终验证

[acceptance.json](acceptance.json)、[final-validation.json](final-validation.json)和[模块信息](integration-build-info.txt)记录实际执行结果。

- 完整库 `go test ./...`、全库race+严格checkptr、vet通过；官方C/Lua oracle逐文件SHA核对后运行，不把跳过当成功。
- 既有 `TestNativeIntegerArrayWire` 扩展0～7字节wire起点偏移，验证解码值、消费长度、源输入不变；原有4/8字节极值、31/32/33边界、后继字段、Pack组合、独占与精确wire测试沿用。三轮候选准备时两侧相关测试也通过race/checkptr。没有复制另一套同义夹具。
- Codec fuzz 10秒474093次通过；Decode fuzz7409次后停增、约11秒正常退出，未延时重跑，不把停增视为持续覆盖。
- linux/amd64、linux/s390x测试编译通过；linux/386、linux/arm生产代码编译通过。这些不是对应架构真机运行。
- 32位测试编译在候选和9e47281基线均失败：旧 `encode_test.go:222/261` 将MaxInt64用于int，常量溢出。两条失败原样保留；因此32位不宣称测试编译通过，没有修改用户已删除的旧测试文件。
- GSR的protocol/sproto、debugserver、benchmarks三个受影响包测试通过；Go/TS 7/7通过，二进制模块信息确认使用最终正常候选快照。
- 最终40代表+32payload/frame共72项语义检查和benchmark完成，216个样本完整保留。源文件、fixture和二进制指纹在各测量阶段前后核验，实际库三个源码/测试文件与最终验收快照SHA相同。

原始accept驱动遇32位旧测试失败后停止，修正为明确的32位生产编译检查后，通过[accept-resume.py](accept-resume.py)只执行剩余工作。日志18条中16条成功，2条为已核对的候选/基线32位旧测试失败；没有改写退出码或重复fuzz掩盖失败。未来直接运行修正后的accept.py即可。

## 收尾结论

本轮常规优化收尾：留下有实际收益的长数组实现，短数组和其他架构保留原循环；所有未采用候选及正向小收益证据仍可检索。阶段产物是已验证源码、可应用补丁、完整对照与明确剩余差距，不能等同于所有场景达到性能极限或全面超过PB。

当前大型游戏与整数正文已明显领先PB，主要剩余项是数组Unpack、小结构通用运行成本和字符串组合。它们继续受到格式恢复、资源校验及独占分配影响；本轮没有新增代码生成、架构汇编或公共codec模型。正式依赖升级、版本发布和推送不属于本次改动。

## 复现

```sh
python3 docs/benchmarks/2026-10-22-gosproto-final/baseline.py --library /private/tmp/gsr-gosproto-fork --output tmp/final-baseline-new
python3 docs/benchmarks/2026-10-22-gosproto-final/run.py tmp/final-decode-new
python3 docs/benchmarks/2026-10-22-gosproto-final/analyze.py
python3 docs/benchmarks/2026-10-22-gosproto-final/encode-review/run.py tmp/final-encode-new
python3 docs/benchmarks/2026-10-22-gosproto-final/encode-review/analyze.py
python3 docs/benchmarks/2026-10-22-gosproto-final/selected-review/run.py tmp/final-selected-new
python3 docs/benchmarks/2026-10-22-gosproto-final/selected-review/analyze.py
python3 docs/benchmarks/2026-10-22-gosproto-final/accept.py tmp/final-accept-new
python3 docs/benchmarks/2026-10-22-gosproto-final/final-analyze.py
```

采样脚本使用固定Git归档，不恢复真实库的用户删除。输出目录必须不存在。baseline脚本将原始记录写到指定tmp目录；本次已将其副本以baseline前缀归档。二进制和完整临时构建目录留在忽略的tmp，Git保留文本数据与指纹。
