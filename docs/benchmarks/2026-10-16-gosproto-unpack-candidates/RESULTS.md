# gosproto 普通 Unpack 候选筛选

2026-10-16，上一轮零段解包已本地提交为 gosproto `22e287e`、GSR `1af93f4`，提交后工作树干净，未推送。当前五个候选全部撤回，独立库全部 Go 生产源码与 `22e287e` 逐字节相同；仅保留 TestPack 的独立预期字节回归。该切片随后已本地提交为 gosproto `d66644a`、GSR `acfb070`，补丁格式属性修正为 `ae2e1bc`，未发布，正式依赖仍为 v0.4.0。后续见 [Decode 诊断与安全 inline](../2026-10-17-gosproto-decode-analysis/RESULTS.md)。

## 结论与范围

普通 Unpack 仍值得关注，但本轮没有找到值得保留的替代执行方式。位置表、计数表、游标和固定 word 区间在某些合成形态上更快，真实 PDecode 收益有限或暴露其他稀疏形态回退。保持当前位扫描，不把失败候选作为已完成的优化。

本轮沿用 [零段专项](../2026-10-16-gosproto-zero-unpack/RESULTS.md) 的同一组夹具：五种代表性消息 Unpack/PDecode/冻结 Decode 与 PB 控制，四档 Echo payload/frame Decode，35 种 Unpack 形态。每个候选 129 项 × 三轮 × 固定 60000 次，共 387 个样本；五个候选共 1935 样本。GOMAXPROCS=1，按轮反转同进程旧/新/PB 的顺序；Apple M2、Go 1.24.13 darwin/arm64、PB v1.36.1。

归档 `22e287e` 仅更改基线 module 名称以同进程加载。候选与基线消费相同 wire；Pack、Encoder、Decoder、RPC、元信息、limits 与 TS 生产源码冻结。完整预检、输出预算、错误优先级、独占输出及分配策略没有改为另一种模型。本轮未进入五轮正式验收，不把三轮筛选与上一轮正式结果混用。

## 诊断与候选

[前轮 profile](../2026-10-16-gosproto-zero-unpack/profiles.json) 指向逐位恢复、源切片推进和 mask 计数。已对照只读 Skynet/固定官方 Sproto 的 sproto_unpack：FF 段批量复制，普通 mask 逐位恢复；当前 Go 已保留 FF 和零段快路径，并在独占输出分配前完整预检。官方 C 的调用方缓冲和指针循环不能直接当作 Go 的相同性能模型。

1. `table`：初始化只读 1 KiB 表，用高八位保存有效字节数，低二十四位以三位一组保存目标位置；预检计数和恢复均查询它。没有消息缓存或新分配。
2. `count`：仅使用 256 B 计数表替换预检 popcount，恢复仍为基线位扫描，用于隔离查表的收益。
3. `cursor`：保留原预检，每个普通 word 固定八字节输出区间，逐位读取源索引，每个 word 完成后再推进源切片。
4. `word`：只固定输出区间，源仍逐字节推进，用于隔离上一个候选的两种变化。
5. `sparse`：在 cursor 上增加单字节 mask 直接恢复，检查能否消除最稀疏形态的代价。随后暴露双字节形态回退，未继续增加更多稀疏分支。

三轮各自中位数相对同组基线的耗时变化（负数更快）：

| 候选 | 大型 Unpack | 整数数组 Unpack | 大型 PDecode | 整数数组 PDecode | 撤回证据 |
| --- | ---: | ---: | ---: | ---: | --- |
| 1 KiB 位置/计数表 | +6.9% | +10.3% | +1.9% | +6.9% | 单字节形态 +37.5%，AddressBook Unpack +17.2% |
| 256 B 计数表 | +0.6% | -0.4% | -0.9% | -2.9% | AddressBook Unpack +12.4%，真实组合没有稳定收益 |
| 普通 word 输出区间 + 源游标 | -5.8% | -5.6% | +0.8% | -4.1% | 单字节形态 +12.0%，大型组合未改善 |
| 只固定输出区间 | +4.8% | +0.2% | -0.7% | +0.5% | 单字节 +24.4%、双字节 +10.9%、AddressBook +22.0% |
| 游标 + 单字节直接恢复 | -3.0% | -3.2% | +0.1% | -1.6% | 双字节 +14.1%，大型组合基本持平 |

