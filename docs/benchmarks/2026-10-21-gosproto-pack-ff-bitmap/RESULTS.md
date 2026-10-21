# Pack：提前从位图识别 FF 的初筛

基线为 gosproto `451998ea0dcf6262bdd4ca0ab6c686d0fb738fac` 的完整 Git 归档；生产工作树未修改。
候选仅把 `header == 0xff` 改成 `nonzero == 0x8080808080808080`，header 乘法移至 FF 分支之后。
意图是减少全非零块计算；不改变 FF run 扫描、稀疏写出和普通 word 排名写出。

结论：暂不进入生产。FF 密度初筛中位数约改善 4.38%，但零块和单字节块分别慢 7.88% 和 8.25%，普通密度也没有一致改善。小收益本身不是舍弃理由；这里需要解决的是收益覆盖范围与退化风险。

## 测量的实际边界

只测 4096 字节、每 word 固定 0 至 8 个非零字节；输出缓冲复用，不含分配。`GOMAXPROCS=1 GOGC=100 GOMEMLIMIT=off`，标准 Go benchmark，100ms/样本、每项 5 个样本。全部原始输出见 samples.txt；table.md 是各项独立中位数及其比值。

这不是 ABBA/BAAB 成对测量，没有 self/clone 或摆放轮换控制，基线与候选入口的位置也不同。因此数字仅用于发现可能的密度取舍，不能确认精确收益或把退化归因于分支/指令摆放；没有校正或删除任何样本。没有测真实 game_large、整数数组和 PEncode，也没有宣称实际业务加速。后续若重访，需要补真实消息和控制实验。

## 正确性

256 种 mask × 14 个长度（0、1、7、8、9、15、16、31、255、256、2047、2048、2049、4096），共 3584 组：候选与基线 wire 一致、输入不变，并额外验证源位于同一输出缓冲末端的写入方式。

现有 TestPack/TestUnpack 与新增 TestBitmapEquivalence 一起通过 race/checkptr。首次检查的附加源区域位于输出之后；随后修正为真正共享输出缓冲末端并重新运行通过。没有更改计时函数，计时结果不因测试修正而重跑。没有运行全库、官方 oracle、fuzz 或 Go/TS 集成；本轮不落地生产代码。

## 复现

先从上述提交导出完整库到隔离目录，再运行 `python3 prepare.py <隔离目录>` 和 gofmt。

```sh
go test -race -gcflags=all=-d=checkptr=2 -run '^(TestBitmapEquivalence|TestPack|TestUnpack)' -timeout=2m .
GOMAXPROCS=1 GOGC=100 GOMEMLIMIT=off go test -run '^$' -bench '^BenchmarkBitmap$' -benchtime=100ms -count=5 -timeout=1m .
```

下一轮优先减少普通 3–7 非零字节路径的工作量；本候选不替换已有 packInto。
