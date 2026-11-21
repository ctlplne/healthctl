package clinical

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func reportCase(t *testing.T, concern string, demo bool) *Encounter {
	t.Helper()
	kind := "compatible"
	if demo {
		kind = "demo"
	}
	e, err := New("report-case", Intake{Concern: concern, Age: 34, ReportFirst: true, NoticeVersion: NoticeVersion}, ProviderRef{Kind: kind, Model: "fixture", Endpoint: "https://example.invalid/v1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if e.PendingTask() != "" {
		t.Fatal("report-first intake dispatched before disclosure")
	}
	return e
}
func docFixture() SourceDocument {
	return SourceDocument{
		ID: "doc-1", Title: "PRIVATE-TITLE urinalysis", Kind: "urine", BelongsTo: "self", Purpose: "A PRIVATE-PURPOSE check", Priority: "PRIVATE-PRIORITY", IssueDate: "2026-11-19",
		Rows: []ReportRow{
			{ID: "lab", Original: "Patient ID: PRIVATE-IDENTIFIER | Leukocyte esterase  Trace", Locator: "page 1, row 2", Name: "Leukocyte esterase", Value: "Trace", CollectionDate: "2026-11-18", Subject: "self"},
			{ID: "other", Original: "PRIVATE-OTHER  42.3", Locator: "page 1, row 3", Name: "PRIVATE-OTHER", Value: "42.3", Subject: "self"},
		},
	}
}
func attach(t *testing.T, e *Encounter, d SourceDocument) {
	t.Helper()
	if err := e.AttachReport(d, ""); err != nil {
		t.Fatal(err)
	}
}
func reviewLab(t *testing.T, e *Encounter) {
	t.Helper()
	r := docFixture().Rows[0]
	r.Excerpt = "Leukocyte esterase  Trace"
	apply(t, e, Command{Action: "review_report_row", DocumentID: "doc-1", RowID: r.ID, Row: r})
}
func approve(t *testing.T, e *Encounter, ids ...string) {
	t.Helper()
	apply(t, e, Command{Action: "authorize_report", DocumentID: "doc-1", RowIDs: ids})
}
func modelJSON(t *testing.T, e *Encounter) string {
	t.Helper()
	in, err := e.ModelInput(e.PendingTask())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(in)
	return string(b)
}

func TestReportFirstDemoUsesReviewedDocumentAndPatientTiming(t *testing.T) {
	e := reportCase(t, DemoReportConcern, true)
	attach(t, e, docFixture())
	reviewLab(t, e)
	if !e.Demo || e.PendingTask() != "" || len(e.Facts) != 1 || e.Facts[0].Status != "documented" {
		t.Fatal("review became a clinical diagnosis or initiated inference")
	}
	preview, err := e.ReportPreview("doc-1", []string{"lab"})
	if err != nil || len(preview) != 1 || preview[0].Quote != "Leukocyte esterase  Trace" || preview[0].IssueDate != "2026-11-19" || preview[0].CollectionDate != "2026-11-18" {
		t.Fatalf("incorrect disclosure: %+v, %v", preview, err)
	}
	approve(t, e, "lab")
	apply(t, e, Command{Action: "continue"})
	if e.QuestionSource == nil || e.QuestionSource.RowID != "lab" || e.PendingTask() != "" {
		t.Fatal("missing locally grounded question")
	}
	apply(t, e, Command{Action: "answer", Text: "Before the sample"})
	body := modelJSON(t, e)
	if strings.Contains(body, "PRIVATE-") || !strings.Contains(body, "Leukocyte esterase") || !strings.Contains(body, "Before the sample") {
		t.Fatalf("wrong disclosure: %s", body)
	}
	play(t, e)
	apply(t, e, Command{Action: "confirm"})
	apply(t, e, Command{Action: "assess"})
	play(t, e)
	if e.State != "assessment_ready" || e.Assessments[0].Items[0].Name != "Bladder infection" || !strings.Contains(e.Assessments[0].Items[0].Explanation, "reviewed report entry") || len(e.Assessments[0].ReviewIDs) != 1 {
		t.Fatalf("document provenance missing: %+v", e.Assessments)
	}
	old := e.Assessments[0]
	r := e.ReportReviews[0].Row
	r.Value = "Negative"
	r.Note = "I disagree with the first transcription."
	apply(t, e, Command{Action: "review_report_row", DocumentID: "doc-1", RowID: r.ID, Row: r})
	report := e.Report()
	if len(report.OutdatedAssessmentVersions) != 1 || report.CurrentAssessmentVersion != 0 || e.Assessments[0].ReviewIDs[0] != old.ReviewIDs[0] || e.ReportReviews[0].Active || !e.ReportReviews[1].Active || e.ReportReviews[1].ApprovedFor != nil || e.Documents[0].Rows[0].Value != "Trace" {
		t.Fatal("correction rewrote the original source, disclosure or assessment")
	}
	apply(t, e, Command{Action: "continue"})
	if e.QuestionSource == nil || e.QuestionSource.Key != "discrepancy" || e.QuestionSource.ReviewID == old.ReviewIDs[0] {
		t.Fatal("follow-up did not re-evaluate corrected evidence")
	}
}

func TestWithdrawingEntryRemovesAllDependentDataFromModel(t *testing.T) {
	e := reportCase(t, "My bladder feels uncomfortable", false)
	attach(t, e, docFixture())
	reviewLab(t, e)
	approve(t, e, "lab")
	apply(t, e, Command{Action: "continue"})
	apply(t, e, Command{Action: "answer", Text: "Sensitive reply keyed to this report row"})
	a := e.Answers[len(e.Answers)-1]
	turn(t, e, Proposal{Action: "ask", Question: "How did the PRIVATE-QUESTION develop?", Reason: "To clarify the reported course.", Outstanding: []string{"PRIVATE-OUTSTANDING"}, Findings: []FindingProposal{find(a, a.Text, "PRIVATE-DERIVED", "reported_present")}})
	apply(t, e, Command{Action: "answer", Text: "PRIVATE-MODEL-REPLY"})
	apply(t, e, Command{Action: "withdraw_report_row", DocumentID: "doc-1", RowID: "lab"})
	apply(t, e, Command{Action: "continue"})
	body := modelJSON(t, e)
	for _, s := range []string{"PRIVATE-", "Leukocyte esterase", "Sensitive reply", "sample for this report", "reportEntries"} {
		if strings.Contains(body, s) {
			t.Fatalf("withdrew row yet %q was transmitted: %s", s, body)
		}
	}
	if !e.ReportReviews[0].Withdrawn || e.Facts[0].Active {
		t.Fatal("lost withdrawal provenance")
	}
	if err := e.Apply(Command{Action: "authorize_report", DocumentID: "doc-1", RowIDs: []string{"lab"}}, ""); err == nil {
		t.Fatal("withdrawn report reauthorized")
	}
}

func TestHeadersWrongIdentityAndEndpointChangesCannotSendReports(t *testing.T) {
	e := reportCase(t, DemoReportConcern, false)
	d := docFixture()
	d.BelongsTo = "unknown"
	attach(t, e, d)
	if err := e.Apply(Command{Action: "review_report_row", DocumentID: d.ID, RowID: "lab", Row: d.Rows[0]}, ""); err == nil {
		t.Fatal("wrong-identity report was reviewed")
	}
	apply(t, e, Command{Action: "report_identity", DocumentID: d.ID, BelongsTo: "self"})
	apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: "lab", Row: d.Rows[0]})
	if _, err := e.ReportPreview(d.ID, []string{"lab"}); err == nil {
		t.Fatal("identifying header was offered for transfer")
	}
	reviewLab(t, e)
	approve(t, e, "lab")
	apply(t, e, Command{Action: "change_provider", ConfirmedDestination: true, Provider: ProviderRef{Kind: "compatible", Model: "another", Endpoint: "http://127.0.0.1:9000/v1"}})
	apply(t, e, Command{Action: "review"})
	apply(t, e, Command{Action: "confirm"})
	apply(t, e, Command{Action: "assess"})
	if strings.Contains(modelJSON(t, e), "Leukocyte") {
		t.Fatal("new provider received an unapproved source")
	}
	apply(t, e, Command{Action: "report_identity", DocumentID: d.ID, BelongsTo: "other"})
	if len(e.disclosed()) != 0 || e.currentReview(d.ID, "lab") != nil || e.Facts[len(e.Facts)-1].Active {
		t.Fatal("wrong patient rows remained in clinical history")
	}
	apply(t, e, Command{Action: "report_identity", DocumentID: d.ID, BelongsTo: "self"})
	if e.currentReview(d.ID, "lab") != nil {
		t.Fatal("earlier review reactivated after identity change")
	}
}

