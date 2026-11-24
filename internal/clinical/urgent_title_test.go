package clinical

import "testing"

// The app records each urgent sign it recognizes as a finding. Its title is a
// readable term, not the raw lowercase phrase, and correcting the finding
// still clears its warning. Development content, not clinically reviewed.
func TestUrgentSignFindingTitles(t *testing.T) {
	e := intake(t, "Yesterday the right side of my face drooped for a few minutes and my speech was slurred.", false)
	titles := map[string]string{}
	for _, f := range e.Facts {
		if f.Active {
			titles[f.Quote] = f.Concept
		}
	}
	if titles["face drooped"] != "Facial droop" || titles["speech was slurred"] != "Slurred speech" {
		t.Fatalf("urgent signs need readable titles: %v", titles)
	}
	for _, w := range e.Warnings {
		if w.Active && w.Rule == "reported_warning" && w.FactID == "" {
			t.Fatalf("a warning lost its finding: %+v", w)
		}
	}

	// Words without a vocabulary term keep their wording, with a capital.
	if got := urgentTitle("lips turned blue"); got != "Lips turned blue" {
		t.Fatalf("fallback title = %q", got)
	}
	// A term the urgent-sign rules wouldn't recognize is not used, so the
	// finding stays linked to its warning.
	if got := urgentTitle("trouble breathing"); got != "Trouble breathing" {
		t.Fatalf("a term that hides the urgent sign was used: %q", got)
	}

	// Correcting the titled finding clears its warning.
	var droop Fact
	for _, f := range e.Facts {
		if f.Active && f.Concept == "Facial droop" {
			droop = f
		}
	}
	apply(t, e, Command{Action: "correct_fact", FactID: droop.ID, Text: "I didn't have any face drooping; my lip was numb from the dentist.", Concept: "Facial droop", Status: "explicitly_denied", Subject: "self"})
	for _, w := range e.Warnings {
		if w.Active && w.FactID == droop.ID {
			t.Fatal("correcting the finding must clear its warning")
		}
	}
}
