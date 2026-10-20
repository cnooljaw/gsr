# 当前生产 Unpack 阶段成本诊断

2026-10-19。word预算报告已本地提交GSR `8e9880f`，未推送。本轮只读采当前生产gosproto `af9169d`，没有生产候选或配对耗时。**主库仍干净，诊断与文档未提交**；依赖v0.4.0、Decode继续开放。

## 采集与归属方法

沿用70项构造/wire/结果/消费长度检查，选8个入口，每项一个独立进程，手动CPU约2秒，清sink/GC/50ms预热后启动，始终新建独占结果。GOMAXPROCS1、GOGC100、无限GOMEMLIMIT；不为测量拆分生产函数、不添加计时器、不改变扫描或恢复源码。归档两副本同为af9169d，只执行显式B侧的canonical模块，不把同源码A/B当成新性能比较。

[独立复算](analyze.py)直接解析pprof raw的整数样本及完整inline帧：从叶向根遇到第一个UnpackWithLimit，调用行193～233记预检、234记分配、235～273记恢复；没有该帧记other。每个样本只计一次，所有组之和等于原始完整CPU样本数；保留runtime子调用在其调用阶段的归属，systemstack中丢失业务父帧的样本留other。不会把父子累计相加或剔除kevent后归一化。

**这些是原函数的调用行归属，不是独立阶段计时，也不是具体机器指令或硬件瓶颈的证明。**每项只有约219～238个10ms采样权重，存在量化/采样偏差；不能把份额直接乘到别的benchmark，或据此给出提速上限。PDecode的other还包含正文Decode。

## 完整样本中的阶段分布

| 入口 | 预检 | 分配及可追溯子调用 | 恢复 | other | 样本数 |
| --- | ---: | ---: | ---: | ---: | ---: |
| body/game_large/unpack | 30.40% | 6.61% | 32.16% | 30.84% | 227 |
| body/game_large/pdecode | 6.03% | 1.29% | 10.34% | 82.33% | 232 |
| body/int32_edges/unpack | 35.32% | 7.23% | 25.96% | 31.49% | 235 |
| array/width8_len896/unpack | 24.56% | 8.33% | 26.75% | 40.35% | 228 |
| shape/nonzero2x256/unpack | 14.16% | 10.05% | 18.72% | 57.08% | 219 |
| shape/FF512-sparse4096/unpack | 32.35% | 9.66% | 23.11% | 34.87% | 238 |
| shape/zeroRun8x64/unpack | 9.69% | 14.54% | 13.22% | 62.56% | 227 |
| shape/ascii4096/unpack | 0.00% | 21.27% | 5.88% | 72.85% | 221 |

大型纯Unpack的预检与恢复同量级；两种大整数数组的预检也有较多样本，因此不存在“预算扫描成本可以忽略”的证据。但此前改游标、拆函数、改word预算的取舍并未因此消失，不能用这张表追认未采用方案。

普通恢复的源读取/目标写入共同行260，在大型纯Unpack完整样本占19.38%，在两种数组约13%，FF接稀疏约8.40%。这行同时含地址计算、读写及边界检查，不能把整行样本都归因一次store或某个bounds check。零段/ASCII不执行这行，样本为零，与实际路径一致。短零段分配归属14.54%、ASCII分配21.27%；不能因其预检少就称整个API已无成本。

大型PDecode里Unpack累计17.67%、正文DecodeWithLimits累计53.88%、decodeMessage累计53.45%、decodeStructSlice累计51.29%。后三项有父子重叠，**不能相加**；正文仍是值得继续投入的方向。reflect.New累计9.05%、runtime.mallocgc累计22.41%，同样存在嵌套，不据此许诺删除所有对象。独占、presence、失败原子性与真实对象身份继续保留。

## 下一项可验证工作

不再排列预检计数器写法，也不因目标视图曾与源游标组合实验过，就直接重做同一候选。当前大型PDecode的countChunks累计4.31%，不是最大热点，但范围小、容易隔离。它只需要验证每个chunk边界并计数；当前调用readChunk时丢弃content。

采样完成后额外核对[countChunks机器码](count-chunks-assembly.txt)：readChunk已经inline，但decode.go:35的content切片cap守卫及panicSliceAcap分支仍在。它不证明守卫就是4.31%的全部成本，却提供了一个明确的数据流候选：**只在countChunks内读取4字节长度并校验完整边界，不构造不使用的content视图**。保留先校验当前chunk再budget.add(1)的顺序，逐元素预算扣减、完整预检先于数组分配、错误class、32/64位长度安全、所有权和失败原子性不变；其他真正需要content的调用继续readChunk。不泛化新接口或helper，不缓存元信息，不减少必要检查。

这是下一候选的方案，**本轮尚未实施、测量或采用**。下一轮先以现有未知/截断chunk、数组预算与失败原子性用例验证，并核对编译器是否实际移除冗余视图，再按真实Decode/PDecode/Dispatch和大小数组测量。不能因静态分支减少就声称提速，也不设最低收益比例。

## 验证与限制

55条profile流程命令成功，外加1条采样后的静态objdump命令；harness统计/筛选/GC/代表夹具/默认指标测试、vet、完整70清单/构造及构建在采样之前完成。8个CPU进程、24快照，limiter全零，最大堆快照4,036,672B（非峰值）。无allocation profile；loop分配计数见[loop-metrics](loop-metrics.json)，不混入构造期对象或当作alloc_objects采样。

fresh/18份源码均等af9169d、工具/生成输入/夹具/harness/二进制/raw指纹、canonical B侧与loop调用数均核对。profile ns/op原样保留，但不是新A/B或PB/TS基线；不剔除GC、kevent、调度样本或重采择优。主库未改，不重复完整库/oracle/race/fuzz或GSR/GoTS验收。

冻结plan沿用了旧报告几段40项/旧基线/未用allocation配方的描述；实际cases和机器核对为70、baselineCommit与18份源码为af9169d、只执行8项CPU。未静默改写采样时plan，逐字段更正见[scope-clarifications](scope-clarifications.json)，实际命令/inputs优先。准备时首次选到已有历史目录，mkdir拒绝后改用新目录；原历史文件和采样未被覆盖。

正式go.mod/go.sum保持HEAD，未推送、发布或tag。[源码参考](source-references.json)记录只读Skynet内Sproto C和固定Protobuf v1.36.1路径/哈希；前者采用FF复制/普通逐位恢复，后者展示显式边界与消费长度；不同wire和Go所有权规则不直接等同。

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-unpack-phase-profile/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-unpack-phase-profile-new
python3 -B docs/benchmarks/2026-10-19-gosproto-unpack-phase-profile/analyze.py \
  tmp/gosproto-unpack-phase-profile-new
```

输出目录必须不存在。原二进制和profile在忽略tmp目录；pprof完整raw文本、来源、复算和机器码随报告保留。
