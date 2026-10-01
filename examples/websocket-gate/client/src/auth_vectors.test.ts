import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import {
  createHash,
  createHmac,
  createPrivateKey,
  createPublicKey,
  diffieHellman,
  timingSafeEqual,
} from "node:crypto";

interface ClientAuthVector {
  reference: string;
  client_private_key: string;
  server_private_key: string;
  client_public_key: string;
  server_public_key: string;
  shared_secret: string;
  salt_label: string;
  salt_input: string;
  salt: string;
  info: string;
  hkdf_prk: string;
  mac_key: string;
  seq: string;
  seq_bytes: string;
  envelope_bytes: string;
  hmac_sha256: string;
  mac: string;
}

const clientAuthVectorPath = fileURLToPath(
  new URL("../../../../gate/testdata/client-auth-vector.json", import.meta.url),
);
const privateKeyPrefix = Buffer.from("302e020100300506032b656e04220420", "hex");
const publicKeyPrefix = Buffer.from("302a300506032b656e032100", "hex");

function loadClientAuthVector(): ClientAuthVector {
  return JSON.parse(readFileSync(clientAuthVectorPath, "utf8")) as ClientAuthVector;
}

function privateKeyFromRaw(encoded: string) {
  return createPrivateKey({
    key: Buffer.concat([privateKeyPrefix, Buffer.from(encoded, "hex")]),
    format: "der",
    type: "pkcs8",
  });
}

function publicKeyFromRaw(encoded: string) {
  return createPublicKey({
    key: Buffer.concat([publicKeyPrefix, Buffer.from(encoded, "hex")]),
    format: "der",
    type: "spki",
  });
}

function publicKeyBytes(key: ReturnType<typeof privateKeyFromRaw>): Buffer {
  return createPublicKey(key)
    .export({ format: "der", type: "spki" })
    .subarray(-32);
}

function hmacSHA256(key: Uint8Array, message: Uint8Array): Buffer {
  return createHmac("sha256", key).update(message).digest();
}

function deriveMACKey(salt: Uint8Array, sharedSecret: Uint8Array, info: Uint8Array): {
  prk: Buffer;
  macKey: Buffer;
} {
  const prk = hmacSHA256(salt, sharedSecret);
  const macKey = hmacSHA256(prk, Buffer.concat([Buffer.from(info), Buffer.from([1])]));
  return { prk, macKey };
}

function verifyMAC(key: Uint8Array, seq: Uint8Array, envelope: Uint8Array, received: Uint8Array): boolean {
  if (received.byteLength !== 16) {
    return false;
  }
  const expected = hmacSHA256(key, Buffer.concat([Buffer.from(seq), Buffer.from(envelope)])).subarray(0, 16);
  return timingSafeEqual(expected, received);
}

test("跨语言固定向量逐阶段匹配 X25519、HKDF、seq 与 MAC 字节", () => {
  const vector = loadClientAuthVector();
  const clientPrivate = privateKeyFromRaw(vector.client_private_key);
  const serverPrivate = privateKeyFromRaw(vector.server_private_key);
  const clientPublic = publicKeyBytes(clientPrivate);
  const serverPublic = publicKeyBytes(serverPrivate);
  assert.deepEqual(clientPublic, Buffer.from(vector.client_public_key, "hex"));
  assert.deepEqual(serverPublic, Buffer.from(vector.server_public_key, "hex"));

  const sharedSecret = diffieHellman({
    privateKey: clientPrivate,
    publicKey: publicKeyFromRaw(vector.server_public_key),
  });
  assert.deepEqual(sharedSecret, Buffer.from(vector.shared_secret, "hex"));
  const reverseSharedSecret = diffieHellman({
    privateKey: serverPrivate,
    publicKey: publicKeyFromRaw(vector.client_public_key),
  });
  assert.deepEqual(reverseSharedSecret, sharedSecret);

  const saltInput = Buffer.concat([
    Buffer.from(vector.salt_label, "ascii"),
    clientPublic,
    serverPublic,
  ]);
  assert.deepEqual(saltInput, Buffer.from(vector.salt_input, "hex"));
  const salt = createHash("sha256").update(saltInput).digest();
  assert.deepEqual(salt, Buffer.from(vector.salt, "hex"));

  const { prk, macKey } = deriveMACKey(
    salt,
    sharedSecret,
    Buffer.from(vector.info, "ascii"),
  );
  assert.deepEqual(prk, Buffer.from(vector.hkdf_prk, "hex"));
  assert.deepEqual(macKey, Buffer.from(vector.mac_key, "hex"));

  const seqBytes = Buffer.alloc(8);
  seqBytes.writeBigUInt64BE(BigInt(`0x${vector.seq}`));
  assert.deepEqual(seqBytes, Buffer.from(vector.seq_bytes, "hex"));
  const envelope = Buffer.from(vector.envelope_bytes, "hex");
  const fullMAC = hmacSHA256(macKey, Buffer.concat([seqBytes, envelope]));
  assert.deepEqual(fullMAC, Buffer.from(vector.hmac_sha256, "hex"));
  assert.deepEqual(fullMAC.subarray(0, 16), Buffer.from(vector.mac, "hex"));
});

test("跨语言固定向量拒绝篡改输入、密钥上下文和错误 MAC 长度", () => {
  const vector = loadClientAuthVector();
  const key = Buffer.from(vector.mac_key, "hex");
  const seq = Buffer.from(vector.seq_bytes, "hex");
  const envelope = Buffer.from(vector.envelope_bytes, "hex");
  const mac = Buffer.from(vector.mac, "hex");
  assert.equal(verifyMAC(key, seq, envelope, mac), true);
  const changedMAC = Buffer.from(mac);
  changedMAC[0] ^= 0x01;
  assert.equal(verifyMAC(key, seq, envelope, changedMAC), false);

  const changedSeq = Buffer.from(seq);
  changedSeq[0] ^= 0x01;
  assert.equal(verifyMAC(key, changedSeq, envelope, mac), false);
  const changedEnvelope = Buffer.from(envelope);
  changedEnvelope[changedEnvelope.length - 1] ^= 0x01;
  assert.equal(verifyMAC(key, seq, changedEnvelope, mac), false);
  assert.equal(verifyMAC(key, seq, envelope, mac.subarray(0, 15)), false);
  assert.equal(verifyMAC(key, seq, envelope, Buffer.from(vector.hmac_sha256, "hex")), false);

  const sharedSecret = Buffer.from(vector.shared_secret, "hex");
  const salt = Buffer.from(vector.salt, "hex");
  const wrongContext = deriveMACKey(salt, sharedSecret, Buffer.from(`${vector.info}-wrong`, "ascii"));
  assert.equal(verifyMAC(wrongContext.macKey, seq, envelope, mac), false);

  const swappedSaltInput = Buffer.concat([
    Buffer.from(vector.salt_label, "ascii"),
    Buffer.from(vector.server_public_key, "hex"),
    Buffer.from(vector.client_public_key, "hex"),
  ]);
  const swappedSalt = createHash("sha256").update(swappedSaltInput).digest();
  const swappedContext = deriveMACKey(swappedSalt, sharedSecret, Buffer.from(vector.info, "ascii"));
  assert.equal(verifyMAC(swappedContext.macKey, seq, envelope, mac), false);
});
