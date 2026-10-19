# 分函数与索引预检组合实验（未采用）

2026-10-19。前轮共享输入诊断和原预检分函数实验已本地提交 GSR `2ca8864`，未推送。本轮直接测此前缺失的组合：私有 `unpackedSize` 加位置索引预检，对照生产库 `af9169d`。**主库未应用候选，仍干净；实验补丁和全部数据保留，新报告未提交。**正式依赖仍 v0.4.0，Decode 继续开放。

## 修改及正确性

只修改 pack.go：先完整预检，成功后分配确切长度并执行原恢复。私有 helper 内以 position/available 取代 remaining 尾切片，零段扫描上限、错误优先级、预算和全部分配前检查不变。恢复源码与上一轮 split-original 逐字一致，无 unsafe、池、公开 API、编译指令或 fixture 特判；没有通过删检查提速。空输入、nil 错误结果、len/cap、输入不变及输出独占复用既有契约与 138,500 组差分验证。

编译后原 UnpackWithLimit 为384条/112B帧，组合为调用者216条/128B帧和 helper196条/32B帧，保留显式调用。源码相同不代表整个恢复机器码或上下文相同，静态条数也不作为性能结论。

这是组合直接对生产版本的比较，**没有隔离“分函数后仅换扫描算法”的边际因果**。历史独立运行的两个比值不能相乘或相减来补出该因果；也不能把本轮归因于某个cache或分支预测机制。

## 固定70项结果

复用70项顺序与采样：self/clone/两轮compare，共280进程、26,880块、6,720四元组、1,680轮、1,120快照。默认 GOMAXPROCS1/GOGC100/无限GOMEMLIMIT，清sink/GC/50ms预热、10ms块、3×128分配观察保持。全部检查/构建/汇编先于第一项采样；不重采择优、删异常或扣控制。

负值更快；完整数据见[全部70项](ALL-CASES.md)。

| 入口 | 第1轮 | 第2轮 | 第1轮累计 ns | 第2轮累计 ns | self / clone 控制 |
| --- | ---: | ---: | ---: | ---: | --- |
| body/addressbook/pdecode | -0.47% | -2.08% | -7.4 | -4.8 | True / True |
| body/game_small/pdecode | -3.42% | -3.89% | -83.7 | -57.1 | True / True |
| body/game_large/pdecode | -3.12% | -4.38% | -435.4 | -705.0 | True / True |
| rpc/game_large/dispatch | -2.87% | -3.36% | -504.4 | -643.2 | True / True |
| body/int32_edges/pdecode | -9.84% | -9.23% | -694.6 | -727.6 | True / False |
| array/width8_len896/pdecode | -10.93% | -11.37% | +643.9 | -1553.8 | False / False |
| rpc/width8_len896/dispatch | -12.90% | -12.10% | -1129.2 | -1069.8 | True / False |
| array/width4_len3/pdecode | +2.46% | +2.01% | +11.1 | +12.9 | True / False |
| array/width8_len3/pdecode | +0.43% | +1.65% | +1.6 | +8.6 | True / False |
| shape/nonzero1x256/unpack | +26.54% | +32.88% | +284.7 | +338.3 | True / False |
| shape/nonzero2x256/unpack | +28.26% | +18.85% | +404.5 | +277.4 | True / False |
| shape/nonzero5x256/unpack | +11.68% | +13.02% | +200.3 | +263.8 | True / False |
| shape/FF512-sparse4096/unpack | +8.53% | +8.41% | +279.1 | +360.9 | True / False |
| shape/zeroRun8x64/unpack | +1.70% | +15.37% | +17.5 | +194.7 | True / True |

大型与两种大数组的 PDecode/Dispatch 收益值得保留记录，不能因未达到固定比例而删除。但普通word的1～7有效字节形态全部两轮更慢（约4%～33%），FF接稀疏两轮慢约8.5%，短数组PDecode也慢。当前选择不替换通用实现：新增私有调用没有解决之前业务与字节形态的取舍，广泛形态代价仍在。不是“没有优化价值”，也不是最低收益门槛。

8字节大数组 PDecode 首轮中位快10.93%，但累计慢643.9ns；次轮中位快11.37%、累计快1553.8ns。ASCII PDecode次轮中位快0.77%，累计慢17.4ns，全部并列保留；不只选有利统计量。self/clone控制存在失败，不能将所有观察变化当成补丁唯一因果，也不扣控制制造校正收益。

## 控制、证据与限制

self 52/70、clone 40/70；全部对象分配中位相同，26项字节尾差、20个入口中位/累计反向均保留。最大堆快照 8,855,792 B，非峰值；limiter全零。四模式最长块分别为 140.9/63.6/214.9/48.6 ms，原始GC事件/最差轮不删除。

313条命令成功：两侧目标契约、原70入口语义、109形态525项差分及138,500预检差分、harness/vet、构建/汇编与280采样。fresh归档、18份源码/输入/工具/夹具/生成文件、二进制/raw和原sampleBlock/blockCalls、冻结采样顺序独立核对。见[验证指纹](validation.json)、[独立复算](analyze.py)。

生产库未应用候选，未重复完整库/oracle/race/checkptr/fuzz或GSR/GoTS验收，不声称新PB/TS排名。生产库仍af9169d，go.mod/go.sum未修改；没有推送、发布或tag。

后续不再仅靠拆函数/索引表示的排列寻找收益。若追查这一条线，先以同一split-original基底控制单一扫描算法，或从预检与恢复各自profile找可解释的计算/读取成本；不对具体mask写补丁。当前实验只能排除“组合自然消除取舍”，不能证明不存在更好的通用实现。

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-unpack-split-indexed/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-unpack-split-indexed-new
python3 -B docs/benchmarks/2026-10-19-gosproto-unpack-split-indexed/analyze.py \
  tmp/gosproto-unpack-split-indexed-new
```

输出目录必须不存在。原始逐项文件/源码/二进制在忽略tmp目录；聚合raw、来源指纹、复算脚本及机器码随报告保留。
