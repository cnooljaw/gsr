import assert from "node:assert/strict";
import { test } from "node:test";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { resolve } from "node:path";
import { readFileSync } from "node:fs";
import { Sproto } from "./vendor/sproto.js";
import { exchange, SprotoEchoClient, waitForOpen, waitForFrame } from "./sproto_client.js";

const c2sSchema = readFileSync(new URL("../../sproto/c2s.sproto", import.meta.url), "utf8");

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
