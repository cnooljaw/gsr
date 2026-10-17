# messageDecodePointer 独立入口实验与 Decode 冻结

2026-10-17。按用户“提交，单独实验入口成本 → 有明确收益才留 → 然后冻结 Decode”执行。上一轮已本地提交为 gosproto `a09fb00`、GSR `d1beb28`，未推送。当前独立库工作树干净，所有内容与 `a09fb00` 一致；Decode 生产源码与 `fc3246a` 相同，GSR 正式依赖仍为 v0.4.0。

**结论：入口候选在微基准明显更快，完整消息也有小幅收益，但未达到预先确定的保留门槛，未纳入库；Decode 以现有实现冻结。** 本报告是本阶段冻结前的最后一次有限入口实验，不延伸到新的 parser、分配模型或 Unpack 候选。新记录与冻结文档尚未提交。

后续状态：本阶段记录已本地提交为 GSR `0db5ed0`。用户明确 Decode 可以后续回头优化，当前先转向 Pack，见 [Pack 执行路径](../2026-10-17-gosproto-pack-execution/RESULTS.md)；本报告不改写原始实验结论。

## 预定门槛

在计时前明确的规则见 [plan.json](plan.json)：

1. 微基准只用于确认入口收益并选择一个候选；不能凭微基准或内联成功直接采用。
2. 五轮完整对照中，大型 Decode 中位数至少快 3%、PDecode 至少快 2%，两项各至少四轮方向一致。
3. 分配不增加，现有 wire、预算、presence、布局回落、写屏障和独占规则保持。
4. 达不到门槛就撤回本候选并冻结，不追加候选或改变门槛。

这里用同场前后中位数计算改变量，同时保留逐轮方向；不从不同组拼最佳值。

## 单独测量入口

在 `a09fb00` 的独立归档副本中加入仅供实验的包内 benchmark。完整入口、此前三种写法，以及新增的分离写法同进程共存；额外三项分解类型查询和地址取得。三种入口状态为原生结构、无原生整数指针、实际类型不匹配。所有即时类型判断都在计时循环内执行，不预先计算 exactNative，不缓存消息地址或跨调用事实。

Apple M2、Go 1.24.13 darwin/arm64、GOMAXPROCS=1；每项固定 200 万次、五轮，反转候选先后。共 18 项、90 样本，全部零分配。循环向共同的全局 sink 保存结果，因此这些值含相同的 sink 成本，不当作生产入口的绝对耗时。源码和完整样本见 [entry_benchmark.go.txt](entry_benchmark.go.txt)、[micro.json](micro.json)。

ns/op，中位数：

| 状态 | 当前入口 | 传入 record | 内部 Elem().Type() | record.Addr() | 即时核对与借用分离 |
| --- | ---: | ---: | ---: | ---: | ---: |
| 原生整数结构 | 8.079 | 5.537 | 7.404 | 11.090 | 4.988 |
| 不含原生整数指针 | 2.425 | 2.395 | 2.400 | 2.404 | 1.106 |
| 实际类型不匹配 | 6.301 | 3.820 | 4.763 | 3.693 | 3.335 |

新增 split 写法把同一对象的即时类型证明放到 parser 调用点，地址取得仍留在私有 helper：

```go
record := value.Elem()
exactNative := schema.nativeInt64Pointer && record.Type() == schema.typ
pointer := messageDecodePointer(value, exactNative)
```

helper 在 exactNative 为 false 时返回空能力，否则仍通过 reflect.Value.UnsafePointer 取得地址；每字段写入的 native/owner/Wire/Array 检查和 Go 类型化赋值完全不改。没有省去布局检查，也没有新增 raw pointer 写法、缓存、池或公共 API。

split 的原生入口从 8.079 → 4.988 ns（约 -38.3%），三个状态的能力结果与基线一致。原生类型检查约 5.184 ns，复用 record 的类型检查约 2.590 ns，单独地址取得约 2.582 ns；分项不能相加或从整体中相减推导提速。编译器确认新 helper 成本 67、会内联，而旧 helper 成本 216、不内联，见 [compiler.json](compiler.json)。

## 只让胜出候选进入完整消息验证

另从同一基线归档建立真实 candidate，只修改 decode.go 的调用点与 decode_pointer.go 的 helper 签名/条件。微基准源码不进入这个副本。沿用固定五组代表性消息、四档 Echo 和 35 种 Unpack 控制，129 项 × 五轮 × 每项 60000 次，共 645 样本，前后顺序按轮反转；PB 为同场控制。

