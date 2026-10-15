# TypeScript FF 扫描、数值写出与有界解包缓冲验收

已按 FF 门槛 → 数值数组 → 内部解包缓冲 → 公开 unpack 容量策略完成。没有更改 Go 代码、公开协议 API、Gate/Runtime 边界、资源额度或官方 wire。新实现尚未提交。

修改前为 GSR `2092005`，其 TS 已包含前两轮优化。该提交与独立 gosproto `6012b06` 已在实施前创建；没有推送或发布，GSR 正式 Go 依赖仍为 v0.4.0。

## 口径

以下全部时间为 µs/op、同进程五轮中位数，负百分比表示新实现耗时减少；差值由原始值计算。PB 控制项使用 @bufbuild/protobuf 2.15.0，未更换夹具、schema 或 wire 签名。历史正式 PB 只作另列参照，不与本次样本混合统计。

本次 188 项、940 个样本；Node v23.11.0 / V8 12.9.202.28-node.14，Apple M2，darwin/arm64。开始 2026-10-15T08:49:31.225Z，结束 2026-10-15T08:51:32.051Z；loadavg [5.3056640625, 6.4169921875, 5.92919921875] → [8.0322265625, 6.8251953125, 6.1376953125]。每项热身至少约 50 ms、每轮约 100 ms，交替反转执行顺序，计时与 profile、测试分开。

原生 pdecode/dispatch 的缓冲在热身时已建立，数据表示热缓冲。冷启动、超保留容量、大小交替和失败复用另有回归及缓冲观测。RPC 不含外层客户端安全预检、会话分配、网络、MAC；dispatch 包括自身路由和 response 闭包构造。手工帧解码使用固定已知 schema，不能与原生入口混算优化比例。新增的 29 种形态扩展了同一进程的热身输入谱，绝对耗时不应与早先较窄的 API 探针直接拼接计算收益。

## 实现与取舍

- FF 批量扫描门槛从 1024 B 降到 64 B，保持最多 256-word 段、6/7 非零字节合并与末尾补零。32/64/128/256 B 候选均作过门槛邻域检查，64 B 在缩短门槛断点的同时保留小块直接路径。
- 数值数组保留完整类型、精度、位宽与尺寸预检；4/8 字节写出拆开，用有界 DataView 替代重复 Node Buffer 值域检查。视图绑定真实 byteOffset，编码缓冲扩容即失效。
- Pack 尾部直接读取剩余字节并隐含补零，不再创建 8 B 临时 Buffer。减少一次临时分配，不据此宣称数值 Pack 的 CPU 已大幅改善。
- pdecode/dispatch 同步借用实例私有解包缓冲，按需增长，最多保留 16 KiB；大包使用调用内临时输出。公开 unpack、encode、pencode、RPC 编码结果仍独占，业务返回值不保留 scratch 视图。
- 公开 unpack 对至少 2 KiB 输入最多查看前八个 packed 段。只有其中 FF 原始字节已覆盖整个 packed 输入的四分之三时，才用现有 lunpack 空输出作完整校验/计数并精确分配；其余保留原路径。该判定不承担安全校验，所有实际解包仍完整校验。
- 未继续改字符串或构造字段编译框架：UTF-8 RPC 编码在本轮后已改善，先保留当前字符串路径。

## 关键原生入口

| 场景 | 操作 | 修改前 | 修改后 | 耗时变化 |
| --- | --- | ---: | ---: | ---: |
| 896 项整数数组 | encode | 9.759 | 2.913 | -70.2% |
| 896 项整数数组 | pencode | 14.854 | 8.078 | -45.6% |
| 1000 B UTF-8 | rpc_encode | 3.558 | 1.917 | -46.1% |
| 4 KiB ASCII body | pdecode | 1.454 | 0.606 | -58.3% |
| 4 KiB ASCII | rpc_decode | 1.498 | 0.693 | -53.7% |
| 4 KiB 零字节 | rpc_decode | 2.541 | 1.278 | -49.7% |
| 32 B ASCII | rpc_encode | 0.602 | 0.541 | -10.2% |
| 32 B ASCII | rpc_decode | 0.285 | 0.247 | -13.2% |

