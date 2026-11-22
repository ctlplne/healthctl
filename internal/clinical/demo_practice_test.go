package clinical

import (
	"strings"
	"testing"
)

func TestPracticeCollectsUnscriptedWords(t *testing.T) {
	for _, concern := range []string{"A sound seems different in the evenings.", "I feel uncomfortable wearing my shoes."} {
		in := Input{Task: "interview", Age: 40, Concern: concern}
		p := PracticeHistory(in)
		if !strings.HasPrefix(p.Question, "When did") || len(p.Choices) > 0 {
			t.Fatal(p)
		}
		in.Answers = append(in.Answers, Answer{Kind: "reply", Question: p.Question, Text: "Skip", Active: true})
		p = PracticeHistory(in)
		if !strings.HasPrefix(p.Question, "What has changed") {
			t.Fatal("Repeated a skipped question", p)
		}
		in.Answers = append(in.Answers, Answer{Kind: "reply", Question: p.Question, Text: "It is harder to join my usual game.", Active: true})
		p = PracticeHistory(in)
		if !strings.HasPrefix(p.Question, "What would you most") || len(p.Findings) > 0 {
			t.Fatal("Did not use volunteered impact", p)
		}
		in.ReviewOnly = true
		if PracticeHistory(in).Action != "review" {
			t.Fatal("Patient review request was lost")
		}
		in.Task = "assessment"
		if p = PracticeHistory(in); p.Action != "insufficient" || len(p.Items) != 0 {
			t.Fatal("Practice generated a diagnosis", p)
		}
	}
}
