import assert from "node:assert/strict";
import {
  createHash,
  createHmac,
  createPrivateKey,
  createPublicKey,
  diffieHellman,
} from "node:crypto";
import { writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

// RFC 7748 §6.1 的公开测试密钥，只能用于生成固定测试向量。
const clientPrivateHex = "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a";
const serverPrivateHex = "5dab087e624a8a4b79e17f8b83800ee66f3bb1292618b6fd1c2f8b27ff88e0eb";
const clientPublicRFC = "8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a";
const serverPublicRFC = "de9edb7d7b7dc1b4d35b61c2ece435373f8343c85b78674dadfc7e146f882b4f";
const sharedSecretRFC = "4a5d9d5ba4ce2de1728e3bf480350f25e07e21c947d19e3376f09b3c1e161742";

const privateKeyPrefix = Buffer.from("302e020100300506032b656e04220420", "hex");
const publicKeyPrefix = Buffer.from("302a300506032b656e032100", "hex");
const privateKeyFromRaw = (hex) => createPrivateKey({
  key: Buffer.concat([privateKeyPrefix, Buffer.from(hex, "hex")]),
  format: "der",
  type: "pkcs8",
});
const publicKeyFromRaw = (hex) => createPublicKey({
  key: Buffer.concat([publicKeyPrefix, Buffer.from(hex, "hex")]),
  format: "der",
  type: "spki",
});
const hmacSHA256 = (key, data) => createHmac("sha256", key).update(data).digest();

const clientPrivateKey = privateKeyFromRaw(clientPrivateHex);
const serverPrivateKey = privateKeyFromRaw(serverPrivateHex);
const clientPublicKey = createPublicKey(clientPrivateKey).export({ format: "der", type: "spki" }).subarray(-32);
const serverPublicKey = createPublicKey(serverPrivateKey).export({ format: "der", type: "spki" }).subarray(-32);
assert.equal(clientPublicKey.toString("hex"), clientPublicRFC);
assert.equal(serverPublicKey.toString("hex"), serverPublicRFC);

const sharedSecret = diffieHellman({
  privateKey: clientPrivateKey,
  publicKey: publicKeyFromRaw(serverPublicRFC),
});
assert.equal(sharedSecret.toString("hex"), sharedSecretRFC);

const saltLabel = "gsr/wss/hkdf-salt/v1";
const info = "gsr/wss/c2s-hmac-sha256/v1";
const saltInput = Buffer.concat([
  Buffer.from(saltLabel, "ascii"),
  clientPublicKey,
  serverPublicKey,
]);
const salt = createHash("sha256").update(saltInput).digest();
const hkdfPRK = hmacSHA256(salt, sharedSecret);
const macKey = hmacSHA256(hkdfPRK, Buffer.concat([Buffer.from(info, "ascii"), Buffer.from([1])]));
const seq = "fedcba9876543210";
const seqBytes = Buffer.from(seq, "hex");
const envelopeBytes = Buffer.from("08e90710ac021a030a01ff", "hex");
const fullMAC = hmacSHA256(macKey, Buffer.concat([seqBytes, envelopeBytes]));

const vector = {
  reference: "RFC 7748 section 6.1 Alice/Bob X25519 vector; HKDF and MAC derived independently by this reference generator.",
  client_private_key: clientPrivateHex,
  server_private_key: serverPrivateHex,
  client_public_key: clientPublicKey.toString("hex"),
  server_public_key: serverPublicKey.toString("hex"),
  shared_secret: sharedSecret.toString("hex"),
  salt_label: saltLabel,
  salt_input: saltInput.toString("hex"),
  salt: salt.toString("hex"),
  info,
  hkdf_prk: hkdfPRK.toString("hex"),
  mac_key: macKey.toString("hex"),
  seq,
  seq_bytes: seqBytes.toString("hex"),
  envelope_bytes: envelopeBytes.toString("hex"),
  hmac_sha256: fullMAC.toString("hex"),
  mac: fullMAC.subarray(0, 16).toString("hex"),
};

writeFileSync(
  fileURLToPath(new URL("./client-auth-vector.json", import.meta.url)),
  `${JSON.stringify(vector, null, 2)}\n`,
);
