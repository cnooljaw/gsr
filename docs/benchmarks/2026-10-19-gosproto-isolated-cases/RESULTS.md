# Go Sproto 原70项逐项独立进程控制

日期：2026-10-19。库基线 `7397804`，GSR 基线 `a125ab4`。只读诊断，生产代码、正式依赖 v0.4.0 与冻结的 Decode 均不变。此前 profile / GC 上下文诊断已提交为 GSR `a125ab4`，未推送。

## 采样方法

[固定计划](plan.json) 沿用原70项输入、内容和顺序：19个 body/手工帧入口、19个真实 RPC 入口、5个 wide64 入口、9种增长形态×3入口。self 与 clone 各70项，每项启动一个新进程，共140个进程；所有构建和检查在计时前完成。每个 mode/case 只跑一次，不重跑失败控制。

每个进程仍完整构造原70项并初始化 schema cache，然后精确筛选单项到独立切片，清空 `encodedSink`，在预热前 GC；其他操作闭包不再被所选切片持有，schema cache 继续保留。这不是绕过初始化或清空 schema cache。原有 `sampleBlock` 与有界校准 `blockCalls` 源码逐字相同；仍交错预热50ms、目标块10ms、6轮×4 quartet、ABBA/BAAB交错，计时块之间不强制 GC。分配仍是每侧3×128次，原始尾差全部保存。

只在隔离归档里增加4个运行时状态快照：GC后预热前、校准后、计时后、分配测量后。快照保存在独立结果字段，不进入计时块或分配测量区间；它本身会多保留少量诊断状态。采样工具默认实现、库源码和基准输入未改。新增结果结构也可能改变编译布局，所以不能把和旧二进制的绝对耗时变化全部归于隔离。

Go 1.24.13 / darwin arm64 / Apple M2，`GOMAXPROCS=1`，默认 `GOGC=100`、`GOMEMLIMIT=MaxInt64`（无软上限）；每项四次快照均校验配置。堆值是离散采样值，**不是峰值**。limiter 的非零值表示曾启用的最近 GC cycle；仅能按快照区间定位变化，不能回填具体计时块或历史结果。

## 复现与原始证据

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-isolated-cases/reproduce.py \
  --library /path/to/gosproto \
  --output tmp/gosproto-isolated-cases/new-run
python3 -B docs/benchmarks/2026-10-19-gosproto-isolated-cases/analyze.py \
  tmp/gosproto-isolated-cases/new-run