这些是各候选相对其同场基线的结果，不能跨组拼接最佳中位数。被冻结的 Decode 控制也有采样波动，不把每项小幅差异都归因于某个循环改动。

例如 table 的七非零字节形态约快 23.7%，cursor 的四非零字节形态约快 14.1%，但这不足以抵消稀疏回退、组合收益不足和新增表/分支的维护成本。sparse 还出现零形态数纳秒级波动，原值一并保留，没有因该路径未改就删除数据。

## 后续热点的独立复核

候选撤回后，用生产源码与当前相同的 baseline.test 单独采集大型/AddressBook 纯 Decode、整数数组 PDecode 的 CPU 和分配 profile，见 [decode-profiles.json](decode-profiles.json)。它们在计时筛选之后运行，不把带 profile 的 ns/op 当成新排名。CPU 原始样本有大量 runtime.kevent，不将其归入编解码热点或用总占比推算可提速比例。

大型纯 Decode 仍为 10416 B/op、532 allocs/op；采样对象数主要归于 decodeInt64Pointer（约 59%）与 reflect.New（约 38%），分配字节则分别约 31% / 31%。这是分配采样的归属，不能等同于精确对象数量或承诺可全部省掉。源码中原生 *int64、结构数组中的独立对象、失败原子性临时根对象都有真实所有权意义。

AddressBook 纯 Decode 为 528 B/op、29 allocs/op，字符串指针与内容复制、结构体构造及切片仍可见。整数数组 PDecode 只有 5 次分配，Unpack 与整数切片占据主要字节；其 CPU 仍集中在普通 Unpack 和数组解码。两类问题应分别评估。

下一轮优先用隔离探针区分大型 Decode 的对象构造、reflect.Set 和 wire 遍历成本，再选择可验证的局部实现；普通 Unpack 保持当前位扫描，不继续根据某一种稀疏形态追加修补分支。独占输出、optional presence、失败原子性和资源预算继续作为前提。

## 保留的验证增量

扩展既有 TestPack：每种 mask 使用不同的高位字节直接构造 wire，后接独立的 0x81 word，再枚举八个源切片偏移，共新增 2048 组。旧的同值字节用例难以单独暴露有效位次序交换；新增后继 word 则让消费长度偏差也直接可见。预期结果由 mask 和位位置独立构造，不依赖 Pack/Unpack 自身往返。

新增回归先在未改生产代码时通过；table 候选也通过 TestPack/TestUnpackOutputBudget，但随后因性能撤回。恢复后已核对全部生产 Go 文件与提交逐字节相同，保留测试与首次运行版本相同；未重复全仓/race/vet/fuzz 或 GSR 集成，详见 [validation.json](validation.json)。这些未采用候选没有新的生产正确性验收结论。

## 复现

```sh
python3 docs/benchmarks/2026-10-16-gosproto-unpack-candidates/reproduce-pairs.py \
  --library /absolute/gosproto \
  --candidate cursor \
  --output tmp/gosproto-unpack-cursor-reproduce
```

需要含 `22e287e` 的独立库、Go 1.24、Python、git、gofmt、patch。输出目录必须不存在。脚本从归档建立独立基线和候选副本，仅在新输出目录应用测试与指定候选补丁，不改变传入库的工作树；可用 --prepare-only 检查构建输入。table 的复现副本已逐字节核对全部 Go 源码、七份 Go harness 和三份 JSON，均与实测指纹一致。

候选补丁见 [patches](patches)，来源与保留源码指纹见 [inputs.json](inputs.json)，逐样本数据见 [raw](raw) 及五份 JSON。二进制和 profile 留在忽略目录 tmp/gosproto-mask-unpack/。本轮是失败候选记录，不替换前轮正式基线，也不产生新的 PB/SP 总体排名。
