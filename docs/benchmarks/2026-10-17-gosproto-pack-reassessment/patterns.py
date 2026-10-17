"""复用原 ABBA/BAAB 采样与合成输入，检查固定 19 种 Pack 形态。"""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import time


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--business", type=Path, required=True)
parser.add_argument("--output", type=Path, required=True)
options = parser.parse_args()
here = Path(__file__).resolve().parent
root = here.parents[2]
tool = root / "examples/websocket-gate/benchmarks/tools/go-pairs"
plan = json.loads((here / "plan.json").read_text())
output = options.output.resolve()
output.mkdir(parents=True, exist_ok=False)
commands = []


def run(args, cwd, environment=None):
    start = time.monotonic()
    result = subprocess.run(args, cwd=cwd, env={**os.environ, **(environment or {})},
                            text=True, capture_output=True, timeout=95)
    commands.append({"args": args, "cwd": str(cwd), "env": environment or {},
                     "exitCode": result.returncode, "elapsedSeconds": round(time.monotonic() - start, 3),
                     "output": result.stdout + result.stderr})
    (output / "commands.json").write_text(json.dumps(commands, indent=2) + "\n")
    if result.returncode:
        raise RuntimeError(result.stdout + result.stderr)


spec = importlib.util.spec_from_file_location("pair_summary", tool / "run.py")
summary_tool = importlib.util.module_from_spec(spec)
spec.loader.exec_module(summary_tool)
for variant in ["sparse", "ff-runs", "combined"]:
    check = options.business.resolve() / ("check-" + variant)
    directory = output / variant
    directory.mkdir()
    harness = directory / "harness"
    shutil.copytree(check / "harness", harness)
    sampler = harness / "sampler_test.go"
    sampler.write_text(sampler.read_text().replace("func pairOperations(", "func businessPairOperations(", 1))
    shutil.copyfile(here / "patterns-probe.go.txt", harness / "patterns_test.go")
    run(["gofmt", "-w", "sampler_test.go", "patterns_test.go"], harness)
    run(["go", "test", "-mod=mod", "-run", "^(TestPairStatistics|TestPairSelection|TestPairGCCounter|TestRepresentativeFixtures)$", ".", "-timeout=1m"], harness)
    binary = directory / "patterns.test"
    run(["go", "test", "-mod=mod", "-c", "-o", str(binary), "."], harness)
    bound = {path: digest(path) for folder in [harness, check / "before", check / "candidate"] for path in folder.glob("*.go")}
    bound.update({here / name: digest(here / name) for name in ["plan.json", "patterns.py", "patterns-probe.go.txt"]})
    inputs = {"variant": variant, "businessInputs": json.loads((check / "inputs.json").read_text()),
              "probeSha256": digest(here / "patterns-probe.go.txt"), "runnerSha256": digest(here / "patterns.py"),
              "harnessGoSource": {path.name: digest(path) for path in harness.glob("*.go")},
              "patterns": plan["patterns"], "binarySha256": digest(binary), "targetBlockMs": 10}
    (directory / "inputs.json").write_text(json.dumps(inputs, indent=2) + "\n")
    run([str(binary), "-test.run=^TestPairSampling$", "-test.v", "-test.timeout=90s"], harness,
        {"GOMAXPROCS": "1", "GSR_PAIR_MODE": "compare", "GSR_PAIR_OUTPUT": str(directory / "raw.json"),
         "GSR_PAIR_BLOCK_TARGET": "10ms", "GSR_PATTERN_NAMES": ",".join(plan["patterns"]), "GSR_PAIR_CASES": ""})
    for path, value in bound.items():
        if digest(path) != value:
            raise RuntimeError("source changed during pattern timing: " + str(path))
    raw = json.loads((directory / "raw.json").read_text())
    if {row["name"] for row in raw["results"]} != set(plan["patterns"]):
        raise RuntimeError("pattern selection differs")
    if sum(len(q["blocks"]) for row in raw["results"] for q in row["quartets"]) != 19 * 96:
        raise RuntimeError("pattern sampling incomplete")
    (directory / "summary.json").write_text(json.dumps(summary_tool.summarize(raw), indent=2) + "\n")
    print(variant, "patterns complete", flush=True)
