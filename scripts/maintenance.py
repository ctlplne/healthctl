"""Offline owner recovery and crash-resumable installation key rotation."""

import argparse
import base64
import getpass
import fcntl
import os
from pathlib import Path
import re
import secrets
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def write_atomic(path, data):
    fd, name = tempfile.mkstemp(dir=path.parent)
    try:
        with os.fdopen(fd, "wb") as out:
            out.write(data)
            out.flush()
            os.fsync(out.fileno())
        os.replace(name, path)
        folder = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(folder)
        finally:
            os.close(folder)
    finally:
        Path(name).unlink(missing_ok=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["recover-owner", "rotate-key"])
    parser.add_argument("--env-file", default=str(ROOT / ".env"), type=Path)
    parser.add_argument(
        "--project", help="Compose project, if using a named deployment"
    )
    parser.add_argument(
        "--password-stdin",
        action="store_true",
        help="Read the new password from standard input for an automated recovery.",
    )
    args = parser.parse_args()
    if args.password_stdin and args.action != "recover-owner":
        parser.error("--password-stdin applies to owner recovery")
    path = args.env_file.resolve()
    folder = path.parent / ".secrets"
    folder.mkdir(mode=0o700, exist_ok=True)
    lock = (folder / ("maintenance-" + path.name + ".lock")).open("a")
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        raise SystemExit("Maintenance is already running for this installation.")
    compose = ["docker", "compose", "--env-file", str(path)]
    if args.project:
        compose += ["-p", args.project]
    env = dict(os.environ)
    # This command operates on the key recorded in the selected installation file.
    env.pop("CLINICAL_DATA_KEY", None)

    def call(*command, data=None, key=None, check=True, quiet=False):
        local_env = env if key is None else dict(env, CLINICAL_DATA_KEY=key.decode())
        return subprocess.run(
            [*compose, *command],
            cwd=ROOT,
            env=local_env,
            input=data,
            check=check,
            stdout=subprocess.DEVNULL if quiet else None,
            stderr=subprocess.DEVNULL if quiet else None,
        )

    if args.action == "recover-owner":
        if args.password_stdin:
            password = sys.stdin.buffer.read(100).removesuffix(b"\n")
        else:
            password = getpass.getpass("New owner password: ").encode()
            if password != getpass.getpass("Repeat new password: ").encode():
                raise SystemExit("The passwords must match.")
        if not 12 <= len(password) <= 72:
            raise SystemExit("Use a password of 12–72 bytes.")
        call("stop", "app")
        try:
            call(
                "run", "--rm", "--no-deps", "-T", "app", "recover-owner", data=password
            )
        finally:
            call("up", "-d", "--wait", "app")
        return

    raw = path.read_bytes()
    matches = list(re.finditer(rb"(?m)^CLINICAL_DATA_KEY=([A-Za-z0-9+/=]+)$", raw))
    if len(matches) != 1:
        raise SystemExit(
            "Expected a single base64 installation key in the selected environment file."
        )
    current = matches[0].group(1)
    folder = path.parent / ".secrets"
    folder.mkdir(mode=0o700, exist_ok=True)
    pending = folder / ("rotation-" + path.name + ".pending")
    if not pending.exists():
        # Save the old key for pre-rotation backups, even after successful rotation.
        previous = folder / ("pre-rotation-" + secrets.token_hex(8) + ".env")
        write_atomic(previous, raw)
        write_atomic(pending, base64.b64encode(secrets.token_bytes(32)))
    new = pending.read_bytes()
    if len(base64.b64decode(new, validate=True)) != 32:
        raise SystemExit(
            "Invalid staged key. Use the protected recovery files before proceeding."
        )
    call("stop", "app")
    # If the process previously stopped after the database commit, install the
    # already-staged key without re-encrypting or generating another key.
    check = ("run", "--rm", "--no-deps", "-T", "app", "check-key")
    if call(*check, key=new, check=False, quiet=True).returncode != 0:
        call(*check, key=current)
        call(
            "run", "--rm", "--no-deps", "-T", "app", "rotate-key", data=new, key=current
        )
    write_atomic(path, raw[: matches[0].start(1)] + new + raw[matches[0].end(1) :])
    call("up", "-d", "--wait", "--force-recreate", "app")
    pending.unlink()
    print(
        "Key rotation verified. Keep the protected pre-rotation configuration with older backups."
    )


if __name__ == "__main__":
    main()
