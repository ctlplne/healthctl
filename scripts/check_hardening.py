"""Run every hardening check and print one result line per check.

Prints ALL CHECKS PASSED and exits 0 only when every check passes and
HARDENING.md has no open item outside HANDOFF. Requires Go, Node with the web
dependencies installed, Python 3 and a running Docker engine.

  python3 scripts/check_hardening.py           # everything, including scripts/verify.py
  python3 scripts/check_hardening.py --quick   # skip verify.py and the evaluator image
"""
import argparse
import os
from pathlib import Path
import re
import secrets
import socket
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
WEB = ROOT / "web"
PACKAGES = ["./cmd/...", "./internal/...", "./evaluation/..."]
PG_IMAGE = "postgres:16.15-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea"
results = []


def run(cmd, cwd=ROOT, env=None, timeout=3600):
    return subprocess.run(cmd, cwd=cwd, env=env, capture_output=True, text=True, timeout=timeout)


def tail(text, n=12):
    return "\n".join(text.strip().splitlines()[-n:])


def check(name, fn):
    start = time.time()
    try:
        ok, detail = fn()
    except Exception as exc:  # a crashed check is a failed check
        ok, detail = False, f"error: {exc}"
    elapsed = time.time() - start
    results.append((name, ok))
    first, _, rest = str(detail).partition("\n")
    print(f"[{'PASS' if ok else 'FAIL'}] {name} ({elapsed:.0f}s): {first}", flush=True)
    if rest and not ok:
        print("    " + rest.replace("\n", "\n    "), flush=True)


def go_vet():
    out = run(["go", "vet", *PACKAGES])
    return out.returncode == 0, "clean" if out.returncode == 0 else tail(out.stderr + out.stdout)


def go_test_race():
    out = run(["go", "test", "-race", "-count=1", *PACKAGES], timeout=1800)
    ok_lines = [l for l in out.stdout.splitlines() if l.startswith("ok ")]
    return out.returncode == 0, f"{len(ok_lines)} packages passed" if out.returncode == 0 else tail(out.stdout + out.stderr, 25)


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def postgres_tests():
    password = secrets.token_hex(16)
    port = free_port()
    name = "healthctl-check-pg-" + secrets.token_hex(4)
    started = run(["docker", "run", "-d", "--rm", "--name", name, "-e", "POSTGRES_DB=clinicalhistory", "-e", "POSTGRES_USER=clinicalhistory",
                   "-e", "POSTGRES_PASSWORD=" + password, "-p", f"127.0.0.1:{port}:5432", PG_IMAGE], timeout=600)
    if started.returncode != 0:
        return False, "could not start PostgreSQL: " + tail(started.stderr)
    try:
        for _ in range(60):
            if run(["docker", "exec", name, "pg_isready", "-U", "clinicalhistory", "-d", "clinicalhistory"]).returncode == 0:
                break
            time.sleep(1)
        dsn = f"postgres://clinicalhistory:{password}@127.0.0.1:{port}/clinicalhistory?sslmode=disable"
        env = dict(os.environ, CLINICAL_TEST_POSTGRES="1", CLINICAL_TEST_DSN=dsn, PGHOST="127.0.0.1", PGPORT=str(port),
                   PGUSER="clinicalhistory", PGPASSWORD=password, PGDATABASE="clinicalhistory", PGSSLMODE="disable")
        out = run(["go", "test", "-race", "-count=1", "-v", "-run", "Postgres", "./internal/...", "./cmd/..."], env=env, timeout=1800)
        passed = len(re.findall(r"^--- PASS: \S*Postgres", out.stdout, re.M))
        skipped = len(re.findall(r"^--- SKIP: \S*Postgres", out.stdout, re.M))
        ok = out.returncode == 0 and passed > 0 and skipped == 0
        return ok, f"{passed} PostgreSQL tests passed, {skipped} skipped" if ok else tail(out.stdout + out.stderr, 25)
    finally:
        run(["docker", "rm", "-f", name])


