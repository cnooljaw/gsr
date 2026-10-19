# 源游标代价形态的独立诊断

2026-10-19。源游标改动已本地提交为库 `af9169d`、GSR `6b47822`，未推送。本轮保持主库干净，归档f7022ac为A、叠加既有补丁得到当前af9169d为B，分别采两个有代价的形态和两个对照入口CPU。**没有确立退化的唯一原因，不以profile耗时替代配对结果，也不扣除控制漂移。** 后续[预检位置游标实验](../2026-10-19-gosproto-unpack-preflight-cursor/RESULTS.md)独立记录，未采用。

## 采集和正确范围

同一70项构造/wire/值/消费长度检查，精确选FF512-sparse4096、zeroRun8x64、nonzero2x256的Unpack及大型游戏PDecode。每个case的A/B各独立进程约2秒CPU，释放其他操作、清三个sink、GC、50ms预热后手动启动profile。选择器使用compare而非self，显式选择left/right；元数据Arm与CPU实际旧/新包命名空间均核对，避免把同一个函数误当两版。

8进程、24个计时外快照；GOMAXPROCS1、GOGC100、无限GOMEMLIMIT，limiter均零。最大堆快照4,086,416B（约3.90MiB），不是峰值。没有allocation profile；ResetTimer后的循环计数在A/B相同：三个形态分别4864B/1、4864B/1、2048B/1次，大型PDecode13872B/533次。所有profile ns/op保留原文，不作新的A/B基线。

## CPU 与实际 wire

| 入口 | A Unpack累计 | B Unpack累计 | A / B kevent flat |
| --- | ---: | ---: | ---: |
| FF接稀疏 | 51.61% | 73.01% | 46.54% / 26.11% |
| 短零段8word×64 | 35.45% | 31.39% | 62.73% / 67.71% |
| 双字节word×256 | 49.36% | 53.42% | 49.36% / 43.38% |
| 大型PDecode | 15.96% | 14.10% | 32.45% / 37.89% |

均以各自完整样本为分母，父子累计不相加，kevent不剔除或归一化；也不能用A/B份额升降推断速度或根因。短零段B的Unpack中make累计350ms/完整Unpack700ms，但包含运行时子路径，不据此把上一轮退化都归因GC。FF接稀疏B的预检、位提取与逐字节写出都有采样，FF复制未成为主要落点；行级样本不等同于特定指令的硬件瓶颈。

配对完成后另建独立探针，核实真实packed wire；不修改profile/配对归档。来源、哈希和统计见[wire-stats](wire-stats.json)：

| 形态 | packed字节 | 零mask | FF段 / 原始FF字节 | 普通word |
| --- | ---: | ---: | ---: | ---: |
| FF512接稀疏4096 | 2050 | 0 | 1 / 512 | 512，全部mask 0x11 |
| 短零段8word×64 | 1152 | 512 | 64 / 512 | 0 |
| 双字节word×256 | 768 | 0 | 0 / 0 | 256，8种旋转mask各32 |

短零段根本不执行普通mask的used循环；因此不能把退化简单解释为“每个普通word多了used++”。但同一函数的寄存器保存、分支和编译布局仍可能影响未改语句，不据此宣称成本必为零。FF接稀疏仅一个512字节FF段，主要是512个固定mask word；这与双字节旋转mask的分布不同，不能只按非零字节个数合并解释。本轮没有建立缓存/分支预测或特定指令为唯一原因的证据。

下一候选据此只改完整预检的输入游标表示，保留恢复循环及所有检查；其结果另见实验报告。没有按0x11或某个fixture名添加分支。

## 验证、偏差与复现

39条CPU诊断命令成功：harness测试/vet、70名称/构造检查、构建、8次采集、24份pprof文本与双版本机器码。旧版全部18份Go源码等于f7022ac、新版等于af9169d；fresh、工具/生成输入/夹具/源码/二进制/rawprofile及Arm命名空间核对通过。没有重跑生产全测/oracle/race/fuzz、GSR/GoTS或PB/TS排名。

独立wire探针第一次在错误根目录运行，误扫描旧tmp源码导致失败；相关no-tests-to-run输出不算验证。更正到独立模块后单项测试成功，正式go.mod/go.sum前后保持HEAD；命令和失败原因保留于[wire-stats-commands](wire-stats-commands.json)。该事故发生在配对采样全部结束之后，没有重采或改动计时输入。

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-unpack-cost-profile/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-unpack-cost-profile-new
```

输出目录须不存在，--prepare-only仅建输入。wire-stats.go.txt在另复制的harness中以TestUnpackCostWireStats运行，GSR_WIRE_STATS指定JSON输出；wire-stats-inputs.json保存原harness和额外探针SHA。原始CPU/二进制/源码在忽略tmp目录，文本与元数据随报告保存。本轮报告未提交，依赖v0.4.0、Decode继续开放。
