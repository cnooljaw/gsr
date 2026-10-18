"""从全部原始块独立复算配对值并连接控制/比较结果；不删除异常样本。"""

import argparse
import json
import math
from pathlib import Path
import statistics

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("directory", type=Path)
options = parser.parse_args()
root = options.directory.resolve()
rows = {}
counts = {"blocks": 0, "quartets": 0, "pairedRounds": 0}
for run in ["self", "clone", "pass1", "pass2"]:
    directory = root / run
    raw = json.loads((directory / "raw.json").read_text())
    summaries = {item["name"]: item for item in json.loads((directory / "summary.json").read_text())}
    assert len(raw["results"]) == 43
    assert (raw["rounds"], raw["quartetsPerRound"], raw["targetBlockNs"]) == (6, 4, 10000000)
    assert raw["gogcPercent"] > 0
    for result in raw["results"]:
        arm_ns = {"A": 0, "B": 0}
        arm_calls = {"A": 0, "B": 0}
        logs = [[] for _ in range(6)]
        assert len(result["quartets"]) == 24
        for index, quartet in enumerate(result["quartets"]):
            assert (quartet["round"], quartet["index"]) == (index // 4, index % 4)
            order = "ABBA" if (index // 4 + index % 4) % 2 == 0 else "BAAB"
            assert "".join(block["arm"] for block in quartet["blocks"]) == order
            value = 0
            for block in quartet["blocks"]:
                assert block["calls"] == result["callsPerBlock"] and block["calls"] > 0
                arm = block["arm"]
                arm_ns[arm] += block["nanoseconds"]
                arm_calls[arm] += block["calls"]
                value += (1 if arm == "B" else -1) * math.log(block["nanoseconds"] / block["calls"])
                counts["blocks"] += 1
            ratio = math.exp(value / 2)
            assert math.isclose(ratio, quartet["rightOverLeft"], rel_tol=1e-12)
            logs[index // 4].append(math.log(ratio))
            counts["quartets"] += 1
        ratios = [math.exp(sum(values) / len(values)) for values in logs]
        assert all(math.isclose(a, b, rel_tol=1e-12) for a, b in zip(ratios, result["roundRatios"]))
        summary = summaries[result["name"]]
        assert math.isclose(statistics.median(ratios), summary["medianRatio"], rel_tol=1e-12)
        for arm, key in [("A", "leftNsPerOp"), ("B", "rightNsPerOp")]:
            assert math.isclose(arm_ns[arm] / arm_calls[arm], result[key], rel_tol=1e-12)
            for metric, allocation_key in [("bytes", "bytes"), ("mallocs", "mallocs")]:
                median = statistics.median(value[metric] / value["calls"] for value in result["allocations"] if value["arm"] == arm)
                assert median == summary["allocationMedian"][arm][allocation_key]
        counts["pairedRounds"] += 6
        deltas = []
        for round_index in range(6):
            arm_logs = {"A": [], "B": []}
            for quartet in result["quartets"][round_index * 4:round_index * 4 + 4]:
                for block in quartet["blocks"]:
                    arm_logs[block["arm"]].append(math.log(block["nanoseconds"] / block["calls"]))
            deltas.append(math.exp(statistics.mean(arm_logs["B"])) - math.exp(statistics.mean(arm_logs["A"])))
        name = result["name"] if run != "business" else "nil-prefix/" + result["name"]
        rows.setdefault(name, {"name": name})[run] = {
            **summary, "leftNsPerOp": result["leftNsPerOp"], "rightNsPerOp": result["rightNsPerOp"],
            "roundGeometricDeltaNs": deltas, "medianRoundGeometricDeltaNs": statistics.median(deltas),
        }
output = {"decision": "assess local record nil caller branch candidate in working tree; uncommitted", "independentRecalculation": counts,
          "relativeTolerance": 1e-12, "results": list(rows.values())}
(root / "joined.json").write_text(json.dumps(output, indent=2) + "\n")
for row in list(rows.values()):
    print(row["name"], *[f"{(row[run]['medianRatio'] - 1) * 100:+.2f}%" for run in ["pass1", "pass2"]])
print(counts)
