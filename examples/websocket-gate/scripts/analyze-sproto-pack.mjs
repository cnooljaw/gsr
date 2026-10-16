import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { Session } from "node:inspector/promises";
import { cpus } from "node:os";
import { resolve } from "node:path";
import { performance } from "node:perf_hooks";
import { fileURLToPath, pathToFileURL } from "node:url";

// 单独运行，不能与计时基准并发。可传入旧模块，使用同一脚本生成前后诊断。
const example = fileURLToPath(new URL("../", import.meta.url));
const root = resolve(example, "../..");
const output = resolve(process.argv[2] ?? resolve(root, "tmp/sproto-pack-analysis"));
const modulePath = resolve(process.argv[3] ?? resolve(example, "client/dist/vendor/sproto.js"));
const { Sproto } = await import(pathToFileURL(modulePath).href);
const read = name => readFileSync(resolve(example, name), "utf8");
const hash = value => createHash("sha256").update(value).digest("hex");
const schema = read("benchmarks/schema/representative.sproto");
const samples = JSON.parse(read("benchmarks/representative.json"));
mkdirSync(output, { recursive: true });
const patterns = samples.map(sample => {
  const codec = new Sproto(schema), raw = codec.encode(sample.type, sample.data);
  const histogram = new Uint32Array(256), nonzeroCounts = new Uint32Array(9);
  for (let offset = 0; offset < raw.length; offset += 8) {
    let mask = 0, count = 0;
    for (let index = 0; index < 8 && offset + index < raw.length; index++) {
      if (raw[offset + index] !== 0) { mask |= 1 << index; count++; }
    }
    histogram[mask]++; nonzeroCounts[count]++;
  }
  // 只在分布观测中包装方法，计时和 CPU profile 使用下面的新实例。
  let wordCalls = 0;
  const packWord = codec.pack_seg;
  codec.pack_seg = function (...args) { wordCalls++; return packWord.apply(this, args); };
  const packed = codec.pack(raw);
  return { name: sample.name, rawBytes: raw.length, packedBytes: packed.length,
    rawSha256: hash(raw), packedSha256: hash(packed), wordCount: Math.ceil(raw.length / 8),
    wordCalls, nonzeroCounts: [...nonzeroCounts], masks: [...histogram.entries()]
      .filter(([, count]) => count).sort((a, b) => b[1] - a[1])
      .map(([mask, count]) => ({ mask: mask.toString(16).padStart(2, "0"), count })) };
});
const operations = [];
for (const name of ["game_large", "int32_edges"]) {
  const sample = samples.find(sample => sample.name === name), codec = new Sproto(schema);
  const raw = codec.encode(sample.type, sample.data);
  operations.push({ name: `${name}/pack`, run: () => codec.pack(raw) },
    { name: `${name}/pencode`, run: () => codec.pencode(sample.type, sample.data) });
}
const echoes = JSON.parse(read("benchmarks/cases.json"));
for (const name of ["ascii4k", "zeros4k"]) {
  const sample = echoes.find(sample => sample.name === name);
  const codec = new Sproto(read("sproto/c2s.sproto")), request = codec.attach();
  const message = { text: sample.pattern.repeat(sample.repeat) }, wire = request("echo", message, 1);
  const manual = new Sproto(read("benchmarks/echo.sproto"));
  operations.push({ name: `${name}/unpack`, run: () => manual.unpack(wire) },
    { name: `${name}/rpc_decode`, run: () => codec.dispatch(wire) },
    { name: `${name}/rpc_decode_manual`, run: () => {
      const raw = manual.unpack(wire);
      manual.decode("package", raw);
      return manual.decode("EchoRequest", raw.subarray(6));
    } });
}
// 只记录显式工厂/视图调用，不能当作完整 JS 对象数或堆分配 B/op。
const bufferCalls = operations.filter(operation => operation.name.startsWith("ascii4k/") ||
  operation.name.startsWith("zeros4k/")).map(operation => {
  for (let index = 0; index < 10; index++) operation.run();
  const originalAllocate = Buffer.allocUnsafe, originalSubarray = Buffer.prototype.subarray;
  let allocations = 0, allocatedBytes = 0, subarrays = 0;
  try {
    Buffer.allocUnsafe = function (size) { allocations++; allocatedBytes += size; return originalAllocate(size); };
    Buffer.prototype.subarray = function (...args) { subarrays++; return originalSubarray.apply(this, args); };
    operation.run();
  } finally {
    Buffer.allocUnsafe = originalAllocate; Buffer.prototype.subarray = originalSubarray;
  }
  return { name: operation.name, allocations, allocatedBytes, subarrays };
});
const session = new Session(); session.connect();
await session.post("Profiler.enable");
await session.post("Profiler.setSamplingInterval", { interval: 250 });
const profiles = [];
let sink;
try {
  for (const operation of operations) {
    for (let index = 0; index < 10000; index++) sink = operation.run();
    await session.post("Profiler.start");
    const end = performance.now() + 1000;
    while (performance.now() < end) for (let index = 0; index < 100; index++) sink = operation.run();
    const { profile } = await session.post("Profiler.stop");
    writeFileSync(resolve(output, operation.name.replace("/", "-") + ".cpuprofile"), JSON.stringify(profile));
    const totals = new Map(), cumulative = new Map();
    const nodes = new Map(profile.nodes.map(node => [node.id, node])), parents = new Map();
    for (const node of profile.nodes) for (const child of node.children ?? []) parents.set(child, node.id);
    const key = node => `${node.callFrame.functionName || "(anonymous)"} ${node.callFrame.url.split("/").at(-1)}`;
    for (const node of profile.nodes) {
      const name = key(node);
      totals.set(name, (totals.get(name) ?? 0) + (node.hitCount ?? 0));
    }
    for (const sample of profile.samples) {
      const seen = new Set();
      for (let id = sample; id !== undefined; id = parents.get(id)) seen.add(key(nodes.get(id)));
      for (const name of seen) cumulative.set(name, (cumulative.get(name) ?? 0) + 1);
    }
    profiles.push({ name: operation.name, samples: profile.samples.length,
      top: [...totals].sort((a, b) => b[1] - a[1]).slice(0, 10)
        .map(([name, hits]) => ({ name, hits, selfPercent: 100 * hits / profile.samples.length,
          cumulativePercent: 100 * (cumulative.get(name) ?? 0) / profile.samples.length })) });
  }
} finally { session.disconnect(); }
if (sink === undefined) throw new Error("empty profile");
writeFileSync(resolve(output, "analysis.json"), JSON.stringify({
  date: new Date().toISOString(), node: process.version, v8: process.versions.v8, cpu: cpus()[0]?.model,
  moduleSha256: hash(readFileSync(modulePath)), scriptSha256: hash(readFileSync(fileURLToPath(import.meta.url))),
  fixtureSha256: hash(read("benchmarks/representative.json")),
  secondsPerProfile: 1, intervalMicros: 250, patterns, bufferCalls, profiles,
}, null, 2) + "\n");
console.log(`Pack 分布、Buffer 调用和 CPU 摘要：${resolve(output, "analysis.json")}`);
