# 当前整数数组 Unpack / PDecode CPU 诊断

2026-10-19。前轮整数数组读取视图已本地提交为库 `f7022ac`、GSR `b3d4517`，均未推送。本轮在该基线上只读采四入口CPU，生产源码不改；随后独立的[源游标实验](../2026-10-19-gosproto-unpack-source-cursor/RESULTS.md)另作配对验证。本页不产生A/B基线或PB/TS排名，Decode继续开放。

## 范围与方法

沿用52项的同一构造、wire/值/消费长度检查，精确选择4字节896数组Unpack/PDecode与8字节896数组Unpack/PDecode。每个入口独立进程、手动CPU约2秒，构造后只保活所选闭包，清三个sink、GC、预热50ms再启动profile；输出始终独占。不采allocation profile；下列分配是ResetTimer后的实际循环B/op/allocs，不是alloc_objects推算。

GOMAXPROCS1、GOGC100、无限GOMEMLIMIT；4进程、12个计时外快照，limiter均零。最大堆快照3,862,888B（约3.68MiB），不是峰值。profile有采样开销，下表只列诊断占比，原耗时保留于benchmark文本，不当作新的常规计时结果。

| 入口 | 循环B/op / allocs | UnpackWithLimit累计 | decodeIntSlice累计 | runtime.kevent flat |
| --- | ---: | ---: | ---: | ---: |
| 4字节 Unpack | 4096 / 1 | 68.47% | — | 28.83% |
| 4字节 PDecode | 12384 / 5 | 35.00% | 13.75% | 47.08% |
| 8字节 Unpack | 8192 / 1 | 65.45% | — | 33.51% |
| 8字节 PDecode | 16536 / 6 | 43.23% | 7.42% | 44.98% |

比例以各自完整CPU样本为分母。父子累计不能相加；kevent不剔除、不归一化或推断唯一原因。它们支持将Unpack列为下一局部候选，不能预测可提速上限，也不能与上轮profile百分比相减计算数组优化收益。

## 源码与参考

行级采样仍落在完整预检、普通mask字节写出与src逐字节切片；FF复制和分配也保留。[4字节源码视图](body-int32_edges-unpack-cpu-source.txt)与[8字节源码视图](array-width8_len896-unpack-cpu-source.txt)分别保存。前者普通写出/源切片约360/160ms，后者约150/180ms；这是源码归属，不能断言每个样本都由某条切片指令造成。

已阅读本地只读Skynet sproto.c的sproto_unpack：FF段memcpy、普通mask逐位读写，并在局部维护输入/输出指针。Go保留完整分配前预检、独占输出和默认预算，不照搬C调用方缓冲或未受Go检查的指针。也查看锁定protobuf-go v1.36.1 ConsumeVarint的按索引读字节并返回消费长度；可借鉴消费计数，不改变Sproto wire或照搬varint算法。文件/行号/SHA见[source-references](source-references.json)，本地Skynet文件不冒称等同于某个官方固定commit。

旧[普通Unpack实验](../2026-10-16-gosproto-unpack-candidates/RESULTS.md)中的cursor同时固定输出word、加&7并改源游标，曾有单字节形态回退。本轮后续只改变源游标，不重试同一组合、不引入更多密度分支；原取舍与计时不改写。新的70项比较独立包含全部18种现有Unpack形态。

## 核对与复现

23条命令全部成功：harness统计/选择/GC/夹具/指标测试与vet、构建、52项精确名称检查、4次CPU采集、12份pprof文本、实际Unpack机器码。全部18份Go源码等于f7022ac；fresh重建的源码、夹具、生成输入、工具、二进制与rawprofile SHA均核对。原始二进制/profile/源码副本在忽略tmp目录，文本与元数据随本页保存。

本诊断不重复库全测/官方/race/fuzz或GSR/GoTS；这些检查由后续采用候选时单独运行和记录。正式go.mod/go.sum不改、依赖v0.4.0。本报告未提交，不推送或发布。

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-unpack-current-profile/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-unpack-current-profile-new
```

输出目录须不存在，--prepare-only只重建输入。CPU/分配范围、kevent和四个固定夹具代表性限制保持如上。
