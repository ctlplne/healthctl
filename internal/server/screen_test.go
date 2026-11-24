package server

import (
	"io"
	"testing"
	"time"

	"github.com/ctlplne/healthctl/internal/clinical"
)

func TestDraftResponseScreens(t *testing.T) {
	s := &Server{Emergency: "Fictional emergency line 000."}
	out := s.draftResponse("I have crushing chest pain right now.", "local")
	w, ok := out["warning"].(clinical.ScreenResult)
	if !ok || w.Level != "emergency" || w.Quote == "" || w.PolicyVersion != clinical.PolicyVersion {
		t.Fatalf("dictation draft was not screened: %+v", out)
	}
	if _, found := s.draftResponse("My nose is runny.", "local")["warning"]; found {
		t.Fatal("routine draft raised a warning")
	}
}

func TestDiarySaveScreensPostgres(t *testing.T) {
	db, _, _ := testStore(t)
	s := newTestServer(db, io.Discard)
	c := caller{h: s.Handler()}
	c.signup(t)
	var saved clinical.DiaryEntry
	body := object{"id": "diary-urgent-0001", "text": "It happened again and I can't breathe.", "onset": time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339), "approximate": false,
		"warning": object{"level": "check", "quote": "forged", "message": "forged", "policyVersion": "forged"}}
	check(t, c.call("POST", "/api/diary", body), 200, &saved)
	if saved.Warning == nil || saved.Warning.Level != "emergency" || saved.Warning.Quote == "forged" {
		t.Fatalf("diary save was not screened by the application: %+v", saved.Warning)
	}
	var entries []clinical.DiaryEntry
	check(t, c.call("GET", "/api/diary", nil), 200, &entries)
	if len(entries) != 1 || entries[0].Warning == nil {
		t.Fatal("the stored diary entry lost its warning")
	}
}
