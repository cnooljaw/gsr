# gosproto 后续性能优化方案

当前双主线诊断（2026-10-20）：[指针优化后成本](benchmarks/2026-10-20-gosproto-post-pointer-cost/RESULTS.md)基于已提交a93bb0d完成9CPU+9allocation及独立实际wire审计。大型Encode的整数记录预检/写出仍累计21.50%/17.00%；大型/数值数组分别386/417与321/449个word为3～7非零字节，下一Packing候选优先普通word目标视图与扫描，不误投FF。ASCII512个FF word另看扫描/分配。80条profile命令+1条审计、fresh/源码/输入/二进制/raw核对通过，无新生产改动或PB/TS排名。前轮库a93bb0d/GSR3f23e02已提交，新诊断未提交；正式依赖、用户原6项删除及两条主线保持。

当前Body实现（2026-10-20）：[整数记录数组受控指针读取](benchmarks/2026-10-20-gosproto-record-pointers/RESULTS.md)仅在当前字段证明及实际元素指针类型核对后，同步读切片指针槽，减少两阶段逐元素反射，命名指针回退。固定37项两轮，大型Encode−18.63%/−19.05%、PEncode−15.87%/−14.34%；AddressBook PEncode+6.43%/+0.43%、数值Encode+1.61%/+2.01%及31项累计反向保留。178条测量流程、基线新回归及7条全库/oracle/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过，候选和永久回归已提交独立库a93bb0d。前轮库7b05b78/GSR2c5c653已提交，正式依赖及原6项删除不动，Body与Packing继续。

当前Body实现（2026-10-20）：[原生字段分布诊断](benchmarks/2026-10-20-gosproto-inline-shapes/RESULTS.md)沿用同一延迟内容切片候选，固定8/64字段×8分布的Encode/PEncode共32项、128进程两轮。8字段混合Encode−0.57%/−0.77%、64字段半缺失PEncode−2.15%/−2.58%；64字段负数PEncode+1.40%/+0.50%及29项累计反向全部保留。决定保留小改动、接受局部代价，不宣称业务全面提速；158条测量流程及7条全库/oracle/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7验收通过，精确源码已提交独立库7b05b78。前轮19项报告已提交ffc591f，正式依赖v0.4.0及用户原6项删除不动，Body与Packing继续。

当前Body实验（2026-10-20）：[通用Encode延迟内容切片](benchmarks/2026-10-20-gosproto-encode-inline-body/RESULTS.md)完成固定原19项、76进程两轮比较。大型Encode中位−0.36%/−1.36%，首轮累计反慢214.9ns；大型PEncode+2.23%/−0.67%方向翻转。UTF8手工帧−2.33%/−1.32%、AddressBook PEncode−0.06%/−2.11%等小收益完整保留；候选待确认、不应用生产，不设收益比例门槛。102条流程命令、既有契约/夹具wire/来源与逐块复算通过；self13/19、clone15/19，17项累计反向及3项分配尾差保留。参考文档已提交7e48d4c，生产库e43df35、正式依赖及原6项删除不变；新报告未提交，双主线继续。

当前目标（2026-10-20，用户裁决）：同时推进Body Encode/Decode与Pack/Unpack，最终相对各自PB的领先幅度向TS结果靠近。已小幅领先不等于Body完成；Unpack组合缺口也不构成冻结正文的理由。Go/TS逐入口参考、边界与执行规则见[双主线目标](GOSPROTO-DUAL-TRACK.md)。

当前Body实现（2026-10-20）：[结构数组受控指针槽](benchmarks/2026-10-20-gosproto-decode-struct-slots/RESULTS.md)已保留并提交独立库 e43df35。相同对象构造下大型Decode−3.50%/−3.63%、Dispatch−5.80%/−3.04%，长结构重复改善、对象数中位不变；AddressBook PDecode+2.56%/+2.07%及全部控制/累计反向保留。新低GOGC/命名类型/元数据回退/异构指针拒绝回归、全库/官方/race/checkptr/vet/有限fuzz/GSR三包/GoTS7of7通过。新增unsafe仅同步借用已核对类型的新切片槽，GC屏障与反射回退保留。前轮成本诊断已提交06adf22，正式依赖v0.4.0不变，原库6项已有删除不动；Body与Packing继续。

当前Body诊断（2026-10-20）：[正文构造与执行成本](benchmarks/2026-10-20-gosproto-body-cost/RESULTS.md)完成PB/SP三消息的12CPU＋12allocation独立profile。大型SP Decode仍10416B/532次、PB13560B/537次，不能默认删对象；PB子消息数组也用reflect.New。下一项隔离同构造/同对象数量的结构数组目标槽写入，保留类型证明、GC屏障、错误/预算/ownership，不新建公共factory或池；原型本轮未实现。Encode的记录预检/写出继续跟踪，Packing不冻结。前轮tag报告已提交c4f06ee，新诊断未提交，生产源af9169d及用户已有删除保持。

当前Body实验（2026-10-20）：[非负tag计数表示](benchmarks/2026-10-20-gosproto-decode-tag-counter/RESULTS.md)完成固定70项两轮，去掉查找负数检查但总指令仍660。大型Decode−0.08%/−4.51%，首轮累计却慢139.8ns；大型PDecode/Dispatch两轮略慢，小型Dispatch−0.83%/−0.17%及全部小收益保留。候选待确认、暂不应用，不设最低收益门槛。新增10组最大tag/跳跃链公共API对照及既有契约/差分/来源核对通过。前轮Packing报告已提交36f03f2，新报告未提交；生产Go源仍af9169d、用户原有删除不动，双主线继续。

当前Packing实验（2026-10-20）：[普通Unpack局部word组装](benchmarks/2026-10-20-gosproto-unpack-word-store/RESULTS.md)固定70项两轮完成，正确性/159275差分及来源核对通过；大型Unpack慢8.51%/9.03%、4/8字节大数组慢约5.5%～7.6%、普通word密度普遍回退，候选未采用。短零段小收益、失败控制/累计翻转全部保留，不设固定收益门槛。前轮Body待确认报告已提交bf1203e，新实验未提交；原库已有删除未动，生产Go源仍af9169d，双主线保持。

当前Body实验（2026-10-20）：[countChunks直接计数](benchmarks/2026-10-20-gosproto-chunk-count/RESULTS.md)已完成固定70项两轮比较。大型Decode观察−0.72%/−0.10%，部分结构数组有小收益；self8/70、clone11/70，组合/累计多有翻转，收益尚不能可靠区分。候选保留待确认、未应用生产，不因低于比例门槛丢弃。既有契约/差分、fresh与来源核对通过；库原有删除保持，正式依赖不变。上一轮双主线目标/对照已提交386727c，新实验未提交，Body与Packing继续开放。

状态更新：2026-10-19，GSR 正式依赖仍为 v0.4.0，后续独立库优化尚未发布。TS 长块和 Go 安全路径见 [首轮结果](benchmarks/2026-10-15-sproto-next/RESULTS.md)，数值 packing、字段查询及类型化构造见 [第二轮结果](benchmarks/2026-10-15-sproto-round2/RESULTS.md)。TS 优化阶段已在 GSR `9801808` 收尾，可迁移经验见 [TS → Go 优化经验](SPROTO-OPTIMIZATION-LESSONS.md)。编码执行层改进见第 9 节，大型 Encode 约 6.54 µs，同场 PB 7.59 µs。混合 word Pack 已本地提交为独立库 `7dc6713`、GSR `e9e1f74`，未推送；长 FF 有界搜索已本地提交为独立库 `07350a9`、GSR `3a78dbd`，见第 11 节。连续零 mask 解包已本地提交为独立库 `22e287e`、GSR `1af93f4`，见第 12 节。普通 Unpack 五个候选已撤回，回归与记录已本地提交为独立库 `d66644a`、GSR `acfb070`，见第 13 节。安全 inline 处理已本地提交为独立库 `70d985c`、GSR `d0328fd`，见第 14 节；后续整数记录 parser 已撤回，原型与提案已本地提交为独立库 `5be3b26`、GSR `2872d46`，见第 15 节；私有 Decode 写入及验收已本地提交为独立库 `fc3246a`、GSR `862fa8b`，见第 16 节；后续四个局部候选均未采用，回归/记录已本地提交为独立库 `a09fb00`、GSR `d1beb28`，见第 17 节。最终入口专项当时未达门槛，Decode 于2026-10-17阶段性冻结，见第18节；用户已于2026-10-19明确解除冻结，当前工作见第54节。公开 API、generator 与预算/所有权契约不变。

第三轮已完成当前基线与候选筛选，只保留调用内布局记录的内存收窄，独立库已本地提交为 `a579920`，GSR 记录提交为 `fdbc736`，均未推送/发布；未取得新的明显 CPU 加速。实测和取舍见 [第三轮结果](benchmarks/2026-10-16-gosproto-round3/RESULTS.md) 及第 8 节。

前两轮以已发布的 gosproto v0.4.0 为版本基线；下一轮从已提交的 `6012b06` 做增量，不从 v0.4.0 重复实施。实现改动应在独立 gosproto 仓库进行，GSR 提供真实接线和跨语言 Benchmark；不修改 Go module cache，也不将库源码复制到 GSR。

当前总览（2026-10-20）：[Go SP/PB同场五轮](benchmarks/2026-10-20-go-sp-pb/RESULTS.md)使用gosproto af9169d提交归档与PB v1.36.1，共72项360样本；大型packed编码−8.1%、解码+14.9%，int32数组packed编码−36.5%、解码+62.2%。纯Encode/Decode已领先的数值路径被Unpack拖累，保持Body与Packing双主线；Packing侧优先Unpack→PDecode，Body继续定位大型结构成本，不继续以旧大型Encode数据选题。Go-only、无新profile/TS或细小收益统计显著性结论。库工作区有用户删除，采样隔离归档未触碰；正式依赖仍v0.4.0，本轮无提交。