def web_checks():
    if not (WEB / "node_modules").is_dir():
        install = run(["npm", "ci", "--no-fund", "--no-audit"], cwd=WEB, timeout=1200)
        if install.returncode != 0:
            return False, "npm ci failed: " + tail(install.stderr)
    tsc = run(["npx", "tsc", "--noEmit", "-p", "."], cwd=WEB)
    if tsc.returncode != 0:
        return False, "tsc: " + tail(tsc.stdout + tsc.stderr)
    with tempfile.TemporaryDirectory() as out_dir:
        build = run(["npx", "vite", "build", "--outDir", out_dir, "--emptyOutDir"], cwd=WEB)
    if build.returncode != 0:
        return False, "vite build: " + tail(build.stdout + build.stderr)
    tests = run(["npm", "test", "--silent"], cwd=WEB)
    if tests.returncode != 0:
        return False, "web unit tests: " + tail(tests.stdout + tests.stderr)
    summary = re.search(r"Tests\s+(\d+) passed", tests.stdout)
    return True, f"tsc clean, vite build ok, {summary.group(1) if summary else 'all'} web unit tests passed"


def go_run_test(pattern, package="./internal/clinical"):
    return run(["go", "test", "-count=1", "-v", "-run", pattern, package], timeout=900)


def corpus_metrics():
    out = go_run_test("^TestCorpus$")
    missed = re.search(r"urgent_missed=(\d+)/(\d+)", out.stdout)
    rejected = re.search(r"correct_rejected=(\d+)/(\d+) \(([\d.]+)%\)", out.stdout)
    accepted = re.search(r"must_reject_accepted=(\d+)/(\d+)", out.stdout)
    alarms = re.search(r"false_alarms=(\d+)/(\d+)", out.stdout)
    if not (missed and rejected and accepted):
        return False, tail(out.stdout + out.stderr)
    ok = out.returncode == 0 and missed.group(1) == "0" and float(rejected.group(3)) <= 5 and accepted.group(1) == "0"
    return ok, (f"{missed.group(1)} of {missed.group(2)} urgent sentences missed, {rejected.group(1)} of {rejected.group(2)} correct extractions rejected "
                f"({rejected.group(3)}%), {accepted.group(1)} of {accepted.group(2)} must-reject accepted, {alarms.group(1)} of {alarms.group(2)} false alarms")


def every_input_path():
    out = go_run_test("TestEveryInputPathScreens")
    paths = re.findall(r"^    --- PASS: TestEveryInputPathScreens/(\S+)", out.stdout, re.M)
    failed = re.findall(r"^    --- FAIL: TestEveryInputPathScreens/(\S+)", out.stdout, re.M)
    draft = go_run_test("TestDraftResponseScreens", "./internal/server")
    ok = out.returncode == 0 and draft.returncode == 0 and len(paths) >= 15 and not failed
    detail = f"{len(paths)} encounter input paths warned ({', '.join(p.replace('_', ' ') for p in sorted(paths))}); dictation drafts screened; diary saves covered by TestDiarySaveScreensPostgres"
    return ok, detail if ok else detail + "\n" + tail(out.stdout + draft.stdout, 20)


def property_test():
    out = go_run_test("TestRandomActionInvariants")
    m = re.search(r"PROPERTY sequences=(\d+) checked_steps=(\d+)", out.stdout)
    ok = out.returncode == 0 and bool(m)
    return ok, f"{m.group(1)} random sequences, {m.group(2)} steps, every invariant held" if ok else tail(out.stdout + out.stderr)


def holdout():
    out = go_run_test("TestCorpusHoldout")
    m = re.search(r"HOLDOUT rows=(\d+) urgent_missed=(\d+)/(\d+) false_alarms=(\d+)/(\d+)", out.stdout)
    if not m:
        return True, "no held-out set waiting (informational)"
    return True, f"informational: {m.group(2)} of {m.group(3)} untouched urgent sentences missed, {m.group(4)} of {m.group(5)} false alarms"


