package server

import (
	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/evidence"
	"github.com/ctlplne/healthctl/internal/provider"
	"io"
	"testing"
)

func TestLibraryLifecycleAndUsagePostgres(t *testing.T) {
	db, _, _ := testStore(t)
	s := newTestServer(db, io.Discard)
	c := caller{h: s.Handler()}
	check(t, c.call("POST", "/api/evidence/search", object{"query": "migraine"}), 401, nil)
	check(t, c.call("GET", "/api/qualification", nil), 401, nil)
	c.signup(t)
	var r evidence.Result
	check(t, c.call("POST", "/api/evidence/search", object{"query": "migraine"}), 200, &r)
	if len(r.Passages) != 1 || r.Outcome != "passages" {
		t.Fatal("bundled source unavailable")
	}
	p := evidence.Starter()
	p.ID = "owner-set"
	p.Version = "v1"
	p.Passages = p.Passages[:1]
	p.Passages[0].Topics = []string{"training topic"}
	install := func(want int) {
		check(t, c.call("POST", "/api/evidence/packs", object{"pack": p, "rightsConfirmed": true}), want, nil)
	}
	check(t, c.call("POST", "/api/evidence/packs", object{"pack": p}), 400, nil)
	install(200)
	p.Passages[0].Topics = []string{"altered training topic"}
	install(409)
	p.Version = "v2"
	install(200)
	check(t, c.call("GET", "/api/evidence/owner-set/v1/cold-definition", nil), 200, &r.Passages[0])
	if r.Passages[0].Status != "retired" {
		t.Fatal("replacement did not retire v1")
	}
	activate := func(v, status string) {
		check(t, c.call("POST", "/api/evidence/collections/owner-set/"+v+"/status", object{"status": status, "reason": "Synthetic version transition"}), 200, nil)
	}
	activate("v1", "active")
	check(t, c.call("POST", "/api/evidence/search", object{"query": "training topic"}), 200, &r)
	if len(r.Passages) != 1 || r.Passages[0].Version != "v1" {
		t.Fatal("rollback did not activate immutable version")
	}
	e := &clinical.Encounter{AssessmentRecord: clinical.AssessmentRecord{Assessments: []clinical.Assessment{{Items: []clinical.Item{{Name: "Training topic"}}}}}}
	s.attachAssessmentEvidence(t.Context(), 1, e)
	activate("v1", "withdrawn")
	s.refreshEvidence(t.Context(), 1, e)
	if e.Assessments[0].Literature[0].Passages[0].Status != "withdrawn" {
		t.Fatal("historical withdrawal missing")
	}
	check(t, c.call("POST", "/api/evidence/search", object{"query": "training topic"}), 200, &r)
	if r.Outcome != "unavailable" {
		t.Fatal("withdrawn source returned to new search")
	}
	m := provider.Metadata{Usage: provider.Usage{Requests: 2, Reported: 1, Retries: 1, InputTokens: 10, OutputTokens: 5, DurationMS: 123}}
	s.recordModelUsage(t.Context(), 1, provider.Config{Kind: "openai", Model: "fixture"}, m, nil, nil)
	var metrics struct {
		Usage struct{ Operations, Requests, IncompleteUsageOperations int }
	}
	check(t, c.call("GET", "/api/qualification", nil), 200, &metrics)
	if metrics.Usage.Operations != 1 || metrics.Usage.Requests != 2 || metrics.Usage.IncompleteUsageOperations != 1 {
		t.Fatal("usage uncertainty was hidden")
	}
}
