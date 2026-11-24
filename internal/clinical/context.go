package clinical

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The context engine decides how a span of user text should be read: whether
// it is negated, uncertain, hypothetical, in the past, or about someone other
// than the user. The safety screen, the finding validator, pathway matching and
// contradiction checks all use it, so they agree with each other.
//
// These are English heuristics. They are development content that has not been
// clinically reviewed. See POLICY.md.

type textSource int

const (
	fromUser    textSource = iota // the user's own words, or a document about the user
	fromWitness                   // someone describing the user
)

func sourceOf(a Answer) textSource {
	if a.Kind == "witness" {
		return fromWitness
	}
	return fromUser
}

const (
	historyNone = iota
	historyRecent
	historyRemote
)

type spanContext struct {
	Negated, Uncertain, Hypothetical, Other bool
	History                                 int
}

type token struct {
	start, end int
	word       string
}

var wordPattern = regexp.MustCompile(`[\p{L}\p{N}]+(?:['’][\p{L}]+)*`)

func tokenize(text string) []token {
	idx := wordPattern.FindAllStringIndex(text, -1)
	out := make([]token, len(idx))
	for i, m := range idx {
		out[i] = token{m[0], m[1], strings.ReplaceAll(strings.ToLower(text[m[0]:m[1]]), "’", "'")}
	}
	return out
}

// Clause and sentence boundaries.
var sentenceBreak = regexp.MustCompile(`[;!?\n]|\.(?:\s|$)`)
var clauseBreak = regexp.MustCompile(`(?i)\b(?:but|however|although|though|whereas|yet|except|apart from|aside from|other than|even though|because)\b|,\s*(?:but\s+|then\s+)?(?:now\s+|then\s+|today\s+|tonight\s+)?(?:i|i['’]m|i['’]ve|i['’]d|i['’]ll|me|my|it|it['’]s|this|that|she|she['’]s|he|he['’]s|they|they['’]re|we|now|then|today|tonight|yesterday|currently|but)\b|,\s*and\s+(?:now\s+|then\s+|today\s+|tonight\s+)?(?:i|i['’]m|i['’]ve|i['’]d|my|it['’]s|it is|now|today|tonight|then)\b|\band\s+(?:now\s+|then\s+|today\s+|tonight\s+)?(?:i|i['’]m|i['’]ve|i['’]d|my|it['’]s|it is)\b|\band\s+(?:now|today|tonight|then)\b`)

func breakPositions(text string) []int {
	var out []int
	for _, m := range sentenceBreak.FindAllStringIndex(text, -1) {
		out = append(out, m[1])
	}
	for _, m := range clauseBreak.FindAllStringIndex(text, -1) {
		out = append(out, m[0])
	}
	return out
}

func rangeAround(text string, at int, positions []int) (int, int) {
	start, end := 0, len(text)
	for _, p := range positions {
		if p <= at && p > start {
			start = p
		}
		if p > at && p < end {
			end = p
		}
	}
	return start, end
}

func clauseRange(text string, at int) (int, int) { return rangeAround(text, at, breakPositions(text)) }

func sentenceRange(text string, at int) (int, int) {
	var positions []int
	for _, m := range sentenceBreak.FindAllStringIndex(text, -1) {
		positions = append(positions, m[1])
	}
	return rangeAround(text, at, positions)
}

// clauseAt returns the clause of text containing byte offset at.
func clauseAt(text string, at int) string {
	if at < 0 {
		at = 0
	}
	s, e := clauseRange(text, at)
	return text[s:e]
}

// Negation. Words such as "can't" and "won't" usually describe a symptom
// ("can't breathe", "won't stop bleeding"), so they are not negation cues.
var negationCue = regexp.MustCompile(`(?i)\b(?:no longer|not any ?more|negative for|free of|absence of|no signs? of|no evidence of|(?:do|does|did|have|has|had|is|are|was|were) not|(?:don|doesn|didn|haven|hasn|hadn|isn|aren|wasn|weren|ain)['’]t|no|not|never|without|none|neither|nor|denies|denied|deny|denying)\b`)

