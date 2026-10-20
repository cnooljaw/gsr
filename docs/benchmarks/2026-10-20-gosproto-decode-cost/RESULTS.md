# Decode / Unpack 热点诊断

2026-10-20，已提交 gosproto `a93bb0d`，Apple M2、Go 1.24.13。继 [72 项单模块基线](../2026-10-20-go-sp-pb-current/RESULTS.md) 后，分别采集 AddressBook、大型结构和整数数组的 Decode、Unpack、PDecode：9 份 CPU、9 份全量分配 profile，80 条命令全部成功。没有修改生产实现、正式依赖或独立库工作区。

## 方法与限制

每个 case/operation/kind 使用独立进程。GOMAXPROCS=1、GOGC=100、GOMEMLIMIT=off；50ms 预热后，CPU 计时 2s，分配采样 5000 次、memprofilerate=1。先完成代表消息语义/wire/消费长度/padding检查、相关 harness vet 和编译，再采样。每次操作均调用公开 API，Decode 创建新结果。快照源码、测试/生成输入、harness、单模块解析、二进制及18份原始profile的哈希均核对。

CPU profile 是定位证据，不用其附带 ns/op 与普通基准排名；全量分配采样本身开销很大。alloc_objects/space 包含初始化、预热和 benchmark 框架，不把总量除以5000冒充单次对象数；下表 B/op、allocs/op 来自 ResetTimer 后循环。Go tiny allocator 的 profile 对象计数也不等于源码 new 次数。

所有百分比使用完整 CPU 样本作分母，累计值有调用树重叠，不能相加。`runtime.kevent` 占各 profile 的18.14%～66.48%，与GC/调度栈相关；不将其解释为协议网络瓶颈，也不删除后重新归一化。2s采样有误差，行级采样是热点位置提示，不是每条表达式的精确耗时。

## Body 主线

| 入口 | decodeMessage 累计 | mallocgc 累计 | reflect.New 累计 | kevent |
| --- | ---: | ---: | ---: | ---: |
| AddressBook Decode | 65.16% | 39.82% | 见完整 profile | 23.98% |
| 大型 Decode | 79.65% | 38.94% | 15.49% | 18.14% |

大型 `decodeMessage` 自身 flat 14.60%，`nativeInteger` 3.54%、`countChunks` 2.65%、`decodePointer.setInteger` 1.77%。对象构造确实是重要成本，但不是所有余量都能归为反射。大型分配 profile 中 `decodeMessage` flat 58.40%、`reflect.New` flat 38.48%；AddressBook 的 `decodeStringPointer` flat 31.74%，反映其字符串 presence 与独立结果的成本。

下一 Body 切片先检查字段解析/原生字段证明与字符串字段设置的重复工作，并核对当前 descriptor 可变和回退测试；保留独立子对象、presence、失败原子性与 Go 写屏障。暂不采用对象池、共享字段指针或将全部子对象合并进一块大存储来降低表面 allocs。不能凭此 profile 宣称这些重复检查已可安全删除。

## Packing 主线

| 入口 | UnpackWithLimit flat | 累计 | kevent |
| --- | ---: | ---: | ---: |
| AddressBook Unpack | 44.52% | 67.10% | 26.45% |
| 大型 Unpack | 59.57% | 70.43% | 28.70% |
| 整数数组 Unpack | 54.04% | 68.09% | 28.94% |
| 大型 PDecode | 16.10% | 17.37% | 28.39% |
| 整数数组 PDecode | 26.11% | 31.86% | 58.41% |

大型与整数数组的普通 mask 写入行 `unpacked[offset+index] = src[used]` 分别采到480ms/2.30s（20.87%）、400ms/2.35s（17.02% flat）。索引、边界检查、数据搬移和循环依赖都可能贡献，不能把整行成本全部叫作 bounds check。整数数组 `OnesCount8` 自身5.96%；预检扫描也有成本，不能只看写出循环。

下一 Packing 候选限定为**已预检普通 word 的固定八字节目标视图与索引处理**：保留完整首遍长度/预算校验、零段和FF逻辑，检查生成代码能否减少动态索引和边界检查；先验证全部 mask、截断、预算边界和精确输出容量，再用单一 SP 模块的独立二进制进程控制测量。若只有小收益，也按重复性、复杂度和回退成本评估，不设5%硬门槛。当前仅确定实验范围，尚未实现或宣称收益。

## 官方实现核对

只读参考 `../skynet/lualib-src/sproto/sproto.c:1386` 的 `sproto_unpack`：调用方传入输出指针与容量，普通 word 按8个bit判断并写零/有效字节，FF走 memcpy；即使缓冲不足仍计算需要的长度。`lsproto.c:704` 的 Lua 包装保留可扩容缓冲，第一次返回所需长度超过容量时扩容再解包，最后 `lua_pushlstring`。

Go 当前 API 返回本次独占的 `[]byte`，在分配前完整验证输入和有限输出长度，再一次精确分配；不能直接照搬可复用调用方缓冲模型，也不能通过删除首遍预检来复现C的循环。可借鉴其短小普通 word 路径，再用 Go 编译器输出与真实 profile 验证。参考文件哈希见 reference.json，未修改参考目录。

## 循环分配

| 消息 | Decode B/op / allocs | Unpack | PDecode |
| --- | ---: | ---: | ---: |
| AddressBook | 528 / 29 | 144 / 1 | 672 / 30 |
| 大型 | 10416 / 532 | 3456 / 1 | 13872 / 533 |
| 整数数组 | 8288 / 4 | 4096 / 1 | 12384 / 5 |

与当前普通基线一致；PDecode 在这些消息上只多一个 unpack 输出分配，没有证据支持“组合入口又额外复制一份完整正文”。单独入口耗时之和不必等于组合入口，不能直接相减生成精确CPU成本。

## 复现及验证范围

```sh
python3 docs/benchmarks/2026-10-20-gosproto-decode-cost/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output /absolute/new-output
python3 docs/benchmarks/2026-10-20-gosproto-decode-cost/collect.py \
  --output /absolute/new-output
```

输出目录必须不存在；采集脚本使用已提交归档并保留原始profile，报告保存文本视图、metadata和哈希，不提交测试二进制或大块原始profile。本轮临时原始数据位于 `tmp/gosproto-decode-cost-20261020`。验证只覆盖相关 fixture、harness vet、profile及证据一致性；没有实现变化，不重复全库/race/fuzz/oracle/GoTS。原独立库六项删除保持，正式依赖仍v0.4.0。
