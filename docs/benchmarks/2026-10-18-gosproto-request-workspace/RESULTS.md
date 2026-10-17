# 有响应 RPC 请求的单缓冲工作区

2026-10-18。上一切片已本地提交为 gosproto `0988af0` / GSR `9876a5b`，提交后两库干净、未推送。本轮只融合声明 Request 且有 Response 的请求；新代码和记录未提交，正式依赖仍为v0.4.0，Decode继续冻结。

## 实现与不变量

之前的请求先独立Encode body，在session校验/登记后再分配Pack工作区并复制body。本轮提取私有encodeBufferWithLimits，复用原有预检、布局与写出，返回本次独占workspace及其中的未压缩frame；普通encodeWithLimits仍立即Pack或返回普通body，有响应请求延后Pack至原有admission之后。

有Request和Response时，锁外先准备固定header，按原预算完整预检并编码body到Pack工作区尾部。工作区容量仍为packedBufferSize(headerSize+bodySize)，读写隔离沿用已验证的FF增长余量与八字节间隔；header/body预算独立，body预检源码未变，KeepAlive位置保留。固定type/session均为int32且默认预算保证私有header准备成功，不改变body错误优先级。

随后继续原有正session检查、加锁、重复检查、1024上限及session登记；持锁完成Pack并返回。body预检/写出始终在锁外，Pack和登记仍在同一临界区；缓冲分配和header准备从锁内移到锁外，持锁工作量减少，没有扩大锁范围。

重复session仍返回未压缩raw body和原有错误，使用encoded[headerSize:len(encoded):len(encoded)]同时限制长度与容量，避免通过reslice接触workspace余量；返回存储独占，不被后续调用或pending表持有。无效session/pending满载仍返回nil和原错误，typed nil仍ErrNil/nil且先于session校验。没有Request的协议沿用packRpcFrame，无响应通知和ResponseEncode方法不变，但它们使用的共用writer提取成本也纳入测量。

Decode/Dispatch、Pack/Unpack算法和公开API/协议模型不变；没有pool/cache、generator、unsafe或新依赖。

## 测试增量

扩展既有1008组RPC场景/2016个成功输出，在有Response的504组加入重复session检查，覆盖有/无Request、int32头边界、FF长段和尾块。逐组对照raw body或nil、精确cap；修改重复body不得影响成功帧或源输入。已有预算用例继续保护8 MiB边界和超限。

既有非法输入测试补typed nil请求在零/负/已占用session下返回ErrNil/nil，并加强无效session的nil输出与重复session的cap约束；新增语义用例先在旧实现通过。分配断言旧notification1/request2/response1对packedBody1按预期失败，候选普通/race构建通过；不将大型消息的布局记录分配算作已消除。

## 固定两轮真实RPC测量

沿用19项实际RPC探针：5项request_cancel（编码并CancelSession）为目标，14项通知/响应为非目标。self/clone各一次，固定候选两轮；另做一次原19项body/Pack/手工帧，因共用writer被提取。Go1.24.13 darwin/arm64、Apple M2、GOMAXPROCS=1、GC开启；六轮×四组ABBA/BAAB，目标块10 ms，无重试/调参。

**9120块、2280四元组、570配对轮次**独立复算到1e-12一致。负百分比表示耗时下降，取六轮配对比值中位数，不是置信区间，也不是网络延迟。

| 有响应请求（含CancelSession） | 第一轮 | 第二轮 | B/op 前→后 | allocs/op 前→后 |
| --- | ---: | ---: | ---: | ---: |
| addressbook/request_cancel | -7.92% | -5.88% | 304→160 | 2→1 |
| game_small/request_cancel | -6.95% | -6.11% | 704→352 | 2→1 |
| game_large/request_cancel | -5.80% | -6.17% | 7424→3968 | 3→2 |
| int32_edges/request_cancel | -9.49% | -11.58% | 8192→4096 | 2→1 |
| echo_ascii4k/request_cancel | -41.04% | -41.91% | 9728→4864 | 2→1 |

