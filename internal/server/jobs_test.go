package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ctlplne/healthctl/internal/clinical"
)

// blockingModel answers like an OpenAI-compatible endpoint after release is
// closed, and reports requests that were cancelled while waiting.
func blockingModel(t *testing.T, started chan<- struct{}, release <-chan struct{}, cancelled *atomic.Int32, concurrent, peak *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reading the body lets the server notice when the client gives up.
		io.Copy(io.Discard, r.Body)
		n := concurrent.Add(1)
		defer concurrent.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
			cancelled.Add(1)
			return
		case <-release:
		}
		proposal := clinical.Proposal{Action: "ask", Question: "When did the change start?", Reason: "The timing is still unknown.", Choices: []string{"Today", "Yesterday"}, Outstanding: []string{}, Findings: []clinical.FindingProposal{}, UrgentEvidence: []clinical.Evidence{}, Items: []clinical.Item{}}
		b, _ := json.Marshal(proposal)
		json.NewEncoder(w).Encode(object{"model": "fixture", "choices": []any{object{"finish_reason": "stop", "message": object{"role": "assistant", "content": string(b)}}}})
	}))
}

func postAsync(t *testing.T, c caller, body object) clinical.Encounter {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequestWithContext(t.Context(), "POST", "http://localhost/api/encounters", bytes.NewReader(b))
	req.Header.Set("Prefer", "respond-async")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	req.AddCookie(c.cookie)
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, req)
	var e clinical.Encounter
	check(t, w, 202, &e)
	return e
}

