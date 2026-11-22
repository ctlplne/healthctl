"""Synthetic compatible API used only in the isolated Compose verification project."""
import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Lock

attempted = set()
lock = Lock()

class Fixture(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        if self.path != "/v1/chat/completions":
            self.send_error(404)
            return
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        message = body["messages"][-1]["content"]
        if message == "Check the configured model connection.":
            proposal = {"status": "ready"}
        else:
            history = json.loads(message)
            if body["model"] == "fixture-slow-interview":
                time.sleep(7)
            if body["model"] == "fixture-fail-once":
                with lock:
                    first = history["concern"] not in attempted
                    attempted.add(history["concern"])
                if first:
                    self.send_error(401)
                    return
            proposal = dict(action="review", question="", choices=[], reason="The synthetic record can be reviewed.",
                            findings=[], urgentEvidence=[], outstanding=[], items=[])
            authored = (body["model"] == "fixture-urgent-supported" and
                        history["concern"] == "My nose is runny and I keep sneezing.")
            report_case = (body["model"] == "fixture-report" and
                           history["concern"] == "It burns when I urinate.")
            if report_case:
                # Only visible, approved extracts may surface on the compatibility wire.
                if "DO-NOT-DISCLOSE" in message or "documents" in history or "reportReviews" in history:
                    self.send_error(422)
                    return
                entries = history.get("reportEntries", [])
                if history["task"] == "assessment":
                    ids = [f["id"] for f in history["facts"] if f["concept"] == "Burning with urination"]
                    if entries and ids:
                        proposal.update(action="differential", items=[dict(
                            name="Bladder irritation", clinicalTerm="", explanation="This condition can cause discomfort with urination.",
                            supporting=[ids[0], entries[0]["factId"]], conflicting=[],
                            limitations=["Additional clinical information is needed."])])
                    else:
                        proposal.update(action="insufficient", outstanding=["History is incomplete in this synthetic fixture."])
                else:
                    source = next(a for a in history["answers"] if a["kind"] == "concern")
                    proposal["findings"] = [dict(concept="Burning with urination", status="reported_present",
                        subject="self", timeContext="", value="", units="", answerId=source["id"], quote=source["text"])]
            elif authored and history["task"] == "interview":
                source = next(a["id"] for a in history["answers"] if a["kind"] == "concern")
                proposal["findings"] = [
                    dict(concept=concept, status="reported_present", subject="self", timeContext="",
                         value="", units="", answerId=source, quote=quote)
                    for concept, quote in [("Runny nose", "nose is runny"), ("Sneezing", "sneezing")]
                ]
            elif authored and history["task"] == "assessment":
                supporting = [f["id"] for f in history["facts"] if f["concept"] in ("Runny nose", "Sneezing")]
                proposal.update(action="differential", items=[
                    dict(name="Nasal allergies", clinicalTerm="Allergic rhinitis",
                         explanation="This condition can cause sneezing.", supporting=supporting,
                         conflicting=[], limitations=["The trigger is unknown in this synthetic case."])
                ])
            elif history["task"] == "assessment":
                proposal.update(action="insufficient", outstanding=["The test fixture has no diagnostic inference engine."])
            elif not any(a["kind"] == "reply" for a in history["answers"]):
                proposal.update(action="ask", question="When did you first notice the change?",
                                choices=["Yesterday", "Not sure"], reason="The timing remains unknown.")
        data = json.dumps({"model": "fixture-returned-v1", "choices": [{"finish_reason": "stop",
                          "message": {"role": "assistant", "content": json.dumps(proposal)}}]}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

ThreadingHTTPServer(("0.0.0.0", 8090), Fixture).serve_forever()
