import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { copyFileSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const example = fileURLToPath(new URL("../", import.meta.url));
const vendor = resolve(example, "client/src/vendor");
const metadataPath = resolve(vendor, "sproto-source.json");
const digest = path => createHash("sha256").update(readFileSync(path)).digest("hex");

// 离线检查仅核对消费副本，无需本机存在 gosproto 检出目录。
if (process.argv[2] === "--check") {
  if (process.argv.length !== 3) throw new Error("usage: sync-sproto.mjs --check");
  const metadata = JSON.parse(readFileSync(metadataPath, "utf8"));
  for (const [name, sha256] of Object.entries(metadata.files)) {
    if (!["sproto.ts", "sproto.LICENSE"].includes(name)) throw new Error("invalid vendor file");
    if (digest(resolve(vendor, name)) !== sha256) throw new Error(`Sproto vendor differs: ${name}`);
  }
  if (!metadata.files["sproto.ts"] || !metadata.files["sproto.LICENSE"]) throw new Error("incomplete Sproto provenance");
  console.log("Sproto vendor 源码及许可证指纹通过");
} else {
  if (process.argv.length > 3) throw new Error("usage: sync-sproto.mjs [gosproto-directory]");
  const library = resolve(process.argv[2] ?? resolve(example, "../../../gosproto"));
  const git = args => execFileSync("git", ["-C", library, ...args], { encoding: "utf8" }).trim();
  const revision = git(["rev-parse", "HEAD"]);
  const status = git(["status", "--porcelain", "--", "typescript/src/sproto.ts", "typescript/LICENSE"]);
  const sources = { "sproto.ts": "typescript/src/sproto.ts", "sproto.LICENSE": "typescript/LICENSE" };
  const files = {};
  for (const [name, source] of Object.entries(sources)) {
    copyFileSync(resolve(library, source), resolve(vendor, name));
    files[name] = digest(resolve(vendor, name));
  }
  writeFileSync(metadataPath, JSON.stringify({
    repository: "https://github.com/cnooljaw/gosproto",
    repositoryHead: revision,
    sourcePaths: sources,
    sourceIncludesUncommittedChanges: status !== "",
    upstream: "sealindx/sproto-ts@bbee46fd76e2c9a7401ff9efd1dcadb538908130",
    files,
  }, null, 2) + "\n");
  console.log(`已从 gosproto ${revision} 同步${status ? "工作树快照" : "已提交源码"}`);
}
