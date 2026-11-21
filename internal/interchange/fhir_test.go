package interchange

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ctlplne/healthctl/internal/clinical"
)

func TestHandoffKeepsWarningButProtectsUnreviewedRowAndRelatedSubjects(t *testing.T) {
	e := &clinical.Encounter{ID: "synthetic", ConfirmedRevision: 1, HistoryRevision: 1, Notice: clinical.Notice, ReviewStatus: clinical.ReviewStatus,
		Warnings:      []clinical.Warning{{DocumentID: "d1", RowID: "r1", Message: "Printed urgent notice", Active: true, Evidence: clinical.Evidence{Quote: "MRN: PRIVATE-SYNTHETIC"}}},
		ReportReviews: []clinical.ReportReview{{ID: "rr2", RowID: "r2", DocumentID: "d1", Active: true, Row: clinical.ReportRow{Name: "Past event", Subject: "family", Excerpt: "family note"}}}}
	b := FromEncounter(e)
	f := FHIR(b)
	if b.ModelGuidance != clinical.ModelGuidance() || e.Report().ModelGuidance != b.ModelGuidance {
		t.Fatal("full and brief exports must carry the current model recommendation")
	}
	composition := f["entry"].([]resource)[0]["resource"].(resource)
	if !strings.Contains(composition["text"].(resource)["div"].(string), b.ModelGuidance) {
		t.Fatal("FHIR document narrative lost the model recommendation")
	}
	raw, _ := json.Marshal(f)
	if strings.Contains(string(raw), "PRIVATE-SYNTHETIC") || !strings.Contains(string(raw), "Printed urgent notice") || !strings.Contains(string(raw), "family note") {
		t.Fatal("handoff lost its source or urgency boundary")
	}
	for _, entry := range f["entry"].([]resource) {
		if entry["resource"].(resource)["resourceType"] == "Observation" {
			t.Fatal("a relative's report became the patient's observation")
		}
	}
}
