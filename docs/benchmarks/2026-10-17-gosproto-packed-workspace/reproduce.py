"""复现 EncodePacked 私有共享工作区的固定业务配对，不修改传入库。"""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import time


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


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
bound = {path: digest(path) for path in [here / "candidate.patch", here / "plan.json", here / "reproduce.py", tool, tool.with_name("sampler_test.go.txt")]}
(output / "driver-inputs.json").write_text(json.dumps({str(p.relative_to(root)): v for p, v in bound.items()}, indent=2) + "\n")
for name in plan["benchmark"]["runs"]:
    mode = name if name in {"self", "clone"} else "compare"
    args = ["python3", "-B", str(tool), "--library", str(options.library.resolve()),
            "--baseline", plan["baselineCommit"], "--mode", mode,
            "--block-ms", str(plan["benchmark"]["blockMs"]), "--output", str(output / name)]
    if mode == "compare":
        args += ["--candidate-patch", str(here / "candidate.patch")]
    if options.prepare_only:
        args += ["--prepare-only"]
    start = time.monotonic()
    result = subprocess.run(args, cwd=root, text=True, capture_output=True, timeout=110)
    commands.append({"args": args, "exitCode": result.returncode, "elapsedSeconds": round(time.monotonic() - start, 3),
                     "output": result.stdout + result.stderr})
    (output / "commands.json").write_text(json.dumps(commands, indent=2) + "\n")
    if result.returncode:
        raise SystemExit(result.stdout + result.stderr)
    for path, value in bound.items():
        if digest(path) != value:
            raise SystemExit("input changed during timing: " + str(path))
    print(name, "complete", flush=True)
