# TypeScript Sproto 剩余性能差距排查与改进方案

状态：2026-10-16，TS 性能优化阶段基本完成。P1–P4 为 `72fc11a`，第四轮为 `d9655b8`，第五轮 Pack 优化及最终验收已提交为 `9801808`。默认停止无业务证据的微调，转向 gosproto；可迁移经验、失败尝试及测量规则见 [TS → Go 优化经验](SPROTO-OPTIMIZATION-LESSONS.md)，后续实施见 [Go 方案](RFC-GOSPROTO-PERFORMANCE.md)。以下保留各轮排查与隔离原型证据，原型数据不替换正式验收数据。

范围为第二轮之后相对正式 TypeScript Protobuf 基线仍慢 10% 以上的六项。Go 后续工作仍见 [Gosproto 性能方案](RFC-GOSPROTO-PERFORMANCE.md)。

## 1. 对照范围与测量口径

- 历史基线：[v0.4.0 完整复测](benchmarks/2026-10-15-v0.4.0/report.md)。
- 当前实现：[第二轮结果](benchmarks/2026-10-15-sproto-round2/RESULTS.md)。
- 实际使用的 TS Protobuf 是 `@bufbuild/protobuf 2.15.0`，不是 protobuf.js。检查其 `to-binary`、`from-binary` 和 `wire/binary-encoding` 源码。
- Sproto 对照官方固定快照 `e93fd5a0653b2bcab38d3b830958bf2e3a901b5f` 的 `sproto.c`、`lsproto.c`，并核对只读 `../skynet/lualib-src/sproto` 中对应实现；不更换 wire 规范。
- 本次诊断的生产 TS 源码 SHA-256：`18be66be36805c274551ee47ad0c43e4af8c5ab064fde7990c6911d5611d439d`。

原有代表性基准的组合编码为 `pack(encode(...))`；Echo frame 编码为两次 `encode`、`Buffer.concat`、`pack`。它们继续计入中间输出复制，不能冒充上一轮已优化的 `pencode` / `attach` 路径。

本次额外做了同进程交错五轮专项采样、输入长度与执行次数探针，以及分开进行的五项 CPU profile。采样范围是 codec，不包含外层客户端安全预检、网络或 MAC。专项结果不与历史样本合并，也不作为新正式基线。

诊断脚本、全部原始样本、原型模块和 profile 保存在忽略的 `tmp/sproto-diagnosis/`，不把候选源码替换到 vendor 实现。后续实施时，应扩展已有可复用基准并保存正式验收来源。

## 2. 六项差距的实际来源

| 原测试项 | 已确认的现象 | 结论 |
| --- | --- | --- |
| AddressBook Encode+Pack，历史对照 +54.9% | 同进程 PB 为 1.221 µs，SP 手工组合 1.260 µs，原生 pencode 1.243 µs；纯 SP encode 0.826 µs、pack 0.441 µs。 | 本次仅约 +3.2% / +1.8%，原先 +54.9% 不是稳定差距。优先解决共同热路径，不为该数据单独增加机制。 |
| 896 项整数数组 Encode+Pack，+27.8% | 3593 B 原始数据拆成 450 个 word，无 FF word；编码 profile 中 `encode_integer_array` self 约 51.1%，Node `checkInt` self 约 35.8%。独立 Pack 的热点仍在其 word 循环。 | 编码重复值域检查和混合 word Pack 都值得处理；不是 FF 阈值问题。 |
| 4 KiB ASCII body Unpack+Decode，+129.3% | packed 4103 B，初始分配 8206 B，实际输出 4104 B；一次扫描即可完成。专项 unpack 约 0.989 µs，纯 decode 约 0.468 µs。 | 原先“可能二次扫描”的猜测在此不成立，主要排查过量分配和 Buffer 创建。单测中位数不能直接相加作为组合耗时。 |
| 1000 B UTF-8 frame Encode，+47.8% | package + body 为 1014 B，低于 FF 批量扫描的 1024 B 门槛；调用 `pack_seg` 127 次，其中 125 次属于 FF word。原生 attach 可消除部分手工组包复制，但 Pack 开销仍存在。 | 首要问题是批量扫描门槛造成的性能断点，字符串编码是次级候选。 |
| 4 KiB ASCII frame Decode，+55.0% | packed 4109 B，分配 8218 B，实际输出 4112 B，只扫描一次；专项 unpack profile 中 FastBuffer self 约 57.4%。 | 优先处理分配与私有缓冲复用，不再重点优化已很快的字符串解码。 |
| 4 KiB 零字节 frame Decode，+173.0% | packed 519 B，先分配 1038 B，扫描得到 4112 B 后再分配 4112 B，并重新扫描和填零；unpack profile 中 lunpack self 约 56.6%、FastBuffer 约 26.1%、fill 约 11.0%。 | 二次扫描、重复分配及填零共同造成成本。不能只改为统一的精确长度预扫。 |

