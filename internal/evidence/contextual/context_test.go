package contextual

import (
	"testing"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/evidence"
)

func TestApplicabilityRequiresSupportingCurrentSelfReport(t *testing.T) {
	pack := evidence.Starter()
	c := evidence.Build([]evidence.ActivePack{{Pack: pack, Hash: "fixture"}})
	fact := clinical.Fact{ID: "f1", Active: true, Confirmed: true, SourceType: "patient_answer",
		FindingProposal: clinical.FindingProposal{Concept: "Sneezing", Status: "reported_present", Subject: "self"}}
	item := clinical.Item{Name: "Nasal allergies", ClinicalTerm: "allergic rhinitis", Supporting: []string{"f1"}}
	for _, tc := range []struct {
		name      string
		age       int
		change    func(*clinical.Fact)
		supported bool
	}{
		{"reviewed symptom", 47, func(*clinical.Fact) {}, true},
		{"unconfirmed", 47, func(f *clinical.Fact) { f.Confirmed = false }, false},
		{"denial", 47, func(f *clinical.Fact) { f.Status = "explicitly_denied" }, false},
		{"family", 47, func(f *clinical.Fact) { f.Subject = "family" }, false},
		{"past", 47, func(f *clinical.Fact) { f.TimeContext = "last year" }, false},
		{"source expired", 47, func(f *clinical.Fact) { f.SourceOutdated = true }, false},
		{"report result", 47, func(f *clinical.Fact) { f.SourceType = "report_entry" }, false},
		{"outside age range", 15, func(*clinical.Fact) {}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fact
			tc.change(&f)
			result := Explain(c, &clinical.Encounter{EncounterIdentity: clinical.EncounterIdentity{Age: tc.age}, HistoryRecord: clinical.HistoryRecord{Facts: []clinical.Fact{f}}}, 0, item)
			if (result.Relation == "supported_context") != tc.supported || (len(result.ClaimLinks) > 0) != tc.supported {
				t.Fatalf("unexpected relation %s", result.Relation)
			}
		})
	}
}