当前切片（2026-10-19）：前轮word预算报告已提交GSR `8e9880f`，未推送。新[生产版阶段成本诊断](benchmarks/2026-10-19-gosproto-unpack-phase-profile/RESULTS.md)只读采af9169d的8个CPU入口：大型纯Unpack预检/恢复占完整样本30.40%/32.16%，大型PDecode中Unpack累计17.67%、正文Decode累计53.88%（嵌套项不相加）。countChunks累计4.31%，inline的未用content视图cap守卫仍在；下一候选为仅计数路径消除不使用的视图，尚未实施。55条流程命令＋1条静态objdump、24快照/1827采样权重、fresh/来源核对通过，无新A/B结论。主库仍af9169d干净，报告未提交，依赖v0.4.0、Decode继续开放。

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

**历史决定：Decode 于2026-10-17阶段性冻结于独立库 `a09fb00`，其生产源码与 `fc3246a` 一致；用户已于2026-10-19解除冻结，见第54节。** 当时暂停入口、parser、构造/写入及其性能绑定的微调，后续可以回头继续优化，不是永久限制。Pack/Unpack 工作以此为控制；回到 Decode 时另定本轮范围与验证，正确性修复照常处理。

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

大型分配仍为3968 B/2次（3456 B输出尺寸级别+512 B布局外溢），其他三项各一次最终buffer，header/session及中间body已不是分配主因。4 KiB ASCII的kevent归因达56%～64%，沿GC阶段栈出现，成因未确定，不作为算法排名或收益上限。累计父子份额不相加，profile耗时不作为配对基线；全部原值保留，见 [融合后热点报告](benchmarks/2026-10-18-gosproto-post-fusion-profile/RESULTS.md)。该诊断已本地提交为GSR d2be5d8，库保持6d854d1，未推送；正式依赖、Decode及生产实现不变。

## 35. 原生整数数组连续游标写出

仅将encodeInt64Slice的4/8字节输出改为remaining游标，生产净增三行。BCE确认旧每元素切片起点/store两处检查变成保留store一处，ARM64汇编减少重复偏移计算；不手工删检查，预检/宽度/signedness/预算、Pack/Unpack、RPC/Decode/TS和公开API均未改。

固定43项（原19body/手工帧、19真实RPC、5条混合64位896数组诊断），self/clone/两次比较共16512块独立复算一致。32位纯Encode−24.98%/−24.89%、PEncode−13.89%/−14.05%；64位纯Encode−22.94%/−22.61%、PEncode−7.86%/−7.70%。大型Encode/PEncode约−3%，真实32位RPC约−12%～−14%、64位约−7%～−10%；小型request第二轮基本持平，不宣称全部更快。

43项对象数中位数相同、20目标B/op相同，手工帧两个±0.125 B/op尾差保留。32 B手工frame约+5.2/+7.9 ns；未改int32纯Pack第一轮−0.16%、第二轮+9.45%（245 ns），后者六轮持续存在。只读取两轮原二进制核对：各臂300条Pack符号指令在两轮一致，但布局/GC/环境原因未定，不删样本或说已经解释为噪声。self/clone各40/43和全部失败披露；按真实组合收益、极低复杂度及公开代价保留，不设最低CPU比例。

新增40组4/8字节、1～4096元素精确wire/cap、后继字段及独占回归，先在基线通过；独立8字节误前移7的负向归档在length2按预期失败。库全测/官方C-Lua/race/checkptr/vet、10秒Codec fuzz474047次、GSR三包及Go/TS 7/7通过，源码/编译器归档/复现一致；未重复不涉及该循环的Pack随机fuzz、byte-body RPC差分或PB/TS排名。该切片已本地提交为gosproto 0c1f6cf / GSR 22eb772，未推送，完整原值、编译器和未解决Pack差值见 [连续游标报告](benchmarks/2026-10-18-gosproto-array-cursor/RESULTS.md)。下一轮另行针对大型整数记录遍历，Decode及正式依赖继续冻结。

## 36. 整数记录 Fields 局部复用未采用

只在两个整数记录helper内捕获schema.Fields切片头，按原schema.order索引，净增两行；ARM64确认每字段两次描述加载移到循环外，索引检查与预算保留。没有新增缓存/字段复制/unsafe。复用原43项，四次固定采样共16512块独立复算一致；self32/43、clone35/43和全部原值保留。

大型纯Encode−0.75%/−1.17%（48/76 ns），其self/clone约−0.69%/−0.72%且self离散未过控制；不做数值相减“校正”。PEncode+0.05%/+0.18%，RPC通知+1.07%/−0.16%、请求+1.29%/+0.32%、响应−0.32%/+0.52%。组合路径没有稳定收益，撤回生产候选；没有最低百分比淘汰线，也不将少加载直接当作提速。新Pack控制以0c1f6cf为基线，不能据此宣称上轮6d→0c的+9.45%已解决。

增强既有记录预算/tag夹具：原生Go声明顺序打乱、加入忽略字段，与原顺序具名参考类型对照。相关用例在基线/候选通过；独立负向归档故意按声明位置遍历，新夹具按预期失败，旧夹具在同一错误实现仍通过，证明覆盖增量。采样后验证candidate指纹，再只撤回自己的生产文件；最终所有生产Go源码与0c1f6cf一致，只保留测试增强。未重复未变化生产库的全测/官方/race/fuzz或GSR/GoTS；完整编译证据、撤回候选与两轮数据见 [Fields实验](benchmarks/2026-10-18-gosproto-record-fields/RESULTS.md)，测试和记录已本地提交为gosproto bd1976a / GSR 4bf1b32，未推送。

下一候选检查记录入口的reflect.Value重复处理：在已验证数组入口借用现有encodePointer，减少IsNil与UnsafePointer/反射值传递；每阶段重读当前对象、不扩大unsafe、不缓存字段值，保留nil/tag/预算/元信息替换和失败原子性。本轮未实现或承诺收益，Decode与正式依赖不变。

## 37. 整数记录指针入口

整数记录指针入口改用已验证数组分支借用现有encodePointer，helper避免三字反射值与重复nil类型分支，生产净减3行，不扩大unsafe或改变预算/wire/所有权。固定43项两轮中，大型Encode约快7.9%～8.3%、PEncode快5.6%～7.3%、三种RPC快5.2%～7.2%，小型也改善，目标分配中位数相同。补充8项nil矩阵确认全nil256记录约慢193～217ns（4%～5%），混合数组更快；明确接受该局部代价，保留候选。主self35/43、clone34/43及全部失败/累计方向相反/离群块保留，第二轮未改int32请求+4.74%未解释；旧Pack+9.45%仍不覆盖。库/官方/race/checkptr/vet、441809次有限Codec fuzz、GSR三包及Go/TS7/7通过。见 [指针入口报告](benchmarks/2026-10-18-gosproto-record-pointer/RESULTS.md)，该切片已本地提交为gosproto a3b3830 / GSR f37fef0，未推送；Decode和正式依赖继续冻结。

下一轮只评估已验证数组入口的nil分支位置，不预设最小收益比例；先确认能减少nil代价且不抵消非nil收益，再决定保留。

## 38. nil 调用分支前移未采用

仅在两个整数记录数组调用点用IsNil跳过nil指针提取，helper仍处理布局/预算，固定43项及8项nil补充、各两轮。全nil256记录约快322～426ns，但大型Encode回退8.85%～9.09%（519～538ns）、PEncode4.15%～4.84%、RPC3.46%～5.46%，混合数组也回退，故撤回候选，不以最低百分比裁决。未进入该分支的int32纯Encode也回退约13%；内联4字节核心15条指令相同、具体原因未定，完整保留。主控制35/43、37/43与nil5/8、6/8、累计方向不同和字节尾差均披露。相关基线/候选测试、独立复算/编译/归档核对通过；最终所有Go源码与a3b3830相同、库工作树干净，没有重复未变生产的全验收。见 [nil分支报告](benchmarks/2026-10-18-gosproto-record-nil-branch/RESULTS.md)，该实验记录已本地提交为GSR a391e42，库保持a3b3830，未推送；Decode及正式依赖不变。

下一轮先从a3b3830刷新大型Encode/PEncode及真实RPC profile，确认当前热点再做局部优化。

## 39. 指针入口优化后的热点刷新

从已提交a3b3830归档刷新四种输入的纯Encode/EncodePacked/RequestEncode+CancelSession，CPU各2秒、逐次分配另起进程各20000次，共24份profile；不作为A/B或PB/TS排名。整数数组Pack约34%/39%，大型记录预检/写出与Pack仍各自可见；父子累计不相加。大型三个入口3968B/2次，其中512B布局外溢，其余输入一次结果分配。4KiB ASCII及整数数组纯Encode的kevent/GC栈占比保留，不归因协议或据此排名。harness检查/vet/构建、选择/指纹及归档复现通过，库保持a3b3830干净，未重复生产全验收。见 [优化后热点报告](benchmarks/2026-10-18-gosproto-post-pointer-profile/RESULTS.md)，该诊断已本地提交为GSR 17996e0，未推送；下一轮先检查普通word Pack游标/BCE，再做单一候选，Decode和正式依赖不变。

## 40. Pack 剩余目标游标未采用

只改packInto目标游标：推进剩余dst并从原packed起点返回，保留cap及FF/尾部/重叠规则。BCE总检查点未减少，ARM64汇编301→309行、栈帧112B不变；既有相关回归及17984组wire/cap/独占差分通过。固定原43项+原19形态，23808块独立复算一致。大型纯Pack+1.33%/+1.68%，大型和整数PEncode方向翻转，四/六非零长word约慢0.1μs；部分RPC/零场景改善与控制限制完整保留，综合撤回候选，不设最小收益比例。self49/62、clone38/62、累计方向不同/离群/字节尾差保留；最终全部Go源码与a3b3830相同、库干净，未重复未变生产的全验收。见 [Pack游标实验](benchmarks/2026-10-18-gosproto-pack-cursor/RESULTS.md)，该实验记录已本地提交为GSR 13d4609，库保持a3b3830，未推送；Decode及正式依赖不变。

