package clinical

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/ctlplne/healthctl/internal/contract"
)

// Question intents let the application own the wording of a question. The model
// names an intent, and optionally what it is about, at the start of its reason:
// "[intent:onset]" or "[intent:also_have:fever]". The app writes the question,
// the answer choices and the "Why this question?" text. A free-text question is
// a fallback with stricter checks.
//
// The catalog, the red-flag questions and the completeness elements are
// development content that has not been clinically reviewed.

type questionIntent struct {
	question string
	reason   string
	choices  []string
	element  string // the completeness element this intent covers
	slot     string // "" (optional words the user used), "symptom" or "pathway"
}

var yesNoUnsure = []string{"Yes", "No", "Not sure"}

var intentCatalog = map[string]questionIntent{
	"onset":           {"When did {topic} start?", "When it began helps tell apart possible causes.", []string{"Today", "Yesterday", "2 to 7 days ago", "More than a week ago"}, "onset", ""},
	"course":          {"How has {topic} changed since it started?", "Whether it is getting better or worse can change what to do next.", []string{"Getting better", "About the same", "Getting worse", "It comes and goes"}, "course", ""},
	"severity":        {"How much is {topic} affecting what you can usually do?", "The effect on daily life helps describe how much it matters.", []string{"I can do my usual activities", "Some activities are harder", "I have stopped my usual activities"}, "severity", ""},
	"location":        {"Where exactly do you notice {topic}?", "The exact place can point toward different causes.", nil, "location", ""},
	"character":       {"How would you describe {topic} in your own words?", "Your own description often matters more than a label.", nil, "character", ""},
	"triggers":        {"Does anything bring {topic} on or make it worse?", "What brings it on helps separate possible causes.", nil, "triggers", ""},
	"relief":          {"Does anything make {topic} better?", "What helps can point toward a cause.", nil, "triggers", ""},
	"associated":      {"Have you noticed anything else along with {topic}?", "Other changes at the same time can change the picture.", nil, "associated", ""},
	"also_have":       {"Have you also noticed {symptom}?", "Whether this is also present can change what might explain your symptoms.", yesNoUnsure, "associated", "symptom"},
	"prior":           {"Have you had {topic} before?", "A first episode and a repeat episode are looked at differently.", []string{"No, this is new", "Yes, it's similar", "Yes, but this is different"}, "prior", ""},
	"tried":           {"What have you tried for {topic} so far?", "What you have tried, and whether it helped, is useful for your clinician.", nil, "tried", ""},
	"medicine_change": {"Have you started, stopped or changed any medicine recently?", "A recent medicine change can explain new symptoms.", yesNoUnsure, "medicines", ""},
	"exposure":        {"Have you been around anyone with similar symptoms, or travelled recently?", "Contacts and travel can change the likely causes.", yesNoUnsure, "exposure", ""},
	"concern":         {"What worries you most about {topic}?", "Your main worry helps your clinician focus the visit.", nil, "concern", ""},
	"red_flag":        {"", "The app asks this safety question for this kind of concern because a yes can need urgent care.", yesNoUnsure, "red_flags", "pathway"},
}

