# gosproto 大型结构体 Encoder 实施结果

2026-10-16，从独立库 `a579920` 做增量，GSR 基准起点 `fdbc736`。用户同意后实施受控只读字段访问，随后已本地提交为 gosproto `819047e`、GSR `bf3ed8c`，未推送/发布；GSR 正式依赖仍为 v0.4.0，全部接线验收使用临时 modfile。

后续发现本切片的字段 owner 检查未覆盖“父描述替换后，子 schema 与实际 Go 布局不同”的情况。修复、失败回归及下一轮数组优化见 [后续报告](../2026-10-16-gosproto-bindings/RESULTS.md)。本报告保留该提交的原始采样，不将后续修复后的数据混入。

## 改动与取舍

最终只在 schema 初始化时为原生 `*int64` 字段绑定 owner、offset 和实际类型事实。预检/写出各读一次当前值，跳过逐字段反射与通用标量分派；signed inline/4/8 字节规则仍使用原 Sproto wire。命名类型及其他字段沿用安全反射；公开字段描述被跨类型替换或 Wire/Array 改变时回落，不能用旧 offset 读取别的 Go 布局。

unsafe 仅封装在 encode_pointer.go，通过 reflect.Value.UnsafePointer 与 unsafe.Add 读取实际 **int64；不解析 Go 接口头或反射内部结构。输入只在同步调用内借用，并显式 KeepAlive。不使用池、不缓存对象地址/上次尺寸、不复制整份消息，不新增 API 或 generator。两遍预检、预算与拒绝边界、双游标、独占输出保持不变。

先前尝试的“操作码缓存 + 通用 scalar 函数”大型 Encode 约慢 8%；合并操作后仍基本持平并出现其他项回退。两份安全分派实现均撤回，不保留无收益的解释层。三份候选各三轮、共 270 个样本及源码指纹见 [experiments.json](experiments.json)，其中 native 为最终整理前的首次只读访问候选，不能冒充最终版本。

## 最终同进程配对

Go 1.24.13 darwin/arm64，GOMAXPROCS=1，PB v1.36.1。固定原代表性 JSON、生成类型与 wire 签名。将 a579920 的归档仅在忽略目录改 module 路径以并列加载，新旧库对同一组 Go 类型执行原有 Encode/EncodePacked 入口；同进程五轮、每项固定 60000 次，每轮交替先后。计时与 profile 分开。

下表单位 µs/op，均取该组五轮中位数，不混入完整矩阵的中位数：

| 消息 | SP Encode 前 | SP Encode 后 | 耗时变化 | 同次 PB Encode | SP PEncode 前→后 |
| --- | ---: | ---: | ---: | ---: | ---: |
| AddressBook | 0.536 | 0.484 | -9.6% | 0.328 | 0.708 → 0.647 |
| 小型游戏 | 1.366 | 0.841 | -38.5% | 0.672 | 1.754 → 1.208 |
| 大型游戏 | 14.345 | 8.456 | -41.1% | 7.863 | 18.301 → 11.880 |
| 896 项整数数组 | 2.183 | 2.189 | +0.3% | 4.864 | 6.037 → 6.078 |
| 4 KiB ASCII body | 0.612 | 0.645 | +5.4% | 0.599 | 2.739 → 2.723 |

大型 Encode 约少 41.1% 耗时，仍比同次 PB 慢约 7.5%，不是全面胜出。大型 PEncode 约少 35.1%，但本轮未改 Pack；不能将组合收益称为 Pack 算法加速。数组 Encode 基本持平，packing 仍是另一项工作。

五种代表性消息以及四档 Echo 的各入口，前后 B/op 与 allocs/op 均相同。大型 Encode 仍为 3968 B / 2 次，PEncode 7424 B / 3 次。新增的是类型初始化时的元信息：64 位 struct Sizeof 中 SprotoField 136 → 160 B，SprotoType 112 → 120 B；这不是实际 heap size class 或每调用额外分配的测量。

## 小包代价与完整矩阵波动

32 B ASCII Echo 纯 body：79.34 → 85.95 ns，增加 6.61 ns（+8.3%）；完整帧 320.5 → 290.0 ns（-9.5%）。同进程代表性 4 KiB ASCII body 为 +5.4%，对应独立 Echo payload 为 +0.6%；UTF-8 body +3.7%、零字节 body +4.1%。这些损失原样保留，不能称为所有入口无回退。

保留该切片的理由是实际游戏与完整小帧受益，原生指针之外的实现未被扩展为另一套操作码解释器，且分配不增加。小型无整数 body 的固定检查成本是明确代价，后续是否进一步缩小 dispatch 成本应单独测量，不通过取消元信息 owner/形状检查抹掉它。

