package interchange

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ctlplne/healthctl/internal/clinical"
)

func TestHandoffKeepsWarningButProtectsUnreviewedRowAndRelatedSubjects(t *testing.T) {
	e := &clinical.Encounter{
		EncounterIdentity: clinical.EncounterIdentity{ID: "synthetic", Notice: "Historical notice at intake", ReviewStatus: clinical.ReviewStatus},
		HistoryRecord:     clinical.HistoryRecord{ConfirmedRevision: 1, HistoryRevision: 1},
		SafetyRecord:      clinical.SafetyRecord{Warnings: []clinical.Warning{{DocumentID: "d1", RowID: "r1", Message: "Printed urgent notice", Active: true, Evidence: clinical.Evidence{Quote: "MRN: PRIVATE-SYNTHETIC"}}}},
		SourceRecords:     clinical.SourceRecords{ReportReviews: []clinical.ReportReview{{ID: "rr2", RowID: "r2", DocumentID: "d1", Active: true, Row: clinical.ReportRow{Name: "Past event", Subject: "family", Excerpt: "family note"}}}},
	}
	b := FromEncounter(e)
	f := FHIR(b)
	if b.ModelGuidance != clinical.ModelGuidance() || e.Report().ModelGuidance != b.ModelGuidance {
		t.Fatal("full and brief exports must carry the current model recommendation")
	}
	composition := f["entry"].([]resource)[0]["resource"].(resource)
	if !strings.Contains(composition["text"].(resource)["div"].(string), b.ModelGuidance) {
		t.Fatal("FHIR document narrative lost the model recommendation")
	}
	if b.Notice != "Historical notice at intake" || e.Report().Notice != b.Notice ||
		b.PreviewNotice != clinical.PreviewNotice() || e.Report().PreviewNotice != b.PreviewNotice ||
		!strings.Contains(composition["text"].(resource)["div"].(string), b.PreviewNotice) {
		t.Fatal("old encounters must retain the consent and export the current beta notice")
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
