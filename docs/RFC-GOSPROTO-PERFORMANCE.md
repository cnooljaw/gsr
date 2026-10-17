# gosproto 后续性能优化方案

状态：2026-10-17，GSR 正式依赖仍为 v0.4.0，后续独立库优化尚未发布。TS 长块和 Go 安全路径见 [首轮结果](benchmarks/2026-10-15-sproto-next/RESULTS.md)，数值 packing、字段查询及类型化构造见 [第二轮结果](benchmarks/2026-10-15-sproto-round2/RESULTS.md)。TS 优化阶段已在 GSR `9801808` 收尾，可迁移经验见 [TS → Go 优化经验](SPROTO-OPTIMIZATION-LESSONS.md)。编码执行层改进见第 9 节，大型 Encode 约 6.54 µs，同场 PB 7.59 µs。混合 word Pack 已本地提交为独立库 `7dc6713`、GSR `e9e1f74`，未推送；长 FF 有界搜索已本地提交为独立库 `07350a9`、GSR `3a78dbd`，见第 11 节。连续零 mask 解包已本地提交为独立库 `22e287e`、GSR `1af93f4`，见第 12 节。普通 Unpack 五个候选已撤回，回归与记录已本地提交为独立库 `d66644a`、GSR `acfb070`，见第 13 节。安全 inline 处理已本地提交为独立库 `70d985c`、GSR `d0328fd`，见第 14 节；后续整数记录 parser 已撤回，原型与提案已本地提交为独立库 `5be3b26`、GSR `2872d46`，见第 15 节；私有 Decode 写入及验收已本地提交为独立库 `fc3246a`、GSR `862fa8b`，见第 16 节；后续四个局部候选均未采用，回归/记录已本地提交为独立库 `a09fb00`、GSR `d1beb28`，见第 17 节。最终入口专项未达门槛，Decode 性能阶段已按用户要求冻结于 `a09fb00`（Decode 生产源码同 `fc3246a`），见第 18 节。公开 API、generator 与预算/所有权契约不变。

第三轮已完成当前基线与候选筛选，只保留调用内布局记录的内存收窄，独立库已本地提交为 `a579920`，GSR 记录提交为 `fdbc736`，均未推送/发布；未取得新的明显 CPU 加速。实测和取舍见 [第三轮结果](benchmarks/2026-10-16-gosproto-round3/RESULTS.md) 及第 8 节。

前两轮以已发布的 gosproto v0.4.0 为版本基线；下一轮从已提交的 `6012b06` 做增量，不从 v0.4.0 重复实施。实现改动应在独立 gosproto 仓库进行，GSR 提供真实接线和跨语言 Benchmark；不修改 Go module cache，也不将库源码复制到 GSR。

当前切片（2026-10-18）：有响应请求融合已本地提交为 gosproto 6d854d1 / GSR f715e6b，见第 33 节，未推送。后续只刷新融合后的八条body/RPC编码profile，定位记录数组遍历、整数数组写出/Pack与512 B布局外溢，见第 34 节；生产代码未改，诊断记录未提交。正式 GSR 依赖仍为 v0.4.0，Decode阶段性冻结。

## 1. 目标与证据

优化重点由“减少临时分配”转到“减少每条消息的重复反射、动态判断和遍历”。保持公开 API、官方 wire、默认及自定义资源预算、optional presence、错误契约、失败原子性与输出独占。

正式版本基线见 [四种实现完整复测](benchmarks/2026-10-15-v0.4.0/report.md)。大型游戏消息纯 body 编码：Go PB 7.756 µs、Go SP 26.973 µs、TS PB 22.781 µs、TS SP 17.853 µs。Go SP 已降到一次编码输出分配，仍有明显 CPU 差距，继续只压 allocs/op 不足以解决问题。

与 v0.4.0 相同源码的 [Go profile](benchmarks/2026-10-15-optimized/profile-go-summary.json) 显示，编码 `encodedSize` 累计约 28.6%，`inlineHeader` 约 18.5%，反射 Elem/Index/FieldByIndex 也占明显成本。累计指标存在调用关系，不能相加，也不能把 encodedSize 全部算作必要的安全检查。解码分配主要归于 reflect.New；其中一部分是当前 optional 指针和独占结果对象的真实需求。

## 2. Protobuf 实际采用的做法

参考本次 Benchmark 正在使用的 google.golang.org/protobuf v1.36.1，而非未固定版本的主分支：

| 做法 | 源码证据 | 对 gosproto 的启发 |
| --- | --- | --- |
| 初始化一次字段编解码信息 | [makeCoderMethods / coderFieldInfo](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.1/internal/impl/codec_message.go) | 字段类型、访问位置和处理函数提前确定 |
| 按字段实际类型绑定专用函数 | [fieldCoder / pointerCoderFuncs](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.1/internal/impl/codec_tables.go) | 减少热循环中的 Kind、Wire、指针和 signed/unsigned 判断 |
| 先 Size，再预留缓冲，写出时使用尺寸信息 | [proto marshal](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.1/proto/encode.go)、[sizePointer / marshalAppendPointer](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.1/internal/impl/encode.go) | 保留预算预检，复用它已经算出的结果 |
| 低编号稠密字段使用数组，其他字段使用 map | [unmarshalPointerEager](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.1/internal/impl/decode.go) | 减少解码的逐字段 map 查询，限制元信息大小 |
| 原生类型的批量循环和小型 wire 函数 | [codec_gen.go](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.1/internal/impl/codec_gen.go) | 数组循环尽量操作数值，而非逐元素重新解释反射类型 |
| 使用字段 offset 与类型化指针 | [pointer_unsafe.go](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.1/internal/impl/pointer_unsafe.go) | 解释其部分速度来源；本轮不直接采用 unsafe |

现代 Go Protobuf 在这里是“生成类型及描述信息 + 运行时初始化专用 codec”，不能简单概括为每个消息都生成了一套 Marshal/Unmarshal 函数。它也存在 Size 与 Marshal 的多次扫描，部分 packed 数组还会重新计算长度；扫描次数必须结合每次操作成本一起看。

### 不能直接照搬的部分

- Protobuf 的尺寸缓存依附消息类型及其内部约定。gosproto 接受普通 Go struct，不能把上次消息值、尺寸或 reflect.Value 缓存在全局 schema，更不能按对象地址跨 Encode 调用复用。
- Protobuf 默认 Unmarshal 会先 Reset 并直接修改目标；gosproto 解码失败要保留原值，继续解到临时根对象，完整成功后才提交。
- Protobuf 的 unsafe 字段访问和生成类型布局是一整套配合。直接复制 offset/pointer 技巧会扩大维护与兼容成本，不能把它当作本轮普通局部实现修改。
- Sproto 有头部与 body 分区、inline 值和数组统一宽度，不能照搬 Protobuf 的逐 tag 追加 wire。借鉴字段处理方式，保留 Sproto 自身布局。

### Sproto / Skynet 参考

已检查只读参考 `../skynet/lualib-src/sproto/sproto.c` 的 `findtag`、`sproto_encode` 和 `sproto_decode`：C 实现保存预解析字段元信息，连续 tag 可直接索引，否则二分查找；编码先预留最大头部，再通过 callback 取值、写 body，最后 memmove 压紧头部空隙。C/Lua 依赖原生指针、连续内存及 C callback 取 Lua 值，不承担 Go reflect.Value 和 GC 对象分配成本。

Gosproto 可以借鉴元信息复用和字段查找方式；不默认照搬最大头部预留及末尾整体搬移，因为这可能增加输出预留、复制和额外缓冲。官方 wire 验收仍固定 cloudwu/sproto `e93fd5a0653b2bcab38d3b830958bf2e3a901b5f`，不复制固定官方版本已知的 64 位解码缺陷。

## 3. 推荐实施顺序

每步独立测量，确认收益后再进入下一步。第一轮限于安全 Go、现有 struct tag 和现有生成物，不新增公共 Codec 接口、Size API、选项、消息包装或注册层。

### P0：固定基线并先检查字节操作

复用现有代表性消息和 Echo 套件，不重建另一套不同数据的性能排名。保持分阶段测量 Encode/Decode/Pack/Unpack，不能只看组合路径。

优先核对 `writeUint16/32/64` 与标准库 `binary.LittleEndian.PutUint*` 的内联及边界检查消除结果。目前读取已使用标准库，写入仍是手工逐字节赋值。是否替换，以编译器诊断和数值数组实测为准，不预设标准库一定更快。这一步不涉及反射结构和状态所有权，便于独立验收。

### P1：初始化时确定字段操作，缩短热路径

Gosproto 已有 schema 缓存和部分 decoder 绑定；在它们上面改进，不重建一套 schema 系统。

具体调整：

