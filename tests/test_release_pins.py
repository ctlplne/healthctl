"""Release inputs are pinned: images by digest and ID, downloads by checksum and revision."""
from pathlib import Path
import os
import re
import stat
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class DockerfilePins(unittest.TestCase):
    def test_every_base_image_has_a_digest(self):
        for line in (ROOT / "Dockerfile").read_text().splitlines():
            if line.startswith("FROM ") and " AS " in line and "${" not in line:
                image = line.split()[1]
                if ":" in image and not image.split(":")[0] in {"ui", "server", "readers", "speech"}:
                    self.assertIn("@sha256:", image, line)

    def test_downloads_are_checksummed_and_the_model_is_pinned(self):
        text = (ROOT / "Dockerfile").read_text()
        for line in text.splitlines():
            if "curl " in line:
                self.assertIn("sha256sum -c", line, line)
        model = re.search(r"huggingface\.co/ggerganov/whisper\.cpp/resolve/([^/]+)/", text)
        self.assertTrue(model and re.fullmatch(r"[0-9a-f]{40}", model.group(1)), "the speech model must name a commit, not a branch")


class PackagedSources(unittest.TestCase):
    """The desktop packages build the app from a source archive on first use.
    The archive must hold every file the Dockerfile copies from the repo."""

    def dockerfile_sources(self):
        sources = set()
        for line in (ROOT / "Dockerfile").read_text().splitlines():
            if not line.startswith("COPY ") or "--from=" in line:
                continue
            sources.update(p for p in line.split()[1:-1] if not p.startswith("--"))
        return sources

    def missing(self, inputs):
        def covered(path):
            return any(path == i or path.startswith(i.rstrip("/") + "/") for i in inputs)
        return sorted(p for p in self.dockerfile_sources() if not covered(p))

    def test_windows_source_archive_covers_the_dockerfile(self):
        block = re.search(r"inputs = \[(.*?)\]", (ROOT / "scripts/build-windows.py").read_text(), re.S).group(1)
        self.assertEqual(self.missing(re.findall(r"'([^']+)'", block)), [], "scripts/build-windows.py leaves these out of its source archive")

    def test_mac_first_use_archive_covers_the_dockerfile(self):
        text = (ROOT / "scripts/build-desktop.sh").read_text()
        block = re.search(r"git archive [^\n]*-- \\\n(.*?)\n\s*dbimage=", text, re.S).group(1)
        self.assertEqual(self.missing(block.replace("\\", " ").split()), [], "scripts/build-desktop.sh leaves these out of its source archive")


class DesktopBundle(unittest.TestCase):
    """Every helper file manage.sh runs from its resources is copied wherever a
    desktop bundle is assembled: the Mac build and the qualification job."""

    def test_bundles_include_the_helpers_manage_runs(self):
        manage = (ROOT / "desktop/manage.sh").read_text()
        helpers = sorted({m for m in re.findall(r"\$resources/([\w.-]+)", manage) if (ROOT / "desktop" / m).is_file()})
        self.assertIn("verify-images.sh", helpers)
        qualification = re.search(r"cp desktop/manage\.sh[^\n]*build/resources/", (ROOT / ".github/workflows/qualification.yml").read_text()).group(0)
        mac = re.search(r"cp compose\.yaml desktop/desktop\.yaml[^\n]*", (ROOT / "scripts/build-desktop.sh").read_text()).group(0)
        for helper in helpers:
            self.assertIn("desktop/" + helper, qualification, "the qualification bundle lacks " + helper)
            self.assertIn("desktop/" + helper, mac, "the Mac bundle lacks " + helper)


class DesktopImages(unittest.TestCase):
    def run_check(self, ids, expected):
        with tempfile.TemporaryDirectory() as d:
            fake = Path(d) / "docker"
            fake.write_text("#!/bin/sh\n# fake docker: image inspect --format ID NAME\ncase \"$5\" in\n" +
                            "".join(f'  {name}) echo {value};;\n' for name, value in ids.items()) + "  *) exit 1;;\nesac\n")
            fake.chmod(fake.stat().st_mode | stat.S_IEXEC)
            env = dict(os.environ, PATH=d + os.pathsep + os.environ["PATH"])
            return subprocess.run(["bash", str(ROOT / "desktop" / "verify-images.sh"), *expected], env=env, capture_output=True, text=True)

    def test_matching_images_start(self):
        self.assertEqual(self.run_check({"app": "sha256:aaa", "db": "sha256:bbb"}, ["app", "sha256:aaa", "db", "sha256:bbb"]).returncode, 0)

    def test_a_different_image_is_refused(self):
        r = self.run_check({"app": "sha256:evil", "db": "sha256:bbb"}, ["app", "sha256:aaa", "db", "sha256:bbb"])
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("expected sha256:aaa", r.stderr)

    def test_launcher_prefers_prebuilt_images(self):
        text = (ROOT / "desktop" / "manage.sh").read_text()
        self.assertLess(text.index('images.tar.gz" ]]'), text.index('healthctl-source.tar.gz" ]]'))
        self.assertIn("verify-images.sh", text)


class WindowsChannel(unittest.TestCase):
    def test_legacy_windows_tags_are_retired(self):
        text = (ROOT / ".github" / "workflows" / "windows.yml").read_text()
        self.assertNotIn("windows-*", text)
        self.assertNotIn("gh release create", text)


if __name__ == "__main__":
    unittest.main()