// IntentKeys lists the catalog in a stable order for the prompt.
func IntentKeys() []string {
	keys := make([]string, 0, len(intentCatalog))
	for k := range intentCatalog {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// IntentText is appended to the system prompt, generated from the catalog.
func IntentText() string {
	var b strings.Builder
	b.WriteString("Question intents: to ask, start the reason with one tag such as [intent:onset] or [intent:onset:the cough], then a short note. The application writes the question, the choices and the explanation, so the question field may be empty. Intents: ")
	for i, k := range IntentKeys() {
		if i > 0 {
			b.WriteString("; ")
		}
		it := intentCatalog[k]
		switch it.slot {
		case "symptom":
			fmt.Fprintf(&b, "%s (topic required: a symptom name from the application's vocabulary)", k)
		case "pathway":
			fmt.Fprintf(&b, "%s (topic required: an active pathway ID)", k)
		default:
			fmt.Fprintf(&b, "%s (%s)", k, strings.ReplaceAll(it.question, "{topic}", "…"))
		}
	}
	fmt.Fprintf(&b, ". A free-text question without a tag is a fallback: one question of up to %d bytes ending with a question mark, and choices that never name a symptom the user hasn't reported.", contract.MaxFreeQuestionBytes)
	return b.String()
}

var intentTag = regexp.MustCompile(`^\[intent:([a-z_]+)(?::([^\]\n]{1,60}))?\]\s*`)

// resolved is a proposed question after the app has applied its own wording.
type resolved struct {
	source   *QuestionSource
	question string
	choices  []string
	intent   string
}

func intentSource(in Input, reason string) (resolved, error) {
	m := intentTag.FindStringSubmatch(strings.TrimSpace(reason))
	if m == nil {
		return resolved{}, reasonErr(ReasonUnknownIntent, "Use one intent tag at the start of the question reason")
	}
	key, topic := m[1], strings.TrimSpace(m[2])
	it, ok := intentCatalog[key]
	if !ok {
		return resolved{}, reasonErr(ReasonUnknownIntent, "Use an intent from the application's catalog")
	}
	note := strings.TrimSpace(strings.TrimSpace(reason)[len(m[0]):])
	src := &QuestionSource{Key: "intent:" + key, Reason: it.reason, ModelNote: note}
	var q string
	switch it.slot {
	case "symptom":
		term, ok := lookupConcept(topic)
		if !ok {
			return resolved{}, reasonErr(ReasonUnknownIntent, "Name a symptom from the application's vocabulary")
		}
		src.Topic = term.Term
		q = strings.ReplaceAll(it.question, "{symptom}", strings.ToLower(term.Term))
	case "pathway":
		flags := dueRedFlags(in.Pathways, in.Answers)
		i := slices.IndexFunc(flags, func(f RedFlagQuestion) bool { return f.Pathway == topic })
		if i < 0 {
			return resolved{}, reasonErr(ReasonAlreadyAnswered, "No safety question is due for that pathway")
		}
		src.Key, src.Pathway = flags[i].Key, flags[i].Pathway
		src.Reason = redFlagReason(flags[i].Pathway)
		q = flags[i].Question
	default:
		src.Topic = groundedTopic(in, topic)
		subject := src.Topic
		if subject == "" {
			subject = "this"
		}
		q = strings.ReplaceAll(it.question, "{topic}", subject)
	}
	if askedIntent(in.Answers, src.Key, src.Topic) {
		return resolved{}, reasonErr(ReasonAlreadyAnswered, "The user already answered that question")
	}
	return resolved{source: src, question: q, choices: append([]string{}, it.choices...), intent: key}, nil
}

// groundedTopic keeps a topic only if the user used those words.
func groundedTopic(in Input, topic string) string {
	topic = strings.TrimSpace(topic)
	if topic == "" || adviceIn(topic) || len(topic) > 60 {
		return ""
	}
	core := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(topic), "the "), "your ")
	if !strings.Contains(strings.ToLower(userWords(in.Answers)), core) {
		return ""
	}
	return topic
}

func userWords(answers []Answer) string {
	var b strings.Builder
	for _, a := range answers {
		if a.Active && a.Kind != "screen" {
			b.WriteString(a.Text)
			b.WriteString(" \n ")
		}
	}
	return b.String()
}

// askedIntent reports whether this app question was already asked and answered.
func askedIntent(answers []Answer, key, topic string) bool {
	for _, a := range answers {
		if a.Active && a.QuestionSource != nil && a.QuestionSource.Key == key && a.QuestionSource.Topic == topic {
			return true
		}
	}
	return false
}

// Free-text questions are a fallback with stricter checks.
var compoundQuestion = regexp.MustCompile(`(?i)\band (when|how|what|where|why|which|who|did|do|does|has|have|can|will|would)\b`)

func freeTextIssue(q string) error {
	q = strings.TrimSpace(q)
	switch {
	case len(q) > contract.MaxFreeQuestionBytes:
		return reasonErr(ReasonTooLong, "Keep a free-text question short, or use an intent")
	case !strings.HasSuffix(q, "?"):
		return reasonErr(ReasonQuestion, "End a free-text question with a question mark")
	case compoundQuestion.MatchString(q):
		return reasonErr(ReasonMultipleQuestions, "Ask one focused question per turn")
	}
	return nil
}

