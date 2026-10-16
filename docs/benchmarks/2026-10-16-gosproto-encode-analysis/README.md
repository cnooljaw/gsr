# 大型结构体编码诊断（2026-10-16）

这是内部机制诊断，正式性能排名仍见[第三轮报告](../2026-10-16-gosproto-round3/RESULTS.md)。结论、限制与源码对照见[编码重构提案](../../RFC-GOSPROTO-ENCODER-REWRITE.md)。未修改生产库，未发布或升级依赖。

- `results.json`：通用 / inline 提前返回 / 类型化 Item 三种副本，45 个样本。
- `item-results.json`：通用 / 固定流程反射 Item / 类型化 Item 三种副本，45 个样本；主文的机制对照只取这一组，不混合两组中位数。
- `profile-summary.json`：固定流程反射与类型化 Item 的 CPU top；focus 为 gosproto 调用栈，采用 relative_percentages，计时与 profile 分开。
- `reproduce.py`：从指定独立库的 `a579920` 归档构造四份隔离源码并重跑两组诊断。已检查脚本生成的四份源码及夹具与本次计时输入逐字节一致；完整二进制、日志和 pprof 留在忽略的 tmp 中。

```sh
python3 docs/benchmarks/2026-10-16-gosproto-encode-analysis/reproduce.py \
  --library /absolute/gosproto \
  --output tmp/gosproto-encode-reproduce
```

输出目录必须不存在；需要能运行 Go 1.24 和 gofmt，使用本机正常构建缓存。默认只跑固定 wire 探针与专项计时，不运行完整库测试。可加 `--prepare-only` 仅生成源码供审查；不会修改原独立库、module cache 或正式 go.mod。该脚本不自动采 profile；原始 profile 的命令为对应二进制加 `-test.run='^$' -test.bench='^BenchmarkDiagnosticEncode$' -test.benchtime=1s -test.cpuprofile=/absolute/profile.pprof`。

固定消息为现有 representative.json 的 game_large；探针 Go 类型与字段布局和生成物对应，但不是正式生成类型。每份副本先验证 body 为 3336 B、SHA-256 为 `dbf8228801e1c9bd5303836df2dfc8640f85cf9272d56564be228665d63e9559`。只证明该输入的 wire 一致，不代表类型化探针已经覆盖完整协议或预算错误路径。Size/Write 是复用输入/输出的微基准，不替代完整 Encode。
