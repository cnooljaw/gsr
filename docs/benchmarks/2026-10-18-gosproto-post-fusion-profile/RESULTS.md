# RPC 融合后的编码热点诊断

2026-10-18。上一切片已本地提交为 gosproto `6d854d1` / GSR `f715e6b`，提交后两库干净、未推送。本轮从该提交归档采样，库生产代码未改，新增诊断和文档未提交；正式依赖仍v0.4.0，Decode冻结。

## 方法与范围

四个既有输入（32 B ASCII、大型游戏消息、896整数数组、4 KiB ASCII），各测body EncodePacked与RequestEncode+CancelSession，共八条路径。请求type/session=1，Request和Response为同一类型。CancelSession仅是维持pending有界的测量搭档，不表示正常业务每次请求必须取消，也不能把其成本全部归于RequestEncode。

每条CPU只采一次2秒，保持默认内存采样；另起进程固定20000次、memprofilerate=1测分配，不同时开CPU profile。Go1.24.13 darwin/arm64、Apple M2、GOMAXPROCS=1、GC开启。无A/A、A/B候选或PB/TS排名，不调参或重试。命令打印的ns/op全部保留，**不作为配对性能基线**；逐次内存采样有明显成本，不能与CPU进程直接比较。

源码、工具、夹具、探针、归档go.mod、二进制和16份原始profile哈希绑定于inputs.json。独立prepare-only重建的源码/harness/夹具/参数相同；go.mod绝对replace路径随输出目录变化，各自原始哈希核对后仅规范化该目录前缀再比较，文本一致。初次直接比较模块原始哈希的断言失败与修正见checks.json，没有重新采样。二进制/profile/归档源码在忽略目录，仓库保留可复现脚本、文本归因与命令。

## CPU 样本

百分比的分母是各进程全部CPU样本，非置信区间。encodedSize、encodeMessageInto含其子调用，记录/数组写出又是其子项，**不能将父子累计份额相加**。未进top40不等于绝对没有成本。

| 输入/入口 | encodedSize | encodeMessageInto | Pack | kevent |
| --- | ---: | ---: | ---: | ---: |
| ascii32/packed_body | 15.68% | 19.49% | 10.17% | 8.05% |
| ascii32/request_cancel | 11.81% | 10.13% | 10.97% | 8.86% |
| game_large/packed_body | 26.97% | 33.20% | 17.84% | 14.52% |
| game_large/request_cancel | 25.22% | 33.48% | 20.00% | 15.22% |
| int32_edges/packed_body | 6.90% | 28.02% | 33.62% | 17.67% |
| int32_edges/request_cancel | 9.79% | 25.11% | 28.94% | 20.00% |
| echo_ascii4k/packed_body | 未列入top40 | 4.44% | 6.22% | 64.44% |
| echo_ascii4k/request_cancel | 未列入top40 | 7.21% | 6.73% | 55.77% |

大型消息：encodedIntegerRecordSize累计约18.7%～19.1%，encodeIntegerRecordInto约21.7%～23.7%，源码归因落在逐字段循环、present/inline/tag判断、整数写出及布局记录。现有同构记录路径已经使用受验证的offset读取*int64，没有逐字段FieldByIndex；不能把剩余成本简单归为“全在安全反射”。reflect.Value.Index/UnsafePointer仍可见，但并非唯一或已证明最大的热点。Pack约17.8%～20.0%，只优化Pack不能解决全部body成本。

整数数组：Pack约28.9%～33.6%，encodeInt64Slice约23.4%～26.7%，其中32位PutUint32的内联归因约15.1%～15.7%。当前写出使用output[1+index*4:]；连续游标/固定宽度切片是可单独实验的方向，但索引运算或边界检查究竟占多少，仍需看BCE/汇编和配对实测，不能从源码直接断言更快。这里没有逐元素reflect.Append或指针对象分配可删。

小包：schema查询、通用字段遍历和最终输出分配仍可见。小包Request+Cancel的CancelSession累计约11.4%，包含map删除；不足以支持改pending/锁语义或新增专用小包codec API。

4 KiB ASCII：kevent占约55.8%～64.4%，栈连到GC的startTheWorldWithSema/systemstack；其他场景也有约8%～20%。没有系统证据确定这些样本的精确成因，不能解释为Sproto算法成本。该场景的CPU排名与微小份额不用于确定候选，不通过关闭GC或重采美化样本。

## 分配落点

| 输入 | body B/op / allocs | RPC B/op / allocs | 归因 |
| --- | ---: | ---: | --- |
| ascii32 | 64 / 1 | 64 / 1 | 最终工作区 |
| game_large | 3968 / 2 | 3968 / 2 | 3456 B最终工作区 + 512 B布局记录 |
| int32_edges | 4096 / 1 | 4096 / 1 | 最终工作区 |
| echo_ascii4k | 4864 / 1 | 4864 / 1 | 最终工作区 |

表内字节是分配计数/尺寸级别，不等同于返回slice的cap。与上一切片验收值一致。大型消息layout.add从32项栈记录溢出后make([]uint32,0,128)，每次512 B，约占分配字节12.9%。其他三项只有独占结果的最终buffer；不能为省它而复用公开输出或缩小既有容量。没有证据支持继续寻找header/session或中间body缓冲；NewRpc/测试框架的少量一次性分配不作为每次消息成本。

## 下一候选边界

1. 先单独实验整数数组连续写出：只动encodeInt64Slice的4/8字节写出游标，保留宽度判定、signed/unsigned语义、完整预检、wire/cap和独占。核对BCE/汇编，再固定对照body Encode/PEncode及真实RPC，小包/大型消息作控制，不同时改Pack。没有最低CPU收益线，不从profile份额预测收益。
2. 再对大型整数记录的残余字段索引/头写出建立局部候选；数组入口已有owner/形状验证，不能缓存消息标量或跨调用沿用可变schema验证，也不能删除深度/元素/字节预算。需要新增unsafe/generator时另行裁决，本次诊断不直接授权。
3. 数值Pack维持独立候选，先识别具体byte-pattern分支；不将旧TS/Go实验直接当当前收益。512 B布局外溢是内存候选，但不盲目扩大所有小消息栈帧或引入池/缓存。

本轮完成热点刷新与下一实验收敛，没有生产候选或新的提速结论。下一轮改动需自行建立可失败回归与固定配对，不与本次profile合并成性能证据。

## 验证与复现

独立harness的统计/选择/固定业务夹具测试、go vet、编译及八条CPU/八条内存采样通过；四个补充源码归因命令通过。全部profile/二进制/模块和最终库源码指纹已核对，库工作树保持干净。生产源码未改，未重复库官方/race/checkptr/fuzz、GSR三包或Go/TS；这些仍以上一切片6d854d1验收为依据，不称作本轮新执行。

从GSR根目录运行，输出目录必须不存在：

```sh
python3 -B docs/benchmarks/2026-10-18-gosproto-post-fusion-profile/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-post-fusion-profile-new
```

库须包含6d854d1，--prepare-only只重建输入。summary.json依据保存的cpu-cum文本整理，没有A/B速度列。补充源码归因见analysis-commands.json，均读取原始profile、不重新采样。
