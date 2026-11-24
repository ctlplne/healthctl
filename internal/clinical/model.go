package clinical

import (
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ctlplne/healthctl/internal/contract"
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
	r, err := questionSource(in, p)
	if err != nil {
		return err
	}
	qsource := r.source
	if qsource != nil {
		if r.intent != "" {
			p.Question, p.Choices = r.question, r.choices
		}
		p.Reason = qsource.Reason
		if len(e.Pathways) > 0 && qsource.Pathway == "" {
			qsource.Pathway = e.Pathways[0].ID
		}
	}
	// The app owns the question budget. Past it, a proposed question becomes an
	// open item and the app moves on to its own questions or the review.
	overBudget := task == "interview" && p.Action == "ask" && e.modelQuestionsAsked() >= maxModelQuestions
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
			if at := strings.Index(a.Text, f.Quote); !past && isCurrentPersonalFinding(f) && urgentSignIn(f.Concept) {
				if c := analyzeSpan(a.Text, at, at+len(f.Quote), sourceOf(*a)); !c.blocked() {
					e.addWarning(f.Evidence, "structured_finding", levelFor(c, warningEmergency), emergency)
				}
			}
		}
		// A model can always raise concern with an exact quote. When the app's
		// own reading disagrees, the warning asks for a prompt check instead.
		for _, ev := range p.UrgentEvidence {
			level := warningCheck
			if a := e.answer(ev.AnswerID); a != nil {
				level = flagLevel(*a, ev.Quote)
			}
			e.addWarning(ev, "model_flag", level, emergency)
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
		case p.Action == "ask" && len(e.Answers) < 64 && !overBudget:
			e.Turns[len(e.Turns)-1].Asked = true
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
		case (p.Action == "review" || overBudget) && !e.Focused && e.ReturnSource == "" && len(e.Answers) < 64 && (e.askOpeningDetail(in) || e.askNextBackground() || e.askNextRedFlag() || e.askNextGap()):
		default:
			e.State = "history_review"
		}
		if overBudget && !contains(e.Outstanding, p.Question) {
			e.Outstanding = append(e.Outstanding, p.Question)
		}
	}
	e.touch()
	return nil
}

// flagLevel decides the level of a model urgent flag. The app keeps the flag
// either way. When its own reading of an urgent sign in the quote disagrees, or
// the quote is negated or about the past, the warning asks for a prompt check.
func flagLevel(a Answer, quote string) string {
	at := strings.Index(a.Text, quote)
	if at < 0 || pastEvent(a) {
		return warningCheck
	}
	end := at + len(quote)
	signs := false
	for _, m := range urgentSignMatches(a.Text) {
		if m.start < end && at < m.end {
			signs = true
		}
	}
	if signs {
		for _, s := range screenText(a.Text, sourceOf(a)) {
			if s.start < end && at < s.end {
				return s.level
			}
		}
		return warningCheck
	}
	c := analyzeSpan(a.Text, at, end, sourceOf(a))
	if c.blocked() || negatedInside(a.Text, at, end) {
		return warningCheck
	}
	return levelFor(c, warningEmergency)
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

var vagueOnly = regexp.MustCompile(`(?i)^(?:i(?: am|'m|’m| feel) |i am feeling )?(?:otherwise |generally |pretty )?(?:fine|good|ok|okay|well|normal|healthy)[.!]?$|^(?:nothing (?:else|much)|not really)[.!]?$`)
var uncertaintyOnly = regexp.MustCompile(`(?i)^\s*(?:i['’]m |i am |i )?(not sure|unsure|don't know|don’t know|do not know|can't remember|cannot remember|uncertain|maybe|rather not(?: say| answer)?|prefer not to (?:say|answer)|decline(?: to answer)?|skip)[\s.!?]*$`)

// alwaysBlocked never belongs in model text: certainty, reassurance, links,
// percentages and attempts to change instructions.
var alwaysBlocked = regexp.MustCompile(`(?i)([0-9]\s*%|https?://|www\.|\b(?:ruled out|definitively|definitely|confirmed diagnosis|no emergency|no need (?:for|to seek) (?:medical )?care|you (?:are safe|can safely)|you (?:have|are suffering from) (?:a |an )?(?:diagnosis|disease)|ignore (?:all |the )?(?:previous |prior )?instructions)\b)`)

// instruction catches treatment directions. A question that asks whether the
// user started or stopped something is not a direction, so questions are exempt.
var instruction = regexp.MustCompile(`(?i)\b(?:you should (?:take|start|stop|increase|decrease|double)|take [0-9]+|(?:start|stop|change) (?:your |taking |using ))\b`)

func statementSentences(s string) []string {
	var out []string
	for _, part := range regexp.MustCompile(`[^.!?\n]+[.!?\n]*`).FindAllString(s, -1) {
		if !strings.HasSuffix(strings.TrimSpace(part), "?") {
			out = append(out, part)
		}
	}
	return out
}

// adviceIn reports advice or certainty in s.
func adviceIn(s string) bool {
	if alwaysBlocked.MatchString(s) {
		return true
	}
	for _, sentence := range statementSentences(s) {
		if instruction.MatchString(sentence) {
			return true
		}
	}
	return false
}

func denies(s string) bool {
	return len(negationCues(s)) > 0 || pureDenial.MatchString(strings.TrimSpace(s))
}
func badOutput(s string, limit int) bool { return len(s) > limit || adviceIn(s) }
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
		return Answer{}, reasonErr(ReasonQuoteNotFound, "Finding or warning lacks original evidence")
	}
	for _, c := range in.CorrectedEvidence {
		if c.AnswerID == ev.AnswerID && (strings.Contains(c.Quote, ev.Quote) || strings.Contains(ev.Quote, c.Quote)) {
			return Answer{}, reasonErr(ReasonSuperseded, "The user superseded that interpretation")
		}
	}
	for _, a := range in.Answers {
		if a.ID == ev.AnswerID && a.Active && strings.Contains(a.Text, ev.Quote) {
			if strings.Count(a.Text, ev.Quote) != 1 {
				return Answer{}, reasonErr(ReasonQuoteAmbiguous, "Quote more surrounding text to identify the exact occurrence")
			}
			return a, nil
		}
	}
	return Answer{}, reasonErr(ReasonQuoteNotFound, "The evidence is not an exact span of a current answer")
}

