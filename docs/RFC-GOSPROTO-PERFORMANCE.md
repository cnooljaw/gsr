# gosproto 后续性能优化方案

状态：2026-10-16，GSR 正式依赖仍为 v0.4.0，后续独立库优化尚未发布。TS 长块和 Go 安全路径见 [首轮结果](benchmarks/2026-10-15-sproto-next/RESULTS.md)，数值 packing、字段查询及类型化构造见 [第二轮结果](benchmarks/2026-10-15-sproto-round2/RESULTS.md)。TS 优化阶段已在 GSR `9801808` 收尾，可迁移经验见 [TS → Go 优化经验](SPROTO-OPTIMIZATION-LESSONS.md)。编码执行层改进见第 9 节，大型 Encode 约 6.54 µs，同场 PB 7.59 µs。混合 word Pack 已本地提交为独立库 `7dc6713`、GSR `e9e1f74`，未推送；当前长 FF 有界搜索新增改动未提交，见第 11 节。公开 API、generator 与预算/所有权契约不变。

第三轮已完成当前基线与候选筛选，只保留调用内布局记录的内存收窄，独立库已本地提交为 `a579920`，GSR 记录提交为 `fdbc736`，均未推送/发布；未取得新的明显 CPU 加速。实测和取舍见 [第三轮结果](benchmarks/2026-10-16-gosproto-round3/RESULTS.md) 及第 8 节。

前两轮以已发布的 gosproto v0.4.0 为版本基线；下一轮从已提交的 `6012b06` 做增量，不从 v0.4.0 重复实施。实现改动应在独立 gosproto 仓库进行，GSR 提供真实接线和跨语言 Benchmark；不修改 Go module cache，也不将库源码复制到 GSR。

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

新增全部 mask 长输入的独立预期字节，17984 组差分、官方对照与完整库 race/checkptr、vet、有限 fuzz 通过。最终源码、接线、profile 和代价见 [FF 搜索结果](benchmarks/2026-10-16-gosproto-ffscan/RESULTS.md)。输入/输出和公开 API 不变，未扩大 unsafe。新增改动未提交/发布，不因字符串收益扩大为协议总体或跨平台结论。