func jobStatus(t *testing.T, s *Server, encounterID string) []string {
	t.Helper()
	rows, err := s.Store.DB.QueryContext(t.Context(), "SELECT status FROM model_jobs WHERE encounter_id=$1 ORDER BY id", encounterID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var st string
		rows.Scan(&st)
		out = append(out, st)
	}
	return out
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestSupersededCallCancelledPostgres(t *testing.T) {
	db, _, _ := testStore(t)
	var cancelled, concurrent, peak atomic.Int32
	started, release := make(chan struct{}, 8), make(chan struct{})
	mock := blockingModel(t, started, release, &cancelled, &concurrent, &peak)
	defer mock.Close()
	defer close(release) // runs first, so no request is left waiting when the mock closes
	base := mock.URL + "/v1"
	t.Setenv("CLINICAL_ALLOWED_ENDPOINTS", base)
	s := newTestServer(db, io.Discard)
	c := caller{h: s.Handler()}
	c.signup(t)
	cfg := clinical.ProviderRef{Kind: "compatible", Endpoint: base, Model: "fixture", Structured: "prompt"}
	selectProvider(t, c, cfg)
	e := postAsync(t, c, intake(cfg, "Fictional shoulder discomfort"))
	select {
	case <-started:
	case <-time.After(8 * time.Second):
		t.Fatal("the queued call did not start")
	}
	// The user stops the interview while the call is still running.
	var next clinical.Encounter
	check(t, c.call("POST", "/api/encounters/"+e.ID+"/actions", object{"action": "review", "revision": e.Revision, "requestId": token(12)}), 200, &next)
	waitFor(t, "the superseded call to be cancelled", func() bool { return cancelled.Load() == 1 })
	waitFor(t, "the job to be marked superseded", func() bool { st := jobStatus(t, s, e.ID); return len(st) == 1 && st[0] == "superseded" })
	var now clinical.Encounter
	check(t, c.call("GET", "/api/encounters/"+e.ID, nil), 200, &now)
	if now.State != clinical.StateHistoryReview || now.Revision != next.Revision || now.Question != "" {
		t.Fatalf("a superseded call overwrote the user's action: %s rev %d", now.State, now.Revision)
	}
}

func TestJobQueuePostgres(t *testing.T) {
	db, _, _ := testStore(t)
	var cancelled, concurrent, peak atomic.Int32
	started, release := make(chan struct{}, 16), make(chan struct{})
	mock := blockingModel(t, started, release, &cancelled, &concurrent, &peak)
	defer mock.Close()
	base := mock.URL + "/v1"
	t.Setenv("CLINICAL_ALLOWED_ENDPOINTS", base)
	t.Setenv("CLINICAL_MODEL_WORKERS", "2")
	first, stopFirst := context.WithCancel(t.Context())
	defer stopFirst()
	s := newTestServer(db, io.Discard)
	s.Lifetime = first
	c := caller{h: s.Handler()}
	c.signup(t)
	cfg := clinical.ProviderRef{Kind: "compatible", Endpoint: base, Model: "fixture", Structured: "prompt"}
	selectProvider(t, c, cfg)

	// Bounded workers: four encounters, never more than two calls at once.
	var ids []string
	for range 4 {
		ids = append(ids, postAsync(t, c, intake(cfg, "Fictional shoulder discomfort")).ID)
	}
	waitFor(t, "two calls to start", func() bool { return concurrent.Load() == 2 })
	time.Sleep(300 * time.Millisecond)
	if peak.Load() > 2 {
		t.Fatalf("%d calls ran at once with two workers", peak.Load())
	}
	close(release)
	for _, id := range ids {
		waitFor(t, "every job to finish", func() bool { st := jobStatus(t, s, id); return len(st) == 1 && st[0] == "done" })
		var e clinical.Encounter
		check(t, c.call("GET", "/api/encounters/"+id, nil), 200, &e)
		if e.Pending != "" || e.Question != "When did the change start?" {
			t.Fatalf("a finished job did not save its result: %+v", e.State)
		}
	}

	// Resume after a restart: a job left running by a stopped server is picked
	// up by the next one.
	e := postAsync(t, c, intake(cfg, "Fictional knee discomfort"))
	waitFor(t, "the job to finish", func() bool { st := jobStatus(t, s, e.ID); return len(st) == 1 && st[0] == "done" })
	var current clinical.Encounter
	check(t, c.call("GET", "/api/encounters/"+e.ID, nil), 200, &current)
	pending, err := db.Get(t.Context(), 1, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	pending.Pending, pending.State = "interview", clinical.StateInterviewing
	rev := pending.Revision
	pending.Revision++
	if err := db.Finish(t.Context(), 1, pending, rev); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.ExecContext(t.Context(), "INSERT INTO model_jobs(owner_id,encounter_id,task,revision,status,attempts) VALUES(1,$1,'interview',$2,'running',1)", e.ID, pending.Revision); err != nil {
		t.Fatal(err)
	}
	stopFirst() // the first server stops; its workers exit
	second, stopSecond := context.WithCancel(t.Context())
	restarted := newTestServer(db, io.Discard)
	restarted.Lifetime = second
	restarted.Handler()
	waitFor(t, "the interrupted job to resume", func() bool { st := jobStatus(t, restarted, e.ID); return len(st) == 2 && st[1] == "done" })
	stopSecond()

	// A panic in one call is recorded and leaves a recoverable failure.
	third, stopThird := context.WithCancel(t.Context())
	defer stopThird()
	crashing := newTestServer(db, io.Discard)
	crashing.Lifetime = third
	crashing.beforeModel = func(*clinical.Encounter) { panic("synthetic failure") }
	broken := postAsync(t, caller{h: crashing.Handler(), csrf: c.csrf, cookie: c.cookie}, intake(cfg, "Fictional wrist discomfort"))
	waitFor(t, "the panicking job to be recorded", func() bool { st := jobStatus(t, crashing, broken.ID); return len(st) == 1 && st[0] == "failed" })
	var failed clinical.Encounter
	check(t, c.call("GET", "/api/encounters/"+broken.ID, nil), 200, &failed)
	if failed.State != clinical.StateFailedRecoverably || failed.FailedTask != "interview" || len(failed.Answers) == 0 {
		t.Fatalf("a crash must keep the history and offer a retry: %s", failed.State)
	}
}
