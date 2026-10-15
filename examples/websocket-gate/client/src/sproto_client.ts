import { readFileSync } from "node:fs";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import process from "node:process";
import { Sproto } from "./vendor/sproto.js";
import { authenticatedFrame, completeClientHandshake, createClientHandshake } from "./secure_auth.js";

const schemaDirectory = resolve(dirname(fileURLToPath(import.meta.url)), "../../sproto");
const maxPacked = 1 << 20;
const maxUnpacked = 8 << 20;

// 在调用参考实现前检查完整 packed 边界和解包后的有限大小。
function validatePacked(frame: Uint8Array): void {
  if (frame.length === 0 || frame.length > maxPacked) throw new Error("invalid Sproto packed size");
  let offset = 0;
  let expanded = 0;
  while (offset < frame.length) {
    const tag = frame[offset++];
    if (tag === 255) {
      if (offset >= frame.length) throw new Error("truncated Sproto run");
      const length = (frame[offset++] + 1) * 8;
      if (offset + length > frame.length) throw new Error("truncated Sproto run");
      offset += length;
      expanded += length;
    } else {
      let count = 0;
      for (let mask = tag; mask !== 0; mask >>= 1) count += mask & 1;
      if (offset + count > frame.length) throw new Error("truncated Sproto word");
      offset += count;
      expanded += 8;
    }
    if (expanded > maxUnpacked) throw new Error("Sproto unpack size limit");
  }
}

// Echo schema 是平坦结构；在参考解码器前核对 record 边界与实际字段类型。
function inspectRecord(data: Buffer, start: number): { end: number; fields: Map<number, number | Buffer> } {
  if (start + 2 > data.length) throw new Error("truncated Sproto record");
  const count = data.readUInt16LE(start);
  if (count > 128) throw new Error("Sproto field limit");
  let end = start + 2 + count * 2;
  if (end > data.length) throw new Error("truncated Sproto header");
  let tag = 0;
  const fields = new Map<number, number | Buffer>();
  for (let index = 0; index < count; index++) {
    const field = data.readUInt16LE(start + 2 + index * 2);
    if (field % 2) { tag += (field + 1) / 2; continue; }
    if (tag > 65535) throw new Error("invalid Sproto field tag");
    if (field) fields.set(tag, field / 2 - 1);
    else {
      if (end + 4 > data.length) throw new Error("truncated Sproto chunk");
      const length = data.readUInt32LE(end);
      end += 4;
      if (length > data.length - end) throw new Error("truncated Sproto data");
      fields.set(tag, data.subarray(end, end + length));
      end += length;
    }
    tag++;
  }
  return { end, fields };
}

function integer(value: number | Buffer | undefined): number {
  if (typeof value === "number") return value;
  if (!value || (value.length !== 4 && value.length !== 8)) throw new Error("invalid Sproto integer");
  const decoded = value.length === 4 ? BigInt(value.readInt32LE()) : value.readBigInt64LE();
  if (decoded > BigInt(Number.MAX_SAFE_INTEGER) || decoded < BigInt(Number.MIN_SAFE_INTEGER)) throw new Error("Sproto integer exceeds safe range");
  return Number(decoded);
}

interface NativeMessage {
  replay: "REQUEST" | "RESPONSE";
  name?: string;
  session?: number;
  result: { text?: string; code?: number; message?: string };
}

// Echo 客户端每条连接独占 schema host 与有界会话表；不作为通用生产客户端 SDK。
export class SprotoEchoClient {
  private readonly host: Sproto;
  private readonly request: ReturnType<Sproto["attach"]>;
  private readonly pending = new Set<number>();
  private nextSession = 1;

  constructor() {
    this.host = new Sproto(readFileSync(resolve(schemaDirectory, "c2s.sproto"), "utf8"));
    this.host.host(new Sproto(readFileSync(resolve(schemaDirectory, "s2c.sproto"), "utf8")));
    this.request = this.host.attach();
  }

  encode(text: string, notify = false): { frame: Uint8Array; session: number } {
    if (Buffer.byteLength(text, "utf8") > 65536) throw new Error("Echo text size limit");
    if (!notify && this.pending.size >= 32) throw new Error("Sproto pending limit");
    if (!notify && this.nextSession > 0x7fff_ffff) throw new Error("Sproto session exhausted; reconnect");
    const session = notify ? 0 : this.nextSession++;
    const frame = this.request(notify ? "notify" : "echo", { text }, notify ? undefined : session);
    if (!frame) throw new Error("Sproto encode failed");
    validatePacked(frame);
    if (!notify) this.pending.add(session);
    return { frame, session };
  }