// Phrases that contain a negation word but don't negate the symptom.
var pseudoNegation = regexp.MustCompile(`(?i)\b(?:not only|not just|no change|no better|no improvement|no relief|no matter|no one|nobody|no idea|nothing helps|no reason|not that i know|not (?:too |really |entirely |completely )?(?:sure|certain)|(?:don['’]t|do not) (?:know|remember)|not (?:usually|normally))\b|\b(?:not|isn['’]t|is not|aren['’]t|wasn['’]t)\s+(?:getting|going|improving|helping|stopping|working|easing|settling|better|coming)\b|\b(?:doesn['’]t|does not|don['’]t|didn['’]t|won['’]t|hasn['’]t|haven['’]t|isn['’]t|aren['’]t)\s+(?:go(?:ne)? away|help|helped|helping|stop(?:ped)?|ease|eased|settle|settled|improve|improved|work|worked|working|matter|answering|home|like)\b|\bnever\b[^.;!?]{0,40}\b(?:before|like this|like that|this bad|so bad|this severe)\b|\b(?:not|n['’]t|never)\b[^.;!?]{0,40}\buntil\b|\b(?:can['’]t|cannot|can not|couldn['’]t|unable to)\b[^.;!?]{0,30}\bwithout\b|\bwithout\s+(?:trying|meaning to|warning|any warning|reason|any reason|explanation|cause|knowing|realizing|realising)\b|\bno\s+(?:appetite|energy|strength|sleep|motivation|interest|sense of (?:smell|taste)|taste|smell)\b|\b(?:not|n['’]t)\s+(?:\w+\s+){0,2}(?:well|properly|normally|myself|great|good)\b|\bnot\s+(?:feeling|doing|looking|sleeping|eating|breathing)\s+right\b`)

var postNegation = regexp.MustCompile(`(?i)^\s*(?:[:?–—-]\s*(?:no|none|nope|negative|denied|absent)\b|(?:is |was |are |were )?(?:absent|not present|negative|denied)\b)`)

var scopeTerminator = map[string]bool{"but": true, "however": true, "although": true, "though": true, "yet": true, "except": true, "whereas": true, "which": true, "who": true, "because": true, "since": true, "unless": true, "while": true, "so": true, "then": true}
var listJoiner = map[string]bool{"or": true, "nor": true}
var clauseVerb = map[string]bool{"is": true, "are": true, "was": true, "were": true, "has": true, "have": true, "had": true, "started": true, "began": true, "feel": true, "feels": true, "felt": true, "get": true, "gets": true, "got": true, "keep": true, "keeps": true, "now": true, "today": true, "since": true, "came": true, "i": true, "i'm": true, "i've": true, "my": true, "it's": true}

// Uncertainty, hypotheticals, history and experiencer.
var uncertaintyCue = regexp.MustCompile(`(?i)\b(?:not (?:too |really |entirely |completely )?(?:sure|certain)|unsure|uncertain|maybe|might|possibly|possible|perhaps|may (?:be|have)|can['’]t tell|cannot tell|hard to (?:say|tell)|can['’]t remember|cannot remember|(?:don['’]t|do not) remember|(?:don['’]t|do not) know (?:if|whether)|wonder(?:ing)? (?:if|whether))\b`)
var uncertainWithIf = regexp.MustCompile(`(?i)\b(?:not (?:too |really |entirely |completely )?(?:sure|certain)|unsure|can['’]t tell|cannot tell|(?:don['’]t|do not) know|wonder(?:ing)?)\s+(?:if|whether)\b`)
var hypotheticalCue = regexp.MustCompile(`(?i)\b(?:what if|just in case|in case|in the event(?: of| that)?|if|hypothetically|for example|for instance|supposing|suppose|imagine|warning signs?|signs? of|symptoms? of|watch (?:out )?for|look out for|risk of|chance of|(?:worried|afraid|scared|anxious|concerned|frightened|nervous|terrified) (?:about|of|that)|(?:worried|afraid|scared|anxious|concerned|frightened|nervous|terrified) (?:i|it) (?:might|may|could|will)|(?:worried|afraid|scared|anxious|concerned) i['’]ll|(?:worry|fear) (?:about|of|that)|could (?:turn into|become|lead to|cause|mean)|might (?:get|turn into|become|develop)|could (?:get|develop)|(?:read|heard|learned|saw) (?:that|about|online))\b`)

