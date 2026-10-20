# Packing主线：普通Unpack局部word组装（未采用）

2026-10-20。上一轮Body countChunks待确认报告已提交GSR `bf1203e`，未推送；其候选未混入本次。本轮独立对照af9169d，只试普通Unpack将有效字节组装成局部uint64，再用标准库LittleEndian一次写出。**候选未采用，生产源码未应用；报告与文档未提交。**Body与Packing双主线继续，正式依赖v0.4.0不变。

## 改动、证明与静态证据

普通mask仍按TrailingZeros8顺序读src[used]并清除最低置位；将字节OR到word的相应八位，循环后PutUint64到unpacked[offset:offset+8]。完整预检、分配/清零、零段和FF路径逐字不变，输入游标/输出offset推进不变；不增加密度分支、表、池、unsafe、公开API或消息特判。

进入普通路径时mask为1..254，每轮非零，故index为0..7；显式&7不改变结果，只限定移位0..56。不同置位映射不同八位，按原序消费字节，不发生符号扩展。未置位输出本已由make清零，新word也为零；一次写出仍在同一独占8字节区域，完整预检保证长度与源有效字节数。没有改变错误顺序、预算、输出len/cap/nil或ownership，没有通过省略完整校验求速度。

实际函数384→396条，总体栈和边界调用点见assembly-summary.json；新增移位/OR、末尾完整word切片与store，省去普通内循环逐字节目标写。静态减少store不是动态提速承诺，未采硬件计数器/新profile来定量归因。

两侧既有allmask/不同高位字节/8种输入偏移/后继word、坏输入/预算/ownership/RPC目标契约通过；109形态525项差分与159275组公开解包输入/预算/错误class/结果len-cap-nil/独占差分通过。没有新增运行行为，不重复建镜像生产测试。

## 固定70项结果

self/clone/两轮compare共280进程、26,880块、6,720四元组、1,680轮、1,120快照。默认GOMAXPROCS1/GOGC100/无限GOMEMLIMIT、原清sink/GC/50ms预热、10ms校准块/3×128分配计数不变。所有构建/测试/汇编先于首块计时；冻结patch/plan/探针/工具/源码/二进制，不重采择优、不删样或扣控制。

| 入口 | 第1轮 | 第2轮 | 第1轮累计ns | 第2轮累计ns | self / clone控制 |
| --- | ---: | ---: | ---: | ---: | --- |
| body/game_large/unpack | +8.51% | +9.03% | +392.9 | +479.5 | True / False |
| body/game_large/pdecode | +1.96% | +2.46% | +138.2 | +545.3 | True / False |
| body/int32_edges/unpack | +6.70% | +7.58% | +353.5 | +363.6 | True / False |
| body/int32_edges/pdecode | +5.20% | +6.18% | -251.7 | +112.1 | False / False |
| array/width8_len896/unpack | +5.87% | +5.48% | +412.4 | +385.6 | True / False |
| array/width8_len896/pdecode | +4.18% | +4.95% | +379.6 | +444.2 | False / True |
| shape/nonzero1x256/unpack | +5.30% | +4.65% | +62.0 | +5.8 | True / False |
| shape/nonzero2x256/unpack | +8.96% | +10.52% | +187.6 | +147.6 | True / False |
| shape/nonzero4x256/unpack | +10.57% | +9.22% | +181.6 | +159.3 | True / False |
| shape/nonzero7x256/unpack | +9.23% | +8.83% | +214.3 | +492.6 | True / False |
| shape/FF512-sparse4096/unpack | +37.92% | +39.49% | +1109.5 | +1276.9 | True / False |
| shape/zeroRun8x64/unpack | -3.85% | -0.87% | -48.7 | -44.3 | False / False |
| shape/zeroRun64x64/unpack | +1.27% | +1.18% | +26.1 | +386.0 | True / True |

大型、两种大数组的Unpack/PDecode两轮普遍更慢，普通word1～7有效字节两轮全部更慢；FF接稀疏约慢38%～39%。因此不替换当前实现。不是“收益不足某个比例”——目标路径本身普遍增加成本。短零段8word两轮快3.85%/0.87%、累计少48.7/44.3ns的观察也保留，不能由此掩盖目标退化。零/FF源码相同也不代表完整函数执行环境必然零变化。

中位与累计不同方向仍保留，例如int32 PDecode第一轮中位慢5.20%但累计快251.7ns，普通3字节word第二轮中位慢2.13%但累计快72.5ns。候选的完整取舍不依赖只取有利统计量。

## 控制、来源与限制

self47/70、clone22/70，失败控制与全部轮次保留；所有对象数分配中位相同，25项字节尾差、24入口中位/累计反向完整列出。最大堆快照22,086,960B非峰值、limiter零；四模式最长块 205.8/379.7/124.9/130.0ms，不删除GC长尾。

313条流程命令成功，fresh/18份Go源/工具/生成输入/夹具/harness/原sampleBlock和blockCalls/二进制/raw和冻结顺序均核对。host-observation仅记录前后loadavg，不能诊断唯一竞争/调度原因。候选仅pack.go不同，原库全部Go源码仍af9169d；用户原有6个删除保持，不能称其工作树干净。

未采用候选，因此不重复完整库/oracle/race/checkptr/fuzz和GSR/GoTS验收，accept.py为未来采用配方、本轮未运行。没有新PB/TS排名；正式go.mod/go.sum保持HEAD，未提交新报告、不推送/发布/tag。

后续不按密度追加分支来修补本候选。保留数据作为“更少内存写≠更快”的证据；下一轮按双主线查看更大的Body结构执行成本或Packing中的实际循环依赖，不据此停止向TS领先程度靠近，也不把这个局部反例推广为Go算法已无优化空间。

```sh
python3 -B docs/benchmarks/2026-10-20-gosproto-unpack-word-store/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-unpack-word-store-new
python3 -B docs/benchmarks/2026-10-20-gosproto-unpack-word-store/analyze.py \
  tmp/gosproto-unpack-word-store-new
```

输出目录必须不存在；原二进制/源码/逐项进程在忽略tmp，聚合raw/复算/汇编/指纹随报告保存。
