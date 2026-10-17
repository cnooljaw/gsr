"""复现 FF 段集中处理候选的固定筛选；候选已撤回，不修改正式库。"""

import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess


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
    tool = root / "examples/websocket-gate/benchmarks/tools/go-pairs"
    plan = json.loads((here / "plan.json").read_text())
    expected = {mode: json.loads((here / mode / "inputs.json").read_text()) for mode in plan["modes"]}
    reference = expected["compare"]
    for name, value in reference["tools"].items():
        if digest(tool / name) != value:
            raise SystemExit("请恢复对应历史工具：" + name)
    for name, value in reference["generatedGoInputs"].items():
        if digest(root / name) != value:
            raise SystemExit("生成类型发生变化：" + name)
    bench = tool.parents[1]
    for name, value in reference["fixtures"].items():
        if digest(bench / name) != value:
            raise SystemExit("夹具发生变化：" + name)
    for name in ["representative_benchmark_test.go", "protocol_benchmark_test.go"]:
        if digest(bench / name) != reference["harnessGoSource"][name]:
            raise SystemExit("夹具实现发生变化：" + name)
    patch = here / "patches/ff-runs.patch"
    if digest(patch) != reference["candidatePatchSha256"]:
        raise SystemExit("候选补丁发生变化")
    output = options.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    for mode in plan["modes"]:
        command = [
            "python3", "-B", str(tool / "run.py"), "--library", str(options.library.resolve()),
            "--baseline", plan["baselineCommit"], "--mode", mode,
            "--block-ms", str(plan["blockMs"]), "--output", str(output / mode),
        ]
        for name in plan["cases"]:
            command += ["--case", name]
        if mode == "compare":
            command += ["--candidate-patch", str(patch)]
        if options.prepare_only:
            command += ["--prepare-only"]
        subprocess.run(command, cwd=root, check=True, timeout=110)
        actual = json.loads((output / mode / "inputs.json").read_text())
        for key in ["beforeGoSource", "candidateGoSource", "harnessGoSource", "fixtures", "tools", "generatedGoInputs", "parameters", "selectedCases"]:
            if actual[key] != expected[mode][key]:
                raise SystemExit("重建输入不一致：" + mode + "/" + key)
    if options.prepare_only:
        return
    # 计时结束后才加入差分测试，不修改采样时的 harness。
    check = output / "differential"
    shutil.copytree(output / "compare/harness", check)
    shutil.copyfile(here / "pack-differential.go.txt", check / "pack_differential_test.go")
    subprocess.run(
        ["go", "test", "-mod=mod", "-run", "^(TestPackDifferential|TestRepresentativeFixtures)$", "-v", ".", "-timeout=1m"],
        cwd=check, check=True, timeout=65,
    )
    subprocess.run(
        ["go", "test", "-mod=mod", "-run", "^(TestPack|TestPackZeroRuns|TestPackPartialRunAtBlockLimit|TestUnpackOutputBudget)$", ".", "-timeout=1m"],
        cwd=output / "compare/candidate", check=True, timeout=65,
    )


if __name__ == "__main__":
    main()
