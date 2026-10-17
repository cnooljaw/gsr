"""在隔离归档中只运行一次同函数墙钟/进程 CPU 诊断；不修改正式库。"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import statistics
import subprocess
import sys
import time


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def describe(ratios):
    quartiles = statistics.quantiles(ratios, n=4, method="inclusive")
    median = statistics.median(ratios)
    within = sum(abs(value - 1) <= 0.05 for value in ratios)
    return {
        "roundRatios": ratios, "medianRatio": median,
        "iqr": quartiles[2] - quartiles[0], "roundsWithin5Pct": within,
        "timingThresholdsMet": abs(median - 1) <= 0.02 and quartiles[2] - quartiles[0] <= 0.03 and within >= 5,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--library", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    options = parser.parse_args()
    if sys.platform != "darwin":
        parser.error("this archived diagnostic is macOS only")
    here = Path(__file__).resolve().parent
    root = here.parents[2]
    tool = root / "examples/websocket-gate/benchmarks/tools/go-pairs"
    plan = json.loads((here / "plan.json").read_text())
    expected = json.loads((root / "docs/benchmarks/2026-10-17-gosproto-pair-resolution/self/inputs.json").read_text())
    for name, value in expected["tools"].items():
        if digest(tool / name) != value:
            raise SystemExit("restore the historical tool version: " + name)
    for name, value in expected["generatedGoInputs"].items():
        if digest(root / name) != value:
            raise SystemExit("generated input differs: " + name)
    bench = tool.parents[1]
    for name, value in expected["fixtures"].items():
        if digest(bench / name) != value:
            raise SystemExit("fixture differs: " + name)
    for name in ["representative_benchmark_test.go", "protocol_benchmark_test.go"]:
        if digest(bench / name) != expected["harnessGoSource"][name]:
            raise SystemExit("harness input differs: " + name)
    output = options.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    commands = []

    def run(arguments, cwd, environment=None, expected_code=0, timeout=60):
        started = time.monotonic()
        process = subprocess.run(arguments, cwd=cwd, env={**os.environ, **(environment or {})},
                                 capture_output=True, text=True, timeout=timeout)
        commands.append({"args": arguments, "cwd": str(cwd), "env": environment or {},
                         "exitCode": process.returncode, "expectedExitCode": expected_code,
                         "elapsedSeconds": round(time.monotonic() - started, 3),
                         "output": process.stdout + process.stderr})
        (output / "commands.json").write_text(json.dumps(commands, ensure_ascii=False, indent=2) + "\n")
        if process.returncode != expected_code:
            raise RuntimeError(process.stdout + process.stderr)
        return process.stdout

    prepared = output / "prepared"
    run(["python3", str(tool / "run.py"), "--library", str(options.library.resolve()),
         "--baseline", plan["baseline"], "--output", str(prepared), "--mode", "self", "--prepare-only"], root)
    harness = prepared / "harness"
    probe = harness / "cpu_probe_test.go"
    shutil.copyfile(here / "cpu-probe.go.txt", probe)
    run(["gofmt", "-w", probe.name], harness)
    # 先证明用例能检出微秒误当纳秒；只在隔离副本做错误注入，恢复后才构建采样二进制。
    correct = probe.read_bytes()
    broken = correct.replace(b"int64(value.Usec)*int64(time.Microsecond)", b"int64(value.Usec)", 1)
    if correct == broken:
        raise RuntimeError("red-test injection did not apply")
    probe.write_bytes(broken)
    try:
        run(["go", "test", "-mod=mod", "-run", "^TestCPUProbeCounters$", ".", "-timeout=1m"], harness, expected_code=1)
    finally:
        probe.write_bytes(correct)
    run(["go", "test", "-mod=mod", "-run", "^(TestCPUProbeCounters|TestPairStatistics|TestPairSelection|TestPairGCCounter|TestRepresentativeFixtures)$", ".", "-timeout=1m"], harness)
    run(["go", "vet", "-mod=mod", "."], harness)
    binary = output / "cpu.test"
    run(["go", "test", "-mod=mod", "-c", "-o", str(binary), "."], harness)
    inputs = json.loads((prepared / "inputs.json").read_text())
    if inputs["candidateGoSource"] != expected["candidateGoSource"]:
        raise RuntimeError("library archive differs")
    bound = {path: digest(path) for folder in [prepared / "before", prepared / "candidate", harness]
             for path in folder.glob("*.go")}
    bound.update({here / name: digest(here / name) for name in ["plan.json", "cpu-probe.go.txt", "reproduce.py"]})
    bound.update({root / name: value for name, value in expected["generatedGoInputs"].items()})
    bound.update({harness / name: value for name, value in expected["fixtures"].items()})
    inputs["cpuProbe"] = {
        "plan": plan, "binarySha256": digest(binary),
        "archiveFilesSha256": {name: digest(here / name) for name in ["plan.json", "cpu-probe.go.txt", "reproduce.py"]},
        "harnessGoSource": {path.name: digest(path) for path in harness.glob("*.go")},
        "scope": "Process CPU includes runtime threads; observation window is wider than wall timing. No allocation measurement or optimization acceptance.",
    }
    (output / "inputs.json").write_text(json.dumps(inputs, indent=2) + "\n")
    run([str(binary), "-test.run=^TestCPUProbe$", "-test.v", "-test.timeout=90s"], harness,
        {"GOMAXPROCS": "1", "GSR_CPU_PROBE": str(output / "raw.json")}, timeout=95)
    for path, value in bound.items():
        if digest(path) != value:
            raise RuntimeError("input changed during measurement: " + str(path))
    raw = json.loads((output / "raw.json").read_text())
    if [value["name"] for value in raw["results"]] != plan["cases"]:
        raise RuntimeError("case selection differs")
    results = []
    for result in raw["results"]:
        if not result["sameFunction"] or len(result["quartets"]) != 24:
            raise RuntimeError("incomplete self control")
        blocks = [block for quartet in result["quartets"] for block in quartet["blocks"]]
        if len(blocks) != 96 or len(result["wallRoundRatios"]) != 6 or len(result["cpuRoundRatios"]) != 6:
            raise RuntimeError("incomplete sampling")
        cpu_fractions = [(block["userCpuNs"] + block["systemCpuNs"]) / block["wallNs"] for block in blocks]
        results.append({
            "name": result["name"], "wall": describe(result["wallRoundRatios"]),
            "processCpu": describe(result["cpuRoundRatios"]),
            "cpuOverWallMinMedianMax": [min(cpu_fractions), statistics.median(cpu_fractions), max(cpu_fractions)],
            "completedGCs": sum(block["completedGcCycles"] for block in blocks),
            "blocksWithCompletedGC": sum(block["completedGcCycles"] > 0 for block in blocks),
            "controlAccepted": None,
        })
    (output / "summary.json").write_text(json.dumps(results, indent=2) + "\n")


if __name__ == "__main__":
    main()
