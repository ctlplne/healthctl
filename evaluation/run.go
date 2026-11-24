package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ctlplne/healthctl/internal/provider"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ctlplne/healthctl/internal/clinical"
)

type Metric struct {
	Matched int `json:"matched"`
	Total   int `json:"total"`
}
type Result struct {
	Literature        []CitationAudit    `json:"citationAudits,omitempty"`
	RunID             string             `json:"runId,omitempty"`
	Repetition        int                `json:"repetition,omitempty"`
	ElapsedUS         *int64             `json:"elapsedMicroseconds,omitempty"`
	PromptVersion     string             `json:"promptVersion,omitempty"`
	SchemaVersion     string             `json:"schemaVersion,omitempty"`
	AppVersion        string             `json:"appVersion,omitempty"`
	MissedAlerts      int                `json:"missedAlertCheckpoints"`
	ExcessiveAlerts   int                `json:"excessiveAlertCheckpoints"`
	ElapsedMS         int64              `json:"elapsedMs"`
	RepeatedQuestions int                `json:"repeatedQuestions"`
	ModelUsage        []provider.Usage   `json:"modelUsage,omitempty"`
	ReturnedModels    []string           `json:"returnedModels,omitempty"`
	Pathways          []clinical.Pathway `json:"pathways,omitempty"`

	Case                    string          `json:"case"`
	Arm                     string          `json:"arm"`
	Model                   string          `json:"model"`
	InterviewQuestions      int             `json:"interviewQuestions"`
	ModelCalls              int             `json:"modelCalls"`
	VisibleTargetExtraction Metric          `json:"visibleTargetExtraction"`
	KeyHistory              Metric          `json:"keyHistory"`
	MissedTargets           []string        `json:"missedTargets"`
	Unadjudicated           []string        `json:"unadjudicatedFindings"`
	FabricatedFindings      []string        `json:"fabricatedFindings"`
	StaleAssessments        []string        `json:"staleAssessments"`
	LostWarnings            []string        `json:"lostWarnings"`
	AlertErrors             []string        `json:"alertErrors"`
	ContradictionErrors     []string        `json:"contradictionErrors"`
	Errors                  []string        `json:"errors"`
	CheckpointAlerts        map[string]bool `json:"checkpointAlerts"`
	Contradictions          int             `json:"contradictions"`
	ExpectedContradiction   bool            `json:"expectedContradiction"`
	AssessmentOutcome       string          `json:"assessmentOutcome"`
	AssessedRevision        int             `json:"assessedRevision"`
	RevisedHistory          int             `json:"revisedHistory"`
	ReferenceRank           *int            `json:"exploratoryReferenceRank"`
}

func (r Result) Failed() bool {
	return len(r.FabricatedFindings)+len(r.StaleAssessments)+len(r.LostWarnings)+len(r.AlertErrors)+len(r.ContradictionErrors)+len(r.Errors) > 0
}

func Run(ctx context.Context, c Case, adaptive bool, model Model, maxTurns int) Result {
	arm := "free_text_only"
	if adaptive {
		arm = "adaptive"
	}
	return RunArm(ctx, c, arm, model, maxTurns)
}