## 业务消息与同次 PB 控制

纯编解码与含 packing 路径分列；PB 不承担 Sproto 的 0 packing。

| 消息 | 操作 | 正式 PB | 本次 PB | SP 前 | SP 后 | SP 变化 |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| AddressBook | encode | 1.176 | 1.281 | 0.818 | 0.802 | -1.9% |
| AddressBook | decode | 1.407 | 1.447 | 0.863 | 0.867 | +0.6% |
| AddressBook | pencode | 1.176 | 1.281 | 1.161 | 1.080 | -6.9% |
| AddressBook | pdecode | 1.407 | 1.447 | 1.103 | 1.058 | -4.1% |
| AddressBook | combined_encode | 1.176 | 1.281 | 1.189 | 1.118 | -5.9% |
| AddressBook | combined_decode | 1.407 | 1.447 | 1.110 | 1.106 | -0.3% |
| 小型游戏状态 | encode | 2.534 | 2.718 | 1.735 | 1.542 | -11.1% |
| 小型游戏状态 | decode | 3.014 | 3.036 | 1.470 | 1.462 | -0.6% |
| 小型游戏状态 | pencode | 2.534 | 2.718 | 2.310 | 2.095 | -9.3% |
| 小型游戏状态 | pdecode | 3.014 | 3.036 | 1.947 | 1.863 | -4.3% |
| 小型游戏状态 | combined_encode | 2.534 | 2.718 | 2.342 | 2.159 | -7.8% |
| 小型游戏状态 | combined_decode | 3.014 | 3.036 | 1.960 | 1.925 | -1.8% |
| 大型游戏状态 | encode | 22.781 | 25.306 | 19.431 | 15.363 | -20.9% |
| 大型游戏状态 | decode | 31.551 | 32.037 | 14.101 | 13.947 | -1.1% |
| 大型游戏状态 | pencode | 22.781 | 25.306 | 22.792 | 20.303 | -10.9% |
| 大型游戏状态 | pdecode | 31.551 | 32.037 | 18.453 | 18.055 | -2.2% |
| 大型游戏状态 | combined_encode | 22.781 | 25.306 | 23.055 | 20.881 | -9.4% |
| 大型游戏状态 | combined_decode | 31.551 | 32.037 | 18.466 | 18.437 | -0.2% |
| 896 项整数数组 | encode | 12.514 | 12.631 | 9.759 | 2.913 | -70.2% |
| 896 项整数数组 | decode | 11.207 | 11.164 | 4.624 | 4.627 | +0.1% |
| 896 项整数数组 | pencode | 12.514 | 12.631 | 14.854 | 8.078 | -45.6% |
| 896 项整数数组 | pdecode | 11.207 | 11.164 | 9.079 | 8.872 | -2.3% |
| 896 项整数数组 | combined_encode | 12.514 | 12.631 | 15.252 | 8.598 | -43.6% |
| 896 项整数数组 | combined_decode | 11.207 | 11.164 | 9.101 | 9.134 | +0.4% |
| 4 KiB ASCII body | encode | 5.828 | 5.632 | 1.247 | 1.251 | +0.4% |
| 4 KiB ASCII body | decode | 0.655 | 0.668 | 0.487 | 0.488 | +0.1% |
| 4 KiB ASCII body | pencode | 5.828 | 5.632 | 1.513 | 1.538 | +1.6% |
| 4 KiB ASCII body | pdecode | 0.655 | 0.668 | 1.454 | 0.606 | -58.3% |
| 4 KiB ASCII body | combined_encode | 5.828 | 5.632 | 2.076 | 2.137 | +2.9% |
| 4 KiB ASCII body | combined_decode | 0.655 | 0.668 | 1.474 | 1.069 | -27.5% |