// quoteContext reads the concept words inside the quote in context. A present
// finding is negated only when every mention of the concept is negated.
func quoteContext(a Answer, f FindingProposal) (spanContext, bool) {
	at := strings.Index(a.Text, f.Quote)
	end := at + len(f.Quote)
	src := sourceOf(a)
	targets := conceptTargets(a.Text, at, end, f.Concept)
	if len(targets) == 0 {
		c := analyzeSpan(a.Text, at, end, src)
		c.Negated = c.Negated || negatedInside(a.Text, at, end)
		return c, false
	}
	var out spanContext
	negated := true
	for i, t := range targets {
		c := analyzeSpan(a.Text, t.start, t.end, src)
		if i == 0 {
			out = c
		}
		negated = negated && c.Negated
		out.Uncertain = out.Uncertain || c.Uncertain
		out.Hypothetical = out.Hypothetical && c.Hypothetical
	}
	out.Negated = negated
	return out, true
}

func currentTime(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "now", "current", "currently", "present", "today", "right now":
		return true
	}
	return false
}

func validateFinding(in Input, f FindingProposal) error {
	a, err := verifySource(in, f.Evidence)
	if err != nil {
		return err
	}
	if !validText(f.Concept, contract.MaxConceptBytes) || badOutput(f.Concept, contract.MaxConceptBytes) || !validSubject(f.Subject) || !validStatus(f.Status) || len(f.TimeContext) > contract.MaxTimeBytes || len(f.Value) > contract.MaxValueBytes || len(f.Units) > contract.MaxUnitsBytes {
		return reasonErr(ReasonAttributes, "Invalid finding attributes")
	}
	disposition := evidenceDisposition(a)
	if disposition == "not_applicable" || disposition == "skipped" && f.Status != "declined" || disposition == "unknown" && f.Status != "uncertain" {
		return reasonErr(ReasonNonanswer, "This response cannot establish the proposed clinical finding")
	}
	if disposition == "none" {
		if topic := backgroundFor(a.Question); topic != nil && (f.Status != "explicitly_denied" || !topic.acceptsDenial(f.Concept)) {
			return reasonErr(ReasonBackgroundScope, "A background denial applies only to the question the user answered")
		}
	}
	if a.Kind == "screen" {
		return reasonErr(ReasonScreenAnswer, "The emergency screen cannot establish other clinical findings")
	}
	if badOutput(f.TimeContext, contract.MaxTimeBytes) || badOutput(f.Value, contract.MaxValueBytes) || badOutput(f.Units, contract.MaxUnitsBytes) {
		return reasonErr(ReasonAdvice, "Invalid normalized finding")
	}
	for _, n := range numericClaims(f.TimeContext) {
		if !slices.Contains(numbers.FindAllString(a.Text+" "+a.EventDate, -1), n) {
			return reasonErr(ReasonUngroundedTiming, "Precise timing must appear in the source answer")
		}
	}
	for _, value := range []string{f.Value, f.Units} {
		if value != "" && !strings.Contains(strings.ToLower(f.Quote), strings.ToLower(value)) {
			return reasonErr(ReasonValueNotQuoted, "A value or unit is missing from the evidence")
		}
	}
	c, named := quoteContext(a, f)
	if f.Subject == "self" && c.Other {
		return reasonErr(ReasonExperiencer, "Confirm who experienced the reported finding")
	}
	if f.Subject != "self" && !c.Other && (f.Status == "reported_present" || f.Status == "uncertain") {
		return reasonErr(ReasonExperiencer, "The quote describes the user, not someone else")
	}
	switch f.Status {
	case "not_asked":
		return reasonErr(ReasonNotAsked, "An unasked concept belongs in outstanding questions")
	case "explicitly_denied":
		if !denialSupports(f.Concept, f.Quote, a) {
			return reasonErr(ReasonDenialUnsupported, "Negative findings require explicit, unambiguous user evidence")
		}
	case "reported_present":
		switch {
		case uncertaintyOnly.MatchString(f.Quote) || vagueOnly.MatchString(strings.TrimSpace(f.Quote)):
			return reasonErr(ReasonVague, "Affirmative finding is unsupported by this answer")
		case c.Negated:
			return reasonErr(ReasonNegated, "Affirmative finding is unsupported by this answer")
		case c.Hypothetical:
			return reasonErr(ReasonHypothetical, "The user described this as a possibility, not an experience")
		case c.Uncertain:
			return reasonErr(ReasonUncertain, "The user expressed uncertainty; record the finding as uncertain")
		case f.Subject == "self" && c.History == historyRemote && currentTime(f.TimeContext) && !pastEvent(a):
			return reasonErr(ReasonHistoricalTime, "The user described this in the past; record its timing")
		}
		_ = named
	case "uncertain":
		if disposition != "unknown" && !c.Uncertain && !uncertaintyOnly.MatchString(f.Quote) && !uncertaintyCue.MatchString(f.Quote) {
			return reasonErr(ReasonUncertaintyUnsupported, "The user did not express uncertainty")
		}
		if c.Hypothetical && disposition != "unknown" {
			return reasonErr(ReasonHypothetical, "The user described this as a possibility, not an experience")
		}
	case "declined":
		if !declined.MatchString(f.Quote) {
			return reasonErr(ReasonDeclineUnsupported, "The user did not explicitly decline")
		}
	}
	return nil
}

