package clinical

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	urgentQuestion = "Do you think you may be having a medical emergency right now?"
	urgentYes      = "Yes, I may be having a medical emergency right now."
	urgentNo       = "I did not select the current emergency warning."
)

func id(prefix string, n int) string { return fmt.Sprintf("%s%d", prefix, n+1) }
func validText(s string, limit int) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && len(s) <= limit
}
func (e *Encounter) touch()              { e.Revision++; e.UpdatedAt = time.Now().UTC() }
func (e *Encounter) PendingTask() string { return e.Pending }

func New(encounterID string, intake Intake, provider ProviderRef, emergency string) (*Encounter, error) {
	if len(intake.Agenda) > 600 || intake.Age < 1 || intake.Age > 125 || !validText(intake.Concern, 4000) || len(intake.Background) > 4000 || intake.NoticeVersion != NoticeVersion {
		return nil, errors.New("Enter an age from 1 to 125, a chief concern and acknowledge the current notice")
	}
	e := &Encounter{
		DemoPractice: intake.DemoPractice && provider.Kind == "demo" && !intake.ReportFirst,
		Agenda:       strings.TrimSpace(intake.Agenda), Focused: intake.Focused, NarrativeFirst: intake.NarrativeFirst, GuidedHistory: intake.GuidedHistory, ReportFirst: intake.ReportFirst, ID: encounterID, Concern: strings.TrimSpace(intake.Concern), Background: strings.TrimSpace(intake.Background),
		Age: intake.Age, PregnancyRelated: intake.PregnancyRelated,
		Provider: provider, Demo: provider.Kind == "demo", CreatedAt: time.Now().UTC(),
		Notice: Notice, NoticeVersion: NoticeVersion, ReviewStatus: ReviewStatus, HistoryRevision: 1,
		IntakeHistory: []IntakeRevision{{Age: intake.Age, PregnancyRelated: intake.PregnancyRelated, HistoryRevision: 1}},
		Answers:       []Answer{}, Facts: []Fact{}, Warnings: []Warning{}, Contradictions: []Contradiction{},
		Outstanding: []string{}, Choices: []string{}, Assessments: []Assessment{},
		Summaries: []Summary{}, Turns: []Turn{}, ProviderChanges: []ProviderChange{},
	}
	e.touch()
	e.appendAnswer("concern", "What brings you here?", e.Concern, "", emergency)
	if e.Background != "" {
		e.appendAnswer("background", "What other background would you like to share?", e.Background, "", emergency)
	}
	screenText := urgentNo
	if intake.UrgentNow {
		screenText = urgentYes
	}
	e.appendAnswer("screen", urgentQuestion, screenText, "", emergency)
	if e.Agenda != "" {
		e.appendAnswer("agenda", "What would you most like this visit to answer?", e.Agenda, "", emergency)
	}
	e.setScope()
	e.routeInterview()
	if intake.ReportFirst {
		e.reportState()
	} else if e.GuidedHistory && e.Provider.Kind == "chatgpt" && e.Pending == "interview" {
		e.askPlanOpening()
	}
	return e, nil
}

func (e *Encounter) appendAnswer(kind, question, text, supersedes, emergency string) Answer {
	return e.appendDatedAnswer(kind, question, text, supersedes, "", emergency)
}

func (e *Encounter) appendDatedAnswer(kind, question, text, supersedes, eventDate, emergency string) Answer {
	a := Answer{EventDate: eventDate, ID: id("a", len(e.Answers)), Kind: kind, Question: question, Text: text, Active: true, Supersedes: supersedes, CreatedAt: time.Now().UTC()}
	if kind == "reply" {
		a.Disposition = responseDisposition(text, question)
	}
	e.Answers = append(e.Answers, a)
	if kind != "correction" || supersedes != "" {
		e.screen(a, emergency)
	}
	return a
}