// "Could this be a stroke?" asks about now, so only an indefinite subject is hypothetical.
// "What if this is a heart attack?" is a worry about now, not a hypothetical.
var presentIf = regexp.MustCompile(`(?i)\b(?:what if|if) (?:this|it|that)(?:['’]s| is| was| could be| might be| may be)\b`)
var modalQuestion = regexp.MustCompile(`(?i)^\s*(?:(?:could|would|can|will|might|should) (?:a|an)\b|is it (?:normal|common|possible|bad|serious|dangerous)\b|are there\b|what are\b|how (?:do|would|can|could) (?:i|you) know\b)`)
var remoteHistory = regexp.MustCompile(`(?i)\b(?:(?:\d+|a|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|fifteen|twenty|thirty|forty|fifty|several|many|a few|few|some) years? ago|years ago|decades ago|last year|a long time ago|long ago|as a (?:child|kid|baby|toddler|teen|teenager|student|young (?:man|woman|adult))|in (?:my )?childhood|since childhood|when i was (?:a |an )?(?:child|kid|baby|toddler|teen|teenager|young|younger|little|small|pregnant|student|\w+teen|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|twenty|thirty|forty|fifty|sixty|seventy|eighty|\d{1,2}|in (?:high )?school|in college|at (?:university|school|uni))|last (?:spring|summer|autumn|fall|winter)|(?:back )?in the (?:\w+ties|\w+ies)|in (?:my|her|his|their) (?:teens|twenties|thirties|forties|fifties|sixties|seventies|eighties|youth)|used to|in the past|previously|history of|past history|resolved|went away|gone now|has gone|had gone)\b`)
var resolvedNegation = regexp.MustCompile(`(?i)\b(?:no longer|not any ?more|any ?more)\b`)
var historyYear = regexp.MustCompile(`(?i)\b(?:back in|in|during|since)\s+((?:19|20)\d\d)\b`)
var recentHistory = regexp.MustCompile(`(?i)\b(?:last (?:week|month|weekend)|(?:a|one|two|three|four|five|six|seven|\d+|a few|few|several|a couple of|couple of) (?:days?|weeks?|months?) ago|days ago|weeks ago|months ago|earlier this (?:week|month)|the other day)\b`)
var currentNow = regexp.MustCompile(`(?i)\b(?:right now|now|currently|at the moment|at present|today|tonight|this (?:morning|afternoon|evening|week)|just now|(?:an|one|a few|few|several|\d+|two|three|five|ten|twenty|thirty) (?:minutes?|mins?|hours?|hrs?) ago|an hour ago|yesterday|last night|ongoing)\b`)
var nowResolved = regexp.MustCompile(`(?i)\bnow\s+(?:\w+\s+)?(?:gone|better|fine|ok|okay|resolved|over|normal|cleared|well|controlled)\b`)
var sameEpisodeAgain = regexp.MustCompile(`(?i)\b(?:again|still|is back|are back|['’]s back|came back|come back|coming back|has come back|have come back|returned|started again|keeps? (?:happening|coming back)|recurr(?:ed|ing))\b`)
var familyHistoryCue = regexp.MustCompile(`(?i)\b(?:runs? in (?:my|the|our) family|in (?:my|the|our) family|family history)\b`)

var relativeNouns = map[string]bool{}
var thirdPronouns = map[string]bool{}
var firstPronouns = map[string]bool{}
var someonePronouns = map[string]bool{}

func init() {
	for _, w := range strings.Fields("mother mom mum mama mommy mummy father dad daddy papa grandad granddad gran nan nana nanna parent parents brother sister sibling siblings son daughter child children kid kids baby wife husband spouse partner boyfriend girlfriend fiance fiancee friend grandmother grandma granny grandfather grandpa grandparent grandparents aunt uncle cousin niece nephew neighbor neighbour coworker colleague roommate flatmate boss family relative relatives stepmother stepfather stepson stepdaughter") {
		relativeNouns[w] = true
	}
	for _, w := range strings.Fields("he she they him them his her their himself herself themselves he's she's they're they've he'd she'd they'd") {
		thirdPronouns[w] = true
	}
	for _, w := range strings.Fields("i i'm i've i'd i'll me myself mine im ive") {
		firstPronouns[w] = true
	}
	for _, w := range strings.Fields("someone somebody anyone anybody") {
		someonePronouns[w] = true
	}
}

type cueSpan struct{ start, end int }

func spans(re *regexp.Regexp, text string, from, to int) []cueSpan {
	var out []cueSpan
	for _, m := range re.FindAllStringIndex(text, -1) {
		if m[0] >= from && m[1] <= to {
			out = append(out, cueSpan{m[0], m[1]})
		}
	}
	return out
}

