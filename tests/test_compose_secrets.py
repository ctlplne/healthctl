"""Secrets can reach the containers as files without weakening read-only containers."""
import importlib.util
from pathlib import Path
import re
import stat
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


def service(text, name):
    m = re.search(r"^  " + name + r":\n((?:    .*\n|\n)+)", text, re.M)
    if not m:
        raise AssertionError("no service " + name)
    return m.group(1)


class ComposeSecrets(unittest.TestCase):
    def test_database_reads_its_password_from_a_file(self):
        text = (ROOT / "compose.yaml").read_text()
        db = service(text, "db")
        self.assertIn("POSTGRES_PASSWORD_FILE: /run/secrets/database_password", db)
        self.assertNotRegex(db, r"\n\s+POSTGRES_PASSWORD:")
        self.assertRegex(text, r"\n  database_password:\n    environment: POSTGRES_PASSWORD\n")
        # Compose can only mount file secrets into read-only services, and
        # the app must stay read-only.
        self.assertIn("read_only: true", service(text, "app"))

    def test_override_passes_files_to_the_app_and_backup(self):
        text = (ROOT / "compose.secrets.yaml").read_text()
        app, backup = service(text, "app"), service(text, "backup")
        self.assertIn("CLINICAL_DATA_KEY_FILE: /run/secrets/clinical_data_key_file", app)
        self.assertIn("PGPASSWORD_FILE: /run/secrets/database_password_file", app)
        self.assertIn('CLINICAL_DATA_KEY: ""', app)
        self.assertIn("PGPASSWORD_FILE: /run/secrets/database_password_file", backup)
        self.assertIn("file: ./.secrets/clinical_data_key", text)

    def test_secret_files_script(self):
        spec = importlib.util.spec_from_file_location("secret_files", ROOT / "scripts" / "secret_files.py")
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        with tempfile.TemporaryDirectory() as d:
            env = Path(d) / ".env"
            env.write_text("CLINICAL_DATA_KEY=synthetic-key\nPOSTGRES_PASSWORD=synthetic-password\n")
            folder = Path(d) / ".secrets"
            mod.write(env, folder)
            self.assertEqual((folder / "clinical_data_key").read_text(), "synthetic-key")
            self.assertEqual(stat.S_IMODE(folder.stat().st_mode), 0o700)
            env.write_text("POSTGRES_PASSWORD=x\n")
            with self.assertRaises(SystemExit):
                mod.write(env, folder)


if __name__ == "__main__":
    unittest.main()
