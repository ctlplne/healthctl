import copy
import json
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
from check_scorecard import compare


class ScorecardGate(unittest.TestCase):
    def test_regressions_cannot_cancel_across_cases(self):
        baseline = json.loads((ROOT / "evaluation/release-fixture-results.json").read_text())
        current = copy.deepcopy(baseline)
        rows = [r for r in current["results"] if r["arm"] == "adaptive"]
        previously_missed = next(row for row in rows if row["missedTargets"])
        previously_missed["missedTargets"] = []
        new_miss = next(row for row in rows if row["case"] != previously_missed["case"])
        new_miss["missedTargets"] = ["a previously captured fact"]
        for field in ("fabricatedFindings", "staleAssessments", "lostWarnings"):
            new_miss[field].append("synthetic regression")
        removed = current["reportComparison"].pop()
        failures, _ = compare(current, baseline)
        for reason in ("fabricatedFindings", "staleAssessments", "lostWarnings", "newly missed"):
            self.assertTrue(any(reason in msg and new_miss["case"] in msg for msg in failures))
        self.assertTrue(any(removed["variant"] in msg for msg in failures))
