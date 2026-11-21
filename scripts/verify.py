"""Rebuild and test the local QA project, isolated from the personal installation."""
import argparse
import os
from pathlib import Path
import secrets
import subprocess

root = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument("suite", nargs="?", choices=("all", "backend", "browser"), default="all")
suite = parser.parse_args().suite
secrets_dir = root / ".secrets"
secrets_dir.mkdir(mode=0o700, exist_ok=True)
credential = secrets_dir / "qa-owner"
try:
    fd = os.open(credential, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
except FileExistsError:
    pass
else:
    with os.fdopen(fd, "w") as f:
        f.write(secrets.token_urlsafe(24))
env = dict(os.environ, CLINICAL_PORT=os.environ.get("CLINICAL_QA_PORT", "18437"),
           CLINICAL_E2E_PASSWORD=credential.read_text().strip(),
           CLINICAL_EHR_NAME="Fictional QA hospital", CLINICAL_EHR_FHIR_BASE="http://127.0.0.1:8091/r4",
           CLINICAL_EHR_AUTHORIZE_URL="http://localhost:8091/authorize", CLINICAL_EHR_TOKEN_URL="http://127.0.0.1:8091/token",
           CLINICAL_EHR_CLIENT_ID="local-qa", CLINICAL_EHR_REDIRECT_URI="http://127.0.0.1:8080/ehr/callback",
           CLINICAL_EHR_AUTH_ISSUER="")
base = ["docker", "compose", "-p", "clinical-history-qa", "-f", "compose.yaml", "-f", "compose.verify.yaml"]

def run(*args):
    subprocess.run([*base, *args], cwd=root, env=env, check=True)

def check_printed_reports():
    guidance = (root / "internal/clinical/model-guidance.txt").read_text().strip()
    results = root / "build/test-results"
    filenames = (
        "synthetic-report.pdf", "synthetic-urgent-report.pdf", "synthetic-nasal-clinician-brief.pdf",
        "synthetic-clinician-brief.pdf", "synthetic-provenance-appendix.pdf",
        "synthetic-corrected-urgent-brief.pdf",
    )
    total_pages = 0
    total_reports = 0
    for filename in filenames:
        viewports = ("desktop",) if filename == "synthetic-nasal-clinician-brief.pdf" else ("desktop", "mobile")
        for viewport in viewports:
            matches = list(results.glob(f"*-{viewport}/{filename}"))
            if len(matches) != 1:
                raise RuntimeError(f"Expected one {viewport} {filename}, found {len(matches)}")
            data = subprocess.run(
                [*base, "exec", "-T", "app", "pdftotext", "-layout", "-", "-"],
                cwd=root, env=env, input=matches[0].read_bytes(), capture_output=True, check=True,
            ).stdout.decode()
            pages = [" ".join(page.split()) for page in data.rstrip("\f\n").split("\f")]
            for index, page in enumerate(pages, 1):
                if "ctlplne studio" not in page or "beta" not in page.lower():
                    raise RuntimeError(f"{viewport} {filename} page {index} is missing the preview attribution")
                if "Verify findings with a qualified healthcare professional" not in page:
                    raise RuntimeError(f"{viewport} {filename} page {index} is missing the clinician-verification reminder")
                if guidance not in page:
                    raise RuntimeError(f"{viewport} {filename} page {index} is missing the model recommendation")
            if filename.endswith("-brief.pdf") and len(pages) != 1:
                raise RuntimeError(f"{viewport} {filename} has {len(pages)} pages")
            if "urgent" in filename and not any("active warning" in p.lower() or "active urgent warning" in p.lower() for p in pages):
                raise RuntimeError(f"{viewport} {filename} is missing its urgent warning")
            total_pages += len(pages)
            total_reports += 1
    print(f"Verified model guidance on all {total_pages} pages across {total_reports} PDFs, plus urgent warnings and one-page briefs.")

if suite in ("all", "backend"):
    run("run", "--build", "--rm", "backend-tests")
if suite in ("all", "browser"):
    (root / "build" / "test-results").mkdir(parents=True, exist_ok=True)
    run("up", "-d", "--wait", "--build", "app", "fixture", "ehr-fixture")
    # The portal and browser share the live app network namespace. Recreating the
    # healthy app here would leave the portal listening in the old namespace.
    run("build", "browser-tests")
    run("run", "--rm", "--no-deps", "browser-tests")
    check_printed_reports()
