"""Initialize an installation exactly once without printing any secrets.

Refuses to create a new installation key when this Compose project already has
a data volume. A new key cannot read the existing journal, so minting one would
make the data unrecoverable. Restore the matching .env from your recovery kit or
backup instead.
"""
import argparse
import base64
import os
from pathlib import Path
import platform
import secrets
import shutil
import subprocess
import sys

root = Path(__file__).resolve().parents[1]
PROJECT = "clinical-history"
DATA_VOLUME = PROJECT + "_clinical_data"


def existing_data_volume():
    """Return True, False, or None when Docker cannot be asked."""
    if shutil.which("docker") is None:
        return None
    try:
        out = subprocess.run(["docker", "volume", "ls", "-q", "--filter", "name=" + DATA_VOLUME],
                             capture_output=True, text=True, timeout=20)
    except (OSError, subprocess.TimeoutExpired):
        return None
    if out.returncode != 0:
        return None
    return DATA_VOLUME in out.stdout.split()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--backup-directory", type=Path, default=Path.home() / "healthctl-backups",
                        help="Folder for scheduled backups, outside Docker and outside this checkout.")
    args = parser.parse_args()
    env = root / ".env"
    if env.exists():
        print("Existing .env preserved.")
        return 0
    found = existing_data_volume()
    if found:
        print(f"Refusing to create a new installation key: the Docker volume {DATA_VOLUME} already holds a journal.", file=sys.stderr)
        print("A new key cannot read that data. Restore the .env from your recovery kit or backup into this folder, then start healthctl.", file=sys.stderr)
        print(f"To start over with an empty journal, first back up or remove that volume yourself (docker volume rm {DATA_VOLUME}), then run this script again.", file=sys.stderr)
        return 2
    if found is None:
        print("Docker is unavailable, so existing data could not be checked. If this computer already ran healthctl, restore its .env instead.", file=sys.stderr)
    template = (root / ".env.example").read_text()
    template = template.replace("POSTGRES_PASSWORD=\n", "POSTGRES_PASSWORD=" + secrets.token_hex(32) + "\n")
    key = base64.b64encode(secrets.token_bytes(32)).decode("ascii")
    template = template.replace("CLINICAL_DATA_KEY=\n", "CLINICAL_DATA_KEY=" + key + "\n")
    backups = args.backup_directory.expanduser().resolve()
    if backups == root or root in backups.parents:
        print("Choose a backup folder outside this checkout.", file=sys.stderr)
        return 2
    backups.mkdir(mode=0o700, parents=True, exist_ok=True)
    template = template.replace("CLINICAL_BACKUP_DIRECTORY=\n", "CLINICAL_BACKUP_DIRECTORY=" + str(backups) + "\n")
    if platform.system() == "Linux":
        template = template.replace("CLINICAL_BACKUP_USER=\n", f"CLINICAL_BACKUP_USER={os.getuid()}:{os.getgid()}\n")
    try:
        fd = os.open(env, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError:
        print("Existing .env preserved.")
        return 0
    with os.fdopen(fd, "w") as out:
        out.write(template)
    print("Created .env. Back it up securely alongside the database recovery procedure.")
    print(f"Scheduled backups will be written to {backups}.")
    print("Next, save a recovery kit: python3 scripts/recovery_kit.py")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
