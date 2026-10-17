# RPC profile 与固定私有 header 编码

2026-10-17。上一轮通知 session 优化已本地提交为 gosproto `69cae73` / GSR `9ecc811`，提交后两库干净，未推送。本轮从该版本继续；实现与记录保留到工作树，尚未提交/发布，正式 GSR 依赖仍为 v0.4.0。Decode 阶段继续冻结。

## 诊断与采样修正

先在独立归档中测 ascii32、game_large、int32_edges、echo_ascii4k 的真实通知 API。首次同时启用 memprofilerate=1 与 CPU profile，采集栈/内存采样成本占了主导；这组 CPU 数据不适合定位业务耗时，原样保存在 *-allocation-instrumented-cpu.txt 和 profile-commands.json，仅用于解释测量修正。该组 alloc_space/alloc_objects 用于分配定位，其 ns/op 不作性能基线。

随后单独采集 CPU（不逐次内存采样），命令与全部结果保存在 cpu-only-commands.json 和 *-cpu.txt / *-header-cpu.txt。Darwin 样本仍有明显 runtime.kevent 节点，不能把所有百分比直接当成实际业务成本占比；本轮只用调用栈及行级位置选择候选，正式收益由下面的配对计时判断。没有删除这些系统节点来重算排名。

小包 packRpcFrame 中能看到 GetSprotoType、encodedSize 和 encodeMessageInto 的重复通用流程；分配 profile 把一个16 B header对象定位到RequestEncode。大型消息的 alloc_space 则主要是 body/Pack 两缓冲，各约46%，布局记录约7%；header不是大型消息内存的主因。性能改动只针对已确认的小包固定开销，不宣称解决全部大包成本。所有 profile 源码/输入/二进制/原始样本指纹见 profile-inputs.json，原始二进制留在忽略目录。

## 实现及边界

Gosproto 的 rpcHeader 是库内固定的两个 *int32 字段（tag0 type / tag1 session），不由用户配置。现在用调用内22 B栈缓冲写header，再复制到原有独占Pack工作区；复用普通编码器的 skipTag、encodeIntegerInto、writeUint16/32，没有实现另一套整数wire，也没有替换普通struct编码器。

- 两个int32非inline字段各最多4 B长度+4 B值，加6 B头项，最坏22 B；response缺type时仍写tag跳跃，零值与不存在继续区分。
- normalizeLimits 保留；每个实际字段继续 budget.add(1)，最终header字节预算在输出分配前检查。header无嵌套、深度固定1，满足normalizeLimits保证的MaxDepth≥1；body仍使用自身完整预检和独立预算。
- 请求body校验、重复session的既有body返回、等待登记、容量上限与锁范围保持不变。
- RequestEncode 使用局部header值，避免通过指针变量字段赋值让session副本保守逃逸；ResponseEncode使用的局部header也不逃逸。没有共享临时表、池、缓存、generator或新增unsafe。
- 长度、cap、独占、Generic Encode/EncodePacked、Pack/Unpack/Decode/Dispatch、业务schema/生成物及TS实现均不变。维护代价是私有header布局若增加字段，必须同步更新该有限编码与独立对照；不推广为业务消息专用编码框架。

官方 sproto.lua 的 gen_response/host:attach 根据package type/session编码header并拼content后pack。GSR仍使用同样wire；没有采用Lua共享header_tmp状态。新独立官方用例直接以C/Lua Package的pencode/pdecode核对三种presence形态。固定官方commit及源文件指纹见validation.json。

## 验证增量与失败记录

扩展原RPC组合用例加入MinInt32，7种type×9种body长度×4种请求/响应角色×4种session，共 **1008组、2016输出**，核对分步普通Encode的精确wire/cap及输入/旧输出独占；预算与错误/session用例继续复用。新增 **39个官方C/Lua RPC头向量**：7种type的通知及4种session请求，另4种session-only响应，直接比较官方编码并解码验证presence。新wire/预算/官方用例在旧生产实现也通过。