func TestPrintedCriticalNoticeIsLocalImmediateAndIndependent(t *testing.T) {
	e := reportCase(t, "Please explain a routine result", false)
	d := docFixture()
	d.Rows[1].Flag = "Critical high"
	d.Rows[1].Uncertain = true
	attach(t, e, d)
	if e.PendingTask() != "" || e.State != "interrupted_for_safety" || len(e.Facts) > 0 || len(e.Warnings) != 1 || !strings.Contains(e.Warnings[0].Message, "appears") {
		t.Fatal("unverified critical label did not interrupt independently of transcription")
	}
	apply(t, e, Command{Action: "review"})
	body := modelJSON(t, e)
	if strings.Contains(body, "PRIVATE-") || strings.Contains(body, "critical") || strings.Contains(body, "42.3") {
		t.Fatal("unselected warning leaked a document excerpt")
	}
	e.Fail("interview")
	if !e.Warnings[0].Active || !e.Report().Warnings[0].Active {
		t.Fatal("failure dropped a document warning")
	}
	r := d.Rows[1]
	apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: r.ID, Row: r})
	apply(t, e, Command{Action: "withdraw_report_row", DocumentID: d.ID, RowID: r.ID})
	if !e.Warnings[0].Active {
		t.Fatal("withdrawing disclosure cleared separate urgency")
	}
	apply(t, e, Command{Action: "report_identity", DocumentID: d.ID, BelongsTo: "other"})
	if e.Warnings[0].Active || e.Warnings[0].ResolvedRevision == 0 {
		t.Fatal("wrong patient correction did not retain and resolve the original warning")
	}
	for _, text := range []string{"If symptoms worsen, seek emergency care.", "No critical value reported."} {
		e = reportCase(t, "Review a note", false)
		d = docFixture()
		d.Rows = []ReportRow{{ID: "a", Original: text, Locator: "line 1"}}
		attach(t, e, d)
		if e.hasWarning() {
			t.Errorf("generic instructions led to a personal alert: %s", text)
		}
	}
}

