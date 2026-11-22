package clinical

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestVisibleChangeHistory(t *testing.T) {
	for _, tc := range []struct{ reply, next string }{{"One nail", "nail_local_event"}, {"Several nails", "nail_sequence"}, {"I prefer not to answer.", ""}} {
		e, err := New("n", Intake{Concern: DemoNailConcern, Age: 40, GuidedHistory: true, NoticeVersion: NoticeVersion}, ProviderRef{Kind: "compatible", Model: "test"}, "")
		if err != nil || e.QuestionSource == nil || e.QuestionSource.Key != "nail_distribution" {
			t.Fatalf("missing question: %v", err)
		}
		rev := e.HistoryRevision
		if err = e.AttachPhoto("private-photo-id", "Fingernails or hands"); err != nil || rev != e.HistoryRevision {
			t.Fatal(err)
		}
		if err = e.Apply(Command{Action: "answer", Text: tc.reply}, ""); err != nil {
			t.Fatal(err)
		}
		if tc.next != "" && (e.QuestionSource == nil || e.QuestionSource.Key != tc.next) {
			t.Fatal(tc, e.Question)
		}
		if tc.next == "" && e.Pending != "interview" {
			t.Fatal("Repeated a skipped question")
		}
		data, _ := json.Marshal(e.inputHistory("interview"))
		if strings.Contains(string(data), "private-photo-id") || strings.Contains(string(data), "Fingernails or hands") {
			t.Fatal("Photo in provider payload")
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
		if tc.warning && (e.Pending != "" || e.AttachPhoto("p", "Eye") == nil) {
			t.Fatal("Photo or interview took priority over warning")
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