原分配测试要求通知严格少于有响应请求，这是当时两者尚有session分配差的实现事实，不宜阻止更好的请求实现。本轮将其加强为三条RPC路径都不多于同构建的 Encode+Pack 两缓冲对照；普通基线为通知3/请求4/响应4，对照2，按预期失败。新实现为2/2/2。大型消息仍保留布局记录，所以不承诺所有消息都只有两次总分配。

首版候选为2/3/2，分配测试失败，逃逸链显示通过header指针字段赋值仍使requestSession进堆；改用局部header值后，普通及race/checkptr检查通过。完整失败、修正和逃逸条目保存在 checks.json、escape-candidate-initial.txt / escape-candidate-final.txt。采样前已完成修正，采样后没有修改任何库Go源码。

隔离harness还以旧/新实际API做10秒差分fuzz：仅58次执行，后续没有新的计数，不将该样本视为充分随机覆盖，也没有为提高数字重试。随后为相同检查增加全256 mask×12种尾块/FF邻域长度，共3072个固定种子，加原6个，共 **3078组、12312帧比较**；以种子模式单次运行通过，涵盖任意构造的int32 type、正session、请求/通知/响应及cap/独占。初始/扩展探针分别保留为rpc-fuzz-initial.go.txt / rpc-fuzz.go.txt，命令与各自源码指纹分开记录。

## 两轮真实 RPC 配对

复用上一轮未改动的19项探针（五种代表消息各通知/请求加CancelSession/响应，四种Echo通知）。self/clone各一次、候选两次；每项六轮×四个ABBA/BAAB四元组、10 ms目标块。没有重试、改参数或挑选轮次；共 **7296块、1824四元组、456配对轮次**，独立复算至1e-12相对容差一致。Apple M2、Go1.24.13、GOMAXPROCS=1、GC开启。

百分比为配对轮次比值中位数，负值表示耗时减少；分配表为首轮中位数，两轮完整数据见joined.json。

| 真实 RPC 路径 | 第一轮耗时差 | 第二轮耗时差 | B/op 前 → 后 | allocs/op 前 → 后 |
| --- | ---: | ---: | ---: | ---: |
| addressbook/notification | -14.26% | -13.61% | 304 → 288 | 3 → 2 |
| addressbook/request_cancel | -17.35% | -16.92% | 324 → 304 | 4 → 2 |
| addressbook/response | -15.28% | -15.19% | 324 → 304 | 4 → 2 |
| game_small/notification | -9.82% | -9.09% | 720 → 704 | 3 → 2 |
| game_small/request_cancel | -12.28% | -11.96% | 724 → 704 | 4 → 2 |
| game_small/response | -9.26% | -9.48% | 724 → 704 | 4 → 2 |
| game_large/notification | -2.22% | -3.22% | 7440 → 7424 | 4 → 3 |
| game_large/request_cancel | -1.70% | -3.70% | 7444 → 7424 | 5 → 3 |
| game_large/response | -1.82% | -2.74% | 7444 → 7424 | 5 → 3 |
| int32_edges/notification | -2.93% | -3.96% | 8208 → 8192 | 3 → 2 |
| int32_edges/request_cancel | -3.14% | -3.94% | 8212 → 8192 | 4 → 2 |
| int32_edges/response | -2.12% | -4.01% | 8212 → 8192 | 4 → 2 |
| echo_ascii4k/notification | -6.30% | -6.54% | 9744 → 9728 | 3 → 2 |
| echo_ascii4k/request_cancel | -8.11% | -9.76% | 9748 → 9728 | 4 → 2 |
| echo_ascii4k/response | -7.93% | -5.73% | 9748 → 9728 | 4 → 2 |
| ascii32/notification | -37.18% | -38.40% | 128 → 112 | 3 → 2 |
| utf8_1k/notification | -16.65% | -16.58% | 2064 → 2048 | 3 → 2 |
| ascii4k/notification | -5.97% | -5.97% | 9744 → 9728 | 3 → 2 |
| zeros4k/notification | -4.06% | -2.99% | 9744 → 9728 | 3 → 2 |