pencode/pdecode 为原生入口；combined_encode 是 pack(encode(...))，combined_decode 是 decode(unpack(...))，包括公开中间输出。

| 消息 | 独立步骤 | SP 前 | SP 后 | 耗时变化 |
| --- | --- | ---: | ---: | ---: |
| AddressBook | pack | 0.372 | 0.315 | -15.4% |
| AddressBook | unpack | 0.242 | 0.237 | -2.3% |
| 小型游戏状态 | pack | 0.618 | 0.618 | -0.1% |
| 小型游戏状态 | unpack | 0.490 | 0.476 | -2.7% |
| 大型游戏状态 | pack | 5.327 | 5.371 | +0.8% |
| 大型游戏状态 | unpack | 4.355 | 4.392 | +0.8% |
| 896 项整数数组 | pack | 5.583 | 5.603 | +0.4% |
| 896 项整数数组 | unpack | 4.551 | 4.567 | +0.3% |
| 4 KiB ASCII body | pack | 0.913 | 0.949 | +3.9% |
| 4 KiB ASCII body | unpack | 0.990 | 0.691 | -30.2% |

## 应用帧：原生与手工路径

| 内容 | 操作 | 正式 PB | 本次 PB | SP 前 | SP 后 | SP 变化 |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| 32 B ASCII | rpc_encode | 0.890 | 0.907 | 0.602 | 0.541 | -10.2% |
| 32 B ASCII | rpc_decode | 0.653 | 0.646 | 0.285 | 0.247 | -13.2% |
| 32 B ASCII | rpc_encode_manual | 0.890 | 0.907 | 0.766 | 0.706 | -7.7% |
| 32 B ASCII | rpc_decode_manual | 0.653 | 0.646 | 0.321 | 0.321 | -0.2% |
| 1000 B UTF-8 | rpc_encode | 3.003 | 3.011 | 3.558 | 1.917 | -46.1% |
| 1000 B UTF-8 | rpc_decode | 4.747 | 4.765 | 4.410 | 4.214 | -4.4% |
| 1000 B UTF-8 | rpc_encode_manual | 3.003 | 3.011 | 3.887 | 2.267 | -41.7% |
| 1000 B UTF-8 | rpc_decode_manual | 4.747 | 4.765 | 4.477 | 4.454 | -0.5% |
| 4 KiB ASCII | rpc_encode | 7.823 | 7.946 | 7.176 | 7.267 | +1.3% |
| 4 KiB ASCII | rpc_decode | 1.073 | 1.048 | 1.498 | 0.693 | -53.7% |
| 4 KiB ASCII | rpc_encode_manual | 7.823 | 7.946 | 8.482 | 8.551 | +0.8% |
| 4 KiB ASCII | rpc_decode_manual | 1.073 | 1.048 | 1.543 | 1.205 | -21.9% |
| 4 KiB 零字节 | rpc_encode | 7.879 | 7.849 | 7.353 | 7.214 | -1.9% |
| 4 KiB 零字节 | rpc_decode | 1.074 | 1.047 | 2.541 | 1.278 | -49.7% |
| 4 KiB 零字节 | rpc_encode_manual | 7.879 | 7.849 | 8.788 | 8.709 | -0.9% |
| 4 KiB 零字节 | rpc_decode_manual | 1.074 | 1.047 | 2.566 | 2.599 | +1.3% |

公开字节返回值的成本仍存在。零字节手工帧解码没有采用内部借用入口，不能把原生 dispatch 的改善算到这条路径。SP 对 PB 的差距还包含额外 packing 工作，不能仅由胜负断定实现质量。

## 公开 unpack 的数据形态门禁