func (e *Encounter) setScope() {
	e.ScopeReason = ""
	switch {
	case e.Age < 18:
		e.ScopeReason = "This interview covers adults aged 18 and older. You can save or export the reported history."
	case e.PregnancyRelated:
		e.ScopeReason = "This interview does not cover pregnancy-related concerns. You can save or export the reported history."
	}
}
func (e *Encounter) routeInterview() {
	e.QuestionSource = nil
	e.QuestionReviewIDs = nil
	e.Pathways = e.activePathways()
	e.ReviewExtraction = false
	e.Question = ""
	e.Choices = []string{}
	e.Pending = ""
	e.Failure = ""
	e.FailedTask = ""
	switch {
	case e.ScopeReason != "":
		e.State = "unsupported_scope"
	case e.hasWarning():
		e.State = "interrupted_for_safety"
	case len(e.Answers) >= 64:
		e.State = "history_review"
		e.Outstanding = []string{"The interview reached its context limit. Review the saved history."}
	case e.focusedLimitReached():
		e.State = "history_review"
		e.Pending = "interview"
		e.ReviewExtraction = true
	case e.askEventTiming():
	case e.askSourceDifference():
	case e.groundedQuestion():
	case e.askEpisodeChange():
	case e.askFocusedQuestion():
	default:
		e.State = "interviewing"
		e.Pending = "interview"
	}
}

func (e *Encounter) answer(aid string) *Answer {
	for i := range e.Answers {
		if e.Answers[i].ID == aid && e.Answers[i].Active {
			return &e.Answers[i]
		}
	}
	return nil
}
func (e *Encounter) fact(fid string) *Fact {
	for i := range e.Facts {
		if e.Facts[i].ID == fid && e.Facts[i].Active {
			return &e.Facts[i]
		}
	}
	return nil
}

func (e *Encounter) changed() {
	e.HistoryRevision++
	e.ConfirmedRevision = 0
}

func (e *Encounter) recordFinding(f FindingProposal, source, supersedes string, corrected bool) {
	a := e.answer(f.AnswerID)
	e.recordFindingAt(f, source, supersedes, corrected, strings.Index(a.Text, f.Quote))
}

func (e *Encounter) recordFindingAt(f FindingProposal, source, supersedes string, corrected bool, i int) {
	for _, old := range e.Facts {
		if old.Active && old.FindingProposal == f {
			return
		}
	}
	e.Facts = append(e.Facts, Fact{
		FindingProposal: f, ID: id("f", len(e.Facts)), HistoryRevision: e.HistoryRevision,
		SourceType: source, StartByte: i, EndByte: i + len(f.Quote),
		Active: true, Corrected: corrected, Supersedes: supersedes,
	})
}

func (e *Encounter) rebuildContradictions() {
	e.Contradictions = []Contradiction{}
	for i, a := range e.Facts {
		if !a.Active {
			continue
		}
		for _, b := range e.Facts[i+1:] {
			if !b.Active || !strings.EqualFold(a.Concept, b.Concept) || a.Subject != b.Subject || !sameTime(a.TimeContext, b.TimeContext) {
				continue
			}
			if a.Status == "reported_present" && b.Status == "explicitly_denied" || b.Status == "reported_present" && a.Status == "explicitly_denied" {
				e.Contradictions = append(e.Contradictions, Contradiction{a.Concept, []string{a.ID, b.ID}})
			}
		}
	}
	e.updateReportConflicts()
}

func sameTime(a, b string) bool {
	if strings.EqualFold(a, b) {
		return true
	}
	// Two undated/current accounts can conflict. Explicitly distinct historical
	// intervals must not be folded into a single present symptom.
	current := func(t string) bool {
		switch strings.ToLower(t) {
		case "", "now", "current", "currently", "present", "today":
			return true
		}
		return false
	}
	return current(a) && current(b)
}