```

驱动引用已提交的上一轮夹具，不重复复制库源码。完整逐项 JSON 保存在忽略的 `tmp/gosproto-isolated-cases/run`，Git 中保存无删减的同序合并原始记录与每个逐项文件的 SHA256、命令日志、源/二进制指纹和独立复算。`--prepare-only` 只生成归档，不采样；fresh 归档用于核对输入可复现性。

## 结果

140个进程各一次，共13,440个计时块、3,360个quartet、840个配对轮、560个状态快照。独立分析从每个块的调用数和纳秒重算配对、轮值、累计ns/调用、分配中位数与控制标记，和工具结果在1e−12容差内一致；没有丢弃GC块或离群值。

控制标准仍是描述性诊断：中位差绝对值≤2%、轮比值IQR≤3pp、至少5/6轮在±5%内，且分配中位数相同；不用于设置优化最低收益门槛。

| 指标 | self | clone |
|---|---:|---:|
| 本轮控制通过 | 61/70 | 61/70 |
| 旧混合环境控制通过 | 40/70 | 38/70 |
| 旧失败、本轮通过 | 27 | 25 |
| 旧通过、本轮失败 | 6 | 2 |
| 本轮分配中位数相同 | 70/70 | 70/70 |
| 中位差绝对值的跨项中位数 | 0.395% | 0.495% |
| IQR的跨项中位数 | 1.252pp | 1.056pp |
| 初始 limiter 非零的进程 | 0 | 0 |
| 任意观测区间 limiter 变化的进程 | 0 | 0 |
| 初始堆最大采样值 | 494,256 B | 494,112 B |
| 四阶段堆最大采样值 | 4,190,760 B | 4,197,456 B |

两组同时通过53/70。旧数据来自已提交的[连续索引报告](../2026-10-18-gosproto-layout-index/RESULTS.md)，本轮没有重跑混合环境；它与本轮的进程生命周期、存活对象和二进制布局不同，表中的通过数变化不是codec性能加速，也不能只据此量化隔离的因果贡献。跨项IQR中位数此前为self2.280pp、clone2.619pp。

### 重点入口

下表均为相同实现的控制，差值是B/A中位差；正负不能解释为候选收益。

| 原70项名称 | self差值 / IQR | clone差值 / IQR | 控制 self / clone |
|---|---:|---:|---|
| `body/game_large/sproto_encode` | +0.485% / 1.116pp | +1.569% / 6.717pp | 通过 / 未通过 |
| `body/game_large/sproto_pencode` | +0.398% / 1.269pp | +0.692% / 1.020pp | 通过 / 通过 |
| `body/int32_edges/sproto_pack` | -0.827% / 1.104pp | -0.653% / 2.258pp | 通过 / 通过 |
| `body/int32_edges/sproto_pencode` | -0.155% / 1.062pp | +1.032% / 1.130pp | 通过 / 通过 |
| `body/echo_ascii4k/sproto_pack` | -0.901% / 2.172pp | +0.694% / 1.341pp | 通过 / 通过 |
| `body/zeros4k/frame` | +0.537% / 2.814pp | +0.800% / 1.394pp | 通过 / 通过 |
| `rpc/game_large/request_cancel` | -0.992% / 1.508pp | +0.784% / 0.839pp | 通过 / 通过 |

4KiB纯Pack和zero手工帧本轮两组通过；大型Encode的clone仍因6.717pp IQR失败。self最大单轮离群为AddressBook Pack **+71.34%**，clone最大单轮离群为4KiB ASCII真实响应 **+33.02%**；后者的总体控制仍通过，不能只看通过标记。完整轮值和所有失败见[独立分析](analysis.json)，不删样本、不从实现比较中减去控制中位数。

### 全部失败控制

self（9项）：

- `body/addressbook/sproto_encode`
- `body/addressbook/sproto_pack`
- `body/game_small/sproto_pencode`
- `rpc/game_small/response`
- `rpc/int32_edges/notification`
- `rpc/ascii32/notification`
- `rpc/utf8_1k/notification`
- `growth/flat/half/4096/pencode`
- `growth/nested/32x128/pencode`

clone（9项）：

- `body/game_large/sproto_encode`
- `body/utf8_1k/frame`
- `rpc/game_small/notification`
- `rpc/echo_ascii4k/notification`
- `rpc/wide64/notification`
- `growth/flat/full/512/request_cancel`
- `growth/nested/32x32/pencode`
- `growth/nested/32x32/request_cancel`
- `growth/nested/32x128/pencode`

## 判断与下一步

本轮140个进程的四次观测均显示 limiter 为零，未观察到上一轮补充full进程的 limiter 激活；初始堆约0.26～0.49MB，未再采到数百MB堆。采样时刻有限，不能宣称已测到精确峰值或完全消除调度、GC相位和系统噪声。仍有17个不同入口至少一组失败，所有异常继续作为不确定性披露。

后续实现比较采用逐项独立进程作为受控补充，保留原70项与混合环境历史，不再为等控制通过而调参/重跑。下一轮只实验**普通word的连续非零段批量写出**：整数数组该形态多，大型结构少，需要同时看纯Pack、PEncode和真实RPC，并观察小包与非目标成本；不按具体消息mask硬编码，不改FF规则、ownership、cap、预算或冻结的Decode。不设固定最低收益比例，按可靠的累计纳秒、分配与复杂度评估。

## 验证范围

两组归档的统计、精确筛选、GC观测、代表性fixture与新增运行时指标检查通过，harness `go vet` 通过。新增的指标检查验证读取与默认配置，列表入口补齐原70项接线/顺序校验；不重复添加已有统计/筛选测试。原计时块/校准函数源码逐字核对；fresh prepare-only归档的库/夹具/生成输入/harness指纹一致；140个逐项原始文件与合并记录逐项对齐。命令总耗时约162.6s，其中140个独立采样进程约157.7s，每项均在15s进程预算内完成。

生产源码仍等于库7397804且工作树干净，GSR仅新增诊断文档/夹具。未重复库全套/官方oracle/race/checkptr/fuzz或GSR/GoTS集成，也未运行新PB/TS排名：本轮没有生产改动。这些限制与全部指纹写入[验收记录](validation.json)。新GSR诊断留待审查，未提交。