原两套 Go 主基准仍完整执行前后各 72 项 × 五轮，共 720 个样本：[main.json](main.json)。其大型 Encode 中位数 15.950 → 8.272 µs，PB 7.544 → 7.516 µs；SP 单轮范围分别 14.425–38.671 / 8.220–18.964 µs，存在明显时序波动，不能将中位数 -48.1% 直接当作稳定算法收益。系统快照中有其他进程负载，pmset 未记录热/性能限制；这只是干扰线索，不是因果证明。

完整矩阵还出现 AddressBook PDecode +13.4%、4 KiB ASCII frame Decode +15.3%、payload Decode +31.0%；并有 PB 控制项明显漂移。随后只针对这三项做同进程五轮固定次数复核，结果分别为 -9.5%、+1.4%、-7.2%，分配不变，未支持持续 >10% 回退。见 [ab-decode-pairs.json](ab-decode-pairs.json)。这不证明 Decode 得到新优化，decode.go 和 pack.go 与起点逐字节相同。

两类采样全部保存、分别报告。新旧同进程的编码复核共 245 个样本，解码复核 45 个；没有删除失败轮次来重算一份更好的完整矩阵。TS 生产源码冻结，未重新测量 TS 性能。

## Profile

[profile-summary.json](profile-summary.json) 保存同一 game_large Encode 的前后 CPU top，focus 为经过 gosproto 的调用栈，采用 relative_percentages。旧 Field/Elem/scalar 分派的成本明显减少，当前样本更多落在共享编码循环、nativeInteger 的 owner/形状检查、根对象 UnsafePointer 和递归布局。

这是实现模型改变有效的证据，不证明全部 body 算法已最优。累计样本有包含关系，不能相加；profile 百分比不能直接换算为 µs 或承诺下一轮收益。当前没有为压掉最后约 7.5% 而扩大 unsafe 到所有类型或 Decode。

## 验证与覆盖增量

- 独立库 `go test -race -gcflags=all=-d=checkptr=2 ./... -timeout=2m` 通过，并启用固定官方 C/Lua oracle；vet 通过。
- 新增三项回归：原生指针在未编码填充和嵌套后与命名类型保持相同 wire，含 nil 与 inline/4/8 字节边界；跨结构体替换公开 Fields 仍正确读取；Wire/Array 改变不使用旧绑定。复用已有预算、失败原子性、ownership 与官方向量测试。
- 既有 Codec fuzz 增加根/子结构原生整数指针；FuzzCodecRoundTrip 与 FuzzDecode 各约 10 秒通过。有限 fuzz 不代表穷尽。Pack 源码未改，不重复其专项 fuzz。
- 临时 modfile 下，GSR protocol/sproto、Echo debugserver、Benchmark 三包测试通过；Go/TS Plain/MAC 集成 7/7、零跳过。未重复 GSR 全仓测试或新增 TS 计时。
- Linux amd64 测试二进制交叉编译通过；Linux 386 库和示例构建通过。32 位测试二进制被旧测试的 math.MaxInt64 → int 常量赋值阻塞，a579920 基线复现相同失败，未顺手改无关测试。跨编译不代表已在 Linux/32 位执行；运行验证平台仅为本机 darwin/arm64。

验证命令、退出状态与范围见 [validation.json](validation.json)。源码、二进制、夹具指纹和实际模块覆盖见 [inputs.json](inputs.json)；未提交本地 replace。

## 复现

正式主套件入口沿用 [Benchmark 说明](../../../examples/websocket-gate/benchmarks/README.md)，分别用基线与候选临时 modfile 编译后交替运行；具体每轮参数见 main.json。独立库未发布，require 行的 v0.4.0 不能标识本轮实现，必须核对 Replace 和源码指纹。

同进程专项可从仓库根目录运行：

```sh
python3 docs/benchmarks/2026-10-16-gosproto-encoder/reproduce-pairs.py \
  --library /absolute/gosproto \
  --output tmp/gosproto-encoder-reproduce
```

需要本机 Go 1.24、gofmt 及包含 a579920 的独立库；新版本直接取指定库工作树。输出目录必须不存在，所有归档副本和生成的测试模块只落在该目录。可加 --prepare-only 审查夹具。已核对生成的四份 Go harness 与三份 JSON 和本次采样输入逐字节一致。

解码专项使用 [decode-probe.go.txt](decode-probe.go.txt)，复制到生成的 ab/paired_decode_test.go 后以 `-bench=^BenchmarkPairedDecode$ -benchtime=60000x -benchmem` 执行；原复核每轮切换 GSR_AB_REVERSE=0/1，完整命令见解码配对 JSON。原始二进制、pprof 和临时源码留在忽略的 tmp/gosproto-encoder/，最终原始计时文本保存在 [raw](raw)。

本轮达到的是大型 Encode 的显著改进，并未消除 packing 或所有消息的 PB 差距。继续优化应由新 profile 决定，不为形式统一给其他类型批量增加 unsafe 路径。
