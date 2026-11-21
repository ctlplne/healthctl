"""Exercise the combined release on isolated local volumes with fictional reports."""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import tempfile
import time
from urllib.error import HTTPError
from urllib.request import Request

import synthetic_reports as samples
from read_brief import read_brief
from verify_restore import Client, demo, owner

ROOT = Path(__file__).resolve().parents[1]
MODEL_GUIDANCE = (ROOT / "internal/clinical/model-guidance.txt").read_text().strip()


def same(left, right, message):
    if left != right:
        raise AssertionError(message)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--v1-image", default="clinical-history:verification-v1")
    parser.add_argument("--out", type=Path, default=ROOT / "build/release-drill.json")
    parser.add_argument(
        "--profile-validator",
        action="store_true",
        help="Also run the official HL7 FHIR validator image offline.",
    )
    parser.add_argument(
        "--study-pair",
        action="store_true",
        help="Save a matched pair of genuine printed synthetic reports for instrument practice.",
    )
    args = parser.parse_args()
    base = secrets.token_hex(5)
    source, dest, rollback = (
        "clinical-upgrade-" + base,
        "clinical-restore-" + base,
        "clinical-rollback-" + base,
    )
    results = []

    def passed(test):
        results.append(test)
        print("PASS", test, flush=True)

    with tempfile.TemporaryDirectory() as directory:
        workspace = Path(directory)
        config = workspace / ".env"
        config.write_text(
            "CLINICAL_DATA_KEY="
            + base64.b64encode(secrets.token_bytes(32)).decode()
            + "\nPOSTGRES_PASSWORD="
            + secrets.token_hex(32)
            + "\n"
        )
        config.chmod(0o600)
        v1 = workspace / "v1.yaml"
        v1.write_text(
            "services:\n  app:\n    image: " + json.dumps(args.v1_image) + "\n"
        )
        env = dict(
            os.environ,
            CLINICAL_PUBLIC_ORIGIN="",
            CLINICAL_SECURE_COOKIE="false",
            OPENAI_API_KEY="",
            ANTHROPIC_API_KEY="",
            BYOM_API_KEY="",
            CLINICAL_ALLOWED_ENDPOINTS="http://fixture:8090/v1",
            CLINICAL_BACKUP_KEEP="1",
            CLINICAL_BACKUP_INTERVAL_HOURS="24",
        )
        for name in (
            "CLINICAL_DATA_KEY",
            "POSTGRES_PASSWORD",
            "CLINICAL_BACKUP_DIRECTORY",
        ):
            env.pop(name, None)
        # The upgrade drill exercises the self-contained installation. The
        # separate browser suite provisions its own fictional SMART endpoints.
        for name in ("NAME", "FHIR_BASE", "AUTHORIZE_URL", "TOKEN_URL", "CLIENT_ID", "REDIRECT_URI", "AUTH_ISSUER"):
            env["CLINICAL_EHR_" + name] = ""

        def compose(project, *command, old=False, **kwargs):
            files = ["-f", "compose.yaml", "-f", "compose.verify.yaml"]
            if old:
                files += ["-f", str(v1)]
            port = {source: "18441", dest: "18442", rollback: "18443"}[project]
            local = dict(env, CLINICAL_PORT=port)
            return subprocess.run(
                [
                    "docker",
                    "compose",
                    "--env-file",
                    str(config),
                    "-p",
                    project,
                    *files,
                    *command,
                ],
                cwd=ROOT,
                env=local,
                check=True,
                **kwargs,
            )

        def print_handout(candidate):
            pair = args.out.resolve().parent / "study-pair"
            pair.mkdir(parents=True, exist_ok=True)
            target = pair / ("brief.pdf" if candidate else "baseline.pdf")
            with target.open("wb") as output:
                subprocess.run(
                    [
                        "docker",
                        "run",
                        "--rm",
                        "-i",
                        "--network",
                        "container:" + source + "-app-1",
                        "--shm-size=512m",
                        "-v",
                        f"{ROOT / 'tests/print-study.cjs'}:/ui/print-study.cjs:ro",
                        "clinical-history-qa-browser-tests",
                        "node",
                        "/ui/print-study.cjs",
                    ],
                    input=json.dumps(
                        {
                            "owner": owner,
                            "concern": "My nose is runny and I keep sneezing.",
                            "candidate": candidate,
                        }
                    ).encode(),
                    stdout=output,
                    check=True,
                )
            if candidate:
                for filename in ("baseline", "brief"):
                    data = (pair / (filename + ".pdf")).read_bytes()
                    readable = compose(
                        source,
                        "exec",
                        "-T",
                        "app",
                        "pdftotext",
                        "-layout",
                        "-",
                        "-",
                        input=data,
                        stdout=subprocess.PIPE,
                    ).stdout
                    assert b"sneezing" in readable.lower() and (
                        b"unreviewed" in readable.lower()
                        or b"not reviewed" in readable.lower()
                    )
                    if filename == "brief":
                        pages = readable.decode().strip().split("\f")
                        assert len(pages) == 1 and MODEL_GUIDANCE in " ".join(pages[0].split())
                    (pair / (filename + ".txt")).write_bytes(readable)
                manifest = {
                    "study": "Printed handoff rehearsal",
                    "practiceOnly": True,
                    "cases": [
                        {
                            "id": "nasal-print-rehearsal",
                            "complaint": "nasal symptoms",
                            "group": "adult",
                            "baseline": "baseline.pdf",
                            "candidate": "brief.pdf",
                            "expectedUrgent": False,
                            "expectedAbstain": False,
                            "facilitatorKey": {
                                "concern": "Runny nose and sneezing",
                                "timeline": "Yesterday, after being outdoors",
                                "warning": "No active warning was shown in this fictional encounter",
                                "outstanding": "Use the report's explicitly stated limitations",
                                "patientUnderstanding": "An AI-generated unreviewed estimate to discuss at a visit.",
                            },
                        }
                    ],
                }
                (pair / "case.json").write_text(json.dumps(manifest, indent=2) + "\n")
                passed(
                    "Matched handouts printed by v1 and the combined UI for the same synthetic visit"
                )

        def act(c, e, action, **fields):
            return c.call(
                "/encounters/" + e["id"] + "/actions",
                "POST",
                dict(
                    fields,
                    action=action,
                    requestId=secrets.token_hex(12),
                    revision=e["revision"],
                ),
            )

        def create(c, cfg, concern, report=False):
            state = c.call("/session")
            c.call("/settings", "PUT", {"provider": cfg})
            body = dict(
                concern=concern,
                age=35,
                pregnancyRelated=False,
                urgentNow=False,
                noticeVersion=state["noticeVersion"],
                disclosedProvider=cfg,
                requestId=secrets.token_hex(12),
            )
            if report:
                body["reportFirst"] = True
            return c.call("/encounters", "POST", body)

        def file_request(c, e, filename, data, key=None):
            boundary = secrets.token_hex(16)
            body = bytearray()
            for name, value in {
                "requestId": key or secrets.token_hex(12),
                "revision": str(e["revision"]),
                "belongsTo": "self",
                "purpose": "A fictional test",
                "priority": "Discuss changes",
            }.items():
                body.extend(
                    f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n{value}\r\n'.encode()
                )
            body.extend(
                f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{filename}"\r\n\r\n'.encode()
                + data
                + f"\r\n--{boundary}--\r\n".encode()
            )
            req = Request(
                c.url + "/encounters/" + e["id"] + "/documents",
                data=body,
                method="POST",
                headers={
                    "Content-Type": "multipart/form-data; boundary=" + boundary,
                    "X-CSRF-Token": c.csrf,
                },
            )
            with c.browser.open(req, timeout=90) as res:
                return json.load(res)

        try:
            subprocess.run(
                ["docker", "image", "inspect", args.v1_image],
                check=True,
                stdout=subprocess.DEVNULL,
            )
            compose(source, "up", "--no-build", "-d", "--wait", "db", "app", old=True)
            c = Client("18441")
            c.call("/setup", "POST", owner)
            v = create(c, demo, "My nose is runny and I keep sneezing.")
            for name, text in [
                ("answer", "Yesterday"),
                ("answer", "After being outdoors"),
                ("review", ""),
                ("confirm", ""),
                ("assess", ""),
            ]:
                v = act(c, v, name, text=text)
            vpath = "/encounters/" + v["id"]
            old = c.call(vpath + "/export")
            assert old["assessments"][0]["items"]
            passed("Actual v1 fixture installed, completed, assessed and exported")
            if args.study_pair:
                print_handout(False)
            snapshot = workspace / "pre-upgrade.dump"
            with snapshot.open("wb") as out:
                compose(
                    source,
                    "exec",
                    "-T",
                    "db",
                    "pg_dump",
                    "-U",
                    "clinicalhistory",
                    "-d",
                    "clinicalhistory",
                    "-Fc",
                    "--no-owner",
                    "--no-acl",
                    "--exclude-table-data=public.sessions",
                    stdout=out,
                )
            compose(source, "up", "-d", "--build", "--wait", "app", "fixture")
            upgraded = c.call(vpath + "/export")
            same(c.call("/ehr"), {"enabled": False}, "unconfigured portal attempted to enable itself")
            passed("Upgraded installation keeps portal integration disabled until the operator configures it")
            for field in (
                "historyRevision",
                "answers",
                "facts",
                "warnings",
                "summaries",
                "assessments",
                "notice",
                "reviewStatus",
            ):
                same(old[field], upgraded[field], "v1 migration changed " + field)
            same(upgraded["modelGuidance"], MODEL_GUIDANCE, "old encounter export lost current model guidance")
            passed(
                "Upgrade preserved the v1 owner, active session, evidence, warning state and historical assessment"
            )
            if args.study_pair:
                print_handout(True)
            compose(rollback, "up", "-d", "--wait", "db", old=True)
            with snapshot.open("rb") as inp:
                compose(
                    rollback,
                    "exec",
                    "-T",
                    "db",
                    "pg_restore",
                    "-U",
                    "clinicalhistory",
                    "-d",
                    "clinicalhistory",
                    "--exit-on-error",
                    "--single-transaction",
                    "--no-owner",
                    "--no-acl",
                    stdin=inp,
                )
            compose(rollback, "up", "-d", "--no-build", "--wait", "app", old=True)
            previous = Client("18443")
            previous.call("/login", "POST", owner)
            rolled = previous.call(vpath + "/export")
            same(
                {k: v for k, v in rolled.items() if k != "exportedAt"},
                {k: v for k, v in old.items() if k != "exportedAt"},
                "rollback report changed",
            )
            passed(
                "Application rollback restored the pre-upgrade snapshot under the actual v1 binary and reproduced the original export"
            )
            assert c.call("/region")["emergency"] == ""
            env.update(CLINICAL_LOCALE="en-US", CLINICAL_UNITS="us-customary")
            compose(source, "up", "-d", "--wait", "--no-build", "app")
            same(c.call("/region")["locale"], "en-US", "US locale not applied")
            passed("Generic emergency default and US English regional configuration")
            archived = upgraded["assessments"][0]
            e = create(
                c,
                {
                    "kind": "compatible",
                    "model": "fixture-report",
                    "structured": "auto",
                    "endpoint": "http://fixture:8090/v1",
                },
                "It burns when I urinate.",
                True,
            )
            path = "/encounters/" + e["id"]
            assert not e.get("pending") and not e.get("turns")
            txt = (
                "Patient name: DO-NOT-DISCLOSE\nTest | Result | Collected\n"
                "Leukocyte esterase | Trace | 2026-10-09\n"
            ).encode()
            receipt = secrets.token_hex(12)
            first = file_request(c, e, "urine.txt", txt, receipt)
            duplicate = file_request(c, e, "urine.txt", txt, receipt)
            same(
                first["revision"],
                duplicate["revision"],
                "duplicate upload changed record",
            )
            e = first
            d = e["documents"][0]
            row = next(x for x in d["rows"] if x["name"] == "Leukocyte esterase")
            assert row["value"] == "Trace" and row["collectionDate"] == "2026-10-09"
            row["subject"] = "self"
            e = act(
                c, e, "review_report_row", documentId=d["id"], rowId=row["id"], row=row
            )
            preview = c.call(
                path + "/documents/" + d["id"] + "/preview",
                "POST",
                {"rowIds": [row["id"]]},
            )
            assert len(
                preview["extracts"]
            ) == 1 and "DO-NOT-DISCLOSE" not in json.dumps(preview)
            e = act(c, e, "authorize_report", documentId=d["id"], rowIds=[row["id"]])
            e = act(c, e, "continue")
            assert e["questionSource"]["rowId"] == row["id"]
            e = act(c, e, "answer", text="Before the sample")
            if e["state"] != "history_review":
                e = act(c, e, "review")
            e = act(c, e, "confirm")
            e = act(c, e, "assess")
            assert (
                e["assessments"][-1]["outcome"] == "differential"
                and e["assessments"][-1]["items"]
            )
            passed(
                "Report-first compatibility wire fixture, line provenance, corrected context, consent and idempotence"
            )
            token = c.call(path + "/access", "POST", {})["token"]
            handoff = read_brief(c.url[:-4], token)
            same(handoff["modelGuidance"], MODEL_GUIDANCE, "scoped handoff lost model guidance")
            assert handoff["reportFindings"][0][
                "locator"
            ] and "DO-NOT-DISCLOSE" not in json.dumps(handoff)
            bundle = c.call(path + "/fhir")
            subprocess.run(
                [
                    "docker",
                    "run",
                    "--rm",
                    "-i",
                    "--network",
                    "none",
                    "clinical-history:r4-validator",
                ],
                input=json.dumps(bundle).encode(),
                check=True,
            )
            artifact_dir = args.out.resolve().parent
            artifact_dir.mkdir(parents=True, exist_ok=True)
            synthetic_fhir = artifact_dir / "synthetic-fhir-r4.json"
            synthetic_fhir.write_text(
                json.dumps(bundle, ensure_ascii=False, indent=2) + "\n"
            )
            passed(
                "Independent local consumer and official R4 schema validated a synthetic report-bearing handoff"
            )
            if args.profile_validator:
                subprocess.run(
                    [
                        "docker",
                        "run",
                        "--rm",
                        "--network",
                        "none",
                        "-v",
                        f"{synthetic_fhir}:/work/brief.json:ro",
                        "clinical-history:r4-profiles",
                        "/work/brief.json",
                        "-version",
                        "4.0.1",
                        "-tx",
                        "n/a",
                        "-profile",
                        "http://hl7.org/fhir/StructureDefinition/Bundle",
                    ],
                    check=True,
                )
                passed(
                    "Official HL7 validator evaluated the R4 Bundle and declared profiles offline"
                )
            c.call(path + "/access", "DELETE", {})
            try:
                read_brief(c.url[:-4], token)
            except HTTPError as exc:
                assert exc.code == 401
            else:
                raise AssertionError("revoked token still works")
            row["value"] = "Negative"
            e = act(
                c, e, "review_report_row", documentId=d["id"], rowId=row["id"], row=row
            )
            stale = c.call(path + "/export")
            assert stale["outdatedAssessmentVersions"] and not stale.get(
                "currentAssessmentVersion"
            )
            passed(
                "Revoked consumer access denied and correction invalidated the assessed revision"
            )
            png = compose(
                source,
                "exec",
                "-T",
                "app",
                "pdftoppm",
                "-r",
                "144",
                "-singlefile",
                "-png",
                "-",
                input=samples.digital(),
                stdout=subprocess.PIPE,
            ).stdout
            jpeg = compose(
                source,
                "exec",
                "-T",
                "app",
                "pdftoppm",
                "-r",
                "144",
                "-singlefile",
                "-jpeg",
                "-",
                input=samples.digital(),
                stdout=subprocess.PIPE,
            ).stdout
            originals = {}
            for filename, data, fragment in [
                ("digital.pdf", samples.digital(), "Leukocyte"),
                ("table.docx", samples.word(), "Synthetic marker"),
                ("scan.pdf", samples.scanned(jpeg, 1080, 1440), "Leukocyte"),
                ("photo.png", png, "Leukocyte"),
                ("photo.jpg", jpeg, "Leukocyte"),
            ]:
                e = file_request(c, e, filename, data)
                doc = e["documents"][-1]
                line = next(
                    x for x in doc["rows"] if fragment.lower() in x["original"].lower()
                )
                if filename == "table.docx":
                    assert (
                        line["value"] == "2.5"
                        and line["units"] == "mg/L"
                        and line["referenceRange"] == "1.0–3.0"
                    )
                if filename in ("scan.pdf", "photo.png", "photo.jpg"):
                    assert line["uncertain"] and "OCR" in line["locator"]
                url = path + "/documents/" + doc["id"] + "/original"
                same(c.browser.open(c.url + url).read(), data, "original changed")
                originals[url] = hashlib.sha256(data).hexdigest()
                if filename.endswith((".pdf", ".png", ".jpg")):
                    preview_url = path + "/documents/" + doc["id"] + "/page/1"
                    with c.browser.open(c.url + preview_url) as response:
                        assert response.headers.get_content_type() == "image/png"
                        assert response.read(8) == b"\x89PNG\r\n\x1a\n"
                    try:
                        Client("18441").browser.open(c.url + preview_url)
                    except HTTPError as exc:
                        assert exc.code == 401
                    else:
                        raise AssertionError("page preview missing owner authorization")
                passed("Upload, local extraction, locator and original " + filename)
            urgent = file_request(
                c,
                e,
                "urgent.txt",
                b"Critical result: contact the issuing service. Patient ID: DO-NOT-DISCLOSE",
            )
            warning = next(w for w in urgent["warnings"] if w["active"])
            assert (
                warning["documentId"] == urgent["documents"][-1]["id"]
                and urgent["state"] == "interrupted_for_safety"
            )
            e = act(
                c, urgent, "authorize_report", documentId=d["id"], rowIds=[row["id"]]
            )
            e = act(c, e, "review")
            e = act(c, e, "confirm")
            e = act(c, e, "assess")
            assert e["assessments"][-1]["outcome"] == "differential"
            for endpoint in ("export", "brief", "fhir"):
                projection = c.call(path + "/" + endpoint)
                assert warning["message"] in json.dumps(projection)
                if endpoint == "fhir":
                    assert MODEL_GUIDANCE in projection["entry"][0]["resource"]["text"]["div"]
                else:
                    same(projection["modelGuidance"], MODEL_GUIDANCE, "urgent handoff lost model guidance")
                if endpoint != "export":
                    assert "DO-NOT-DISCLOSE" not in json.dumps(projection)
            pending_token = c.call(path + "/access", "POST", {})["token"]
            passed(
                "Printed critical flag interrupted questioning and remained in JSON, brief and FHIR alongside a deliberately requested differential"
            )
            passed("Model recommendation retained on the upgraded v1 export, scoped read-only brief and all urgent handoff formats")
            e = act(
                c,
                e,
                "add_outcome",
                text="I brought the report to the appointment.",
                date="2026-10-12",
                source="patient_reported",
            )
            try:
                read_brief(c.url[:-4], pending_token)
            except HTTPError as exc:
                assert exc.code == 401
            else:
                raise AssertionError("revised brief still available on old grant")
            before = c.call(path + "/export")
            with tempfile.TemporaryFile() as backup:
                compose(source, "--profile", "backups", "up", "-d", "backup")
                for _ in range(50):
                    log = compose(
                        source, "logs", "backup", stdout=subprocess.PIPE
                    ).stdout
                    if b"Backup completed" in log:
                        break
                    time.sleep(0.3)
                else:
                    raise AssertionError("scheduler did not complete first backup")
                compose(source, "--profile", "backups", "stop", "backup")
                compose(source, "run", "--rm", "--no-deps", "-T", "backup", "once")
                count = compose(
                    source,
                    "run",
                    "--rm",
                    "--no-deps",
                    "-T",
                    "--entrypoint",
                    "sh",
                    "backup",
                    "-c",
                    "ls -1 /var/lib/postgresql/data/clinical-*.dump | wc -l",
                    stdout=subprocess.PIPE,
                ).stdout
                assert int(count) == 1
                passed(
                    "Scheduled worker created a restorable archive; the next backup retained the configured count"
                )
                compose(
                    source,
                    "run",
                    "--rm",
                    "--no-deps",
                    "-T",
                    "--entrypoint",
                    "sh",
                    "backup",
                    "-c",
                    "cat /var/lib/postgresql/data/clinical-*.dump",
                    stdout=backup,
                )
                backup.seek(0)
                compose(dest, "up", "-d", "--wait", "db")
                compose(
                    dest,
                    "exec",
                    "-T",
                    "db",
                    "pg_restore",
                    "-U",
                    "clinicalhistory",
                    "-d",
                    "clinicalhistory",
                    "--single-transaction",
                    "--exit-on-error",
                    "--no-owner",
                    "--no-acl",
                    stdin=backup,
                )
            env.update(
                CLINICAL_LOCALE="en-IN",
                CLINICAL_UNITS="metric",
                CLINICAL_EMERGENCY_CONTACT="Fictional district emergency desk",
            )
            compose(dest, "up", "-d", "--build", "--wait", "app")
            other = Client("18442")
            other.call("/login", "POST", owner)
            region = other.call("/region")
            assert region["locale"] == "en-IN" and region["units"] == "metric"
            after = other.call(path + "/export")
            for report in (before, after):
                report.pop("exportedAt")
            same(before, after, "restore altered encounter")
            for url, h in originals.items():
                same(
                    hashlib.sha256(
                        other.browser.open(other.url + url).read()
                    ).hexdigest(),
                    h,
                    "restored original differs",
                )
            passed(
                "Scheduler backup restored full report, follow-up, all binary originals and alternate regional settings"
            )
            # The dedicated database for this test uses the same saved config before rotation.
            env["CLINICAL_PORT"] = "18442"
            subprocess.run(
                [
                    sys.executable,
                    str(ROOT / "scripts/maintenance.py"),
                    "rotate-key",
                    "--env-file",
                    str(config),
                    "--project",
                    dest,
                ],
                cwd=ROOT,
                env=env,
                check=True,
            )
            other.call("/session")
            same(
                other.call(vpath + "/export")["assessments"][0],
                archived,
                "rotated historical estimate changed",
            )
            for url, h in originals.items():
                same(
                    hashlib.sha256(
                        other.browser.open(other.url + url).read()
                    ).hexdigest(),
                    h,
                    "rotated original differs",
                )
            password = secrets.token_urlsafe(24)
            subprocess.run(
                [
                    sys.executable,
                    str(ROOT / "scripts/maintenance.py"),
                    "recover-owner",
                    "--env-file",
                    str(config),
                    "--project",
                    dest,
                    "--password-stdin",
                ],
                cwd=ROOT,
                env=env,
                check=True,
                input=password.encode(),
            )
            assert not other.call("/session")["authenticated"]
            other.call("/login", "POST", dict(owner, password=password))
            assert other.call(path + "/export")["outcomes"]
            passed(
                "Offline key rotation and owner recovery retained history and invalidated existing login sessions"
            )
            # No owner access, even to the original source bytes, after explicit deletion.
            c.call(path, "DELETE", {"revision": c.call(path)["revision"]})
            for url in [path + "/export", *originals]:
                try:
                    c.browser.open(c.url + url)
                except HTTPError as exc:
                    assert exc.code == 404
                else:
                    raise AssertionError("deleted encounter still available")
            passed("Owner deletion cascaded to document originals and report exports")
            args.out.parent.mkdir(parents=True, exist_ok=True)
            args.out.write_text(
                json.dumps(
                    {"synthetic": True, "passed": results, "v1Image": args.v1_image},
                    indent=2,
                )
                + "\n"
            )
        finally:
            for project in (dest, source, rollback):
                compose(
                    project,
                    "--profile",
                    "backups",
                    "down",
                    "--volumes",
                    "--remove-orphans",
                )


if __name__ == "__main__":
    main()
