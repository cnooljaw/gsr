import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { cpus, arch, platform, release, loadavg } from "node:os";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { codecEnvironment } from "./benchmark-environment.mjs";

const example = fileURLToPath(new URL("../", import.meta.url));
const root = resolve(example, "../..");
const output = resolve(process.argv[2] ?? resolve(root, "tmp/protocol-representative"));
mkdirSync(output, { recursive: true });
const command = (program, args, options = {}) => execFileSync(program, args, {
  cwd: root, encoding: "utf8", maxBuffer: 16 << 20, ...options,
});
const goArgs = ["test", "./examples/websocket-gate/benchmarks", "-run", "^TestRepresentativeFixtures$",
  "-bench", "^BenchmarkRepresentativeCodec$", "-benchmem", "-benchtime=200ms", "-count=5", "-timeout=2m"];
const metadata = {
  date: new Date().toISOString(), commit: command("git", ["rev-parse", "HEAD"]).trim(),
  workingTree: command("git", ["status", "--short"]), go: command("go", ["version"]).trim(),
  node: process.version, cpu: cpus()[0]?.model, cpuCount: cpus().length,
  os: `${platform()} ${release()} ${arch()}`, loadBefore: loadavg(),
  codecs: codecEnvironment(root),
  commands: { go: ["GOMAXPROCS=1", "go", ...goArgs], typescript: [process.execPath, "client/dist/representative_benchmark.js"] },
};
console.error("运行代表性消息 Go 基准（40 项、5 轮）…");
const goRaw = command("go", goArgs, { env: { ...process.env, GOMAXPROCS: "1" }, timeout: 180000 });
writeFileSync(resolve(output, "go.txt"), goRaw);
console.error("运行代表性消息 TS 基准（40 项、5 轮）…");
const tsRaw = command(process.execPath, ["client/dist/representative_benchmark.js"], { cwd: example, timeout: 120000 });
writeFileSync(resolve(output, "typescript.json"), tsRaw);
metadata.loadAfter = loadavg();
writeFileSync(resolve(output, "environment.json"), JSON.stringify(metadata, null, 2) + "\n");
const ts = JSON.parse(tsRaw);
const median = numbers => [...numbers].sort((a, b) => a - b)[Math.floor(numbers.length / 2)];
const grouped = new Map();
for (const line of goRaw.split("\n")) {
  const match = line.match(/^BenchmarkRepresentativeCodec\/(\S+?)\s+\d+\s+([\d.]+) ns\/op\s+(\d+) B\/op\s+(\d+) allocs\/op$/);
  if (!match) continue;
  const name = match[1].replace(/-\d+$/, "");
  if (!grouped.has(name)) grouped.set(name, []);
  grouped.get(name).push({ ns: Number(match[2]), bytes: Number(match[3]), allocs: Number(match[4]) });
}
const go = new Map([...grouped].map(([name, samples]) => {
  if (samples.length !== 5) throw new Error(`missing rounds: ${name}`);
  return [name, { nsPerOp: median(samples.map(sample => sample.ns)),
    bytes: median(samples.map(sample => sample.bytes)), allocs: median(samples.map(sample => sample.allocs)),
    samples: samples.map(sample => sample.ns) }];
}));
const typescript = new Map(ts.results.map(result => [result.name, result]));
const cases = JSON.parse(readFileSync(resolve(example, "benchmarks/representative.json"), "utf8"));
if (go.size !== cases.length * 8 || typescript.size !== go.size) throw new Error("incomplete benchmark output");
const steps = ["protobuf_encode", "protobuf_decode", "sproto_encode", "sproto_decode",
  "sproto_pack", "sproto_unpack", "sproto_pencode", "sproto_pdecode"];