下一轮先核对密集word写出串行位置计算、mask生成及历史前缀候选，不同时改FF/body或扩大unsafe。

## 41. 当前主循环密集前缀写出

核对历史前缀源码/二进制后，仅将当前Pack密集word串行index改为字节前缀和，写入位置/顺序、FF/稀疏/尾部、offset及重叠/cap/独占不变。ARM64汇编301→297行，栈帧112B、检查调用点7/6不变，生产净减6行。固定原62项两轮：大型Pack快8.6%～9.5%、PEncode1.9%～4.6%、三种RPC2.7%～5.0%；整数数组Pack快7.2%～9.1%、PEncode4.9%～5.8%、RPC4.6%～6.3%，保留候选。小型稀疏约1～4ns代价、未改路径+21ns、累计方向不同、self52/62/clone46/62失败及0.125B尾差均保留。相关回归/17984差分、库/官方/race/checkptr/vet、有限Pack3995与Codec478204次fuzz、GSR三包/GoTS7/7通过；Pack计数停增的覆盖限制披露。见 [主循环前缀报告](benchmarks/2026-10-18-gosproto-pack-prefix-main/RESULTS.md)，已本地提交为库a161158、GSR d6c672c，未推送，正式依赖和Decode不变。

下一轮单独检查512B布局外溢的实际项数与增长策略，不放大小消息栈、不引入池/缓存，先量化再选局部候选。

## 42. 布局外溢项数与增长诊断

已提交a161158归档中核对109种形态、317个入口分配观测，并对48个项数做192组增长模型；诊断helper只在独立归档，分配入口调用未插桩基线。game_large共131项（栈内32+外溢99），512B布局一次容纳；精确最终容量模型只省96B，初始64反而512B/1次→768B/2次。长数组才有累计增长成本：4096条平坦记录布局49280B/8次；按需倍增模型32256B/6次，但1057项嵌套形态多3456B、4129项最终容量多10992B，完整保留，尚无CPU或真实API收益结论。

预算很小时原路径可零分配失败；数组长度已计入预算不代表各元素的深度/字段/字节均通过，不能据此提前大块预分配。既有fixture、诊断/模型检查与vet、独立复算、fresh归档指纹核对通过；生产未改、库干净，不重复生产全验收。见 [布局增长诊断](benchmarks/2026-10-18-gosproto-layout-audit/RESULTS.md)，该诊断已本地提交为GSR c201237，库保持a161158，未推送。下一轮只实验按需增长，保留初始128/栈内32，固定包含中间长度代价形态的真实Encode/PEncode/RPC矩阵后再判断；Decode和正式依赖不变。

## 43. 调用内布局按需倍增

只在外溢区满载时按倍增分配/复制，保留初始128/栈内32、逐项预算、记录次序与wire/cap/独占，不按数组总长预分配。原43项+9形态×3真实入口，固定70项两轮：1024/4096 full 的请求快1.7%～4.6%，4096个含整数数组记录的Encode/PEncode/请求快7.3%～9.2%，230784B/12次→155136B/8次；nil/half长数组同向改善，保留候选。256/512常规追加有约0.1～0.7μs成本，32×32纯Encode慢2.9%～6.3%且多3456B，32×128最终布局多10992B；game_small PEncode多18/28ns、game_large请求第二轮多197ns，全部披露。

add仍内联但两个预检helper帧各增16B，不宣称全部栈成本不变。self34/70、clone45/70；未改纯Pack存在回退但四二进制296条符号指令一致，wide64大幅翻转原因未定，原值/控制/累计/离群全留。扩展既有测试、109形态525差分、独立复算、归档核对、官方/库/race/checkptr/vet、有限fuzz（Pack6822且3s后计数停增、Codec483726）、GSR三包/GoTS7/7通过。见 [布局按需增长报告](benchmarks/2026-10-18-gosproto-layout-growth/RESULTS.md)，该轮已本地提交为库bdfe1d0、GSR bf828b0，未推送，正式依赖和Decode不变。下一轮单独检查正常追加的len/cap重读与帧成本，保持本轮容量序列及70项矩阵。

## 44. 连续逻辑索引替代冗余追加路径

依据调用内extra长度=count−32不变量，复用逻辑索引判断满载并安全reslice/写入；初始128及倍增序列、add接纳点、头项回填、预算/wire/cap/独占不变。计时前草稿内联成本81超预算，改等价max规则后76保持内联，草稿补丁/记录保留、未采CPU。固定原70项两轮：大型Encode快2.94%～4.67%、PEncode2.52%～3.19%、实际请求3.13%～3.48%，主要PEncode/请求/响应控制均通过；全部分配中位数相同，保留候选。

两个预检helper各少20条符号指令，整数记录帧144→112B、通用帧272B不变，切片/索引检查点增加；int32纯Encode多35～49ns、zero手工帧多154～166ns及未改纯Pack+16%～17%完整披露。self40/70、clone38/70，纯Pack四二进制296条符号指令相同，原因未定，所有控制/累计/离群不删。既有回归、109形态525差分、独立复算/归档、官方/库/race/checkptr/vet、有限fuzz（Pack9053且3s后停增、Codec475186）、GSR三包/GoTS7/7通过。见 [连续索引报告](benchmarks/2026-10-18-gosproto-layout-index/RESULTS.md)，该轮已本地提交为库7397804、GSR 25b0b0d，未推送，正式依赖/Decode不变。下一轮先只读刷新当前profile并核对反复出现的未改4KiB路径波动，不以新诊断覆盖旧比较。

## 45. 当前profile与混合夹具GC干扰

7397804只读采26份profile（4输入×3入口CPU/内存+两种纯Pack环境CPU），大型仍3968B/2次，数值数组组合Pack累计37%～41%；父子累计不相加。固定同二进制/同函数4KiB A/A四条件，384块独立复算：Pack full中位−3.35%/IQR11.76pp，isolated−0.72%/1.04pp；isolated仍一轮−24.5%，zero frame隔离也未通过，不重跑。full Pack瞬时堆637MB、25块无GC完成，GC相位与存活夹具显著相关。

另起无耗时的limiter事件诊断确认full计数0→1279、isolated0→0，堆峰459/13MB；仅证明该补充进程loop中启用，不能回填原A/A或旧比较。24项body/RPC形态统计显示整数密集word连续非零段约60%，大型body约8%/请求17%，不等同分支执行频率。harness/vet、独立384块/24形态/194点复核、源/26profile/二进制/fresh归档指纹通过，生产未改/库干净，不重复生产全验收。见 [当前热点与GC上下文诊断](benchmarks/2026-10-18-gosproto-post-layout-profile/RESULTS.md)，该轮已本地提交为 GSR `a125ab4`，未推送。下一轮先补单项独立进程采样，保留混合数据；确认后再单独实验普通word连续非零段批量写出，不关闭GC、不改cap/预算或Decode。


## 46. 原70项逐项独立进程控制

保持原矩阵、输入、顺序和默认 GC，self/clone 各70个独立进程；构造所有夹具后仅保留所选操作，清掉输出引用再 GC。计时块/有界校准源码不变，4次状态快照只在计时/分配区间外读取。140个进程一次完成，13,440块、3,360 quartet、840轮、560快照独立复算一致；两组各61/70通过（旧混合环境40/70、38/70），所有分配中位数相同，limiter全程观测为零，最大堆快照约4.20MB，不能当峰值。

仍有17个不同入口至少一组失败、单轮最大离群self +71.34% / clone +33.02%，完整保留，未重跑或扣噪声。旧/新环境与二进制布局不同，不把通过数或绝对耗时变化解释为codec加速。harness测试/vet、140逐项与合并原值、fresh归档和源码指纹通过；生产未改，不重复生产全验收。见[逐项进程报告](benchmarks/2026-10-19-gosproto-isolated-cases/RESULTS.md)，该轮已本地提交为 GSR `4ff411f`，未推送。后续用逐项进程补充比较，继续保留混合历史；下一轮只实验普通word连续非零段批量写出，按累计纳秒、内存与复杂度评估，不设最低收益比例，不改Decode/正式依赖。


## 47. 普通word连续段批量写出实验未保留

只在归档对3～7个非零字节的连续段增加一次word写出，其余回到前缀路径。既有全mask/FF/尾块/EncodePacked重叠与17,984差分通过；固定原70项，self/clone和两轮比较共280个逐项进程，26,880块/1,680轮/1,120快照独立复算、harness/vet/fresh指纹通过，无重跑。指令296→328、帧112B不变；整数数组Pack慢5.45%～6.36%，大型Pack慢10.28%～11.64%，大型请求配对多349.1～393.8ns，目标没有预期收益，候选不应用到库。

self/clone仅13/70、42/70通过，limiter全零，故不宣称损失比例精确或把全部波动归于limiter。nil4096 PEncode两轮−0.80%/−1.47%的小收益、累计方向不同、控制失败/分配尾差与离群全部保留；不是最低收益门槛或单独因控制失败而拒绝。见[连续段实验](benchmarks/2026-10-19-gosproto-pack-contiguous/RESULTS.md)。库干净、源码仍7397804；不保留候选，不重复发布级官方/race/checkptr/fuzz/GSR/GoTS。该轮已本地提交为 GSR `d1dceaa`，未推送。下一轮先分析安全切片定位与计数数据流是否能真实减少重复工作，再单独实验；不据此否定所有连续段实现，不改Decode/FF/cap/预算/正式依赖。


## 48. 固定payload视图减少逐次索引加法

保留dst[8]检查，普通word使用dst[1:9]的固定8字节视图，七个1+prefix索引改为0～7的prefix；地址/字节/store顺序、最终计数、容量与所有权完全相同，无新分支或unsafe。预检及实际二进制296→288条符号指令、112B帧不变。固定原70项四组280进程：大型Pack快8.87%/9.13%，整数数组PEncode快4.70%/4.19%，大型请求配对少154.1/206.5ns；4096条含数组记录的请求只快1.13%/0.83%也计入收益。精确候选保留到库工作树，尚未提交。

