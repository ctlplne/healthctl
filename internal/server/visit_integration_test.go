package server

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ctlplne/healthctl/internal/clinical"
)

func TestReturnVisitSnapshotsAndDiaryPostgres(t *testing.T) {
	t.Setenv("CLINICAL_ALLOWED_ENDPOINTS", "http://127.0.0.1:18031/v1")
	db, _, _ := testStore(t)
	s := newTestServer(db, io.Discard)
	c := caller{h: s.Handler()}
	c.signup(t)
	cfg := clinical.ProviderRef{Kind: "compatible", Endpoint: "http://127.0.0.1:18031/v1", Model: "fictional", Structured: "auto"}
	selectProvider(t, c, cfg)
	item := object{"id": token(16), "onset": "2025-02-04T12:00:00Z", "text": "A synthetic observation during my walk."}
	var original, second clinical.DiaryEntry
	check(t, c.call("POST", "/api/diary", item), 200, &original)
	check(t, c.call("POST", "/api/diary", item), 200, &second)
	if original.Recorded.IsZero() || original != second {
		t.Fatal("idempotent diary timestamp changed")
	}
	item["text"] = "Different words with the same key"
	check(t, c.call("POST", "/api/diary", item), 409, nil)
	in := intake(cfg, "Earlier fictional appointment")
	in["reportFirst"] = true
	var e clinical.Encounter
	check(t, c.call("POST", "/api/encounters", in), 200, &e)
	var reviews []string
	ctx := context.Background()
	for _, id := range []string{"first", "second"} {
		quote := "Reviewed printed wording " + id
		r := clinical.ReportRow{ID: "r1", Original: quote, Excerpt: quote, Locator: "line one", Name: "Fictional measurement", Value: "3", Subject: "self"}
		d := clinical.SourceDocument{ID: id, Title: "Fictional " + id, Kind: "text", BelongsTo: "self", Rows: []clinical.ReportRow{r}}
		updated, err := db.AttachDocument(ctx, 1, e.ID, e.Revision, token(16), []byte("synthetic"), d, []byte("original-"+id), "")
		if err != nil {
			t.Fatal(err)
		}
		e = *updated
		command(t, c, &e, object{"action": "review_report_row", "documentId": id, "rowId": "r1", "row": r})
		reviews = append(reviews, e.ReportReviews[len(e.ReportReviews)-1].ID)
	}
	command(t, c, &e, object{"action": "confirm"})
	retry := intake(cfg, "Second fictional visit")
	visit := clinical.ReturnVisit{Source: e.ID, Revision: e.Revision, ReviewIDs: reviews, DiaryIDs: []string{original.ID}, Consent: true}
	retry["returnVisit"] = visit
	var next clinical.Encounter
	check(t, c.call("POST", "/api/encounters", retry), 200, &next)
	if len(next.ReportReviews) != 2 || next.QuestionSource == nil || next.QuestionSource.Key != "return_change" {
		t.Fatal("return visit did not open with source snapshots")
	}
	for _, d := range next.Documents {
		b, err := db.Original(ctx, 1, next.ID, d.ID)
		if err != nil || !bytes.Equal(b, []byte("original-"+d.ID)) || d.PriorEncounter != e.ID {
			t.Fatal("local original or attribution lost", err)
		}
	}
	var again clinical.Encounter
	check(t, c.call("POST", "/api/encounters", retry), 200, &again)
	if again.ID != next.ID {
		t.Fatal("duplicated return encounter")
	}
	command(t, c, &e, object{"action": "withdraw_report_row", "documentId": "first", "rowId": "r1"})
	retry["requestId"] = token(16)
	check(t, c.call("POST", "/api/encounters", retry), 409, nil)
	data := c.call("GET", "/api/encounters/"+next.ID+"/brief", nil)
	check(t, data, 200, nil)
	if !strings.Contains(data.Body.String(), "A synthetic observation during my walk.") {
		t.Fatal("diary entry missing from handoff")
	}
}

func TestVoiceNeedsSeparateConsentPostgres(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "fixture-platform-key")
	db, _, _ := testStore(t)
	c := caller{h: newTestServer(db, io.Discard).Handler()}
	c.signup(t)
	var caps map[string]bool
	check(t, c.call("GET", "/api/voice", nil), 200, &caps)
	if !caps["openai"] || caps["chatgptPlan"] {
		t.Fatal("incorrect speech capabilities")
	}
	for _, mode := range []string{"chatgpt", "openai"} {
		req := httptest.NewRequest("POST", "http://localhost/api/voice?destination="+mode, strings.NewReader("mock audio"))
		req.Header.Set("X-CSRF-Token", c.csrf)
		req.Header.Set("Content-Type", "audio/wav")
		req.AddCookie(c.cookie)
		res := httptest.NewRecorder()
		c.h.ServeHTTP(res, req)
		check(t, res, 400, nil)
		if mode == "openai" && !strings.Contains(res.Body.String(), "Confirm sending") {
			t.Fatal("remote audio did not require separate clip consent")
		}
	}
}
