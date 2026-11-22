package clinical

import "testing"

func TestDisplayPreservesOriginalWordsAndCorrections(t *testing.T) {
	const original = "I told the patient to call back."
	const label = "Soreness when the patient kneels"
	e := Encounter{Facts: []Fact{
		{SourceType: "patient_answer", FindingProposal: FindingProposal{Concept: label, Evidence: Evidence{Quote: original}}},
		{SourceType: "patient_correction", Corrected: true, FindingProposal: FindingProposal{Concept: label}},
	}}
	report := e.Report()
	if report.Facts[0].Concept != "Soreness when the user kneels" || report.Facts[0].Quote != original || report.Facts[1].Concept != label || e.Facts[0].Concept != label {
		t.Fatal("display changed evidence, a manual edit, or the stored history")
	}
}

func TestRoleLabelsInOlderModelText(t *testing.T) {
	const q = "Patient: when did this start?"
	const reason = "Would this change the patient’s plans?"
	const evidence = "The patient at work told me to stay patient."
	old := Encounter{Question: q, QuestionSource: &QuestionSource{Reason: reason}, Choices: []string{"The patient is unsure"}, Answers: []Answer{{Question: q, Text: evidence}}, Outstanding: []string{"Patient-reported timing"}, Assessments: []Assessment{{Items: []Item{{Explanation: "Consider the patient. See “The patient at work”.", Limitations: []string{reason}}}}}}
	shown := old.ForDisplay()
	if shown.Question != "User: when did this start?" || shown.Answers[0].Text != evidence || shown.QuestionSource.Reason != "Would this change the user’s plans?" || shown.Choices[0] != "The user is unsure" || shown.Outstanding[0] != "Self-reported timing" || shown.Assessments[0].Items[0].Explanation != "Consider the user. See “The patient at work”." || shown.Assessments[0].Items[0].Limitations[0] != shown.QuestionSource.Reason {
		t.Fatal("role label or evidence changed unexpectedly", shown)
	}
	if old.QuestionSource.Reason != reason || old.Choices[0] != "The patient is unsure" || old.Assessments[0].Items[0].Limitations[0] != reason || userReference("Please stay patient during outpatient care.") != "Please stay patient during outpatient care." {
		t.Fatal("presentation changed source data or unrelated words")
	}
}
