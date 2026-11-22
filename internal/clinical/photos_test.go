package clinical

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVisibleChangeHistory(t *testing.T) {
	for _, concern := range []string{DemoNailConcern, "My shoulder clicks.", "My shoes feel different on one side."} {
		e := guided(t, concern)
		if e.Pending != "interview" || e.Question != "" {
			t.Fatal("Connected provider did not receive the open concern", concern)
		}
		rev := e.HistoryRevision
		if err := e.AttachPhoto("private-photo-id", "Outside of my left ankle"); err == nil {
			t.Fatal("A pending request must finish before saving a photo")
		}
		turn(t, e, Proposal{Action: "ask", Question: "What did you notice first?", Reason: "Clarify the change."})
		if err := e.AttachPhoto("private-photo-id", "Outside of my left ankle"); err != nil || rev != e.HistoryRevision {
			t.Fatal(err)
		}
		data, _ := json.Marshal(e.inputHistory("interview"))
		if strings.Contains(string(data), "private-photo-id") || strings.Contains(string(data), "Outside of my left ankle") {
			t.Fatal("Photo in provider payload")
		}
	}
	for _, area := range []string{"", " ", "Two\nlines", strings.Repeat("x", 161)} {
		if ValidPhotoArea(area) {
			t.Fatal("Accepted invalid body area")
		}
	}
	for _, tc := range []struct {
		s       string
		warning bool
	}{{"The whites of my eyes look yellow.", true}, {"My eyes have turned yellow.", true}, {"My eyes are not yellow.", false}, {"My father has yellow eyes.", false}, {"I had yellow eyes years ago.", false}} {
		e := intake(t, tc.s, false)
		if e.hasWarning() != tc.warning {
			t.Fatal(tc.s)
		}
		if tc.warning {
			rev := e.HistoryRevision
			if err := e.AttachPhoto("p", "Eye"); err != nil {
				t.Fatal(err)
			}
			if !e.hasWarning() || e.Pending != "" || e.State != "interrupted_for_safety" || e.HistoryRevision != rev {
				t.Fatal("Saving a local photo changed the urgent interruption")
			}
		}
	}
}

func TestDemoNailBranches(t *testing.T) {
	for _, tc := range []struct{ where, next string }{{"One thumbnail", "What happened to that thumb"}, {"Several nails on both hands", "What else, if anything"}} {
		e, err := New("case", Intake{Concern: DemoNailConcern, Age: 34, NoticeVersion: NoticeVersion}, ProviderRef{Kind: "demo", Model: "fictional"}, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, answer := range []string{"The centers dip inward", tc.where, "Gradually over three months"} {
			play(t, e)
			apply(t, e, Command{Action: "answer", Text: answer})
		}
		play(t, e)
		if !strings.HasPrefix(e.Question, tc.next) {
			t.Fatal(e.Question)
		}
		for _, f := range e.Facts {
			if f.Concept == "Koilonychia" || f.Concept == "Anemia" {
				t.Fatal("invented diagnosis")
			}
		}
		if tc.where == "Several nails on both hands" {
			apply(t, e, Command{Action: "answer", Text: "I need to rest halfway up the stairs"})
			play(t, e)
			apply(t, e, Command{Action: "answer", Text: "I used to climb two flights in one go"})
			play(t, e)
			if e.State != "history_review" {
				t.Fatal("Review unavailable:", e.State)
			}
		}
		if len(e.Facts) < 4 {
			t.Fatal("Missing quoted history", len(e.Facts))
		}
	}
}

func TestNewVisualObservationChangesConfirmedHistory(t *testing.T) {
	e := intake(t, "My fingernails have changed shape.", false)
	turn(t, e, Proposal{Action: "review"})
	e.ConfirmedRevision = e.HistoryRevision
	previous := e.HistoryRevision
	apply(t, e, Command{Action: "visual_observation", Category: "Eye", Text: "Today the whites of my eyes look yellow."})
	if e.HistoryRevision <= previous || e.ConfirmedRevision != 0 || !e.hasWarning() || e.State != "interrupted_for_safety" {
		t.Fatal("Lost revision or warning")
	}
	in := e.inputHistory("interview")
	last := in.Answers[len(in.Answers)-1]
	if last.Kind != "visual_observation" || last.Text != "Today the whites of my eyes look yellow." {
		t.Fatal("Lost patient attribution")
	}
}