1. 将指针/值、有符号/无符号、目标位宽、数组元素类型和子消息 schema 等稳定信息提前确定。解码已有绑定，但通用 setInteger/targetValue 仍反复解释目标类型，应逐项拆出热点。
2. 每次处理结构体只取得一次根 Elem；按实际字段索引直接取字段，减少重复 FieldByIndex。当前元信息来自逐个 StructField，优先使用直接字段编号，保持嵌套结构原有解释，不引入新的匿名字段展开规则。
3. 遍历仅参与协议的有序字段，跳过未打 tag 的字段放在初始化阶段完成。
4. 为 size/write/read 选好字段操作。内部可使用少量具名函数或预计算枚举；间接函数调用可能阻止内联，需与简单 switch 实测比较，不机械将所有分支改成函数指针。
5. 不改全局缓存的并发与失败回滚机制作为顺手优化。递归 schema 不能读到半初始化状态；仅在实际 profile 显示缓存锁成为热点后再单独处理。

内部元信息只保存类型与算法；消息数据、预算、输出和预检结果均属于当前调用。公开 GetSprotoType / Fields 的既有行为和调用方也需核对，不能借内部缓存优化悄悄新增或改变外部修改元信息的契约。

### P2：专门化数值数组和标量循环

数组每个元素的 signed/unsigned、位宽与转换路径不应重新通过完整 Kind switch 决定。

- 元素类型固定后，整数读取、范围检查、4/8 字节写入以及解码窄化各走对应路径；保留每个元素必须进行的值域检查。
- 空数组、负整数、首次宽值在首/中/尾、int32 极值、int64 极值、uint64 超界拒绝均保持原契约。
- 优先测试 Go 原生类型批量循环。若使用 Interface 类型断言，需测量是否引入装箱分配；用户定义的命名标量/切片类型继续支持，不因快路径只识别内建类型而失效。
- 保留 double 原始位模式，包括负零、NaN payload 和无穷；不借统一整数函数改变浮点解释。

先看 int32_edges 和 game_large 的纯 Encode/Decode。出现新的 slice 装箱分配、小数组回退或大量重复类型函数时，应调整或撤回该路径。

### P3：复用预检结果，合并头部与 body 的写出遍历

当前编码通常执行：尺寸/预算预检 → 遍历写头 → 再遍历写 body。第二、三阶段重复访问字段和判断 inline。

推荐保留完整预检，在写出阶段维护两个游标：

```text
预检：校验并确定尺寸、实际头项数等必要布局
分配：最终输出缓冲
写出：headerCursor 与 bodyCursor 同时推进
      每个字段在写出阶段只取一次值
```

对每个嵌套结构，必须知道实际头项数才能确定 body 起点；只缓存根头长度不足以消除子消息重复遍历。

布局记录只保留必要数值，例如结构体尺寸/头项数、数值数组宽度；不默认保存每字段 reflect.Value 或复制整份业务对象。尝试调用内的小型 scratch，超出后按实际需求有界扩展，不按 MaxElements 一次预分配，不使用无界对象池。

需要明确测量布局的代价：中型/大型消息可能新增少量临时分配，能否换来足够的 CPU 收益要看 B/op、allocs/op 和耗时一起变化。小包尽量保持栈内；若元信息管理比重新检查更贵，保留 P1/P2 的简化路径，撤回该尝试。一次分配不是唯一验收指标，也不能用显著增加临时内存换漂亮的 ns/op。

数组宽度可以由预检传给写出，减少宽度重新判断与 4→8 字节扩展搬移，但不会消除“读一次确定宽度、再读一次写数据”的必要数据处理。

失败原子性继续通过“预检失败不返回部分结果、完整成功后返回独占输出”保证；本轮不暴露供调用方持久化的 PreparedMessage 或 Size cache。

### P4：解码的字段查找与赋值

- 借鉴 Protobuf 的限定稠密表：低编号、稠密 tag 可用小数组，稀疏部分保留 map；必须设置内部大小上限，不为单个 tag=65535 分配巨大稠密表。
- 将目标类型的位宽、signedness、可分配元素类型等保存在字段信息中，减少逐元素 Type/Elem/Kind 查询。
- 保留临时根对象、未知字段跳过、inline 类型校验、深度和元素预算，以及 decode 消费长度。数组边界预检不能因追求单遍而取消。
- 不把真实 optional 字段改成值字段，不共享上次消息的子对象，也不把 buffer 借用当作独占快照。

字段查询不是当前已证明的最大热点，因此排在字段访问与数组循环之后；以新的 profile 决定是否值得保留额外索引结构。

### P5：Pack/Unpack 单独优化

Protobuf 没有对应的 Sproto 0 packing，不能把 PB 编码循环直接移植到这里。Go 当前已有 FF 批量复制、零初始化输出、精确解包分配和位扫描；不重复建设已经实现的路径。

优先检查完整零 word、FF 连续块、混合 word、末尾不足 8 字节和 256 word 边界。是否增加 word 级分支，以 ASCII、零字节及游戏数值输入分别测量；继续检查内联预算，避免重现 v0.4.0 优化过程中 packSegment 无法内联造成的回退。

TS 的零块与双遍解包优化属于独立切片，使用同一套 wire 和回退门禁；不与本轮 Go 修改混在一个性能差值中归因。

## 4. 验证与验收

先复用独立库已有的官方向量、C/Lua 对照、资源边界、schema 失败回滚、命名类型、字段演进、失败保留原值及三个 fuzz 入口。只在确有缺口时扩展最相关用例，例如：

- 新布局结果不跨 Encode 调用复用，同一对象修改字段 presence、数组长度/宽度后再次编码仍正确。
- 新稠密查找包含未知 tag、稀疏高 tag，与旧 map 行为一致。
- 类型化数组快路径兼容命名类型，保留整数窄化/溢出和 double 位模式。
- scratch 超出栈内容量时仍受限，深度/字节/元素预算边界不变，小包不被大包状态污染。

不为内部函数形状编写镜像测试，不在 GSR 再复制整套底层编解码回归。

开发循环先对本步相关测试和专项 Benchmark 验证。连贯切片完成后运行独立库完整回归、race、vet、固定 C/Lua 对照和有限 fuzz；之后通过临时 modfile 验证 GSR 接线。发布后才升级正式依赖，不提交本地 replace。

性能验收：

2026-10-17 按用户裁决修正：取消后续实验中引入的“Pack 至少 5%、PEncode 至少 2%”统一淘汰线。它们是当时的筛选目标，不是收益是否有价值的边界。历史数据和当时判定保留，候选可按以下原则重新评估：

- 小而可靠的改善可以累积；简单局部修改即使只有 1%～2% 也可以保留，不要求每个阶段和组合路径分别超过固定比例。
- 分开判断收益幅度、测量可信度、真实业务效果和维护代价。控制异常表示该项证据待确认，不等同于候选没有价值或已经证明回退。
- 同时报告绝对耗时、逐轮方向、分配与回退；已确认的 GC/tiny 分配计数边界单独解释，不把工具的单个布尔字段当作最终工程决策，也不删除原始计数。
- 优先保留不增加抽象、资源或维护负担的局部改善；新增另一套解析器或显著分支复杂度需要更充分的实际收益依据。
- 对有希望但证据不足的候选保留补丁并标为待确认，采用预先确定的独立复测，而不是反复运行挑选最好结果。分别确认后再验证组合的累计收益与回退。
- 正确性、官方 wire、预算、失败原子性和所有权要求不变；真实且有实际影响的回退需要处理或明确权衡。

1. 每步保存同机器、同消息、同版本工具链的前后五轮原始值、源码指纹及分配数据；计时与 profile 分开。
2. 优先比较 AddressBook、game_large、int32_edges 的纯编解码，另测 32 B 小包、UTF-8、4 KiB ASCII/零字节、packing 与完整帧。
3. 改善必须在对应完整消息路径中成立，不能仅依靠孤立微基准。明显或持续的小包回退、元信息分配膨胀都需要撤回或缩小适用路径。
4. 独立 Go 切片稳定后重跑现有 Go/TS × PB/SP 两套完整基准，报告收益、无改善与回退；TS 未修改时不把运行噪声算作 Go 优化收益。
5. 不承诺一轮内追平 PB。若剩余主要差距来自安全反射，拿具体 profile 和收益估计再讨论代码生成或受控 unsafe；它们是后续单独裁决，不在当前方案中默认启用。

## 5. 首轮实施范围

