package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/provider"
	"github.com/ctlplne/healthctl/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// PostgreSQL checks use a new randomly named schema per test, so no existing
// encounters, settings or accounts are read or changed.
func testStore(t *testing.T) (*store.Store, string, []byte) {
	t.Helper()
	return isolatedStore(t, false)
}

// Offline maintenance takes a database-wide lock. Give maintenance tests a
// disposable database so the running QA application can keep its own lock.
func isolatedStore(t *testing.T, separateDatabase bool) (*store.Store, string, []byte) {
	t.Helper()
	if os.Getenv("CLINICAL_TEST_POSTGRES") != "1" {
		t.Skip("set CLINICAL_TEST_POSTGRES=1 with PG* environment for PostgreSQL checks")
	}
	ctx := context.Background()
	config, err := pgx.ParseConfig(os.Getenv("CLINICAL_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	base := stdlib.OpenDB(*config)
	name := "test_" + token(12)
	create, drop := "CREATE SCHEMA "+name, "DROP SCHEMA "+name+" CASCADE"
	if separateDatabase {
		create, drop = "CREATE DATABASE "+name, "DROP DATABASE "+name
	}
	if _, err = base.ExecContext(ctx, create); err != nil {
		base.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := base.ExecContext(ctx, drop)
		base.Close()
		if err != nil {
			t.Error(err)
		}
	})
	if separateDatabase {
		config.Database = name
	} else {
		config.RuntimeParams["search_path"] = name
	}
	dsn := stdlib.RegisterConnConfig(config)
	t.Cleanup(func() { stdlib.UnregisterConnConfig(dsn) })
	key := make([]byte, 32)
	rand.Read(key)
	db, err := store.Open(ctx, dsn, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.DB.Close() })
	return db, dsn, key
}

type caller struct {
	h      http.Handler
	cookie *http.Cookie
	csrf   string
}

func (c caller) call(method, path string, in any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(in)
	req := httptest.NewRequest(method, "http://localhost"+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", c.csrf)
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, req)
	return w
}
func check(t *testing.T, w *httptest.ResponseRecorder, want int, out any) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("want status %d; got %d: %s", want, w.Code, w.Body.String())
	}
	if out != nil {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatal(err)
		}
	}
}
func newTestServer(db *store.Store, logs io.Writer) *Server {
	return &Server{Store: db, Models: provider.New(), Name: "healthctl", Logger: log.New(logs, "", 0)}
}
func (c *caller) signup(t *testing.T) {
	t.Helper()
	w := c.call("POST", "/api/setup", object{"username": "synthetic-owner", "password": "A synthetic owner password!"})
	var s struct{ CSRF string }
	check(t, w, 200, &s)
	c.csrf = s.CSRF
	c.cookie = w.Result().Cookies()[0]
	if !c.cookie.HttpOnly || c.cookie.SameSite != http.SameSiteStrictMode || s.CSRF == "" {
		t.Fatal("session controls missing")
	}
}
func selectProvider(t *testing.T, c caller, cfg clinical.ProviderRef) {
	t.Helper()
	check(t, c.call("PUT", "/api/settings", object{"provider": cfg}), 200, nil)
}
func intake(cfg clinical.ProviderRef, concern string) object {
	return object{"requestId": token(12), "pregnancyRelated": false, "urgentNow": false, "age": 36, "noticeVersion": clinical.NoticeVersion, "concern": concern, "disclosedProvider": cfg}
}
func command(t *testing.T, c caller, e *clinical.Encounter, in object) {
	t.Helper()
	in["requestId"] = token(12)
	in["revision"] = e.Revision
	var next clinical.Encounter
	check(t, c.call("POST", "/api/encounters/"+e.ID+"/actions", in), 200, &next)
	*e = next
}

