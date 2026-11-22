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
