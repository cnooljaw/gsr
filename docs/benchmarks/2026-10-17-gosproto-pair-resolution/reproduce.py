"""核对固定输入后重跑聚焦校准与 tiny 分配探针，不修改正式库。"""

import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import os

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--library", type=Path, required=True)
parser.add_argument("--output", type=Path, required=True)
options = parser.parse_args()
here = Path(__file__).resolve().parent
root = here.parents[2]
tool = root / "examples/websocket-gate/benchmarks/tools/go-pairs"
expected = json.loads((here / "self/inputs.json").read_text())
sha = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
for name, digest in expected["tools"].items():
    if sha(tool / name) != digest:
        raise SystemExit("工具已变化，请恢复本报告所在提交的对应工具")
for name, digest in expected["generatedGoInputs"].items():
    if sha(root / name) != digest:
        raise SystemExit("生成类型已变化：" + name)
bench = tool.parents[1]
for name, digest in expected["fixtures"].items():
    if sha(bench / name) != digest:
        raise SystemExit("夹具已变化：" + name)
for name in ["representative_benchmark_test.go", "protocol_benchmark_test.go"]:
    if sha(bench / name) != expected["harnessGoSource"][name]:
        raise SystemExit("夹具代码已变化：" + name)
plan = json.loads((here / "plan.json").read_text())
output = options.output.resolve()
output.mkdir(parents=True, exist_ok=False)
for mode in plan["modes"]:
    command = [
        "python3", str(tool / "run.py"), "--library", str(options.library.resolve()),
        "--baseline", expected["baselineCommit"], "--output", str(output / mode),
        "--mode", mode, "--block-ms", str(plan["blockMs"]),
    ]
    for name in plan["cases"]:
        command += ["--case", name]
    subprocess.run(command, check=True)
probe = output / "probe"
shutil.copytree(output / "self/harness", probe)
shutil.copyfile(here / "allocation-probe.go.txt", probe / "allocation_probe_test.go")
subprocess.run(
    ["go", "test", "-mod=mod", "-run", "^TestAllocationProbe$", "-v", ".", "-timeout=1m"],
    cwd=probe, env={**os.environ, "GOMAXPROCS": "1", "GSR_ALLOCATION_PROBE": str(output / "allocation-probe.json")},
    check=True, timeout=65,
)
