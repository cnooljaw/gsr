import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { cpus, arch, platform, release } from "node:os";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { codecEnvironment } from "./benchmark-environment.mjs";

const example = fileURLToPath(new URL("../", import.meta.url));
const root = resolve(example, "../..");
const output = resolve(process.argv[2] ?? resolve(root, "tmp/protocol-benchmark"));
mkdirSync(output, { recursive: true });
const command = (program, args, options = {}) => execFileSync(program, args, {
  cwd: root, encoding: "utf8", maxBuffer: 16 << 20, ...options,
});
const metadata = {
  date: new Date().toISOString(),
  commit: command("git", ["rev-parse", "HEAD"]).trim(),
  workingTree: command("git", ["status", "--short"]),
  go: command("go", ["version"]).trim(),
  node: process.version, cpu: cpus()[0]?.model,
  os: `${platform()} ${release()} ${arch()}`,
  codecs: codecEnvironment(root),
  goCommand: "GOMAXPROCS=1 go test ./examples/websocket-gate/benchmarks -run ^$ -bench ^BenchmarkProtocolCodec$ -benchmem -benchtime=200ms -count=5 -timeout=2m",
};
console.error("运行 Go 编解码基准（单线程，5 轮）…");
const goRaw = command("go", [
  "test", "./examples/websocket-gate/benchmarks", "-run", "^$",
  "-bench", "^BenchmarkProtocolCodec$", "-benchmem", "-benchtime=200ms", "-count=5", "-timeout=2m",
], { env: { ...process.env, GOMAXPROCS: "1" }, timeout: 180000 });
writeFileSync(resolve(output, "go.txt"), goRaw);
console.error("运行 TypeScript 编解码基准（单线程，热身后 5 轮）…");
const tsRaw = command(process.execPath, ["client/dist/protocol_benchmark.js"], {
  cwd: example, timeout: 120000,
});
writeFileSync(resolve(output, "typescript.json"), tsRaw);
writeFileSync(resolve(output, "environment.json"), JSON.stringify(metadata, null, 2) + "\n");
const ts = JSON.parse(tsRaw);
const median = values => [...values].sort((a, b) => a - b)[Math.floor(values.length / 2)];
const grouped = new Map();
for (const line of goRaw.split("\n")) {
  const match = line.match(/^BenchmarkProtocolCodec\/(\S+?)\s+\d+\s+([\d.]+) ns\/op\s+[\d.]+ MB\/s\s+(\d+) B\/op\s+(\d+) allocs\/op$/);
  if (!match) continue;
  const name = match[1].replace(/-\d+$/, "");
  if (!grouped.has(name)) grouped.set(name, []);
  grouped.get(name).push({ ns: Number(match[2]), bytes: Number(match[3]), allocs: Number(match[4]) });
}
const go = new Map([...grouped].map(([name, samples]) => {
  if (samples.length !== 5) throw new Error(`missing Go rounds: ${name}`);
  return [name, {
    nsPerOp: median(samples.map(sample => sample.ns)),
    minNs: Math.min(...samples.map(sample => sample.ns)),
    maxNs: Math.max(...samples.map(sample => sample.ns)),
    bytes: median(samples.map(sample => sample.bytes)),
    allocs: median(samples.map(sample => sample.allocs)),
  }];
}));
const typescript = new Map(ts.results.map(result => [result.name, result]));
const cases = JSON.parse(readFileSync(resolve(example, "benchmarks/cases.json"), "utf8"));
if (go.size !== cases.length * 8 || typescript.size !== go.size) throw new Error("incomplete benchmark output");
const lines = [
  "# Protobuf / Sproto Go 与 TypeScript 编解码基准", "",
  `采样时间：${metadata.date}。CPU：${metadata.cpu}；系统：${metadata.os}。`, "",
  `${metadata.go}；Node ${metadata.node}；V8 ${ts.v8}。`, "",
  `基于提交 ${metadata.commit}${metadata.workingTree.trim() ? "，包含本报告随附的工作树修改" : ""}。`, "",
  `Go：google.golang.org/protobuf v1.36.1、cnooljaw/gosproto ${metadata.codecs.module.Version}${metadata.codecs.module.Replace ? " 基础的本地工作树（未发布优化版）" : ""}。TS：@bufbuild/protobuf 2.15.0、示例内修正的 sealindx/sproto-ts；源码哈希见 environment.json。`, "",
  "## 测量范围", "",
  "共用 cases.json 的 EchoRequest{text} 内容和 UTF-8 字节；两端在计时前验证固定 SHA-256、字节数和解码内容。TS Sproto 应用帧还与真实 SprotoEchoClient 的输出逐字节核对。", "",
  "payload 仅测消息体，不做 packing；frame 测实际应用层格式：Protobuf Envelope(msg_id=1001, request_id=1, payload)，Sproto package(type=1, session=1)+payload 后 pack，解码包含 unpack。两种协议编号沿用现有 Echo 定义。", "",
  "不含 schema 构建、网络、WebSocket、MAC、Dispatcher、安全预检、业务 Handler 或 RPC 会话表。payload 输入对象复用，frame 的外层头对象每次构造；输出分配计时，每次解码生成新对象。TS Sproto 的固定内联请求头长 6 字节，解码头后据此切分 payload，不代表通用路由成本。", "",
  "Go 固定 GOMAXPROCS=1，200ms/样本、5 样本；TS 每操作至少约 50ms 热身并校准约 100ms/样本、5 样本，轮间反转顺序；均取中位数。TS 保留常规 JIT/GC，不手工 GC。两种语言顺序运行。Go 吞吐率分母是原始 text UTF-8 字节数。", "",
  "## 包大小（字节）", "",
  "| 内容 | text | PB payload | SP payload | PB frame | SP packed frame |",
  "| --- | ---: | ---: | ---: | ---: | ---: |",
];
for (const sample of cases) {
  lines.push(`| ${sample.name} | ${Buffer.byteLength(sample.pattern.repeat(sample.repeat))} | ${sample.wire.protobuf_payload.bytes} | ${sample.wire.sproto_payload.bytes} | ${sample.wire.protobuf_frame.bytes} | ${sample.wire.sproto_frame.bytes} |`);
}
for (const [language, results] of [["Go", go], ["TypeScript", typescript]]) {
  lines.push("", `## ${language} 耗时`, "", "单位 µs/op。SP/PB 大于 1 表示本轮 Sproto 更慢；小于 1 表示更快。", "",
    "| 层级 | 内容 | PB 编码 | SP 编码 | SP/PB | PB 解码 | SP 解码 | SP/PB |",
    "| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |");
  for (const stage of ["payload", "frame"]) {
    for (const sample of cases) {
      const value = (codec, action) => results.get(`${sample.name}/${codec}_${stage}/${action}`).nsPerOp;
      const pe = value("protobuf", "encode"), se = value("sproto", "encode");
      const pd = value("protobuf", "decode"), sd = value("sproto", "decode");
      lines.push(`| ${stage} | ${sample.name} | ${(pe / 1000).toFixed(3)} | ${(se / 1000).toFixed(3)} | ${(se / pe).toFixed(2)} | ${(pd / 1000).toFixed(3)} | ${(sd / 1000).toFixed(3)} | ${(sd / pd).toFixed(2)} |`);
    }
  }
}
lines.push("", "## Go 分配", "", "每格为 B/op / allocs/op；TS 未测堆分配。", "",
  "| 层级 | 内容 | PB 编码 | SP 编码 | PB 解码 | SP 解码 |",
  "| --- | --- | ---: | ---: | ---: | ---: |");
