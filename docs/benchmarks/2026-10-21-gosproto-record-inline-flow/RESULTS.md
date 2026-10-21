# 整数记录写出：inline 路径直接继续

基线为 gosproto `451998ea0dcf6262bdd4ca0ab6c686d0fb738fac`，使用完整 Git 归档，避免恢复实际库七项无关删除。正式依赖仍为 v0.4.0。

## 候选

仅改变 encodeIntegerRecordInto 的字段写出控制流：inline 整数直接写 header 后继续下一字段；非 inline 分支单独生成内容、写入零 header 和内容长度。原逻辑中的 header/length/body 合流状态，以及写完后 `header == 0` 的判断不再需要。previous 更新保持在下一字段处理之前；跳 tag、nil/presence、字段顺序、输出长度和布局计数不变。

由 `bits > MaxEmbeddedInt` 已确认 encodeIntegerInto 返回 header 为零，因此外置分支直接写零 header 等价。没有缓存字段值、增加元信息或移除预检/预算，没有新 unsafe 或公开 API。不是对安全检查换速度。

隔离 arm64 二进制中记录写出符号从 196 条指令降到 188；这只是局部静态证据，不当作整个 Encode 耗时的比例。

## 测量与正确性

采用同二进制静态调用树，baseline/clone/candidate 各自有独立的顶层、Buffer、Message、Field、Record、Array 入口。选择只发生在最外层，不在 encodeLayout 指针传递处引入函数选择器。候选树正常入口分配 parity 单独检查；源码重命名可逆、gofmt 后还原亦独立核对。

真实 game_small/game_large/int32_edges，以及短 records7、跨布局 prefix31，均测 Encode/PEncode。每项 self、clone、pass1、pass2；每进程六轮，每轮四组 ABBA/BAAB，双侧各预热50ms，基线256次校准目标20ms，两侧等调用数。GOMAXPROCS=1、GOGC=100、GOMEMLIMIT=off。全部 GC/长尾保留，不扣控制，没有摆放轮换。

复用现有三模式整数记录契约与外部契约，覆盖布局边界、nil、相邻数组、字段描述替换/回退、预算、tag、未知字段等；开启 race/checkptr。harness 验证真实 fixture、正常分配 parity、全部10项预检 wire/cap，执行 vet/build。没有为本次无新增行为的局部改写另造重复夹具。源码、fixture、生成文件和二进制哈希在计时后复核。

复现：

```sh
python3 docs/benchmarks/2026-10-21-gosproto-record-inline-flow/run.py --library /private/tmp/gsr-gosproto-fork --output tmp/新的空目录
python3 docs/benchmarks/2026-10-21-gosproto-record-inline-flow/analyze.py --output tmp/新的空目录
```

脚本固定导出上述基线；输出目录须不存在，分析会把新样本写入本报告目录，复测前另存旧记录。

## 实测结果及保留决定

| 场景 | 第一遍中位/累计 | 第二遍中位/累计 |
|---|---:|---:|
| 小型 Encode | −0.42% / −2.40% | −0.21% / −3.48% |
| 小型 PEncode | −0.59% / −0.53% | −0.84% / −3.17% |
| 大型 Encode | −0.02% / −0.43% | −0.81% / −1.96% |
| 大型 PEncode | −0.41% / +1.58% | −1.08% / +2.04% |
| 7项记录 Encode | −1.92% / −2.32% | −2.57% / −3.78% |
| 7项记录 PEncode | −2.32% / −7.99% | −1.83% / −5.97% |
| prefix31 Encode | −2.63% / −4.63% | −1.89% / −5.12% |
| prefix31 PEncode | −1.36% / −1.23% | −0.95% / −1.17% |

负数表示候选更快。决定保留这次局部控制流简化，不因为小收益低于5%而丢弃；已有两个短/边界记录场景的双遍改善、静态指令减少和正常入口契约验证支持。大型 Encode 改善很小，不能夸大；大型 PEncode 的两遍中位/累计均方向相反，未确认组合吞吐改善，完整保留。整数数组不经过这个记录路径，其中位仍有 +0.11%/+0.48% 等变化，作为实验噪声与布局影响的证据保留，不能把所有耗时差异归因于候选。

同源码 clone 的大型 Encode 中位 −0.48%、IQR1.95pp，prefix31 Encode −0.72%、IQR1.38pp；控制不相减，也没有宣称排除全部摆放影响。40进程、240轮、960四元组、3840块，60条实验流程全部通过。正常大型 Encode/PEncode 3968B/2alloc保持，计时中的微小遥测尾数仍在 analysis.json，不删GC或长尾。

## 正常入口生产验收

accept.py 从完整基线归档恢复候选源文件，去掉全部实验选择器，执行8条验收命令：gofmt、全库测试、全库 race/checkptr、vet、两项10秒 fuzz、GSR三个受影响 package、Go/TS集成。官方 oracle 使用冻结来源并核对 provenance 中的所有文件哈希。全部成功。

- Codec fuzz：432195次；Decode fuzz：156066次，6秒以后计数停增，11.272秒正常通过退出。记录停增，不把它写成持续满速运行，也没有盲目延长时间。
- Go/TS：7/7，通过Plain/MAC Echo及官方向量等检查；integration-build-info.txt确认实际替换到本次正常候选归档。
- 实际库只更新encode_integer_record.go及CHANGES.md，已提交为 `9048cfa`（未推送）；源码哈希与验收归档一致。没有新增接口/依赖、没有删除检查，也未改动七项无关删除。正式GSR依赖未升级。

完整日志见 acceptance.json。若复现生产验收，先完成实验分析，再运行 `python3 docs/benchmarks/2026-10-21-gosproto-record-inline-flow/accept.py tmp/新的验收目录`。