func TestReportGroundedQuestionSelectionAndRetirement(t *testing.T) {
	for _, tc := range []struct {
		concern, kind, name string
		want                bool
	}{
		{"I have burning when urinating.", "urine", "Leukocyte esterase", true},
		{"The burning when I urinate started before the sample.", "urine", "Leukocyte esterase", false},
		{"I began having urinary symptoms on 2026-11-14.", "urine", "Leukocyte esterase", false},
		{"Routine screening, no symptoms.", "urine", "Leukocyte esterase", false},
		{"My ankle is sore.", "urine", "Leukocyte esterase", false},
		{"I have a headache.", "blood", "Ferritin", false},
		{"I have a cough.", "respiratory swab", "Influenza", true},
	} {
		e := reportCase(t, tc.concern, false)
		d := docFixture()
		d.Title = "Lab"
		d.Kind = tc.kind
		d.Rows[0].Name = tc.name
		attach(t, e, d)
		r := d.Rows[0]
		r.Excerpt = "Leukocyte esterase  Trace"
		apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: r.ID, Row: r})
		apply(t, e, Command{Action: "continue"})
		if (e.QuestionSource != nil && e.QuestionSource.Key == "sample_timing") != tc.want {
			t.Errorf("wrong grounded question for %q: %s", tc.concern, e.Question)
		}
	}
}

