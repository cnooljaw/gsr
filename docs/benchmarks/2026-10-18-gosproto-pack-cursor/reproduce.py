"""复现Pack输出游标的固定62项配对。"""

import argparse
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--library", type=Path, required=True)
parser.add_argument("--output", type=Path, required=True)
parser.add_argument("--prepare-only", action="store_true")
options = parser.parse_args()
here = Path(__file__).resolve().parent
root = here.parents[2]
tool = root / "examples/websocket-gate/benchmarks/tools/go-pairs/run.py"
spec = importlib.util.spec_from_file_location("pairs", tool)
pairs = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pairs)
plan = json.loads((here / "plan.json").read_text())
output = options.output.resolve()
output.mkdir(parents=True, exist_ok=False)
bound = {path: pairs.digest(path) for path in [here / "candidate.patch", here / "plan.json", here / "rpc-probe.go.txt", here / "joint-probe.go.txt", here / "patterns-probe.go.txt", here / "pack-differential.go.txt", here / "reproduce.py", tool, tool.with_name("sampler_test.go.txt")]}
(output / "driver-inputs.json").write_text(json.dumps({str(p.relative_to(root)): v for p, v in bound.items()}, indent=2) + "\n")
commands = []


def run(args, cwd, env=None, timeout=60):
    start = time.monotonic()
    try:
        result = subprocess.run(args, cwd=cwd, env={**os.environ, **(env or {})}, text=True, capture_output=True, timeout=timeout)
        record = {"args": args, "cwd": str(cwd), "env": env or {}, "exitCode": result.returncode,
                  "elapsedSeconds": round(time.monotonic() - start, 3), "output": result.stdout + result.stderr}
    except subprocess.TimeoutExpired as error:
        record = {"args": args, "cwd": str(cwd), "env": env or {}, "timeout": timeout,
                  "elapsedSeconds": round(time.monotonic() - start, 3), "output": str(error)}
        commands.append(record)
        (output / "commands.json").write_text(json.dumps(commands, indent=2) + "\n")
        raise
    commands.append(record)
    (output / "commands.json").write_text(json.dumps(commands, indent=2) + "\n")
    if result.returncode:
        raise SystemExit(record["output"])
    return result.stdout


def check_bound():
    for path, expected in bound.items():
        if pairs.digest(path) != expected:
            raise SystemExit("input changed: " + str(path))


for name in plan["measurement"]["modes"]:
    mode = name if name in {"self", "clone"} else "compare"
    directory = output / name
    args = ["python3", "-B", str(tool), "--library", str(options.library.resolve()), "--baseline", plan["baselineCommit"],
            "--mode", mode, "--output", str(directory), "--block-ms", str(plan["measurement"]["blockMs"]), "--prepare-only"]
    if mode == "compare":
        args += ["--candidate-patch", str(here / "candidate.patch")]
    run(args, root)
    harness = directory / "harness"
    sampler = harness / "sampler_test.go"
    sampler.write_text(sampler.read_text().replace("func pairOperations(", "func bodyPairOperations(", 1))
    (harness / "rpc_probe_test.go").write_text((here / "rpc-probe.go.txt").read_text().replace("func pairOperations(", "func rpcPairOperations(", 1))
    (harness / "joint_probe_test.go").write_bytes((here / "joint-probe.go.txt").read_bytes())
    (harness / "patterns_test.go").write_text((here / "patterns-probe.go.txt").read_text().replace("func pairOperations(", "func patternPairOperations(", 1))
    (harness / "pack_differential_test.go").write_bytes((here / "pack-differential.go.txt").read_bytes())
    run(["gofmt", "-w", "sampler_test.go", "rpc_probe_test.go", "joint_probe_test.go", "patterns_test.go", "pack_differential_test.go"], harness)
    manifest = json.loads((directory / "inputs.json").read_text())
    manifest["harnessGoSource"] = pairs.sources(harness)
    manifest["selectedCases"] = "62 operations:original43 body/RPC/wide64 plus19 existing Pack patterns;isolated harness only"
    manifest["probeSha256"] = pairs.digest(here / "rpc-probe.go.txt")
    (directory / "inputs.json").write_text(json.dumps(manifest, indent=2) + "\n")
    if options.prepare_only:
        print(name, "prepared", flush=True)
        continue
    run(["go", "test", "-mod=mod", "-run", "^(TestPairStatistics|TestPairSelection|TestPairGCCounter|TestRepresentativeFixtures)$", ".", "-timeout=1m"], harness)
    binary = directory / "rpc.test"
    run(["go", "test", "-mod=mod", "-c", "-o", str(binary), "."], harness)
    manifest["binarySha256"] = pairs.digest(binary)
    (directory / "inputs.json").write_text(json.dumps(manifest, indent=2) + "\n")
    run([str(binary), "-test.run=^TestPairSampling$", "-test.v", "-test.timeout=90s"], harness,
        {"GOMAXPROCS": "1", "GSR_PAIR_MODE": mode, "GSR_PAIR_OUTPUT": str(directory / "raw.json"),
         "GSR_PAIR_BLOCK_TARGET": str(plan["measurement"]["blockMs"]) + "ms", "GSR_PAIR_CASES": "", "GSR_PATTERN_NAMES": ",".join(plan["measurement"]["patterns"])}, timeout=95)
    for source, key in [(directory / "before", "beforeGoSource"), (directory / "candidate", "candidateGoSource"), (harness, "harnessGoSource")]:
        if pairs.sources(source) != manifest[key]:
            raise SystemExit("source changed: " + str(source))
    for path, expected in manifest["generatedGoInputs"].items():
        if pairs.digest(root / path) != expected:
            raise SystemExit("generated source changed: " + path)
    raw = json.loads((directory / "raw.json").read_text())
    if (raw["rounds"], raw["quartetsPerRound"], raw["targetBlockNs"], len(raw["results"])) != (6, 4, 10000000, 62):
        raise SystemExit("sampling incomplete")
    for result in raw["results"]:
        if len(result["quartets"]) != 24 or len(result["roundRatios"]) != 6 or (mode == "self" and not result["sameFunction"]):
            raise SystemExit("sampling incomplete or self function differs")
    (directory / "summary.json").write_text(json.dumps(pairs.summarize(raw), indent=2) + "\n")
    check_bound()
    print(name, "complete", flush=True)
