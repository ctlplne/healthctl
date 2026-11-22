package clinical

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
)

var errContextBudget = errors.New("Evidence exceeds the model context budget. Review or export the saved history")

// Reject evidence that cannot be represented faithfully. Never truncate an original
// quote or silently discard a known finding to fit a provider's context.
func (e *Encounter) ModelInput(task string) (Input, error) {
	if task == "" || task != e.Pending || (task != "interview" && task != "assessment") {
		return Input{}, errors.New("This model request is no longer pending")
	}
	if e.ScopeReason != "" || task == "assessment" && e.ConfirmedRevision != e.HistoryRevision {
		return Input{}, errors.New("This history cannot be assessed in its current state")
	}
	in := e.inputHistory(task)
	data, _ := json.Marshal(in)
	if len(data) > 64000 {
		return Input{}, errContextBudget
	}
	return in, nil
}

func (e *Encounter) Accept(task string, p Proposal, meta ModelMeta, emergency string) error {
	in, err := e.ModelInput(task)
	if err != nil {
		return err
	}
	if err = Validate(in, p); err != nil {
		return err
	}
	// Keep source quotations, literal values and user-authored corrections intact.
	for i := range p.Findings {
		p.Findings[i].Concept = userReference(p.Findings[i].Concept)
	}
	p.Question = userReference(p.Question)
	p.Reason = userReference(p.Reason)
	p.Choices = userLabels(p.Choices)
	p.Outstanding = userLabels(p.Outstanding)
	for i := range p.Items {
		p.Items[i].Limitations = userLabels(p.Items[i].Limitations)
	}
	qsource, err := questionSource(in, p)
	if err != nil {
		return err
	}
	if qsource != nil {
		p.Reason = qsource.Reason
		if len(e.Pathways) > 0 {
			qsource.Pathway = e.Pathways[0].ID
		}
	}
	e.Pending = ""
	e.Failure = ""
	e.FailedTask = ""
	e.Turns = append(e.Turns, Turn{QuestionSource: qsource, Task: task, Question: p.Question, Reason: p.Reason, HistoryRevision: e.HistoryRevision, Meta: meta, CreatedAt: time.Now().UTC()})
	if task == "assessment" {
		var factIDs []string
		for _, f := range in.Facts {
			factIDs = append(factIDs, f.ID)
		}
		if e.Focused {
			for _, gap := range in.Outstanding {
				if !contains(p.Outstanding, gap) {
					p.Outstanding = append(p.Outstanding, gap)
				}
			}
		}
		e.Assessments = append(e.Assessments, Assessment{
			Pathways: in.Pathways, PolicyVersion: PolicyVersion, FactIDs: factIDs, ReviewIDs: inputReviews(in),
			Version: len(e.Assessments) + 1, HistoryRevision: e.HistoryRevision, CreatedAt: time.Now().UTC(),
			ReviewStatus: ReviewStatus, Outcome: p.Action, Provider: meta, Destination: e.Provider,
			Items: assessmentItems(in, p), Outstanding: nonNil(p.Outstanding),
		})
		e.Outstanding = nonNil(p.Outstanding)
		e.OutstandingReviewIDs = inputReviews(in)
		if p.Action == "insufficient" {
			e.State = "needs_clarification"
		} else {
			e.State = "assessment_ready"
		}
	} else {
		if e.ConfirmedRevision == e.HistoryRevision && e.revisesHistory(p) {
			e.changed()
		}
		e.ExtractedRevision = e.HistoryRevision
		for _, f := range p.Findings {
			a := e.answer(f.AnswerID)
			past := pastEvent(*a)
			if past && (f.TimeContext == "" || sameTime(f.TimeContext, "current")) {
				f.TimeContext = a.EventDate
			}
			sourceType := "patient_answer"
			if a.Kind == "witness" {
				sourceType = "witness_observation"
			}
			e.recordFinding(f, sourceType, "", false)
			source := a.Text
			if !past && isCurrentPersonalFinding(f) && emergencySigns.MatchString(f.Concept) && contextualRisk(clauseAt(source, strings.Index(source, f.Quote))) {
				e.addWarning(f.Evidence, "structured_finding", emergency)
			}
		}
		for _, ev := range p.UrgentEvidence {
			e.addWarning(ev, "model_flag", emergency)
		}
		e.rebuildContradictions()
		e.Question = ""
		e.Choices = []string{}
		e.Outstanding = nonNil(p.Outstanding)
		e.OutstandingReviewIDs = inputReviews(in)
		switch {
		case e.ReviewExtraction:
			if e.Focused {
				for _, topic := range e.remainingBackground() {
					if !contains(e.Outstanding, topic.question) {
						e.Outstanding = append(e.Outstanding, topic.question)
					}
				}
			}
			e.State = "history_review"
			e.ReviewExtraction = false
		case e.hasWarning():
			e.State = "interrupted_for_safety"
		case p.Action == "ask" && len(e.Answers) < 64:
			e.State = "interviewing"
			e.Question = p.Question
			e.QuestionReviewIDs = inputReviews(in)
			e.QuestionSource = qsource
			e.Choices = nonNil(p.Choices)
			// A model can bring a core question forward when it matters to this concern.
			if qsource != nil && qsource.ReviewID == "" {
				for _, topic := range e.remainingBackground() {
					if topic.matches(p.Question) {
						e.askBackground(topic)
						break
					}
				}
			}
		case p.Action == "review" && !e.Focused && e.ReturnSource == "" && len(e.Answers) < 64 && (e.askOpeningDetail(in) || e.askNextBackground()):
		default:
			e.State = "history_review"
		}
	}
	e.touch()
	return nil
}

