"""Initialize an installation exactly once without printing any secrets."""
import base64
import os
from pathlib import Path
import secrets

root = Path(__file__).resolve().parents[1]
template = (root / ".env.example").read_text()
template = template.replace("POSTGRES_PASSWORD=\n", "POSTGRES_PASSWORD=" + secrets.token_hex(32) + "\n")
key = base64.b64encode(secrets.token_bytes(32)).decode("ascii")
template = template.replace("CLINICAL_DATA_KEY=\n", "CLINICAL_DATA_KEY=" + key + "\n")
try:
    fd = os.open(root / ".env", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
except FileExistsError:
    print("Existing .env preserved.")
else:
    with os.fdopen(fd, "w") as out:
        out.write(template)
    print("Created .env. Back it up securely alongside the database recovery procedure.")
