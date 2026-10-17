"""按固定顺序复核两个 Pack 候选及组合，不使用最低收益百分比。"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import time


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--library", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--prepare-only", action="store_true")
    options = parser.parse_args()
    here = Path(__file__).resolve().parent
    root = here.parents[2]
    tool = root / "examples/websocket-gate/benchmarks/tools/go-pairs/run.py"
    plan = json.loads((here / "plan.json").read_text())
    output = options.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    commands = []

    def run(args, cwd=root):
        started = time.monotonic()
        result = subprocess.run(args, cwd=cwd, capture_output=True, text=True, timeout=110)
        commands.append({"args": args, "cwd": str(cwd), "exitCode": result.returncode,
                         "elapsedSeconds": round(time.monotonic() - started, 3),
                         "output": result.stdout + result.stderr})
        (output / "commands.json").write_text(json.dumps(commands, indent=2) + "\n")
        if result.returncode:
            raise RuntimeError(result.stdout + result.stderr)

    bound = {path: digest(path) for path in [here / "plan.json", here / "reproduce.py", tool,
             tool.with_name("sampler_test.go.txt"), *sorted((here / "patches").glob("*.patch"))]}
    (output / "driver-inputs.json").write_text(json.dumps({str(p.relative_to(root)): v for p, v in bound.items()}, indent=2) + "\n")
    # 三种变体各验证一次原有边界和差分，避免计时之后才发现 wire 不同。
    for variant in ["sparse", "ff-runs", "combined"]:
        check = output / ("check-" + variant)
        run(["python3", "-B", str(tool), "--library", str(options.library.resolve()),
             "--baseline", plan["baselineCommit"], "--mode", "compare", "--candidate-patch",
             str(here / "patches" / (variant + ".patch")), "--output", str(check), "--prepare-only"])
        shutil.copyfile(root / "docs/benchmarks/2026-10-17-gosproto-pack-runs/pack-differential.go.txt", check / "harness/pack_differential_test.go")
        if not options.prepare_only:
            run(["go", "test", "-mod=mod", "-run", "^(TestPack|TestPackZeroRuns|TestPackPartialRunAtBlockLimit|TestUnpackOutputBudget)$", ".", "-timeout=1m"], check / "candidate")
            run(["go", "test", "-mod=mod", "-run", "^(TestPackDifferential|TestRepresentativeFixtures)$", "-v", ".", "-timeout=1m"], check / "harness")
    for item in plan["runs"]:
        args = ["python3", "-B", str(tool), "--library", str(options.library.resolve()),
                "--baseline", plan["baselineCommit"], "--mode", item["mode"],
                "--block-ms", str(plan["blockMs"]), "--output", str(output / item["name"])]
        if "patch" in item:
            args += ["--candidate-patch", str(here / "patches" / (item["patch"] + ".patch"))]
        if options.prepare_only:
            args += ["--prepare-only"]
        run(args)
        for path, value in bound.items():
            if digest(path) != value:
                raise RuntimeError("measurement input changed: " + str(path))
        print(item["name"], "complete", flush=True)


if __name__ == "__main__":
    main()