func RunArm(ctx context.Context, c Case, arm string, model Model, maxTurns int) (r Result) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		us := elapsed.Microseconds()
		r.ElapsedUS = &us
		r.ElapsedMS = elapsed.Milliseconds()
	}()
	r = Result{Case: c.ID, Arm: arm, Model: model.Reference().Model, CheckpointAlerts: map[string]bool{}, Errors: []string{}, MissedTargets: []string{}, Unadjudicated: []string{}, FabricatedFindings: []string{}, StaleAssessments: []string{}, LostWarnings: []string{}, AlertErrors: []string{}, ContradictionErrors: []string{}}
	intake := clinical.Intake{GuidedHistory: arm == "adaptive", Age: c.Age, Concern: c.Opening, UrgentNow: c.UrgentScreen, PregnancyRelated: c.PregnancyRelated, NoticeVersion: clinical.NoticeVersion}
	e, err := clinical.New("synthetic-encounter", intake, model.Reference(), "")
	if err != nil {
		r.Errors = append(r.Errors, err.Error())
		return r
	}
	r.Pathways = e.Pathways
	if e.ScopeReason != "" {
		r.AssessmentOutcome = "history_only"
		if len(e.Report().Assessments) != 0 || e.PendingTask() != "" || e.ScopeReason == "" {
			r.Errors = append(r.Errors, "unsupported case entered assessment workflow")
		}
		return r
	}
	p := &patient{replies: c.Replies, sent: map[int]bool{}}
	err = runJourney(ctx, &r, c, e, model, maxTurns, arm == "adaptive", scripted{p})
	if err != nil {
		r.Errors = append(r.Errors, err.Error())
	}
	return r
}

