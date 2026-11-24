package clinical

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestEvidenceSelection(t *testing.T) {
	e := intake(t, "I have a rash on my arm.", false)
	long := strings.Repeat("The rash on my arm looks the same as it did, and I am describing it in detail. ", 20)
	for i := 0; i < 50; i++ {
		e.appendAnswer("reply", "What else have you noticed?", long, "", "")
	}
	e.appendAnswer("reply", "Anything else?", "I have chest pain right now.", "", "")
	e.Pending, e.State = "interview", StateInterviewing
	full, _ := json.Marshal(e.inputHistory("interview"))
	if len(full) <= modelInputBudget {
		t.Fatalf("the test history must exceed the budget, got %d bytes", len(full))
	}
	in, err := e.ModelInput("interview")
	if err != nil {
		t.Fatalf("a long history must be fitted, not refused: %v", err)
	}
	b, _ := json.Marshal(in)
	if len(b) > modelInputBudget || len(in.OmittedAnswerIDs) == 0 {
		t.Fatalf("input %d bytes with %d omitted answers", len(b), len(in.OmittedAnswerIDs))
	}
	kept := map[string]Answer{}
	for _, a := range in.Answers {
		kept[a.ID] = a
		if a.Kind == "reply" && a.Text != long && a.Text != "I have chest pain right now." {
			t.Fatal("a user's words were changed")
		}
	}
	for _, a := range e.Answers {
		if (a.Kind == "concern" || a.Kind == "screen") && kept[a.ID].ID == "" {
			t.Fatalf("%s answer %s was left out", a.Kind, a.ID)
		}
	}
	for _, w := range e.Warnings {
		if w.Active && kept[w.AnswerID].ID == "" {
			t.Fatal("the answer behind an active warning was left out")
		}
	}
	if last := e.Answers[len(e.Answers)-2]; kept[last.ID].ID == "" {
		t.Fatal("the newest answers must be kept")
	}
	if first := in.OmittedAnswerIDs[0]; first != e.Answers[3].ID && first != e.Answers[2].ID {
		t.Fatalf("the oldest answers go first, got %s", first)
	}
	again, _ := e.ModelInput("interview")
	if !reflect.DeepEqual(in, again) {
		t.Fatal("selection must be deterministic")
	}
	turn(t, e, Proposal{Action: "review"})
	if got := e.Turns[len(e.Turns)-1].OmittedAnswerIDs; len(got) != len(in.OmittedAnswerIDs) {
		t.Fatal("the turn must record what was left out")
	}
}