µs/op，中位数，PDecode 包含解包：

| 消息 | Decode 前 → 后 | 变化 | PDecode 前 → 后 | 变化 |
| --- | ---: | ---: | ---: | ---: |
| AddressBook | 1.0030 → 0.9840 | -1.89% | 1.2030 → 1.1630 | -3.33% |
| 小型游戏 | 1.6030 → 1.5750 | -1.75% | 2.1130 → 2.0840 | -1.37% |
| 大型游戏 | 15.7730 → 15.4180 | -2.25% | 20.7260 → 20.4610 | -1.28% |
| 整数数组 | 2.2220 → 2.2330 | +0.50% | 7.6270 → 7.6470 | +0.26% |
| 4 KiB ASCII | 0.6245 → 0.6215 | -0.48% | 1.1690 → 1.1580 | -0.94% |

大型逐轮方向：

| 轮次 | Decode | PDecode |
| --- | ---: | ---: |
| 1 | -5.34% | -1.28% |
| 2 | -2.62% | -1.95% |
| 3 | -2.19% | -1.09% |
| 4 | -1.09% | -2.96% |
| 5 | -2.28% | -1.27% |

两项均五轮更快，字节数与分配次数不变；但总体中位数只改善 **2.25% / 1.28%**，未达到预定 **3% / 2%** 门槛。不是“没有收益”，而是这次有限实验没有达到值得改动冻结基线的标准。不得因为结果接近而事后降低门槛。

[split.json](split.json) / [raw](raw/) 保存全部样本；[gate.json](gate.json) 保存门槛计算与拒绝结果。所有 SP 项的分配与同场基线一致。未改的 Encode/Pack/Unpack/meta/RPC/TS 保持固定；普通 Unpack 控制中的差异不计作本轮算法收益。候选未采用，不据此替换已采用实现的性能基线，也不产生新的 PB/SP 排名。

## 冻结决定

冻结基线为独立库 `a09fb00`，其中 Decode 生产源码与 `fc3246a` 一致，保留通用 parser、受控原生整数写入和完整约束。split 只留在隔离实验与补丁，不进入正式库工作分支。

这是阶段性冻结：当前暂停 Decode 入口、解析、字段构造/写入及其性能绑定的微调，后续可以回头继续优化，不作为永久限制。正确性缺陷修复不受冻结阻挡。若共享 schema 因其他任务变化，仍须保持这些解码契约，不借机调整热路径。

Pack/Unpack 的独立优化不属于这次冻结范围，但后续应将冻结 Decode 作为控制。后续回到 Decode 时，以本阶段基线和已有实验为对照，另定当轮范围与验证，不把当前冻结当作禁止优化的长期规则。

公开 API、wire、错误优先级、资源预算、presence、独占与 GC 规则没有变化。当前只是工作分支的性能阶段收尾，不是升级 GSR 依赖或发布新版本。

## 验证范围与复现

[validation.json](validation.json) 区分实际执行与未执行：每轮微基准运行 TestEntryCapabilities，完整配对每轮运行 TestRepresentativeFixtures；微基准与 split 的 prepare-only 副本均重建出精确的完整 Go、harness 和夹具指纹，所有计时源码在采样期间固定。正式独立库所有文件仍与 `a09fb00` 一致。

没有采用生产修改，因此未重复完整库/race/checkptr/官方对照/fuzz、GSR/GoTS 或 TS 性能。此前已采用代码的验收见 [受控写入验收](../2026-10-17-gosproto-decode-store/RESULTS.md)，没有把历史执行当作本轮重跑；候选也没有被宣称通过这些完整验收。

```sh
python3 docs/benchmarks/2026-10-17-gosproto-decode-freeze/reproduce-entry.py \
  --library /path/to/gosproto --output /tmp/gsr-entry-micro

python3 docs/benchmarks/2026-10-17-gosproto-decode-freeze/reproduce-pairs.py \
  --library /path/to/gosproto --output /tmp/gsr-entry-pairs
```

两者支持 --prepare-only，只操作新输出目录的归档副本。微基准默认五轮 18 项，完整对照默认五轮 129 项。命令和指纹足以重建输入，不保证相同 ns/op。门槛独立记录，不根据复现实测自动修改库或解除冻结。
