"""Upgrade a Docker installation with a snapshot first, and roll back if needed.

  python3 scripts/upgrade.py              # snapshot, keep the current image, rebuild and restart
  python3 scripts/upgrade.py --rollback   # restore the pre-upgrade snapshot and the previous image

The snapshot is a verified pg_dump in the backup folder, named
pre-upgrade-<time>.dump. It is encrypted at rest only if the folder is; the
installation key in .env is still required to read the records. Nothing leaves
this computer.
"""
import argparse
import datetime
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
STATE = ROOT / ".upgrade" / "last.json"
IMAGE = "clinical-history-app"
DUMP = ["pg_dump", "-Fc", "-U", "clinicalhistory", "-d", "clinicalhistory", "--no-owner", "--no-acl",
        "--exclude-table-data=public.sessions", "--exclude-table-data=public.brief_access"]


class Failed(Exception):
    pass


def default_run(cmd, stdin=None, stdout=None):
    result = subprocess.run(cmd, cwd=ROOT, stdin=stdin, stdout=stdout or subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode != 0:
        raise Failed(" ".join(cmd[:4]) + " failed: " + (result.stderr or b"").decode(errors="replace").strip()[-300:])
    return result


def compose(*args):
    return ["docker", "compose", *args]


def backup_folder():
    folder = os.environ.get("CLINICAL_BACKUP_DIRECTORY") or ""
    env = ROOT / ".env"
    if not folder and env.exists():
        for line in env.read_text().splitlines():
            if line.startswith("CLINICAL_BACKUP_DIRECTORY="):
                folder = line.split("=", 1)[1].strip()
    if not folder:
        raise Failed("Set CLINICAL_BACKUP_DIRECTORY (python3 scripts/init.py does) so the snapshot lands outside the database volume.")
    path = Path(folder).expanduser()
    path.mkdir(parents=True, exist_ok=True)
    return path


def snapshot(run, folder, now):
    """Write and verify a pre-upgrade dump. Returns its path."""
    path = folder / ("pre-upgrade-" + now.strftime("%Y%m%dT%H%M%SZ") + ".dump")
    tmp = path.with_suffix(".partial")
    with open(tmp, "wb") as out:
        run(compose("exec", "-T", "db", *DUMP), stdout=out)
    with open(tmp, "rb") as dump:
        run(compose("exec", "-T", "db", "pg_restore", "--list"), stdin=dump)
    if tmp.stat().st_size == 0:
        tmp.unlink()
        raise Failed("The snapshot is empty; the upgrade was not started.")
    os.chmod(tmp, 0o600)
    tmp.rename(path)
    return path


def upgrade(run=default_run, now=None, folder=None):
    now = now or datetime.datetime.now(datetime.timezone.utc)
    folder = folder or backup_folder()
    snap = snapshot(run, folder, now)
    previous = IMAGE + ":previous"
    run(["docker", "tag", IMAGE + ":latest", previous])
    STATE.parent.mkdir(exist_ok=True)
    STATE.write_text(json.dumps({"snapshot": str(snap), "image": previous, "at": now.isoformat()}))
    run(compose("build", "app"))
    run(compose("up", "-d", "--wait"))
    print("Upgraded. A verified snapshot was taken first: " + str(snap))
    print("To go back: python3 scripts/upgrade.py --rollback")
    return snap


def rollback(run=default_run):
    if not STATE.exists():
        raise Failed("No pre-upgrade snapshot is recorded; nothing to roll back.")
    state = json.loads(STATE.read_text())
    snap = Path(state["snapshot"])
    if not snap.exists():
        raise Failed("The pre-upgrade snapshot " + str(snap) + " is missing.")
    # Stop the app, recreate the database from the snapshot, then start the
    # previous image. Migrations the newer version applied are discarded.
    run(compose("stop", "app", "backup"))
    run(compose("exec", "-T", "db", "dropdb", "-U", "clinicalhistory", "--if-exists", "--force", "clinicalhistory"))
    run(compose("exec", "-T", "db", "createdb", "-U", "clinicalhistory", "clinicalhistory"))
    with open(snap, "rb") as dump:
        run(compose("exec", "-T", "db", "pg_restore", "-U", "clinicalhistory", "-d", "clinicalhistory", "--no-owner", "--no-acl"), stdin=dump)
    run(["docker", "tag", state["image"], IMAGE + ":latest"])
    run(compose("up", "-d", "--no-build", "--wait"))
    print("Rolled back to the version and data from " + state["at"] + ".")


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--rollback", action="store_true")
    args = parser.parse_args()
    try:
        rollback() if args.rollback else upgrade()
    except Failed as exc:
        print("upgrade: " + str(exc), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