func runJourney(ctx context.Context, r *Result, c Case, e *clinical.Encounter, model Model, maxTurns int, adaptive bool, patient Answerer) error {
	activeWarning := func() bool {
		for _, w := range e.Warnings {
			if w.Active {
				return true
			}
		}
		return false
	}
	checkAlert := func(checkpoint string, want bool) {
		got := activeWarning()
		r.CheckpointAlerts[checkpoint] = got
		if got != want {
			if want {
				r.MissedAlerts++
			} else {
				r.ExcessiveAlerts++
			}
			r.AlertErrors = append(r.AlertErrors, fmt.Sprintf("%s: expected %t, observed %t", checkpoint, want, got))
		}
	}
	checkAlert("intake", c.Alert)
	initialWarnings := slices.Clone(e.Warnings)
	expectedAlert := c.Alert
	if c.Correction != "" {
		if err := e.Apply(clinical.Command{Action: "correct_answer", AnswerID: e.Answers[0].ID, Text: c.Correction}, ""); err != nil {
			return err
		}
		expectedAlert = c.AfterCorrectionAlert
		checkAlert("after_patient_correction", expectedAlert)
		for _, initial := range initialWarnings {
			if !slices.ContainsFunc(e.Report().Warnings, func(w clinical.Warning) bool { return w.ID == initial.ID && w.Message == initial.Message }) {
				r.LostWarnings = append(r.LostWarnings, "corrected warning missing from historical report: "+initial.ID)
			}
		}
	}
	remembered := map[string]bool{}
	saveWarnings := func() {
		for _, w := range e.Warnings {
			if w.Active {
				remembered[w.ID] = true
			}
		}
	}
	guardWarnings := func(phase string) { checkWarnings(r, remembered, e.Report(), phase) }
	saveWarnings()

	invoke := func(task string, reviewOnly bool) error {
		in, err := e.ModelInput(task)
		if err != nil {
			return err
		}
		if reviewOnly {
			in.ReviewOnly = true
		}
		r.ModelCalls++
		p, meta, err := model.Propose(ctx, in)
		if meta.PromptVersion != "" {
			r.PromptVersion, r.SchemaVersion, r.AppVersion = meta.PromptVersion, meta.SchemaVersion, meta.AppVersion
		}
		if meta.ReturnedModel != "" && !slices.Contains(r.ReturnedModels, meta.ReturnedModel) {
			r.ReturnedModels = append(r.ReturnedModels, meta.ReturnedModel)
		}
		if usage, ok := meta.Settings["usage"].(provider.Usage); ok {
			r.ModelUsage = append(r.ModelUsage, usage)
		}
		if err != nil {
			e.Fail(task)
			guardWarnings("model failure")
			return fmt.Errorf("%s proposal: %w", task, err)
		}
		if err := e.Accept(task, p, meta, ""); err != nil {
			e.Fail(task)
			guardWarnings("rejected output")
			return err
		}
		guardWarnings(task)
		saveWarnings()
		return nil
	}
	seenQuestions := map[string]bool{}
	if r.Arm == "static_form" {
		for _, q := range []string{"When did the symptoms begin?", "What other symptoms have you noticed?", "What medications do you take?"} {
			if activeWarning() {
				break
			}
			// This benchmark arm substitutes a prespecified questionnaire for
			// the interviewer, then uses the ordinary extraction and assessment.
			e.Question = q
			text, err := patient.Answer(ctx, q)
			if err != nil {
				return err
			}
			if err := e.Apply(clinical.Command{Action: "answer", Text: text}, ""); err != nil {
				return err
			}
			r.InterviewQuestions++
			if c.AlertWhen != "" && strings.Contains(text, c.AlertWhen) {
				expectedAlert = true
			}
			checkAlert(fmt.Sprintf("static_%d", r.InterviewQuestions), expectedAlert)
			guardWarnings("static answer")
			saveWarnings()
		}
	}
	for {
		if e.PendingTask() == "interview" {
			if err := invoke("interview", !adaptive); err != nil {
				return err
			}
		}
		if !adaptive || e.Question == "" || r.InterviewQuestions >= maxTurns {
			break
		}
		if seenQuestions[e.Question] {
			r.RepeatedQuestions++
		}
		seenQuestions[e.Question] = true
		answer, err := patient.Answer(ctx, e.Question)
		if err != nil {
			return err
		}
		if err := e.Apply(clinical.Command{Action: "answer", Text: answer}, ""); err != nil {
			return err
		}
		r.InterviewQuestions++
		if c.AlertWhen != "" && strings.Contains(answer, c.AlertWhen) {
			expectedAlert = true
		}
		checkAlert(fmt.Sprintf("answer_%d", r.InterviewQuestions), expectedAlert)
		guardWarnings("answer")
		saveWarnings()
	}
	if err := e.Apply(clinical.Command{Action: "review"}, ""); err != nil {
		return err
	}
	if e.PendingTask() == "interview" {
		if err := invoke("interview", true); err != nil {
			return err
		}
	}
	checkAlert("review", expectedAlert)
	guardWarnings("review")
	saveWarnings()
	scoreFacts(r, c, e)
	r.ExpectedContradiction = c.ConflictWhen != "" && disclosed(e, c.ConflictWhen)
	r.Contradictions = len(e.Contradictions)
	if (r.Contradictions > 0) != r.ExpectedContradiction {
		r.ContradictionErrors = append(r.ContradictionErrors, "Unexpected contradiction state after review")
	}
	for _, action := range []string{"confirm", "assess"} {
		if err := e.Apply(clinical.Command{Action: action}, ""); err != nil {
			return err
		}
	}
	// Exercise the real failure transition on a detached copy of the pending
	// encounter. Scoring the copied report avoids extra live provider calls.
	var failed clinical.Encounter
	snapshot, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(snapshot, &failed); err != nil {
		return err
	}
	failed.Fail("assessment")
	if failed.State != "failed_recoverably" {
		r.Errors = append(r.Errors, "assessment failure did not enter recoverable state")
	}
	checkWarnings(r, remembered, failed.Report(), "simulated provider failure")

	if err := invoke("assessment", false); err != nil {
		return err
	}
	checkAlert("assessment", expectedAlert)
	a := e.Assessments[len(e.Assessments)-1]
	r.AssessmentOutcome = a.Outcome
	r.Literature = auditAssessment(e, a, &r.Errors)
	r.AssessedRevision = a.HistoryRevision
	if c.Insufficient && a.Outcome != "insufficient" {
		r.Errors = append(r.Errors, "expected insufficient information for this authored case")
	}
	if c.Reference != "" {
		rank := 0
		pattern := regexp.MustCompile("(?i)" + c.Reference)
		for i, item := range a.Items {
			if pattern.MatchString(item.Name + " " + item.ClinicalTerm) {
				rank = i + 1
				break
			}
		}
		r.ReferenceRank = &rank
	}
	before, err := json.Marshal(e.Report().Assessments)
	if err != nil {
		return err
	}
	if err := e.Apply(clinical.Command{Action: "answer", Text: "I have nothing further to add just now."}, ""); err != nil {
		return err
	}
	report := e.Report()
	if err := checkRevision(r, before, report); err != nil {
		return err
	}
	guardWarnings("revised history export")
	return nil
}

