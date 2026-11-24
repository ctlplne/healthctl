"""init.py must never mint a key that cannot read an existing journal."""
import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class InitTest(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())
        self.checkout = self.tmp / "checkout"
        (self.checkout / "scripts").mkdir(parents=True)
        shutil.copy(ROOT / "scripts" / "init.py", self.checkout / "scripts" / "init.py")
        shutil.copy(ROOT / ".env.example", self.checkout / ".env.example")
        self.bin = self.tmp / "bin"
        self.bin.mkdir()

    def tearDown(self):
        shutil.rmtree(self.tmp)

    def fake_docker(self, volumes):
        script = self.bin / "docker"
        script.write_text("#!/bin/sh\nprintf '%s'\n" % volumes)
        script.chmod(0o755)

    def run_init(self):
        env = dict(os.environ, PATH=f"{self.bin}{os.pathsep}/usr/bin:/bin", HOME=str(self.tmp / "home"))
        return subprocess.run([sys.executable, "-I", str(self.checkout / "scripts" / "init.py"), "--backup-directory", str(self.tmp / "backups")],
                              capture_output=True, text=True, env=env)

    def test_refuses_when_the_data_volume_exists(self):
        self.fake_docker("clinical-history_clinical_data\\n")
        result = self.run_init()
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("Refusing to create a new installation key", result.stderr)
        self.assertFalse((self.checkout / ".env").exists())

    def test_creates_a_private_env_with_an_outside_backup_folder(self):
        self.fake_docker("")
        result = self.run_init()
        self.assertEqual(result.returncode, 0, result.stderr)
        env = self.checkout / ".env"
        self.assertEqual(stat.S_IMODE(env.stat().st_mode), 0o600)
        text = env.read_text()
        self.assertIn("CLINICAL_BACKUP_DIRECTORY=" + str((self.tmp / "backups").resolve()), text)
        self.assertNotIn("CLINICAL_DATA_KEY=\n", text)
        self.assertNotIn(text.split("CLINICAL_DATA_KEY=")[1].split("\n")[0], result.stdout + result.stderr)
        self.assertTrue((self.tmp / "backups").is_dir())
        self.assertEqual(self.run_init().stdout.strip(), "Existing .env preserved.")

    def test_rejects_a_backup_folder_inside_the_checkout(self):
        self.fake_docker("")
        env = dict(os.environ, PATH=f"{self.bin}{os.pathsep}/usr/bin:/bin")
        result = subprocess.run([sys.executable, "-I", str(self.checkout / "scripts" / "init.py"), "--backup-directory", str(self.checkout / "backups")],
                                capture_output=True, text=True, env=env)
        self.assertEqual(result.returncode, 2)
        self.assertFalse((self.checkout / ".env").exists())


if __name__ == "__main__":
    unittest.main()
