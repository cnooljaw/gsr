import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

// 本地优化依赖必须与已发布版本明确区分，哈希用于绑定实际参与测量的源码。
export function codecEnvironment(root) {
  const readModule = () => JSON.parse(execFileSync("go", ["list", "-m", "-json", "github.com/cnooljaw/gosproto"], {
    cwd: root, encoding: "utf8",
  }));
  let module = readModule();
  if (!module.Replace?.Dir && !module.Dir) {
    execFileSync("go", ["mod", "download", "github.com/cnooljaw/gosproto"], { cwd: root });
    module = readModule();
  }
  const directory = module.Replace?.Dir ?? module.Dir;
  const digest = path => createHash("sha256").update(readFileSync(path)).digest("hex");
  return {
    module, goFlags: process.env.GOFLAGS ?? "",
    goSource: Object.fromEntries(["encode.go", "decode.go", "meta.go", "limits.go", "pack.go"]
      .map(name => [name, digest(resolve(directory, name))])),
    tsSource: digest(resolve(root, "examples/websocket-gate/client/src/vendor/sproto.ts")),
  };
}