func checkWarnings(r *Result, expected map[string]bool, report clinical.Report, phase string) {
	for id := range expected {
		if !slices.ContainsFunc(report.Warnings, func(w clinical.Warning) bool { return w.Active && w.ID == id }) {
			r.LostWarnings = append(r.LostWarnings, phase+": "+id)
		}
	}
}

func checkRevision(r *Result, before []byte, report clinical.Report) error {
	r.RevisedHistory = report.HistoryRevision
	if len(report.Assessments) == 0 || report.Assessments[len(report.Assessments)-1].HistoryRevision == report.HistoryRevision || report.ConfirmedRevision == report.HistoryRevision || report.State == "assessment_ready" {
		r.StaleAssessments = append(r.StaleAssessments, "earlier estimate remained current after history revision")
	}
	after, err := json.Marshal(report.Assessments)
	if err != nil {
		return err
	}
	if !bytes.Equal(before, after) {
		r.StaleAssessments = append(r.StaleAssessments, "a previous assessment changed after adding information")
	}
	return nil
}

func disclosed(e *clinical.Encounter, fragment string) bool {
	for _, a := range e.Answers {
		if a.Active && strings.Contains(strings.ToLower(a.Text), strings.ToLower(fragment)) {
			return true
		}
	}
	return false
}
func past(value string) bool {
	return regexp.MustCompile(`(?i)\b(past|histor|ago|last year|childhood|prior|previous)`).MatchString(value)
}
func scoreFacts(r *Result, c Case, e *clinical.Encounter) {
	active := map[string]clinical.Answer{}
	for _, a := range e.Answers {
		if a.Active {
			active[a.ID] = a
		}
	}
	found := map[string]bool{}
	for _, f := range e.Facts {
		if !f.Active {
			continue
		}
		a, ok := active[f.AnswerID]
		if !ok || f.Quote == "" || !strings.Contains(a.Text, f.Quote) {
			r.FabricatedFindings = append(r.FabricatedFindings, f.ID+": missing active source span")
			continue
		}
		known, matched, unresolved := false, false, false
		for _, target := range c.Facts {
			if !regexp.MustCompile("(?i)" + target.Concept).MatchString(f.Concept) {
				continue
			}
			known = true
			if !strings.Contains(strings.ToLower(a.Text), strings.ToLower(target.Source)) {
				continue
			}
			matched = true
			if f.Status != target.Status || f.Subject != target.Subject {
				r.FabricatedFindings = append(r.FabricatedFindings, f.ID+": unsupported status or subject for "+target.Label)
				continue
			}
			if target.Time == "past" && !past(f.TimeContext) {
				unresolved = true
				continue
			}
			found[target.Label] = true
		}
		if known && !matched {
			r.FabricatedFindings = append(r.FabricatedFindings, f.ID+": authored concept unsupported by cited answer")
		}
		for _, rule := range c.Forbidden {
			if regexp.MustCompile("(?i)"+rule.Concept).MatchString(f.Concept) && f.Status == rule.Status && (rule.Subject == "" || f.Subject == rule.Subject) {
				r.FabricatedFindings = append(r.FabricatedFindings, f.ID+": authored prohibited finding "+f.Concept)
			}
		}
		if !known || unresolved {
			r.Unadjudicated = append(r.Unadjudicated, f.ID+": "+f.Concept)
		}
	}
	for _, target := range c.Facts {
		r.KeyHistory.Total++
		if found[target.Label] {
			r.KeyHistory.Matched++
		} else {
			r.MissedTargets = append(r.MissedTargets, target.Label)
		}
		if disclosed(e, target.Source) {
			r.VisibleTargetExtraction.Total++
			if found[target.Label] {
				r.VisibleTargetExtraction.Matched++
			}
		}
	}
}