self/clone各56/70通过，limiter全零；全部控制/分配尾差/累计值保留。大型PEncode、4096条full记录请求等有累计方向翻转（后者pass2有233.678ms/43调用的B块），wide64 PEncode两轮多39.0/4.0ns，不能只报配对赢家。既有契约/17,984差分、26,880块复算、harness/vet/fresh指纹、库全测/官方C-Lua/race/checkptr/vet、有限fuzz（Pack36,026且3s后停增，Codec442,757）、GSR三包/GoTS7/7零跳过通过；oracle沿用相同SHA的官方core和Lua facade，parser来源单独披露。见[固定payload报告](benchmarks/2026-10-19-gosproto-pack-payload/RESULTS.md)。该轮已本地提交为库 `0b38019`、GSR `31b3f99`，未推送，正式依赖/Decode不变；下一轮再看剩余切片定位和计数数据流，不在本轮叠加。


## 49. 分支内固定len/cap视图

三份计时前草稿均通过既有契约/17,984差分：两索引固定len仍有指针掩码（304条），最高字节计数偏置未生成MADD（288条），不计时；只选三索引固定len/cap3/9（296条，基线288，均112B帧）测量，目标动态定位序列消失，未按总指令数自动否决。所有生产调用者输出len==cap，临时cap限制不影响公开结果cap或存储地址/顺序；计数/FF/尾块/预算不改。

固定原70项四组280进程/26,880块独立复算：整数数组PEncode快6.07%/3.73%，真实通知快5.68%/5.17%、请求快3.92%/5.08%，这些入口控制均通过，保留精确候选。self46/70、clone51/70；ASCII4KiB请求多5.1/16.5ns、大记录请求多2,284.1/1,291.7ns、全部累计翻转/异常块/16B尾差均保留，不宣称全面提速。harness/vet/fresh/所有指纹、库全测/官方/race/checkptr/vet/有限fuzz（Pack5,487且3s后停增；Codec416,938）、GSR三包/GoTS7/7通过。见[固定len/cap报告](benchmarks/2026-10-19-gosproto-pack-window/RESULTS.md)。该轮已本地提交为库 `d2f6027`、GSR `175028b`，未推送，正式依赖/Decode不变；后续组合路径profile见第50节。


## 50. 固定视图后的单项进程 profile

从已提交d2f6027归档，原70项构造后仅保留所选闭包，GC/预热后手动开启CPU；16个入口分别CPU2秒及20,000次逐次分配，共32进程/96快照，无重试。limiter全零、最大堆快照约3.85MiB（非峰值）。CPU与分配采样分开，完整内存profile仍包含构造/预热，B/op以循环计数为主；不把profiler耗时当新A/B或PB/TS排名。

整数PEncode/真实RPC的Pack累计26.58%～31.74%，大型记录预检/写出在PEncode/RPC各12.83%～16.88%，父子累计不相加。大型3968B/2次仍为输出+512B布局，其他主要组合一次输出分配；不再把记录内部成本笼统归于safe reflect。4KiB ASCII/零帧kevent约70%～75%，沿runtime栈，原因未定、原值不删/不校正，limiter零不等于无GC成本。

harness统计/选择/GC/夹具/指标/70项顺序、vet、32采集和fresh归档、源/二进制/profile指纹独立核对通过；生产未改/库干净，不重复生产全验收。见[当前编码热点报告](benchmarks/2026-10-19-gosproto-current-profile/RESULTS.md)。该轮已本地提交为GSR `7c91913`，未推送，Decode/正式依赖不变；后续记录头部视图实验见第51节。


## 51. 整数记录固定两字节头部视图

只将encodeIntegerRecordInto两处header store的动态剩余切片改为固定len/cap2，消除两处指针掩码；216→212条符号指令、32B帧与14个panic调用点不变。预检、字段/值读取、内容写出、布局及Pack/Decode不改。基线/候选既有记录/tag/nil/元信息/预算/RPC契约和109形态525项差分通过；计时前有一次before模块改名导致测试导入失败，修正独立modfile后再采正式矩阵，失败与零计时证据保留。

固定70项四组280进程，26,880块/1,680轮/1,120快照独立复算：4096条full记录请求快2.50%/1.19%（少5,418.2/2,677.4ns），half记录与32×128嵌套PEncode同向改善；小型请求只少10.5/14.3ns也记入。大型游戏收益很小且有翻转，含数组记录请求多1,509.3/315.8ns、小型PEncode多5/6ns，均接受披露；不设收益比例门槛或宣称全面更快。self63/70、clone62/70，limiter全零；分配中位数70/69/68/69项相同，16B尾差、27项累计翻转和262.349ms等离群全留。未改Pack四二进制296条指令一致但仍有耗时漂移，原因未定，不扣噪声。

harness/vet/顺序/源/二进制/fresh、库全测/官方/race/checkptr/vet、有限Codec fuzz436,240次、GSR三包/GoTS7/7零跳过通过；gofmt无变化、生产Go源码等于冻结候选。见[记录头部视图报告](benchmarks/2026-10-19-gosproto-record-header/RESULTS.md)。该轮已本地提交为库 `9067933`、GSR `ca02105`，未推送，Decode/正式依赖不变；后续inline内容切片见第52节。


## 52. inline整数延后内容视图

记录写出仅在非inline分支构造内容切片，inline仍用原公式；其余整数复用原encodeIntegerInto，不改预检/字段值/store顺序/预算/布局/Pack/Decode。212→216条指令、32B帧不变、panic调用点14→15；重复inline判断折叠，非inline长度写出却多一次定位，保留静态代价实测。扩展既有记录数组测试10个inline/4vs8字节边界，先在基线通过；负向上限+1在32767被新用例捕获、旧用例仍通过，证明覆盖增量。

固定原70项四组280进程/26,880块独立复算：大型Encode快2.57%/2.34%、PEncode快1.72%/3.26%、请求3.18%/2.44%、响应2.29%/2.44%，这些入口控制均通过、分配不变，保留候选。接受256～4096混合full记录约0.5%～1.5%成本（部分PEncode2.32%），half请求多1,551.7/1,213.8ns，未改整数数组PEncode多21.0/78.9ns也披露；不设收益比例门槛，不宣称全面提速。

self60/70、clone64/70，limiter全零；分配中位数68/69/70/69相同，16B尾差/28项累计翻转/119.294ms大型请求等离群全留。Pack四二进制296条指令一致；通用字段写出864条但反射Float错误路径一条ADD立即数不同，过强整函数一致断言及差异保留，不做噪声校正。既有契约/109形态525差分、harness/vet/顺序/源/二进制/fresh、库全测/官方/race/checkptr/vet、Codec fuzz470,086次、GSR三包/GoTS7/7零跳过通过。见[inline内容视图报告](benchmarks/2026-10-19-gosproto-record-inline/RESULTS.md)。该轮已本地提交为库 `2ea804e`、GSR `b004da7`，未推送，正式依赖/Decode不变；后续非inline内容视图复用见第53节。


## 53. 非inline内容视图复用

记录写出仅将非inline内容切片保存给原整数和后续长度store共用，地址/顺序、条件/公式、预检/预算/布局/Pack/Decode均不变，无新分配/unsafe。216→216条指令、32B帧，panic调用点15→14；第二次SUB/NEG/AND消失，但inline新增两条寄存器清零指令，静态取舍完整保留。

固定原70项四组280进程/26,880块独立复算：大型PEncode快0.51%/0.77%（35.3/54.2ns），half4096真实请求少1,390.3/982.1ns，控制均通过、分配不变，保留小收益。full4096请求多1,810.3/593.0ns（两组控制失败）、含数组记录配对成本、大型Encode/请求翻转全留，不说上一轮混合记录代价已全面解决，不合并独立基线比例。self50/70、clone56/70，limiter零；分配中位数69/69/69/68相同，16B尾差/27项累计翻转/270.225ms等离群不删。未改Pack296条/通用字段864条序列四二进制相同，耗时仍漂移、原因未定，不校正。

既有含10边界的记录用例/元信息/预算/RPC契约、109形态525差分、harness/vet/顺序/源码/二进制/fresh、库全测/官方/race/checkptr/vet、Codec fuzz512,102次（5个新增interesting，无失败）、GSR三包/GoTS7/7零跳过通过。见[非inline内容视图报告](benchmarks/2026-10-19-gosproto-record-body-view/RESULTS.md)。该轮已本地提交为库 `e50dfef`、GSR `29c954b`，未推送；正式依赖不变。用户随后解除Decode冻结，当前切片见第54节。

## 54. Decode解冻与入口即时证明复核

用户明确“decode不再冻结”，入口、解析、构造/写入重新开放按证据优化，不在切片结束后自行冻结。此前入口split候选曾因3%/2%统一门槛未采用，本轮以e50dfef为新基线独立评估，不改写旧实验，也不使用最低收益比例。

parser复用已有value.Elem()所得record的实际类型，调用点即时核对原生能力/同一Go类型；私有helper仅接收该事实和同一value借用地址。原helper成本216不能内联，新67可内联，parser616→664条/320B帧不变，真实类型化赋值写屏障保留。无新unsafe/分配/缓存/API，不动Encode/Pack/Unpack/RPC/meta和预算/失败原子性/独占模型。

新40项×四模式160进程/15,360块独立复算：大型Decode快2.37%/2.02%（369.2/314.6ns），full4096 Decode快3.04%/2.83%，nil4096 Decode快6.19%/7.06%、PDecode快6.40%/6.94%，保留小幅和结构增长收益。大型RPC通知+0.63%/+0.03%、PDecode控制失败及第一轮累计+492.9ns、Echo小成本和未改Encode漂移同时披露，不宣称全路径获益。self27/40、clone35/40，所有对象数中位数相同，B/op尾差/7入口累计翻转/离群不删，limiter全零。