// validateUrgent checks a model urgent flag. Raising concern needs only an
// exact quote of a substantive user statement. The app decides the warning level.
func validateUrgent(in Input, ev Evidence) error {
	a, err := verifySource(in, ev)
	if err != nil {
		return err
	}
	if a.Kind == "screen" {
		return reasonErr(ReasonScreenAnswer, "The emergency screen belongs to the application")
	}
	if evidenceDisposition(a) != "provided" {
		return reasonErr(ReasonNonanswer, "The urgent flag needs a substantive user statement")
	}
	return nil
}

func questionIssue(in Input, p Proposal, r resolved) error {
	if !validText(p.Question, contract.MaxQuestionBytes) || !validText(p.Reason, contract.MaxReasonBytes) {
		return reasonErr(ReasonQuestion, "A question must explain which uncertainty it addresses")
	}
	if strings.Count(p.Question, "?") > 1 {
		return reasonErr(ReasonMultipleQuestions, "Ask one focused question per turn")
	}
	if r.intent == "" {
		// A free-text question is a fallback with stricter checks.
		if err := freeTextIssue(p.Question); err != nil {
			return err
		}
		for _, c := range p.Choices {
			if !choiceAllowed(in, c) {
				return reasonErr(ReasonChoiceSymptom, "A choice names a symptom the user hasn't reported")
			}
		}
		if el := questionElement(p.Question); el != "" && answeredElement(in.Answers, el) {
			return reasonErr(ReasonAlreadyAnswered, "The app already asked about that")
		}
	}
	topic := backgroundFor(p.Question)
	for _, a := range in.Answers {
		if (a.Kind == "reply" && sameQuestion(a.Question, p.Question)) || (topic != nil && topic.addressedBy(a)) {
			return reasonErr(ReasonAlreadyAnswered, "The user already answered that question")
		}
	}
	return nil
}

