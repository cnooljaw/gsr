import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { performance } from "node:perf_hooks";
import { resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { cpus, loadavg } from "node:os";
import { create, fromBinary, fromJson, toBinary, toJson } from "@bufbuild/protobuf";
import { AddressBookSchema, GameStateSchema, IntArraysSchema, EchoRequestSchema as BodyEchoSchema }
  from "../client/dist/gen/benchmark/representative_pb.js";
import { EchoRequestSchema, MsgId } from "../client/dist/gen/gsr/example/echo/v1/echo_pb.js";
import { EnvelopeSchema } from "../client/dist/gen/gsr/common/v1/common_pb.js";

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
// 同值的独立 repeat 结果可能有不同的 V8 字符串表示；三方必须共享同一份输入。
const echoMessages = new Map(echoes.map(sample => [sample.name, { text: sample.pattern.repeat(sample.repeat) }]));
const schema = read("benchmarks/schema/representative.sproto");
const rpcSchema = read("sproto/c2s.sproto");
const echoSchema = read("benchmarks/echo.sproto");
const descriptors = { AddressBook: AddressBookSchema, GameState: GameStateSchema,
  IntArrays: IntArraysSchema, EchoRequest: BodyEchoSchema };
const operations = [], sources = {};
// 解包容量策略同时覆盖原始块、零块、混合数值 word 和长 FF 前缀后的稀疏数据。
const unpackShapes = [];
for (const length of [32, 256, 1024, 2048, 4096, 16384, 65536]) {
  for (const pattern of ["ascii", "zero", "sparse"]) {
    const raw = Buffer.alloc(length);
    for (let index = 0; index < length; index++) {
      raw[index] = pattern === "ascii" ? 127 : pattern === "zero" ? 0 : index % 4 === 0 ? 128 : 0;
    }
    unpackShapes.push({ name: `${pattern}${length}`, raw });
  }
}
for (const prefix of [512, 1024, 2048, 4096]) {
  for (const length of [4096, 16384]) {
    const sparse = Buffer.alloc(length);
    for (let index = 0; index < length; index += 4) sparse[index] = 128;
    unpackShapes.push({ name: `FF${prefix}-sparse${length}`, raw: Buffer.concat([Buffer.alloc(prefix, 127), sparse]) });
  }
}
// 重复短零串保护批量比较失败的成本；长零前缀后仍需正确恢复非零段。
for (const words of [2, 8, 9, 16, 17, 32, 33, 64, 65, 128, 129, 256, 257]) {
  const block = Buffer.concat([Buffer.alloc(words * 8), Buffer.alloc(8, 127)]);
  unpackShapes.push({ name: `zeroRun${words}x64`, raw: Buffer.concat(Array(64).fill(block)) });
}
for (const prefix of [64, 512, 4096, 16384]) {
  unpackShapes.push({ name: `zero${prefix}-ascii4096`, raw: Buffer.concat([Buffer.alloc(prefix), Buffer.alloc(4096, 127)]) });
}
const referenceShapes = new Map();
const hash = bytes => createHash("sha256").update(bytes).digest("hex");
const startedAt = new Date().toISOString(), startLoad = loadavg();
// PB 与 SP 交错采样；原生组合入口和历史分阶段组合分别保留。
for (const sample of representative) {
  const descriptor = descriptors[sample.type], message = fromJson(descriptor, sample.data);
  const wire = toBinary(descriptor, message);
  assert.equal(hash(wire), sample.wire.protobuf_encode.sha256);
  assert.deepEqual(toJson(descriptor, fromBinary(descriptor, wire)), sample.data);
  for (const [name, run] of Object.entries({ encode: () => toBinary(descriptor, message),
    decode: () => fromBinary(descriptor, wire) })) {
    operations.push({ name: `${sample.name}/${name}`, version: "protobuf", run, samples: [] });
  }
}
for (const sample of echoes) {
  const message = create(EchoRequestSchema, echoMessages.get(sample.name));
  const encode = () => toBinary(EnvelopeSchema, create(EnvelopeSchema, {
    msgId: MsgId.C2S_ECHO_REQUEST, requestId: 1, payload: toBinary(EchoRequestSchema, message),
  }));
  const wire = encode();
  const decode = () => fromBinary(EchoRequestSchema, fromBinary(EnvelopeSchema, wire).payload);
  assert.equal(hash(wire), sample.wire.protobuf_frame.sha256);
  assert.equal(decode().text, message.text);
  for (const [name, run] of Object.entries({ rpc_encode: encode, rpc_decode: decode })) {
    operations.push({ name: `${sample.name}/${name}`, version: "protobuf", run, samples: [] });
  }
}
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
      combined_encode: () => codec.pack(codec.encode(sample.type, sample.data)),
      combined_decode: () => codec.decode(sample.type, codec.unpack(wire)),
    };
    for (const [name, run] of Object.entries(steps)) {
      operations.push({ name: `${sample.name}/${name}`, version, run, samples: [] });
    }
  }
  for (const sample of echoes) {
    const codec = new Sproto(rpcSchema), request = codec.attach();
    const manual = new Sproto(echoSchema), server = new Sproto(rpcSchema);
    const message = echoMessages.get(sample.name);
    // 固定 session 只覆盖一个等待项，避免基准自己积累会话；不含客户端安全预检。
    const run = () => request("echo", message, 1);
    const wire = run();
    assert.equal(hash(wire), sample.wire.sproto_frame.sha256);
    const snapshot = Buffer.from(wire);
    request("echo", { text: "next" }, 1);
    assert.deepEqual(wire, snapshot);
    const manualEncode = () => manual.pack(Buffer.concat([
      manual.encode("package", { type: 1, session: 1 }), manual.encode("EchoRequest", message),
    ]));
    const manualDecode = () => {
      const raw = manual.unpack(wire);
      manual.decode("package", raw);
      return manual.decode("EchoRequest", raw.subarray(6));
    };
    assert.deepEqual(manualEncode(), wire);
    assert.deepEqual(manualDecode(), message);
    assert.deepEqual(server.dispatch(wire).result, message);
    const steps = { rpc_encode: run, rpc_decode: () => server.dispatch(wire),
      rpc_encode_manual: manualEncode, rpc_decode_manual: manualDecode };
    for (const [name, run] of Object.entries(steps)) {
      operations.push({ name: `${sample.name}/${name}`, version, run, samples: [] });
    }
  }
  const shapeCodec = new Sproto(".Probe { value 0 : integer }");
  for (const { name, raw } of unpackShapes) {
    const packed = shapeCodec.pack(raw);
    if (!referenceShapes.has(name)) referenceShapes.set(name, packed);
    assert.deepEqual(packed, referenceShapes.get(name));
    assert.deepEqual(shapeCodec.unpack(packed), raw);
    operations.push({ name: `unpack_shape/${name}`, version,
      run: () => shapeCodec.unpack(packed), samples: [] });
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
  startedAt, startLoad, endLoad: loadavg(), platform: process.platform, arch: process.arch,
  cpu: cpus()[0]?.model, protobufVersion: JSON.parse(read("node_modules/@bufbuild/protobuf/package.json")).version,
  fixtures: Object.fromEntries(["benchmarks/representative.json", "benchmarks/cases.json",
    "benchmarks/schema/representative.sproto", "benchmarks/echo.sproto", "sproto/c2s.sproto"]
    .map(name => [name, hash(read(name))])),
  scriptSha256: hash(readFileSync(fileURLToPath(import.meta.url))),
  unpackShapes: unpackShapes.map(({ name, raw }) => ({ name, rawBytes: raw.length,
    packedBytes: referenceShapes.get(name).length, rawSha256: hash(raw), packedSha256: hash(referenceShapes.get(name)) })),
  rounds: 5, targetMilliseconds: 100, sources,
  echoInputsShared: true,
  results: operations.map(({ name, version, iterations, samples }) => ({
    name, version, iterations, samples, nsPerOp: [...samples].sort((a, b) => a - b)[2],
  })),
}, null, 2) + "\n");
console.log(`Sproto API Benchmark 完成：${resolve(output, "typescript.json")}`);