这些 self 百分比来自各自独立 profile，不能跨场景相加，也不能直接换算成预计收益。数字数组专项中的 PB 控制值和历史值也有较大差异，不能据此重写正式排名。

## 3. 官方与 Protobuf 的可借鉴做法

### 官方 Sproto / Skynet

`sproto_pack` 对 FF 连续块通过 `write_ff` / memcpy 批量复制；普通混合 word 仍逐字节生成 mask。`sproto_unpack` 写入调用方提供的缓冲。

Lua 绑定的 `pushfunction_withbuffer` 把可扩容缓冲保存在 C closure upvalue 中。`lunpack` 只有在已有容量不够时才扩容和重扫，后续调用能复用已有容量；最终 `lua_pushlstring` 创建独立 Lua 字符串。它不是每包都按 packed 长度乘二重新分配。

GSR 可以借鉴“内部复用、输出独占”，但 TS 不能把可变 Buffer 当成不可变 Lua 字符串直接交出去。不能照搬 Lua C 指针、全局共享缓冲或无上限容量保留。

### Protobuf-ES 2.15.0

`BinaryWriter` 使用位置游标，容量不足才增长，固定宽度数值通过缓存 DataView 写入；最终 `finish()` 复制为独立输出。`BinaryReader.bytes()` 返回输入的 subarray，在同步解析中避免不必要的中间复制。字段 reader / writer 在 schema 首次使用时确定并缓存。

可直接借鉴 DataView 写数值、同步阶段借用字节和稳定 schema 信息复用。Sproto 的固定宽度数组、头/body 分区与 0 packing 仍遵循自己的规则；PB 不承担 0 packing，不能要求对应额外开销凭空消失。

参考源码：