// questionElement classifies a question by the completeness element it covers.
// It is deliberately narrow: it only recognizes plain forms of each question.
var questionElements = []struct {
	element string
	re      *regexp.Regexp
}{
	{"onset", regexp.MustCompile(`(?i)^(when did|when was|when were|how long (have|has|ago)|since when)|first notice`)},
	{"course", regexp.MustCompile(`(?i)\b(changed since|how has (it|this|the \w+) changed|getting (better|worse)|better or worse)\b`)},
	{"severity", regexp.MustCompile(`(?i)\b(affecting what you can usually do|usual activities|how (bad|severe)|how much (does|is) (it|this))\b`)},
	{"location", regexp.MustCompile(`(?i)^where\b|\bwhich (side|part|nails?)\b`)},
	{"character", regexp.MustCompile(`(?i)\b(describe|feel like|what (kind|type) of|looks? different)\b`)},
	{"triggers", regexp.MustCompile(`(?i)\b(bring (it|this) on|make (it|this) (better|worse)|makes? (it|this|your \w+) (better|worse)|trigger|when you notice it|what are you doing when)\b`)},
	{"associated", regexp.MustCompile(`(?i)\b(anything else|other symptoms|other changes|along with|what else)\b`)},
	{"prior", regexp.MustCompile(`(?i)\b(had (this|it|that) before|happened before|first time)\b`)},
}

// A question that names a medicine is about medicines, whatever else it asks.
var medicineWords = regexp.MustCompile(`(?i)\b(medicines?|medications?|supplements?|tablets?|pills?|capsules?|inhalers?|doses?|creams?|ointments?|drops|sprays?|patch(es)?|injections?|antibiotics?|prescri\w*|treatments?)\b`)

func questionElement(q string) string {
	if medicineWords.MatchString(q) {
		return "medicines"
	}
	for _, e := range questionElements {
		if e.re.MatchString(q) {
			return e.element
		}
	}
	return ""
}

// The user's own words can already cover an element.
var coveredByWords = map[string]*regexp.Regexp{
	"onset":     openingTiming,
	"course":    openingCourse,
	"severity":  regexp.MustCompile(`(?i)\b(mild|moderate|severe|terrible|unbearable|can't|cannot|unable|out of 10|usual activities|stopped|hard to|harder to)\b`),
	"location":  regexp.MustCompile(`(?i)\b(left|right|upper|lower|both sides|one side|middle|centre|center|behind|under|all over|thumbnail|thumb|several nails|both hands)\b`),
	"character": regexp.MustCompile(`(?i)\b(sharp|dull|throbbing|burning|stabbing|aching|cramp(?:ing|y)?|pressure|tight|spinning|lightheaded|light-headed|woozy|pounding|squeezing|dry|wet|productive|watery|loose)\b`),
	"triggers":  regexp.MustCompile(`(?i)\b(when I|after (eating|exercise|standing|walking|meals?|being)|worse (with|when|after|at)|better (with|when|after)|brings? it on|stand(ing)? up|lying down)\b`),
	"prior":     regexp.MustCompile(`(?i)\b(before|again|first time|never had|used to|previous(ly)?)\b`),
	"medicines": regexp.MustCompile(`(?i)\b(medicines?|medications?|tablets?|pills?|supplements?|inhaler|prescri(bed|ption))\b`),
}

// Completeness elements the app expects for each pathway.
var completeness = map[string][]string{
	"respiratory":    {"onset", "course", "severity"},
	"headache":       {"onset", "course", "character", "prior"},
	"abdominal":      {"onset", "location", "course", "severity"},
	"dizziness":      {"onset", "character", "triggers", "medicines"},
	"bowel_change":   {"onset", "course", "severity"},
	"eye_appearance": {"onset", "associated"},
	"nail_change":    {"onset", "location"},
}

var elementIntent = map[string]string{
	"onset": "onset", "course": "course", "severity": "severity", "location": "location", "character": "character",
	"triggers": "triggers", "associated": "associated", "prior": "prior", "medicines": "medicine_change",
}

var elementLabel = map[string]string{
	"onset": "when it started", "course": "how it has changed since it started", "severity": "how much it affects daily life",
	"location": "where exactly it is", "character": "what it feels like", "associated": "anything else noticed at the same time",
	"prior": "whether it has happened before", "triggers": "what brings it on or makes it worse", "medicines": "any recent medicine change",
}

// Gap is something the app expects for an active pathway that no question has covered.
type Gap struct {
	Pathway string `json:"pathway"`
	Element string `json:"element"`
	Note    string `json:"note"`
}

