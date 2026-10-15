import assert from "node:assert/strict";
import { test } from "node:test";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { resolve } from "node:path";
import { readFileSync } from "node:fs";
import { Sproto } from "./vendor/sproto.js";
import { exchange, SprotoEchoClient, waitForOpen, waitForFrame } from "./sproto_client.js";

const c2sSchema = readFileSync(new URL("../../sproto/c2s.sproto", import.meta.url), "utf8");

// 官方向量保护负整数和 UTF-8，超出 JavaScript 安全范围必须失败。
test("official Sproto vectors and safe integer boundary", () => {
  const schema = new Sproto(".Scalar { value 0 : integer } .Text { text 0 : string }");
  const negative = Buffer.from("0100000004000000ffffffff", "hex");
  assert.equal((schema.decode("Scalar", negative) as { value: number }).value, -1);
  assert.deepEqual(schema.encode("Scalar", { value: -1 }), negative);
  const unicode = Buffer.from("010000000a000000e4b8ade69687f09f9982", "hex");
  assert.deepEqual(schema.encode("Text", { text: "中文🙂" }), unicode);
  assert.throws(() => schema.encode("Scalar", { value: Number.MAX_SAFE_INTEGER + 1 }), /safe/);
  assert.throws(() => schema.decode("Scalar", Buffer.from("01000000080000000000000000000080", "hex")), /safe/);
});

// 官方编码向量保护布尔、数组宽度切换与正负安全 int64，而非只做 TS 自身往返。
test("official Sproto boolean and mixed signed int64 arrays", () => {
  const vectors = JSON.parse(readFileSync(new URL("../../benchmarks/correctness.json", import.meta.url), "utf8"));
  for (const vector of vectors) {
    const codec = new Sproto(vector.schema);
    const raw = Buffer.from(vector.wire.sproto_encode.hex, "hex");
    const packed = Buffer.from(vector.wire.sproto_pack.hex, "hex");
    assert.deepEqual(codec.encode(vector.type, vector.data), raw, `${vector.name} encode`);
    assert.deepEqual(codec.decode(vector.type, raw), vector.data, `${vector.name} decode`);
    assert.deepEqual(codec.pack(raw), packed, `${vector.name} pack`);
    assert.deepEqual(codec.pencode(vector.type, vector.data), packed, `${vector.name} pencode`);
    assert.deepEqual(codec.decode(vector.type, codec.unpack(packed)!), vector.data, `${vector.name} unpack/decode`);
  }
  const integer = new Sproto(".Probe { value 0 : integer } .Array { value 0 : *integer }");
  for (const value of [NaN, Infinity, 1.5, Number.MAX_SAFE_INTEGER + 1, Number.MIN_SAFE_INTEGER - 1]) {
    assert.throws(() => integer.encode("Probe", { value }), /safe/);
    assert.throws(() => integer.encode("Array", { value: [1, value] }), /safe/);
  }
  const encoded = integer.encode("Array", { value: [1, 4294967296, -1] })!;
  const snapshot = Buffer.from(encoded);
  integer.encode("Array", { value: [7, 8, 9] });
  assert.deepEqual(encoded, snapshot, "encoded buffers remain owned by the caller");
  const packedSnapshot = integer.pencode("Array", { value: [1, 4294967296, -1] })!;
  const packedCopy = Buffer.from(packedSnapshot);
  integer.pencode("Array", { value: [7] });
  assert.deepEqual(packedSnapshot, packedCopy, "packed buffers remain owned by the caller");
  const future = new Sproto(".Future { unknown 0 : string middle 30000 : integer later 60000 : string }");
  const current = new Sproto(".Current { later 60000 : string }");
  const evolved = future.encode("Future", { unknown: "skip", middle: 7, later: "keep" })!;
  assert.deepEqual(current.decode("Current", evolved), { later: "keep" });
  // 数组宽值在末尾、扩容前后和非对齐 body 均不能改变低/高位写出。
  for (const value of [
    [1, -1, 4294967296],
    [Number.MIN_SAFE_INTEGER, 0, Number.MAX_SAFE_INTEGER],
    Array.from({ length: 1024 }, (_, index) => index % 2 ? -2147483648 : 2147483647),
    [7, -7],
  ]) {
    const width = value.some(n => n < -2147483648 || n > 2147483647) ? 8 : 4;
    const expected = Buffer.alloc(9 + value.length * width);
    expected.writeUInt16LE(1, 0);
    expected.writeUInt32LE(1 + value.length * width, 4);
    expected[8] = width;
    value.forEach((n, index) => {
      if (width === 8) expected.writeBigInt64LE(BigInt(n), 9 + index * width);
      else expected.writeInt32LE(n, 9 + index * width);
    });
    assert.deepEqual(integer.encode("Array", { value }), expected);
    assert.deepEqual(integer.decode("Array", expected), { value });
  }
  // 合并 scratch 后，package 不挤占原本属于单条 body 的 8 MiB 编码预算。
  const large = new Sproto(c2sSchema + "\n.Text { text 0 : string }");
  const request = large.attach();
  const text = "x".repeat((8 << 20) - 8);
  const frame = request("echo", { text }, 1)!;
  assert.ok(frame.length > 8 << 20);
  assert.throws(() => request("echo", { text: text + "x" }, 1), /size limit/);
  assert.throws(() => large.encode("Text", { text: text + "x" }), /size limit/);
});

