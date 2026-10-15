import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { performance } from "node:perf_hooks";
import { resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

// 复用已有夹具，补测 pencode/attach 的内部缓冲优化；不替换原有分阶段基准。
const example = fileURLToPath(new URL("../", import.meta.url));
const root = resolve(example, "../..");
const output = resolve(process.argv[2] ?? resolve(root, "tmp/protocol-sproto-api"));
const current = resolve(example, "client/dist/vendor/sproto.js");
const variants = process.argv[3]
  ? [["before", resolve(process.argv[3])], ["after", current]] : [["after", current]];
const read = name => readFileSync(resolve(example, name), "utf8");
const representative = JSON.parse(read("benchmarks/representative.json"));
const echoes = JSON.parse(read("benchmarks/cases.json"));
const schema = read("benchmarks/schema/representative.sproto");
const rpcSchema = read("sproto/c2s.sproto");
const operations = [], sources = {};
const hash = bytes => createHash("sha256").update(bytes).digest("hex");
for (const [version, modulePath] of variants) {
  const { Sproto } = await import(pathToFileURL(modulePath).href);
  sources[version] = { path: modulePath, sha256: hash(readFileSync(modulePath)) };
  for (const sample of representative) {
    const codec = new Sproto(schema);
    const raw = codec.encode(sample.type, sample.data);
    const wire = codec.pencode(sample.type, sample.data);
    assert.equal(hash(raw), sample.wire.sproto_encode.sha256);
    assert.equal(hash(wire), sample.wire.sproto_pack.sha256);
    assert.deepEqual(codec.pdecode(sample.type, wire), sample.data);
    const steps = {
      encode: () => codec.encode(sample.type, sample.data),
      decode: () => codec.decode(sample.type, raw),
      pack: () => codec.pack(raw), unpack: () => codec.unpack(wire),
      pencode: () => codec.pencode(sample.type, sample.data),
      pdecode: () => codec.pdecode(sample.type, wire),
    };
    for (const [name, run] of Object.entries(steps)) {
      operations.push({ name: `${sample.name}/${name}`, version, run, samples: [] });
    }
  }
  for (const sample of echoes) {
    const codec = new Sproto(rpcSchema), request = codec.attach();
    const message = { text: sample.pattern.repeat(sample.repeat) };
    // 固定 session 只覆盖一个等待项，避免基准自己积累会话；不含客户端安全预检。
    const run = () => request("echo", message, 1);
    const wire = run();
    assert.equal(hash(wire), sample.wire.sproto_frame.sha256);
    const snapshot = Buffer.from(wire);
    request("echo", { text: "next" }, 1);
    assert.deepEqual(wire, snapshot);
    operations.push({ name: `${sample.name}/rpc_encode`, version, run, samples: [] });
  }
}
let sink;
function batch(operation, iterations) {
  const start = performance.now();
  for (let index = 0; index < iterations; index++) sink = operation.run();
  return performance.now() - start;
}
for (const operation of operations) {
  let iterations = 128, elapsed = batch(operation, iterations);
  while (elapsed < 50) { iterations *= 2; elapsed = batch(operation, iterations); }
  operation.iterations = Math.max(1, Math.ceil(iterations * 100 / elapsed));
}
for (let round = 0; round < 5; round++) {
  for (const operation of round % 2 ? [...operations].reverse() : operations) {
    operation.samples.push(batch(operation, operation.iterations) * 1e6 / operation.iterations);
  }
}
assert.notEqual(sink, undefined);
mkdirSync(output, { recursive: true });
writeFileSync(resolve(output, "typescript.json"), JSON.stringify({
  date: new Date().toISOString(), node: process.version, v8: process.versions.v8,
  rounds: 5, targetMilliseconds: 100, sources,
  results: operations.map(({ name, version, iterations, samples }) => ({
    name, version, iterations, samples, nsPerOp: [...samples].sort((a, b) => a - b)[2],
  })),
}, null, 2) + "\n");
console.log(`Sproto API Benchmark 完成：${resolve(output, "typescript.json")}`);