const lines = ["# AddressBook 与游戏消息编解码基准", "",
  `时间：${metadata.date}；CPU：${metadata.cpu}；${metadata.os}。`, "",
  `${metadata.go}；Node ${metadata.node}；V8 ${ts.v8}；基础提交 ${metadata.commit}，包含随附工作树修改。`, "",
  `Go：google.golang.org/protobuf v1.36.1、cnooljaw/gosproto ${metadata.codecs.module.Version}${metadata.codecs.module.Replace ? " 基础的本地工作树（未发布优化版）" : ""}；TS：@bufbuild/protobuf 2.15.0、示例内修正的 sealindx/sproto-ts。实际源码哈希和依赖覆盖见 environment.json。`, "",
  "## 口径", "",
  "共用 representative.json 数据；AddressBook 对应官方 Alice/Bob/三条电话，Protobuf 使用官方 proto2 required/optional 定义。Sproto PhoneNumber 提至顶层以适应 TS parser，字段布局不变。游戏消息用整数状态位、正负坐标、数值数组、物品结构体数组和字符串数组；int32_edges 为 896 个正负 int32 边界值。两侧 Protobuf 数值数组显式 packed，负值使用 sint32。", "",
  "纯 payload 编解码与 Pack/Unpack 分开测；PEncode/PDecode 是组合路径。不含 Envelope/package、网络、MAC、Dispatcher、RPC、schema 构造和 fixture 校验。输入对象复用，编码输出分配计时；每次解码创建新对象。Go 的 Sproto 资源预算与失败原子性保留。Pack 只处理预先编码的 Sproto body，Unpack 只处理预先 packed 的 body，不与 Protobuf 强造对应操作。", "",
  "两端在计时前检查 raw/packed 字节数、SHA-256、完整语义往返和 padding。Sproto 固定签名已与官方 C/Lua 编码器对照；AddressBook 复现 130/83 字节，Protobuf 为 69 字节。", "",
  "Go 固定 GOMAXPROCS=1、200ms/样本、5 轮；TS 每操作至少约 50ms 热身、校准约 100ms/样本、5 轮并反转顺序。以下均为中位数。CPU/分配 profile 另起进程采集，不用于表格计时。", "",
  `采样前 load average：${metadata.loadBefore.map(value => value.toFixed(2)).join(" / ")}；采样后：${metadata.loadAfter.map(value => value.toFixed(2)).join(" / ")}；逻辑 CPU ${metadata.cpuCount}。这是开发机基线。`, "",
  "## 编码大小（字节）", "", "| 消息 | PB | Sproto raw | Sproto packed |", "| --- | ---: | ---: | ---: |"];
for (const sample of cases) lines.push(`| ${sample.name} | ${sample.wire.protobuf_encode.bytes} | ${sample.wire.sproto_encode.bytes} | ${sample.wire.sproto_pack.bytes} |`);
for (const [language, results] of [["Go", go], ["TypeScript", typescript]]) {
  lines.push("", `## ${language} 耗时（µs/op）`, "",
    "| 消息 | PB Encode | PB Decode | SP Encode | SP Decode | Pack | Unpack | PEncode | PDecode |",
    "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |");
  for (const sample of cases) lines.push(`| ${sample.name} | ${steps.map(step => (results.get(`${sample.name}/${step}`).nsPerOp / 1000).toFixed(3)).join(" | ")} |`);
}
lines.push("", "## Go 分配（B/op / allocs/op）", "",
  "| 消息 | PB Encode | PB Decode | SP Encode | SP Decode | Pack | Unpack | PEncode | PDecode |",
  "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |");
for (const sample of cases) lines.push(`| ${sample.name} | ${steps.map(step => {
  const result = go.get(`${sample.name}/${step}`); return `${result.bytes} / ${result.allocs}`;
}).join(" | ")} |`);
lines.push("", "## 波动提示", "", "下列项目五轮最大/最小超过 2；阈值不是统计显著性检验。", "",
  "| 语言 | 项目 | min µs | median µs | max µs |", "| --- | --- | ---: | ---: | ---: |");
for (const [language, results] of [["Go", go], ["TypeScript", typescript]]) {
  for (const [name, result] of results) {
    const min = Math.min(...result.samples), max = Math.max(...result.samples);
    if (max / min > 2) lines.push(`| ${language} | ${name} | ${(min / 1000).toFixed(3)} | ${(result.nsPerOp / 1000).toFixed(3)} | ${(max / 1000).toFixed(3)} |`);
  }
}
lines.push("", "## 单独能力探针", "", "以下是额外正确性探针，不进入性能排名；失败不被解释成更快或更小。", "");
for (const probe of ts.capabilityProbes) lines.push(`- ${probe.name}：${probe.passed ? "通过" : "失败，" + probe.error.split("\n")[0]}`);
lines.push("", "## 复现", "", "```sh", "npm --prefix examples/websocket-gate run bench:representative",
  "npm --prefix examples/websocket-gate run profile:protocol", "```", "",
  "原始数据：[Go](go.txt)、[TS](typescript.json)、[环境](environment.json)。默认新结果位于 tmp/protocol-representative/；完整 profile 位于 tmp/protocol-profile/。profile 说明与优化方案见同目录分析文档。", "",
  "本轮没有重跑官方历史 C/Lua 对 pbc-lua 的速度排名，不宣称复现它的性能倍数，也没有测 BigInt、完整 int64、浮点和浏览器。", "");
writeFileSync(resolve(output, "report.md"), lines.join("\n"));
console.log(`代表性消息 Benchmark 完成：${resolve(output, "report.md")}`);