func TestManualReportReviewCannotBeLaunderedThroughPatientEdit(t *testing.T) {
	e := reportCase(t, "Review a note", false)
	d := docFixture()
	d.Rows = nil
	attach(t, e, d)
	apply(t, e, Command{Action: "add_report_passage", DocumentID: d.ID, Text: "History of a blood clot", Locator: "page 2, handwritten line 3"})
	r := e.Documents[0].Rows[0]
	if !r.Manual || !r.Uncertain {
		t.Fatal("manual candidate invented certainty")
	}
	r.Name = "Blood clot history"
	r.Subject = "self"
	r.Uncertain = false
	apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: r.ID, Row: r})
	if err := e.Apply(Command{Action: "correct_fact", FactID: e.Facts[0].ID, Text: "My father had a blood clot.", Concept: r.Name, Subject: "family", Status: "reported_present"}, ""); err == nil {
		t.Fatal("report data laundered into patient answers")
	}
	r.Subject = "family"
	r.Note = "My father had the clot."
	apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: r.ID, Row: r})
	approve(t, e, r.ID)
	apply(t, e, Command{Action: "confirm"})
	apply(t, e, Command{Action: "assess"})
	in, _ := e.ModelInput("assessment")
	if len(in.ReportEntries) != 1 || in.ReportEntries[0].Subject != "family" || in.ReportEntries[0].Note != r.Note || len(in.Facts) != 1 || in.Facts[0].Status != "documented" || len(in.Answers) != 2 {
		t.Fatal("family note turned into a personal symptom or lost its source")
	}
}

func TestReconciledHistoryAndOutcomesRetainSourceAndDate(t *testing.T) {
	source := intake(t, "I used an inhaler yesterday", false)
	turn(t, source, Proposal{Action: "review", Findings: []FindingProposal{find(source.Answers[0], "used an inhaler yesterday", "Inhaler use", "reported_present")}})
	apply(t, source, Command{Action: "confirm"})
	target := reportCase(t, "Preparing for a visit", false)
	if err := target.ReuseFact(source, source.Facts[0].ID, "2026-11-20", "I do not remember its name.", ""); err != nil {
		t.Fatal(err)
	}
	f := target.Facts[0]
	if f.Origin == nil || f.Origin.EncounterID != source.ID || f.SourceType != "reconciled_history" || f.TimeContext == "" || f.Confirmed || source.Facts[0].SourceType != "patient_answer" {
		t.Fatal("reconciliation mutated source or invented current use")
	}
	apply(t, target, Command{Action: "confirm"})
	apply(t, target, Command{Action: "assess"})
	turn(t, target, directItem(f.ID))
	before := target.Assessments[0]
	apply(t, target, Command{Action: "add_outcome", Source: "patient_reported", Date: "2026-11-21", Text: "I discussed the concern at my visit."})
	if target.Outcomes[0].AssessmentVersion != before.Version || target.Assessments[0].ReviewStatus != ReviewStatus {
		t.Fatal("follow-up changed historical review")
	}
	if err := target.Apply(Command{Action: "add_outcome", Source: "documented", Date: "2026-11-21", Text: "Doctor confirmed infection."}, ""); err == nil {
		t.Fatal("undocumented follow-up accepted as a document")
	}
	apply(t, target, Command{Action: "add_outcome", Source: "symptom_change", Date: time.Now().UTC().Format(time.DateOnly), Text: "I cannot breathe."})
	if !target.hasWarning() || target.Report().CurrentAssessmentVersion != 0 || target.Assessments[0].HistoryRevision != before.HistoryRevision {
		t.Fatal("new symptom did not invalidate current estimate or raise warning")
	}
}