func (e *Encounter) covered(element string) bool {
	var words strings.Builder
	for _, a := range e.Answers {
		if !a.Active || a.SourceOutdated || a.Kind == "screen" {
			continue
		}
		if a.QuestionSource != nil && strings.HasPrefix(a.QuestionSource.Key, "intent:") && intentCatalog[strings.TrimPrefix(a.QuestionSource.Key, "intent:")].element == element {
			return true
		}
		if a.Kind == "reply" && questionElement(a.Question) == element {
			return true
		}
		if element == "medicines" && a.Kind == "reply" && backgroundTopics[0].matches(a.Question) {
			return true
		}
		if a.Kind != "safety_net" && (a.Kind != "reply" || evidenceDisposition(a) == "provided") {
			words.WriteString(a.Text)
			words.WriteString("\n")
		}
	}
	re := coveredByWords[element]
	return re != nil && re.MatchString(words.String())
}

// MissingElements lists what the app still expects for the active pathways.
func (e *Encounter) MissingElements() []Gap {
	var out []Gap
	seen := map[string]bool{}
	for _, p := range e.Pathways {
		for _, el := range completeness[p.ID] {
			if seen[el] || e.covered(el) {
				continue
			}
			seen[el] = true
			out = append(out, Gap{Pathway: p.ID, Element: el, Note: "Not yet asked: " + elementLabel[el] + "."})
		}
	}
	return out
}

// appInterview reports whether the app asks its own completeness and red-flag
// questions in the interview. Scripted demonstrations and a user's choice of a
// short interview, a return visit or a report-first visit keep their own flow;
// anything left is listed at review instead.
func (e *Encounter) appInterview() bool {
	return e.GuidedHistory && !e.Demo && !e.DemoPractice && !e.Focused && !e.ReportFirst && e.ReturnSource == "" && e.ScopeReason == ""
}

func (e *Encounter) askNextGap() bool {
	if !e.appInterview() {
		return false
	}
	missing := e.MissingElements()
	if len(missing) == 0 {
		return false
	}
	g := missing[0]
	key := elementIntent[g.Element]
	it := intentCatalog[key]
	e.State, e.Pending = "interviewing", ""
	e.Question = strings.ReplaceAll(it.question, "{topic}", "this")
	e.Choices = append([]string{}, it.choices...)
	e.QuestionSource = &QuestionSource{Key: "intent:" + key, Pathway: g.Pathway, Reason: it.reason + " The app asks this for every " + pathwayLabel(g.Pathway) + " concern."}
	e.QuestionReviewIDs = nil
	return true
}

func pathwayLabel(id string) string { return strings.ReplaceAll(id, "_", " ") }

// Red-flag questions for each pathway are asked by the app before review.
type redFlag struct{ key, question string }

var redFlags = map[string][]redFlag{
	"respiratory":    {{"rest", "Are you short of breath at rest, or too breathless to speak in full sentences?"}, {"blood", "Have you coughed up any blood?"}},
	"headache":       {{"sudden", "Did this headache come on suddenly and severely, reaching its worst within about a minute?"}, {"neck", "Do you have a fever with a stiff neck, or any new weakness, numbness or confusion?"}},
	"abdominal":      {{"severe", "Is the pain in your abdomen severe right now, or getting rapidly worse?"}, {"blood", "Have you vomited blood, or passed black or tarry stools?"}},
	"dizziness":      {{"neuro", "Have you fainted, or had new weakness on one side, slurred speech or double vision?"}},
	"bowel_change":   {{"blood", "Have you seen blood in your stool, or passed black or tarry stools?"}},
	"eye_appearance": {{"yellow", "Do the whites of your eyes or your skin look newly yellow?"}},
}

func redFlagReason(pathway string) string {
	return "The app asks this safety question for every " + pathwayLabel(pathway) + " concern because a yes can need urgent care."
}

// RedFlagQuestion is an app-owned safety question for an active pathway.
type RedFlagQuestion struct {
	Key      string `json:"key"`
	Pathway  string `json:"pathway"`
	Question string `json:"question"`
}

func dueRedFlags(pathways []Pathway, answers []Answer) []RedFlagQuestion {
	var out []RedFlagQuestion
	for _, p := range pathways {
		for _, rf := range redFlags[p.ID] {
			key := "red_flag:" + p.ID + ":" + rf.key
			if !askedIntent(answers, key, "") {
				out = append(out, RedFlagQuestion{Key: key, Pathway: p.ID, Question: rf.question})
			}
		}
	}
	return out
}

