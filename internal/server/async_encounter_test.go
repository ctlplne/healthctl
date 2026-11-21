package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ctlplne/healthctl/internal/clinical"
)

func TestAsyncInterviewCommittedBeforeInferenceAndReplayPostgres(t *testing.T) {
	db, _, _ := testStore(t)
	var calls atomic.Int32
	ready, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(ready)
		}
		<-release
		proposal := clinical.Proposal{Action: "ask", Question: "When did the change start?", Reason: "The timing is still unknown.", Choices: []string{"Today", "Yesterday"}, Outstanding: []string{}, Findings: []clinical.FindingProposal{}, UrgentEvidence: []clinical.Evidence{}, Items: []clinical.Item{}}
		b, _ := json.Marshal(proposal)
		json.NewEncoder(w).Encode(object{"model": "fixture", "choices": []any{object{"finish_reason": "stop", "message": object{"role": "assistant", "content": string(b)}}}})
	}))
	defer mock.Close()
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	base := mock.URL + "/v1"
	t.Setenv("CLINICAL_ALLOWED_ENDPOINTS", base)
	c := caller{h: newTestServer(db, io.Discard).Handler()}
	c.signup(t)
	cfg := clinical.ProviderRef{Kind: "compatible", Endpoint: base, Model: "fixture", Structured: "prompt"}
	selectProvider(t, c, cfg)
	b, _ := json.Marshal(intake(cfg, "Fictional shoulder discomfort"))
	post := func() clinical.Encounter {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		req := httptest.NewRequestWithContext(ctx, "POST", "http://localhost/api/encounters", bytes.NewReader(b))
		req.Header.Set("Prefer", "respond-async")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", c.csrf)
		req.AddCookie(c.cookie)
		w := httptest.NewRecorder()
		c.h.ServeHTTP(w, req)
		var e clinical.Encounter
		check(t, w, 202, &e)
		if e.Pending != "interview" || w.Header().Get("Location") != "/api/encounters/"+e.ID {
			t.Fatal("saved encounter location or pending status missing")
		}
		return e
	}
	e := post()
	select {
	case <-ready:
	case <-time.After(4 * time.Second):
		t.Fatal("the authorized job did not start")
	}
	replay := post()
	if replay.ID != e.ID || replay.Revision != e.Revision {
		t.Fatal("lost receipt replay")
	}
	path := "/api/encounters/" + e.ID
	check(t, c.call("GET", path, nil), 200, &e)
	if e.Pending != "interview" {
		t.Fatal("pending encounter was unavailable")
	}
	unblock()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		check(t, c.call("GET", path, nil), 200, &e)
		if e.Pending == "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if e.Question != "When did the change start?" || len(e.Turns) != 1 || calls.Load() != 1 {
		t.Fatal("canceled browser request, replay or poll changed the model operation")
	}
}