func TestPathwaysAreScopedUnreviewedAndPermitMixedConcerns(t *testing.T) {
	for _, tc := range []struct{ s, key string }{{"I have a cough", "respiratory"}, {"I have a headache", "headache"}, {"My belly hurts", "abdominal"}, {"I feel dizzy", "dizziness"}} {
		e := intake(t, tc.s, false)
		in, err := e.ModelInput("interview")
		if err != nil || len(in.Pathways) != 1 || in.Pathways[0].ID != tc.key || in.Pathways[0].ReviewStatus != "unvalidated_development" || len(in.Pathways[0].Context) == 0 {
			t.Fatalf("missing scoped content for %s", tc.key)
		}
	}
	e := intake(t, "I feel dizzy and I have a headache", false)
	if len(e.Pathways) != 2 {
		t.Fatal("second concern lost")
	}
	e = intake(t, "My father had a headache years ago", false)
	if len(e.Pathways) != 0 {
		t.Fatal("family history triggered personal pathway")
	}
	for _, s := range []string{"I have a sudden severe headache", "I have severe abdominal pain", "I have sudden trouble walking", "I cannot breathe"} {
		e = intake(t, s, false)
		if !e.hasWarning() || e.PendingTask() != "" {
			t.Errorf("warning did not interrupt immediately for %q", s)
		}
	}
}

func TestEpisodeMetadataAndStructuredReconciliation(t *testing.T) {
	source := intake(t, "I take an inhaler today.", false)
	f := find(source.Answers[0], "I take an inhaler today.", "Inhaler use", "reported_present")
	f.TimeContext = "today"
	turn(t, source, Proposal{Action: "review", Findings: []FindingProposal{f}})
	apply(t, source, Command{Action: "confirm"})
	apply(t, source, Command{Action: "assess"})
	turn(t, source, directItem(source.Facts[0].ID))
	h := source.HistoryRevision
	assessment := source.Assessments[0]
	apply(t, source, Command{Action: "set_episode", EpisodeTitle: "Autumn visit"})
	target := reportCase(t, "New concern", false)
	if err := target.JoinEpisode(source); err != nil {
		t.Fatal(err)
	}
	if source.HistoryRevision != h || source.ConfirmedRevision != h || source.Report().CurrentAssessmentVersion != assessment.Version || target.EpisodeID != source.ID || target.EpisodeTitle != source.EpisodeTitle || target.PendingTask() != "" {
		t.Fatal("group metadata changed clinical state")
	}
	c := Command{FactID: source.Facts[0].ID, Date: "2026-11-21", Category: "medication", ReconciliationStatus: "uncertain", Text: "I am unsure whether I still use that inhaler."}
	if err := target.ReconcileFact(source, c, ""); err != nil {
		t.Fatal(err)
	}
	copy := target.Facts[0]
	if copy.Status != "uncertain" || copy.Origin.OriginalStatus != "reported_present" || copy.Origin.Category != "medication" || copy.Origin.Date != c.Date || copy.Origin.ReconciliationStatus != "uncertain" || copy.Origin.Note != c.Text || source.Facts[0].Status != "reported_present" || copy.TimeContext == "today" {
		t.Fatalf("current uncertainty lost prior evidence or period: %+v", copy)
	}
	c.ReconciliationStatus = "invented_status"
	if target.ReconcileFact(source, c, "") == nil {
		t.Fatal("accepted invented reconciliation status")
	}
	c.ReconciliationStatus = "no_longer_applies"
	c.Category = "allergy"
	c.Text = ""
	if target.ReconcileFact(source, c, "") == nil {
		t.Fatal("changed medication or allergy status without current evidence")
	}
	standalone := intake(t, "another concern", false)
	standalone.ID = "another"
	if err := target.JoinEpisode(standalone); err != nil || target.EpisodeID != standalone.ID || target.EpisodeTitle != standalone.Concern {
		t.Fatal("failed to join a standalone episode")
	}
}