func (e *Encounter) askNextRedFlag() bool {
	if !e.appInterview() {
		return false
	}
	due := dueRedFlags(e.Pathways, e.Answers)
	if len(due) == 0 {
		return false
	}
	e.State, e.Pending = "interviewing", ""
	e.Question = due[0].Question
	e.Choices = append([]string{}, yesNoUnsure...)
	e.QuestionSource = &QuestionSource{Key: due[0].Key, Pathway: due[0].Pathway, Reason: redFlagReason(due[0].Pathway)}
	e.QuestionReviewIDs = nil
	return true
}

// appQuestion reports whether the app wrote this question itself for
// completeness or safety, so the next app question can follow without a model call.
func appQuestion(s *QuestionSource) bool {
	return s != nil && (strings.HasPrefix(s.Key, "red_flag:") || strings.HasPrefix(s.Key, "intent:") && s.Pathway != "" && s.ModelNote == "")
}

// recordRedFlag raises a warning for a yes. The app wrote the question, so a
// yes is enough; the model is not asked to interpret it.
func (e *Encounter) recordRedFlag(a Answer, emergency string) {
	if a.QuestionSource == nil || !strings.HasPrefix(a.QuestionSource.Key, "red_flag:") {
		return
	}
	if affirmative.MatchString(a.Text) && !negationCue.MatchString(a.Text) {
		e.addWarning(Evidence{a.ID, strings.TrimSpace(a.Text)}, "red_flag", warningEmergency, emergency)
	}
}

// answerRedFlag records an answer given on the review screen.
func (e *Encounter) answerRedFlag(c Command, emergency string) error {
	due := dueRedFlags(e.Pathways, e.Answers)
	i := slices.IndexFunc(due, func(f RedFlagQuestion) bool { return f.Key == c.Category })
	if i < 0 {
		return errors.New("Choose a safety question that still needs an answer")
	}
	text := map[string]string{"yes": "Yes", "no": "No", "unsure": "Not sure"}[c.Status]
	if text == "" {
		return errors.New("Answer yes, no or not sure")
	}
	if len(e.Answers) >= 256 {
		return errors.New("This encounter has reached its input limit")
	}
	e.changed()
	e.appendAnswer("reply", due[i].Question, text, "", emergency)
	stored := &e.Answers[len(e.Answers)-1]
	stored.QuestionSource = &QuestionSource{Key: due[i].Key, Pathway: due[i].Pathway, Reason: redFlagReason(due[i].Pathway)}
	e.recordRedFlag(*stored, emergency)
	if e.ScopeReason == "" && e.hasWarning() {
		e.State, e.Question, e.Choices, e.QuestionSource, e.Pending = "interrupted_for_safety", "", []string{}, nil, ""
	}
	return nil
}

// refreshCompleteness keeps the app's own view of what is still missing.
func (e *Encounter) refreshCompleteness() {
	e.RedFlagsDue = nil
	e.Gaps = nil
	if e.ScopeReason != "" {
		return
	}
	e.RedFlagsDue = dueRedFlags(e.Pathways, e.Answers)
	e.Gaps = e.MissingElements()
	for _, a := range e.Answers {
		if a.Active && a.QuestionSource != nil && strings.HasPrefix(a.QuestionSource.Key, "red_flag:") && responseDisposition(a.Text, a.Question) == "unknown" {
			e.Gaps = append(e.Gaps, Gap{Pathway: a.QuestionSource.Pathway, Element: "red_flags", Note: "Not sure about a safety question: " + a.Question})
		}
	}
}

// maxModelQuestions bounds how many questions the model may ask in one
// encounter. After that the app moves on and keeps the question as an open item.
const maxModelQuestions = 12

func (e *Encounter) modelQuestionsAsked() int {
	n := 0
	for _, t := range e.Turns {
		if t.Task == "interview" && t.Asked {
			n++
		}
	}
	return n
}

// choiceAllowed rejects a choice that names a symptom or urgent sign the user
// hasn't reported. A choice must never put words in the user's mouth.
func choiceAllowed(in Input, choice string) bool {
	said := strings.ToLower(userWords(in.Answers))
	c := strings.ToLower(choice)
	for _, t := range vocabulary {
		for _, name := range t.names() {
			if wholeWord(c, name) && !t.mentionedIn(said) {
				return false
			}
		}
	}
	for _, m := range urgentSignMatches(choice) {
		if !strings.Contains(said, strings.ToLower(choice[m.start:m.end])) {
			return false
		}
	}
	return true
}
