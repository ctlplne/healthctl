"""Exercise the built macOS image and a disposable signed launcher workspace."""

import argparse
from contextlib import ExitStack
import hashlib
import io
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import wave
from urllib.request import Request
from verify_restore import Client, demo

ROOT = Path(__file__).resolve().parents[1]


def verify(package, wav_source, resources_only=False):
    with tempfile.TemporaryDirectory() as temp, ExitStack() as mounts:
        work = Path(temp)
        state = work / "workspace"
        if resources_only:
            res = package
        else:
            mount = work / "mounted"
            mount.mkdir()
            subprocess.run(
                ["hdiutil", "attach", "-readonly", "-nobrowse", "-mountpoint", str(mount), str(package)],
                check=True, stdout=subprocess.DEVNULL,
            )
            mounts.callback(subprocess.run, ["hdiutil", "detach", str(mount)], check=True, stdout=subprocess.DEVNULL)
            app = mount / "healthctl.app"
            subprocess.run(["codesign", "--verify", "--deep", "--strict", app], check=True)
            res = app / "Contents/Resources"
        notices = res / "notices"
        source_archive = res / "healthctl-source.tar.gz"
        image_packages = {"app": {"poppler-utils", "tesseract-ocr", "whisper.cpp", "ggml-tiny.en-q5_1.bin", "nih-adult-reference", "react"},
                          "db": {"postgresql", "musl"}}
        for name, required in ({} if source_archive.exists() else image_packages).items():
            inventory = json.loads((notices / (name + ".spdx.json")).read_bytes())
            components = {p["name"] for p in inventory["packages"]}
            if inventory["spdxVersion"] != "SPDX-2.3" or required - components:
                raise ValueError(f"Incomplete {name} SBOM: {required - components}")
        if not resources_only:
            for source, bundled in [('internal/clinical/preview.txt', 'preview.txt'), ('docs/use-notice.md', 'use-notice.md'), ('LICENSE', 'LICENSE')]:
                if (res / bundled).read_bytes() != (ROOT / source).read_bytes():
                    raise ValueError(f"Packaged notice differs from the source: {bundled}")
            native = json.loads((notices / "desktop.spdx.json").read_bytes())
            if source_archive.exists():
                packaged = next(p for p in native["packages"] if p["SPDXID"] == "SPDXRef-source")
                revision = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
                if (packaged["versionInfo"] != revision or
                        packaged["checksums"][0]["checksumValue"] != hashlib.sha256(source_archive.read_bytes()).hexdigest()):
                    raise ValueError("Packaged source differs from the release under test")
            else:
                refs = {r["externalDocumentId"]: r["checksum"]["checksumValue"] for r in native["externalDocumentRefs"]}
                for name in image_packages:
                    if refs["DocumentRef-" + name] != hashlib.sha256((notices / (name + ".spdx.json")).read_bytes()).hexdigest():
                        raise ValueError("Desktop SBOM does not match embedded " + name + " inventory")
            if not (notices / "archive-go/go-LICENSE").is_file() or not (res / "THIRD_PARTY_NOTICES.md").is_file():
                raise ValueError("Missing native distribution notices")
        env = dict(os.environ, HEALTHCTL_DESKTOP_STATE=str(state))
        password = secrets.token_urlsafe(26).encode() + b"\n"

        def manage(*args, data=b"", success=True):
            proc = subprocess.run(
                [res / "manage.sh", *args],
                capture_output=True,
                input=data,
                env=env,
                timeout=1800 if source_archive.exists() and args[0] == "start" else 190,
            )
            if success and proc.returncode:
                raise RuntimeError(proc.stderr.decode()[-1500:])
            assert success == (proc.returncode == 0), "unexpected launcher result"
            return proc.stdout.decode().strip()

        try:
            original = manage("start")
            port = original.rstrip("/").rsplit(":", 1)[1]
            client = Client(port)
            owner = {
                "username": "desktop-synthetic",
                "password": secrets.token_urlsafe(24),
            }
            setup = client.call("/setup", "POST", owner)
            client.call("/settings", "PUT", {"provider": demo})
            encounter = client.call(
                "/encounters",
                "POST",
                {
                    "age": 35,
                    "concern": "My nose is runny and I keep sneezing.",
                    "pregnancyRelated": False,
                    "urgentNow": False,
                    "noticeVersion": setup["noticeVersion"],
                    "requestId": secrets.token_hex(12),
                    "disclosedProvider": demo,
                },
            )
            path = "/encounters/" + encounter["id"]
            for action, words in [
                ("answer", "Yesterday"),
                ("answer", "After being outdoors"),
                ("review", ""),
                ("confirm", ""),
                ("assess", ""),
            ]:
                encounter = client.call(
                    path + "/actions",
                    "POST",
                    {
                        "action": action,
                        **({"assessmentAcknowledged": True, "assessmentNoticeVersion": setup["assessmentNotice"]["version"]} if action == "assess" else {}),
                        "text": words,
                        "revision": encounter["revision"],
                        "requestId": secrets.token_hex(12),
                    },
                )
            report = client.call(path + "/export")
            assert report["assessments"][-1]["items"]
            assert "Verify findings with a qualified healthcare professional" in report["previewNotice"]
            manage("platform-key", data=b"sk-fictional-local-drill-key\n")
            assert client.call("/voice")["openai"]
            manage("platform-key", data=b"\n")
            assert not client.call("/voice")["openai"]
            item = client.call(
                "/diary",
                "POST",
                {
                    "id": secrets.token_hex(16),
                    "onset": "2025-01-13T07:05:00Z",
                    "text": "Synthetic diary observation during a walk.",
                },
            )
            caps = client.call("/voice")
            assert caps["local"] and not caps["chatgptPlan"]
            with wave.open(str(wav_source)) as source:
                assert (
                    source.getframerate() == 16000
                    and source.getnchannels() == 1
                    and source.getsampwidth() == 2
                )
                wav = io.BytesIO()
                with wave.open(wav, "wb") as out:
                    out.setparams(source.getparams())
                    out.writeframes(source.readframes(source.getnframes()))
            request = Request(
                client.url + "/voice?destination=local",
                method="POST",
                data=wav.getvalue(),
                headers={"X-CSRF-Token": client.csrf, "Content-Type": "audio/wav"},
            )
            with client.browser.open(request, timeout=120) as response:
                transcript = json.load(response)["draft"]
            assert (
                "country" in transcript.lower()
            ), "offline reference speech did not transcribe"
            assert client.call("/diary") == [item], "draft modified a patient record"
            backup = work / "fixture.healthctl"
            manage("backup", backup, data=password)
            assert b"Synthetic diary observation" not in backup.read_bytes()
            manage(
                "restore",
                backup,
                data=b"incorrect synthetic passphrase\n",
                success=False,
            )
            assert manage("start") == original
            restored = manage("restore", backup, data=password)
            assert restored != original
            dest = Client(restored.rstrip("/").rsplit(":", 1)[1])
            assert not dest.call("/session")["authenticated"]
            dest.call("/login", "POST", owner)
            assert dest.call("/diary") == [item]
            recovered = dest.call(path + "/export")
            for record in (report, recovered):
                record.pop("exportedAt")
            assert report == recovered
            print(
                "Verified bundled startup, real offline speech, encrypted backup, wrong-passphrase recovery, restored diary and demo assessment equality, platform key configuration and fresh login."
            )
        finally:
            for folder in state.glob("healthctl-desktop-*"):
                imgs = (res / "images.txt").read_text().split()
                subprocess.run(
                    [
                        "docker",
                        "compose",
                        "--env-file",
                        str(folder / "settings.env"),
                        "-p",
                        folder.name.lower(),
                        "-f",
                        res / "compose.yaml",
                        "-f",
                        res / "desktop.yaml",
                        "down",
                        "-v",
                    ],
                    env=dict(
                        env, HEALTHCTL_APP_IMAGE=imgs[0], HEALTHCTL_DB_IMAGE=imgs[1]
                    ),
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL,
                )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("package", type=Path, help="macOS disk image or staged resources directory")
    parser.add_argument("recording", type=Path)
    parser.add_argument("--resources-only", action="store_true", help="Exercise bundled services on a native Linux runner")
    args = parser.parse_args()
    verify(args.package.resolve(), args.recording.resolve(), args.resources_only)