| 阶段 | 实施与取舍 |
| --- | --- |
| TS | 长 FF 块使用有界原生非零扫描，长零块批量处理；解包合并连续零 word，首次输出空间耗尽后继续校验和计数。普通数值 word 保留原路径。 |
| P0 | 改用可内联的 `binary.LittleEndian.PutUint*`，保留最终缓冲直写。 |
| P1 | schema 缓存指针形式、nilability、signedness 和目标类型；每个结构体只取得一次 Elem，直接按字段编号访问。不修改缓存锁和递归失败回滚。 |
| P2 | 原生 `[]int64` 使用安全类型断言与批量循环；命名类型及其他整数类型保留反射兼容路径。标量 int64 解码跳过重复位宽查询，其他类型仍检查窄化。安全类型化访问影响逃逸分析：Echo 每次新建的 package 额外分配 16 B / 1 次，完整小帧编码耗时仍改善，代价明确保留。 |
| P3 | 当前调用记录各结构体头项数和非空整数数组宽度，写出使用双游标；32 项栈内记录，溢出后按实际记录数扩展。大型游戏消息新增 1024 B / 1 次临时分配，换取完整编码路径的 CPU 收益。 |
| P4 | 低 tag 按密度选择最多 128 项的索引，其余继续使用 map；仍从现有 Fields 取字段，不缓存调用方替换前的字段指针。 |
| P5 | Go 完整零 word 快路径在 ASCII/数值输入回退，已撤回，`pack.go` 与 v0.4.0 一致。 |

布局记录不保存业务对象或 reflect.Value，不按对象地址跨调用复用。完整预检、预算、临时解码根对象、失败原子性、optional presence、输出独占和官方 wire 均保留。没有将所有整数/浮点类型机械复制为专用 codec；后续是否扩大专门化范围，仍需真实消息和 profile 证据。

本轮扩展现有整数范围、命名数组、未知/高 tag 测试，并新增同一对象改变 presence、数组宽度和嵌套数量后重编码的回归。完整采样、profile、复现方式与未改善项见实施结果；正式 v0.4.0 的历史数据保持不变。

## 6. 第二轮实施范围

按“数值 Pack → TS 字段查询 → Go 标量写出 → Go 结构体解码 → 完整帧与解包”推进：

- Go Pack 对完整 word 生成精确非零 mask，按有效字节写出；FF 连续全非零 word 保留原快路径，尾部仍按实际长度处理。TS Pack/Unpack 使用固定八字节展开，减少普通数值 word 的循环开销。
- TS 每条结构只取一次 schema，低 tag 使用最多 128 项稠密索引，其余使用 Map；数组直接使用已解析的 elementType。同时修复原先未知 body 字段不消费内容、导致后续字段错位的问题，增加跨 schema 的低/高 tag 回归。
- Go 标量一次读取即可完成 inline 判定和非 inline 写出；原生 *int64 / *string 使用类型化构造，命名类型保留反射路径。缓存子结构 Go 类型，避免结构体数组逐元素查询类型。分配对象仍独占，未引入 slab、池或 unsafe。
- TS pencode 与实际 attach/Response 编码在内部 scratch 中接续写入，再返回 Pack 的独占输出；公开 encode 仍复制输出。扩容仅保留已完成的 package，并保留 package/body 原有的独立编码额度，避免合并缓冲挤占 8 MiB body 预算。
- Go 解包查表、TS 预清零输出后只写非零字节的尝试分别在数值或 ASCII 输入回退，均撤回；高压缩率路径继续采用首轮的有界计数和重分配。

原有两套基准继续保持逐阶段调用方式，TS `pack(encode(...))` 和手工组帧仍计入其中间复制成本。新增小型 [API 基准脚本](../examples/websocket-gate/scripts/benchmark-sproto-api.mjs) 复用相同夹具，单独测量已存在的 pencode 和 attach，不以不同调用路径替换旧样本。报告分别保存最终原始值、来源和撤回的尝试。

## 7. TS 阶段收尾后的 Go 增量入口

TS 当前阶段基本完成，默认冻结于 GSR `9801808`。其五种代表性消息 × pencode/pdecode 为 10/10 低于同进程 PB，但这是 Node.js 中的实现结果，不是 Go 的性能预测。下一轮采用 [优化经验](SPROTO-OPTIMIZATION-LESSONS.md) 中的输入、所有权和测量规则，重新回答当前 Go 的瓶颈在哪里。

### G0：绑定当前实现并重建基线

- 独立 Go 起点为 `6012b06533657721aff66e1ea16ab158153f6e44`，正式依赖 v0.4.0 作为另一份固定对照。保存工作树状态及实际源码哈希，不能只记录 go.mod 的 require 行。
- 通过临时 modfile 接入当前库；用相同 GOFLAGS 执行 `go list -m -json github.com/cnooljaw/gosproto`，保存 Replace/Dir 与 [环境采集器](../examples/websocket-gate/scripts/benchmark-environment.mjs) 的实际源码指纹。不提交 replace，不修改 module cache。
- 复用现有 [代表性消息与 Echo 基准](../examples/websocket-gate/benchmarks/README.md)，固定同一 Go 工具链、GOMAXPROCS 和 PB 版本。前后固定二进制配对、交替先后，保存五轮原始 ns/op、B/op、allocs/op；不把历史不同版本/输入谱的值拼接成当前收益。
- 对 Pack 的非零字节密度、FF 六/七字节转换和 Unpack 的前缀/稀疏尾部、重复短段，先查 Go 已有用例，再补实际缺口。TS 的 14/46 种形态及 9560 组差分不冒充 Go 已覆盖数量。

### G1：按当前 profile 选择切片

单独采 game_large、int32_edges、AddressBook 与小帧的 CPU/分配 profile，区分纯编解码、packing 和完整组合。TS 的整数数组 pencode 中 Pack 约占 70% 累计样本，大型状态约 29%，已经说明不同消息的优先级不同；Go 更不能按 TS 的占比直接排序。

| 当前证据若指向 | 优先检验 | 必须保持的代价与语义 |
| --- | --- | --- |
| 字段访问、反射或重复判定 | 已有元信息是否仍有可消除的 Kind/Elem/Field 查询、presence/inline 重读 | 命名类型和 nil/presence；类型事实可缓存，消息值与布局不跨调用缓存 |
| 尺寸/布局遍历 | 当前布局记录是否真正复用预检信息，是否存在昂贵重复扫描 | 完整预算检查、双游标和独占输出；同时记录布局分配，不唯 allocs/op |
| reflect.New/结果构造 | 原生类型的安全构造、元素一次分配，区分必要对象与临时对象 | optional 和命名类型、独占子对象、临时根对象及失败原子性 |
| Pack/Unpack | 当前 mask/有效字节写出、FF 快路径、普通 word 以及失败探测成本 | 高位字节精确 mask、256-word/尾部与预算；不得普遍重算 FF mask 或机械移植 TS 扫描门槛 |
| 小帧装箱或逃逸 | 构造一次与复用对象分别测量，检查类型断言和辅助函数的逃逸/内联 | 既有额外 16 B package 分配明确记录，不能通过复用本不允许共享的对象抹掉成本 |

前两轮已增加的布局记录、类型化访问和 tag 索引不再当作新方案重复提出；需要新 profile 证明哪里仍有重复工作。检查编译器内联、逃逸和边界检查结果，不默认函数指针、查表、无分支写出或池一定更快。

### G2：逐切片验收，再比较 PB

每次只改变一个主要机制，先复用最相关回归和有限专项，再进入受影响库验证与 GSR 接线；完整矩阵和验证层级沿用第 4 节。计时、CPU、分配与测试顺序执行，专项复核不替换失败全量。

收益要同时落在真实组合路径、合理的小包成本和可解释的内存代价上。即使没有超过 10% 的回退，也不忽略持续的小幅损失；反之，未改方法体的采样差异也不能直接归因于算法变化。TS 的公开/私有视图候选最终均撤回，是这一规则的直接案例。

稳定后重跑现有 Go/TS × PB/SP 两套完整基准；TS 作为冻结控制，不修改其实现来配合 Go 排名。是否能追平或超过 Go Protobuf 由数据回答，不以去掉预算、失败原子性或 ownership 换取结论。unsafe、generator、对象池或新公开 API 若确有必要，另行以具体热点和预期收益讨论，不因“剩余差距较大”默认启用。

## 8. 第三轮基线与候选筛选

TS 经验提取已在 GSR `f45a8c6` 提交。Go 从 `6012b06` 重建两套主基准，前后各 72 项、每项五轮；分别采 CPU/分配，并新增 18 种解包形态。当前整数数组纯编解码已经快于同次 Go PB，packing 组合仍落后；大型结构体的主要编码差距也仍存在。不能将 Go 概括为所有路径都慢，更不能根据 TS 结果承诺 Go 必然全面胜出。

解包八分支展开在大型输入改善，但 AddressBook 约回退 38%；固定窗口又暴露单/双字节稀疏 word 回退。增加稀疏分支后，真实 PDecode 收益仅约 1%–3%，不值得继续增加路径，已全部撤回。原生指针字段通过 Addr/Interface 直接赋值也在大型消息约慢 19%，恢复原路径；类型已知不等于这一读取/赋值方式更便宜。

最终只将 encodeLayout 的数值记录从 int 改为 uint32，合法头项数与数组宽度完整可表示。64 位环境下大型消息 Encode 4480 → 3968 B/op，PEncode 7936 → 7424 B/op，分配次数不变；CPU 基本持平。记录数量、32 项栈内空间的项数、增长策略、预算与 wire 都不变；Decode/Meta/Limits/Pack 生产源码与起点一致。

