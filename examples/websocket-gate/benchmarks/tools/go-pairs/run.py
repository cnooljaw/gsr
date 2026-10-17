"""在归档副本中构建并运行 Go 编码配对诊断，不修改正式依赖或库工作树。"""

import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import statistics
import subprocess
import tarfile
import time


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def sources(directory):
    return {path.name: digest(path) for path in sorted(directory.glob("*.go"))}


def summarize(raw):
    summary = []
    for result in raw["results"]:
        ratios = result["roundRatios"]
        quartiles = statistics.quantiles(ratios, n=4, method="inclusive")
        median = statistics.median(ratios)
        within = sum(abs(value - 1) <= 0.05 for value in ratios)
        allocation = {}
        for arm in "AB":
            values = [value for value in result["allocations"] if value["arm"] == arm]
            allocation[arm] = {
                key: statistics.median(value[key] / value["calls"] for value in values)
                for key in ["bytes", "mallocs"]
            }
        same_allocation = allocation["A"] == allocation["B"]
        accepted = (
            abs(median - 1) <= 0.02
            and quartiles[2] - quartiles[0] <= 0.03
            and within >= 5
            and same_allocation
        )
        summary.append({
            "name": result["name"],
            "medianRatio": median,
            "ratioIQR": quartiles[2] - quartiles[0],
            "roundsWithin5Pct": within,
            "allocationMedian": allocation,
            "sameAllocation": same_allocation,
            "controlAccepted": accepted if raw["mode"] != "compare" else None,
            "roundRatios": ratios,
        })
    return summary


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--library", required=True, type=Path)
    parser.add_argument("--baseline", required=True)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--mode", choices=["self", "clone", "compare"], default="self")
    parser.add_argument("--candidate-patch", type=Path)
    parser.add_argument("--prepare-only", action="store_true")
    options = parser.parse_args()
    if (options.mode == "compare") != bool(options.candidate_patch):
        parser.error("compare requires --candidate-patch; self/clone must use unchanged archives")
    library = options.library.resolve()
    output = options.output.resolve()
    if output.exists():
        parser.error("output already exists; choose a new directory")
    output.mkdir(parents=True)
    tool = Path(__file__).resolve().parent
    root = tool.parents[4]
    bench = tool.parents[1]
    commands = []

    def run(arguments, cwd, environment=None, timeout=60):
        start = time.monotonic()
        process = subprocess.run(
            arguments, cwd=cwd, env={**os.environ, **(environment or {})},
            capture_output=True, text=True, timeout=timeout,
        )
        commands.append({
            "args": arguments, "cwd": str(cwd), "env": environment or {},
            "elapsedSeconds": round(time.monotonic() - start, 3),
            "exitCode": process.returncode, "output": process.stdout + process.stderr,
        })
        (output / "commands.json").write_text(json.dumps(commands, ensure_ascii=False, indent=2) + "\n")
        if process.returncode:
            raise RuntimeError(process.stdout + process.stderr)
        return process.stdout

    baseline = run(["git", "rev-parse", "--verify", options.baseline + "^{commit}"], library).strip()
    archive = subprocess.check_output(["git", "archive", baseline], cwd=library)
    before, candidate, harness = output / "before", output / "candidate", output / "harness"
    for directory in [before, candidate]:
        directory.mkdir()
        with tarfile.open(fileobj=io.BytesIO(archive)) as contents:
            contents.extractall(directory, filter="data")
    module = before / "go.mod"
    module.write_text(module.read_text().replace(
        "module github.com/cnooljaw/gosproto", "module github.com/cnooljaw/gosproto-before", 1,
    ))
    if options.candidate_patch:
        run(["patch", "--batch", "-p1", "-i", str(options.candidate_patch.resolve())], candidate)
    harness.mkdir()
    (harness / "go.mod").write_text(f"""module codecpairstool

go 1.24.0

require (
 github.com/cnooljaw/gsr v0.0.0
 github.com/cnooljaw/gosproto v0.4.0
 github.com/cnooljaw/gosproto-before v0.0.0
 google.golang.org/protobuf v1.36.1
)
replace github.com/cnooljaw/gsr => {root}
replace github.com/cnooljaw/gosproto => {candidate}
replace github.com/cnooljaw/gosproto-before => {before}
""")
    (harness / "go.sum").write_bytes((root / "go.sum").read_bytes())
    for name in ["representative_benchmark_test.go", "protocol_benchmark_test.go", "representative.json", "cases.json", "correctness.json"]:
        (harness / name).write_bytes((bench / name).read_bytes())
    factory = (bench / "protocol_benchmark_test.go").read_text()
    factory = factory[factory.index("func echoCodecs("):]
    factory = factory.replace("func echoCodecs(", "func beforeEchoCodecs(", 1).replace("sproto.", "before.")
    (harness / "before_echo_test.go").write_text('''package benchmarks
import (
 before "github.com/cnooljaw/gosproto-before"
 echopb "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/example/echo/v1"
 "github.com/cnooljaw/gsr/examples/websocket-gate/gen/sproto/c2s"
 commonpb "github.com/cnooljaw/gsr/protocol/gen/go/gsr/common/v1"
 "google.golang.org/protobuf/proto"
)
''' + factory)
    (harness / "sampler_test.go").write_bytes((tool / "sampler_test.go.txt").read_bytes())
    run(["gofmt", "-w", "before_echo_test.go", "sampler_test.go"], harness)
    manifest = {
        "baselineCommit": baseline, "mode": options.mode,
        "goVersion": run(["go", "version"], root).strip(),
        "beforeGoSource": sources(before), "candidateGoSource": sources(candidate),
        "harnessGoSource": sources(harness),
        "fixtures": {name: digest(harness / name) for name in ["representative.json", "cases.json", "correctness.json"]},
        "tools": {name: digest(tool / name) for name in ["run.py", "sampler_test.go.txt"]},
        "candidatePatchSha256": digest(options.candidate_patch) if options.candidate_patch else None,
        "controlGate": "abs(median ratio - 1) <= 2%; round ratio IQR <= 3 percentage points; at least 5/6 rounds within 5%; allocation medians equal",
        "parameters": {"rounds": 6, "quartetsPerRound": 4, "targetBlockMs": 10, "warmupMs": 50, "gcEnabled": True},
        "scope": "Controls calibrate this case/run only; compare has no automatic adoption decision. Median/IQR are descriptive, not a confidence interval.",
    }
    if options.mode != "compare" and manifest["beforeGoSource"] != manifest["candidateGoSource"]:
        raise RuntimeError("control archives differ")
    (output / "inputs.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    if options.prepare_only:
        return
    run(["go", "test", "-mod=mod", "-run", "^(TestPairStatistics|TestRepresentativeFixtures)$", ".", "-timeout=1m"], harness)
    binary = output / "pairs.test"
    run(["go", "test", "-mod=mod", "-c", "-o", str(binary), "."], harness)
    manifest["binarySha256"] = digest(binary)
    (output / "inputs.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    run([str(binary), "-test.run=^TestPairSampling$", "-test.v", "-test.timeout=90s"], harness, {
        "GOMAXPROCS": "1", "GSR_PAIR_MODE": options.mode,
        "GSR_PAIR_OUTPUT": str(output / "raw.json"),
    }, timeout=95)
    for directory, key in [(before, "beforeGoSource"), (candidate, "candidateGoSource"), (harness, "harnessGoSource")]:
        if sources(directory) != manifest[key]:
            raise RuntimeError("source changed during sampling")
    raw = json.loads((output / "raw.json").read_text())
    if len(raw["results"]) != 19:
        raise RuntimeError("sampling incomplete")
    for result in raw["results"]:
        if len(result["quartets"]) != 24 or len(result["roundRatios"]) != 6:
            raise RuntimeError("sampling incomplete")
        if options.mode == "self" and not result["sameFunction"]:
            raise RuntimeError("self call target differs")
    summary = summarize(raw)
    (output / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
    for item in summary:
        print(f"{item['name']}: {(item['medianRatio'] - 1) * 100:+.2f}%; IQR {item['ratioIQR'] * 100:.2f}pp; control={item['controlAccepted']}", flush=True)


if __name__ == "__main__":
    main()
