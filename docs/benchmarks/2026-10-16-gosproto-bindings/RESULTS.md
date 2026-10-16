# gosproto 嵌套类型校验与整数记录数组编码

2026-10-16，上一轮已本地提交为 gosproto `819047e`、GSR `bf3ed8c`，未推送/发布。本轮继续大型 Encode，先修正嵌套字段布局校验，再缩短同构整数结构数组的循环。该切片随后已本地提交为 gosproto `5e491ff`、GSR `70c84af`，GSR 正式依赖仍为 v0.4.0；验证使用临时 modfile。下一轮见 [Pack 结果](../2026-10-16-gosproto-pack/RESULTS.md)。

## 正确性修复

上一轮只检查 `field.encOwner == schema`，不足以保证嵌套值的实际 Go 类型匹配该 schema。调用方把另一种父类型的公开 Fields 描述替换进来后，子 schema 内的 owner 检查仍可能成立，但实际子对象的布局不同。回归用两个字段索引相同、整数指针 offset 分别为 32 与 8 的子结构复现：`819047e` 的单子结构及数组均输出错误字节；原安全反射版本 `a579920` 通过。

现实现让根类型事实沿私有构造关系传递。只有父类型、字段 owner 与数组/指针形状均匹配，才继承子类型事实；否则检查实际反射类型，不匹配就沿原字段索引使用反射。子 schema 自身的 Fields 被替换时，原生字段检查同样回落。验证结果仅在当前调用/数组遍历内传递，不缓存消息值、地址或跨调用绑定，不收紧公开描述在调用间变更的原有行为。

为区分修复与加速，另保留 [最小修复补丁](type-check-only.patch)：在每个结构入口检查实际指针类型。这是计时对照，最终实现没有保留该补丁中的额外 `ptrType` 字段。

## 最终实现

只为全部协议字段均为原生 `*int64` 的结构数组增加专用预检/写出循环，由实际 Go 类型和 schema 驱动，不识别业务类型名。数组入口逐字段核对当前 owner、Wire 与 Array，再逐元素读取当前值、处理 presence、inline/4/8 字节和 tag 间隔。

预算仍逐元素/字段计数，深度和字节边界不放宽；完整预检后才分配独占输出。nil、空记录、稀疏 tag 及失败返回 nil 的行为保持一致。unsafe 仍只读取已验证的原生整数指针，未扩展到数组底层存储、其他标量或 Decode。其他结构沿通用路径，输入和描述在编码期间不得并发修改。

曾尝试在所有结构的通用循环中增加同构分支，大型消息虽快，AddressBook 却明显回退，已撤回。最终专用循环限定在结构数组，避免每个普通字段承担这层选择。

## 同进程配对结果

Apple M2，Go 1.24.13 darwin/arm64，GOMAXPROCS=1，PB v1.36.1。固定原 JSON、Go 类型、公共 Encode/EncodePacked 入口和 wire 签名。新旧库与 PB 同进程五轮，每项固定 60000 次，各轮交替顺序。计时与 CPU profile 分开；表中为各组五轮中位数，单位 µs/op。

相对上一提交 `819047e`，见 [records.json](records.json)：

| 消息 | SP Encode 前 | SP Encode 后 | 耗时变化 | 同次 PB Encode | SP PEncode 前→后 |
| --- | ---: | ---: | ---: | ---: | ---: |
| AddressBook | 0.4790 | 0.5366 | +12.0% | 0.3319 | 0.6524 → 0.6788 |
| 小型游戏 | 0.8437 | 0.7374 | -12.6% | 0.6725 | 1.208 → 1.100 |
| 大型游戏 | 8.382 | 6.528 | -22.1% | 7.606 | 11.869 → 10.003 |
| 896 项整数数组 | 2.186 | 2.197 | +0.5% | 4.906 | 5.934 → 5.901 |
| 4 KiB ASCII body | 0.5995 | 0.5950 | -0.8% | 0.5619 | 2.652 → 2.659 |

AddressBook 的 +12.0% 不能隐藏。为看清必要校验与新循环的成本，再与“819047e + 最小类型修复”配对，见 [fixed-before.json](fixed-before.json)：

| 消息 | 最小修复 Encode | 当前 Encode | 耗时变化 | 同次 PB Encode | 当前相对 PB |
| --- | ---: | ---: | ---: | ---: | ---: |
| AddressBook | 0.5256 | 0.5326 | +1.3% | 0.3285 | +62.1% |
| 小型游戏 | 0.9314 | 0.7392 | -20.6% | 0.6721 | +10.0% |
| 大型游戏 | 9.730 | 6.544 | -32.7% | 7.588 | -13.8% |
| 896 项整数数组 | 2.191 | 2.189 | -0.1% | 4.956 | -55.8% |
| 4 KiB ASCII body | 0.5837 | 0.5774 | -1.1% | 0.5562 | +3.8% |