四项当前基线CPU诊断先于计时，kevent占比原样保留，不用profile份额推演加速上限。fresh重建/源码/二进制/命令顺序/160单项SHA、库全测/官方/race/checkptr/vet、Codec/Decode有限fuzz497,350/1,217次、GSR三包与GoTS7/7零跳过通过；Decode fuzz后段计数停止如实记录，不盲目重跑。见[解冻与入口报告](benchmarks/2026-10-19-gosproto-decode-resume/RESULTS.md)。该轮已本地提交为库 `913c465`、GSR `b708108`，未推送；正式依赖v0.4.0，Decode继续开放，结构数组子schema实验见第55节。

## 55. 结构数组子schema局部复用（未采用）

只在decodeStructSlice的countChunks/MakeSlice之后保存私有schema链接，保留循环内schema.typ读取、每元素独占reflect.New与通用parser，未缓存公开Fields/值或新增unsafe。两次循环字段加载变为循环前一次，符号指令208不变、栈帧224→240B；据此进入同40项实测，不按静态条数决定收益。

固定160进程/15,360块独立复算：含数组记录4096纯Decode快0.92%/1.05%（4,482.9/3,973.2ns），局部收益保留在报告；大型Decode+1.25%/+0.79%、通知Dispatch+0.48%/+0.65%，部分控制失败，nil4096 Decode+1.59%/+1.81%、PDecode+1.25%/+1.87%，nil两类控制均通过且累计同向。未改Encode/PEncode也漂移，不校正；基于主要目标成本和局部收益的整体取舍不采用，不设置最低收益比例或断言栈帧是唯一原因。

self27/40、clone30/40；对象数中位数均相同，25项字节尾差、22入口累计翻转、1,858.944ms等离群原值保留，limiter零。目标契约/109形态525编码控制/40解码wire语义、harness/vet/四组构建与汇编/顺序/源码/二进制/fresh通过；预检查缓存权限失败在计时前解决，未清缓存或重试计时。候选从未应用主库，18份Go源码仍等于913c465，未重复生产全测/官方/race/fuzz/GSR/GoTS或PB/TS排名。见[数组schema报告](benchmarks/2026-10-19-gosproto-decode-array-schema/RESULTS.md)。该轮报告已本地提交为GSR `26a5748`，未推送；正式依赖v0.4.0、库保持913c465，Decode继续开放。

## 56. 原生整数写入资格前移（未采用）

原生整数helper保留pointer/native类型/owner/Wire/Array全部守卫，在唯一独占new(int64)前计算资格，返回同一对象和written供原生store或原反射索引回退。scalarBits/错误/预算仍在前，不扩展unsafe或改变分配/失败原子性/独占/写屏障模型。helper成本39→56仍内联，parser664条/320B帧不变，重复标记加载2→1，新增资格布尔值跨分配保存，不据此推演加速。

固定40项两轮：大型Decode+1.76%/+1.51%、PDecode+0.90%/+3.64%，部分控制失败；含数组记录4096 Decode+2.31%/+1.22%、PDecode+1.37%/+0.84%，两类控制通过且累计同向更慢。小型/嵌套PDecode局部收益完整保留，基于目标成本取舍未采用，不设最低收益比例，不将布尔保存单独定为回退原因。self28/40、clone25/40，全部对象数中位数相同、26项字节尾差/18入口累计翻转/全部离群保留，limiter零。

160进程/15,360块复算，既有目标契约/109形态525编码控制/40入口wire语义、harness/vet/构建/机器码/源码/二进制/fresh核对通过，无准备或计时重试。候选从未应用主库，18份Go源码仍等于913c465、工作树干净，未重复生产完整验收或PB/TS排名。见[整数资格前移报告](benchmarks/2026-10-19-gosproto-decode-integer-proof/RESULTS.md)。该轮报告已本地提交为GSR `f7381dd`，未推送；依赖v0.4.0，Decode继续开放，header视图见第57节。

## 57. 固定两字节header读取视图

只将readUint16的开放尾切片收窄为chunk[2+index*2:4+index*2:4+index*2]。count/total在循环前的len校验保证每个视图都在原两个字节内，32位int也不溢出，空循环不创建视图；不改变公开输出cap、预算/错误/独占、未知字段chunk校验或任何写入guard。两索引初稿仍有cap调整，证据保留且未计时；计时前补全len/cap2，parser664→660条/320B帧不变，真实类型化赋值写屏障保留。

固定40项两轮half4096 PDecode快2.41%/2.54%（11,141.8/11,794.6ns），双控制通过、累计同向，保留单行收益；大型PDecode快1.72%/1.89%、嵌套PDecode快3.11%/3.94%，部分控制失败。接受4KiB Echo Dispatch+30.0/+52.9ns、小型PDecode+32.4/+14.4ns、含数组记录纯Decode+5.19%/+0.11%等成本，不宣称全路径改善；self21/40、clone13/40偏低、15入口累计翻转/全部异常原值保留，未改控制漂移不校正，无最低收益比例门槛。

160进程/15,360块复算，源码/二进制/160raw/fresh、目标契约/40解码语义/525编码差分/harness/vet，库全测/官方/race/checkptr/vet、有限Codec/Decode fuzz432,127/38,718次、GSR三包/GoTS7/7零跳过通过；Decode fuzz后段计数停止如实记录。对象数中位数相同，字节尾差保留，limiter零。见[header视图报告](benchmarks/2026-10-19-gosproto-decode-header-view/RESULTS.md)。该轮已本地提交为库 `503a725`、GSR `2104eca`，未推送；正式依赖v0.4.0、Decode继续开放。

## 58. Header字节偏移遍历（未采用）

只保存初始headerEnd := total，按offset从2起步进2读取固定len/cap2视图，避免每轮2+index*2。headerEnd不随payload消费增长，序列/次数/字节与旧循环相同，count0/32位范围和原边界/预算/错误/独占/全部guard保持。parser660→656条、320B帧不变，结构数组及真实写屏障保持；静态减少不直接作为采纳依据。

固定40项两轮大型Decode/PDecode/通知有局部收益，但控制均失败，第二轮Decode/PDecode累计反向；小型通知快1.21%/1.16%也保留。整数数组PDecode+1.10%/+1.24%，两类控制通过且累计更慢，含数组记录PDecode+1.68%/+1.57%，部分控制失败。组合路径取舍未支持采用，不设收益比例门槛，未改控制漂移不校正。self20/40、clone19/40，22项字节尾差、16入口累计翻转和全部离群保留，limiter零。

160进程/15,360块独立复算，目标契约/40入口wire语义/525编码控制、harness/vet、构建/汇编/顺序/源码/二进制/fresh核对通过。主库从未应用补丁，18份Go源码仍等于503a725且干净，未重复生产完整验收或PB/TS排名。见[字节偏移报告](benchmarks/2026-10-19-gosproto-decode-header-offset/RESULTS.md)。该轮报告已本地提交为GSR `9699583`，未推送；库保持503a725，依赖v0.4.0、Decode继续开放。

## 59. 当前Decode/PDecode重新定位（只读诊断）

在503a725上精确选大型与896整数数组的Decode/PDecode四入口，每项CPU2秒和独立memprofilerate1固定20,000次，共8进程/24快照；清三个sink后GC/预热再手动启动CPU，内存profile保留初始化/预热影响，loop B/op与对象数另列。大型Decode/PDecode分别10416B/532、13872B/533次，整数数组8288B/4、12384B/5次，两类独立进程计数一致。

大型CPU样本仍指向parser/子结构构造/独占整数；整数数组纯Decode的decodeIntSlice累计34.60%、readUint32累计8.44%，PDecode则Unpack30.90%而数组decoder12.88%，父子累计不相加。kevent各33.82%～60.76%原样保留，不归一化或推演加速上限，profile耗时不作A/B基线。实际4/8字节数组循环仍有开放尾切片len/cap/NEG/ASR/AND处理，下一轮单独验证固定元素视图，并补8字节大数组与短包/游戏/PDecode控制；当前仅定位，未实施候选。

harness测试/vet/40项顺序与构造校验、构建、8采集/20pprof文本/机器码、fresh及全部source/binary/profileSHA核对通过，limiter零，最大堆快照3.86MiB。生产18份Go源码等于503a725且库干净，未重复生产完整验收/PBTS排名，不把旧差分次数当本轮新验证。见[当前Decode诊断](benchmarks/2026-10-19-gosproto-decode-current-profile/RESULTS.md)。该诊断已本地提交为GSR `16d1491`，未推送；库基线503a725，依赖v0.4.0，Decode继续开放。

## 60. 原生整数数组固定4/8字节读取（保留）

只收窄native []int64两处元素切片len/cap，保留宽度/整除/预算预检、符号扩展、精确分配、命名类型回退、失败原子性和独占；不加unsafe/池/parser/API，不改Pack/Unpack。实际ARM64数组decoder360→348条、160B帧，parser不变。扩展原NativeIntegerArrayWire的40宽度/长度组合，增加Decode/DecodePacked、消费长度与结果/wire独占。

固定52项208进程/19,968块，两轮4字节896元素Decode−25.88%/−26.84%、PDecode−6.86%/−8.44%、真实Dispatch−8.77%/−8.76%；8字节Decode−28.09%/−28.50%。短8字节PDecode+1.95%/+0.81%仅数ns也披露；8字节大PDecode首轮累计+7157ns、次轮−1431ns，不能称组合稳定全面加速。大型游戏无稳定收益。self21/52、clone23/52，目标多项控制失败；全部对象数中位相同、29项字节尾差、19入口累计翻转、limiter零及离群保留。采用依据为两种宽度重复逐轮收益、真实4字节组合收益和两行局部改动，无最低比例门槛。

归档git apply静默跳过测试补丁，生产补丁正确；指纹验收发现后保留原测量，独立强化基线测试+候选完整验收验证，未冒称双侧新测试在计时前运行。修正复现入口用patch并校验测试SHA，本轮只prepare验证。库全测/固定官方oracle/race/checkptr/vet、两项有限fuzz、GSR三包与Go/TS7/7零skip通过，源码/原fresh/checked-fresh/二进制/工具指纹已核对。Decode fuzz三秒后停滞限制保留，未重测PB/TS排名。见[数组读取报告](benchmarks/2026-10-19-gosproto-decode-array-view/RESULTS.md)。该轮已本地提交为库 `f7022ac`、GSR `b3d4517`，依赖v0.4.0，Decode继续开放。