func (e *Encounter) Apply(c Command, emergency string) error {
	switch c.Action {
	case "episode_log", "witness", "medication_use", "resolve_sources", "clinician_feedback":
		if err := e.visitEntry(c, emergency); err != nil {
			return err
		}
	case "visual_observation":
		if e.Pending != "" || e.ScopeReason != "" || e.hasWarning() || len(e.Answers) >= 64 || !ValidPhotoArea(c.Category) || !validText(c.Text, 2000) {
			return errors.New("Choose a body area and describe your own observation in an available interview")
		}
		e.changed()
		e.appendAnswer("visual_observation", "What did you notice yourself? ("+c.Category+")", strings.TrimSpace(c.Text), "", emergency)
		e.routeInterview()
	case "safety_check":
		if e.Pending != "" {
			return errors.New("Finish or cancel the current request before answering the safety question")
		}
		if err := e.safetyCheck(c, emergency); err != nil {
			return err
		}
	case "set_episode":
		if e.Pending != "" {
			return errors.New("Finish or cancel the current request before changing the episode")
		}
		if c.SourceEncounterID != "" || !validText(c.EpisodeTitle, 180) {
			return errors.New("Enter a title for this episode, or choose another encounter to join")
		}
		if e.EpisodeID == "" {
			e.EpisodeID = e.ID
		}
		e.EpisodeTitle = strings.TrimSpace(c.EpisodeTitle)
	case "review_report_row", "withdraw_report_row", "authorize_report", "report_identity", "add_report_passage":
		if err := e.applyReport(c, emergency); err != nil {
			return err
		}
	case "add_outcome":
		if err := e.addOutcome(c, emergency); err != nil {
			return err
		}
	case "correct_intake":
		if c.Age < 1 || c.Age > 125 || c.PregnancyRelated == nil {
			return errors.New("Enter an age from 1 to 125 and choose the pregnancy-related scope")
		}
		e.changed()
		e.Age = c.Age
		e.PregnancyRelated = *c.PregnancyRelated
		e.IntakeHistory = append(e.IntakeHistory, IntakeRevision{Age: e.Age, PregnancyRelated: e.PregnancyRelated, HistoryRevision: e.HistoryRevision})
		e.setScope()
		e.routeInterview()
	case "answer", "correct_answer":
		if err := e.applyAnswer(c, emergency); err != nil {
			return err
		}
	case "correct_fact":
		if err := e.correctFact(c, emergency); err != nil {
			return err
		}
	case "report_emergency":
		if len(e.Answers) >= 256 {
			return errors.New("This encounter has reached its input limit")
		}
		e.changed()
		e.appendAnswer("screen", urgentQuestion, urgentYes, "", emergency)
		e.routeInterview()
	case "review":
		if e.ScopeReason != "" {
			return errors.New("This encounter is outside the interview scope")
		}
		previousGaps := len(e.Outstanding)
		if e.Question != "" && !contains(e.Outstanding, e.Question) {
			e.Outstanding = append(e.Outstanding, e.Question)
			for _, id := range e.QuestionReviewIDs {
				if !contains(e.OutstandingReviewIDs, id) {
					e.OutstandingReviewIDs = append(e.OutstandingReviewIDs, id)
				}
			}
		}
		for _, topic := range e.remainingBackground() {
			if !contains(e.Outstanding, topic.question) {
				e.Outstanding = append(e.Outstanding, topic.question)
			}
		}
		if len(e.Outstanding) != previousGaps && e.ConfirmedRevision == e.HistoryRevision {
			e.changed()
		}
		localOpening := e.QuestionSource != nil && e.QuestionSource.Key == "symptom_story"
		extract := e.ExtractedRevision != e.HistoryRevision && (e.State == "interrupted_for_safety" || localOpening || e.Focused || e.ReturnSource != "")
		e.State = "history_review"
		e.Question = ""
		e.QuestionSource = nil
		e.QuestionReviewIDs = nil
		e.Choices = []string{}
		e.Pending = ""
		e.ReviewExtraction = extract
		if extract {
			e.Pending = "interview"
		}
		e.Failure = ""
		e.FailedTask = ""
	case "confirm":
		if e.State != "history_review" || e.Pending != "" {
			return errors.New("Open the history review before confirming it")
		}
		if e.ConfirmedRevision != e.HistoryRevision {
			s := Summary{
				Outstanding: append([]string{}, e.Outstanding...), Contradictions: append([]Contradiction{}, e.Contradictions...),
				Age: e.Age, PregnancyRelated: e.PregnancyRelated, HistoryRevision: e.HistoryRevision,
				ConfirmedAt: time.Now().UTC(), FactIDs: []string{}, AnswerIDs: []string{},
			}
			for i := range e.Facts {
				if e.Facts[i].Active {
					s.FactIDs = append(s.FactIDs, e.Facts[i].ID)
					e.Facts[i].Confirmed = true
				}
			}
			for _, a := range e.Answers {
				if a.Active && !a.SourceOutdated {
					s.AnswerIDs = append(s.AnswerIDs, a.ID)
				}
			}
			e.Summaries = append(e.Summaries, s)
		}
		e.ConfirmedRevision = e.HistoryRevision
	case "assess":
		if e.ConfirmedRevision != e.HistoryRevision || e.ScopeReason != "" || e.State != "history_review" && e.State != "needs_clarification" && e.State != "assessment_ready" {
			return errors.New("Review and confirm this history before requesting an estimate")
		}
		e.State = "assessing"
		e.Pending = "assessment"
		e.Failure = ""
		e.FailedTask = ""
	case "retry":
		if e.State != "failed_recoverably" || (e.FailedTask != "assessment" && e.FailedTask != "interview") {
			return errors.New("There is no failed request to retry")
		}
		e.Pending = e.FailedTask
		e.FailedTask = ""
		e.Failure = ""
		if e.Pending == "assessment" {
			e.State = "assessing"
		} else {
			e.State = "interviewing"
		}
	case "focus_interview":
		if e.Pending != "" || e.State != "interviewing" || e.Demo {
			return errors.New("Choose a shorter interview while answering a question")
		}
		if !e.Focused {
			e.Focused = true
			e.FocusedFrom = len(e.Answers)
			e.routeInterview()
		}
	case "continue":
		if e.focusedLimitReached() {
			e.Focused = false
		} // The user explicitly asked for more questions after the short interview.
		if e.ScopeReason != "" {
			return errors.New("This encounter is outside the interview scope")
		}
		e.routeInterview()
	case "change_provider":
		if !c.ConfirmedDestination || c.Provider.Kind == "" {
			return errors.New("Confirm the destination before changing this encounter's provider")
		}
		if e.Demo != (c.Provider.Kind == "demo") {
			return errors.New("Demo encounters must stay separate from real inference")
		}
		if e.Provider != c.Provider {
			e.ProviderChanges = append(e.ProviderChanges, ProviderChange{e.Provider, c.Provider, time.Now().UTC()})
			e.Provider = c.Provider
			for i := range e.ReportReviews {
				e.ReportReviews[i].ApprovedFor = nil
			}
		}
		// A user must explicitly retry or continue before the new destination receives history.
		e.Pending = ""
		e.Question = ""
		e.Choices = []string{}
		if e.ScopeReason != "" {
			e.State = "unsupported_scope"
		} else {
			e.State = "history_review"
		}
		e.Failure = ""
		e.FailedTask = ""
	default:
		return errors.New("Unknown encounter action")
	}
	e.touch()
	return nil
}

