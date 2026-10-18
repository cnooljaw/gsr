# 指针入口优化后的编码热点刷新

2026-10-18。nil调用分支实验已本地提交为 GSR `a391e42`，库保持已验收的 gosproto `a3b3830`；提交后两库干净，未推送。本轮只从a3b3830归档做CPU/分配诊断，生产源码未改。新增诊断与文档未提交，正式依赖仍v0.4.0，Decode冻结。

## 范围与方法

沿用32 B ASCII、大型游戏消息、896整数数组、4 KiB ASCII四个代表输入，在既有EncodePacked与实际RequestEncode+CancelSession之外补充纯Encode，共12条路径。RPC使用type/session=1，同一类型声明Request/Response；CancelSession用于维持pending有界，不代表生产每次请求都应取消，其成本也不能全部归于RequestEncode。

每条CPU只采一次2秒，保持默认内存采样；另起进程固定20000次、memprofilerate=1观察分配，不同时打开CPU profile。Go1.24.13 darwin/arm64 / Apple M2 / GOMAXPROCS=1 / GC开启。没有候选、A/A、A/B或PB/TS排名，不调参、不重采。原命令的插桩ns/op全部保留在commands.json/summary.json，不能当配对基线，不能比较CPU与逐次内存采样进程来推断EncodePacked比Encode快。

主库、归档、harness、工具、夹具、计划、模块、普通二进制和24份原始profile的指纹全部保存。每次采样后核对源码/计划/探针，未在采样期间改Go源码。每个命令还校验输出的唯一Benchmark名称，防止正则未选中或多选入口后把空采样当成功。另用prepare-only重建一次归档，模块绝对replace路径按各自输出根规范化后比较，原始哈希分别核对；不重做诊断采样。

## CPU 归因

下表均为各进程全部CPU样本中的累计份额；总样本时间见summary.json。预检encodedSize与其记录helper、写出encodeMessageInto与其数组/记录helper属于父子栈，不能相加；列未进top40不等于零成本。纯Encode没有调用Pack，未列入不被当作“测得严格0”。单次约2秒profile有采样离散，行百分比不等于可实现的提速比例。

| 输入/入口 | 预检 | 写出 | 记录预检 | 记录写出 | 数组写出 | Pack | kevent |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| ascii32/raw_body | 19.57% | 17.83% | 未列top40 | 未列top40 | 未列top40 | 未列top40 | 13.04% |
| ascii32/packed_body | 12.24% | 13.08% | 未列top40 | 未列top40 | 未列top40 | 11.39% | 17.72% |
| ascii32/request_cancel | 11.56% | 7.56% | 未列top40 | 未列top40 | 未列top40 | 11.56% | 11.56% |
| game_large/raw_body | 35.86% | 36.29% | 22.78% | 22.78% | 2.53% | 未列top40 | 21.10% |
| game_large/packed_body | 26.29% | 30.60% | 18.53% | 13.79% | 2.16% | 19.83% | 18.10% |
| game_large/request_cancel | 29.39% | 28.51% | 14.91% | 15.35% | 3.07% | 23.68% | 13.16% |
| int32_edges/raw_body | 11.76% | 18.10% | 未列top40 | 未列top40 | 14.48% | 未列top40 | 55.20% |
| int32_edges/packed_body | 7.63% | 13.14% | 未列top40 | 未列top40 | 11.86% | 33.90% | 34.75% |
| int32_edges/request_cancel | 7.88% | 10.37% | 未列top40 | 未列top40 | 9.96% | 38.59% | 24.90% |
| echo_ascii4k/raw_body | 1.46% | 5.85% | 未列top40 | 未列top40 | 未列top40 | 未列top40 | 72.68% |
| echo_ascii4k/packed_body | 未列top40 | 5.94% | 未列top40 | 未列top40 | 未列top40 | 7.31% | 58.45% |
| echo_ascii4k/request_cancel | 未列top40 | 5.16% | 未列top40 | 未列top40 | 未列top40 | 2.82% | 69.01% |

大型消息仍由body预检与写出共同构成主要成本。纯Encode两者累计约35.9%/36.3%，记录预检与记录写出各约22.8%；这几列相互包含。EncodePacked记录预检/写出约18.5%/13.8%，RPC约14.9%/15.4%，Pack约19.8%/23.7%。源码归因仍落在记录字段遍历、present/inline/tag判断、整数写入、布局add/set与预算检查；已有受验证offset读取，没有逐字段FieldByIndex，不能简单归为全是安全反射。UnsafePointer/Index仍可见，但nil分支位置实验已经证明不能只删某一次调用就预言更快。

整数数组的body/RPC Pack累计约33.9%/38.6%，连续数组写出约11.9%/10.0%，预检约7.6%/7.9%。源码中宽度检查循环、固定写出循环和普通word Pack仍可见；Pack注释中的循环首/目标切片/游标推进有采样，不能从某一源码行有大量样本就断言都是该处边界检查。连续数组游标与指针入口已经合入，但本轮份额不能与旧profile相减当作各项优化收益；运行进程、样本分母及GC栈不同。

