"""Exercise the instrument with scripted inputs and temporary, fictional records."""

import contextlib
import io
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace as Args
import unittest
from unittest.mock import patch
import study


class StudyTest(unittest.TestCase):
    def test_instrument_rehearsal(self):
        manifest = Path(__file__).parent / "practice/case.json"
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp) / "packets"
            args = Args(
                manifest=manifest, directory=directory, clinicians=2, patients=2
            )
            with contextlib.redirect_stdout(io.StringIO()):
                study.prepare(args)
            plan = study.read(directory / "facilitator/plan.json")
            self.assertTrue(plan["practiceOnly"])
            self.assertEqual(len(plan["packets"]), 4)
            self.assertEqual(
                {
                    p["arm"]
                    for p in plan["packets"].values()
                    if p["role"] == "clinician"
                },
                {"candidate", "baseline"},
            )
            t, packet = next(
                (t, p)
                for t, p in plan["packets"].items()
                if p["arm"] == "candidate" and p["role"] == "clinician"
            )
            args.ticket = t
            self.assertNotIn("candidate", packet["filename"])
            with (
                contextlib.redirect_stdout(io.StringIO()),
                patch("builtins.input", side_effect=[""] * 8),
            ):
                study.collect(args)
            for reviewer in ["gradeA", "gradeB"]:
                args.reviewer = reviewer
                with (
                    contextlib.redirect_stdout(io.StringIO()),
                    patch(
                        "builtins.input",
                        side_effect=["4", "yes", "no", "no", "no", "yes", "rehearsal"],
                    ),
                ):
                    study.adjudicate(args)

            def result():
                with contextlib.redirect_stdout(io.StringIO()) as text:
                    study.score(args)
                return json.loads(text.getvalue())

            run = result()
            self.assertTrue(run["practiceOnly"])
            self.assertEqual(run["unanswered"], 3)
            self.assertEqual(run["arms"]["candidate"]["urgentDenominator"], 1)
            self.assertEqual(run["arms"]["candidate"]["essentialRecovery"], 1)
            self.assertIsNone(run["medianClinicianReadingTimeRatio"])
            self.assertEqual(
                run["proposedComparisons"]["appropriateAbstention"]["status"], "pending"
            )
            self.assertEqual(
                run["proposedComparisons"]["heldOutCaseCount"]["status"],
                "below_proposal",
            )
            third = directory / "facilitator" / f"grade-{t}-gradeB.json"
            grade = study.read(third)
            grade["grades"]["urgentShown"] = False
            third.write_text(json.dumps(grade))
            run = result()
            self.assertEqual(run["disagreementPackets"], 1)
            self.assertEqual(run["arms"]["candidate"]["urgentDenominator"], 0)


if __name__ == "__main__":
    unittest.main()
