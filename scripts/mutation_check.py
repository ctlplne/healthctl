"""Mutation check for internal/clinical.

Each mutant changes one safety or control rule in a copy of one source file
and runs the clinical tests against it with `go test -overlay`, so the real
source files are never modified. A mutant is killed when the tests fail. The
check fails if any mutant survives, because that rule has no test that
notices it breaking.

  python3 scripts/mutation_check.py           # every mutant
  python3 scripts/mutation_check.py --list    # describe the mutants only
"""
import argparse
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
PKG = ROOT / "internal" / "clinical"

# (file, what the mutant breaks, original text, mutated text)
MUTANTS = [
    ("intents.go", "a yes to a red flag only asks for a prompt check",
     '"red_flag", warningEmergency, emergency)', '"red_flag", warningCheck, emergency)'),
    ("encounter.go", "an interview answer to a red flag is never recorded as one",
     "\te.recordRedFlag(e.Answers[len(e.Answers)-1], emergency)\n", "\t_ = emergency\n"),
    ("commands.go", "commands may carry fields their action ignores",
     "if !slices.Contains(allowed, f) && !slices.Contains(envelopeFields, f) {", "if !slices.Contains(allowed, f) && !slices.Contains(envelopeFields, f) && false {"),
    ("model.go", "the model's urgent flags are ignored",
     'e.addWarning(ev, "model_flag", level, emergency)', "_ = level"),
    ("model.go", "any denial is accepted as support for lowering concern",
     "func denialSupports(concept, quote string, a Answer) bool {", "func denialSupports(concept, quote string, a Answer) bool {\n\treturn true"),
    ("sanitize.go", "invalid findings are kept",
     "if err := validateFinding(in, f); err != nil {", "if err := validateFinding(in, f); false && err != nil {"),
    ("intents.go", "choices may name symptoms the user never reported",
     "if wholeWord(c, name) && !t.mentionedIn(said) {", "if wholeWord(c, name) && !t.mentionedIn(said) && false {"),
    ("intents.go", "the model's question budget is unlimited",
     "const maxModelQuestions = 12", "const maxModelQuestions = 1200"),
    ("budget.go", "the concern and emergency screen can be left out of the model input",
     'if a.Kind == "concern" || a.Kind == "screen" || i >= len(in.Answers)-keepNewest {', "if i >= len(in.Answers)-keepNewest {"),
    ("payload.go", "older payloads are not migrated",
     "migrations[v](&e)", "_ = migrations[v]"),
    ("conditions.go", "dangerous causes left out of an assessment get no don't-miss line",
     "if !present && !contains(out, d.line) {", "if !present && !contains(out, d.line) && false {"),
    ("context.go", "negation reaches far past the words it governs",
     "if count >= 8 {", "if count >= 80 {"),
    ("safety.go", "chest pain is not an urgent sign",
     "sign(warningEmergency, false, `chest (?:pains?|pressure", "sign(warningEmergency, false, `chestx (?:pains?|pressure"),
    ("transitions.go", "every state change is accepted by the transition check",
     "if slices.Contains(r.to, to) || to == from && slices.Contains(r.to, same) {", "if true {"),
]


def run_mutant(i, file, original, mutated, timeout):
    src = (PKG / file).read_text()
    if src.count(original) != 1:
        return None, f"original text appears {src.count(original)} times in {file}"
    with tempfile.TemporaryDirectory() as tmp:
        copy = Path(tmp) / file
        copy.write_text(src.replace(original, mutated, 1))
        overlay = Path(tmp) / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {str(PKG / file): str(copy)}}))
        try:
            out = subprocess.run(["go", "test", "-count=1", "-overlay", str(overlay), "./internal/clinical"], cwd=ROOT,
                                 capture_output=True, text=True, timeout=timeout)
        except subprocess.TimeoutExpired:
            return True, "killed (timed out)"
        failed = [l.strip() for l in out.stdout.splitlines() if l.startswith("--- FAIL")]
        if "[build failed]" in out.stdout + out.stderr or "[setup failed]" in out.stdout + out.stderr:
            # A mutant that doesn't compile tests nothing.
            return None, "the mutant does not compile: " + (out.stdout + out.stderr).strip().splitlines()[-1][:160]
        if out.returncode != 0:
            return True, "killed by " + (", ".join(f.split()[2] for f in failed[:3]) if failed else "a test failure")
        return False, "survived"


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--list", action="store_true")
    parser.add_argument("--timeout", type=int, default=600)
    args = parser.parse_args()
    if args.list:
        for file, what, _, _ in MUTANTS:
            print(f"{file}: {what}")
        return 0
    killed = survived = broken = 0
    for i, (file, what, original, mutated) in enumerate(MUTANTS, 1):
        start = time.time()
        result, detail = run_mutant(i, file, original, mutated, args.timeout)
        if result is None:
            broken += 1
            status = "ERROR"
        elif result:
            killed += 1
            status = "KILLED"
        else:
            survived += 1
            status = "SURVIVED"
        print(f"[{status}] {i:2d} {file}: {what} — {detail} ({time.time() - start:.0f}s)", flush=True)
    print(f"MUTATION mutants={len(MUTANTS)} killed={killed} survived={survived} errors={broken}")
    return 0 if survived == 0 and broken == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
