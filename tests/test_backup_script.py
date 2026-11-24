"""scripts/backup.sh keeps snapshots by age, with a minimum count, and fails loudly."""
import os
from pathlib import Path
import stat
import subprocess
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parents[1]


class BackupScript(unittest.TestCase):
    def run_once(self, dump_ok=True):
        bin_dir = Path(self.tmp.name) / "bin"
        bin_dir.mkdir(exist_ok=True)
        tools = {
            "pg_dump": "#!/bin/sh\necho synthetic-dump\n" if dump_ok else "#!/bin/sh\nexit 1\n",
            "pg_restore": "#!/bin/sh\ncat >/dev/null 2>&1; exit 0\n",
            "psql": "#!/bin/sh\ncat >/dev/null; exit 0\n",
            "sleep": "#!/bin/sh\nexit 0\n",
        }
        for name, body in tools.items():
            p = bin_dir / name
            p.write_text(body)
            p.chmod(p.stat().st_mode | stat.S_IEXEC)
        env = dict(os.environ, PATH=str(bin_dir) + os.pathsep + os.environ["PATH"], BACKUP_DIR=str(self.data),
                   BACKUP_KEEP="2", BACKUP_KEEP_DAYS="7", BACKUP_INTERVAL_HOURS="24")
        return subprocess.run(["sh", str(ROOT / "scripts" / "backup.sh"), "once"], env=env, capture_output=True, text=True, timeout=60)

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.data = Path(self.tmp.name) / "data"
        self.data.mkdir()
        now = time.time()
        for name, age_days in [("clinical-20260101T000000Z.dump", 60), ("clinical-20260201T000000Z.dump", 40),
                               ("clinical-20261120T000000Z.dump", 4), ("clinical-20261122T000000Z.dump", 2)]:
            p = self.data / name
            p.write_text("old")
            os.utime(p, (now - age_days * 86400, now - age_days * 86400))

    def tearDown(self):
        self.tmp.cleanup()

    def test_old_snapshots_go_and_recent_ones_stay(self):
        r = self.run_once()
        self.assertEqual(r.returncode, 0, r.stderr)
        names = sorted(p.name for p in self.data.glob("clinical-*.dump"))
        self.assertNotIn("clinical-20260101T000000Z.dump", names)
        self.assertNotIn("clinical-20260201T000000Z.dump", names)
        self.assertIn("clinical-20261120T000000Z.dump", names, "a recent snapshot beyond the count must stay")
        self.assertEqual(len(names), 3)

    def test_a_failure_is_loud_and_changes_nothing(self):
        before = sorted(p.name for p in self.data.glob("clinical-*.dump"))
        r = self.run_once(dump_ok=False)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("backup: FAILED", r.stderr)
        self.assertEqual(sorted(p.name for p in self.data.glob("clinical-*.dump")), before)

    def test_bad_settings_fail_loudly(self):
        env = dict(os.environ, BACKUP_DIR=str(self.data), BACKUP_KEEP="2", BACKUP_KEEP_DAYS="none", BACKUP_INTERVAL_HOURS="24")
        r = subprocess.run(["sh", str(ROOT / "scripts" / "backup.sh"), "once"], env=env, capture_output=True, text=True, timeout=30)
        self.assertEqual(r.returncode, 2)
        self.assertIn("backup: FAILED: BACKUP_KEEP_DAYS", r.stderr)


if __name__ == "__main__":
    unittest.main()
