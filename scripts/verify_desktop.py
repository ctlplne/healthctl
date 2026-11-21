"""Exercise the built macOS image and a disposable signed launcher workspace."""

import io
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import tempfile
import wave
from urllib.request import Request
from verify_restore import Client, demo

ROOT = Path(__file__).resolve().parents[1]


def verify(image, wav_source):
    with tempfile.TemporaryDirectory() as temp:
        work = Path(temp)
        mount = work / "mounted"
        mount.mkdir()
        state = work / "workspace"
        subprocess.run(
            [
                "hdiutil",
                "attach",
                "-readonly",
                "-nobrowse",
                "-mountpoint",
                str(mount),
                str(image),
            ],
            check=True,
            stdout=subprocess.DEVNULL,
        )
        app = mount / "healthctl.app"
        res = app / "Contents/Resources"
        env = dict(os.environ, HEALTHCTL_DESKTOP_STATE=str(state))
        password = secrets.token_urlsafe(26).encode() + b"\n"

        def manage(*args, data=b"", success=True):
            proc = subprocess.run(
                [res / "manage.sh", *args],
                capture_output=True,
                input=data,
                env=env,
                timeout=190,
            )
            if success and proc.returncode:
                raise RuntimeError(proc.stderr.decode()[-1500:])
            assert success == (proc.returncode == 0), "unexpected launcher result"
            return proc.stdout.decode().strip()

        try:
            subprocess.run(
                ["codesign", "--verify", "--deep", "--strict", app], check=True
            )
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
                        "text": words,
                        "revision": encounter["revision"],
                        "requestId": secrets.token_hex(12),
                    },
                )
            report = client.call(path + "/export")
            assert report["assessments"][-1]["items"]
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
                "Verified app signature, bundled startup, real offline speech, encrypted backup, wrong-passphrase recovery, restored diary and demo assessment equality, platform key configuration and fresh login."
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
            subprocess.run(
                ["hdiutil", "detach", str(mount)], check=True, stdout=subprocess.DEVNULL
            )


if __name__ == "__main__":
    if len(sys.argv) != 3:
        raise SystemExit(
            "Usage: python3 scripts/verify_desktop.py healthctl.dmg path/to/whisper.cpp/samples/jfk.wav"
        )
    verify(Path(sys.argv[1]).resolve(), Path(sys.argv[2]).resolve())