## 61. Unpack重新定位与仅源游标（保留）

在f7022ac只读采4/8字节数组Unpack/PDecode四个CPU进程，12快照limiter零，全部18份源码/工具/二进制/fresh核对。8字节PDecode的Unpack累计43.23%、数组decoder7.42%；4字节为35.00%/13.75%。kevent和递归累计限制保留，profile耗时不作新基线。见[当前Unpack诊断](benchmarks/2026-10-19-gosproto-unpack-current-profile/RESULTS.md)。

与旧cursor同时改变目标区间/&7不同，本轮保持目标地址不变，只将普通mask逐有效字节切源切片改为src[used]，word后一次推进；完整预检、源/目标边界检查、预算/错误/独占/分配、零/FF/Decode不变。实际循环回跳22→15条、整函数仍384条，帧128→112B。原52加全部18现有Unpack形态，共70项280进程/26,880块固定比较。

两轮大型Unpack−6.33%/−6.15%、PDecode−1.16%/−1.42%、Dispatch−1.23%/−1.74%；4字节数组PDecode−4.69%/−3.34%、8字节−4.16%/−4.00%。约1%业务收益也保留，无最低比例门槛。FF接稀疏+11.47%/+10.55%且clone漂移不扣除，短零段第二轮+14.28%、短包与未改Decode代价均列出，不按fixture加分支。self59/70、clone52/70；全部对象数中位相同、27项字节尾差、17入口累计翻转和GC离群保留，limiter零。

复用已提交全mask/偏移/后继/坏输入/预算/ownership测试，两侧归档测试相同。本轮8条验收通过：官方oracle/库全测、race/checkptr、vet、3项各10秒有限fuzz、GSR三包、Go/TS7/7零skip；Pack/Decode fuzz三秒后停滞限制保留。源码与冻结候选逐字节相同，正式依赖文件不改。见[源游标报告](benchmarks/2026-10-19-gosproto-unpack-source-cursor/RESULTS.md)。该轮已本地提交为库 `af9169d`、GSR `6b47822`、不推送；Decode继续开放，下一步先独立定位稀疏/短零段代价。

## 62. 代价诊断与预检输入位置游标（未采用）

对f7022ac/af9169d各采FF接稀疏、短零段、双字节word和大型PDecode的CPU，共8进程/24快照，Arm实际包命名空间/源码/fresh/raw指纹核对。未确认代价唯一原因，不扣kevent或跨profile份额推断速度。独立wire探针确认短零段为512个零mask+64FF、没有普通word；FF接稀疏为一个512字节FF段+512个固定0x11 word，与旋转双字节word不同。探针初次目录错误误扫旧tmp导致失败，更正后独立单项通过，正式依赖未变，未污染此前计时。见[代价诊断](benchmarks/2026-10-19-gosproto-unpack-cost-profile/RESULTS.md)。

候选只将完整预检的remaining尾切片改为position/available输入游标，保持所有读取范围、坏输入先于输出预算的错误优先级、零段扫描上限、size/分配/独占和恢复循环。新增隔离138,500组截断/预算差分，复用库已有契约，计时前通过。ARM64整函数384→396条、112B帧不变；实际70项280进程/26,880块中，4字节PDecode−11.85%/−9.45%、8字节−8.72%/−8.09%、8字节Dispatch−11.38%/−10.88%是保留的正面证据；两种大数组PDecode均12/12轮更快。

不采用的取舍为普通1..7字节word两轮约+4%～+33%、FF接稀疏+10.39%/+6.42%、短零段+2.49%/+0.65%，通用字节分布代价尚未解释，不是收益低于比例门槛。self39/70、clone33/70，全部对象数中位相同；27字节尾差、21入口累计反向、全部离群和控制漂移保留，不校正。313条测量命令/原采样函数/顺序/源码/二进制/fresh核对通过，limiter零。主库从未应用候选，全部18份Go仍等于af9169d且干净，故未重复生产完整验收/PBTS排名。见[预检游标实验](benchmarks/2026-10-19-gosproto-unpack-preflight-cursor/RESULTS.md)。该轮已本地提交为GSR `71d313d`，未推送；依赖v0.4.0、Decode继续开放。

## 63. 相同packed字节的上下文控制诊断

沿用未采用的位置游标补丁，三种业务样本纯Unpack/PDecode及四种普通/零/FF形态各控制源对齐0/1、spare cap0/64，共40项160进程/15,360块。160条实际输入metadata逐条核对：A/B捕获同一wire，同样本四变体SHA一致；业务两入口构造时也共享wire，独立进程仍有各自backing和结果/GC。三个业务的48个compare中位数均更快，单/双字节word与FF接稀疏的24个均更慢，短零段方向混合。对齐/cap/额外Decode调用不足以单独解释原反向，不代表这些因素绝无影响。

原70项及本轮四模式普通mask内循环16条机器字逐字相同，原70两版起点绝对地址差20B；这是代码范围/布局证据，不能扩张到整个恢复阶段或cache/分支预测的唯一根因。self22/40、clone9/40、全部异常/尾差/累计翻转和GC块保留，不校正。193命令、原70/新40语义、138500预检差分、harness/vet、fresh/源码/二进制/顺序/metadata/机器字独立复算通过。见[共享输入诊断](benchmarks/2026-10-19-gosproto-unpack-shared-wire/RESULTS.md)。本轮不改变位置游标未采用状态，主库af9169d干净，新报告未提交。

## 64. 原完整预检私有函数提取（未采用）

仅提取旧remaining尾切片扫描为unpackedSize，调用者成功后照原size分配/恢复，不携带位置游标；扫描语句仅私有错误值nil→0、恢复源码逐字相同，错误优先级/预算/空输入/结果独占不变。实际原384条/112B帧，变为调用者216条/128B和helper180条/32B，无noinline/填充/特定mask提示。

固定70项280进程/26,880块，两轮普通word约快1%～12%、短零段约快3%～5%，收益保留；4字节PDecode−0.03%/+0.33%、大型−1.65%/+1.05%、AddressBook+1.28%/+1.51%，FF接稀疏+26.48%/+24.22%。FF clone本身+27.80%，不能全归因提取也不扣除控制制造收益。当前组合取舍不支持新增调用，不是收益比例门槛。self33/70、clone26/70，全部对象数中位相同、30字节尾差、26入口累计翻转和全部异常保留，limiter零。

313命令/两侧契约、109形态525差分、138500预检差分、harness/vet/原70语义、构建/机器码、来源/fresh/原采样函数核对通过。主库从未应用候选、18份源码仍af9169d且干净，不重复生产全验收或PBTS排名。见[分函数实验](benchmarks/2026-10-19-gosproto-unpack-size-phase/RESULTS.md)。新诊断/报告未提交；下一步若继续，需同一固定来源分别控制扫描算法与阶段分离、直接测交互，不跨轮拼接比值或按fixture修补；Decode继续开放。

## 65. 分函数与索引预检组合（未采用）

直接比较此前缺失的split-indexed组合与生产af9169d，不跨历史轮次相乘预测。私有unpackedSize内索引扫描，完整预检先于分配；恢复/调用者与split-original源码一致，错误优先级/预算/len-cap-nil/所有权不变。编译为216条/128B调用者与196条/32B helper，保留调用；不把机器码条数当性能依据。

固定70项280进程/26,880块：大型PDecode−3.12%/−4.38%、Dispatch−2.87%/−3.36%；4字节大数组PDecode−9.84%/−9.23%、8字节大数组−10.93%/−11.37%。普通word1～7有效字节两轮慢约4%～33%，FF接稀疏+8.53%/+8.41%，短数组PDecode也慢。业务收益全部保留，但新增调用没有消除通用形态代价，未替换生产版本；不设最低收益门槛。8字节大数组首轮中位快10.93%但累计慢643.9ns，第二轮累计快1553.8ns，均保留。

self52/70、clone40/70，全部对象数中位相同，26项字节尾差、20入口中位/累计反向保留；313命令、两侧目标契约/109形态525差分/138500预检差分、harness/vet/汇编/fresh及来源核对完成。未应用生产候选，不重复完整生产验收。此比较未隔离split下扫描算法的单一边际因果，不能将历史独立比值拼接补出结论。见[组合报告](benchmarks/2026-10-19-gosproto-unpack-split-indexed/RESULTS.md)。前轮报告已提交2ca8864，新报告未提交，主库af9169d干净，Decode继续开放。

## 66. 完整预检按word记录预算（未采用）

仅改变单位：maxWords=maxBytes/8、wordsLeft递减，produced按word计数，FF输入消费仍按字节；完整校验后按(maxWords-wordsLeft)*8精确分配。remaining扫描/恢复源码不变，不叠加分函数或索引。每段输出均整word，floor((maxBytes-8W)/8)=floor(maxBytes/8)-W，保持预算/零段扫描上限/错误顺序/结果len-cap-nil与所有权。既有差分增加MaxInt-7/MaxInt-1/MaxInt，共23预算159275组；原mask/截断/FF/零段仍覆盖。

固定70项280进程/26,880块：单/双字节word约快5%～7%、零段小幅改善，收益保留；大型Unpack+7.93%/+6.77%、4字节大数组PDecode+4.02%/+4.02%、8字节大数组+4.13%/+6.24%、FF接稀疏+32.95%/+34.19%。此取舍不支持替换生产版本，不是小收益比例门槛。实际指令384→372但栈帧112→128B，不把静态简化当动态优化。

self48/70、clone39/70；全部对象数中位相同，26字节尾差、22入口中位/累计反向保留，最大堆快照49,684,096B非峰值、limiter零。313命令、两侧契约/109形态525差分/159275预检差分、harness/vet/机器码/fresh/来源/原采样函数核对通过。未应用候选，不重复完整生产验收。见[报告](benchmarks/2026-10-19-gosproto-unpack-word-budget/RESULTS.md)。下一轮不再仅排列计数器写法，应先获取当前生产预检/恢复各阶段成本证据；不冒称唯一硬件根因。前轮885505c已提交，新报告未提交，库af9169d干净，Decode继续开放。

