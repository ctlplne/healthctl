package evaluation

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/documents"
)

//go:embed fixtures/blurred-test-panel.png
var blurredPanel []byte

type ReportCase struct {
	Literature        []CitationAudit          `json:"citationAudits,omitempty"`
	RunID             string                   `json:"runId,omitempty"`
	Repetition        int                      `json:"repetition,omitempty"`
	Variant           string                   `json:"variant"`
	Status            string                   `json:"status"`
	FirstQuestion     string                   `json:"firstQuestion,omitempty"`
	QuestionSource    *clinical.QuestionSource `json:"questionSource,omitempty"`
	DisclosureEntries int                      `json:"disclosedEntries"`
	Outcome           string                   `json:"assessmentOutcome,omitempty"`
	Failures          []string                 `json:"failures"`
	Observations      []string                 `json:"observations"`
}

func ReportComparisons(ctx context.Context, m Model) []ReportCase {
	out := []ReportCase{}
	for _, variant := range []string{"opening_only", "relevant_urine_report", "unrelated_report", "timing_already_known", "wrong_patient", "blurred_decimal_missing_unit"} {
		r := ReportCase{Variant: variant, Status: "exercised", Failures: []string{}, Observations: []string{}}
		if err := runReportCase(ctx, &r, m); err != nil {
			r.Failures = append(r.Failures, err.Error())
			r.Status = "failed"
		}
		out = append(out, r)
	}
	return out
}
func runReportCase(ctx context.Context, r *ReportCase, m Model) error {
	variant := r.Variant
	if variant == "blurred_decimal_missing_unit" {
		if _, err := exec.LookPath("tesseract"); err != nil {
			r.Status = "tool_unavailable"
			r.Observations = append(r.Observations, "Tesseract missing in this runtime. Repeat on the app image with English OCR.")
			return nil
		}
		_, rows, err := documents.Read(ctx, "synthetic.png", blurredPanel)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return errors.New("synthetic scan yielded no text")
		}
		foundDecimal := false
		for _, row := range rows {
			if !row.Uncertain || row.Units != "" {
				return errors.New("OCR silently asserted certainty or filled the missing unit")
			}
			if strings.Contains(row.Original, "1.8") {
				foundDecimal = true
			}
		}
		r.Observations = append(r.Observations, fmt.Sprintf("Rows marked uncertain: %d; expected decimal visible: %t; missing unit preserved.", len(rows), foundDecimal))
		if !foundDecimal {
			return errors.New("expected decimal missing or misread; source review required")
		}
		return nil
	}
	opening := clinical.DemoReportConcern
	if variant == "timing_already_known" {
		opening += " The symptoms started before the urine sample."
	}
	e, err := clinical.New("report-benchmark", clinical.Intake{Concern: opening, Age: 34, ReportFirst: true, NoticeVersion: clinical.NoticeVersion}, m.Reference(), "")
	if err != nil {
		return err
	}
	command := func(c clinical.Command) error { return e.Apply(c, "") }
	rowName := "Leukocyte esterase"
	if variant == "unrelated_report" {
		rowName = "Visual acuity"
	}
	raw := "Patient name: PRIVATE_HEADER_MARKER\nTest | Result | Units | Reference range | Flag | Collection date | Issue date\n" + rowName + " | Trace |  | Negative |  | 2026-11-18 | 2026-11-19\nSECOND_PRIVATE_ROW | 37 | units | 35-40 |  | 2026-11-18 | 2026-11-19"
	kind, rows, err := documents.Read(ctx, "fictional.txt", []byte(raw))
	if err != nil {
		return err
	}
	var row clinical.ReportRow
	for _, candidate := range rows {
		if candidate.Name == rowName {
			row = candidate
		}
	}
	if row.Value != "Trace" || row.Units != "" || row.ReferenceRange != "Negative" || row.CollectionDate != "2026-11-18" || row.IssueDate != "2026-11-19" {
		return errors.New("multi-column extraction lost a result, qualifier, empty unit, reference or dates")
	}
	row.Subject = "self"
	if variant != "opening_only" {
		owner := "self"
		if variant == "wrong_patient" {
			owner = "other"
		}
		if err := e.AttachReport(clinical.SourceDocument{ID: "doc", Title: "Synthetic report", Kind: kind, BelongsTo: owner, Rows: rows}, ""); err != nil {
			return err
		}
		review := clinical.Command{Action: "review_report_row", DocumentID: "doc", RowID: row.ID, Row: row}
		err := command(review)
		if variant == "wrong_patient" {
			if err == nil {
				return errors.New("entry belonging to someone else accepted")
			}
			r.Observations = append(r.Observations, "Entry belonging to someone else rejected; no report findings shared.")
			return nil
		}
		if err != nil {
			return err
		}
		if err := command(clinical.Command{Action: "authorize_report", DocumentID: "doc", RowIDs: []string{row.ID}}); err != nil {
			return err
		}
	}
	if err := command(clinical.Command{Action: "continue"}); err != nil {
		return err
	}
	r.FirstQuestion = e.Question
	r.QuestionSource = e.QuestionSource
	grounded := e.QuestionSource != nil && e.QuestionSource.Key == "sample_timing"
	if grounded != (variant == "relevant_urine_report") {
		return errors.New("timing question was added or omitted for the wrong reason")
	}
	if grounded {
		if e.QuestionSource.RowID != row.ID || e.QuestionSource.DocumentID != "doc" {
			return errors.New("question attribution did not identify selected row")
		}
		if err := command(clinical.Command{Action: "answer", Text: "Before the sample"}); err != nil {
			return err
		}
	}
	call := func(task string) error {
		in, err := e.ModelInput(task)
		if err != nil {
			return err
		}
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "PRIVATE_") {
			return errors.New("unselected original or identifier appeared in proposed outbound input")
		}
		if variant != "opening_only" && len(in.ReportEntries) != 1 {
			return errors.New("authorized entry missing from provider payload")
		}
		r.DisclosureEntries = len(in.ReportEntries)
		p, meta, err := m.Propose(ctx, in)
		if err != nil {
			return err
		}
		return e.Accept(task, p, meta, "")
	}
	if e.PendingTask() == "interview" {
		if err := call("interview"); err != nil {
			return err
		}
		if r.FirstQuestion == "" {
			r.FirstQuestion = e.Question
			r.QuestionSource = e.QuestionSource
		}
	}
	for _, action := range []string{"review", "confirm", "assess"} {
		if err := command(clinical.Command{Action: action}); err != nil {
			return err
		}
	}
	if err := call("assessment"); err != nil {
		return err
	}
	a := e.Assessments[len(e.Assessments)-1]
	r.Outcome = a.Outcome
	r.Literature = auditAssessment(e, a, &r.Failures)
	if variant != "opening_only" {
		for _, f := range e.Facts {
			if f.Concept == row.Name && f.SourceType != "report_entry" {
				return errors.New("document result turned into a reported symptom")
			}
		}
		original := row.Original
		// A corrected transcription retains its original, supersedes the old
		// evidence and requires new disclosure to the bound destination.
		row.Value = "Uncertain"
		row.Uncertain = true
		if err := command(clinical.Command{Action: "review_report_row", DocumentID: "doc", RowID: row.ID, Row: row}); err != nil {
			return err
		}
		if e.Report().CurrentAssessmentVersion != 0 {
			return errors.New("correction left an older assessment current")
		}
		x, err := e.ReportPreview("doc", []string{row.ID})
		if err != nil || len(x) != 1 || x[0].Value != "Uncertain" || x[0].Quote != original {
			return errors.New("correction lost source, value or revision linkage")
		}
		if err := command(clinical.Command{Action: "withdraw_report_row", DocumentID: "doc", RowID: row.ID}); err != nil {
			return err
		}
		if _, err := e.ReportPreview("doc", []string{row.ID}); err == nil {
			return errors.New("withdrawn value remained disclosable")
		}
		for _, f := range e.Report().Facts {
			if f.Active && f.SourceType == "report_entry" {
				return errors.New("withdrawal left a current report claim")
			}
		}
		r.Observations = append(r.Observations, "Original row and collection/issue dates preserved; corrected assessment stale; withdrawn row excluded.")
	}
	return nil
}
