import { readdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";

// Protobuf-ES 输出多一个末尾空行；统一收口以保持 git diff --check 干净。
async function normalize(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const path = join(directory, entry.name);
    if (entry.isDirectory()) {
      await normalize(path);
    } else if (entry.name.endsWith(".ts")) {
      const source = await readFile(path, "utf8");
      await writeFile(path, source.replace(/\n+$/, "\n"));
    }
  }
}

await normalize("examples/websocket-gate/client/src/gen");