全量出现四项约 10%–13% 的耗时中位数回退，保留全部原始值；源码不变、固定 60000 次并配对 PB 控制的五轮复核约 -1%～+3%，未支持持续回退。两类样本不混用，不将内存收益描述为 CPU 突破。

独立库官方 C/Lua 对照、race、vet 与三个各约 10 秒的 fuzz 通过；GSR 临时依赖三包与 Go/TS Plain/MAC 集成 7/7 通过。两套主基准本轮只重测 Go，TS 源码指纹保持冻结；不宣称产生新的 TS 计时或四种实现全量结果。原始数据、源码指纹、验证和复现见 [第三轮报告](benchmarks/2026-10-16-gosproto-round3/RESULTS.md)。下一步继续针对大型 Encode 的字段访问、标量判定和预检/写出配合，不扩大既定 API 或安全边界。

## 9. 大型结构体 Encode 专项提案

用户允许重新设计 gosproto 内部实现。已对照官方 C/Lua、Skynet 与 PB v1.36.1，并做固定 Item 的隔离诊断：同组通用 Encode 14.631 µs，固定流程但仍用安全反射为 10.993 µs，类型化读取为 7.188 µs，分配均为 3968 B / 2 次。探针仅覆盖固定输入，不能作为生产胜出或通用重构已完成的结论。

推荐先将稳定字段事实绑定为内部编码操作，复盘通用分派与访问成本；若仍停留在 11–12 µs 且反射占主要成本，停止堆同类技巧，优先讨论受控 unsafe 类型化访问，生成器为备选。保留两遍预检、现有 wire 与预算，Decode 和 packing 冻结。完整证据、代价与待裁决点见 [编码执行层重构提案](RFC-GOSPROTO-ENCODER-REWRITE.md)。

### 第 9 节实施结果

按用户同意的顺序先验证安全操作码分派，候选没有稳定收益，已撤回。最终只为 schema 验证的原生 `*int64` 字段绑定 owner/offset，封装只读访问；公开 Fields 替换和形状变化保留原语义，命名类型/其他字段保持反射。两遍预检、预算、wire 和独占输出不变，Decode/Pack/Unpack/TS 生产源码冻结。

同进程五轮固定次数复核：大型 Encode 14.345 → 8.456 µs（-41.1%），同次 PB 7.863 µs，仍慢约 7.5%；大型 PEncode 18.301 → 11.880 µs。分配均不增加。32 B Echo 纯 body 增加约 6.6 ns，完整帧反而减少约 30.5 ns，代价明确保留；未声称所有入口都加速。完整矩阵 720 样本与同进程 245+45 样本分别报告，不混用中位数。

独立库官方对照、race/严格 checkptr、vet 与两项有限 fuzz 通过；GSR 三包及 Go/TS 集成 7/7 通过。32 位库构建通过，但旧测试的 int 常量溢出阻塞 32 位测试二进制；仅本机执行了运行验证。该切片已本地提交为独立库 `819047e`、GSR `bf3ed8c`，未发布或升级正式依赖。数据、来源、验证与复现见 [本轮结果](benchmarks/2026-10-16-gosproto-encoder/RESULTS.md)。

后续发现父 Fields 替换后子 schema 可能与实际 Go 布局不同，单独核对字段 owner 不够。当前调用内传递已验证类型事实，描述变化时核对实际类型或回落；同构整数结构数组在入口核对全部字段后复用，保留逐元素预算与 wire 规则。通用循环增加同构分支的候选因 AddressBook 回退撤回。

最终相对上一提交大型 Encode 8.382 → 6.528 µs，同场 PB 7.606 µs；相对最小类型修复版 9.730 → 6.544 µs，同场 PB 7.588 µs。AddressBook 相对上一提交 +12.0%、相对修复版 +1.3%，两组分别记录，分配不变。新增回归、结构数组 fuzz、完整库 race/checkptr/官方对照及 GSR 接线通过，该切片已本地提交为独立库 `5e491ff`、GSR `70c84af`。详见 [嵌套绑定与整数记录数组结果](benchmarks/2026-10-16-gosproto-bindings/RESULTS.md)。

## 10. Pack 混合数值 word 切片

从 `5e491ff` 重采纯 Pack 与实际 PEncode profile，确认成本集中在 packWord，且大型/整数数组主要是四至六个非零字节的混合 word。参考官方 FF 段规则和固定字节展开思想，只改变内部执行方式：稀疏 word 仍提取有效字节，混合 word 固定位置写出并按 mask 递增目标游标；用至多两个置位的判断替代完整计数，FF 状态集中在连续段内。尾块、256-word、输出预留、wire 和独占语义不变；未扩展 unsafe。

所有 word 使用条件展开的首个候选因稀疏/全零回退撤回。最终同进程 115 项 × 五轮：整数数组 Pack 3.664 → 3.287 µs（-10.3%）、PEncode 5.902 → 5.498 µs（-6.8%）；大型分别为 3.473 → 3.301 µs（-5.0%）、10.045 → 9.917 µs（-1.3%）。所有分配不变，无 SP 中位数回退超过 10%，但部分稀疏形态有小幅损失；不能将纯 Pack 降幅直接归给组合路径，也不代表 Go 全面胜出 PB。

扩展既有全部 mask 的 FF 合并/退出精确字节检查，9216 组旧新差分、官方对照与完整库 race/checkptr/vet 通过。源码、失败值、组合与小包代价、最终验收见 [Pack 结果](benchmarks/2026-10-16-gosproto-pack/RESULTS.md)。该切片已本地提交为独立库 `7dc6713`、GSR `e9e1f74`，未发布。

## 11. 长 FF 段有界搜索

混合 word 前缀位置计算的候选仅有约 2%～3% 纯 Pack 收益，组合没有稳定改善，未采用。当前只在已确认 FF 起点且剩余输入至少 256 B 时，使用标准库搜索后续最多 255 个完整非零 word。含零 word 留给原六/七字节规则，256-word 及部分尾块仍由原批量复制/补齐处理。主循环移除中间 input 切片和重复长度判断，输出预留及全部资源契约不变。

最终 137 项 × 五轮：4 KiB ASCII Pack 2.075 → 0.5851 µs（-71.8%）、PEncode 2.682 → 1.142 µs（-57.4%），完整帧 3.352 → 1.906 µs（-43.1%）；UTF-8 帧 -34.2%。数值组合基本持平，部分小包 Pack 约 +5%，不是所有场景都加速。直接内嵌扫描、中间五轮及容量收窄等失败/未采用数据单独保留，不混用中位数。

新增全部 mask 长输入的独立预期字节，17984 组差分、官方对照与完整库 race/checkptr、vet、有限 fuzz 通过。最终源码、接线、profile 和代价见 [FF 搜索结果](benchmarks/2026-10-16-gosproto-ffscan/RESULTS.md)。输入/输出和公开 API 不变，未扩大 unsafe。该切片已本地提交为独立库 `07350a9`、GSR `3a78dbd`，未发布；不因字符串收益扩大为协议总体或跨平台结论。

## 12. 连续零 mask 的有界解包

从 `07350a9` 重采 Unpack/PDecode profile，确认长零块仍消耗在逐 mask 预检和游标推进。两个连续零 mask 后按有效切片长度批量计数；预检 helper 的扫描范围由剩余输出预算加一个 mask 限制，超额在长度乘法及输出分配前拒绝。完整预检后仍按确切长度分配独占输出，零段批量推进；合并零、FF、普通分类，保留原错误优先级及 bounds check，不扩展 unsafe。

最终同进程 129 项、五轮 645 样本：4 KiB 零块 Unpack 1.620 → 0.5839 µs，64 KiB 23.940 → 7.593 µs；4 KiB 零字节 Echo 完整帧 2.891 → 1.411 µs，同场 PB 1.595 µs。数值业务组合基本持平，AddressBook Unpack 增加约 16 ns（约 10%）、PDecode 增加约 19 ns（1.6%）；普通稀疏/ASCII 形态的约 5% 代价与失败候选分别保留，不宣称全面无回退。

45824 组输入/预算差分、完整独立库 race/严格 checkptr/官方对照、vet、相关有限 fuzz，以及 GSR 三包和 Go/TS Plain/MAC 集成通过。Pack、Encoder、Decoder、RPC、元信息与 TS 生产源码冻结，分配次数和字节数不变。源码指纹、原始样本、profile、复现及未验证范围见 [解包结果](benchmarks/2026-10-16-gosproto-zero-unpack/RESULTS.md)。该切片已本地提交为独立库 `22e287e`、GSR `1af93f4`，未发布，正式依赖仍为 v0.4.0。

## 13. 普通 Unpack 候选筛选