func overlaps(c cueSpan, ranges []cueSpan) bool {
	for _, r := range ranges {
		if c.start < r.end && r.start < c.end {
			return true
		}
	}
	return false
}

// masks returns phrases whose negation words belong to the phrase itself.
func masks(text string) []cueSpan {
	out := spans(pseudoNegation, text, 0, len(text))
	for _, m := range urgentSignMatches(text) {
		if m.atomic {
			out = append(out, cueSpan{m.start, m.end})
		}
	}
	return out
}

// negationCues returns the active negation cues in text.
func negationCues(text string) []cueSpan {
	mask := masks(text)
	var out []cueSpan
	for _, c := range spans(negationCue, text, 0, len(text)) {
		if !overlaps(c, mask) {
			out = append(out, c)
		}
	}
	return out
}

// negationScopeEnd is the byte offset where cue c stops applying.
func negationScopeEnd(text string, toks []token, c cueSpan, clauseEnd int) int {
	count := 0
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.start < c.end {
			continue
		}
		if t.start >= clauseEnd {
			return clauseEnd
		}
		if scopeTerminator[t.word] {
			return t.start
		}
		gap := text[prevEnd(toks, i, c.end):t.start]
		if strings.Contains(gap, ",") && !listContinues(toks, i, clauseEnd, text) {
			return t.start
		}
		if t.word == "and" && !shortItemFollows(toks, i+1, clauseEnd, text) {
			return t.start
		}
		count++
		if count >= 8 {
			return t.end
		}
	}
	return clauseEnd
}

func prevEnd(toks []token, i, fallback int) int {
	if i > 0 && toks[i-1].end > fallback {
		return toks[i-1].end
	}
	return fallback
}

// listContinues reports whether the comma before toks[i] continues a negated
// list such as "no fever, chills, or difficulty breathing".
func listContinues(toks []token, i, clauseEnd int, text string) bool {
	items := 0
	for j := i; j < len(toks) && toks[j].start < clauseEnd; j++ {
		if listJoiner[toks[j].word] {
			return true
		}
		if clauseVerb[toks[j].word] {
			return false
		}
		items++
		if j+1 < len(toks) && strings.Contains(text[toks[j].end:toks[j+1].start], ",") {
			items = 0
			continue
		}
		if items > 4 {
			return false
		}
	}
	return false
}

func shortItemFollows(toks []token, i, clauseEnd int, text string) bool {
	n := 0
	for j := i; j < len(toks) && toks[j].start < clauseEnd; j++ {
		w := toks[j].word
		if w == "and" || w == "or" || strings.Contains(text[toks[j-1].end:toks[j].start], ",") && j > i {
			break
		}
		if clauseVerb[w] || firstPronouns[w] {
			return false
		}
		n++
		if n > 3 {
			return false
		}
	}
	return n > 0
}

// subjectWords start a clause. Possessives such as "my" don't, so the condition
// in "if I lose my vision" stays whole.
var subjectWords = map[string]bool{"i": true, "i'm": true, "i've": true, "i'd": true, "i'll": true, "he": true, "he's": true, "she": true, "she's": true, "it": true, "it's": true, "they": true, "they're": true, "we": true, "we're": true, "you": true, "you're": true, "someone": true, "somebody": true}

func hypotheticalScopeEnd(text string, toks []token, c cueSpan, clauseEnd int) int {
	cue := strings.ToLower(text[c.start:c.end])
	if cue != "if" && cue != "what if" {
		return clauseEnd
	}
	subjects := 0
	for i, t := range toks {
		if t.start < c.end {
			continue
		}
		if t.start >= clauseEnd {
			break
		}
		if i > 0 && strings.Contains(text[toks[i-1].end:t.start], ",") && toks[i-1].end >= c.end {
			return t.start
		}
		if subjectWords[t.word] {
			subjects++
			if subjects == 2 {
				return t.start
			}
		}
	}
	return clauseEnd
}

func contentless(text string, start, end int) bool {
	clause := text[start:end]
	for _, m := range uncertaintyCue.FindAllStringIndex(clause, -1) {
		clause = clause[:m[0]] + strings.Repeat(" ", m[1]-m[0]) + clause[m[1]:]
	}
	for _, t := range tokenize(clause) {
		switch t.word {
		case "i", "i'm", "im", "am", "really", "just", "so", "but", "and", "very", "totally", "quite", "it's", "it", "is", "that", "honestly", "well", "about", "this":
		default:
			return false
		}
	}
	return true
}

