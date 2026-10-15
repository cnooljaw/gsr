import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const example = fileURLToPath(new URL("../", import.meta.url));
const root = resolve(example, "../..");
const source = "examples/websocket-gate/benchmarks/schema/representative";
const tool = process.env.SPROTODUMP_DIR;
if (!tool) throw new Error("set SPROTODUMP_DIR to the pinned lvzixun/sprotodump checkout");
const goOutput = resolve(example, "benchmarks/gen/sproto/representative.go");
mkdirSync(resolve(example, "benchmarks/gen/sproto"), { recursive: true });
mkdirSync(resolve(example, "client/src/gen/benchmark"), { recursive: true });
execFileSync("protoc", [
  "--proto_path=examples/websocket-gate/benchmarks/schema",
  "--go_out=.", "--go_opt=module=github.com/cnooljaw/gsr",
  `--plugin=protoc-gen-es=${resolve(example, "node_modules/.bin/protoc-gen-es")}`,
  "--es_out=examples/websocket-gate/client/src/gen/benchmark",
  "--es_opt=target=ts,import_extension=js", `${source}.proto`,
], { cwd: root, stdio: "inherit" });
const tsOutput = resolve(example, "client/src/gen/benchmark/representative_pb.ts");
writeFileSync(tsOutput, readFileSync(tsOutput, "utf8").trimEnd() + "\n");
execFileSync("lua", [resolve(tool, "sprotodump.lua"), "-go", resolve(root, `${source}.sproto`),
  "-o", goOutput, "-p", "benchsp"], { cwd: tool, stdio: "inherit" });
const generated = readFileSync(goOutput, "utf8")
  .replaceAll("github.com/xjdrew/gosproto", "github.com/cnooljaw/gosproto")
  .replace(/^\/\/ source:.*$/m, `// source: ${source}.sproto`)
  .replace(/^[\t ]*MethodName:[^\n]*\n/gm, "");
writeFileSync(goOutput, generated);
execFileSync("gofmt", ["-w", goOutput], { stdio: "inherit" });
