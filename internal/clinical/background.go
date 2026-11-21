package clinical

import (
	"regexp"
	"slices"
	"strings"
)

type backgroundTopic struct {
	question, reason, entryLabel, hint, none, concept string
	aliases, denials                                  []string
}

var backgroundTopics = []backgroundTopic{
	{
		question:   "What medicines or supplements do you take?",
		reason:     "What you take can change how we understand your symptoms.",
		entryLabel: "Add a medicine or supplement", hint: "A name or what you take it for is enough to start.",
		none: "I take no medicines or supplements.", concept: "Current medicine or supplement use",
		aliases: []string{"What medicines and supplements do you take?", "What medications and supplements do you take?", "Are you taking any medicines or supplements?"},
		denials: []string{"medication use", "current medications", "medicines", "medications", "supplement use", "supplements"},
	},
	{
		question:   "What allergies do you know you have?",
		reason:     "Knowing what has caused a reaction can help explain a new symptom.",
		entryLabel: "Add an allergy", hint: "Start with what caused the reaction. Use the name you know.",
		none: "No known allergies.", concept: "Known allergies",
		aliases: []string{"Do you have any known allergies?", "Do you have any allergies?", "What allergies do you have?"},
		denials: []string{"allergy", "allergies", "known allergies"},
	},
	{
		question:   "What ongoing health conditions do you have?",
		reason:     "An existing condition can change which follow-up questions matter.",
		entryLabel: "Add a condition", hint: "A name or short description is enough.",
		none: "I have no known ongoing health conditions.", concept: "Known ongoing health conditions",
		aliases: []string{"Do you have any ongoing health conditions?", "What chronic conditions do you have?"},
		denials: []string{"ongoing health conditions", "chronic conditions", "known ongoing health conditions", "chronic medical conditions"},
	},
	{
		question:   "What operations have you had in the past?",
		reason:     "An earlier operation may help explain a change you are noticing now.",
		entryLabel: "Add an operation", hint: "Use your own words for the operation. An approximate date is fine if you remember.",
		none: "I have had no operations.", concept: "Past operations",
		aliases: []string{"Have you had any operations?", "Have you had any surgeries?", "What surgeries have you had?"},
		denials: []string{"operations", "previous operations", "past operations", "surgery", "surgical history", "previous surgery"},
	},
}

func sameQuestion(a, b string) bool {
	clean := func(s string) string { return strings.TrimRight(strings.Join(strings.Fields(s), " "), "?.!") }
	return strings.EqualFold(clean(a), clean(b))
}

func backgroundFor(question string) *backgroundTopic {
	for i := range backgroundTopics {
		if backgroundTopics[i].matches(question) {
			return &backgroundTopics[i]
		}
	}
	return nil
}

func (t backgroundTopic) matches(question string) bool {
	return sameQuestion(t.question, question) || slices.ContainsFunc(t.aliases, func(s string) bool { return sameQuestion(s, question) })
}

func (t backgroundTopic) addressedBy(a Answer) bool {
	if a.Kind == "reply" && t.matches(a.Question) {
		return true
	}
	// Recognize a complete statement volunteered in the patient's opening.
	// A partial medication list or a family member's history still needs clarification.
	if a.Kind != "concern" && a.Kind != "background" || thirdParty.MatchString(a.Text) {
		return false
	}
	for _, sentence := range strings.FieldsFunc(a.Text, func(r rune) bool { return r == '.' || r == '\n' }) {
		if strings.EqualFold(strings.TrimSpace(sentence), strings.TrimSuffix(t.none, ".")) {
			return true
		}
	}
	return false
}

func (t backgroundTopic) acceptsDenial(concept string) bool {
	return strings.EqualFold(t.concept, strings.TrimSpace(concept)) || slices.Contains(t.denials, strings.ToLower(strings.TrimSpace(concept)))
}

func (e *Encounter) remainingBackground() []backgroundTopic {
	if !e.GuidedHistory {
		return nil
	}
	var remaining []backgroundTopic
	allowed := e.disclosed()
	for _, topic := range backgroundTopics {
		addressed := slices.ContainsFunc(e.Answers, func(a Answer) bool {
			return a.Active && !a.SourceOutdated && allApproved(a.ReviewIDs, allowed) &&
				topic.addressedBy(a)
		})
		if !addressed && e.ReturnSource != "" {
			addressed = slices.ContainsFunc(e.Facts, func(f Fact) bool {
				return f.Active && !f.SourceOutdated && f.Status == "explicitly_denied" && f.Origin != nil && f.Origin.ReconciliationStatus == "still_applies" && topic.acceptsDenial(f.Concept)
			})
		}
		if !addressed {
			remaining = append(remaining, topic)
		}
	}
	return remaining
}

func (e *Encounter) askBackground(t backgroundTopic) {
	e.State, e.Pending = "interviewing", ""
	e.Question = t.question
	e.Choices = []string{t.none}
	e.QuestionSource = &QuestionSource{Key: "background", Reason: t.reason, EntryLabel: t.entryLabel, AnswerHint: t.hint}
}

