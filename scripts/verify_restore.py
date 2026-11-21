"""Exercise an encrypted pg_dump/pg_restore round trip using two disposable installations."""
import base64
import http.cookiejar
import json
import os
import secrets
import subprocess
import tempfile
from pathlib import Path
from urllib.request import build_opener, HTTPCookieProcessor, Request

root = Path(__file__).resolve().parents[1]
run_id = secrets.token_hex(5)
env = dict(os.environ, CLINICAL_DATA_KEY=base64.b64encode(secrets.token_bytes(32)).decode(),
           POSTGRES_PASSWORD=secrets.token_hex(32), CLINICAL_SECURE_COOKIE="false",
           CLINICAL_PUBLIC_ORIGIN="", OPENAI_API_KEY="", ANTHROPIC_API_KEY="",
           BYOM_API_KEY="", CLINICAL_ALLOWED_ENDPOINTS="")
source = ("clinical-history-restore-src-" + run_id, "18439")
restored = ("clinical-history-restore-dst-" + run_id, "18440")
owner = {"username": "restore-test-owner", "password": secrets.token_urlsafe(24)}
demo = {"kind": "demo", "model": "synthetic", "structured": "auto", "endpoint": ""}

def compose(project, *args, **kwargs):
    name, port = project
    return subprocess.run(["docker", "compose", "-p", name, *args], cwd=root,
                          env=dict(env, CLINICAL_PORT=port), check=True, **kwargs)

class Client:
    def __init__(self, port):
        self.url = "http://127.0.0.1:" + port + "/api"
        self.jar = http.cookiejar.CookieJar()
        self.browser = build_opener(HTTPCookieProcessor(self.jar))
        self.csrf = ""

    def call(self, path, method="GET", body=None):
        payload = json.dumps(body).encode() if body is not None else None
        request = Request(self.url + path, method=method, data=payload,
                          headers={"Content-Type": "application/json", "X-CSRF-Token": self.csrf})
        with self.browser.open(request, timeout=20) as response:
            value = json.load(response)
        if "csrf" in value:
            self.csrf = value["csrf"]
        return value

def verify():
    compose(source, "up", "-d", "--build", "--wait")
    client = Client(source[1])
    state = client.call("/setup", "POST", owner)
    client.call("/settings", "PUT", {"provider": demo})
    encounter = client.call("/encounters", "POST", {
        "age": 35, "concern": "My nose is runny and I keep sneezing.", "requestId": secrets.token_hex(12),
        "pregnancyRelated": False, "urgentNow": False, "noticeVersion": state["noticeVersion"],
        "disclosedProvider": demo})
    path = "/encounters/" + encounter["id"]
    for action, text in [("answer", "Yesterday"), ("answer", "After being outdoors"),
                         ("review", ""), ("confirm", ""), ("assess", "")]:
        encounter = client.call(path + "/actions", "POST", {"action": action, "text": text,
                                "revision": encounter["revision"], "requestId": secrets.token_hex(12)})
    before = client.call(path + "/export")
    assert before["assessments"][-1]["items"]
    with tempfile.TemporaryFile() as backup:
        compose(source, "exec", "-T", "db", "pg_dump", "-U", "clinicalhistory", "-d", "clinicalhistory",
                "-Fc", "--no-owner", "--no-acl", "--exclude-table-data=public.sessions", "--exclude-table-data=public.brief_access", "--exclude-table-data=public.chatgpt_vault", stdout=backup)
        backup.seek(0)
        compose(restored, "up", "-d", "--wait", "db")
        compose(restored, "exec", "-T", "db", "pg_restore", "-U", "clinicalhistory", "-d", "clinicalhistory",
                "--exit-on-error", "--single-transaction", "--no-owner", "--no-acl", stdin=backup)
    compose(restored, "up", "-d", "--build", "--wait")
    other = Client(restored[1])
    other.jar = client.jar
    other.browser = build_opener(HTTPCookieProcessor(other.jar))
    state = other.call("/session")
    assert not state["setupRequired"] and not state["authenticated"]
    other.call("/login", "POST", owner)
    after = other.call(path + "/export")
    for record in (before, after):
        del record["exportedAt"]
    assert before == after
    print("Restored the complete encrypted synthetic report. Previous sessions were excluded.", flush=True)

if __name__ == "__main__":
    try:
        verify()
    finally:
        # Both project names are fresh random names generated for this invocation.
        for project in (source, restored):
            compose(project, "down", "--volumes")
