"""Run a local, counterbalanced fictional interview comparison with masked grading."""

import argparse
from collections import defaultdict
from datetime import datetime, timezone
import json
from pathlib import Path
import random
import secrets
import statistics

ARMS = (
    "healthctl-story-first",
    "healthctl-choices-first",
    "practice-intake",
    "plain-chat",
)
FAILURES = ("fabricatedFindings", "staleAssessments", "lostWarnings")
PRACTICE = [
    {
        "case": "dizziness",
        "intro": "I had an episode of dizziness and recently started a fictional medicine.",
        "variants": [
            ["I felt dizzy before my first dose.", "before the first dose"],
            ["I felt dizzy after my first dose.", "after the first dose"],
        ],
        "details": [
            "The label said five milligrams.",
            "I actually took half a tablet.",
            "I never lost consciousness.",
        ],
    },
    {
        "case": "bowel-change",
        "intro": "I have a persistent change in my bowel habits and brought two fictional reports.",
        "variants": [
            ["The change began before the sample was collected.", "before the sample"],
            ["The change began after the sample was collected.", "after the sample"],
        ],
        "details": [
            "The second report amended the first.",
            "I am unsure about the exact number of episodes.",
            "I did not see blood.",
        ],
    },
]


def put(path, data):
    with path.open("x", encoding="utf-8") as out:
        json.dump(data, out, indent=2)


def get(path):
    return json.loads(path.read_text())


def prepare(folder, count, seed, cases):
    if count < 8 or count % 8:
        raise ValueError("Use balanced blocks of eight fictional participants")
    folder.mkdir(mode=0o700)
    private = folder / "facilitator"
    private.mkdir(mode=0o700)
    rng = random.Random(seed)
    subjects = list(range(count))
    rng.shuffle(subjects)
    mapping = {}
    for subject, bucket in enumerate(subjects):
        tasks = []
        for i, case in enumerate(cases):
            ticket = secrets.token_hex(8)
            arm = ARMS[(bucket + i) % 4]
            variant = (bucket // 4 + i) % 2
            fact, key = case["variants"][variant]
            mapping[ticket] = {
                "arm": arm,
                "case": case["case"],
                "variant": variant,
                "expected": [key, *case["details"]],
            }
            tasks.append(
                {
                    "ticket": ticket,
                    "arm": arm,
                    "opening": case["intro"],
                    "fictionalExperience": [fact, *case["details"]],
                }
            )
        rng.shuffle(tasks)
        put(folder / f"actor-{subject+1:02}.json", tasks)
    put(private / "allocation.json", mapping)


def score(folder):
    mapping = get(folder / "facilitator/allocation.json")
    rows = []
    arms = defaultdict(list)
    for t, assignment in mapping.items():
        p = folder / ("grade-" + t + ".json")
        if not p.exists():
            rows.append({**assignment, "ticket": t, "status": "ungraded"})
            continue
        grade = get(p)
        timing = get(folder / ("timing-" + t + ".json"))
        row = {**assignment, **grade, **timing, "ticket": t, "status": "graded"}
        rows.append(row)
        arms[row["arm"]].append(row)
    totals = {}
    for arm, group in arms.items():
        usable = sum(r["usableWithoutRetake"] for r in group)
        complete_cost = all(r["cost"] is not None for r in group)
        totals[arm] = {
            "graded": len(group),
            "usableWithoutRetake": usable,
            "fractionUsable": usable / len(group),
            "essentialFactsMissed": sum(len(r["missedFacts"]) for r in group),
            "repeatedQuestions": sum(r["repeatedQuestions"] for r in group),
            "medianCompletionSeconds": statistics.median(r["seconds"] for r in group),
            "costPerUsableBrief": (
                sum(r["cost"] for r in group) / usable
                if usable and complete_cost
                else None
            ),
        }
    return {
        "arms": totals,
        "cases": rows,
        "failures": [
            {
                "ticket": r["ticket"],
                "case": r["case"],
                "arm": r["arm"],
                "type": field,
                "detail": v,
            }
            for r in rows
            if r["status"] == "graded"
            for field in FAILURES
            for v in r[field]
        ],
    }


def strings(prompt):
    return [
        v.strip()
        for v in input(prompt + " (separate entries with |, blank for none) ").split(
            "|"
        )
        if v.strip()
    ]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "action", choices=["prepare", "start", "finish", "grade", "score"]
    )
    parser.add_argument("folder", type=Path)
    parser.add_argument("ticket", nargs="?")
    parser.add_argument("--participants", type=int, default=8)
    parser.add_argument("--seed", type=int, default=23)
    parser.add_argument(
        "--cases",
        type=Path,
        help="Facilitator-approved fictional pairs in the practice manifest format",
    )
    parser.add_argument(
        "--brief",
        type=Path,
        help="De-identified plain-text handoff with arm and model labels removed",
    )
    parser.add_argument(
        "--cost",
        type=float,
        help="Actual attributable cost in a single currency across arms",
    )
    a = parser.parse_args()
    folder = a.folder
    if a.action == "prepare":
        prepare(folder, a.participants, a.seed, get(a.cases) if a.cases else PRACTICE)
        return
    if a.action == "score":
        print(json.dumps(score(folder), indent=2))
        return
    allocation = get(folder / "facilitator/allocation.json")
    if a.ticket not in allocation:
        parser.error("Choose an assigned ticket")
    t = a.ticket
    start = folder / ("start-" + t + ".json")
    now = datetime.now(timezone.utc)
    if a.action == "start":
        put(start, now.isoformat())
    if a.action == "finish":
        if not a.brief or not a.brief.is_file():
            parser.error("Supply a masked fictional brief using --brief")
        if a.cost is not None and a.cost < 0:
            parser.error("Cost must be nonnegative")
        elapsed = (now - datetime.fromisoformat(get(start))).total_seconds()
        # Keep the grading packet independent of actor instructions and assignment.
        put(
            folder / ("packet-" + t + ".json"),
            {
                "ticket": t,
                "expected": allocation[t]["expected"],
                "brief": a.brief.read_text(),
            },
        )
        put(folder / ("timing-" + t + ".json"), {"seconds": elapsed, "cost": a.cost})
    if a.action == "grade":
        packet = get(folder / ("packet-" + t + ".json"))
        print(packet["brief"])
        print("Fictional source facts:", *packet["expected"], sep="\n")
        grade = {field: strings(field) for field in ("missedFacts", *FAILURES)}
        grade["repeatedQuestions"] = int(
            input("How many unnecessary repeat questions? ")
        )
        usable = (
            input(
                "Could you use this brief without retaking the core history? [yes/no] "
            )
            .strip()
            .lower()
        )
        if usable not in ("yes", "no") or grade["repeatedQuestions"] < 0:
            parser.error("Check the grading entries")
        grade["usableWithoutRetake"] = usable == "yes"
        put(folder / ("grade-" + t + ".json"), grade)


if __name__ == "__main__":
    main()
