package clinical

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTransitionTable checks the explicit state machine: every action Apply
// handles is in the table, actions outside their allowed states are refused,
// every observed change matches the table, and the generated diagram is current.
func TestTransitionTable(t *testing.T) {
	for action := range commandFields {
		if action == "reuse_fact" {
			continue // the server reconciles facts across encounters
		}
		if _, ok := transitionTable[action]; !ok {
			t.Fatalf("%s has command fields but no transition rule", action)
		}
	}
	for action := range transitionTable {
		if strings.Contains(action, ":") {
			continue
		}
		if _, ok := commandFields[action]; !ok {
			t.Fatalf("%s has a transition rule but no command fields", action)
		}
		e := intake(t, "I have a rash on my arm.", false)
		if err := e.Apply(Command{Action: action}, ""); err != nil && err.Error() == "Unknown encounter action" {
			t.Fatalf("Apply does not handle %s", action)
		}
	}
	e := intake(t, "I have a rash on my arm.", false)
	for _, action := range []string{"confirm", "assess", "retry"} {
		if err := e.Apply(Command{Action: action}, ""); err == nil {
			t.Fatalf("%s was allowed while interviewing", action)
		}
	}
	if err := checkTransition("confirm", StateHistoryReview, StateAssessing); err == nil {
		t.Fatal("the table allowed a change it doesn't list")
	}

	// Drive random encounters, with scripted model turns, and check every change.
	texts := []string{"Yesterday", "No", "not sure", "Yes", "I have chest pain and no fever", "It started 3 days ago", "None", "Getting worse", "I have a cough.", "I have a headache.", DemoConcern}
	actions := []string{"answer", "answer", "answer", "correct_answer", "correct_fact", "review", "confirm", "assess", "retry", "continue", "focus_interview", "report_emergency", "correct_intake", "visual_observation", "set_episode", "red_flag", "safety_check", "change_provider"}
	proposals := []Proposal{{Action: "review"}, {Action: "ask", Question: "Is it worse at night?", Reason: "Pattern."}, {Action: "insufficient", Outstanding: []string{"More detail."}}, {Action: "ask", Reason: "[intent:onset]"}}
	observed := 0
	for seed := int64(0); seed < 1200; seed++ {
		r := rand.New(rand.NewSource(seed))
		kind := []string{"demo", "compatible"}[seed%2]
		in := Intake{Concern: texts[r.Intn(len(texts))], Age: 16 + r.Intn(70), NoticeVersion: NoticeVersion, GuidedHistory: r.Intn(2) == 0, Focused: r.Intn(3) == 0, UrgentNow: r.Intn(8) == 0, ReportFirst: r.Intn(10) == 0}
		e, err := New("t", in, ProviderRef{Kind: kind, Model: "m"}, "")
		if err != nil {
			continue
		}
		for step := 0; step < 30; step++ {
			from := e.State
			if task := e.PendingTask(); task != "" {
				mi, err := e.ModelInput(task)
				var p Proposal
				switch {
				case kind == "demo" && err == nil:
					p = Demo(mi)
				case task == "assessment" && len(e.Facts) > 0 && r.Intn(2) == 0:
					p = directItem(e.Facts[0].ID)
				case task == "assessment":
					p = Proposal{Action: "insufficient", Outstanding: []string{"More."}}
				default:
					p = proposals[r.Intn(len(proposals))]
				}
				event := "model:" + task
				if err != nil || r.Intn(6) == 0 || e.Accept(task, p, ModelMeta{}, "") != nil {
					e.Fail(task)
					event = "fail:" + task
				}
				if err := checkTransition(event, from, e.State); err != nil {
					t.Fatalf("seed %d: %v", seed, err)
				}
				observed++
				continue
			}
			c := Command{Action: actions[r.Intn(len(actions))]}
			switch c.Action {
			case "answer", "visual_observation":
				c.Text = texts[r.Intn(len(texts))]
				if c.Action == "visual_observation" {
					c.Category = "Eye"
				}
			case "correct_answer":
				c.Text = texts[r.Intn(len(texts))]
				if len(e.Answers) > 0 {
					c.AnswerID = e.Answers[r.Intn(len(e.Answers))].ID
				}
				b := r.Intn(2) == 0
				c.UrgentNow = &b
			case "correct_fact":
				c.Text = "No cough"
				if len(e.Facts) > 0 {
					c.FactID = e.Facts[r.Intn(len(e.Facts))].ID
				}
				c.Concept, c.Status, c.Subject = "cough", "uncertain", "self"
			case "correct_intake":
				b := r.Intn(4) == 0
				c.Age, c.PregnancyRelated = 10+r.Intn(80), &b
			case "set_episode":
				c.EpisodeTitle = "Episode"
			case "red_flag":
				if len(e.RedFlagsDue) > 0 {
					c.Category = e.RedFlagsDue[0].Key
				}
				c.Status = []string{"yes", "no", "unsure"}[r.Intn(3)]
			case "safety_check":
				c.Category, c.Status = "review", []string{"none", "changed", "emergency"}[r.Intn(3)]
			case "change_provider":
				c.ConfirmedDestination, c.Provider = true, ProviderRef{Kind: kind, Model: "other"}
			}
			if err := e.Apply(c, ""); err != nil {
				if e.State != from {
					t.Fatalf("seed %d: a refused %s changed the state from %s to %s", seed, c.Action, from, e.State)
				}
				continue
			}
			if err := checkTransition(c.Action, from, e.State); err != nil {
				t.Fatalf("seed %d: %v", seed, err)
			}
			observed++
		}
	}
	if observed < 10000 {
		t.Fatalf("only %d transitions observed", observed)
	}

	path := filepath.Join("..", "..", "docs", "state-machine.md")
	if os.Getenv("UPDATE_STATE_DIAGRAM") == "1" {
		if err := os.WriteFile(path, []byte(StateDiagram()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	committed, err := os.ReadFile(path)
	if err != nil || string(committed) != StateDiagram() {
		t.Fatal("docs/state-machine.md is out of date; run UPDATE_STATE_DIAGRAM=1 go test -run TestTransitionTable ./internal/clinical")
	}
}

func TestTypedCommands(t *testing.T) {
	e := intake(t, "I have a rash on my arm.", false)
	for _, c := range []Command{
		{Action: "answer", Text: "Yesterday", Age: 40},
		{Action: "review", Text: "ignored words"},
		{Action: "confirm", FactID: "f1"},
		{Action: "red_flag", Category: "red_flag:respiratory:rest", Status: "no", Speaker: "someone"},
		{Action: "launch_rocket"},
	} {
		if err := e.Apply(c, ""); err == nil {
			t.Fatalf("%+v was accepted with a field its action doesn't use", c)
		}
	}
	if len(e.Answers) != 2 || e.HistoryRevision != 1 {
		t.Fatal("a refused command changed the encounter")
	}
	if err := CheckCommand(Command{Action: "answer", Text: "Yesterday", ForRevision: 3}); err != nil {
		t.Fatalf("the envelope fields belong to every command: %v", err)
	}
	b := true
	if err := CheckCommand(Command{Action: "report_emergency", UrgentNow: &b}); err != nil {
		t.Fatalf("the emergency flag belongs to report_emergency: %v", err)
	}
}