## 67. 当前生产Unpack阶段成本及后续正文候选

af9169d原函数只读采8个独立CPU入口，无候选/配对计时，不拆函数或插入阶段计时器。直接从pprof raw完整inline调用栈将每个样本互斥归属为预检、分配、恢复、other，runtime子调用仅在保留Unpack父帧时归入调用阶段，systemstack/GC/kevent等仍保留other。分母为完整样本，不归一化隐藏运行时；归属不是独立阶段耗时或硬件根因。

大型纯Unpack预检30.40%、恢复32.16%，4/8字节大数组预检35.32%/24.56%、恢复25.96%/26.75%；ASCII预检零样本、分配21.27%，短零段分配14.54%。普通恢复260行包含源读/目标写/地址/边界检查，不能将整行归因某一条store。大型PDecode中Unpack累计17.67%、正文DecodeWithLimits53.88%、decodeStructSlice51.29%，父子不能相加；正文仍有投入价值。

countChunks在大型PDecode累计4.31%；采样后静态核对readChunk已inline，但丢弃content仍有decode.go:35 cap守卫/panicSliceAcap。下一项只考虑countChunks直接读取长度与完整边界、避免未用content视图；保留先chunk校验再逐项budget.add、完整预检先于数组分配、错误顺序/长度安全/失败原子性/独占，不改其他需content路径。本轮尚未实施/测量，不以静态减少预告提速或设收益门槛。

55流程命令＋1静态objdump，8CPU/24快照/1827采样权重，limiter零，最大堆快照4,036,672B非峰值；harness测试/vet/70清单构造/构建、fresh/18源码/工具/夹具/二进制/raw/canonicalB来源核对通过。冻结plan沿用旧40/旧基线等描述已在scope-clarifications逐字段澄清，实际70/af9169d/8CPU，不静默改写测量输入。不重跑生产全验收或给PBTS排名。见[报告](benchmarks/2026-10-19-gosproto-unpack-phase-profile/RESULTS.md)。GSR前轮8e9880f已提交，新诊断未提交，库af9169d干净，Decode继续开放。

## 68. 当前Go Protobuf对照与优先级重排

当前总览（2026-10-20）：[Go SP/PB同场五轮](benchmarks/2026-10-20-go-sp-pb/RESULTS.md)使用gosproto af9169d提交归档与PB v1.36.1，共72项360样本；大型packed编码−8.1%、解码+14.9%，int32数组packed编码−36.5%、解码+62.2%。纯Encode/Decode已领先的数值路径被Unpack拖累，保持Body与Packing双主线；Packing侧优先Unpack→PDecode，Body继续定位大型结构成本，不继续以旧大型Encode数据选题。Go-only、无新profile/TS或细小收益统计显著性结论。库工作区有用户删除，采样隔离归档未触碰；正式依赖仍v0.4.0，本轮无提交。

原五组正文与四组Echo共72项各五轮。大型纯Encode5.555µs vs PB7.665、Decode15.489 vs17.807，但PDecode20.463；int32纯Decode1.739 vs4.294，Unpack5.131、PDecode6.967。Packing侧优先解包组合，Body侧继续改进纯Encode/Decode；保留全字节形态预算/错误/ownership约束；不直接恢复此前有泛化代价的游标/分函数/word预算候选。AddressBook和小型消息还存在固定成本。手工package+append帧不等于真实Request/Attach，UTF8差异含PB的UTF8校验契约，不能据此泛化内核优劣。全部耗时/分配/wire/五轮波动在报告保存，后续局部采用仍须配对验证，不设固定收益下限。

## 69. Body countChunks未用视图候选（收益待确认）

仅在countChunks直接读取长度/检查chunk，不构造readChunk的未用content视图；完整输入边界先于逐项budget.add，数组分配前完整预检、失败原子性/所有权/错误顺序保留，size<=len-4保证int转换安全。真正需要content的调用不改。实际countChunks约97→85条，未用cap守卫1→0，输入推进的边界守卫仍在；不以静态条数推定提速。

固定70项280进程/26,880块，大型Decode−0.72%/−0.10%、full记录Decode−0.12%/−1.51%，收益保留；大型PDecode+1.15%/−2.00%且累计两轮慢32.7/3149.4ns，Dispatch中位翻转但累计两轮快。self8/70、clone11/70，目标控制IQR达约5～6个百分点，小收益尚不能可靠区分，故候选留存待确认而不应用。不是优化失败结论或最低收益门槛，也不盲目重采等控制通过。

两侧原契约/70入口/109形态525差分、harness/vet/编译/机器码、fresh/源码/原采样函数/二进制/raw及顺序核对完成。对象数中位相同，26字节尾差/30入口累计翻转全部保留。未采用，不重复生产全验收；原库6个删除保留，生产Go文件仍af9169d。见[完整报告](benchmarks/2026-10-20-gosproto-chunk-count/RESULTS.md)。前轮386727c已提交，新实验未提交，双主线不变。

## 70. Packing普通word局部组装（未采用）

单一候选将普通mask有效字节按原序组装至局部uint64，末尾LittleEndian一次写同一8字节区域。mask非零保证位置0..7、移位0..56，未置位为零；完整预检/分配/零段/FF路径不改，错误/预算/len-cap-nil/独占保持。函数384→396条，减少store也增加移位/OR和完整word视图检查，不能预告速度。

固定70项280进程/26,880块：大型Unpack+8.51%/+9.03%、PDecode+1.96%/+2.46%，4字节数组Unpack+6.70%/+7.58%、8字节+5.87%/+5.48%；普通word1..7有效字节两轮均慢、FF接稀疏+37.92%/+39.49%。未采用；短零段8word−3.85%/−0.87%、累计少48.7/44.3ns亦保留，不是收益比例门槛。

self47/70、clone22/70，所有对象数中位相同、25字节尾差/24入口累计翻转与全部异常保留。原契约/109形态525差分/159275解包输入预算差分、harness/vet/机器码/fresh/来源/原采样函数核对通过。未采用，不重复生产全验收。见[报告](benchmarks/2026-10-20-gosproto-unpack-word-store/RESULTS.md)。用户已有删除保持，生产Go源仍af9169d；bf1203e前轮已提交，新报告未提交，Body与Packing双主线继续。

## 71. Body非负tag计数（待确认）

仅将decodeMessage tag累加表示为uint32，原>65535实际字段检查后转int；最多65535项、每项最多32768，总计2147450880<MaxInt32/MaxUint32，不回绕。纯跳跃链结束仍允许最终tag超过65535，不强化拒绝规则。公开Fields即时读取/owner证明、预算/错误顺序/消费长度/失败原子性/所有权保持，不混入countChunks或Packing候选。

新增5形态×2预算共10组公共Decode对照：最大合法tag、附近未知tag、越界实际字段、65535最大跳跃头及最后实际字段，验证原值与指针独占。机器码负数位检查1→0，总指令仍660；静态去分支不等于速度。

固定70项280进程/26,880块，大型Decode−0.08%/−4.51%但首轮累计慢139.8ns；PDecode+1.07%/+0.30%、Dispatch+0.70%/+0.27%，累计亦两轮慢。小型Dispatch−0.83%/−0.17%、累计少16.1/38.9ns且两类控制通过，收益保留。主要Body幅度不稳定、组合有成本，候选待确认不应用；不是固定收益门槛或全面失败结论。

self48/70、clone41/70，对象分配中位相同，28字节尾差/28入口累计翻转及所有异常保留；目标契约/109形态525差分/新增10边界、harness/vet/机器码/fresh/原采样函数/来源核对通过。未采用，不重复生产全验收。见[报告](benchmarks/2026-10-20-gosproto-decode-tag-counter/RESULTS.md)。后续扩大到结构构造与字段执行成本，不只排列单条件；Body和Packing双主线保持。36f03f2已提交，新报告未提交，原库删除保持、生产Go源af9169d不变。

## 72. 当前Body构造/字段执行诊断

三种代表消息、PB/SP纯Encode/Decode，12CPU＋12全量allocation进程；CPU约2秒、allocation5000次，初始化/预热包含在memory profile，loop分配ResetTimer-only，不用带profile的ns/op更新排名。完整分母保留运行时，父子累计不相加。

大型SP Decode10416B/532次、PB13560B/537次，SP并不分配更多；SP生成数组[]int64与PB[]int32也有类型/size-class差异，不能全归因实现坏。SP CPU中reflect.New累计13.16%、Value.Set5.70%，但PB子消息数组也reflect.New（独立profile9.71%不能与SP作差计算速度），后续用pointer unmarshal/AppendPointerSlice。alloc_objects与MemStats逻辑tiny计数口径不同，不能把profile份额换算成532次中可删除对象。

Encode中整数记录预检/写出仍有明显样本，不能以已领先PB而停止；保留预算/当前值读取/元信息证明。下一Body候选仅隔离同对象构造的结构数组目标槽写入，不改parser/对象数量/生命周期；若借用底层指针槽，先证明实际元素类型/尺寸/长度、元信息变化回退和GC屏障，禁止uintptr缓存/指针memcpy。尚未实现或采用，不设收益比例门槛；Packing同步跟踪。

104完整命令/fixture测试/vet/模块/24profile与摘要、fresh/18Go源/生成依赖/夹具/二进制/raw核对完成。首次PB源码提取正则无匹配失败及后续修正记录完整保留；未按性能择优。无生产改动，不重复全验收或GoTS/新TS排名。见[报告](benchmarks/2026-10-20-gosproto-body-cost/RESULTS.md)。c4f06ee已提交，新诊断未提交，原库删除未触碰，双主线不变。

## 73. Body结构数组受控指针槽（已保留）

