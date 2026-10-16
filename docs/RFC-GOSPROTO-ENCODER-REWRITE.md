# gosproto 大型结构体编码：诊断与重构提案

状态：2026-10-16，用户已同意按本提案实施。原生 `*int64` 受控只读字段访问已本地提交：独立库 `819047e`、GSR `bf3ed8c`；随后嵌套类型校验及整数记录数组循环已提交为独立库 `5e491ff`、GSR `70c84af`，均未推送/发布。大型 Encode 约 6.54 µs，同场 PB 7.59 µs。该专项 Decode、Pack/Unpack 与 TS 保持冻结，不引入生成器，正式依赖仍为 v0.4.0。实现和验证见下文第 7、8 节；后续 packing 切片另见 [结果](benchmarks/2026-10-16-gosproto-pack/RESULTS.md)。

## 1. 当前问题已经可以拆开

[第三轮正式采样](benchmarks/2026-10-16-gosproto-round3/RESULTS.md) 中，Go SP Encode 14.498 µs，PB 7.558 µs，约为 PB 的 1.92 倍；SP 3968 B/op、2 allocs/op。整数数组纯编解码胜出，说明数值数组路径已有效，但不能据此认定所有 Sproto body 算法都已最优。

这个大型夹具不是一块大数组，而是一个根对象与 128 个 `Item`：

| 当前工作量 | 每次 Encode |
| --- | ---: |
| 结构体实例 | 129，预检和写出各递归一次 |
| 参与协议的字段 | 396，`record.Field` 合计 792 次 |
| 整数标量 | 392，预检和写出合计读取 784 次 |
| 可 inline 的整数标量 | 337，占标量 86.0% |
| 非 inline 的整数标量 | 55，均可用 4 字节表示 |
| 原生整数数组 | 256 + 64 项，已经使用类型化批量路径 |
| 当前调用的布局记录 | 131 项：129 个结构头数、2 个数组宽度 |

`fieldSize` 即使判断出 inline，也先调用了 `integerWidth`。写出阶段则再次 Field / IsNil / Elem / Int，再判 inline。当前双游标已经消除了第三遍头/body 分别遍历，不能再把“合并头与 body 遍历”当成未实施的新优化。

wire 成本也有区别：SP body 3336 B，PB 2081 B，SP 大 60.3%；SP packing 后为 2280 B。SP body 中结构头共 1050 B、Item 数组元素长度前缀 512 B，都是实际格式成本。类型访问只是其中一个问题，不能把所有差距归于反射，也不能拿 packed 长度解释纯 Encode。

## 2. 官方 Sproto / Skynet 实际怎么做

本次同时阅读固定官方 oracle 源码及只读 `../skynet/lualib-src/sproto/{sproto.c,lsproto.c}`。官方验收来源固定为 cloudwu/sproto `e93fd5a0653b2bcab38d3b830958bf2e3a901b5f`，本机 oracle 的 sproto.c SHA-256 为 `c47a13a15ae15d35550ed584b9102644374032ff13f1170bffadf81a2604f7bc`；Skynet 副本并非逐字节相同，但以下编码主流程一致。