// 同步解码复用缓冲时，大小交替、失败和大包均不污染已经返回的值或字节快照。
test("Sproto decoded results and unpacked buffers stay independent", () => {
  const codec = new Sproto(".Data { text 0 : string values 1 : *integer }");
  const original = { text: "first".repeat(1000), values: [1, -1, 4294967296] };
  const wire = codec.pencode("Data", original)!;
  const first = codec.pdecode("Data", wire);
  const unpacked = codec.unpack(wire)!;
  const snapshot = Buffer.from(unpacked);
  for (const length of [0, 32, 4096, 12000, 20000, 65536, 8, 4096]) {
    const message = { text: (length % 3 ? "\0" : "x").repeat(length), values: [length, length === 0 ? 0 : -length] };
    const packed = codec.pencode("Data", message)!;
    assert.deepEqual(codec.pdecode("Data", packed), message);
    assert.deepEqual(codec.unpack(packed)!.subarray(0, codec.encode("Data", message)!.length), codec.encode("Data", message));
  }
  assert.deepEqual(first, original);
  assert.deepEqual(unpacked, snapshot);
  assert.throws(() => codec.pdecode("Data", Buffer.from([255])), TypeError);
  assert.deepEqual(codec.pdecode("Data", wire), original);
  assert.throws(() => codec.pdecode("Data", Buffer.alloc((8 << 20) / 8 + 1)), /size limit/);
  assert.deepEqual(codec.pdecode("Data", wire), original);
});

// 每连接 host 保留独立会话；一个客户端关闭不清理另一个的等待。
test("Sproto sessions stay independent and close clears pending", () => {
  const first = new SprotoEchoClient();
  const second = new SprotoEchoClient();
  const request1 = first.encode("first");
  const request2 = second.encode("second");
  const server = new Sproto(c2sSchema);
  const decoded1 = server.dispatch(Buffer.from(request1.frame));
  assert.ok(decoded1?.response);
  const reply1 = decoded1.response({ text: "first", code: 1 });
  const decoded2 = server.dispatch(Buffer.from(request2.frame));
  assert.ok(decoded2?.response);
  const reply2 = decoded2.response({ text: "second", code: 1 });
  first.close();
  assert.equal(second.decode(reply2).result.text, "second");
  assert.throws(() => first.decode(reply1), /decode|session/);
  second.close();
});

// 有界接纳和 packed 预检阻止无限等待与截断输入。
test("Sproto pending and packet limits", () => {
  const client = new SprotoEchoClient();
  for (let index = 0; index < 32; index++) client.encode("pending");
  assert.throws(() => client.encode("overflow"), /pending limit/);
  assert.equal(client.encode("notice", true).session, 0);
  assert.throws(() => client.decode(Uint8Array.of(255)), /truncated/);
  assert.throws(() => client.decode(new Uint8Array((1 << 20) + 1)), /size/);
  client.close();

  // 原生解包器必须完整统计输出长度，包括首次缓冲区放不下的 FF 连续块。
  const codec = new Sproto(c2sSchema);
  for (let mask = 0; mask < 256; mask++) {
    for (const value of [1, 0x7f, 0x80, 0xff]) {
      const raw = Buffer.alloc(8);
      const expected = mask === 255 ? [255, 0] : [mask];
      for (let index = 0; index < 8; index++) {
        if ((mask & (1 << index)) !== 0) { raw[index] = value; expected.push(value); }
      }
      assert.deepEqual(codec.pack(raw), Buffer.from(expected));
    }
  }
  // 批量扫描门槛附近及 FF 的 256-word 分段必须保持确定的官方 wire。
  for (const length of [32, 64, 128, 256, 1016, 1024, 1032, 2048, 2056, 4096]) {
    const raw = Buffer.alloc(length + 3, 0x80).subarray(3);
    const blocks: Buffer[] = [];
    for (let offset = 0; offset < length; offset += 2048) {
      const chunk = raw.subarray(offset, Math.min(offset + 2048, length));
      blocks.push(Buffer.from([255, chunk.length / 8 - 1]), chunk);
    }
    assert.deepEqual(codec.pack(raw), Buffer.concat(blocks), `FF length ${length}`);
  }
  for (const raw of [
    Buffer.alloc(4096),
    Buffer.concat([Buffer.alloc(4096), Buffer.alloc(2048, 1)]),
    Buffer.concat([Buffer.alloc(2048, 1), Buffer.alloc(4096)]),
    Buffer.concat([Buffer.alloc(4096), Buffer.from([1, 0, 0, 0, 0, 0, 0, 0]), Buffer.alloc(4096)]),
  ]) {
    assert.deepEqual(codec.unpack(codec.pack(raw)), raw);
  }
  // 缓冲不足仍计算完整长度，并写入可容纳的普通/零块前缀。
  const packed = Buffer.from([1, 7, 0, 0, 1, 9]);
  const expected = Buffer.alloc(32); expected[0] = 7; expected[24] = 9;
  for (const capacity of [0, 1, 7, 8, 9, 13, 25, 32]) {
    const prefix = Buffer.alloc(capacity, 0xcc);
    assert.equal(codec.lunpack(packed, prefix), expected.length);
    assert.deepEqual(prefix, expected.subarray(0, capacity));
  }
  for (const malformed of [[255], [255, 0, 1], [3, 1], [0, 0, 255]]) {
    assert.equal(codec.unpack(Buffer.from(malformed)), null);
  }
  // 大输入的计数路径同样拒绝末尾截断，恰好达到解包额度仍可接受。
  for (const prefix of [Buffer.alloc(2048), codec.pack(Buffer.alloc(2048, 1))]) {
    assert.equal(codec.unpack(Buffer.concat([prefix, Buffer.from([255])])), null);
  }
  const limit = codec.unpack(Buffer.alloc((8 << 20) / 8))!;
  assert.equal(limit.length, 8 << 20);
  assert.ok(limit.every(byte => byte === 0));
  assert.throws(() => codec.unpack(Buffer.alloc((8 << 20) / 8 + 1)), /size limit/);
});