// answeredElement reports whether one of the app's own questions about this
// element was already answered.
func answeredElement(answers []Answer, element string) bool {
	for _, a := range answers {
		if !a.Active || a.QuestionSource == nil {
			continue
		}
		k := a.QuestionSource.Key
		if strings.HasPrefix(k, "intent:") && intentCatalog[strings.TrimPrefix(k, "intent:")].element == element || k == "symptom_story" && questionElement(a.Question) == element {
			return true
		}
	}
	return false
}

// Validate is the strict gate before a proposal touches the aggregate. Every
// item must be valid. Sanitize drops invalid items first, so a valid remainder
// can still be accepted.
func Validate(in Input, p Proposal) error {
	r, err := questionSource(in, p)
	if err != nil {
		return err
	}
	if r.intent != "" {
		p.Question, p.Choices = r.question, r.choices
	}
	if !listValid(p.Outstanding, contract.MaxOutstanding, contract.MaxOutstandingBytes) || !listValid(p.Choices, contract.MaxChoices, contract.MaxChoiceBytes) || badOutput(p.Reason, contract.MaxReasonBytes) || badOutput(p.Question, contract.MaxQuestionBytes) || len(p.Findings) > contract.MaxFindings || len(p.UrgentEvidence) > contract.MaxUrgent || len(p.Items) > contract.MaxItems {
		return reasonErr(ReasonAdvice, "Model output exceeds the permitted content or size")
	}
	switch in.Task {
	case "interview":
		if p.Action != "ask" && p.Action != "review" && p.Action != "insufficient" || len(p.Items) > 0 {
			return reasonErr(ReasonInvalidAction, "Invalid interview action")
		}
		if p.Action == "ask" {
			if err := questionIssue(in, p, r); err != nil {
				return err
			}
		} else if p.Question != "" || len(p.Choices) > 0 {
			return reasonErr(ReasonQuestion, "Only an open question can include choices")
		}
		if p.Action == "insufficient" && len(p.Outstanding) == 0 {
			return reasonErr(ReasonMissingOutstanding, "Identify missing information")
		}
		for _, f := range p.Findings {
			if err := validateFinding(in, f); err != nil {
				return err
			}
		}
		for _, ev := range p.UrgentEvidence {
			if err := validateUrgent(in, ev); err != nil {
				return err
			}
		}
	case "assessment":
		if len(p.Findings) > 0 || len(p.UrgentEvidence) > 0 || p.Question != "" || len(p.Choices) > 0 {
			return reasonErr(ReasonAlteredHistory, "An assessment cannot alter the confirmed history")
		}
		if p.Action == "insufficient" {
			if len(p.Items) != 0 || len(p.Outstanding) == 0 {
				return reasonErr(ReasonMissingOutstanding, "An insufficient assessment must identify missing information")
			}
			return nil
		}
		if p.Action != "differential" {
			return reasonErr(ReasonInvalidAction, "Invalid differential assessment")
		}
		if len(p.Items) == 0 {
			return reasonErr(ReasonEmptyDifferential, "Invalid differential assessment")
		}
		facts := activeFacts(in)
		names := map[string]bool{}
		for _, item := range p.Items {
			if err := validateItem(item, facts); err != nil {
				return err
			}
			name := strings.ToLower(strings.TrimSpace(item.Name))
			if names[name] {
				return reasonErr(ReasonDuplicate, "Duplicate diagnostic item")
			}
			names[name] = true
		}
	default:
		return reasonErr(ReasonUnknownTask, "Unknown model task")
	}
	return nil
}

func activeFacts(in Input) map[string]Fact {
	facts := map[string]Fact{}
	for _, f := range in.Facts {
		if f.Active {
			facts[f.ID] = f
		}
	}
	return facts
}