// If the provider offers routine review on a terse symptom alone, elicit one
// concrete detail first. Report-first visits and patient-requested review have
// their own flows. Any prior symptom question, including a skip, counts.
var openingTiming = regexp.MustCompile(`(?i)\b(today|yesterday|tonight|since|ago|morning|afternoon|evening|last|minutes?|hours?|days?|weeks?|months?|years?|monday|tuesday|wednesday|thursday|friday|saturday|sunday)\b`)

var openingCourse = regexp.MustCompile(`(?i)\b(better|worse|worsen(?:ed|ing)?|improv(?:ed|ing)|unchanged|same|comes and goes)\b`)
var openingImpact = regexp.MustCompile(`(?i)\b(can't|cannot|can't do|hard to|harder to|unable to|stopped|usual activities|limits my|affects my)\b`)

// The local scope and urgency checks have already run. The first answer and
// opening reach the plan together, or review requests extraction immediately.
func (e *Encounter) askPlanOpening() {
	e.setStoryQuestion()
	if !openingTiming.MatchString(e.Concern) {
		return
	}
	if !openingCourse.MatchString(e.Concern) {
		e.Question = "How has this changed since you first noticed it?"
		e.Choices = []string{"Getting better", "About the same", "Getting worse", "It comes and goes"}
		e.QuestionSource.Reason = "The way this is changing helps guide the next question."
	} else if openingImpact.MatchString(e.Concern) {
		e.Question = "What concerns you most about this change?"
		e.Choices = []string{"How it feels", "How long it has lasted", "How it affects my day"}
		e.QuestionSource.Reason = "Your main concern helps focus the interview."
	}
}

func (e *Encounter) askOpeningDetail(in Input) bool {
	if !e.GuidedHistory || e.ReportFirst || len(in.Pathways) == 0 || len(in.ReportEntries) > 0 || len(strings.Fields(in.Concern)) > 12 {
		return false
	}
	for _, a := range in.Answers {
		if a.Kind != "screen" && a.Kind != "concern" && !(a.Kind == "reply" && backgroundFor(a.Question) != nil) {
			return false
		}
	}
	e.setStoryQuestion()
	return true
}

func (e *Encounter) setStoryQuestion() {
	q := QuestionSource{Key: "symptom_story", Reason: "When this first changed helps establish the course of your symptoms."}
	e.Question = "When did you first notice this change?"
	e.Choices = []string{"Today", "Yesterday", "2 to 7 days ago", "More than a week ago"}
	if openingTiming.MatchString(e.Concern) {
		q.Reason = "How this affects your usual activities helps describe its impact."
		e.Question = "How is this affecting what you can usually do?"
		e.Choices = []string{"I can do my usual activities", "Some activities are harder", "I have stopped my usual activities"}
	}
	e.QuestionSource = &q
	e.QuestionReviewIDs = nil
	e.State, e.Pending = "interviewing", ""
}

func (e *Encounter) askNextBackground() bool {
	topics := e.remainingBackground()
	if len(topics) == 0 {
		return false
	}
	e.QuestionReviewIDs = nil
	e.askBackground(topics[0])
	return true
}

// Classify only whole responses. An uncertain phrase alongside a symptom or
// medication name must keep its original detail and still go through screening.
func responseDisposition(text, question string) string {
	value := strings.ToLower(strings.TrimRight(strings.TrimSpace(text), ".!?"))
	switch value {
	case "skip", "skip this question", "skip for now", "prefer not to answer", "i prefer not to answer", "i'd rather not answer", "i’d rather not answer":
		return "skipped"
	case "not applicable", "not applicable to me", "not needed", "does not apply to me", "doesn't apply to me", "doesn’t apply to me", "doesn't apply", "doesn’t apply", "does not apply", "n/a", "na":
		return "not_applicable"
	case "unknown", "not sure", "i'm not sure", "i’m not sure", "i am not sure", "unsure", "i don't know", "i don’t know", "i do not know", "i can't remember", "i cannot remember":
		return "unknown"
	case "none", "no", "nope":
		return "none"
	}
	if t := backgroundFor(question); t != nil && strings.EqualFold(value, strings.TrimSuffix(t.none, ".")) {
		return "none"
	}
	return "provided"
}

func evidenceDisposition(a Answer) string {
	if a.Disposition != "" {
		return a.Disposition
	}
	if a.Kind != "reply" {
		return "provided"
	}
	return responseDisposition(a.Text, a.Question)
}

// Only the fixed, non-substantive background choices fit the authored demo.
// Any new patient detail needs a real provider to interpret it.
func fixtureBackground(a Answer) bool {
	t := backgroundFor(a.Question)
	if a.Kind != "reply" || t == nil {
		return false
	}
	switch evidenceDisposition(a) {
	case "unknown", "skipped", "not_applicable":
		return true
	case "none":
		return a.Text == t.none
	}
	return false
}
