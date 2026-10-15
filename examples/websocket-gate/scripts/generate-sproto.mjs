import { readFileSync, writeFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";

// 复用现有 sprotodump，规范模块与协议名称，剔除非协议的 Go 方法绑定元数据。
const root = resolve(dirname(fileURLToPath(import.meta.url)), "../../..");
const tool = process.env.SPROTODUMP_DIR;
if (!tool) throw new Error("set SPROTODUMP_DIR to the pinned lvzixun/sprotodump checkout");
for (const name of ["c2s", "s2c"]) {
  const output = resolve(root, `examples/websocket-gate/gen/sproto/${name}/${name}.go`);
  const source = `examples/websocket-gate/sproto/${name}.sproto`;
  const result = spawnSync("lua", [resolve(tool, "sprotodump.lua"), "-go", resolve(root, source), "-o", output, "-p", name], { cwd: tool, stdio: "inherit" });
  if (result.status !== 0) throw new Error("sprotodump failed");
  let generated = readFileSync(output, "utf8");
  generated = generated.replaceAll("github.com/xjdrew/gosproto", "github.com/cnooljaw/gosproto")
    .replace(/^\/\/ source:.*$/m, `// source: ${source}`)
    .replace(/^[\t ]*MethodName:[^\n]*\n/gm, "")
    .replace(/(Name:\s+)"[^".]+\./g, '$1"');
  writeFileSync(output, generated);
  const formatted = spawnSync("gofmt", ["-w", output], { stdio: "inherit" });
  if (formatted.status !== 0) throw new Error("gofmt failed");
}