// A new inference can change the interpreted history without a new user message.
// Preserve the confirmation when the model only restates identical records.
func (e *Encounter) revisesHistory(p Proposal) bool {
	if !slices.Equal(e.Outstanding, p.Outstanding) {
		return true
	}
	for _, f := range p.Findings {
		if !slices.ContainsFunc(e.Facts, func(old Fact) bool { return old.Active && old.FindingProposal == f }) {
			return true
		}
	}
	for _, ev := range p.UrgentEvidence {
		if !slices.ContainsFunc(e.Warnings, func(w Warning) bool { return w.Active && w.Evidence == ev }) {
			return true
		}
	}
	return false
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (e *Encounter) Fail(task string) {
	if e.Pending == "" || e.Pending != task {
		return
	}
	_, inputErr := e.ModelInput(task)
	e.State = "failed_recoverably"
	e.Failure = "The model could not complete this request. Your history is saved. Retry the request or review and export your incomplete history."
	if errors.Is(inputErr, errContextBudget) {
		e.Failure = errContextBudget.Error()
	}
	e.Pending = ""
	e.FailedTask = task
	e.touch()
}

func contains(items []string, s string) bool {
	for _, v := range items {
		if v == s {
			return true
		}
	}
	return false
}
func validStatus(s string) bool {
	switch s {
	case "reported_present", "explicitly_denied", "unknown", "not_asked", "uncertain", "declined":
		return true
	}
	return false
}
func validSubject(s string) bool { return s == "self" || s == "family" || s == "other" }

var pureDenial = regexp.MustCompile(`(?i)^(nope|no|none|not at all)[.!]?$|\b(hasn['’]t|haven['’]t|didn['’]t) (?:had|have|experienced|noticed)\b`)
var vagueOnly = regexp.MustCompile(`(?i)^(?:i(?: am|'m|’m| feel) |i am feeling )?(?:otherwise |generally |pretty )?(?:fine|good|ok|okay|well|normal|healthy)[.!]?$|^(?:nothing (?:else|much)|not really)[.!]?$`)
var uncertaintyOnly = regexp.MustCompile(`(?i)^\s*(?:i['’]m |i am |i )?(not sure|unsure|don't know|don’t know|do not know|can't remember|cannot remember|uncertain|maybe|rather not(?: say| answer)?|prefer not to (?:say|answer)|decline(?: to answer)?|skip)[\s.!?]*$`)
var adviceOrCertainty = regexp.MustCompile(`(?i)([0-9]\s*%|https?://|www\.|\b(?:ruled out|definitively|definitely|confirmed diagnosis|no emergency|no need (?:for|to seek) (?:medical )?care|you (?:are safe|should (?:take|start|stop|increase|decrease|double)|can safely)|take [0-9]+|(?:start|stop|change) (?:your |taking |using )|you (?:have|are suffering from) (?:a |an )?(?:diagnosis|disease)|ignore (?:all |the )?(?:previous |prior )?instructions)\b)`)

var explicitFaintDenial = regexp.MustCompile(`(?i)^(?:i )?(?:(?:have not|haven['’]t|have never) fainted|(?:did not|didn['’]t) faint)[.!]?$`)
var faintConcept = regexp.MustCompile(`(?i)^(?:syncope|faint(?:ing|ed)?)(?: \(syncope\)| \(fainting\))?$`)

func denies(s string) bool {
	return negative.MatchString(s) || pureDenial.MatchString(strings.TrimSpace(s)) || explicitFaintDenial.MatchString(strings.TrimSpace(s))
}
func badOutput(s string, limit int) bool { return len(s) > limit || adviceOrCertainty.MatchString(s) }
func listValid(items []string, maxN, maxLen int) bool {
	if len(items) > maxN {
		return false
	}
	for _, s := range items {
		if !validText(s, maxLen) || badOutput(s, maxLen) {
			return false
		}
	}
	return true
}

func verifySource(in Input, ev Evidence) (Answer, error) {
	if !validText(ev.Quote, 4000) || ev.AnswerID == "" {
		return Answer{}, errors.New("Finding or warning lacks original evidence")
	}
	for _, c := range in.CorrectedEvidence {
		if c.AnswerID == ev.AnswerID && (strings.Contains(c.Quote, ev.Quote) || strings.Contains(ev.Quote, c.Quote)) {
			return Answer{}, errors.New("The user superseded that interpretation")
		}
	}
	for _, a := range in.Answers {
		if a.ID == ev.AnswerID && a.Active && strings.Contains(a.Text, ev.Quote) {
			if strings.Count(a.Text, ev.Quote) != 1 {
				return Answer{}, errors.New("Quote more surrounding text to identify the exact occurrence")
			}
			return a, nil
		}
	}
	return Answer{}, errors.New("The evidence is not an exact span of a current answer")
}

func validateFinding(in Input, f FindingProposal) error {
	a, err := verifySource(in, f.Evidence)
	if err != nil {
		return err
	}
	if !validText(f.Concept, 120) || badOutput(f.Concept, 120) || !validSubject(f.Subject) || !validStatus(f.Status) || len(f.TimeContext) > 180 || len(f.Value) > 120 || len(f.Units) > 64 {
		return errors.New("Invalid finding attributes")
	}
	disposition := evidenceDisposition(a)
	if disposition == "not_applicable" || disposition == "skipped" && f.Status != "declined" || disposition == "unknown" && f.Status != "uncertain" {
		return errors.New("This response cannot establish the proposed clinical finding")
	}
	if disposition == "none" {
		if topic := backgroundFor(a.Question); topic != nil && (f.Status != "explicitly_denied" || !topic.acceptsDenial(f.Concept)) {
			return errors.New("A background denial applies only to the question the user answered")
		}
	}
	if a.Kind == "screen" {
		return errors.New("The emergency screen cannot establish other clinical findings")
	}
	if badOutput(f.TimeContext, 180) || badOutput(f.Value, 120) || badOutput(f.Units, 64) {
		return errors.New("Invalid normalized finding")
	}
	for _, n := range numbers.FindAllString(f.TimeContext, -1) {
		if !slices.Contains(numbers.FindAllString(a.Text+" "+a.EventDate, -1), n) {
			return errors.New("Precise timing must appear in the source answer")
		}
	}
	for _, value := range []string{f.Value, f.Units} {
		if value != "" && !strings.Contains(strings.ToLower(f.Quote), strings.ToLower(value)) {
			return errors.New("A value or unit is missing from the evidence")
		}
	}
	clause := clauseAt(a.Text, strings.Index(a.Text, f.Quote))
	if f.Subject == "self" && thirdParty.MatchString(clause) {
		return errors.New("Confirm who experienced the reported finding")
	}
	switch f.Status {
	case "not_asked":
		return errors.New("An unasked concept belongs in outstanding questions")
	case "explicitly_denied":
		if !denialSupports(f.Concept, f.Quote, a) {
			return errors.New("Negative findings require explicit, unambiguous user evidence")
		}
	case "reported_present":
		if denies(clause) && !uncertain.MatchString(clause) || uncertaintyOnly.MatchString(f.Quote) || vagueOnly.MatchString(strings.TrimSpace(f.Quote)) {
			return errors.New("Affirmative finding is unsupported by this answer")
		}
	case "declined":
		if !declined.MatchString(f.Quote) {
			return errors.New("The user did not explicitly decline")
		}
	}
	return nil
}

// Validate is called before persisting a provider result. It never touches the
// aggregate, so an optional bounded repair cannot partially apply a bad response.
func Validate(in Input, p Proposal) error {
	if _, err := questionSource(in, p); err != nil {
		return err
	}
	if !listValid(p.Outstanding, 16, 300) || !listValid(p.Choices, 8, 160) || badOutput(p.Reason, 700) || badOutput(p.Question, 500) || len(p.Findings) > 32 || len(p.UrgentEvidence) > 8 || len(p.Items) > 8 {
		return errors.New("Model output exceeds the permitted content or size")
	}
	switch in.Task {
	case "interview":
		if p.Action != "ask" && p.Action != "review" && p.Action != "insufficient" || len(p.Items) > 0 {
			return errors.New("Invalid interview action")
		}
		if p.Action == "ask" {
			if !validText(p.Question, 500) || !validText(p.Reason, 700) {
				return errors.New("A question must explain which uncertainty it addresses")
			}
			if strings.Count(p.Question, "?") > 1 {
				return errors.New("Ask one focused question per turn")
			}
			topic := backgroundFor(p.Question)
			for _, a := range in.Answers {
				if (a.Kind == "reply" && sameQuestion(a.Question, p.Question)) || (topic != nil && topic.addressedBy(a)) {
					return errors.New("The user already answered that question")
				}
			}
		} else if p.Question != "" || len(p.Choices) > 0 {
			return errors.New("Only an open question can include choices")
		}
		if p.Action == "insufficient" && len(p.Outstanding) == 0 {
			return errors.New("Identify missing information")
		}
		for _, f := range p.Findings {
			if err := validateFinding(in, f); err != nil {
				return err
			}
		}
		for _, ev := range p.UrgentEvidence {
			a, err := verifySource(in, ev)
			if err != nil {
				return err
			}
			if evidenceDisposition(a) != "provided" || a.Kind == "screen" || pastEvent(a) || !contextualRisk(clauseAt(a.Text, strings.Index(a.Text, ev.Quote))) {
				return errors.New("The urgent flag needs evidence of a current, relevant user report")
			}
		}
	case "assessment":
		if len(p.Findings) > 0 || len(p.UrgentEvidence) > 0 || p.Question != "" || len(p.Choices) > 0 {
			return errors.New("An assessment cannot alter the confirmed history")
		}
		if p.Action == "insufficient" {
			if len(p.Items) != 0 || len(p.Outstanding) == 0 {
				return errors.New("An insufficient assessment must identify missing information")
			}
			return nil
		}
		if p.Action != "differential" || len(p.Items) == 0 {
			return errors.New("Invalid differential assessment")
		}
		facts := map[string]Fact{}
		for _, f := range in.Facts {
			if f.Active {
				facts[f.ID] = f
			}
		}
		names := map[string]bool{}
		for _, item := range p.Items {
			if !validText(item.Name, 120) || badOutput(item.Name, 120) || badOutput(item.ClinicalTerm, 120) || !validText(item.Explanation, 800) || badOutput(item.Explanation, 800) || len(item.Supporting) == 0 || len(item.Supporting) > 12 || len(item.Conflicting) > 12 || len(item.Limitations) == 0 || !listValid(item.Limitations, 8, 300) {
				return errors.New("Each possibility needs an explanation, evidence and limitations")
			}
			name := strings.ToLower(strings.TrimSpace(item.Name))
			if names[name] {
				return errors.New("Duplicate diagnostic item")
			}
			names[name] = true
			seen := map[string]bool{}
			for _, refs := range [][]string{item.Supporting, item.Conflicting} {
				for _, fid := range refs {
					f, ok := facts[fid]
					if !ok || seen[fid] || (f.Status != "reported_present" && f.Status != "explicitly_denied" && !(f.SourceType == "report_entry" && f.Status == "documented")) {
						return errors.New("Assessments must reference distinct, active reported, denied or reviewed document findings")
					}
					seen[fid] = true
				}
			}
			if !groundedNumbers(item, facts) {
				return errors.New("Numerical claims must be grounded in referenced findings")
			}
		}
	default:
		return errors.New("Unknown model task")
	}
	return nil
}

var declined = regexp.MustCompile(`(?i)\b(declin(?:e|ed)|prefer not|rather not|skip|won['’]t answer|don['’]t want|do not want)\b`)
var binaryQuestion = regexp.MustCompile(`(?i)^(?:do|did|does|is|are|was|were|have|has|had|can|could)\b`)
var words = regexp.MustCompile(`[[:alnum:]]+`)

// A generic "no" can establish a focused symptom only through its originating
// question. A denial in one clause must not turn another symptom into a negative.
func denialSupports(concept, quote string, a Answer) bool {
	if !denies(quote) || uncertain.MatchString(quote) {
		return false
	}
	if faintConcept.MatchString(strings.TrimSpace(concept)) && explicitFaintDenial.MatchString(strings.TrimSpace(quote)) {
		at := strings.Index(a.Text, quote)
		return at >= 0 && explicitFaintDenial.MatchString(strings.TrimSpace(clauseAt(a.Text, at)))
	}
	source := strings.ToLower(quote)
	if pureDenial.MatchString(strings.TrimSpace(quote)) && len(strings.Fields(quote)) <= 4 {
		if a.Kind != "reply" || (backgroundFor(a.Question) == nil && !binaryQuestion.MatchString(strings.TrimSpace(a.Question))) {
			return false
		}
		source = strings.ToLower(a.Question)
	}
	for _, w := range words.FindAllString(strings.ToLower(concept), -1) {
		switch w {
		case "no", "denied", "reported", "current", "any", "symptom", "history", "of", "or", "and":
			continue
		}
		at := strings.Index(source, w)
		if at >= 0 && (source == strings.ToLower(a.Question) || denies(clauseAt(source, at))) {
			return true
		}
	}
	return false
}

var numbers = regexp.MustCompile(`(?:[0-9]+(?:[.][0-9]+)?)`)

// Numeric details often change the meaning of a condition. Reject a numerical
// claim in the explanatory text unless its exact number occurs in cited evidence.
func groundedNumbers(item Item, facts map[string]Fact) bool {
	values := map[string]bool{}
	for _, ids := range [][]string{item.Supporting, item.Conflicting} {
		for _, fid := range ids {
			f := facts[fid]
			numericSource := f.Quote
			if f.SourceType == "report_entry" {
				numericSource += " " + f.Value
			}
			for _, n := range numbers.FindAllString(numericSource, -1) {
				values[n] = true
			}
		}
	}
	for _, part := range append([]string{item.Explanation}, item.Limitations...) {
		for _, n := range numbers.FindAllString(part, -1) {
			if !values[n] {
				return false
			}
		}
	}
	return true
}
