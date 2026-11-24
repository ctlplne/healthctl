"""The installation key and database password reach containers as files."""
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[1]


def service(text, name):
    m = re.search(r"^  " + name + r":\n((?:    .*\n|\n)+)", text, re.M)
    if not m:
        raise AssertionError("no service " + name)
    return m.group(1)


class ComposeSecrets(unittest.TestCase):
    def setUp(self):
        self.text = (ROOT / "compose.yaml").read_text()

    def test_app_reads_secrets_from_files(self):
        app = service(self.text, "app")
        self.assertNotRegex(app, r"\n\s+CLINICAL_DATA_KEY:", "the key must not be an environment variable")
        self.assertNotRegex(app, r"\n\s+PGPASSWORD:", "the password must not be an environment variable")
        self.assertIn("CLINICAL_DATA_KEY_FILE: /run/secrets/clinical_data_key", app)
        self.assertIn("PGPASSWORD_FILE: /run/secrets/database_password", app)
        self.assertIn("secrets: [clinical_data_key, database_password]", app)

    def test_database_and_backup_use_the_password_file(self):
        for name in ("db", "backup"):
            block = service(self.text, name)
            self.assertNotRegex(block, r"\n\s+(POSTGRES_PASSWORD|PGPASSWORD):", name)
            self.assertIn("secrets: [database_password]", block)

    def test_secrets_are_defined(self):
        self.assertRegex(self.text, r"\nsecrets:\n(?:  .*\n)*  clinical_data_key:\n    environment: CLINICAL_DATA_KEY\n")
        self.assertRegex(self.text, r"\n  database_password:\n    environment: POSTGRES_PASSWORD\n")


if __name__ == "__main__":
    unittest.main()