func TestAuthenticatedPatientJourneyPostgres(t *testing.T) {
	db, dsn, key := testStore(t)
	var logs bytes.Buffer
	c := caller{h: newTestServer(db, &logs).Handler()}
	for _, p := range []string{"/api/settings", "/api/encounters", "/api/encounters/missing/export"} {
		check(t, c.call("GET", p, nil), 401, nil)
	}
	c.signup(t)
	noCSRF := c
	noCSRF.csrf = ""
	check(t, noCSRF.call("PUT", "/api/settings", object{}), 403, nil)
	cfg := clinical.ProviderRef{Kind: "demo", Model: "synthetic", Structured: "auto"}
	selectProvider(t, c, cfg)
	omitted := intake(cfg, clinical.DemoConcern)
	delete(omitted, "urgentNow")
	check(t, c.call("POST", "/api/encounters", omitted), 400, nil)
	body := intake(cfg, clinical.DemoConcern)
	var e clinical.Encounter
	check(t, c.call("POST", "/api/encounters", body), 200, &e)
	if e.Question == "" || !e.Demo {
		t.Fatal("missing demo interview")
	}
	var copyE clinical.Encounter
	check(t, c.call("POST", "/api/encounters", body), 200, &copyE)
	if e.Revision != copyE.Revision || e.ID != copyE.ID {
		t.Fatal("retried creation duplicated work")
	}
	first := object{"action": "answer", "text": "Yesterday", "revision": e.Revision, "requestId": token(12)}
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		wg.Go(func() { responses <- c.call("POST", "/api/encounters/"+e.ID+"/actions", first) })
	}
	wg.Wait()
	close(responses)
	for w := range responses {
		check(t, w, 200, nil)
	}
	check(t, c.call("GET", "/api/encounters/"+e.ID, nil), 200, &e)
	count := 0
	for _, a := range e.Answers {
		if a.Text == "Yesterday" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("duplicate answer")
	}
	command(t, c, &e, object{"action": "answer", "text": "After being outdoors"})
	command(t, c, &e, object{"action": "review"})
	command(t, c, &e, object{"action": "confirm"})
	var session struct{ AssessmentNotice struct{ Version string } }
	check(t, c.call("GET", "/api/session", nil), 200, &session)
	version := session.AssessmentNotice.Version
	if version == "" {
		t.Fatal("missing assessment notice")
	}
	path := "/api/encounters/" + e.ID + "/actions"
	for _, fields := range []object{{}, {"assessmentAcknowledged": true}, {"assessmentNoticeVersion": version}, {"assessmentAcknowledged": true, "assessmentNoticeVersion": "old-version"}} {
		fields["action"], fields["revision"], fields["requestId"] = "assess", e.Revision, token(12)
		check(t, c.call("POST", path, fields), 400, nil)
	}
	var historyOnly clinical.Report
	check(t, c.call("GET", "/api/encounters/"+e.ID+"/export", nil), 200, &historyOnly)
	if historyOnly.Revision != e.Revision || len(historyOnly.Assessments) != 0 || len(historyOnly.AssessmentAcknowledgments) != 0 {
		t.Fatal("declining the assessment must preserve the history-only report")
	}
	ack := object{"action": "assess", "assessmentAcknowledged": true, "assessmentNoticeVersion": version,
		"revision": e.Revision, "requestId": token(12)}
	for range 2 {
		check(t, c.call("POST", path, ack), 200, &e)
		if len(e.AssessmentAcknowledgments) != 1 || len(e.Assessments) != 1 {
			t.Fatal("expected exactly one acknowledgment and one assessment even after replay")
		}
	}
	receipt := e.AssessmentAcknowledgments[0]
	if receipt.NoticeVersion != version || receipt.At.IsZero() || receipt.HistoryRevision != e.HistoryRevision || receipt.Provider != e.Provider {
		t.Fatal("acknowledgment does not identify the assessment request")
	}
	if len(e.Assessments) != 1 || len(e.Assessments[0].Items) == 0 {
		t.Fatal("missing differential")
	}
	initialVersion := e.HistoryRevision
	var raw []byte
	if err := db.DB.QueryRow("SELECT payload FROM encounters WHERE id=$1", e.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(clinical.DemoConcern)) {
		t.Fatal("plaintext clinical payload")
	}
	wrongKey := make([]byte, 32)
	if _, err := store.Open(context.Background(), dsn, wrongKey); err == nil {
		t.Fatal("wrong key accepted")
	}
	reopened, err := store.Open(context.Background(), dsn, key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	c.h = newTestServer(reopened, &logs).Handler()
	var resumed clinical.Encounter
	check(t, c.call("GET", "/api/encounters/"+e.ID, nil), 200, &resumed)
	if resumed.Revision != e.Revision || len(resumed.Assessments) != 1 {
		t.Fatal("restart lost encounter")
	}
	syntheticText := "<em>SYNTHETIC</em> I have chest pain."
	command(t, c, &e, object{"action": "correct_answer", "answerId": e.Answers[0].ID, "text": syntheticText})
	if e.ConfirmedRevision != 0 || e.Assessments[0].HistoryRevision != initialVersion || e.HistoryRevision == initialVersion || len(e.Warnings) == 0 {
		t.Fatal("correction or warning lost")
	}
	w := c.call("GET", "/api/encounters/"+e.ID+"/export", nil)
	var report clinical.Report
	check(t, w, 200, &report)
	if report.ReviewStatus != clinical.ReviewStatus || report.Notice != clinical.Notice || len(report.Warnings) == 0 || report.ExportVersion == "" || report.Assessments[0].HistoryRevision == report.HistoryRevision || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("export missing status, warnings or version")
	}
	check(t, c.call("DELETE", "/api/encounters/"+e.ID, object{"revision": e.Revision - 1}), 409, nil)
	check(t, c.call("DELETE", "/api/encounters/"+e.ID, object{"revision": e.Revision}), 200, nil)
	check(t, c.call("GET", "/api/encounters/"+e.ID+"/export", nil), 404, nil)
	for _, table := range []string{"encounters", "submissions"} {
		var n int
		if err := db.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("residual %s records", table)
		}
	}
	if strings.Contains(logs.String(), syntheticText) || strings.Contains(logs.String(), "Yesterday") {
		t.Fatal("clinical data in ordinary logs")
	}
	check(t, c.call("POST", "/api/logout", object{}), 200, nil)
	check(t, c.call("GET", "/api/encounters", nil), 401, nil)
	for i := 0; i < 10; i++ {
		check(t, c.call("POST", "/api/login", object{"username": "synthetic-owner", "password": "invalid"}), 401, nil)
	}
	check(t, c.call("POST", "/api/login", object{"username": "synthetic-owner", "password": "invalid"}), 429, nil)
}