for (const stage of ["payload", "frame"]) {
  for (const sample of cases) {
    const allocation = (codec, action) => {
      const result = go.get(`${sample.name}/${codec}_${stage}/${action}`);
      return `${result.bytes} / ${result.allocs}`;
    };
    lines.push(`| ${stage} | ${sample.name} | ${allocation("protobuf", "encode")} | ${allocation("sproto", "encode")} | ${allocation("protobuf", "decode")} | ${allocation("sproto", "decode")} |`);
  }
}
lines.push("", "## 采样波动", "", "以下列出五轮最大/最小耗时超过 2 的项目。原始文件保留全部样本；这个阈值只用于提示波动，不是显著性检验。", "",
  "| 语言 | 项目 | 最小 µs | 中位 µs | 最大 µs | 最大/最小 |",
  "| --- | --- | ---: | ---: | ---: | ---: |");
for (const [language, results] of [["Go", go], ["TypeScript", typescript]]) {
  for (const [name, result] of results) {
    const min = result.minNs ?? Math.min(...result.samples);
    const max = result.maxNs ?? Math.max(...result.samples);
    if (max / min <= 2) continue;
    lines.push(`| ${language} | ${name} | ${(min / 1000).toFixed(3)} | ${(result.nsPerOp / 1000).toFixed(3)} | ${(max / 1000).toFixed(3)} | ${(max / min).toFixed(2)} |`);
  }
}
lines.push("", "## 复现与解释", "", "从仓库根目录运行：", "", "```sh",
  "npm --prefix examples/websocket-gate ci",
  "npm --prefix examples/websocket-gate run bench:protocol", "```", "",
  "默认产物位于 tmp/protocol-benchmark/：report.md、go.txt、typescript.json、environment.json。也可在 npm 命令末尾加 -- /绝对输出目录。原始结果保留每轮耗时；Go 原始结果另含吞吐和分配。", "",
  "这是当前实现对 Echo 字符串的本机基线。zeros4k 是刻意构造的高压缩率输入，不能代表普通游戏消息；未覆盖整数数组、嵌套结构、浏览器、Lua/C 实现或端到端吞吐。Go Sproto 含反射、资源预算和失败原子性成本，TS 为示例参考实现，结果不能归因为 wire 格式的理论性能，也不能用于承诺所有消息的胜负。", "");
writeFileSync(resolve(output, "report.md"), lines.join("\n"));
console.log(`Benchmark 完成：${resolve(output, "report.md")}`);
