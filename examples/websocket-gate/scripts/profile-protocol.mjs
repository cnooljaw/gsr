import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { cpus, loadavg } from "node:os";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const example = fileURLToPath(new URL("../", import.meta.url));
const root = resolve(example, "../..");
const output = resolve(process.argv[2] ?? resolve(root, "tmp/protocol-profile"));
mkdirSync(output, { recursive: true });
const command = (program, args, options = {}) => execFileSync(program, args, {
  cwd: root, encoding: "utf8", maxBuffer: 32 << 20, timeout: 60000, ...options,
});
const steps = ["protobuf_encode", "protobuf_decode", "sproto_encode", "sproto_decode", "sproto_pack", "sproto_unpack"];
const scenarios = ["addressbook", "game_large"];
const targets = scenarios.flatMap(scenario => steps.map(step => ({ scenario, step })));
const metadata = { date: new Date().toISOString(), cpu: cpus()[0]?.model, node: process.version,
  go: command("go", ["version"]).trim(), loadBefore: loadavg(),
  commit: command("git", ["rev-parse", "HEAD"]).trim(),
  secondsPerCPU: 2, goAllocationIterations: 1000, nodeHeapIntervalBytes: 16384,
  scenarios, steps, commands: [] };
const binary = resolve(output, "codec.test");
command("go", ["test", "-c", "./examples/websocket-gate/benchmarks", "-o", binary]);
const summaries = {};
for (const { scenario, step } of targets) {
  console.error(`采集 Go ${scenario}/${step} CPU 与分配（独立运行）…`);
  const directory = resolve(output, "go", scenario, step);
  mkdirSync(directory, { recursive: true });
  const base = ["-test.run=^$", `-test.bench=^BenchmarkRepresentativeCodec$/^${scenario}$/^${step}$`, "-test.count=1", "-test.timeout=30s"];
  const cpuArgs = [...base, "-test.benchtime=2s", `-test.cpuprofile=${resolve(directory, "cpu.pprof")}`];
  const allocationArgs = [...base, "-test.benchtime=1000x", "-test.memprofilerate=1", `-test.memprofile=${resolve(directory, "alloc.pprof")}`];
  const options = { cwd: resolve(example, "benchmarks"), env: { ...process.env, GOMAXPROCS: "1" } };
  metadata.commands.push([binary, ...cpuArgs], [binary, ...allocationArgs]);
  writeFileSync(resolve(directory, "cpu-run.txt"), command(binary, cpuArgs, options));
  writeFileSync(resolve(directory, "alloc-run.txt"), command(binary, allocationArgs, options));
  summaries[scenario] ??= {};
  summaries[scenario][step] = {};
  for (const [name, flags, profile] of [
    ["cpu_self", [], "cpu.pprof"], ["cpu_cumulative", ["-cum"], "cpu.pprof"],
    ["alloc_space", ["-alloc_space", "-focus=BenchmarkRepresentativeCodec.func1", "-relative_percentages"], "alloc.pprof"],
    ["alloc_objects", ["-alloc_objects", "-focus=BenchmarkRepresentativeCodec.func1", "-relative_percentages"], "alloc.pprof"],
  ]) {
    const text = command("go", ["tool", "pprof", "-top", "-nodecount=15", ...flags, binary, resolve(directory, profile)]);
    writeFileSync(resolve(directory, `${name}.txt`), text);
    summaries[scenario][step][name] = text;
  }
}

function frameKey(frame) {
  return `${frame.functionName || "(anonymous)"} @ ${(frame.url || "(native)").replace(root + "/", "")}:${frame.lineNumber + 1}`;
}

function summarizeNode(directory) {
  const cpu = JSON.parse(readFileSync(resolve(directory, "cpu.cpuprofile"), "utf8"));
  const nodes = new Map(cpu.nodes.map(node => [node.id, node]));
  const parents = new Map();
  for (const node of cpu.nodes) for (const child of node.children ?? []) parents.set(child, node.id);
  const weights = new Map();
  let total = 0;
  for (let index = 0; index < cpu.samples.length; index++) {
    const duration = cpu.timeDeltas[index];
    const id = cpu.samples[index];
    total += duration;
    const seen = new Set();
    for (let ancestor = id; ancestor !== undefined; ancestor = parents.get(ancestor)) {
      const key = frameKey(nodes.get(ancestor).callFrame);
      if (seen.has(key)) continue;
      seen.add(key);
      if (!weights.has(key)) weights.set(key, { frame: key, self: 0, cumulative: 0 });
      const row = weights.get(key);
      if (ancestor === id) row.self += duration;
      row.cumulative += duration;
    }
  }
  const cpuTop = [...weights.values()].sort((a, b) => b.self - a.self).slice(0, 20)
    .map(row => ({ frame: row.frame, selfPercent: row.self * 100 / total, cumulativePercent: row.cumulative * 100 / total }));
  const heap = JSON.parse(readFileSync(resolve(directory, "heap.heapprofile"), "utf8"));
  const heapRows = new Map();
  let heapTotal = 0;
  function visit(node, ancestors) {
    const key = frameKey(node.callFrame);
    const path = new Set([...ancestors, key]);
    heapTotal += node.selfSize;
    for (const ancestor of path) {
      if (!heapRows.has(ancestor)) heapRows.set(ancestor, { frame: ancestor, selfBytes: 0, cumulativeBytes: 0 });
      heapRows.get(ancestor).cumulativeBytes += node.selfSize;
    }
    heapRows.get(key).selfBytes += node.selfSize;
    for (const child of node.children ?? []) visit(child, path);
  }
  visit(heap.head, []);
  return { cpuSampleMicros: total, cpuTop, heapSampledEstimatedBytes: heapTotal,
    heapTop: [...heapRows.values()].sort((a, b) => b.selfBytes - a.selfBytes).slice(0, 20),
    metadata: JSON.parse(readFileSync(resolve(directory, "profile.json"), "utf8")) };
}

const nodeSummaries = {};
for (const { scenario, step } of targets) {
  console.error(`采集 TS ${scenario}/${step} CPU 与 V8 分配采样（分时段）…`);
  const directory = resolve(output, "typescript", scenario, step);
  const args = ["client/dist/representative_benchmark.js", "--mode", "profile", "--operation", `${scenario}/${step}`,
    "--seconds", "2", "--output", directory];
  metadata.commands.push([process.execPath, ...args]);
  command(process.execPath, args, { cwd: example });
  nodeSummaries[scenario] ??= {};
  nodeSummaries[scenario][step] = summarizeNode(directory);
}
metadata.loadAfter = loadavg();
writeFileSync(resolve(output, "environment.json"), JSON.stringify(metadata, null, 2) + "\n");
writeFileSync(resolve(output, "go-summary.json"), JSON.stringify(summaries, null, 2) + "\n");
writeFileSync(resolve(output, "typescript-summary.json"), JSON.stringify(nodeSummaries, null, 2) + "\n");
console.log(`CPU 与分配 profile 完成：${output}`);