- [官方 sproto.c](https://github.com/cloudwu/sproto/blob/e93fd5a0653b2bcab38d3b830958bf2e3a901b5f/sproto.c)
- [官方 lsproto.c](https://github.com/cloudwu/sproto/blob/e93fd5a0653b2bcab38d3b830958bf2e3a901b5f/lsproto.c)
- [Protobuf-ES BinaryReader / BinaryWriter](https://github.com/bufbuild/protobuf-es/blob/v2.15.0/packages/protobuf/src/wire/binary-encoding.ts)
- [Protobuf-ES 编码器](https://github.com/bufbuild/protobuf-es/blob/v2.15.0/packages/protobuf/src/to-binary.ts)
- [Protobuf-ES 解码器](https://github.com/bufbuild/protobuf-es/blob/v2.15.0/packages/protobuf/src/from-binary.ts)

## 4. 推荐实施顺序

### P1：先修 FF 批量扫描门槛

定位：`client/src/vendor/sproto.ts` 的 Pack FF 起始分支，当前条件 `srcsz - i >= 1024`。

在已确认进入 FF 后，允许更短的连续非零块使用现有有界 `nonzeroWords` 扫描。以 64 B 为已验证候选，再用 32/64/128/256 B 和 1 KiB 邻域测门槛；不新增公开配置，不按消息名称特判。

保持每段最多 256 word、尾部补零、FF 中 6/7 个非零字节的合并规则，以及官方固定 wire。只跳过已确认全非零的完整 word，遇到零字节后继续原规则。

仅把该门槛改成 64 B 的隔离原型，UTF-8 attach 编码从 3.384 降至 1.933 µs（-42.9%）；4 KiB ASCII 约 1.705 → 1.678 µs。这里只证明方向有效，尚未完成全边界回归，64 B 不视为最终参数。

### P2：数值数组保留预检，写出使用固定宽度循环

定位：`encode_integer_array`。当前首遍已校验类型、整数精度、位宽和总长度，随后逐项 `Buffer.writeUInt32LE/writeInt32LE` 又进入 Node 通用值域检查。

推荐在预检后按 4 / 8 字节拆开写出循环，候选为 `DataView.setUint32/setInt32(..., true)`。DataView 必须绑定真实 buffer、byteOffset 和 byteLength，扩容后重建；是否缓存视图，结合每数组构造成本测试。不使用要求对齐的 Uint32Array 直接解释 wire，不取消首遍值域和长度检查。

8 字节路径继续按 low/high 写出，保留负数符号扩展及 Number 安全整数边界。普通整数类型、混合正负 int64、首/中/尾宽值、空数组、非零 byteOffset 都要验证。

隔离原型在已有三类数值消息中均有收益，但原实现样本出现明显抖动（整数数组单纯编码约 20–119 µs），本次不据此发布收益百分比；实施后必须稳定复测。

混合 word Pack 另做局部尝试：完整 word 与尾部分开循环，尾部用带界读取替代每次分配 8 B 临时 Buffer；保留当前八字节展开和 FF/零块快路径。450 个 word 中没有 FF，不能指望 P1 解决它。上一轮已回退的算术 mask 方案不原样重做。

### P3：内部同步解码借用有界解包缓冲

借鉴官方 Lua 绑定的持久缓冲，但限定在 Sproto 实例内部。私有 unpack 路径供 `pdecode` / `dispatch` 同步消费；公开 `unpack` 仍返回独占结果，不能返回下次调用会覆盖的 scratch。

内部借用结果不能进入业务对象、闭包或异步任务。先检查所有解码字段与返回路径不保留 Buffer 视图；未来增加 binary 字段时同样必须保持输出所有权。

容量按实际需求增长，并设置内部保留上限；超出保留上限的大消息使用调用内临时缓冲。不能让一条偶发 8 MiB 消息令每连接实例永久保留 8 MiB。上限以并发连接数 × 保留容量和冷热混合测试选择，不新增业务配置项。

同步帧解码隔离原型（同一方法下的当前实现 → 借用原型）：

| 输入 | 当前 µs | 原型 µs | 耗时变化 |
| --- | ---: | ---: | ---: |
| 4 KiB ASCII | 1.348 | 0.675 | -49.9% |
| 4 KiB 零字节 | 2.559 | 1.279 | -50.0% |

原型只检查固定夹具等价性，测的是热缓冲下 codec 同步解码，不是完整客户端；它不是可直接合入的实现。验收必须增加冷启动、大小交替、超保留上限和失败后复用。

### P4：单独优化公开 unpack 的容量选择

不能把 P3 的内部借用收益套到 `unpack` 的公开字节返回值。

当前近乎未压缩的 ASCII 也分配 packed × 2，容量约是有效输出的两倍；优先试验轻量尺寸扫描、按确切长度分配，并保留高压缩率输入原路径作为比较。

本次“统一先计数，再分配和解包”的原型，ASCII unpack 为 1.018 → 0.691 µs，但零字节为 2.048 → 2.581 µs（+26.0%），因此不推荐统一采用。内部 scratch 后复制独占输出在 32 B 小包也可能回退。

优先目标是减少明显过量分配；最终选择必须由低/高压缩率和小包共同决定。保留有界重扫作为回退是可接受结果，不为了消灭第二遍扫描建立复杂的新解包框架。

### P5：前四步后再判断字符串与 schema 专门化

Sproto 当前字符串为 `Buffer.byteLength` 后 `Buffer.write`；PB 使用预留容量、encodeInto，再回填长度。Sproto 固定四字节长度，理论上可先预留、编码后回填，但按 UTF-16 长度 × 3 预留会放大内存，也可能错误拒绝实际编码后仍在额度内的消息。

仅在 P1 后 UTF-8 编码仍明显落后、profile 仍指向字符串时尝试。保留精确长度回退与现有 Unicode 行为，不统一替换已占优势的 ASCII 编码。类似地，字段专用 reader/writer 只在新 profile 证明收益后局部引入，不因 PB 有缓存编译器就再造一套框架。

## 5. 验收条件

1. 扩展已有 API 基准，同时列出 PB、SP 分阶段组合、SP 原生 pencode/attach/pdecode；同输入交错五轮，保存全部样本和源码指纹，历史正式数据不覆盖。
2. 复用现有官方向量、所有八位 mask、高位非零字节、FF 256-word、尾部和错误输入差分；门槛邻域、非零 byteOffset、输出快照和 scratch 复用仅在未覆盖处扩展。
3. 保留现有整数精度、8 MiB 解包/编码预算、package/body 额度、未知字段、optional presence、失败与输出所有权契约，不删除外层安全预检换速度。
4. 内部借用同时测冷/热缓冲、32 B 小包、UTF-8、4 KiB ASCII/零字节、稀疏和高压缩率数据；报告每实例保留容量与并发连接下内存成本。
5. 每个局部切片先跑对应回归；连贯切片完成后运行现有 TS 官方差分和 Go/TS Plain/MAC 集成。CPU/分配 profile 与计时分开；持续回退或不稳定结果不算通过。

## 6. 实施取舍

- FF 门槛采用 64 B；数值数组保留预检，按 4/8 字节使用缓存 DataView 写出，扩容后失效；Pack 尾部取消临时 Buffer。
- pdecode/dispatch 按需建立解包缓冲，单实例最多保留 16 KiB。大包使用临时输出，公开字节结果仍独占。
- 公开 unpack 不能统一按大包预扫：真实数值消息回退约 36%；仅有长 FF 前缀的判定也会使混合包回退。最终最多查看前八段，只有 FF 字节已覆盖整个 packed 输入的四分之三时才精确分配。其余保留原路径。
- 新增 29 种可复现解包形态门禁；完整采样分别列出 PB 控制、SP 原生入口和分阶段组合，不混用调用形式计算收益。
- P5 暂不扩展。UTF-8 RPC 编码已改善，保留当前字符串写出与字段模型；数值 Pack 和公开零块解包仍有后续空间。

未升级依赖，未改变 Gate / Runtime / 协议边界。第三轮已本地提交为 GSR `72fc11a`，gosproto 保持 `6012b06`，未推送或发布。

## 7. 后续零头计数与基准输入修正

第四轮仅在连续零 mask 路径确认四个零头后展开计数，尾部继续逐字节处理；尺寸检查、有界输出前缀和既有缓冲所有权不变。公开 4/16/64 KiB 零块解包耗时中位数分别减少约 30%/37%/45%，4 KiB 零字节原生 RPC 解码约减少 21%。新增重复短零段与零前缀混合输入后，共 46 种解包形态、222 项操作、1110 个样本，没有 SP 项的耗时中位数回退超过 10%；这不是统计显著性结论。

无分支、先算 mask 和双字节查表的 Pack 候选均因数值消息回退撤回；Unpack 原生长零块比较会拖慢重复短段，也未采用。最终没有增加查表内存、缓存状态或生产 Pack 改动。

排查发现原 Echo 基准为 PB 和每个 SP 版本分别 repeat 文本，相同值在 V8 中可能成为平坦或拼接字符串，造成编码源码相同却耗时相差数倍。现在准备期只构造一次并共享输入，重新完整采样；不修改生产编码器或历史结果。历史 Echo 编码百分比需考虑输入表示因素，代表性 body 原先已共享输入。完整证据、原始值、官方对照及复现方式见 [第四轮验收](benchmarks/2026-10-15-sproto-round4/RESULTS.md)。第四轮已提交为 `d9655b8`，未推送。

## 8. Pack 字节分布与公开路径验收

后续优先级采用 Pack 热路径、public/manual 解包、零块 Unpack。先看编码后的字节分布：896 项整数数组的 450 word 没有全非零 FF word；大型游戏消息 417 word 中只有两个全非零 word，主要为每 word 四至六个非零字节。因此这一热点是通用混合字节 Pack，不按数值类型或消息名称特判。

第五轮保留八字节条件写出，把 FF 六/七字节 word 的合并规则集中到段状态所在循环，普通混合 word 提前结束段处理。原容量保护、FF/零块批量扫描、尾部补零和 wire 不变。小表压紧、switch、游标与短循环候选未获得稳定收益，不合入。

公开/私有解包省略 subarray 视图的候选均因完整形态采样中的回退撤回。最终公开 unpack、私有解包和 lunpack 方法体与第四轮基线逐字节相同，不改变独占输出或 16 KiB 保留上限。Buffer 观测没有显示公开路径存在可直接删除的数据复制；分配和重复扫描成本仍需按调用入口区分。

基准收紧为同操作的 PB/SP 前后版本紧邻采样，并在奇数轮反转；新增 14 种 Pack 字节形态，连同 46 种 Unpack 形态共 250 项、1250 个样本。大型游戏与整数数组 Pack 约 -7.3%/-4.9%，FF 六/七字节混合段约 -35%；没有 SP 项耗时中位数回退超过 10%。五种代表性消息的原生组合编码/解码当前为 10/10 低于 PB，不扩大为协议整体或 Go/浏览器结论。阶段中位数相除不等于业务 CPU 占比。

增加独立诊断脚本记录 mask 分布、显式 Buffer 工厂/视图调用以及 Pack/PEncode CPU profile。实际 pencode profile 中，整数数组 Pack 累计样本占比约 73% → 70%，大型游戏消息约 27% → 29%；后者其余编码工作仍占主要部分，不把两类消息一概归为 Pack 主导。最终数据、失败候选和口径限制见 [第五轮验收](benchmarks/2026-10-15-sproto-round5/RESULTS.md)。第五轮已提交为 `9801808`，未推送。