func TestCurrentPrintedConcernAndWarningHandoffBoundary(t *testing.T) {
	for _, tc := range []struct {
		s, day string
		want   bool
	}{
		{"Patient ID: PRIVATE | Patient currently has crushing chest pain.", "", true},
		{"Patient has no chest pain now.", "", false},
		{"My father has chest pain now.", "", false},
		{"If you have chest pain now, call the office.", "", false},
		{"Patient currently has chest pain.", "1996-01-14", false},
	} {
		e := reportCase(t, "Review a note", false)
		d := docFixture()
		d.IssueDate = tc.day
		d.Rows = []ReportRow{{ID: "note", Original: tc.s, Locator: "line 3"}}
		attach(t, e, d)
		if e.hasWarning() != tc.want {
			t.Errorf("incorrect upload warning for %q", tc.s)
		}
		if tc.want {
			w := e.HandoffWarnings()
			if len(w) != 1 || !w[0].Active || strings.Contains(w[0].Quote, "PRIVATE") || !strings.Contains(e.Report().Warnings[0].Quote, "PRIVATE") {
				t.Fatal("handoff exposed an unreviewed original or lost the underlying notice")
			}
		}
	}
}

func TestReviewConflictUsesSameDayTestAndUnits(t *testing.T) {
	const date = "2026-11-18"
	e := intake(t, "I remember glucose 105 mg/dL on 2026-11-18.", false)
	f := find(e.Answers[0], e.Concern, "Glucose", "reported_present")
	f.Value = "105"
	f.Units = "mg/dL"
	f.TimeContext = date
	turn(t, e, Proposal{Action: "review", Findings: []FindingProposal{f}})
	d := docFixture()
	d.Rows = []ReportRow{{ID: "g", Original: "Glucose  155 mg/dL", Locator: "row 1", Name: "Glucose", Value: "155", Units: "mg/dL", CollectionDate: date, Subject: "self"}}
	attach(t, e, d)
	r := d.Rows[0]
	apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: r.ID, Row: r})
	apply(t, e, Command{Action: "continue"})
	if e.QuestionSource == nil || e.QuestionSource.Key != "discrepancy" || len(e.Contradictions) != 1 || len(e.Contradictions[0].FactIDs) != 2 {
		t.Fatal("same-day conflict was lost")
	}
	r.CollectionDate = "2026-11-17"
	apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: r.ID, Row: r})
	if len(e.Contradictions) != 0 {
		t.Fatal("a measurement from another day became a contradiction")
	}
	r.CollectionDate = date
	r.Value = "105.0"
	apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: r.ID, Row: r})
	if len(e.Contradictions) != 0 {
		t.Fatal("numeric formatting alone became a contradiction")
	}
}

func TestDatedReconciliationDoesNotResurrectPastMedicationStatus(t *testing.T) {
	s := intake(t, "I used an inhaler.", false)
	turn(t, s, Proposal{Action: "review", Findings: []FindingProposal{find(s.Answers[0], s.Concern, "Inhaler use", "reported_present")}})
	apply(t, s, Command{Action: "confirm"})
	e := reportCase(t, "Review my earlier medications", false)
	for _, c := range []Command{
		{Date: "2026-10-20", Text: "I still used it that day.", ReconciliationStatus: "still_applies"},
		{Date: "2026-11-15", Text: "I had stopped using it by this date.", ReconciliationStatus: "no_longer_applies"},
		{Date: "2026-10-28", Text: "I was unsure what I used.", ReconciliationStatus: "uncertain"},
	} {
		c.Category = "medication"
		c.FactID = s.Facts[0].ID
		if err := e.ReconcileFact(s, c, ""); err != nil {
			t.Fatal(err)
		}
	}
	active := 0
	for _, f := range e.Facts {
		if f.Active {
			active++
			if f.Origin.Date != "2026-11-15" || f.Origin.ReconciliationStatus != "no_longer_applies" {
				t.Fatal("backdated note replaced current status")
			}
		}
	}
	if active != 1 || len(e.Facts) != 3 || e.Facts[1].Supersedes != e.Facts[0].ID || s.Facts[0].Status != "reported_present" {
		t.Fatal("reconciliation versions missing")
	}
}

