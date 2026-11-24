"""Write the installation key and database password from .env to .secrets/.

compose.secrets.yaml mounts these files into the read-only app and backup
containers. The folder is private to you (0700); the files must be readable by
the container's user, so they are 0644 inside it. The values are never printed.

  python3 scripts/secret_files.py [--env-file .env] [--folder .secrets]
"""
import argparse
import os
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
NAMES = {"CLINICAL_DATA_KEY": "clinical_data_key", "POSTGRES_PASSWORD": "database_password"}


def read_env(path):
    values = {}
    for line in path.read_text().splitlines():
        if "=" in line and not line.lstrip().startswith("#"):
            key, value = line.split("=", 1)
            values[key.strip()] = value.strip()
    return values


def write(env_file, folder):
    values = read_env(env_file)
    missing = [k for k in NAMES if not values.get(k)]
    if missing:
        raise SystemExit("secret_files: " + env_file.name + " has no " + ", ".join(missing) + "; run python3 scripts/init.py first")
    folder.mkdir(mode=0o700, exist_ok=True)
    os.chmod(folder, 0o700)
    for key, name in NAMES.items():
        target = folder / name
        tmp = folder / (name + ".tmp")
        fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o644)
        with os.fdopen(fd, "w") as f:
            f.write(values[key])
        os.chmod(tmp, 0o644)
        os.replace(tmp, target)
    return [folder / n for n in NAMES.values()]


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--env-file", default=str(ROOT / ".env"))
    parser.add_argument("--folder", default=str(ROOT / ".secrets"))
    args = parser.parse_args()
    written = write(Path(args.env_file), Path(args.folder))
    print("Wrote " + ", ".join(p.name for p in written) + " to " + str(Path(args.folder)) + ". Start with -f compose.yaml -f compose.secrets.yaml.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