从 `22e287e` 验证位置/计数表、仅计数表、源游标与固定输出区间、仅输出区间、单字节直接恢复五个候选。各 129 项、三轮，共 1935 样本；全部因稀疏回退或真实组合收益不足撤回，生产源码恢复为 `22e287e`。例如源游标使大型/整数数组纯 Unpack 约 -5.8%/-5.6%，但单字节形态 +12%；增加直接恢复后双字节 +14.1%，大型 PDecode 基本持平。没有进入五轮正式验收。

保留 TestPack 的 2048 组独立 mask 字节、后继 word 与切片偏移回归，已在未改生产代码时验证。额外 CPU/alloc profile 显示大型纯 Decode 仍为 532 次分配，主要对象归于原生整数指针和结构体构造；CPU 中的 kevent 不当作业务耗时。下一步以隔离探针拆分构造、reflect.Set 与 wire 遍历，保留真实输出所有权和失败原子性。未新增 unsafe、API、池或缓存。原始值、五个候选补丁及复现见 [筛选记录](benchmarks/2026-10-16-gosproto-unpack-candidates/RESULTS.md)；该切片已本地提交为独立库 `d66644a`、GSR `acfb070`；补丁格式属性修正为 `ae2e1bc`，未推送。

## 14. Decode 同分配诊断与安全 inline 处理

大型夹具同场 SP Decode 10416 B / 532 次、PB 13560 B / 537 次，不应把 SP 当前问题概括为“对象数明显更多”。固定 Item 的通用/反射/类型化探针保留全部分配，Decode 为 18.208 / 17.261 / 15.390 µs；构造微基准也显示相同 514 次分配下仍有明显执行成本。它们只用于区分固定流程与反射赋值，不作为通用 decoder 或与分段耗时相加。

从探针落地七行安全改动：已确认 inline 的原生 *int64 直接构造独占值并用 reflect.Set 赋值，省去重复 scalarBits 判定和间接分派；未使用 offset 写字段或新增 unsafe。完整 root 成功后才替换调用方值，预算、错误行为、presence、独占和分配不变。

最终 129 项五轮：大型 Decode 18.225 → 17.253 µs（-5.3%），PDecode 23.488 → 22.515 µs（-4.1%）；小型 Decode/PDecode 约 -4.6%/-3.1%。大型纯 Decode 低于同场 PB 17.840 µs，但 PDecode 仍高约 26.2%。主矩阵未改的极小零段 +13% 原值与后续 A/A 复核分别保留，不声称全项无回退，也不把未改 Unpack 的约 -14% 计作算法收益。

131068 组全部 inline 值/预算/坏帧/packed 差分、完整库 race/checkptr/官方对照、vet、有限 Decode/Codec fuzz 及 GSR 三包/GoTS 7/7 通过。指针独占回归增加 inline 边界与存在的零值。诊断、最终与筛选数据、控制、复现及验证见 [Decode 诊断与验收](benchmarks/2026-10-17-gosproto-decode-analysis/RESULTS.md)。该切片已本地提交为独立库 `70d985c`、GSR `d0328fd`，未发布，正式依赖仍为 v0.4.0。

## 15. 受控 Decode 写入的隔离验证

从 `70d985c` 验证整数叶子数组专用 parser：129 项三轮中，大型 Decode/PDecode 约 -3.1%/-1.3%，小型约 +1.5%/+1.4%。组合收益不足以维护第二份解析器，已撤回；仅保留新增稀疏 header 预算及 Fields 替换回归，已在原实现通过。

另在完整通用 parser 的临时副本验证原生 *int64 类型化写入，保留实际类型、字段 owner/形状、scalar 校验与反射回落。129 项五轮中，大型 Decode 17.535 → 15.565 µs（-11.2%），PDecode 22.715 → 20.636 µs（-9.2%），分配不变；字符串约 +4.9% 和未改极小 Unpack +11.6% 的代价原样保留。

隔离原型通过官方对照、完整库 race/checkptr/vet、131068 组差分与 GC/padding 探针、两项有限 fuzz、GSR 三包与 Go/TS 7/7。原型未放入正式库，正式生产源码仍为 `70d985c`。推荐范围、Go 写屏障/布局责任和正式化门槛见 [受控写入提案](RFC-GOSPROTO-DECODER-WRITES.md)，数据、未验证范围及复现见 [实验记录](benchmarks/2026-10-17-gosproto-decode-writes/RESULTS.md)。该轮提案与回归已本地提交为独立库 `5be3b26`、GSR `2872d46`；当时尚未采用写入原型。正式封装后的新数据见下一节，未发布。

## 16. 原生整数 Decode 的私有类型化写入

按受控写入提案的最小范围实施：类型核对和地址取得封装于 decodePointer，通用 parser 保留完整校验，逐字段核对 native/owner/Wire/Array 后做 Go 类型化指针赋值；不满足时回落反射索引。每字段独占分配和根结果失败原子性不变。没有更改 Encode、Pack/Unpack、元信息、API 或生成类型。

封装后重新采样 129 项五轮：大型 Decode 17.484 → 16.189 µs（-7.4%），PDecode 22.904 → 21.548 µs（-5.9%）；小型约 -7.7%/-5.6%，分配不变。大型纯 Decode 低于同场 PB 17.777 µs，但 PDecode 仍高约 21.2%。AddressBook 约 +2%、未改极小 Unpack +16.6% 与独立 A/A +2.6% 原值保留，不宣称全面无回退。setInteger 内联、入口 helper 未内联和实际 Go 写屏障证据均记录。

新增库内 GC/padding/独占回归；完整库 race/checkptr/官方对照/vet、131068 差分、两项有限 fuzz、GSR 三包及 GoTS 7/7 通过。缺失 Decode owner guard 的隔离副本按预期失败。Linux 三架构构建通过，未跨平台运行。数据、复现与边界见 [受控写入实施](benchmarks/2026-10-17-gosproto-decode-store/RESULTS.md)。该切片已本地提交为独立库 `fc3246a`、GSR `862fa8b`，未推送/发布，正式依赖仍为 v0.4.0。

## 17. Decode 入口与固定 word 恢复筛选

从 `fc3246a` 比较传入现成 record、helper 内 Elem().Type()、record.Addr() 三种入口写法，各 59 项三轮。大型纯 Decode 最好约 -1.9%，对应 PDecode 仅 -0.6%，其他写法还出现组合回退，全部未采用。编译器复杂度降低不等于真实组合收益。

随后另采已提交基线的 Decode/PDecode profile：入口 helper 在大型 PDecode 累计约 2.41%，普通 Unpack 恢复仍可见；kevent 样本保留，不归于业务成本。固定 body 的 mask 分布表明大型游戏主要为四/五/六字节普通 word，只有两个 FF word。新验证较密 word 固定八次恢复，129 项三轮中，大型/整数数组 Unpack 约 +20.0%/+12.1%，均撤回；不追加更多密度分支。

共 918 个筛选样本，没有进入五轮验收，不产生新的 PB/SP 排名。全部生产代码保持 `fc3246a`，只保留 TestPack 新增 2048 组独立 mask 后继 FF/输入偏移回归，已在基线及单独候选副本通过。未重复完整库/fuzz/GSR/TS 验收。候选、profile、分布、复现和范围见 [局部候选筛选](benchmarks/2026-10-17-gosproto-decode-binding/RESULTS.md)。该切片已本地提交为独立库 `a09fb00`、GSR `d1beb28`，未推送，正式依赖仍为 v0.4.0。

## 18. 最终入口实验与 Decode 冻结

按用户决定仅再做一次 messageDecodePointer 独立入口实验。微基准 18 项五轮显示，即时类型证明留在调用点、让私有地址借用 helper 内联，可把原生结构入口 8.079 → 4.988 ns（约 -38.3%），零分配；原生、无原生字段及实际类型不匹配三种状态能力一致。未删 native/owner/形状检查，未缓存事实。

只让该候选进入 129 项五轮完整对照：大型 Decode 15.773 → 15.418 µs（-2.25%），PDecode 20.726 → 20.461 µs（-1.28%），两项五轮均更快，分配不变。但未达到计时前确定的至少 3%/2% 门槛，未采用；不事后降低门槛，也不继续扩展候选。

**Decode 阶段性冻结于独立库 `a09fb00`，其生产源码与 `fc3246a` 一致。** 当前暂停入口、parser、构造/写入及其性能绑定的微调，后续可以回头继续优化，不是永久限制。Pack/Unpack 工作以此为控制；回到 Decode 时另定本轮范围与验证，正确性修复照常处理。

本轮只新增独立实验与冻结文档，正式独立库工作树干净，未重复候选完整正确性验收或 GSR/TS 接线。735 个样本、门槛、逐轮方向、复现与范围见 [冻结报告](benchmarks/2026-10-17-gosproto-decode-freeze/RESULTS.md)。该记录已本地提交为 GSR `0db5ed0`，未推送/发布，GSR 正式依赖仍为 v0.4.0。