func TestHistoricalSymptomChangeKeepsEventDateWithoutNewPersonalWarning(t *testing.T) {
	e := reportCase(t, "Preparing for a visit", false)
	apply(t, e, Command{Action: "add_outcome", Source: "symptom_change", Text: "I have chest pain.", Date: "1996-03-02"})
	last := e.Answers[len(e.Answers)-1]
	if e.hasWarning() || last.EventDate != "1996-03-02" || last.CreatedAt.Year() == 1996 {
		t.Fatal("historical event treated as a present symptom")
	}
	apply(t, e, Command{Action: "continue"})
	f := find(last, last.Text, "Chest pain", "reported_present")
	turn(t, e, Proposal{Action: "review", Findings: []FindingProposal{f}})
	if e.hasWarning() || e.Facts[len(e.Facts)-1].TimeContext != "1996-03-02" {
		t.Fatal("model erased event timing or fabricated a current warning")
	}
}

func TestReportConsentAndIdentityChangeAuditIsLocal(t *testing.T) {
	e := reportCase(t, "Preparing for a visit", false)
	attach(t, e, docFixture())
	reviewLab(t, e)
	approve(t, e, "lab")
	oldID := e.ReportReviews[0].ID
	approve(t, e)
	if len(e.ReportConsents) != 2 || e.ReportConsents[0].ReviewIDs[0] != oldID || len(e.ReportConsents[1].ReviewIDs) != 0 {
		t.Fatal("historical consent missing")
	}
	apply(t, e, Command{Action: "report_identity", DocumentID: "doc-1", BelongsTo: "other"})
	if len(e.ReportIdentityChanges) != 1 || e.ReportIdentityChanges[0].Previous != "self" || e.ReportReviews[0].RetiredRevision == 0 {
		t.Fatal("missing identity/review history")
	}
	apply(t, e, Command{Action: "continue"})
	b := modelJSON(t, e)
	if strings.Contains(b, "reportConsents") || strings.Contains(b, "reportIdentityChanges") {
		t.Fatal("local audit sent to model")
	}
}

func TestGroupedSampleDoesNotRepeatQuestionOrIgnoreDecline(t *testing.T) {
	e := reportCase(t, DemoReportConcern, false)
	d := docFixture()
	d.Rows[1] = ReportRow{ID: "nitrite", Name: "Nitrite", Value: "Negative", Subject: "self", Original: "Urine nitrite: negative", Locator: "page 1 row 3", CollectionDate: d.Rows[0].CollectionDate}
	attach(t, e, d)
	reviewLab(t, e)
	apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: "nitrite", Row: d.Rows[1]})
	apply(t, e, Command{Action: "continue"})
	if e.QuestionSource == nil || e.QuestionSource.Key != "sample_timing" {
		t.Fatal("missing first question")
	}
	apply(t, e, Command{Action: "answer", Text: "Prefer not to answer"})
	if e.QuestionSource != nil && e.QuestionSource.Key == "sample_timing" {
		t.Fatal("asked the declined question again for another row in the same specimen")
	}
	r := d.Rows[1]
	r.CollectionDate = "2026-10-01"
	apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: "nitrite", Row: r})
	apply(t, e, Command{Action: "continue"})
	if e.QuestionSource == nil || e.QuestionSource.Key != "sample_timing" || e.QuestionSource.RowID != "nitrite" {
		t.Fatal("did not reconsider a materially corrected collection date")
	}
}