  decode(frame: Uint8Array): NativeMessage {
    validatePacked(frame);
    const unpacked = this.host.unpack(Buffer.from(frame));
    if (!unpacked) throw new Error("Sproto unpack failed");
    const header = inspectRecord(unpacked, 0);
    const isPush = header.fields.has(0);
    if (isPush) {
      if (integer(header.fields.get(0)) !== 1 || header.fields.has(1)) throw new Error("unexpected Sproto push");
    } else {
      const session = integer(header.fields.get(1));
      if (!this.pending.has(session)) throw new Error("unknown Sproto session");
    }
    const body = inspectRecord(unpacked, header.end);
    for (const tag of isPush ? [0] : [0, 2]) {
      const text = body.fields.get(tag);
      if (text !== undefined && (!(text instanceof Buffer) || text.length > 65536)) throw new Error("invalid Sproto text");
    }
    if (!isPush && body.fields.has(1)) integer(body.fields.get(1));
    if (unpacked.length - body.end > 7 || unpacked.subarray(body.end).some(value => value !== 0)) throw new Error("invalid Sproto padding");
    const message = this.host.dispatch(Buffer.from(frame)) as NativeMessage | undefined;
    if (!message) throw new Error("Sproto decode failed");
    if (message.replay === "RESPONSE") {
      if (!message.session || !this.pending.delete(message.session)) throw new Error("unknown Sproto session");
    } else if (message.name !== "push") {
      throw new Error("unexpected Sproto server protocol");
    }
    return message;
  }

  // timeout、发送失败与断线均由连接 owner 结束等待，不复用原 session。
  close(): void {
    for (const session of this.pending) delete this.host.__session[session];
    this.pending.clear();
  }
}

export function waitForOpen(connection: WebSocket, timeoutMs = 5000): Promise<void> {
  return new Promise((resolvePromise, reject) => {
    const finish = (error?: Error) => {
      clearTimeout(timer);
      connection.removeEventListener("open", opened);
      connection.removeEventListener("error", failed);
      connection.removeEventListener("close", failed);
      if (error) reject(error); else resolvePromise();
    };
    const opened = () => finish();
    const failed = () => finish(new Error("WebSocket open failed"));
    const timer = setTimeout(() => finish(new Error("WebSocket open timeout")), timeoutMs);
    connection.addEventListener("open", opened, { once: true });
    connection.addEventListener("error", failed, { once: true });
    connection.addEventListener("close", failed, { once: true });
  });
}

export function waitForFrame(connection: WebSocket, timeoutMs = 5000): Promise<Uint8Array> {
  return new Promise((resolvePromise, reject) => {
    const finish = (error?: Error, frame?: Uint8Array) => {
      clearTimeout(timer);
      connection.removeEventListener("message", message);
      connection.removeEventListener("error", failed);
      connection.removeEventListener("close", failed);
      if (error) reject(error); else resolvePromise(frame!);
    };
    const message = (event: MessageEvent) => {
      if (!(event.data instanceof ArrayBuffer)) { finish(new Error("expected binary frame")); return; }
      finish(undefined, new Uint8Array(event.data));
    };
    const failed = () => finish(new Error("WebSocket closed while awaiting Sproto response"));
    const timer = setTimeout(() => finish(new Error("Sproto response timeout")), timeoutMs);
    connection.addEventListener("message", message, { once: true });
    connection.addEventListener("error", failed, { once: true });
    connection.addEventListener("close", failed, { once: true });
  });
}

export async function exchange(url: string, text: string, notify: boolean, secure: boolean): Promise<NativeMessage> {
  const connection = new WebSocket(url);
  connection.binaryType = "arraybuffer";
  const client = new SprotoEchoClient();
  try {
    await waitForOpen(connection);
    let authenticator;
    if (secure) {
      const handshake = createClientHandshake();
      const received = waitForFrame(connection);
      connection.send(handshake.frame);
      authenticator = completeClientHandshake(handshake, await received);
    }
    const request = client.encode(text, notify);
    const received = waitForFrame(connection);
    connection.send(authenticator ? authenticatedFrame(authenticator, request.frame) : request.frame);
    const message = client.decode(await received);
    if (!notify && message.session !== request.session) throw new Error("response session mismatch");
    return message;
  } finally {
    client.close();
    connection.close();
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  const value = (flag: string, fallback: string) => {
    const index = args.indexOf(flag);
    return index >= 0 ? args[index + 1] : fallback;
  };
  exchange(value("--url", "ws://127.0.0.1:7888/ws"), value("--text", "hello from TypeScript"), args.includes("--notify"), args.includes("--secure"))
    .then(message => {
      if (message.replay === "RESPONSE") console.log(`session=${message.session} code=${message.result.code} echo=${message.result.text ?? ""} message=${message.result.message ?? ""}`);
      else console.log(`push=${message.result.text}`);
    }).catch(error => { console.error(error); process.exitCode = 1; });
}
