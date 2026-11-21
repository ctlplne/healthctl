"""Compare deterministic runs by case and arm, retaining individual safety failures."""
import argparse
import json
from pathlib import Path


def index(rows, fields):
    result = {}
    for row in rows:
        key = tuple(row[field] for field in fields)
        if key in result:
            raise ValueError(f"Duplicate scorecard row {key}")
        result[key] = row
    return result


def compare(current, baseline):
    if current["mode"] != "deterministic_fixture":
        raise ValueError("Expected the offline fixture scorecard")
    failures = []
    known = []
    for collection, keys in [
        ("results", ("case", "arm", "repetition")),
        ("reportComparison", ("variant", "repetition")),
    ]:
        actual = index(current[collection], keys)
        expected = index(baseline[collection], keys)
        for key in sorted(actual.keys() | expected.keys()):
            name = " / ".join(map(str, key))
            if key not in actual or key not in expected:
                failures.append(f"{name}: case set differs from the versioned baseline")
                continue
            row = actual[key]
            if collection == "reportComparison":
                if row["status"] != "exercised" or row["failures"]:
                    failures.append(f"{name}: {row['status']}, failures {row['failures']}")
                continue
            for field in ("fabricatedFindings", "staleAssessments", "lostWarnings",
                          "alertErrors", "contradictionErrors", "errors"):
                for issue in row[field]:
                    failures.append(f"{name}: {field}: {issue}")
            missed = set(row["missedTargets"])
            previous = set(expected[key]["missedTargets"])
            for target in sorted(missed - previous):
                failures.append(f"{name}: newly missed history target: {target}")
            if row["arm"] == "adaptive":
                for target in sorted(missed & previous):
                    known.append(f"{name}: previously missed history target: {target}")
    if current["failedRuns"]:
        failures.append(f"The evaluator recorded {current['failedRuns']} failed runs.")
    return failures, known


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("scorecard", type=Path)
    parser.add_argument("--baseline", type=Path, default=Path("evaluation/release-fixture-results.json"))
    args = parser.parse_args()
    current = json.loads(args.scorecard.read_text())
    failures, known = compare(current, json.loads(args.baseline.read_text()))
    print(f"Evaluated {len(current['results'])} interviews and {len(current['reportComparison'])} report comparisons.")
    for message in failures + known:
        print(f"- {message}")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