const binary = process.env.GSR_SPROTO_SERVER_BIN;
for (const secure of [false, true]) {
  // 同一业务分别经 Plain 与 MAC 的真实 Go/TS 链路验证，不修改握手实现。
  test(`Go/TS Sproto Echo ${secure ? "MAC" : "Plain"}`, { skip: !binary, timeout: 20000 }, async (t) => {
    const server = spawn(resolve(binary!), ["--protocol", "sproto", "--addr", "127.0.0.1:0", ...(secure ? ["--prod"] : [])], { stdio: ["ignore", "pipe", "pipe"] });
    t.after(async () => {
      if (server.exitCode !== null || server.signalCode !== null) return;
      const stopped = once(server, "exit");
      server.kill("SIGTERM");
      await stopped;
    });
    let output = "";
    let diagnostics = "";
    server.stderr.on("data", chunk => { diagnostics += String(chunk); });
    const url = await new Promise<string>((resolveURL, reject) => {
      const timer = setTimeout(() => reject(new Error(`Go server start timeout: ${diagnostics}`)), 10000);
      server.stdout.on("data", chunk => {
        output += String(chunk);
        const match = output.match(/listening at (ws:\/\/[^;]+);/);
        if (match) { clearTimeout(timer); resolveURL(match[1]); }
      });
      server.once("error", error => { clearTimeout(timer); reject(error); });
      server.once("exit", code => { clearTimeout(timer); reject(new Error(`server exited ${code}: ${diagnostics}`)); });
    });
    try {
      // 大量非零字节会跨越连续 packing 块；在真实 TS→Go→TS 链路验证长度与内容。
      const texts = [
        "中文🙂",
        ...[2045, 2046, 2047, 2048, 2049, 4094, 4095, 4096].map(length => "x".repeat(length)),
        "\0".repeat(4096),
        "\0".repeat(2048) + "x".repeat(2046),
      ];
      for (const text of texts) {
        const response = await exchange(url, text, false, secure);
        assert.equal(response.replay, "RESPONSE");
        assert.equal(response.session, 1);
        assert.equal(response.result.code, 1);
        assert.equal(response.result.text, text, `Echo changed ${Buffer.byteLength(text)} bytes`);
      }
      const rejection = await exchange(url, "", false, secure);
      assert.equal(rejection.result.code, 2);
      assert.equal(rejection.result.message, "text is required");
      for (const text of ["notice", "x".repeat(4094), "\0".repeat(4096)]) {
        const push = await exchange(url, text, true, secure);
        assert.equal(push.replay, "REQUEST");
        assert.equal(push.name, "push");
        assert.equal(push.result.text, text, `Push changed ${Buffer.byteLength(text)} bytes`);
      }
      if (!secure) {
        const connection = new WebSocket(url);
        connection.binaryType = "arraybuffer";
        await waitForOpen(connection);
        const waiting = waitForFrame(connection);
        connection.send(Uint8Array.of(255));
        await assert.rejects(waiting, /closed/);
        connection.close();
      }
    } finally {
      const stopped = once(server, "exit");
      server.kill("SIGTERM");
      const [code] = await stopped;
      assert.equal(code, 0, diagnostics);
    }
  });
}