第二组解释了 AddressBook 回退主要随完整类型校验出现，不能把修复成本记为零，也不能用第二组替换第一组的历史提交对照。两组大型 Encode 分别比同场 PB 少 14.2% / 13.8% 耗时；这是固定大型夹具的 body 结果，不代表全部 Go 消息胜出。

代表性消息与四档 Echo 的 B/op、allocs/op 前后均不变。大型 Encode 仍为 3968 B / 2 次，PEncode 为 7424 B / 3 次。PEncode 约 10 µs，仍高于同场 PB Encode；本轮未改 Pack。32 B ASCII Echo body 相对上一提交 86.52 → 91.68 ns（+6.0%），完整帧 290.5 → 294.5 ns（+1.4%）；与最小修复版配对则为 87.96 → 88.98 ns、305.7 → 294.0 ns。保留这些数纳秒代价和采样波动，不称为全项无回退。

最终两组各 49 项 × 五轮，共 490 个样本。另保留最小修复、通用同构分支、未内联绑定以及整理后通用分支四个候选的 380 个样本，见 [experiments.json](experiments.json)。其中名为 `final` 的候选因 AddressBook 回退被撤回，实际保留版本为 `records` / `fixed-before`；文件名不表示验收结论。没有删除失败轮次，也没有重测未变化的 Decode、Pack 或 TS 全矩阵。

## Profile 与后续方向

[CPU 摘要](profile-summary.json) 对最小修复版和当前版分别采 game_large Encode。旧样本中逐字段 nativeInteger 判断约占 10% flat；新样本热点转到整数记录预检/写出、整数宽度与真实字节写入，支持“数组入口验证后复用”的优化方向。profile 只作为热点证据，累计百分比不可相加，也不转换成单项微秒收益。

本轮大型 body 已低于同场 PB，继续堆字段访问技巧的收益下降。下一次性能切片应重新 profile 数值/大型消息的 Pack 与组合路径；AddressBook、命名类型和小包作为回退检查。当前不扩大 unsafe、引入 generator 或改为可增长的单遍 writer。

## 验证与覆盖增量

- 新增嵌套描述替换回归，覆盖单子结构、结构数组，以及子 schema 的 Fields 被替换三种情况；前两种在 `819047e` 确认失败、在安全反射基线通过。
- 新增同构整数记录数组与命名类型的逐字节对照，覆盖 nil/空元素、负数/零/MaxInt64、稀疏 tag、恰好满足及少一单位的字节/深度/元素预算、缺失桥接字段造成过大 tag 间隔。新循环有独立计数代码，需要这项增量，而非重复一般数组测试。
- 现有 Codec fuzz 增加原生整数记录数组、nil 与空元素。FuzzCodecRoundTrip 约 10 秒、589498 次，FuzzDecode 约 11 秒、17768 次，通过；有限 fuzz 不代表穷尽。
- 独立库启用固定官方 C/Lua oracle 的完整 race + 严格 checkptr、vet 通过。GSR 临时 modfile 下 Sproto、Echo debugserver、Benchmark 三包测试通过；TypeScript 构建及 Go/TS Plain/MAC 集成 7/7、零跳过。
- 运行验证平台仅 darwin/arm64，本轮不重复上一轮交叉编译；不声称已在 Linux/32 位运行。未运行 GSR 全仓测试、全仓 race 或新 TS 性能采样。

命令与范围见 [validation.json](validation.json)。[inputs.json](inputs.json) 记录当前源码、冻结源码、夹具与临时覆盖。两组最终生产源码相同；`records` 采样后扩展了 fuzz 输入，因此测试文件指纹不同，最终 `fixed-before` 与完整验收对应当前源码。原始计时文本保存在 [raw](raw)，二进制和 pprof 留在忽略的 `tmp/gosproto-bindings/`，不提交可重建产物。

## 复现

在 GSR 根目录执行，需要本机 Go 1.24、gofmt、git 与包含 `819047e` 的独立库：

```sh
python3 docs/benchmarks/2026-10-16-gosproto-bindings/reproduce-pairs.py \
  --library /absolute/gosproto \
  --output tmp/gosproto-bindings-reproduce
```

增加 `--corrected-baseline` 则先在归档副本叠加最小类型修复，复现第二组对照；可加 `--prepare-only` 审查输入。输出目录必须不存在，临时模块与源码仅写入该目录。已核对生成的修复基线、四份 Go harness 和三份 JSON 与实测输入逐字节一致；当前库读取指定工作树，require 中的 v0.4.0 不能代表本轮未发布源码，必须核对实际 replace 和指纹。
