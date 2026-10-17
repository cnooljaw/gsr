# Go 编码配对采样工具

用于校准局部优化的计时分辨能力，再复测候选。只在输出目录的归档副本中构建；不修改库工作树、GSR 正式依赖或发布版本，不代替 Go/TS 四实现完整 Benchmark。

先做两类控制：

```sh
python3 examples/websocket-gate/benchmarks/tools/go-pairs/run.py \
  --library /path/to/gosproto --baseline bba3326 \
  --mode self --output /tmp/gsr-pairs-self

python3 examples/websocket-gate/benchmarks/tools/go-pairs/run.py \
  --library /path/to/gosproto --baseline bba3326 \
  --mode clone --output /tmp/gsr-pairs-clone
```

- `self`：A/B 直接复用同一个函数值和输入，检查计时与轮次偏差。
- `clone`：两份完全相同的源码，只替换其中一份 module 路径，同进程比较；保留代码位置等差异。
- `compare`：基线与应用指定补丁后的归档副本比较。必须传 `--candidate-patch /absolute/candidate.patch`；self/clone 禁止补丁，避免把不同实现误当作 A/A。

可以精确筛选诊断项目，并显式选择 40 ms 块；默认仍为 10 ms：

```sh
python3 examples/websocket-gate/benchmarks/tools/go-pairs/run.py \
  --library /path/to/gosproto --baseline bba3326 --mode self \
  --block-ms 40 --case echo_ascii4k/sproto_pack --case ascii32/frame \
  --output /tmp/gsr-pairs-focused
```

`--case` 可重复，名称必须精确匹配，未知或重复名称拒绝；采样保留原矩阵顺序。较长的块不保证更稳定，工具不会为过门自动重试或调整门槛。

输出目录必须不存在。`--prepare-only` 只生成模块和输入清单，不测试或计时。使用 Go 1.24+、Python 3.12+ 和系统 patch；首次构建需要可用的 Go 模块依赖。复用项目中固定的 PB v1.36.1 与现有夹具，不重新生成协议类型。

## 采样与判定

复用五组业务消息的 Encode/Pack/PEncode 和四档 Echo 完整帧，默认共 19 项，筛选时只计时指定项目；计时前检查固定 wire 与原夹具语义。每项先交错预热 50 ms，再分别校准并选取同一调用次数，目标每块约 10 ms 或显式指定的 40 ms。

每轮四组 ABBA/BAAB，首发一方在轮内均衡；六轮共 96 个计时块。计时块之间不强制 GC，不关闭 GC；每项预热前显式 GC 一次。所有输出仍正常分配并写入原 sink，不加入复用池。计时区间外用 runtime/metrics 记录每块前后完成 GC 周期之差，分配窗口也记录 NumGC 差；周期计数不是暂停时长，不能换算为 GC 耗时。所有含 GC 的块都保留，不按 GC 筛掉样本。显式关闭 GC 的环境被拒绝，原始结果记录实际 GOGC。

每组按实际调用数归一化，在对数域配对；每轮对四组取几何平均，最终取六轮比值的中位数及 IQR。保留全部块和全部轮次，不丢弃离群值，也不将两边独立中位数相除。IQR 是离散程度描述，不是置信区间。

控制门槛在采样前写入 inputs.json：中位比值距 1 不超过 2%、六轮比值 IQR 不超过 3 个百分点、至少五轮距 1 不超过 5%，分配统计中位数相同。逐项标记 controlAccepted，并单列 timingAccepted；后者只描述计时门槛，不豁免分配差值，总控制仍要求两者同时满足。不要求通过的项目替失败项目背书。分配统计在计时外另采三次、每次 128 调用，保留字节/对象原始计数；极小差值也会保守地拒绝控制，不将其直接认定为实现增加了分配。

同一个项目总控制通过 self 和 clone 后，才有条件解释 compare 的小幅差值；compare 自身若波动明显也不能确认收益。候选是否保留仍按事先规定的业务收益、回退、正确性和资源门槛判断。工具不自动批准候选；compare 的 controlAccepted 为 null。

## 输出与验证

- `inputs.json`：提交、工具/Go 源码/夹具指纹、参数和控制门槛。
- `commands.json`：构建、统计/夹具测试和采样命令及完整输出。
- `raw.json`：全部计时块、轮次、校准次数和独立分配样本。
- `summary.json`：逐项配对中位数、IQR、控制状态；应与 raw 一起阅读。

命令退出成功只表示采样完整，控制是否通过须读取 summary.json。采样失败时目录保留已有原始数据，不能当作完整结果。采样前后核对两份库与 harness 的 Go 源码指纹；不要在采样期间修改它们。不同模式、旧方法与新方法的数据分别解释，不拼接最好值。

工具先运行生成模块中的统计用例和原夹具测试。Go 模板以 `.txt` 保存，仅在独立模块中编译，不加入默认 `go test ./...`；采样入口必须由脚本显式启动。

Python 判定用例：

```sh
python3 -B -m unittest discover \
  -s examples/websocket-gate/benchmarks/tools/go-pairs -p test_run.py -v
```

首轮校准和原候选复测见 [校准记录](../../../../../docs/benchmarks/2026-10-17-gosproto-pair-calibration/RESULTS.md)。

后续 [GC 边界探针与 40 ms 诊断](../../../../../docs/benchmarks/2026-10-17-gosproto-pair-resolution/RESULTS.md) 验证了 tiny 分配可以产生 16 B 的窗口差值；40 ms 聚焦实验未改善全部控制，默认和总门槛均未调整。
