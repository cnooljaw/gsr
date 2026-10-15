import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { Session } from "node:inspector/promises";
import { performance } from "node:perf_hooks";
import { resolve } from "node:path";
import { parseArgs } from "node:util";
import { fromBinary, fromJson, toBinary, toJson, type JsonValue } from "@bufbuild/protobuf";
import {
  AddressBookSchema, GameStateSchema, IntArraysSchema, EchoRequestSchema,
} from "./gen/benchmark/representative_pb.js";
import { Sproto } from "./vendor/sproto.js";

const { values } = parseArgs({ options: {
  mode: { type: "string", default: "bench" },
  operation: { type: "string" },
  seconds: { type: "string", default: "2" },
  output: { type: "string" },
} });
const schemas = { AddressBook: AddressBookSchema, GameState: GameStateSchema,
  IntArrays: IntArraysSchema, EchoRequest: EchoRequestSchema };
interface Sample {
  name: string;
  type: keyof typeof schemas;
  data: JsonValue;
  wire: Record<string, { bytes: number; sha256: string }>;
}
interface Operation {
  name: string;
  run: () => unknown;
  iterations: number;
  samples: number[];
}
const directory = new URL("../../benchmarks/", import.meta.url);
const cases: Sample[] = JSON.parse(readFileSync(new URL("representative.json", directory), "utf8"));
const schemaText = readFileSync(new URL("schema/representative.sproto", directory), "utf8");
const operations: Operation[] = [];
let sink: unknown;
// 单独记录已知能力缺口，不把失败数据混入性能排名，也不将当前失败固化成接口契约。
const capabilityProbes = [
  { name: "boolean", schema: ".Probe { value 0 : boolean }", input: { value: true } },
  { name: "mixed_signed_int64_array", schema: ".Probe { value 0 : *integer }",
    input: { value: [1, 4294967296, -1] } },
].map(probe => {
  try {
    const codec = new Sproto(probe.schema);
    const wire = codec.encode("Probe", probe.input)!;
    assert.deepEqual(codec.decode("Probe", wire), probe.input);
    return { name: probe.name, passed: true };
  } catch (error) {
    return { name: probe.name, passed: false, error: String(error) };
  }
});
for (const sample of cases) {
  const descriptor = schemas[sample.type];
  assert.ok(descriptor);
  const protobufMessage = fromJson(descriptor, sample.data);
  const codec = new Sproto(schemaText);
  const protobufWire = toBinary(descriptor, protobufMessage);
  const sprotoWire = codec.encode(sample.type, sample.data)!;
  const packed = codec.pack(sprotoWire);
  const wires = { protobuf_encode: protobufWire, sproto_encode: sprotoWire, sproto_pack: packed };
  for (const [name, wire] of Object.entries(wires)) {
    assert.equal(wire.length, sample.wire[name].bytes, `${sample.name}/${name} bytes`);
    assert.equal(createHash("sha256").update(wire).digest("hex"), sample.wire[name].sha256,
      `${sample.name}/${name} hash`);
  }
  assert.deepEqual(toJson(descriptor, fromBinary(descriptor, protobufWire)), sample.data);
  assert.deepEqual(codec.decode(sample.type, sprotoWire), sample.data, `${sample.name} semantic round trip`);
  const unpacked = codec.unpack(packed)!;
  assert.deepEqual(unpacked.subarray(0, sprotoWire.length), sprotoWire);
  assert.ok(unpacked.length - sprotoWire.length <= 7);
  assert.ok(unpacked.subarray(sprotoWire.length).every(byte => byte === 0));
  const steps = {
    protobuf_encode: () => toBinary(descriptor, protobufMessage),
    protobuf_decode: () => fromBinary(descriptor, protobufWire),
    sproto_encode: () => codec.encode(sample.type, sample.data),
    sproto_decode: () => codec.decode(sample.type, sprotoWire),
    sproto_pack: () => codec.pack(sprotoWire),
    sproto_unpack: () => codec.unpack(packed),
    sproto_pencode: () => codec.pack(codec.encode(sample.type, sample.data)!),
    sproto_pdecode: () => codec.decode(sample.type, codec.unpack(packed)!),
  };
  for (const [name, run] of Object.entries(steps)) {
    operations.push({ name: `${sample.name}/${name}`, run, iterations: 1, samples: [] });
  }
}

function batch(operation: Operation, iterations: number): number {
  const start = performance.now();
  for (let index = 0; index < iterations; index++) sink = operation.run();
  return performance.now() - start;
}

function warmup(operation: Operation): void {
  let iterations = 128;
  let elapsed = batch(operation, iterations);
  while (elapsed < 50) {
    iterations *= 2;
    elapsed = batch(operation, iterations);
  }
  operation.iterations = Math.max(1, Math.ceil(iterations * 100 / elapsed));
}

function runFor(operation: Operation, seconds: number): number {
  const deadline = performance.now() + seconds * 1000;
  let iterations = 0;
  while (performance.now() < deadline) {
    batch(operation, operation.iterations);
    iterations += operation.iterations;
  }
  assert.notEqual(sink, undefined);
  return iterations;
}

if (values.mode === "check") {
  console.log(JSON.stringify({ checked: cases.length, operations: operations.length, capabilityProbes }));
} else if (values.mode === "profile") {
  const operation = operations.find(operation => operation.name === values.operation);
  assert.ok(operation, "choose a known --operation case/operation");
  assert.ok(values.output, "set --output directory");
  const seconds = Number(values.seconds);
  assert.ok(seconds > 0 && seconds <= 10);
  warmup(operation);
  const output = resolve(values.output);
  mkdirSync(output, { recursive: true });
  const session = new Session();
  session.connect();
  try {
    await session.post("Profiler.enable");
    await session.post("Profiler.setSamplingInterval", { interval: 1000 });
    await session.post("Profiler.start");
    const cpuIterations = runFor(operation, seconds);
    const cpu = await session.post("Profiler.stop");
    writeFileSync(resolve(output, "cpu.cpuprofile"), JSON.stringify(cpu.profile));
    // 分开采集，避免堆分配采样器影响 CPU profile。包括采样期间已被 GC 的对象。
    await session.post("HeapProfiler.startSampling", {
      samplingInterval: 16384,
      includeObjectsCollectedByMajorGC: true, includeObjectsCollectedByMinorGC: true,
    });
    const heapIterations = runFor(operation, seconds);
    const heap = await session.post("HeapProfiler.stopSampling");
    writeFileSync(resolve(output, "heap.heapprofile"), JSON.stringify(heap.profile));
    writeFileSync(resolve(output, "profile.json"), JSON.stringify({
      operation: operation.name, seconds, intervalBytes: 16384, cpuIntervalMicros: 1000,
      node: process.version, v8: process.versions.v8, cpuIterations, heapIterations,
    }, null, 2) + "\n");
  } finally {
    session.disconnect();
  }
} else {
  assert.equal(values.mode, "bench");
  for (const operation of operations) warmup(operation);
  for (let round = 0; round < 5; round++) {
    const ordered = round % 2 === 0 ? operations : [...operations].reverse();
    for (const operation of ordered) {
      operation.samples.push(batch(operation, operation.iterations) * 1e6 / operation.iterations);
    }
  }
  assert.notEqual(sink, undefined);
  console.log(JSON.stringify({ node: process.version, v8: process.versions.v8, rounds: 5, capabilityProbes,
    results: operations.map(({ name, iterations, samples }) => ({ name, iterations, samples,
      nsPerOp: [...samples].sort((a, b) => a - b)[2] })),
  }, null, 2));
}