仅decodeStructSlice元素写入：countChunks/MakeSlice/逐对象reflect.New/通用decodeMessage与全部预算、消费检查、最终value.Set不改。在当前字段Array/Wire为结构数组且实际元素恰为PointerTo(私有子schema.typ)时，局部unsafe.Slice借用新分配的count个指针槽，用Go指针赋值写入；命名指针或形状不符仍reflect.Index/Set。每个子对象仍独立，GC写屏障实际存在，不缓存uintptr/字段/地址，不用池/arena或指针memcpy。函数208→352条、栈224→256B，证明/回退有成本。

固定70项280进程/26,880块，大型Decode−3.50%/−3.63%、Dispatch−5.80%/−3.04%，nil4096 Decode−8.77%/−8.74%、half4096−6.21%/−5.85%，长结构组合重复改善。AddressBook PDecode+2.56%/+2.07%（+45.3/36.6ns）为接受的局部代价；大型PDecode次轮中位快2.66%却累计慢994.4ns，非目标数值Dispatch成本亦保留。full4096组合首轮−0.01%仍记录，不设收益门槛。

self39/70、clone37/70，所有对象数中位相同，29字节尾差/21入口累计翻转与全部异常保留。新低GOGC/命名类型/独占/坏尾部与形状回退先经race/checkptr，永久回归补异构描述不可静默写不兼容指针，先在未改基线通过；既有契约/109形态525差分/70入口、fresh/源码/原采样函数/机器码/二进制/raw通过。7条收尾全库＋固定官方oracle/race/严格checkptr/vet、Codec/Decode有限fuzz472819/9614次、GSR三包、GoTS7/7零skip通过。Decode fuzz3秒后计数停止，有限退出原文保留，不重试。

精确被测运行时代码及新增安全测试落到独立库，运行时与完整验收快照一致；GoTS来源验证在临时replace上，正式依赖不变。原库6项用户删除不恢复/提交，该轮只改decode.go、CHANGES.md和新增decode_struct_slots_test.go，已提交库e43df35。见[报告](benchmarks/2026-10-20-gosproto-decode-struct-slots/RESULTS.md)及安全评审。库e43df35与报告GSRa8b9fa3已提交，不升级依赖/发布/PBTS新排名；双主线继续。

## 74. Body通用Encode延迟内容视图（待确认）

继[优化参考](GOSPROTO-OPTIMIZATION-REFERENCES.md)后，仅将原生整数的内容切片构造移到非inline分支；与整数记录专用路径一致，不缓存值，不改预检/布局/错误/所有权。机器码证实分支顺序改变，总指令544、栈224B、panicSlice静态数8均不变，不据此预测速度。

固定原19项（Encode/Pack/PEncode与manual帧），独立进程self/clone/pass1/pass2，共76进程7296块。大型Encode−0.36%/−1.36%，首轮累计却慢214.9ns；大型PEncode+2.23%/−0.67%，小型组合亦翻转。UTF8手工帧−2.33%/−1.32%且累计/两控制支持，AddressBook PEncode−0.06%/−2.11%亦保留。小收益不按固定门槛丢弃，主要路径净收益待确认，不应用生产。

self13/19、clone15/19通过；全部limiter为0，最大堆快照4101360B（不是峰值），17项累计反向/3项分配尾差完整保留。102条流程命令包含两侧相关编码/预算/元信息/RpcFrame契约、四模式夹具/统计/telemetry、harness vet/构建、fresh/生成源/原采样函数/二进制/逐项raw核对，全部通过。最初沙箱缓存访问失败在计时前，获沙箱权限后新目录重建，记录保留、缓存未清理，不是按结果重试。未采用，未重复生产全验收，不声称覆盖Decode、增长形态、融合Rpc或新PB/TS排名。

下一步隔离纯inline/混合/非inline原生字段的实际执行与内容区边界成本，并继续尺寸计算/写出重复工作；不盲目重采控制、不叠加另一个候选。Packing保持独立主线。见[完整报告](benchmarks/2026-10-20-gosproto-encode-inline-body/RESULTS.md)。生产库e43df35及原6项删除保持，参考文档GSR7e48d4c已提交，新实验未提交。

## 75. 通用Encode分布对照与保留

继续同一候选而非叠加优化：8/64个原生指针字段，0/内联上界/非内联32位/负数/64位/混合/半缺失/全缺失8种分布，完整Encode/PEncode共32项。计入预检与分配，不把它称为纯写出；静态具名结构，不改业务夹具。独立进程self/clone/两轮比较共128进程12288块，所有构建/测试先于计时。

8字段混合Encode−0.57%/−0.77%，累计少0.87/0.62ns；64字段半缺失PEncode−2.15%/−2.58%，累计少12.9/31.4ns，两类控制通过。64字段全0/内联上界Encode中位快约1.8%～3.0%，但控制与累计均有不一致。64字段负数PEncode+1.40%/+0.50%，全缺失PEncode+0.35%/+0.37%亦保留；后者不进入目标分支，不能把全部变化解释成操作数差异。前轮业务大型组合仍方向翻转，不外推全路径提速。

决定保留简单局部改动并接受披露代价，不按最低百分比丢弃小收益。未新增unsafe/分配/缓存，不改当前值读取、元信息证明、预检/预算或所有权。158条测量流程与来源核对、self22/32与clone23/32、全部分配中位相同，29项累计反向完整保留，limiter全0，4144336B为最大快照而非峰值。

7条顺序完整验收全部通过：库全测/官方固定oracle、race/checkptr、vet、有限fuzz472494/32060次、GSR三包与GoTS7/7零skip，实际二进制来源已核对；Decode fuzz3秒后计数停止、11秒正常退出原文保留不重试。精确候选encode.go与CHANGES已提交独立库7b05b78，原6项删除不动，正式依赖仍v0.4.0。前轮GSRffc591f已提交，新[报告](benchmarks/2026-10-20-gosproto-inline-shapes/RESULTS.md)留审查。后续回到尺寸计算与字段执行的高占比重复工作，Packing也继续，不再围绕此小分支反复排列。

## 76. 整数记录数组两阶段指针取得（已保留）

原生整数记录数组在预检与写出均逐元素反射Index/UnsafePointer。单一候选在原有child binding/nativeIntegerFields证明后，每个数组入口核对实际元素恰为私有子schema的*T，同步借用[]unsafe.Pointer读槽；命名切片可用，命名指针仍回退。每个元素指针、nil与标量当前值逐次读取，不缓存地址、不跨调用借用、不改预算/布局/错误/输出独占。既有KeepAlive保留。具体安全证明与有限平台限制随报告保存。

原19项加18增长入口，148进程14208块两轮：大型Encode−18.63%/−19.05%、PEncode−15.87%/−14.34%，累计亦两轮快；小型Encode−7.02%/−4.90%、PEncode−5.97%/−3.92%。4096完整记录Encode−16.33%/−15.35%、nil记录−37.60%/−37.86%，nested32x128约快14%。AddressBook PEncode+6.43%/+0.43%、数值Encode+1.61%/+2.01%及manual小回退保留，不把非目标全部变化归因于指针槽。无新PB/TS排名或跨轮比例相乘。

self22/37、clone24/37，比较分配中位全相同；两项clone字节尾差与31项累计反向、全部异常保留，不扣噪声或按最低收益比例筛选。最大堆快照4195232B、limiter全0。fieldSize指令956→996/栈240→256B、encodeFieldInto864→912/208→224B、helper120/96B；不消除fallback反射，不据静态条数预告速度。

178条测量流程含109形态525差分、既有编码/元信息/预算契约、harness/vet/构建/fresh/生成输入/夹具/二进制/逐项raw核对。新增切片形状、nil/改值/GC/预算/独占回归先在未改基线通过race/checkptr，再执行7条完整验收：库全测/固定官方oracle/race/checkptr/vet/有限fuzz463887/10154、GSR三包与GoTS7/7零skip通过；Decode fuzz3秒后计数停止、11秒正常退出原文保留。精确候选3个运行时文件和永久测试已提交独立库a93bb0d，旧6项删除不动，正式依赖v0.4.0不变。前轮7b05b78/2c5c653已提交，新[报告](benchmarks/2026-10-20-gosproto-record-pointers/RESULTS.md)留审查。后续重新定位Body高占比重复工作，Packing主线继续。

## 77. 数组优化后双主线成本更新

仅采已提交a93bb0d的9个CPU＋9个全量allocation进程：大型/整数数组/ASCII4KiB的Encode、Pack、PEncode。每CPU约2秒、每allocation5000次，GOMAXPROCS1/GOGC100，profile完整分母及GC/kevent均保留，不删运行时样本重算占比，不把带profile的ns/op当排名。80条流程命令与fresh/源码/生成输入/夹具/二进制/raw核对通过，无生产候选，不重复上轮完整验收。

大型Encode的encodedSize28.50%（整数记录预检21.50%）、encodeMessageInto30.00%（整数记录写出17.00%）仍值得推进；父子不相加。大型PEncode的packInto17.39%，整数数组PEncode的packInto29.00%。下一Body候选仅考虑数组入口的当前字段引用局部计划，减少order→Fields重复索引，继续当前值读取/每次调用重绑定；先测拷贝/栈/分配代价，尚未实施。

独立wire审计1条命令通过：大型417完整word中386个为3～7非零字节，整数数组449中321个同类、无FF；ASCII513中512个全非零、实际FF消费4096B。下一Packing候选隔离普通路径稀疏/密集固定目标视图是否可共享构造，先证明完整增长/重叠间隔，再测mask/长度/密度/小包；可能伤稀疏，不预设采用。FF与所有权/cap不改，ASCII另按FF扫描和分配看待。

循环分配大型Encode/PEncode3968B/2次、Pack3456B/1次；整数数组各4096B/1次、ASCII各4864B/1次。allocation profile含初始化/预热，不把物理对象计数当loop逻辑次数；PEncode已有融合缓冲，不等于两操作分配相加。无新PB/TS对照，无性能比例拼接。见[完整诊断](benchmarks/2026-10-20-gosproto-post-pointer-cost/RESULTS.md)。原库Go源仍a93bb0d及6项原删除保持，前轮GSR3f23e02已提交，新报告留审查，依赖v0.4.0与双主线保持。
