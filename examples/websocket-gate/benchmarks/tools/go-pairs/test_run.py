"""保护校准判定，不允许中位数掩盖轮次波动或分配变化。"""

import copy
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

from run import summarize


def control(ratios):
    return {
        "mode": "self",
        "results": [{
            "name": "fixture/pack",
            "roundRatios": ratios,
            "allocations": [
                {"arm": arm, "calls": 128, "bytes": 4096, "mallocs": 128}
                for _ in range(3) for arm in "AB"
            ],
        }],
    }


class ControlDecisionTests(unittest.TestCase):
    def test_stable_control_preserves_all_rounds(self):
        raw = control([1, 1.001, 0.999, 1.002, 0.998, 1])
        original = copy.deepcopy(raw)
        result = summarize(raw)[0]
        self.assertTrue(result["controlAccepted"])
        self.assertEqual(result["roundRatios"], raw["results"][0]["roundRatios"])
        self.assertEqual(raw, original)

    def test_good_median_does_not_hide_dispersion(self):
        result = summarize(control([0.9, 1.1, 0.9, 1.1, 1, 1]))[0]
        self.assertAlmostEqual(result["medianRatio"], 1)
        self.assertFalse(result["controlAccepted"])

    def test_allocation_difference_rejects_control(self):
        raw = control([1] * 6)
        for value in raw["results"][0]["allocations"]:
            if value["arm"] == "B":
                value["bytes"] += 16
        self.assertFalse(summarize(raw)[0]["controlAccepted"])

    def test_timing_status_does_not_waive_allocation_rejection(self):
        raw = control([1] * 6)
        for value in raw["results"][0]["allocations"]:
            if value["arm"] == "B":
                value["bytes"] += 16
        result = summarize(raw)[0]
        self.assertTrue(result["timingAccepted"])
        self.assertFalse(result["sameAllocation"])
        self.assertFalse(result["controlAccepted"])

    def test_comparison_never_grants_automatic_adoption(self):
        raw = control([0.8] * 6)
        raw["mode"] = "compare"
        self.assertIsNone(summarize(raw)[0]["controlAccepted"])


class ConfigurationTests(unittest.TestCase):
    def reject_before_preparing(self, extra, fragment, environment=None):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "output"
            command = [
                "python3", str(Path(__file__).with_name("run.py")),
                "--library", directory, "--baseline", "unused", "--output", str(output),
                "--prepare-only",
            ] + extra
            result = subprocess.run(command, env={**os.environ, **(environment or {})}, capture_output=True, text=True)
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertIn(fragment, result.stderr)
            self.assertFalse(output.exists())

    def test_unsupported_duration_is_rejected(self):
        self.reject_before_preparing(["--block-ms", "30"], "invalid choice")

    def test_duplicate_case_is_rejected(self):
        self.reject_before_preparing(["--case", "a", "--case", "a"], "duplicate --case")

    def test_disabled_gc_is_rejected(self):
        self.reject_before_preparing([], "requires enabled GC", {"GOGC": "off"})


if __name__ == "__main__":
    unittest.main()