两轮19项全部更快。通知少一个16 B header对象；请求与响应各少header与session两个对象、约20 B/op，后者含tiny分配，不等同于每条消息少两个独立GC扫描对象。少数0.125 B窗口尾差保留，不作为一个新对象计数。

## 控制与结论

self 原工具总控制通过 14/19；未通过：game_large/request_cancel（中位差 -0.41%，IQR 1.06pp，分配中位数相同=False）、int32_edges/response（中位差 +0.38%，IQR 0.25pp，分配中位数相同=False）、echo_ascii4k/notification（中位差 -1.62%，IQR 3.07pp，分配中位数相同=True）、echo_ascii4k/request_cancel（中位差 -0.74%，IQR 1.32pp，分配中位数相同=False）、echo_ascii4k/response（中位差 +0.37%，IQR 2.36pp，分配中位数相同=False）。

clone 原工具总控制通过 11/19；未通过：game_large/request_cancel（中位差 -1.26%，IQR 0.81pp，分配中位数相同=False）、game_large/response（中位差 -1.09%，IQR 2.35pp，分配中位数相同=False）、int32_edges/notification（中位差 +0.94%，IQR 7.20pp，分配中位数相同=True）、int32_edges/response（中位差 -1.13%，IQR 3.56pp，分配中位数相同=True）、echo_ascii4k/request_cancel（中位差 -0.33%，IQR 1.54pp，分配中位数相同=False）、echo_ascii4k/response（中位差 +0.75%，IQR 2.36pp，分配中位数相同=False）、ascii32/notification（中位差 -3.11%，IQR 3.91pp，分配中位数相同=True）、ascii4k/notification（中位差 -2.03%，IQR 4.03pp，分配中位数相同=True）。

原始控制标记、逐轮比值、分配窗口和GC计数全部保留；控制未全过意味着精确百分比仍有不确定性，尤其大型消息的几个百分点不能当成精密跨机器结论。小包两轮收益及确定分配减少、共享wire函数和有界局部实现成本共同支持保留；不设统一最低加速门槛。没有新的PB/TS性能排名、网络吞吐或完整业务延迟结论。

## 最终验收与复现

库普通全测（启用官方C/Lua）、race+checkptr=2、vet，以及临时modfile下GSR protocol/sproto、debugserver、benchmarks三包通过；Go/TS Plain/MAC 7/7、零跳过。CPU/分配profile、全部失败、有限fuzz与固定差分证据公开。未重复GSR全仓/race、跨平台或原Pack/Codec fuzz；Decode阶段与正式依赖不变。

```sh
python3 -B docs/benchmarks/2026-10-17-gosproto-rpc-profile/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/rpc-header-new
python3 -B docs/benchmarks/2026-10-17-gosproto-rpc-profile/analyze.py tmp/rpc-header-new
```

目录必须不存在；--prepare-only只重建四份源码。基线69cae73，candidate.patch包含全部库Go改动，CHANGES只记录发布事项。源文件/夹具/工具与四份归档重建一致，最终源码与采样一致。

重建profile时，使用prepare-only的clone/harness，复制profile-probe.go.txt为profile_test.go后gofmt，按profile-commands.json/cpu-only-commands.json中的四个bench名称及2s参数构建/运行；CPU与逐次内存采样应分开，初次合并记录只用于诊断。重建差分时，向prepare-only的pass1/harness复制rpc-fuzz.go.txt为rpc_fuzz_test.go，用 go test -mod=mod -run=^FuzzRPCHeader$ . -timeout=1m 执行固定3078种子；随机fuzz初始记录使用初始探针，不混用两个版本。二进制、原始profile及归档源码留在忽略目录，不把开发机profile时间加入正式对照表。