## 19. Pack 普通 word 执行路径收敛

从 `a09fb00` 的新 Pack/PEncode profile 及编译器输出确认，packWord 仍是主要执行点且无法内联；不把函数全部成本当作调用开销。仅把同一份完整 word mask/稀疏/混合写出逻辑移入 Pack 主循环，保留小型可内联 FF 位判断，FF/尾块/搜索和输出预留不变。没有增加 unsafe、池、表或分配，Decode/Encode/Unpack/meta/RPC/TS 不变。

最终 137 项五轮：大型 Pack 3.316 → 2.865 µs（-13.6%），PEncode 9.921 → 9.321 µs（-6.0%）；整数数组 Pack 3.163 → 2.542 µs（-19.6%），PEncode 5.334 → 5.031 µs（-5.7%）。分配不变。完整数值路径仍慢于同场 PB 约 22.9%/1.8%，不称为全面胜出。三轮 ASCII PEncode +4.3% 的筛选值与五轮 -0.5% 分开保存；最终没有 SP 项回退 >5%。

代价是 Pack 主循环更长/嵌套更深，同一逻辑没有复制两份，Go 净增四行；保留组合收益而不将手工内嵌泛化为编码规范。最终 profile 中 kevent 强烈干扰，不用份额变化推算提速。17984 差分、完整库 race/checkptr/官方对照/vet、有限 Pack/Codec fuzz、GSR 三包/TS 构建/GoTS 7/7 通过，Linux 三架构仅构建。筛选/最终共 1096 样本，复现与范围见 [Pack 执行路径验收](benchmarks/2026-10-17-gosproto-pack-execution/RESULTS.md)。该轮已本地提交为 gosproto `faa6a21` / GSR `3626b3d`，未推送/发布，正式依赖仍为 v0.4.0。

## 20. 连续零 word Pack 筛选

从 `faa6a21` 分别验证主循环批量扫描、私有冷 helper 加孤立零直接写出、已有稀疏分支内识别三种候选。各 191 项三轮，总计 1719 个样本；4 KiB 零块 Pack 约 -61%～-62%、完整帧约 -34%～-40%，分配不变，但分别存在 ASCII Pack +12.5%、大型 PEncode +7.6%、ASCII PEncode +7.1% 等代价，全部未采用，未进入五轮验收。控制路径波动原样保留，不将所有变化强行归因为分支成本。

正式生产代码逐字节保持 `faa6a21`，只保留 TestPackZeroRuns 的 FF 前缀、零段数量、部分尾块、偏移/额外容量精确 wire 回归；先在基线通过，再在三个候选最终快照通过。归档/补丁复现核对源码、七份 harness 和三份夹具一致；没有重复完整库/官方/fuzz 或 GSR/GoTS 验收。候选、全量原值、复现与范围见 [连续零 word 筛选](benchmarks/2026-10-17-gosproto-zero-pack/RESULTS.md)。该轮已本地提交为 gosproto `bba3326` / GSR `a27ec4e`，未推送/发布，正式依赖仍为 v0.4.0。

## 21. 稀疏 word 筛选与 A/A 分辨能力

从 `bba3326` 另采四项有限 Pack/PEncode CPU profile，只展开已确认至多两个非零字节的提取循环，保留其他生产路径。预设 Pack ≥5% 且 PEncode ≥2%、其他业务组合和完整帧 ≤5% 回退、分配不增加。191 项三轮中，大型/整数数组 Pack 约 -6.0%/-5.8%，但小型 PEncode +11.0% 超额，候选未采用；未改纯 Encode 等控制的差值也保留。

另做不含候选的 A/A：59 项三轮，两份 Go 源码完全相同，只有导入路径不同。大型纯 Encode +19.0%、整数数组 Pack -23.0%，且逐轮明显变化。两份 Pack 反汇编的 640 条指令除指向同一页的 PC 相对 ADRP 位移外一致，不能归为算法收益，也不足以单独确定噪声原因。A/A 不用于替候选重新过门；下一步先改善测量分辨能力，再评估小幅优化。

候选精确 Pack 回归及两份归档复现通过，正式库干净并保持 `bba3326`；没有采用生产改动，未重复完整库/官方/fuzz/GSR 验收。750 个配对样本、CPU 摘要、汇编、预设门槛及验证范围见 [稀疏 word 与 A/A 诊断](benchmarks/2026-10-17-gosproto-pack-sparse/RESULTS.md)。该轮记录已本地提交为 GSR `8f0003f`，未推送，正式依赖仍为 v0.4.0。

## 22. 交错配对采样校准

新增独立工具，在固定归档中分别运行同函数 self A/A、同源码不同导入路径 clone A/A，以及原候选 compare。先预热/校准调用数，再按 ABBA/BAAB 交错，每项六轮、每轮四组，保留所有块与轮次。按配对比值中位数和 IQR 逐项标记控制，不将命令 PASS 解释为全部控制通过；不关闭 GC、不复用输出或删除检查。

self 11/19、clone 14/19 通过，9/19 同时通过；两类控制的全部配对中位差值分别约在 ±2.7%/±2.5% 内，但部分轮次离散与微小分配计数差值仍被拒绝。原候选源码未变，新复测小型 PEncode -3.04%，旧 +11% 回退未复现；整数数组 Pack/PEncode -4.77%/-2.65%，大型 Pack IQR 9.06 pp，未达到原保留门槛且不能解释全部回退项目，继续不采用。

工具统计/固定 wire 用例、四项 Python 判定用例、错误归一化副本的预期失败及 go vet 通过；两份 prepare-only 复现核对完整输入一致，正式库干净并保持 `bba3326`。没有重复生产库/官方/fuzz/GSR/GoTS 验收。三个模式共 342 个配对轮次、5472 个计时块，详见 [配对校准记录](benchmarks/2026-10-17-gosproto-pair-calibration/RESULTS.md)。该轮工具与记录已本地提交为 GSR `f064584`，未推送，正式依赖不变。

## 23. GC 边界观测与 40 ms 聚焦实验

隔离探针固定 128 次 6 B 分配，仅改变显式 GC 边界：无 GC 或完成 64 次后 GC 均为 1024 B / 128 对象，完成 63 次后 GC 为 1040 B / 128 对象；空窗口均 0 B，三轮一致。结合 Go 1.24.13 tiny allocator 和 mcache 清理源码，验证了 16 B/128=0.125 B/op 的一种具体来源，不调整原分配门槛。

工具新增精确选择、10/40 ms 显式配置、块前后完成 GC 周期观测，并单列 timingAccepted；controlAccepted 仍为原计时与分配要求的交集。只对上一轮 10 个未通过项加两项数值 Pack 参照做一次 40 ms self/clone：总控制分别 1/12、4/12，双控制仅整数数组 Pack 通过，全部 2304 个块都有完成 GC 事件。单纯拉长块未解决墙钟波动；未丢弃样本、重试到通过或合并旧轮通过项。默认仍为 10 ms，未重跑新版默认全矩阵，也没有比较候选。

选择/状态用例由失败到通过，新增 GC 观测及 CLI 拒绝检查；Go 统计/固定 wire、八项 Python 用例、go vet 和两个归档复现通过。正式库保持 `bba3326` 且干净，无生产改动或重复完整验收。数据、源码依据、机制探针和范围见 [GC 边界与聚焦校准](benchmarks/2026-10-17-gosproto-pair-resolution/RESULTS.md)。该轮已本地提交为 GSR `3a88564`，未推送，正式依赖和 Decode 阶段基线不变。

## 24. 墙钟与进程 CPU 的隔离诊断

固定三项 self、40 ms、六轮且只运行一次：大型 Pack、整数数组 Pack、ASCII 完整帧的墙钟控制均未满足原计时要求，进程 CPU 则均满足相同的数值要求。大型 Pack IQR 从墙钟 9.57 pp 降为 CPU 2.06 pp；极端块 CPU 仅约为墙钟的 44%～53%，支持存在明显执行之外等待的判断，但没有足够系统证据归因到具体调度或 Runtime 因素。

进程 CPU 包含 Runtime/GC 线程，观测有成本且窗口略宽；它只用于机制交叉检查，不替换墙钟验收。本轮没有 clone 或分配测量，不能称为总控制通过。相关用例与 go vet 通过，288 块、72 组和 18 个配对轮次独立复算一致；没有生产代码变化，正式库与 Decode 阶段基线不变。下一轮回到 Pack 的结构性成本，停止继续调采样参数；候选保留仍须通过原验收。详见 [CPU 诊断](benchmarks/2026-10-17-gosproto-pair-cpu/RESULTS.md)，已本地提交为 GSR `b2b3dd7`，未推送。

## 25. FF 段集中处理候选