五项请求两轮均改善，少一次分配和body复制；大型仍保留布局记录。已经融合的通知/响应不会再次获得这次请求的降幅。

| 非目标真实RPC路径 | 第一轮 | 第二轮 | 两轮配对差 ns |
| --- | ---: | ---: | ---: |
| addressbook/notification | +0.20% | -1.75% | +1.4 / -11.4 |
| addressbook/response | +1.19% | +0.37% | +7.7 / +2.4 |
| game_small/notification | -0.02% | -1.97% | -0.2 / -19.8 |
| game_small/response | -0.21% | +0.15% | -2.1 / +1.5 |
| game_large/notification | -0.42% | -0.14% | -36.1 / -12.7 |
| game_large/response | -0.63% | -0.29% | -55.3 / -25.5 |
| int32_edges/notification | -0.61% | -0.66% | -26.5 / -28.6 |
| int32_edges/response | +0.24% | -0.30% | +10.6 / -12.9 |
| echo_ascii4k/notification | +0.77% | +1.41% | +6.7 / +12.4 |
| echo_ascii4k/response | -0.06% | +0.89% | -0.5 / +7.8 |
| ascii32/notification | +1.71% | +0.25% | +2.5 / +0.4 |
| utf8_1k/notification | -0.26% | +0.85% | -0.9 / +2.8 |
| ascii4k/notification | -0.37% | +0.42% | -3.3 / +3.8 |
| zeros4k/notification | +0.10% | -0.75% | +1.9 / -15.2 |

14项非目标分配不变；AddressBook响应+1.19%/+0.37%（7.7/2.4 ns）、ASCII4KiB通知+0.77%/+1.41%（6.7/12.4 ns）等观测代价保留，不将未针对路径的下降称作算法收益。

| 原body/Pack/手工帧控制（单次） | 耗时差 | 配对差 ns |
| --- | ---: | ---: |
| addressbook/sproto_encode | +0.38% | +2.1 |
| addressbook/sproto_pack | -0.84% | -1.0 |
| addressbook/sproto_pencode | -0.05% | -0.3 |
| game_small/sproto_encode | +1.03% | +7.6 |
| game_small/sproto_pack | -0.86% | -2.5 |
| game_small/sproto_pencode | +0.79% | +7.6 |
| game_large/sproto_encode | -0.52% | -34.0 |
| game_large/sproto_pack | +0.62% | +16.7 |
| game_large/sproto_pencode | -0.91% | -78.6 |
| int32_edges/sproto_encode | +0.13% | +3.0 |
| int32_edges/sproto_pack | +0.13% | +3.2 |
| int32_edges/sproto_pencode | +1.39% | +58.8 |
| echo_ascii4k/sproto_encode | +2.94% | +20.9 |
| echo_ascii4k/sproto_pack | -2.04% | -15.1 |
| echo_ascii4k/sproto_pencode | +0.80% | +6.7 |
| ascii32/frame | -0.34% | -0.9 |
| utf8_1k/frame | -1.10% | -7.5 |
| ascii4k/frame | +1.71% | +34.3 |
| zeros4k/frame | -0.45% | -14.7 |

原19项分配全部不变，耗时差−2.04%～+2.94%。ASCII纯Encode+2.94%（20.9 ns）、整数数组PEncode+1.39%（58.8 ns）及ASCII手工帧+1.71%（34.3 ns）明确披露；共用writer增加了函数层次，不能将这些路径宣称为完全无成本。单次控制不构成微小变化的确定归因。

self **17/19**、clone **15/19** 满足原controlAccepted标记，全部失败/IQR/轮次/GC/分配保留。不重试到通过，不用控制标记自动否定小收益，也不把近零差值当作确定变化。按请求路径明确分配减少、两轮一致改善、可审查的私有分阶段写出及公开代价保留；没有最低CPU百分比淘汰线。

