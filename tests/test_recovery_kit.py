"""The recovery kit is private, never printed, and matches the app's fingerprint."""
import importlib.util
from pathlib import Path
import stat
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
KEY = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="  # same vector as TestRecoveryKitFingerprint


class RecoveryKitTest(unittest.TestCase):
    def test_fingerprint_matches_the_server(self):
        spec = importlib.util.spec_from_file_location("recovery_kit", ROOT / "scripts" / "recovery_kit.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        self.assertEqual(module.fingerprint(KEY), "5bde-af0d-59df")

    def test_writes_a_private_kit_without_printing_the_key(self):
        with tempfile.TemporaryDirectory() as tmp:
            env = Path(tmp) / "settings.env"
            env.write_text(f"POSTGRES_PASSWORD=synthetic\nCLINICAL_DATA_KEY={KEY}\n")
            kit = Path(tmp) / "kit.txt"
            run = lambda: subprocess.run([sys.executable, "-I", str(ROOT / "scripts" / "recovery_kit.py"), "--env-file", str(env), "--output", str(kit)], capture_output=True, text=True)
            result = run()
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertNotIn(KEY, result.stdout + result.stderr)
            self.assertIn("5bde-af0d-59df", result.stdout)
            self.assertEqual(stat.S_IMODE(kit.stat().st_mode), 0o600)
            self.assertIn(KEY, kit.read_text())
            self.assertNotEqual(run().returncode, 0, "an existing kit must not be overwritten")


if __name__ == "__main__":
    unittest.main()
