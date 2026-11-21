"""Fictional SMART patient portal for disposable loopback browser tests."""
import base64
import hashlib
import html
import json
import secrets
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlencode, urlsplit

BASE = "http://127.0.0.1:8091/r4"
AUTH = "http://localhost:8091/authorize"
TOKEN = "http://127.0.0.1:8091/token"
CALLBACK = "http://127.0.0.1:8080/ehr/callback"
SCOPES = "launch/patient patient/Observation.rs"
grants, access = {}, {}
lock = threading.Lock()


def observation(key, patient, label, result, unit):
    return {
        "resourceType": "Observation", "id": key, "meta": {"versionId": "2"},
        "status": "final", "subject": {"reference": "Patient/" + patient},
        "category": [{"coding": [{"system": "http://terminology.hl7.org/CodeSystem/observation-category", "code": "laboratory"}]}],
        "code": {"text": label}, "valueQuantity": {"value": result, "unit": unit},
        "effectiveDateTime": "2026-09-04", "issued": "2026-09-05T12:00:00Z",
    }


class Portal(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def reply(self, status, content, kind="application/json"):
        data = content.encode() if isinstance(content, str) else json.dumps(content).encode()
        self.send_response(status)
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Type", kind)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        path = urlsplit(self.path)
        params = {k: v[0] for k, v in parse_qs(path.query).items()}
        if path.path == "/ready":
            return self.reply(200, "ready", "text/plain")
        if path.path == "/r4/.well-known/smart-configuration":
            return self.reply(200, {
                "authorization_endpoint": AUTH, "token_endpoint": TOKEN,
                "code_challenge_methods_supported": ["S256"], "scopes_supported": SCOPES.split(),
                "capabilities": ["launch-standalone", "client-public", "context-standalone-patient", "permission-patient", "permission-v2"],
            })
        if path.path == "/authorize":
            if not self.valid_launch(params):
                return self.reply(400, {"error": "invalid_launch"})
            fields = "".join('<input type="hidden" name="' + html.escape(k, quote=True) +
                             '" value="' + html.escape(v, quote=True) + '">' for k, v in params.items())
            return self.reply(200, '<!doctype html><html lang="en"><title>Fictional hospital portal</title>'
                              '<h1>Fictional hospital portal</h1><p>Release synthetic laboratory results?</p>'
                              '<form method="post" action="/decision">' + fields +
                              '<button name="allow" value="yes">Allow laboratory access</button>'
                              '<button name="allow" value="no">Decline access</button></form></html>', "text/html")
        if path.path == "/r4/Observation":
            bearer = self.headers.get("Authorization", "").removeprefix("Bearer ")
            with lock:
                valid = access.pop(bearer, 0) > time.monotonic()
            if not valid or params.get("patient") != "synthetic-qa" or params.get("category", "").split("|")[-1] != "laboratory":
                return self.reply(403, {"error": "scope"})
            rows = [
                observation("sample-a", "synthetic-qa", "Fictional marker A", 4.2, "mg/L"),
                observation("sample-b", "synthetic-qa", "Fictional marker B", 19, "mmol/L"),
                observation("other", "another-patient", "Wrong-patient marker", 88, "g/L"),
            ]
            rows[1]["interpretation"] = [{"coding": [{
                "system": "http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation",
                "code": "HH", "display": "Critical high",
            }]}]
            return self.reply(200, {"resourceType": "Bundle", "type": "searchset", "total": len(rows),
                                    "entry": [{"resource": row} for row in rows]}, "application/fhir+json")
        self.reply(404, {})

    @staticmethod
    def valid_launch(p):
        return (p.get("aud") == BASE and p.get("scope", "").split() and set(p["scope"].split()) == set(SCOPES.split())
                and p.get("client_id") == "local-qa" and p.get("response_type") == "code"
                and p.get("redirect_uri") == CALLBACK and p.get("code_challenge_method") == "S256"
                and len(p.get("state", "")) >= 16 and len(p.get("code_challenge", "")) == 43)

    def do_POST(self):
        size = int(self.headers.get("Content-Length", 0))
        if size > 16384:
            return self.reply(400, {})
        p = {k: v[0] for k, v in parse_qs(self.rfile.read(size).decode()).items()}
        if self.path == "/decision" and self.valid_launch(p):
            answer = {"state": p["state"]}
            if p.get("allow") == "yes":
                answer["code"] = secrets.token_urlsafe(32)
                with lock:
                    grants[answer["code"]] = (p["code_challenge"], time.monotonic() + 120)
            else:
                answer["error"] = "access_denied"
            self.send_response(303)
            self.send_header("Location", CALLBACK + "?" + urlencode(answer))
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            return
        if self.path == "/token":
            challenge = base64.urlsafe_b64encode(hashlib.sha256(p.get("code_verifier", "").encode()).digest()).decode().rstrip("=")
            with lock:
                old, expiry = grants.pop(p.get("code"), ("", 0))
                if (challenge == old and expiry > time.monotonic() and p.get("client_id") == "local-qa"
                        and p.get("redirect_uri") == CALLBACK and p.get("grant_type") == "authorization_code"):
                    bearer = secrets.token_urlsafe(32)
                    access[bearer] = time.monotonic() + 120
                    return self.reply(200, {"access_token": bearer, "token_type": "Bearer",
                                            "patient": "synthetic-qa", "scope": SCOPES, "expires_in": 120})
            return self.reply(400, {"error": "invalid_grant"})
        self.reply(404, {})


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", 8091), Portal).serve_forever()
