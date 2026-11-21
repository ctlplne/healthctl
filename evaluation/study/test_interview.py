import collections
from pathlib import Path
import tempfile
import unittest
import interview


class InterviewTest(unittest.TestCase):
    def test_allocation_masking_and_individual_failures(self):
        with tempfile.TemporaryDirectory() as temp:
            folder = Path(temp) / "run"
            interview.prepare(folder, 8, 13, interview.PRACTICE)
            mapping = interview.get(folder / "facilitator/allocation.json")
            cells = collections.Counter(
                (r["case"], r["arm"], r["variant"]) for r in mapping.values()
            )
            self.assertEqual(len(cells), 16)
            self.assertEqual(set(cells.values()), {1})
            t = next(iter(mapping))
            interview.put(
                folder / ("grade-" + t + ".json"),
                dict(
                    usableWithoutRetake=False,
                    repeatedQuestions=1,
                    missedFacts=["timing"],
                    fabricatedFindings=["fainting"],
                    staleAssessments=["old revision"],
                    lostWarnings=["omitted warning"],
                ),
            )
            interview.put(
                folder / ("timing-" + t + ".json"), dict(seconds=12, cost=None)
            )
            report = interview.score(folder)
            self.assertEqual(len(report["cases"]), 16)
            self.assertEqual(len(report["failures"]), 3)
            self.assertIsNone(report["arms"][mapping[t]["arm"]]["costPerUsableBrief"])
