# 原生整数数组解码：四项固定视图

保留候选，正常源码与既有测试的增量已放入独立 gosproto 工作树，尚未提交。基线为 `9048cfa`，GSR 正式依赖仍为 v0.4.0。

## 改动与约束

仅原生 `[]int64` 的4/8字节解码循环按四项共用固定源/目标视图；剩余0～3项走原逐项循环。仍先校验宽度、长度和集合预算，再分配精确容量的新切片。符号扩展、命名/窄整数回退、错误顺序、独占输出和失败原子性保持，无新增 unsafe 或公共 API。

原函数与clone均348条指令/10个静态panic调用点，候选440/20。静态代码增长是真实代价，不能解释为减少了所有边界检查。收益来自实测，尚未定位到单一机器指令。

## 同进程结果

负值为候选耗时降低；每项列两遍六轮比值中位，累计耗时另列。完整self/clone控制见 [ALL-CASES.md](ALL-CASES.md)，全部原始块保留，不扣减控制。

| 消息/入口 | 第一遍中位 / 累计 | 第二遍中位 / 累计 |
| --- | ---: | ---: |
| 896整数数组 Decode | −13.82% / −3.23% | −18.71% / −19.07% |
| 896整数数组 PDecode | −2.77% / −2.98% | −4.63% / −17.22% |
| 大型游戏 Decode | −1.17% / −1.44% | −1.28% / −4.00% |
| 大型游戏 PDecode | +0.04% / −0.76% | −2.88% / −3.05% |

16进程1536块，166批校准决策独立复算通过。全部最终pilot达到40ms；正式块最短37.547ms、中位59.067ms、最长490.048ms，6块低于40ms，没有低于20ms。GC与长尾全部保留。self/clone仍有波动，大型PDecode第一遍中位接近持平，不宣称全部入口稳定提速。短数组本轮做正确性而非性能覆盖；后续正式基线应补短数组和不同机器。没有重跑Protobuf，不能更新正式PB排名。

整数数组Decode两臂均8288 B/4 alloc，PDecode均12384 B/5 alloc；大型仍约10416 B/532 alloc和13872 B/533 alloc，字节计数有极小后台摊销波动，无额外结果分配。

## 验证

- 三模式差分覆盖4/8字节、3/4/5等分块与余数边界、极值、截断/非法宽度、原生/命名/窄整数、预算与既有计数；比较错误、完整budget、结果/cap、跨调用独占及源输入不变，通过race/checkptr。
- 正常入口完整库测试、官方oracle（逐文件SHA核验）、全库race/checkptr、vet通过。既有 `TestNativeIntegerArrayWire` 增加5项边界，新增项单独race/checkptr通过；其余wire/所有权夹具沿用。
- Codec fuzz 10秒425618次通过；Decode fuzz10981次后停增，约11秒正常退出，未延时重跑，也不把停止增长视为持续覆盖。
- GSR三个受影响package测试及Go/TS 7/7通过；二进制模块信息确认实际指向正常候选快照。正式go.mod/go.sum未修改。

[acceptance.json](acceptance.json)记录8条正常入口验收，[acceptance-extra.json](acceptance-extra.json)记录新增边界与二进制模块检查。`accept.py` 已包含新增边界，实际首次全库验收后才补5项，因此另列额外检查，不改写历史命令。实验辅助函数名沿用旧harness的String前缀，计时选择的实际内容已由源码断言确认为整数数组decoder；生产库不含选择器。

## 复现

```sh
python3 docs/benchmarks/2026-10-22-gosproto-int-array-block4/run.py --library /private/tmp/gsr-gosproto-fork --output tmp/array-block4-new
python3 docs/benchmarks/2026-10-22-gosproto-int-array-block4/analyze.py --output tmp/array-block4-new
python3 docs/benchmarks/2026-10-22-gosproto-int-array-block4/check-calibration.py
python3 docs/benchmarks/2026-10-22-gosproto-int-array-block4/accept.py tmp/array-block4-accept-new
```

运行前需保留现有fixtures、生成Go代码与oracle。脚本使用完整Git归档，不恢复或使用实际库工作树的十项用户删除。下一步补短数组非目标性能与正式正常入口复核，再决定后续Body/Pack热点，不叠加其他改写。
