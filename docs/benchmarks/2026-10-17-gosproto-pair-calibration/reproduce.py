"""核对历史工具和夹具后，重跑三种模式；不修改正式库。"""

import argparse
import hashlib
import json
from pathlib import Path
import subprocess

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
        raise SystemExit("工具已变化，请从本报告所在历史提交恢复对应工具")
bench = tool.parents[1]
for name, digest in expected["fixtures"].items():
    if sha(bench / name) != digest:
        raise SystemExit("夹具已变化：" + name)
for name in ["representative_benchmark_test.go", "protocol_benchmark_test.go"]:
    if sha(bench / name) != expected["harnessGoSource"][name]:
        raise SystemExit("夹具 Go 代码已变化：" + name)
patch = here.parent / "2026-10-17-gosproto-pack-sparse/patches/unroll.patch"
if sha(patch) != json.loads((here / "compare/inputs.json").read_text())["candidatePatchSha256"]:
    raise SystemExit("候选补丁已变化")
output = options.output.resolve()
output.mkdir(exist_ok=False, parents=True)
for mode in ["self", "clone", "compare"]:
    command = [
        "python3", str(tool / "run.py"), "--library", str(options.library.resolve()),
        "--baseline", expected["baselineCommit"], "--output", str(output / mode), "--mode", mode,
    ]
    if mode == "compare":
        command += ["--candidate-patch", str(patch)]
    subprocess.run(command, check=True)