func uncertainAt(text string, at int) bool {
	cs, ce := clauseRange(text, at)
	if len(spans(uncertaintyCue, text, cs, ce)) > 0 {
		return true
	}
	ss, se := sentenceRange(text, at)
	// "I'm not sure, but I think I fainted" and "I fainted, I'm not sure."
	if cs > ss {
		ps, pe := clauseRange(text, cs-1)
		if ps >= ss && len(spans(uncertaintyCue, text, ps, pe)) > 0 && contentless(text, ps, pe) {
			return true
		}
	}
	if ce < se {
		ns, ne := clauseRange(text, ce)
		if ne <= se && len(spans(uncertaintyCue, text, ns, ne)) > 0 && contentless(text, ns, ne) {
			return true
		}
	}
	return false
}

func historyAt(text string, at int, cues []cueSpan) int {
	cs, ce := clauseRange(text, at)
	ss, se := sentenceRange(text, at)
	resolved := spans(nowResolved, text, cs, ce)
	for _, c := range spans(currentNow, text, cs, ce) {
		if !overlaps(c, resolved) {
			return historyNone
		}
	}
	if len(spans(sameEpisodeAgain, text, ss, se)) > 0 {
		return historyNone
	}
	if len(spans(remoteHistory, text, cs, ce)) > 0 {
		return historyRemote
	}
	negatedClause := false
	for _, c := range cues {
		if c.start >= cs && c.end <= ce {
			negatedClause = true
		}
	}
	if negatedClause && len(spans(resolvedNegation, text, cs, ce)) > 0 {
		return historyRemote
	}
	for _, m := range historyYear.FindAllStringSubmatchIndex(text, -1) {
		if m[0] >= cs && m[1] <= ce {
			year, _ := strconv.Atoi(text[m[2]:m[3]])
			if year < time.Now().Year() {
				return historyRemote
			}
			return historyRecent
		}
	}
	if len(spans(recentHistory, text, cs, ce)) > 0 {
		return historyRecent
	}
	return historyNone
}

// otherAt decides whether the words at a span describe someone other than the user.
func otherAt(text string, toks []token, start, end int, src textSource) bool {
	cs, ce := clauseRange(text, start)
	if len(spans(familyHistoryCue, text, cs, ce)) > 0 {
		return true
	}
	other, found := false, false
	for i, t := range toks {
		if t.start < cs || t.start >= end || t.start >= ce {
			continue
		}
		var marks, isOther bool
		switch {
		case (t.word == "my" || t.word == "our" || t.word == "his" || t.word == "her" || t.word == "their" || t.word == "your") && i+1 < len(toks) && relativeNouns[strings.TrimSuffix(toks[i+1].word, "'s")]:
			marks = true
			if src == fromWitness {
				isOther = t.word != "my" && t.word != "our"
			} else {
				isOther = true
			}
		case thirdPronouns[t.word]:
			marks, isOther = true, src == fromUser
		case relativeNouns[strings.TrimSuffix(t.word, "'s")] && (t.start == cs || i > 0 && toks[i-1].start < cs || i == 0 || i > 0 && (toks[i-1].word == "a" || toks[i-1].word == "the" || toks[i-1].word == "one")):
			marks, isOther = true, true
		case someonePronouns[t.word] && t.start < start:
			marks, isOther = true, true
		case firstPronouns[t.word] || t.word == "my":
			marks, isOther = true, src == fromWitness
		}
		if marks {
			found = true
			other = isOther
		}
	}
	if !found {
		return false
	}
	return other
}

// analyzeSpan reads the span text[start:end] in context.
func analyzeSpan(text string, start, end int, src textSource) spanContext {
	var c spanContext
	if start < 0 || end > len(text) || start > end {
		return c
	}
	toks := tokenize(text)
	cs, ce := clauseRange(text, start)
	cues := negationCues(text)
	for _, cue := range cues {
		if cue.start < cs || cue.end > start {
			continue
		}
		if negationScopeEnd(text, toks, cue, ce) > start {
			c.Negated = true
		}
	}
	if postNegation.MatchString(text[end:]) {
		c.Negated = true
	}
	c.Uncertain = uncertainAt(text, start)
	mask := append(spans(uncertainWithIf, text, 0, len(text)), spans(presentIf, text, 0, len(text))...)
	for _, h := range spans(hypotheticalCue, text, cs, ce) {
		if overlaps(h, mask) || h.end > start {
			continue
		}
		if hypotheticalScopeEnd(text, toks, h, ce) > start {
			c.Hypothetical = true
		}
	}
	ss, _ := sentenceRange(text, start)
	if modalQuestion.MatchString(text[ss:]) {
		c.Hypothetical = true
	}
	c.Other = otherAt(text, toks, start, end, src)
	c.History = historyAt(text, start, cues)
	return c
}