func validateItem(item Item, facts map[string]Fact) error {
	if !validText(item.Name, contract.MaxItemNameBytes) || badOutput(item.Name, contract.MaxItemNameBytes) || badOutput(item.ClinicalTerm, contract.MaxItemNameBytes) || !validText(item.Explanation, contract.MaxExplanationBytes) || badOutput(item.Explanation, contract.MaxExplanationBytes) || len(item.Supporting) == 0 || len(item.Supporting) > contract.MaxReferences || len(item.Conflicting) > contract.MaxReferences || len(item.Limitations) == 0 || !listValid(item.Limitations, contract.MaxLimitations, contract.MaxLimitationBytes) {
		return reasonErr(ReasonItemContent, "Each possibility needs an explanation, evidence and limitations")
	}
	seen := map[string]bool{}
	for _, refs := range [][]string{item.Supporting, item.Conflicting} {
		for _, fid := range refs {
			f, ok := facts[fid]
			if !ok || seen[fid] || (f.Status != "reported_present" && f.Status != "explicitly_denied" && !(f.SourceType == "report_entry" && f.Status == "documented")) {
				return reasonErr(ReasonFactReference, "Assessments must reference distinct, active reported, denied or reviewed document findings")
			}
			seen[fid] = true
		}
	}
	if !groundedNumbers(item, facts) {
		return reasonErr(ReasonUngroundedNumber, "Numerical claims must be grounded in referenced findings")
	}
	return nil
}

var declined = regexp.MustCompile(`(?i)\b(declin(?:e|ed)|prefer not|rather not|skip|won['’]t answer|don['’]t want|do not want)\b`)
var binaryQuestion = regexp.MustCompile(`(?i)^(?:do|did|does|is|are|was|were|have|has|had|can|could|any)\b`)

// denialSupports reports whether quote explicitly denies concept. Lowering
// concern needs strong support: the negation must sit in the quote and apply to
// the concept, or the quote must be a bare "no" to a question about it.
func denialSupports(concept, quote string, a Answer) bool {
	at := strings.Index(a.Text, quote)
	if at < 0 || !denies(quote) {
		return false
	}
	end := at + len(quote)
	if uncertainAt(a.Text, at) || analyzeSpan(a.Text, at, end, sourceOf(a)).Hypothetical {
		return false
	}
	if a.Kind == "reply" && bareDenial.MatchString(quote) && (backgroundFor(a.Question) != nil || binaryQuestion.MatchString(strings.TrimSpace(a.Question))) {
		q := a.Question
		if len(conceptTargets(q, 0, len(q), concept)) > 0 {
			return true
		}
	}
	cues := negationCues(a.Text)
	for _, t := range conceptTargets(a.Text, at, end, concept) {
		if !analyzeSpan(a.Text, t.start, t.end, sourceOf(a)).Negated {
			continue
		}
		if t.end < end && postNegation.MatchString(a.Text[t.end:end]) {
			return true
		}
		for _, c := range cues {
			if c.start >= at && c.end <= t.start {
				return true
			}
		}
	}
	return false
}

var numbers = regexp.MustCompile(`(?:[0-9]+(?:[.][0-9]+)?)`)

// standaloneNumber skips digits that are part of a name, such as COVID-19,
// vitamin B12 or type 2 diabetes.
var standaloneNumber = regexp.MustCompile(`(?:^|[^\p{L}\p{N}.\-])([0-9]+(?:[.][0-9]+)?)`)
var nameBeforeNumber = regexp.MustCompile(`(?i)\b(?:type|stage|grade|class|phase|level|schedule|step|tier|omega|vitamin|covid|h\d?n)\s*$`)

func numericClaims(s string) []string {
	var out []string
	for _, m := range standaloneNumber.FindAllStringSubmatchIndex(s, -1) {
		start, end := m[2], m[3]
		if end < len(s) {
			if r := s[end]; r == '-' && end+1 < len(s) && isLetter(s[end+1]) || isLetter(r) {
				continue
			}
		}
		if nameBeforeNumber.MatchString(s[:start]) {
			continue
		}
		out = append(out, s[start:end])
	}
	return out
}

func isLetter(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }

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
		for _, n := range numericClaims(part) {
			if !values[n] {
				return false
			}
		}
	}
	return true
}