小包还有schema查询、通用标量访问、分配与RPC session map操作。32B请求CancelSession累计约8.9%，包含删除map，不据此改pending/锁语义或创造小包codec入口。完整top40、flat/cumulative与源码注释保留，缺少top40项不补零。

runtime.kevent在4KiB ASCII三个入口约58.5%～72.7%，整数数组纯Encode约55.2%，其栈连到netpoll/startTheWorldWithSema/systemstack。样本保留，没有足够系统证据确定精确成因；不能当作Sproto算法成本，不能按这些占比安排字符串微优化，也不关闭GC或重采美化结果。其它场景中的runtime/GC样本同样不剔除，不重新归一化分母。

## 分配落点

| 输入 | 纯Encode B/op / 次数 | EncodePacked B/op / 次数 | RPC B/op / 次数 |
| --- | ---: | ---: | ---: |
| ascii32 | 48 / 1 | 64 / 1 | 64 / 1 |
| game_large | 3968 / 2 | 3968 / 2 | 3968 / 2 |
| int32_edges | 4096 / 1 | 4096 / 1 | 4096 / 1 |
| echo_ascii4k | 4864 / 1 | 4864 / 1 | 4864 / 1 |

以上为Benchmark计数窗口，不是返回slice的len/cap。大型三个入口各3968B/2次，pprof的alloc_space/alloc_objects和源码共同指向3456B最终缓冲的分配级别加512B布局外溢。32项栈记录溢出后make([]uint32,0,128)，这512B在当前大型输入每次发生；约占其分配字节12.9%。内存profile包含预热/框架/夹具初始化，能看到20001个结果与20001个布局对象，不能把其所有对象直接除20000称作Benchmark每次分配。

其它三种输入各一次结果缓冲分配；32B纯Encode为48B、组合与RPC为64B。实际字节预留与Go分配尺寸级别不同，不为得到更小B/op缩小既有输出cap，不能复用公开输出破坏独占。没有证据支持再次寻找中间body或header/session缓冲；NewRpc、反射schema和测试正则的少量初始化对象不算每次消息成本。

## 下一轮顺序与边界

1. 优先检查普通word Pack循环的输入/输出游标、切片与边界计算：先读BCE和实际ARM64编译结果，确认哪些重复计算真实存在，再选择一个局部候选；沿用现有稀疏/混合/FF/尾部模型，不同时换mask算法或增加密度分支。保持FF起点及六/七字节合并、256-word上限、最后不完整word、预留增长、重叠工作区和输出cap/独占。默认检查保留，由编译器证明冗余后消除；不新增unsafe、池或表。收益要看纯Pack、PEncode和真实RPC共同表现，不能从这里34%/39%份额预测收益。
2. 大型body的记录预检/写出仍值得看，但与Pack分开。Fields局部捕获没有确认组合收益、nil分支前移有稳定正常路径回退，不能直接重复。下一处遍历候选必须保留当前Fields描述替换、owner/形状回落、逐次取值、nil、tag、深度/字段数/字节预算及失败行为；需要新增unsafe或generator时另行裁决。
3. 512B布局外溢作为独立内存候选保留，不直接扩大所有消息的栈数组，也不引入池/缓存。先确认实际布局项数量与增长/栈帧代价，再选方案；本轮没有实施或承诺能够去掉它。

所有后续候选继续使用固定交错配对、完整控制/原值/非目标成本，不设统一最低收益百分比，不以微小差值或单个profile行作为提速证据。历史数组游标纯Pack+9.45%、指针入口未改请求+4.74%、已撤回nil候选int32纯Encode约+13%仍保留在原比较，本轮不同基线的诊断不解释或覆盖这些观察。

## 验证与复现

独立harness的统计、选择与固定代表夹具检查、go vet、构建及12条CPU/12条逐次分配采样通过；24个Benchmark选择逐一核对。归档重建、全部源码/模块/二进制/profile/工具/夹具指纹核对通过，主库工作树保持干净。本轮没有生产改动，没有重复官方/race/checkptr/fuzz、GSR三包或Go/TS验收；它们仍以a3b3830验收为依据，不计作本轮新执行。

```sh
python3 -B docs/benchmarks/2026-10-18-gosproto-post-pointer-profile/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/post-pointer-profile-new
python3 -B docs/benchmarks/2026-10-18-gosproto-post-pointer-profile/analyze.py tmp/post-pointer-profile-new
```

输出目录须不存在，库须包含a3b3830；--prepare-only仅重建输入。analyze.py从保存的pprof/原命令整理归因，不生成速度比。仓库保留计划、探针、复现/分析脚本、原命令、文本归因与哈希；原始二进制/profile/整份源码归档留在忽略的tmp/。当前新记录待审查、未提交，未推送或发布，正式依赖与Decode不变。