func TestUrgentPatientReviewCommentDoesNotOverwritePrintedFlag(t *testing.T) {
	e := reportCase(t, "Review my notes", false)
	d := docFixture()
	r := d.Rows[1]
	r.Flag = "Critical"
	d.Rows = []ReportRow{r}
	attach(t, e, d)
	r.Note = "I cannot breathe right now."
	apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: r.ID, Row: r})
	if len(e.Warnings) != 2 || e.Warnings[1].Rule != "report_review_comment" || e.Warnings[0].Rule != "report_printed_urgency" {
		t.Fatal("new personal comment was merged with a document flag")
	}
	r.Note = "I meant my father could not breathe in 1998."
	apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: r.ID, Row: r})
	if !e.Warnings[0].Active || e.Warnings[1].Active {
		t.Fatal("comment correction cleared an independent document warning")
	}
}

func TestModelFollowupLinksSpecificSelectedImagingRow(t *testing.T) {
	e := reportCase(t, "I want to prepare for a visit.", false)
	d := docFixture()
	d.Kind = "imaging"
	d.Title = "Written MRI report"
	d.Rows = []ReportRow{
		{ID: "impression", Original: "Impression: unchanged focus.", Locator: "page 2 line 10", Name: "Impression", Subject: "self"},
		{ID: "comparison", Original: "Comparison: prior study unavailable.", Locator: "page 2 line 2", Name: "Comparison", Subject: "self"},
	}
	attach(t, e, d)
	for _, r := range d.Rows {
		apply(t, e, Command{Action: "review_report_row", DocumentID: d.ID, RowID: r.ID, Row: r})
	}
	approve(t, e, "impression", "comparison")
	apply(t, e, Command{Action: "continue"})
	in, _ := e.ModelInput("interview")
	target := e.currentReview(d.ID, "comparison").ID
	q := Proposal{Action: "ask", Question: "Do you remember when the prior study described by the comparison was performed?", Reason: "The comparison passage does not list the date."}
	for _, reason := range []string{q.Reason, "[report:unknown] The report lacks a date.", "[report:rr9999] The report lacks a date.", "[report:rr1] The comparison passage lacks a date.", "[report:" + target + "] [report:" + target + "] A date would clarify this."} {
		bad := q
		bad.Reason = reason
		if Validate(in, bad) == nil {
			t.Fatalf("invalid question reference accepted: %s", reason)
		}
	}
	q.Reason = "[report:" + target + "] " + q.Reason
	turn(t, e, q)
	if e.QuestionSource == nil || e.QuestionSource.DocumentID != d.ID || e.QuestionSource.RowID != "comparison" || e.QuestionSource.ReviewID != target || strings.Contains(e.QuestionSource.Reason, "[report:") || len(e.QuestionReviewIDs) != 2 {
		t.Fatal("question attribution did not identify the selected row")
	}
	last := e.Turns[len(e.Turns)-1]
	if last.QuestionSource == nil || last.QuestionSource.RowID != "comparison" || strings.Contains(last.Reason, "[report:") {
		t.Fatal("lost source in archived turn")
	}
	apply(t, e, Command{Action: "withdraw_report_row", DocumentID: d.ID, RowID: "comparison"})
	apply(t, e, Command{Action: "continue"})
	in, _ = e.ModelInput("interview")
	if Validate(in, q) == nil {
		t.Fatal("withdrew a source but its reference was still accepted")
	}
	generic := Proposal{Action: "ask", Question: "How has your concern changed?", Reason: "The course can affect the assessment."}
	turn(t, e, generic)
	if e.QuestionSource == nil || e.QuestionSource.ReviewID != "" {
		t.Fatal("general question acquired a false document attribution")
	}
}