func (e *Encounter) applyAnswer(c Command, emergency string) error {
	if len(e.Answers) >= 256 {
		return errors.New("This encounter has reached its input limit; its history can still be reviewed and exported")
	}
	kind, question := "reply", e.Question
	source, deps := e.QuestionSource, e.QuestionReviewIDs
	date := ""
	text := strings.TrimSpace(c.Text)
	if question == "" {
		question = "What would you like to add?"
	}
	var old *Answer
	if c.Action == "correct_answer" {
		old = e.answer(c.AnswerID)
		if old == nil {
			return errors.New("Choose an active answer to correct")
		}
		if old.Kind == "witness" {
			return errors.New("Preserve the witness’s original words and add a separate clarification")
		}
		kind, question = old.Kind, old.Question
		source, deps = old.QuestionSource, old.ReviewIDs
		date = old.EventDate
		if kind == "screen" {
			if c.UrgentNow == nil {
				return errors.New("Select the corrected emergency screening response")
			}
			text = urgentNo
			if *c.UrgentNow {
				text = urgentYes
			}
		}
	}
	if c.Action == "answer" && source != nil && source.Key == "medication_start" && c.Date != "" {
		if !isoDay(c.Date) {
			return errors.New("Choose a valid start date")
		}
		date = c.Date
	}
	if !validText(text, 4000) {
		return errors.New("Enter a response of up to 4000 bytes")
	}
	e.changed()
	oldID := ""
	if old != nil {
		old.Active = false
		oldID = old.ID
		e.resolveSource(oldID)
		for i := range e.Facts {
			f := &e.Facts[i]
			if f.Active && f.AnswerID == oldID {
				f.Active = false
			}
		}
	}
	a := e.appendDatedAnswer(kind, question, text, oldID, date, emergency)
	if old != nil {
		e.answer(a.ID).Detail = old.Detail
	}
	if kind == "agenda" {
		e.Agenda = text
	}
	if old != nil && old.SourceOutdated {
		e.Answers[len(e.Answers)-1].SourceOutdated = true
	}
	e.Answers[len(e.Answers)-1].QuestionSource = source
	e.Answers[len(e.Answers)-1].ReviewIDs = append([]string(nil), deps...)
	if topic := backgroundFor(question); kind == "reply" && topic != nil && a.Disposition == "none" {
		e.recordFinding(FindingProposal{Evidence: Evidence{a.ID, a.Text}, Concept: topic.concept, Status: "explicitly_denied", Subject: "self"}, "patient_answer", "", false)
	}
	switch kind {
	case "concern":
		e.Concern = a.Text
	case "background":
		e.Background = a.Text
	}
	if kind != "screen" && c.UrgentNow != nil && *c.UrgentNow {
		e.appendAnswer("screen", urgentQuestion, urgentYes, "", emergency)
	}
	e.rebuildContradictions()
	e.routeInterview()
	if c.Action == "answer" && !e.Focused && source != nil && source.Key == "background" && a.Disposition != "provided" && e.Pending == "interview" {
		e.askNextBackground()
	}
	return nil
}