## 失败路径成本

相比原先只分配body，session校验前会分配整个未压缩frame工作区。无效session/满载失败后它可被GC回收；重复session返回的body切片虽限长限容，仍保留底层header和Pack余量。逻辑cap/所有权相同不等于底层分配字节完全相同。

另用独立探针固定四种payload、三种失败、三轮每臂64次，轮间交换顺序，采MemStats；它只诊断分配，不计时排名，也不反复重跑。duplicate使用type/session MaxInt32（22 B header），invalid为session0、full为1025。表内为三轮中位数，所有72个窗口原值保留在error-allocation.json。

| payload B | 失败 | B/op 前→后 | mallocs/op 前→后 |
| ---: | --- | ---: | ---: |
| 32 | invalid-session | 48→64 | 1→1 |
| 32 | duplicate-session | 138.875→170.875 | 4.07812→4.07812 |
| 32 | pending-full | 48→64 | 1→1 |
| 256 | invalid-session | 288→288 | 1→1 |
| 256 | duplicate-session | 374.125→410.875 | 4.03125→4.07812 |
| 256 | pending-full | 288→288 | 1→1 |
| 4096 | invalid-session | 4864→4864 | 1→1 |
| 4096 | duplicate-session | 4954.88→4954.88 | 4.07812→4.07812 |
| 4096 | pending-full | 4864→4864 | 1→1 |
| 65536 | invalid-session | 73728→73728 | 1→1 |
| 65536 | duplicate-session | 73837.2→73843.5 | 4.10938→4.17188 |
| 65536 | pending-full | 73728→73728 | 1→1 |

32 B输入的无效session/满载从48→64 B；MaxInt32重复session的缓冲尺寸级别从48→80 B，256 B输入重复缓冲从288→320 B。4 KiB/64 KiB缓冲落在同一尺寸级别。duplicate还包含既有fmt.Errorf/格式化池分配，GC边界和池冷启动使64次窗口出现小数及少量对象差，不能把所有尾差都算作新增对象；不以该诊断替代成功路径计量。资源预算、失败nil或raw body返回不变，接受并披露这部分代价。

## 验收与复现

- 库全测、固定官方C/Lua对照（含39个RPC头向量）、race/checkptr及vet通过；官方e93fd5a归档的源码/core.so指纹一致。
- 不换探针的3078固定RPC种子/12312帧旧新wire/cap/输入及旧输出独占差分通过。
- Pack/Codec各10秒有限fuzz通过，分别3946/464565次。Pack最后约8秒计数不再增加，原始输出保留、未重试，不称作充分随机覆盖。
- 临时modfile下GSR三包、TS构建及Go/TS Plain/MAC **7/7、零跳过**通过；未运行GSR全仓/race、跨平台或新PB/TS排名。
- 四份prepare-only归档的源码/harness/夹具/工具/probe/patch/参数相同；最终库全部Go源码与测量candidate一致。未改正式go.mod依赖。

从GSR根目录复现，输出目录必须不存在：

```sh
python3 -B docs/benchmarks/2026-10-18-gosproto-request-workspace/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-request-workspace-new
python3 -B docs/benchmarks/2026-10-18-gosproto-request-workspace/analyze.py \
  tmp/gosproto-request-workspace-new
```

库须包含0988af0基线；candidate.patch固定本轮Go改动，驱动从归档构建。--prepare-only只重建四份RPC输入。失败分配诊断可在独立compare harness复制error-probe.go.txt为error_allocation_test.go，设置GSR_ERROR_ALLOCATION_OUTPUT后运行TestRequestFailureAllocation；不能在已计时harness里插入文件。

本轮完成RPC三种编码入口的中间body缓冲消除。下一轮先重新采当前EncodePacked/真实RPC的CPU与分配profile，依据剩余body遍历、布局记录或Pack热点选一个具体方向；Decode保持冻结，不凭旧profile继续堆技巧。
