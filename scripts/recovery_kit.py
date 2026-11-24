"""Write a private recovery kit for this installation.

The kit holds the installation key and database password that a restore needs.
Without them nobody, including the publisher, can open the journal. The script
never prints the key. It prints a short fingerprint that the app asks you to
enter, which proves you saved the kit for the key that is in use now.
"""
import argparse
import base64
import datetime
import hashlib
import hmac
import os
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
LABEL = b"healthctl-recovery-kit-v1"


def fingerprint(key_b64):
    digest = hmac.new(base64.b64decode(key_b64), LABEL, hashlib.sha256).hexdigest()[:12]
    return "-".join(digest[i:i + 4] for i in range(0, 12, 4))


def read_env(path):
    values = {}
    for line in path.read_text().splitlines():
        if "=" in line and not line.lstrip().startswith("#"):
            name, value = line.split("=", 1)
            values[name.strip()] = value.strip()
    return values


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--env-file", type=Path, default=ROOT / ".env")
    parser.add_argument("--output", type=Path, help="Where to write the kit (default: your home folder).")
    args = parser.parse_args()
    values = read_env(args.env_file)
    key = values.get("CLINICAL_DATA_KEY", "")
    try:
        if len(base64.b64decode(key, validate=True)) != 32:
            raise ValueError
    except ValueError:
        print("The environment file has no valid CLINICAL_DATA_KEY.", file=sys.stderr)
        return 2
    today = datetime.date.today().isoformat()
    output = args.output or Path.home() / f"healthctl-recovery-kit-{today}.txt"
    output = output.expanduser()
    if output.exists():
        print(f"{output} already exists. Choose another --output path.", file=sys.stderr)
        return 2
    code = fingerprint(key)
    sheet = f"""healthctl recovery kit
Created {today} from {args.env_file.resolve()}

Keep this file somewhere safe and private, such as a password manager or an
encrypted drive. Anyone with this kit and a copy of your data can read your
journal. Without it, nobody can.

Fingerprint (enter this in healthctl to confirm): {code}

CLINICAL_DATA_KEY={key}
POSTGRES_PASSWORD={values.get("POSTGRES_PASSWORD", "")}

To restore: put these two lines into the .env of a checkout of the same
healthctl version, then follow "Restore and verify" in docs/operations.md.
After a key rotation, make a new kit. The old kit only opens older backups.
"""
    fd = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as out:
        out.write(sheet)
    print(f"Saved the recovery kit to {output} (readable only by you).")
    print(f"Fingerprint: {code}")
    print("Move the kit to safe storage, then enter the fingerprint in healthctl.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