| 数据 | 原始 B | Packed B | 修改前 | 修改后 | 耗时变化 |
| --- | ---: | ---: | ---: | ---: | ---: |
| ascii32 | 32 | 34 | 0.093 | 0.094 | +1.6% |
| zero32 | 32 | 4 | 0.180 | 0.186 | +3.0% |
| sparse32 | 32 | 12 | 0.169 | 0.171 | +0.9% |
| ascii256 | 256 | 258 | 0.139 | 0.138 | -0.6% |
| zero256 | 256 | 32 | 0.254 | 0.261 | +2.8% |
| sparse256 | 256 | 96 | 0.637 | 0.630 | -1.1% |
| ascii1024 | 1024 | 1026 | 0.378 | 0.372 | -1.5% |
| zero1024 | 1024 | 128 | 0.571 | 0.577 | +1.0% |
| sparse1024 | 1024 | 384 | 2.238 | 2.256 | +0.8% |
| ascii2048 | 2048 | 2050 | 0.754 | 0.309 | -59.1% |
| zero2048 | 2048 | 256 | 0.994 | 1.013 | +1.9% |
| sparse2048 | 2048 | 768 | 4.419 | 4.415 | -0.1% |
| ascii4096 | 4096 | 4100 | 0.987 | 0.628 | -36.4% |
| zero4096 | 4096 | 512 | 2.040 | 2.019 | -1.0% |
| sparse4096 | 4096 | 1536 | 8.809 | 8.676 | -1.5% |
| ascii16384 | 16384 | 16400 | 2.263 | 1.696 | -25.1% |
| zero16384 | 16384 | 2048 | 6.929 | 7.623 | +10.0% |
| sparse16384 | 16384 | 6144 | 33.406 | 33.251 | -0.5% |
| ascii65536 | 65536 | 65600 | 8.137 | 8.570 | +5.3% |
| zero65536 | 65536 | 8192 | 23.885 | 26.256 | +9.9% |
| sparse65536 | 65536 | 24576 | 134.865 | 138.087 | +2.4% |
| FF512-sparse4096 | 4608 | 2050 | 9.201 | 9.247 | +0.5% |
| FF512-sparse16384 | 16896 | 6658 | 33.830 | 34.025 | +0.6% |
| FF1024-sparse4096 | 5120 | 2562 | 4.805 | 4.927 | +2.5% |
| FF1024-sparse16384 | 17408 | 7170 | 34.810 | 35.099 | +0.8% |
| FF2048-sparse4096 | 6144 | 3586 | 5.021 | 5.153 | +2.6% |
| FF2048-sparse16384 | 18432 | 8194 | 35.365 | 35.544 | +0.5% |
| FF4096-sparse4096 | 8192 | 5636 | 5.183 | 5.408 | +4.3% |
| FF4096-sparse16384 | 20480 | 10244 | 18.315 | 18.615 | +1.6% |

## 撤回与限制

统一对所有 ≥2 KiB packed 输入预扫，会令真实 game_large / int32_edges 的公开 unpack 约慢 36%，组合解码约慢 11% / 18%；已撤回。仅检查前部存在一个长 FF 块仍不足够：FF 前缀后接稀疏数值时可慢 25%–39%；进一步收窄到上述四分之三覆盖条件。全部中间尝试留在忽略的 tmp/，没有混入最终表格。

数值 Pack 的逐 word 混合 mask 循环仍是热点；本轮主要改善数值写出，不重做上一轮已回退的算术 mask。公开零字节 unpack 仍可二次扫描，它的独占输出成本没有因内部借用而消失。该条件也不保证对所有形态最优，例如很大的纯 ASCII 可能不在八段检查内满足覆盖比例，仍走原路径。

## 内存与 profile

解码缓冲懒分配，首次约 2 KiB；按实际需求增长，保留上限 16 KiB，超限输出不进入缓存。冷/热、大包前后实际容量与 lunpack 调用见 [缓冲观测](buffers.json)。仅计算新增解包工作区：一万个 codec 实例全部达到上限约 156.25 MiB；这不是预分配；一个连接使用多个 codec 时需分别累计，也不包含原有编码 scratch、schema、消息结果或网络内存。实例销毁后由 GC 回收，没有全局池。

