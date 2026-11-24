"""scripts/upgrade.py snapshots before it changes anything and can roll back."""
import datetime
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("upgrade", ROOT / "scripts" / "upgrade.py")
upgrade = importlib.util.module_from_spec(spec)
spec.loader.exec_module(upgrade)


class Recorder:
    def __init__(self, fail_on=None):
        self.calls, self.fail_on = [], fail_on

    def __call__(self, cmd, stdin=None, stdout=None):
        joined = " ".join(cmd)
        self.calls.append(joined)
        if self.fail_on and self.fail_on in joined:
            raise upgrade.Failed(joined + " failed")
        if stdout is not None:
            stdout.write(b"PGDMP synthetic dump")


class UpgradeTests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.folder = Path(self.dir.name)
        upgrade.STATE = self.folder / ".upgrade" / "last.json"
        self.now = datetime.datetime(2026, 11, 24, 12, 0, tzinfo=datetime.timezone.utc)

    def tearDown(self):
        self.dir.cleanup()

    def test_snapshot_comes_before_any_change(self):
        run = Recorder()
        snap = upgrade.upgrade(run, self.now, self.folder)
        dump = next(i for i, c in enumerate(run.calls) if "pg_dump" in c)
        verify = next(i for i, c in enumerate(run.calls) if "pg_restore --list" in c)
        build = next(i for i, c in enumerate(run.calls) if "compose build" in c)
        self.assertLess(dump, verify)
        self.assertLess(verify, build)
        self.assertTrue(snap.exists() and snap.name == "pre-upgrade-20261124T120000Z.dump")
        self.assertEqual(snap.stat().st_mode & 0o777, 0o600)
        state = json.loads(upgrade.STATE.read_text())
        self.assertEqual(state["image"], "clinical-history-app:previous")

    def test_a_failed_snapshot_stops_the_upgrade(self):
        run = Recorder(fail_on="pg_restore --list")
        with self.assertRaises(upgrade.Failed):
            upgrade.upgrade(run, self.now, self.folder)
        self.assertFalse(any("compose build" in c or "docker tag" in c for c in run.calls))
        self.assertFalse(upgrade.STATE.exists())

    def test_rollback_restores_data_then_the_previous_image(self):
        upgrade.upgrade(Recorder(), self.now, self.folder)
        run = Recorder()
        upgrade.rollback(run)
        order = [next(i for i, c in enumerate(run.calls) if key in c) for key in ("stop app", "dropdb", "createdb", "pg_restore -U", "docker tag clinical-history-app:previous", "up -d --no-build")]
        self.assertEqual(order, sorted(order))

    def test_rollback_needs_a_recorded_snapshot(self):
        with self.assertRaises(upgrade.Failed):
            upgrade.rollback(Recorder())


if __name__ == "__main__":
    unittest.main()