- [`sproto_encode`](https://github.com/cloudwu/sproto/blob/e93fd5a0653b2bcab38d3b830958bf2e3a901b5f/sproto.c#L944)：遍历预解析字段，先预留 schema 最大头区，callback 取到数值后立即判 inline 或写 body，最后按实际头长 memmove 压紧。
- [`encode_integer_array`](https://github.com/cloudwu/sproto/blob/e93fd5a0653b2bcab38d3b830958bf2e3a901b5f/sproto.c#L768)：先按 4 字节写，遇到宽整数再扩展此前元素；它也有搬移和条件成本，并非每处算法都天然优于当前 Go 实现。
- [`encode_one / lencode`](https://github.com/cloudwu/sproto/blob/e93fd5a0653b2bcab38d3b830958bf2e3a901b5f/lsproto.c#L283)：C callback 从 Lua table/栈取值，递归写进当前缓冲；容量不足时扩容并重新编码，成功后 `lua_pushlstring` 创建结果。

它依赖 C 指针、连续内存、Lua/C 栈和闭包持有的 scratch。可借鉴的是：稳定字段事实提前解析、标量取值后立即完成判定、减少通用解释层。不能直接把“单遍”理解为一次分配、零复制或无需预算；在 Go 中照搬最大头区、重试和保留缓冲，会引入另一套内存及并发所有权问题。

当前库 README 已承诺编码先做有界尺寸预检，再分配确切长度的独占输出。直接换成边扩容边验证，虽然也能保持最终 wire 和返回 nil 的失败表现，却改变拒绝前的资源使用约束。暂不推荐把这种改变混入下一轮性能优化。

## 3. Protobuf 值得学习的是执行模型

固定本次实际使用的 protobuf-go v1.36.1，已读本机 module cache 对应源码：

| 工程机制 | 源码 | 对本轮的意义 |
| --- | --- | --- |
| 初始化时绑定字段位置与具体操作 | [makeCoderMethods](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.1/internal/impl/codec_message.go)、[fieldCoder](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.1/internal/impl/codec_tables.go) | 热循环不用重新解释完整的 Wire、指针、signedness 组合 |
| 先 Size 再 Marshal，并在本次 Marshal 使用尺寸缓存 | [proto/encode.go](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.1/proto/encode.go)、[impl/encode.go](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.1/internal/impl/encode.go) | 两遍本身不是不能快，关键是每遍的访问和分派成本 |
| offset 加类型化指针 | [pointer_unsafe.go](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.1/internal/impl/pointer_unsafe.go)、[sizeInt64Ptr / appendInt64Ptr](https://github.com/protocolbuffers/protobuf-go/blob/v1.36.1/internal/impl/codec_gen.go#L1917) | 常见标量直接读 Go 值，避免 Field / Elem / Int 链 |

这里的 PB 是“生成消息类型与描述 + 初始化 runtime codec”，并非每个业务消息都有生成的 Marshal 方法。也不能把其消息尺寸缓存照抄到普通 struct：gosproto 不得按对象地址跨调用复用上次的值或布局。

## 4. 隔离探针：先区分分派与字段访问

只在忽略的 tmp 中，从 `a579920` 归档创建四份副本；正式 gosproto 和 GSR 生产源码均未改动。复用同一 JSON 与固定 SP body 签名，Go 1.24.13 darwin/arm64、GOMAXPROCS=1，五轮、每项固定 60000 次，交替版本顺序。计时与 profile 分开。

主要对照如下，单位 µs/op；三个版本 Encode 均为 3968 B/op、2 allocs/op：

| 探针 | 完整 Encode | 仅预检 | 仅写出 |
| --- | ---: | ---: | ---: |
| 通用基线 | 14.631 | 6.525 | 7.350 |
| 固定 Item 流程，仍用 safe reflect | 10.993 | 4.962 | 5.655 |
| 固定 Item 流程，改为类型化读取 | 7.188 | 2.827 | 3.094 |

两种 Item 探针均固定三字段 schema，去掉 Wire/Array/pointer/unsigned 等通用分派和可由该 schema 排除的分支；仍执行当前值的 presence、字段/深度/字节预算和两遍布局/写出。类型化探针每个 Item 每遍仍经过一次 `Interface().(*diagItem)`，根结构及其数组路径仍使用原实现。

这不是“仅替换一个反射 API”的对照，也不是可发布的通用 codec。它只验证固定模型、固定夹具的可行空间，没有验证全部类型、schema 演进和错误路径。预检与写出微基准分别复用了输入/输出，不能相加当完整 Encode，也不能拿本探针 7.188 与上一轮 PB 7.558 宣称胜出：本次没有同进程重测 PB，且诊断类型与入口不同。

但差距很清楚：在同一套两遍算法、相同输出和分配下，缩短固定字段流程约快 24.9%；再换类型化访问，较反射探针约快 34.6%。反射探针 profile 的 Elem / Field / IsNil / Index / Int **flat** 样本合计约 47.6%（仅统计经过 gosproto 调用栈的样本）；它们已成为明显成本。这是用户提出的 11–12 µs 复盘点的具体证据，不能再只根据“减少一次调用”推测收益。

另一个独立系列只将 inline 提前返回，完整 Encode 中位数 15.125 → 14.939 µs，约 -1.2%；但独立预检反而 6.494 → 7.100 µs。全部样本保留，不据此认定已有稳定加速，更不把它当成解决约两倍差距的主方案。

[诊断原始值、profile 摘要与复现](benchmarks/2026-10-16-gosproto-encode-analysis/README.md) 单独保存，不改写第三轮正式基线。

## 5. 推荐：重构编码执行层，不再无限堆反射技巧

建议把下一轮视为一次有边界的 Encoder 内部重构，保留已正确的 wire、预算和数值数组，而不是重写整个库或引入通用协议框架。

### 第一切片：把稳定字段事实编译为内部编码操作

- 在现有 schema 构造中确定编码操作，例如 int64 指针、其他整数、字符串、整数数组、子结构；热循环只执行已确定的操作，不再经过多层 Wire 字符串和类型组合判断。
- 将 scalar 取值、inline、尺寸/写出规则放在同一种字段操作中；预检对 inline 立即返回，不计算无用宽度。先保留安全反射访问，作为明确对照。
- 继续使用现有两遍预检、精确输出、双游标和调用内布局，不新增每字段 reflect.Value tape，不复制业务对象，不扩大布局来缓存所有标量。
- 不硬编码 GameState / Item，不为该 Benchmark 单独注册快速方法。探针说明了价值，生产实现必须由实际 Go 类型/schema 驱动。
- 检查导出的 SprotoType / Fields 及现有调用者，不能让新缓存静默使用旧字段事实。若要收紧元信息可变性，需要另行明确契约；本轮不得顺便改动。

这一步是验证可泛化的分派简化能省多少，不承诺通用实现能达到专门探针的 10.993 µs。函数指针与短枚举 switch 选一种经真实路径证明较好的实现，避免同时保留多套分派框架。

### 复盘点：11–12 µs 不再追加同类微优化

若通用实现只到 11–12 µs，且 profile 仍主要是 Field / Elem / IsNil / Int，则停止继续堆 safe-reflect 分支。若出现更明确的算法或重复数据扫描热点，再按新证据处理，数字本身不是停止正确性修复的理由。

本次固定 Item 探针已说明：类型化访问值得进入架构选择，没必要等更多轮无方向试错。不能把“数组能赢”当作全部算法无问题的证明；更准确的结论是“同一大型结构夹具、同一两遍 body 算法，字段执行模型足以产生显著差距”。

### 后续选择：优先评估受控 unsafe，生成器作为备选

| 路线 | 获益与代价 | 建议 |
| --- | --- | --- |
| 内部 offset + 类型化字段操作 | 普通带 tag struct 和现有生成物继续可用，调用方 API 不变；库承担指针、GC、类型布局与工具链验证责任 | 若允许 unsafe，优先以 Encode 只读字段访问做最小原型 |
| 生成 typed codec | Go 字段访问可由编译器检查，不需要 unsafe；需要维护生成链、生成物与运行库版本，并确定跨 package 调用预算/写出能力的公共契约 | 对禁止 unsafe 或明确接受生成流程的使用方有价值，不能只加一个隐藏 hook 就视作完成 |
| 官方式单遍可增长 writer | 可能减少取值次数；但需重新讨论拒绝前分配、头区空洞、数组扩宽搬移、scratch 保留与并发所有权 | 当前证据不足以优先承担这些变化 |

推荐先改通用执行层，再用受控 unsafe 原型验证访问成本；它比要求所有使用方引入新 generator 更贴合当前“普通 Go struct 可直接编码”的定位。unsafe 只封装地址获取、offset 和类型化读取，不扩散到 wire、预算、schema 解析或 Decode；不读取 reflect.Value 内部布局，不复制 PB 的 interface header 技巧，不保存跨 safepoint 的 uintptr。输入根对象在调用期间保持存活，初期只覆盖已确认的原生字段形态，命名类型等保留安全路径并验证语义一致。

上述是本提案提出时的备选取舍。用户随后同意按推荐路线实施，当前选择只在原生 `*int64` 读取处封装 unsafe，不扩大到其他字段或 Decode。若未来选择坚持纯安全 Go，则按生成器路线单独设计；先检查固定 sprotodump 的输出能力，再决定扩展模板或增加 Go 类型生成步骤，不立即新建 schema parser。两条路线都不改变 Sproto wire，也不能要求业务手工维护 codec 注册和预算协调。

## 6. 实施验收

下一次生产切片先只改独立库 Encoder；Decode、Pack/Unpack 和 TS 作为冻结控制。保持已有公开 API、错误分类、预算计数与拒绝边界、nil/空/presence、命名类型、整数范围、double 位模式和输出独占。保留 schema 初始化失败回滚与并发初始化语义。

优先扩展现有用例：字段 presence 变化、同一对象再次编码、tag 间隔、命名标量/数组、嵌套 nil、4/8 字节边界、自定义预算。逐步完成相关回归，再执行独立库完整测试、官方 C/Lua 对照、vet、有限 fuzz 与 GSR 临时依赖接线；不在 GSR 重建底层测试。unsafe 若被选定，还须按实际访问机制补充 race/checkptr 和可运行的平台验证，不能把仅交叉编译称为运行通过。

性能先固定同一代表性夹具，针对 game_large Encode/分配做前后配对，同时保留 AddressBook、小型游戏、整数数组和 Echo 回退门禁；PEncode 只观察总效果，不改 Pack。稳定后重新同次比较 PB，不沿用 7.558 µs 当最终胜负线。只有真实完整路径受益、回归通过且内存代价合理，才替换现有执行层。

## 7. 实施切片

第一版缓存标量操作码后通过独立通用函数分派，大型 Encode 约回退 8%；合并进既有流程后仍基本持平，并出现其他消息回退。未达到固定 Item 探针的收益，说明把常量信息放进另一层解释器并不能获得相同效果；两份安全分派候选均撤回，不保留无收益的操作码框架。

最终只为经实际 Go 类型验证的原生 `*int64` 字段绑定 owner、offset 与字段形状。预检和写出各读取一次当前值，直接完成 presence、inline 和宽度/写出判断，跳过逐字段 Field / Elem / Int 与通用 Wire 分派。命名类型及其他字段完全保留原路径。普通 struct 与现有生成物继续直接可用，没有新增公开 API。

unsafe 仅存在于独立的 `encode_pointer.go`：通过反射公开 API 获取对象指针，再按实际 StructField.Offset 读取 **int64；不解析 interface header 或 reflect.Value 内存布局。消息地址不转换成整数保存，预检和写出结束显式 KeepAlive 输入。字段描述被替换为另一结构体的字段时检查 owner 并回落；Wire/Array 变化同样不使用旧的 native 绑定。布局记录、预算累计、完整预检、精确输出和失败返回仍共用原流程，不用放宽检查换速度。

测试增量为：原生字段位于未编码填充后、嵌套/空指针及 inline/4/8 字节边界，与命名类型逐字节对照；公开 Fields 跨结构体替换，以及 Wire/Array 形状改变仍读取当前描述。既有 Codec fuzz 加入根/子结构的原生整数指针，避免只覆盖数组与 double。其余已有预算、ownership 和官方向量用例复用，不在 GSR 重复搭建底层测试。

验收数据、失败候选、profile 与平台范围记录在 [首个实施切片](benchmarks/2026-10-16-gosproto-encoder/RESULTS.md)，已本地提交为独立库 `819047e`、GSR `bf3ed8c`。正式依赖与发布仍独立处理。

## 8. 嵌套类型事实与数组入口复用

后续发现第 7 节的字段 owner 检查不足以覆盖父描述替换：子 schema 可能自洽，但实际 Go 子值具有不同布局。新增回归在 `819047e` 失败、在原安全反射基线通过。当前调用显式携带实际类型事实，普通路径通过私有构造关系继承；描述替换或形状改变时核对实际反射类型，不匹配则回落，不能仅依赖子 schema 自身的 owner。

同构原生整数结构数组在入口核对实际类型及全部当前字段，再以专用循环处理元素；不再对每个元素的每个字段重复检查 owner/Wire/Array。presence、预算、深度、tag 间隔和字节规则仍逐元素执行，输入与输出契约不变。通用循环增加同构分支的候选因 AddressBook 回退撤回，未扩大 unsafe 的类型覆盖。

大型 Encode 相对上一提交同进程 8.382 → 6.528 µs，同场 PB 7.606 µs；相对最小类型修复版 9.730 → 6.544 µs，同场 PB 7.588 µs。AddressBook 相对上一提交 +12.0%，相对最小修复版 +1.3%，修复成本单独公开，不称为无回退。新预算/tag 回归、结构数组 fuzz、完整库 race/checkptr/官方对照与 GSR 接线通过。完整结果及可复现修复对照见 [嵌套绑定与整数记录数组](benchmarks/2026-10-16-gosproto-bindings/RESULTS.md)。该切片已本地提交为独立库 `5e491ff`、GSR `70c84af`，正式依赖仍为 v0.4.0。