CPU 在计时结束后单独采集六条路径，每条约 1 秒、500 µs 采样间隔；摘要见 [profile-summary.json](profile-summary.json)。self 百分比不跨场景相加，未使用 sampled heap 冒充精确 B/op。完整 cpuprofile 保留在 tmp/。数值编码热点现在集中于 encode_integer_array（self 约 79.8%）；独立混合 word Pack 约 94.0% 归于 Pack 循环。原生零字节 RPC 解码仍有 lunpack（约 38.8%）与字符串转换成本，属于剩余空间。这里没有把相对占比变化直接换算成收益。

## 验证与来源

- 最终 TS 构建及 Go/TS Plain/MAC 集成 7 项全部通过，零跳过。Go 接收端通过临时 modfile 使用已提交的 gosproto 6012b06；GSR 正式 go.mod 不变。
- 固定官方 C/Lua 快照的 20 组向量复核通过；TS 用例核对独立 raw/packed 向量，不只验证自身往返。
- 新旧 Pack/Unpack 3416 组差分通过，包含高位字节、偏移、尾部、FF 边界、有限前缀和畸形输入。
- 扩展现有测试：FF 门槛与分段的确定 wire；数组末尾宽值、非对齐 body、扩容后复用；大小包交替、失败后恢复、既有对象/字节快照；大输入迟到截断、恰好 8 MiB 接受和超额拒绝。没有为相同底层契约另建跨层夹具。
- 生产 Go 未变，不重复全仓 Go/race/vet。未测浏览器、完整客户端吞吐或线上并发内存。

最终原始值：[typescript.json](typescript.json)；代码、工具与依赖：[inputs.json](inputs.json)。

最终 max/min > 2 的操作有 2 项；中位数回退超过 10% 的 SP 项有 1 项。原始样本全部保留；中位数不等于显著性检验。

| 波动项 | 版本 | min | median | max |
| --- | --- | ---: | ---: | ---: |
| unpack_shape/ascii16384 | before | 2.196 | 2.263 | 5.038 |
| zeros4k/rpc_encode | after | 7.133 | 7.214 | 25.669 |

16 KiB 零块在全量中约 +10.0%；保持源码不变，针对三种零块交错复核五轮（另 30 个样本），结果如下，未支持持续 10% 回退。专项不替换全量中位数，原始值见 [零块复核](zero-check.json)。

| 原始零块 | 修改前 | 修改后 | 耗时变化 |
| --- | ---: | ---: | ---: |
| zero4096 | 1.955 | 1.962 | +0.3% |
| zero16384 | 6.647 | 6.815 | +2.5% |
| zero65536 | 23.662 | 23.709 | +0.2% |

## 复现

修改前模块已核对可从提交 2092005 的 TS 源码逐字节复现。先在仓库根目录执行：

```sh
mkdir -p tmp/ts-perf-baseline
git show 2092005:examples/websocket-gate/client/src/vendor/sproto.ts > tmp/ts-perf-baseline/sproto.ts
examples/websocket-gate/node_modules/.bin/tsc tmp/ts-perf-baseline/sproto.ts \
  --target ES2022 --module ES2022 --moduleResolution bundler --types node \
  --typeRoots examples/websocket-gate/node_modules/@types --skipLibCheck \
  --outDir tmp/ts-perf-baseline/compiled
cp tmp/ts-perf-baseline/compiled/sproto.js tmp/ts-perf-baseline/sproto.mjs
npm --prefix examples/websocket-gate run build
node examples/websocket-gate/scripts/benchmark-sproto-api.mjs tmp/protocol-sproto-round3 tmp/ts-perf-baseline/sproto.mjs
GOFLAGS=-modfile=/private/tmp/gsr-next-codec.mod npm --prefix examples/websocket-gate run test:sproto:integration
SPROTO_ORACLE_DIR=/absolute/official-sproto python3 examples/websocket-gate/scripts/benchmark-oracle.py --correctness
```

临时 modfile 的准备见前两轮说明；不要提交本地 replace。正确性与官方对照、性能采样和 profile 顺序运行。