def scorecard():
    image = "healthctl-check-evaluator"
    build = run(["docker", "build", "-q", "--target", "evaluator", "-t", image, "."], timeout=1800)
    if build.returncode != 0:
        return False, "evaluator image: " + tail(build.stderr)
    with tempfile.NamedTemporaryFile("w+", suffix=".json", delete=False) as fresh:
        path = fresh.name
    try:
        with open(path, "w") as out:
            evaluate = subprocess.run(["docker", "run", "--rm", "--network", "none", "--read-only", "--tmpfs", "/tmp", image, "-release"], stdout=out, stderr=subprocess.PIPE, text=True, timeout=900)
        if evaluate.returncode != 0:
            return False, "evaluator: " + tail(evaluate.stderr)
        compare = run([sys.executable, "scripts/check_scorecard.py", "--drift", path])
        first = compare.stdout.strip().splitlines()[0] if compare.stdout.strip() else ""
        return compare.returncode == 0, ("no drift from the committed scorecard; " + first) if compare.returncode == 0 else tail(compare.stdout, 30)
    finally:
        os.unlink(path)


def docs():
    out = run([sys.executable, "scripts/check_docs.py"])
    return out.returncode == 0, tail(out.stdout, 3)


def python_tests():
    out = run([sys.executable, "-m", "unittest", "discover", "-s", "tests", "-p", "test_*.py"])
    study = run([sys.executable, "-m", "unittest", "discover", "-s", "evaluation/study"])
    count = re.search(r"Ran (\d+) tests", out.stderr)
    ok = out.returncode == 0 and study.returncode == 0
    return ok, f"{count.group(1) if count else '?'} script tests and the study tests passed" if ok else tail(out.stderr + study.stderr)


def verify():
    out = run([sys.executable, "scripts/verify.py"], timeout=5400)
    return out.returncode == 0, tail(out.stdout + out.stderr, 4 if out.returncode == 0 else 30)


def hardening_summary():
    text = (ROOT / "HARDENING.md").read_text()
    items = re.findall(r"^- \[([ xh])\] \*\*([A-Z]+\d+)\*\*", text, re.M)
    done = [i for s, i in items if s == "x"]
    handoff = [i for s, i in items if s == "h"]
    open_items = [i for s, i in items if s == " "]
    print(f"HARDENING.md summary: {len(done)} done, {len(handoff)} handoff, {len(open_items)} open outside HANDOFF"
          + (f": {', '.join(open_items)}" if open_items else ""), flush=True)
    return not open_items, f"{len(done)} done, {len(handoff)} handoff ({', '.join(handoff)}), {len(open_items)} open"


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--quick", action="store_true", help="Skip scripts/verify.py and the evaluator image.")
    args = parser.parse_args()
    os.chdir(ROOT)
    check("go vet ./cmd/... ./internal/... ./evaluation/...", go_vet)
    check("go test -race ./cmd/... ./internal/... ./evaluation/...", go_test_race)
    check("PostgreSQL integration tests", postgres_tests)
    check("web tsc, vite build and unit tests", web_checks)
    check("corpus metrics", corpus_metrics)
    check("one urgent sentence through every input path", every_input_path)
    check("random-action property test", property_test)
    check("held-out set", holdout)
    if not args.quick:
        check("fresh release scorecard without drift", scorecard)
    check("scripts/check_docs.py", docs)
    check("Python script and study tests", python_tests)
    if not args.quick:
        check("python3 scripts/verify.py", verify)
    check("HARDENING.md has no open items outside HANDOFF", hardening_summary)
    failed = [name for name, ok in results if not ok]
    if failed:
        print("CHECKS FAILED: " + "; ".join(failed))
        return 1
    if args.quick:
        print("QUICK CHECKS PASSED (scripts/verify.py and the scorecard were skipped)")
        return 0
    print("ALL CHECKS PASSED")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