从普通 word 循环移除 FF 状态维护，在完整 FF 起点单独扫描并一次写出；六/七字节合并、256-word、部分尾块、输出容量和所有权不变。候选仅在隔离归档修改 pack.go，既有 Pack 用例和增加 cap 相等断言的 17984 组差分通过。

固定六项 self/clone/compare 各一次，10 ms 默认不变，共 1728 块。大型 Pack/PEncode 配对中位差 −4.59%/−3.99%，整数数组 −2.16%/−1.02%；Pack 均未达 5% 收益门槛，两项 Pack 的 self 控制也未通过，大型 Pack compare 轮次稳定性不足。大型 PEncode 虽有局部改善信号，不能独自替整版过门，候选撤回、不追加变体或扩大验收。原始值、补丁和三个归档复现见 [FF 段实验](benchmarks/2026-10-17-gosproto-pack-runs/RESULTS.md)，新记录待审查。

正式库干净并保持 bba3326，Decode 继续冻结。后续先分析 EncodePacked 中 Encode 缓冲与 Pack 输出的两次字节分配，任何私有共享工作区尝试必须证明读写边界，保持公开输入、输出长度/容量、预算和失败行为；本轮未实施该方向，不引入新 API。

上述为按旧筛选线作出的当时判断；用户修正收益标准后，本候选已恢复并与稀疏展开共同复评，当前结果以第 26 节为准，原始数据不改写。

## 26. 小幅收益复评与组合保留

稀疏展开和 FF 段分离分别进行两轮原 19 项业务复测，反转候选顺序，开始和结束各测 self/clone；另各测一次固定 19 种回退形态。共 23712 块、1482 个配对轮次，完整原值和独立复算见 [复评报告](benchmarks/2026-10-17-gosproto-pack-reassessment/RESULTS.md)。取消最低收益比例不等于忽略测量噪声：控制状态和未改 Encode 的波动仍全部披露，不将近零变化当作确定收益。

稀疏展开的大型 PEncode 两轮 −2.24%/−2.22%、整数数组 −2.59%/−2.93%；FF 分离大型 −3.32%/−2.95%，整数数组基本持平。组合直接测得大型 −3.72%/−4.24%、整数数组 −2.83%/−3.04%、ASCII 32 B 帧 −3.29%/−4.06%、零字节 4 KiB 帧 −19.58%/−20.32%；ASCII 4 KiB 基本持平。4 KiB 纯四非零字节形态约慢 3.33%（约 0.1 μs），结合真实业务改善接受并披露该代价。

保留组合到独立库工作树，只改变 pack.go 生产文件及 CHANGES。输出容量、官方 FF/尾块规则与所有权不变；完整帧的 0.125 B/op tiny/GC 边界计数差单列解释，不自动当作新增对象。三个候选的既有用例及各 17984 组差分通过；最终工作树库 race/checkptr/官方 C/Lua 对照、vet、两项有限 fuzz、GSR 三包与 Go/TS 7/7 通过。Decode/Unpack/Encode/RPC/TS 均未改，GSR 正式依赖不变；该轮已提交为 gosproto d1ab622 / GSR d3b3ef4，未推送或发布。

## 27. EncodePacked 单缓冲工作区

复用预检得到的确切 body 长度，在原 Pack 容量的缓冲后部编码，从前部压缩；FF 头最坏增长额度和八字节余量隔开读写游标。公开 Encode/Pack 仍各自独占，输出容量、默认预算和错误语义不变；私有 packInto 复用原压缩循环，不增加池、unsafe 或 public API。

新增 10240 组组合入口边界及六种长输入，核对 wire/cap、源存储和旧输出独占；移除间隔的负向副本在 65535 B 输入失败。分配测试先证明旧实现有中间分配；插桩构建计数不同后，改为同构建的组合/分步相对比较，仍在旧实现失败。仅测试在采样后修正，三个生产文件指纹保持一致，最终普通库/官方对照/race/checkptr/vet、两项有限 fuzz 与 Go/TS 7/7 通过。

固定四次原 19 项配对，7296 块：大型 PEncode 两轮 −4.82%/−5.52%，7424→3968 B、3→2 次；整数数组 −9.46%/−9.70%，8192→4096 B、2→1 次；ASCII 4 KiB −40.03%/−40.71%。原手工/RPC 完整帧未改用组合入口，32 B 帧有约 7～9 ns（2%～3%）代价，其他完整帧基本持平，不将 body PEncode 降幅推广为网络收益。按实际收益与代价保留，不使用最低百分比淘汰线。

证据、重叠边界依据及测试修正见 [工作区报告](benchmarks/2026-10-17-gosproto-packed-workspace/RESULTS.md)。该切片已本地提交为 gosproto 56f5c21 / GSR 0f0f779，未推送；当时 Decode/Unpack/RPC/TS 保持原实现，正式依赖不变。

## 28. RPC 编码组帧工作区

保留 body 编码先于 session 检查的顺序、等待登记及锁范围，只将 header 直接编码到独占 Pack 工作区，复制已编码 body 后从前部压缩，省去 header 字节缓冲与 Append 拼接缓冲。复用现有 schema 预检/写出，不手写另一套 header codec；header/body 默认预算各自计算，重复 session 返回已编码 body 的既有行为也不变。

新增真实 RPC 探针：五种代表消息各测通知、请求编码加 CancelSession、响应编码，四种 Echo 测通知，共 19 项。固定 self/clone/两次比较与原 19 项未改路径对照，共 9120 块独立复算一致。两轮大型消息约快 7.9%～10.3%、整数数组约快 17.7%～20.2%、ASCII 4 KiB 约快 37.3%～39.1%，所有项少两次分配。self/clone 总控制 13/19、14/19，失败与逐轮波动原样保留；原未改路径中位差约在 ±2% 内，不据此宣称新算法收益或网络/PB 排名。

864 组 header/角色/长度场景、1728 输出保护 wire/cap/独占，另保护预算、错误优先级和分配；旧实现分配测试按预期失败。普通库、race/checkptr、官方 C/Lua、vet、有限 fuzz、GSR 三包及 Go/TS 7/7 通过；最终与采样源码逐字节一致。已本地提交为 gosproto dc4948d / GSR c4622bf，未推送，见 [RPC 工作区报告](benchmarks/2026-10-17-gosproto-rpc-workspace/RESULTS.md)。body 缓冲与复制仍在，后续若融合须另行验证错误顺序及锁语义；Decode/Unpack/TS 和正式依赖不变。

## 29. 通知路径避免 session 参数逃逸

RPC RequestEncode 原先在有响应分支取 &session，导致参数的堆存储在通知路径也分配。仅在该分支声明 requestSession 副本再取地址，保持原锁范围、body 校验、session 登记及错误顺序；不用另一套 header codec 或公共接口。编译器逃逸报告和同构建分配回归确认问题：旧实现通知/有响应请求均4次，新通知3次，请求仍4次；大型通知因布局记录为5→4次。

固定 self/clone/两次19项真实RPC配对，共7296块独立复算一致。9种通知全部少一次逻辑分配、约4 B/op；32 B通知两轮约快2.71%/3.38%，AddressBook约快1.92%/2.13%。大型通知约慢0.73%/0.51%（68/48 ns），ASCII请求约慢1.06%/1.20%（18/20 ns），代价原样保留。self/clone原控制14/19、10/19，相同源码clone大型通知也有-2.51%偏差；不把大消息细微CPU差解释成已确认算法收益或回退。Go tiny allocator合并小值，少一次allocs/op不等同于每次少一个独立GC对象。

按确定的分配减少、小包收益和极低维护成本保留，不设统一最低CPU收益线。库全测/官方C-Lua/race/checkptr/vet、GSR三包及Go/TS 7/7通过，源码与归档重建一致；该切片已提交为 gosproto 69cae73 / GSR 9ecc811，未推送。完整数据、控制失败及复现见 [session逃逸报告](benchmarks/2026-10-17-gosproto-rpc-session/RESULTS.md)。Decode、Pack/Unpack、body编解码、TS和正式依赖均不变，本轮未重复相关fuzz或PB/TS性能矩阵。

## 30. 固定私有 RPC header 编码

先对当前通知API分开采集CPU与分配profile。首次把memprofilerate=1与CPU合用，采样开销主导；保留该组诊断，另采CPU，不把profile ns/op当成基线。小包header仍走schema查询及反射预检/写出、产生16 B对象；大型消息内存主要在body/Pack缓冲。只针对库内固定type/session两字段，复用skipTag/encodeIntegerInto/writeUint函数，在22 B调用内栈缓冲编码并复制到原Pack工作区。元素与字节预算检查保留，固定深度1满足规范化默认深度下限；不删body预检、不新增业务消息专用编码框架。

RequestEncode改用局部header值，避免字段指针赋值使session副本保守逃逸。首版2/3/2分配失败记录保留，最终通知/请求/响应的平坦消息均只承担body与Pack两个缓冲；大型消息另有布局记录。session登记、锁范围、错误顺序、默认预算、wire/cap/独占及Decode/Dispatch/通用codec/Pack/Unpack/TS均不变。