func TestLiveAdapterFixtureRaceAndRecoveryPostgres(t *testing.T) {
	db, _, _ := testStore(t)
	var n atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		switch n.Add(1) {
		case 1:
			w.WriteHeader(401)
			return
		case 2:
			close(entered)
			<-release
		}
		prop := clinical.Proposal{Action: "review", Reason: "The reported information is ready for review.", Choices: []string{}, Findings: []clinical.FindingProposal{}, Items: []clinical.Item{}, UrgentEvidence: []clinical.Evidence{}, Outstanding: []string{}}
		b, _ := json.Marshal(prop)
		json.NewEncoder(w).Encode(object{"model": "synthetic-fixture-returned", "choices": []any{object{"finish_reason": "stop", "message": object{"role": "assistant", "content": string(b)}}}})
	}))
	defer mock.Close()
	var destinationCalls atomic.Int32
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		destinationCalls.Add(1)
		mock.Config.Handler.ServeHTTP(w, r)
	}))
	defer second.Close()
	base := mock.URL + "/v1"
	secondBase := second.URL + "/v1"
	t.Setenv("CLINICAL_ALLOWED_ENDPOINTS", base+","+secondBase)
	c := caller{h: newTestServer(db, io.Discard).Handler()}
	c.signup(t)
	cfg := clinical.ProviderRef{Kind: "compatible", Endpoint: base, Model: "fixture-model", Structured: "prompt"}
	selectProvider(t, c, cfg)
	body := intake(cfg, "Synthetic mild fatigue")
	var e clinical.Encounter
	check(t, c.call("POST", "/api/encounters", body), 200, &e)
	if e.State != "failed_recoverably" || e.Demo || e.Concern != "Synthetic mild fatigue" {
		t.Fatal("rejection lost history or substituted a result")
	}
	endpoint := "/api/encounters/" + e.ID + "/actions"
	goResult := make(chan *httptest.ResponseRecorder, 1)
	go func(rev int) {
		goResult <- c.call("POST", endpoint, object{"action": "retry", "revision": rev, "requestId": token(12)})
	}(e.Revision)
	<-entered
	check(t, c.call("GET", "/api/encounters/"+e.ID, nil), 200, &e)
	command(t, c, &e, object{"action": "correct_answer", "answerId": e.Answers[0].ID, "text": "I cannot breathe."})
	close(release)
	check(t, <-goResult, 409, nil)
	check(t, c.call("GET", "/api/encounters/"+e.ID, nil), 200, &e)
	if e.Concern != "I cannot breathe." || len(e.Warnings) == 0 {
		t.Fatal("late result overwrote correction or warning")
	}
	cfg2 := cfg
	cfg2.Model = "another-fixture"
	cfg2.Endpoint = secondBase
	selectProvider(t, c, cfg2)
	check(t, c.call("POST", "/api/encounters", intake(cfg, "Another synthetic concern")), 409, nil)
	if e.Provider != cfg {
		t.Fatal("existing provider changed silently")
	}
	check(t, c.call("POST", endpoint, object{"action": "change_provider", "provider": cfg2, "revision": e.Revision, "requestId": token(12)}), 400, nil)
	command(t, c, &e, object{"action": "change_provider", "provider": cfg2, "confirmedDestination": true})
	if e.Provider != cfg2 || len(e.ProviderChanges) != 1 || n.Load() != 2 || destinationCalls.Load() != 0 {
		t.Fatal("provider move dispatched prematurely or was not audited")
	}
	activeConcern := ""
	for _, a := range e.Answers {
		if a.Active && a.Kind == "concern" {
			activeConcern = a.ID
		}
	}
	command(t, c, &e, object{"action": "correct_answer", "answerId": activeConcern, "text": "Synthetic revised history after confirmed move."})
	if destinationCalls.Load() != 1 || len(e.Turns) == 0 || e.Turns[len(e.Turns)-1].Meta.Endpoint != secondBase {
		t.Fatal("next explicit clinical request did not use and record the confirmed destination")
	}
	check(t, c.call("POST", "/api/encounters", intake(cfg2, "Synthetic recovery test")), 200, &e)
	if e.State == "failed_recoverably" || len(e.Turns) != 1 || e.Turns[0].Meta.ReturnedModel != "synthetic-fixture-returned" {
		t.Fatal("compatible fixture output failed")
	}
	unready, err := clinical.New(token(16), clinical.Intake{Age: 38, Concern: "Synthetic restart test", NoticeVersion: clinical.NoticeVersion}, cfg2, "")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = db.Create(context.Background(), 1, unready, token(12), []byte("test"), cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.RecoverPending(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored, err := db.Get(context.Background(), 1, unready.ID)
	if err != nil || stored.State != "failed_recoverably" {
		t.Fatal("pending inference did not recover")
	}
	if _, err = db.Get(context.Background(), 2, unready.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("ownership not enforced")
	}
}

func TestRejectCrossOriginAndUnknownHost(t *testing.T) {
	h := newTestServer(nil, io.Discard).Handler()
	for _, tc := range []struct{ host, origin string }{{"unrecognized.invalid", ""}, {"localhost", "https://unrelated.invalid"}} {
		req := httptest.NewRequest("POST", "http://"+tc.host+"/api/setup", strings.NewReader("{}"))
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatal("origin or host accepted", w.Code)
		}
	}
}