func (e *Encounter) correctFact(c Command, emergency string) error {
	if len(e.Answers) >= 256 {
		return errors.New("This encounter has reached its input limit; its history can still be reviewed and exported")
	}
	f := e.fact(c.FactID)
	if f == nil {
		return errors.New("Choose an active finding to correct")
	}
	if f.SourceType == "witness_observation" {
		return errors.New("Add a separate account to clarify a witness observation")
	}
	if f.SourceType == "report_entry" {
		return errors.New("Correct this transcription next to its original report row")
	}
	if !validText(c.Text, 4000) || !validText(c.Concept, 120) || !validStatus(c.Status) || !validSubject(c.Subject) || len(c.TimeContext) > 180 {
		return errors.New("Enter corrected wording, concept, status, subject and relevant timing")
	}
	if c.Status == "explicitly_denied" && !denies(c.Text) {
		return errors.New("Include your explicit denial in the corrected wording")
	}
	e.changed()
	f.Active = false
	prior := *f
	// A corrected finding supersedes this interpretation and its own source-specific
	// model flag. An explicit screening response can only be corrected separately.
	for i := range e.Warnings {
		w := &e.Warnings[i]
		if w.Active && w.FactID == prior.ID && w.Rule != "explicit_screen" {
			w.Active = false
			w.ResolvedRevision = e.HistoryRevision
		}
	}
	a := e.appendAnswer("correction", "How should this finding be recorded?", strings.TrimSpace(c.Text), "", emergency)
	corrected := FindingProposal{Evidence: Evidence{a.ID, a.Text}, Concept: strings.TrimSpace(c.Concept), Status: c.Status, Subject: c.Subject, TimeContext: strings.TrimSpace(c.TimeContext)}
	e.recordFinding(corrected, "patient_correction", prior.ID, true)
	e.Facts[len(e.Facts)-1].Origin = prior.Origin
	if previous := e.answer(prior.AnswerID); previous != nil {
		e.Answers[len(e.Answers)-1].ReviewIDs = append([]string(nil), previous.ReviewIDs...)
	}
	e.screenCorrection(a, corrected, emergency)
	e.rebuildContradictions()
	e.Question = ""
	e.Choices = []string{}
	e.Pending = ""
	e.FailedTask = ""
	e.Failure = ""
	if e.ScopeReason != "" {
		e.State = "unsupported_scope"
	} else if e.hasWarning() {
		e.State = "interrupted_for_safety"
	} else {
		e.State = "history_review"
	}
	return nil
}