固定四次19项配对共7296块独立复算一致。两轮19项均更快：32 B通知−37.18%/−38.40%，AddressBook约−13.6%～−17.4%，大型消息约−1.7%～−3.7%，整数数组约−2.1%～−4.0%；通知少1次16 B分配，请求/响应少2次约20 B分配。self/clone原控制14/19、11/19及全部失败/波动保留，不以固定最低CPU比例淘汰，也不扩展为PB/TS或网络排名。

既有组合用例扩展至1008组/2016输出；新增39个官方C/Lua RPC头编码/解码对照，在旧实现也通过。有限随机fuzz仅58次，覆盖不足明确保留；随后单次固定3078种子/12312帧旧新差分通过，没有重试随机fuzz凑数字。最终库普通/官方/race/checkptr/vet、GSR三包及Go/TS 7/7通过；库Go源码与采样/归档重建一致。该切片已提交为 gosproto f9de3d2 / GSR dbb681f，未推送，完整依据及复现见 [RPC profile与header报告](benchmarks/2026-10-17-gosproto-rpc-profile/RESULTS.md)。

## 31. 仅融合 RPC 响应的 body 缓冲

声明body的ResponseEncode把已验证header字节作为私有前缀，body完整预检后直接写入最终Pack工作区，省去中间body分配与复制。普通Encode/EncodePacked传nil前缀；不增加公共API/协议模型，前缀不逃逸、不别名输出。容量与读写间隔按header+body完整尺寸计算，header/body独立预算与错误契约保持。无body响应和RequestEncode方法/登记/锁/重复错误body返回保持原样，共用header写出提取后也计入测量。

固定四次19项真实RPC配对加一次原19项body/Pack/手工帧，共9120块独立复算一致。五项响应两轮均更快且少一次分配：大型−7.58%/−6.04%（7424→3968 B，3→2次），整数数组−14.79%/−12.54%（8192→4096 B，2→1次），ASCII4KiB−43.90%/−43.04%（9728→4864 B，2→1次）。请求/通知分配不变；AddressBook通知约+1.30%/+0.98%（9/7 ns）代价保留。nil-prefix普通19项约−2.86%～+1.33%，不把未针对路径的波动归因于算法。self/clone17/19、16/19控制及全部失败保留；按真实响应收益、低复杂度及透明代价保留，无最低CPU比例门槛。

复用1008组RPC边界与39官方头对照，新增3个typed nil响应必须ErrNil/nil输出，旧实现也通过；初版空结构测试预期不符合getbase契约，仅纠正测试，未改生产行为。相对分配回归旧2对1失败，新普通/race/checkptr通过。最终库全测/官方/race/checkptr/vet、Pack/Codec各10秒fuzz（136238/220094次）、3078固定RPC差分/12312帧、GSR三包及Go/TS 7/7通过，源码与归档一致。该切片已本地提交为 gosproto b626637 / GSR b2825cf，未推送，见 [响应工作区报告](benchmarks/2026-10-18-gosproto-response-workspace/RESULTS.md)。正式依赖、Decode及新的PB/TS排名不变。

## 32. 仅融合无响应通知的 body 缓冲

仅在Request存在且HasResponse为false的分支，将私有type-only header作为既有encodeWithLimits前缀，直接写入最终Pack工作区，省掉中间body分配及复制；生产仅rpc.go增加八行。通知仍忽略session且不登记等待，typed nil/预算失败返回nil；header/body预算、wire/cap、输入/旧输出独占不变。有响应请求的body校验顺序、session锁/登记及重复session返回raw body保持，ResponseEncode/通用encoder/Decode/Dispatch/Pack/Unpack/TS均未改。

固定四次19项真实RPC配对共7296块独立复算一致。9项通知两轮均改善且少一次分配：大型−6.53%/−7.34%（7424→3968 B，3→2次），整数数组−13.46%/−11.71%（8192→4096 B，2→1次），ASCII4KiB约−43%～−45%，32 B−11.91%/−16.91%。非目标10项分配不变；整数数组请求+0.43%/+0.32%（20.1/14.4 ns）及ASCII响应第二轮+1.07%（8.9 ns）代价保留，不将其他控制加速当算法收益。self13/19、clone7/19及全部失败保留，不重试到通过，不设最低收益百分比。

扩展既有RPC测试保护忽略session、不登记pending、typed nil与8 MiB通知预算，旧实现先通过；分配断言旧2对1按预期失败，新普通/race通过。库全测/官方C-Lua/race/checkptr/vet、3078固定RPC差分/12312帧、GSR三包及Go/TS 7/7、零跳过通过；源码与采样/重建归档一致。通用codec/Pack未改，不重复随机fuzz、PB/TS矩阵或GSR全仓/race。该切片已本地提交为 gosproto 0988af0 / GSR 9876a5b，未推送，见 [通知工作区报告](benchmarks/2026-10-18-gosproto-notification-workspace/RESULTS.md)。正式依赖仍v0.4.0，Decode继续冻结。

## 33. 有响应请求的 body 工作区融合

提取私有encodeBufferWithLimits，将原预检/写出结果保留为同一独占workspace和未压缩frame。有Request且有Response时，先在锁外编码header/body，继续原session校验、锁/重复检查、1024上限及登记，再持锁Pack；body预检、错误优先级、wire/cap及独占不变。重复session返回限长限容raw body，无效session/满载仍nil错误；无Request流程、ResponseEncode方法、Decode/Dispatch、Pack/Unpack算法均未改。

四次19项RPC加一次原19项body/Pack/手工帧，共9120块独立复算一致。五项请求两轮改善并少一次分配：大型−5.80%/−6.17%（7424→3968 B，3→2次），整数数组−9.49%/−11.58%（8192→4096 B，2→1次），ASCII4KiB−41.04%/−41.91%。14项非目标分配不变；AddressBook响应约+7.7/+2.4 ns、ASCII通知约+6.7/+12.4 ns代价保留。普通19项约−2.04%～+2.94%，ASCII纯Encode约+21 ns、整数数组PEncode约+59 ns；self17/19、clone15/19与全部失败披露，不重试或设最低收益线。

失败路径现在先分配完整workspace，重复返回body仍保留底层header/slack；独立72窗口分配诊断显示32 B输入invalid/full为48→64 B，MaxInt32重复缓冲尺寸级别48→80 B，fmt池/GC尾差不归为确定新增对象。接受并披露失败成本，不以cap相同声称底层字节完全不变。

既有1008组成功用例扩展504组重复raw body/cap/独占，补typed nil请求优先于session错误；语义在旧实现先通过，分配旧2对1失败、新普通/race通过。库全测/官方C-Lua/race/checkptr/vet、3078固定RPC差分/12312帧、Pack/Codec各10秒有限fuzz（3946/464565次）、GSR三包和Go/TS 7/7通过，源码/归档一致。Pack后段计数不增长如实保留，未凑随机次数。该切片已本地提交为 gosproto 6d854d1 / GSR f715e6b，未推送，见 [请求工作区报告](benchmarks/2026-10-18-gosproto-request-workspace/RESULTS.md)。后续先重采当前profile再选热点，正式依赖仍v0.4.0，Decode继续冻结。

## 34. 融合后的 CPU 与分配热点刷新

从6d854d1归档分别采集32 B ASCII、大型游戏、896整数数组、4 KiB ASCII的EncodePacked与RequestEncode+CancelSession，共八条路径。CPU各一次2秒；逐次分配另起进程各20000次，GC开启，不重试、混用插桩耗时或形成新PB/TS排名。独立harness的夹具/统计/选择用例、vet、构建及16次采样通过，归档重建/源码/二进制/profile指纹一致。模块replace绝对路径只规范化输出目录后比较，原始哈希仍各自保留；库工作树保持干净，未重复生产库/GoTS验收。

大型消息的整数记录预检/写出累计份额各约19%/22%～24%，其内已用受验证offset读*int64，不能简单归咎于逐字段反射；Pack约18%～20%。整数数组Pack约29%～34%、encodeInt64Slice约23%～27%，下一轮先单独核对连续游标/固定宽度写出的BCE与汇编，再配对实测；不同时改Pack或记录遍历。小包仍有schema查询/通用遍历成本，但不为此改pending语义或新建codec API。

大型分配仍为3968 B/2次（3456 B输出尺寸级别+512 B布局外溢），其他三项各一次最终buffer，header/session及中间body已不是分配主因。4 KiB ASCII的kevent归因达56%～64%，沿GC阶段栈出现，成因未确定，不作为算法排名或收益上限。累计父子份额不相加，profile耗时不作为配对基线；全部原值保留，见 [融合后热点报告](benchmarks/2026-10-18-gosproto-post-fusion-profile/RESULTS.md)。本轮只新增诊断与文档，未提交；正式依赖、Decode及生产实现不变。
