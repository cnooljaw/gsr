import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { performance } from "node:perf_hooks";
import { create, fromBinary, toBinary } from "@bufbuild/protobuf";
import { EnvelopeSchema } from "./gen/gsr/common/v1/common_pb.js";
import { EchoRequestSchema, MsgId } from "./gen/gsr/example/echo/v1/echo_pb.js";
import { SprotoEchoClient } from "./sproto_client.js";
import { Sproto } from "./vendor/sproto.js";

interface Sample {
  name: string;
  pattern: string;
  repeat: number;
  wire: Record<string, { bytes: number; sha256: string }>;
}

interface Operation {
  name: string;
  run: () => unknown;
  iterations: number;
  samples: number[];
  wireBytes: number;
  textBytes: number;
}

const directory = new URL("../../benchmarks/", import.meta.url);
const cases: Sample[] = JSON.parse(readFileSync(new URL("cases.json", directory), "utf8"));
const schema = readFileSync(new URL("echo.sproto", directory), "utf8");
const operations: Operation[] = [];
let sink: unknown;

for (const sample of cases) {
  const text = sample.pattern.repeat(sample.repeat);
  const protobufMessage = create(EchoRequestSchema, { text });
  const sprotoMessage = { text };
  const codec = new Sproto(schema);
  const header = { type: 1, session: 1 };
  const headerLength = codec.encode("package", header)!.length;
  const codecs: Record<string, { encode: () => Uint8Array; decode: (wire: Uint8Array) => string }> = {
    protobuf_payload: {
      encode: () => toBinary(EchoRequestSchema, protobufMessage),
      decode: wire => fromBinary(EchoRequestSchema, wire).text,
    },
    sproto_payload: {
      encode: () => codec.encode("EchoRequest", sprotoMessage)!,
      decode: wire => (codec.decode("EchoRequest", wire as Buffer) as { text: string }).text,
    },
    protobuf_frame: {
      encode: () => toBinary(EnvelopeSchema, create(EnvelopeSchema, {
        msgId: MsgId.C2S_ECHO_REQUEST, requestId: 1,
        payload: toBinary(EchoRequestSchema, protobufMessage),
      })),
      decode: wire => fromBinary(EchoRequestSchema, fromBinary(EnvelopeSchema, wire).payload).text,
    },
    sproto_frame: {
      encode: () => codec.pack(Buffer.concat([
        codec.encode("package", { type: 1, session: 1 })!, codec.encode("EchoRequest", sprotoMessage)!,
      ])),
      decode: wire => {
        const unpacked = codec.unpack(wire as Buffer)!;
        codec.decode("package", unpacked);
        // 此固定请求的 type/session 均内联，package 长度恒为 6；不测通用路由解析。
        return (codec.decode("EchoRequest", unpacked.subarray(headerLength)) as { text: string }).text;
      },
    },
  };
  // 具名 payload schema 必须与真实 Echo 客户端生成的应用帧逐字节相同。
  const client = new SprotoEchoClient();
  assert.deepEqual(codecs.sproto_frame.encode(), client.encode(text).frame);
  client.close();
  for (const [name, codec] of Object.entries(codecs)) {
    const wire = codec.encode();
    assert.equal(wire.length, sample.wire[name].bytes);
    assert.equal(createHash("sha256").update(wire).digest("hex"), sample.wire[name].sha256);
    assert.equal(codec.decode(wire), text);
    for (const [action, run] of Object.entries({ encode: codec.encode, decode: () => codec.decode(wire) })) {
      operations.push({
        name: `${sample.name}/${name}/${action}`, run, iterations: 1, samples: [],
        wireBytes: wire.length, textBytes: Buffer.byteLength(text),
      });
    }
  }
}

// 批次外读时钟；每次保留结果，编码分配和新对象解码均计入时间。
function batch(operation: Operation, iterations: number): number {
  const start = performance.now();
  for (let index = 0; index < iterations; index++) sink = operation.run();
  return performance.now() - start;
}

const targetMilliseconds = 100;
for (const operation of operations) {
  // 每个操作独立热身并校准；schema 解析、断言和初次扩容不计时。
  let iterations = 128;
  let elapsed = batch(operation, iterations);
  while (elapsed < 50) {
    iterations *= 2;
    elapsed = batch(operation, iterations);
  }
  operation.iterations = Math.max(1, Math.ceil(iterations * targetMilliseconds / elapsed));
}
for (let round = 0; round < 5; round++) {
  // 交替反转顺序，降低固定执行次序造成的温度与 GC 偏差。
  const ordered = round % 2 === 0 ? operations : [...operations].reverse();
  for (const operation of ordered) {
    const elapsed = batch(operation, operation.iterations);
    operation.samples.push(elapsed * 1e6 / operation.iterations);
  }
}
assert.notEqual(sink, undefined);
console.log(JSON.stringify({
  node: process.version, v8: process.versions.v8, targetMilliseconds, rounds: 5,
  results: operations.map(({ name, samples, iterations, wireBytes, textBytes }) => ({
    name, samples, iterations, wireBytes, textBytes,
    nsPerOp: [...samples].sort((a, b) => a - b)[Math.floor(samples.length / 2)],
  })),
}, null, 2));