// negatedInside reports whether a negation cue sits inside the span itself,
// as in "speech wasn't slurred".
func negatedInside(text string, start, end int) bool {
	for _, c := range negationCues(text) {
		if c.start >= start && c.end <= end {
			return true
		}
	}
	return false
}

var conceptStopwords = map[string]bool{"a": true, "an": true, "the": true, "of": true, "in": true, "on": true, "to": true, "my": true, "and": true, "or": true, "with": true, "at": true, "from": true, "for": true, "when": true, "after": true, "before": true, "during": true, "any": true, "some": true, "is": true, "are": true, "possible": true, "concern": true, "about": true, "history": true, "new": true, "mild": true, "severe": true, "sudden": true, "left": true, "right": true, "current": true, "recent": true, "symptoms": true, "symptom": true}

var synonymGroups = [][]string{
	{"pain", "painful", "hurt", "hurts", "hurting", "ache", "aches", "aching", "sore", "soreness", "tender"},
	{"breath", "breathe", "breathing", "breathless", "breathlessness", "air", "gasping"},
	{"vomit", "vomiting", "vomited", "throw", "threw", "throwing"},
	{"itch", "itching", "itchy"},
	{"dizzy", "dizziness", "lightheaded", "lightheadedness", "vertigo", "spinning"},
	{"tired", "tiredness", "fatigue", "exhausted", "exhaustion"},
	{"swell", "swelling", "swollen", "swelled", "swells"},
	{"bleed", "bleeding", "blood", "bleeds", "bloody"},
	{"faint", "fainting", "fainted", "faintness", "syncope", "pass", "passed", "passing", "blacked", "blackout", "collapse", "collapsed", "consciousness", "unconscious", "unconsciousness"},
	{"fever", "feverish", "temperature"},
	{"nausea", "nauseous", "nauseated", "sick", "queasy"},
	{"radiating", "radiate", "radiates", "spread", "spreading", "spreads"},
	{"seizure", "seizures", "fit", "fits", "convulsion", "convulsions"},
	{"confusion", "confused"},
	{"weakness", "weak", "weaker"},
	{"numbness", "numb"},
	{"urination", "urinate", "urinating", "pee", "peeing", "urine"},
	{"stool", "stools", "poo", "bowel"},
	{"suicidal", "suicide", "kill", "die", "dying", "alive", "life"},
	{"harm", "hurt", "hurting", "cut", "cutting"},
	{"sweating", "sweat", "sweaty"},
	{"speech", "speaking", "speak", "talking", "talk", "words", "slurred", "slurring"},
	{"difficulty", "trouble", "problem", "problems"},
	{"droop", "drooping", "droopy", "drooped"},
	{"facial", "face"},
	{"consciousness", "conscious", "responding", "unresponsive"},
}

var synonyms = map[string]map[string]bool{}

func init() {
	for _, g := range synonymGroups {
		for _, a := range g {
			if synonyms[a] == nil {
				synonyms[a] = map[string]bool{}
			}
			for _, b := range g {
				synonyms[a][b] = true
			}
		}
	}
}

func sameWord(concept, word string) bool {
	if concept == word || synonyms[concept][word] {
		return true
	}
	n := 4
	if len(concept) >= n && len(word) >= n && concept[:n] == word[:n] {
		return true
	}
	return false
}

// conceptTargets returns the spans inside [start,end) that name the concept.
func conceptTargets(text string, start, end int, concept string) []cueSpan {
	var words []string
	for _, t := range tokenize(concept) {
		for _, part := range strings.Split(t.word, "-") {
			if part != "" && !conceptStopwords[part] {
				words = append(words, part)
			}
		}
	}
	var out []cueSpan
	for _, t := range tokenize(text) {
		if t.start < start || t.end > end {
			continue
		}
		for _, w := range words {
			if sameWord(w, t.word) {
				out = append(out, cueSpan{t.start, t.end})
				break
			}
		}
	}
	return out
}
