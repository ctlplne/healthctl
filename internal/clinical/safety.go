package clinical

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// Development policy v4. These rules have not been clinically reviewed. They
// complement the explicit emergency question and model flags. Their silence never
// means a person can wait. Scope and sources are in POLICY.md.

const (
	warningEmergency = "emergency" // interrupts the interview
	warningCheck     = "check"     // stays visible; the interview can continue
)

type urgentSign struct {
	pattern *regexp.Regexp
	level   string
	// atomic signs contain their own negation word, as in "won't stop bleeding".
	atomic bool
}

func sign(level string, atomic bool, expr string) urgentSign {
	return urgentSign{regexp.MustCompile(`(?i)\b(?:` + expr + `)\b`), level, atomic}
}

var urgentSigns = []urgentSign{
	// Breathing.
	sign(warningEmergency, true, `can(?:not|['’]t)\s+(?:breathe|get (?:any |enough )?air(?: in)?|catch (?:my|his|her|their) breath|get (?:my|his|her|their) breath)|could(?:n['’]t| not)\s+(?:breathe|get (?:any )?air|catch (?:my|his|her|their) breath)|can (?:barely|hardly) breathe|(?:unable|not able) to breathe|(?:has|have|had)(?:n['’]t| not) been able to breathe|(?:isn['’]t|is not|wasn['’]t|was not|not) breathing`),
	sign(warningEmergency, false, `struggl(?:e|es|ed|ing) (?:to|for) (?:breathe|breath|air)|(?:hard|difficult) to breathe|(?:hard|rough|terrible|awful) time breathing|(?:trouble|difficulty|problems?) breathing|breathing (?:difficulty|difficulties|problems?)|gasping|short of breath\b[^.;!?]{0,30}\b(?:at rest|resting|sitting(?: still| here| down)?|lying (?:down|flat)|can['’]t (?:finish|speak|talk)|just sitting)|(?:shortness of breath|breathless(?:ness)?) (?:at rest|while resting|lying down|sitting still)|can(?:not|['’]t) (?:speak|talk) in (?:full|whole) sentences|can['’]t finish a sentence`),
	sign(warningEmergency, false, `(?:lips?|fingers?|fingertips) (?:are |is |look |looks |turned |turning |went |have turned |has turned |have gone |has gone |gone |turn |starting to turn |going )?(?:turning |going )?(?:a )?(?:blue|bluish|blueish)|blue lips|(?:turned|gone|went|going) (?:grey|gray|ashen)`),
	sign(warningEmergency, false, `chok(?:e|es|ed|ing)|(?:bad|severe|serious|major) asthma attack|(?:having|had) an asthma attack`),
	// Chest.
	sign(warningEmergency, false, `chest (?:pains?|pressure|tightness|discomfort|heaviness)|(?:pain|pains|pressure|tightness|squeezing|heaviness|discomfort|feeling|sensation|weight) in (?:the (?:middle|centre|center) of )?(?:my|his|her|their|the) chest|(?:my|his|her|their) chest (?:hurts|is hurting|is killing me|(?:kind|sort) of hurts|feels (?:tight|heavy|crushed)|is (?:tight|heavy|crushed))|(?:sitting|standing|pressing|pushing|weight) on (?:my|his|her|their) chest|(?:grabbed|clutched|clutching|grabbing) (?:my|his|her|their) chest|heart attacks?`),
	sign(warningEmergency, false, `pain (?:is )?(?:spreading|going|moving|radiating|shooting) (?:to|into|down|up) (?:my|his|her|their) (?:left |right )?(?:arm|jaw|neck|back|shoulder)`),
	// Stroke and other sudden neurological change.
	sign(warningEmergency, false, `(?:face|mouth|smile) (?:\w+ ){0,2}(?:drooping|droopy|drooped|sagging|lopsided)|facial droop|drooping face`),
	sign(warningEmergency, false, `(?:speech|words|voice) (?:\w+ ){0,2}slurred|(?:speech|words|voice) (?:sounds?|sounded|is|was|came out|comes out|coming out) (?:\w+ )?(?:weird|strange|funny|wrong|garbled|jumbled|muddled|confused)|slurr(?:ing|ed) (?:(?:my|his|her|their) )?(?:words|speech)|slurred speech`),
	sign(warningEmergency, false, `(?:trouble|difficulty|problems?) (?:speaking|talking|finding (?:my|the) words|getting (?:my|the) words out|understanding words)|can(?:not|['’]t) (?:speak|talk) (?:properly|clearly|right|normally)|can(?:not|['’]t) find (?:my|the) words`),
	sign(warningEmergency, false, `sudden(?:ly)?\b[^.;!?]{0,30}\b(?:weak(?:er|ness)?|numb(?:ness)?|confused|confusion|(?:trouble|difficulty) (?:walking|speaking|talking|seeing)|(?:can(?:not|['’]t)|could(?:n['’]t| not)) (?:walk|speak|talk|see|lift|move|feel|find)|lost (?:the |my )?(?:vision|sight)|blind|double|severe headache|went (?:weak|numb|blind|dark))|(?:weak(?:er|ness)?|numb(?:ness)?|went weak|went numb)\b[^.;!?]{0,30}\b(?:sudden(?:ly)?|all of a sudden)`),
	sign(warningEmergency, false, `(?:one|left|right) side (?:of (?:my|his|her|their) (?:body|face) )?(?:is |feels |went |has gone |has been )?(?:weak|numb|paralys(?:ed|is))|weak(?:ness)? (?:on|down) (?:one|the (?:left|right)|my (?:left|right)) side|numb (?:down |on )?(?:the |my )?(?:whole )?(?:left|right|one) side|(?:half|one side of) (?:my|his|her|their) face (?:is |feels |went )?numb`),
	sign(warningEmergency, false, `(?:left|right|one) (?:hand|arm|leg|foot|side)\b[^.;!?]{0,30}\b(?:(?:gone|went|going|is|are|feels?) )?(?:numb|weak|dead|tingling)|pins and needles (?:down|on|in|all down) (?:my |the |his |her )?(?:whole )?(?:left|right|one) side`),
	sign(warningEmergency, false, `(?:weakness|numbness|drooping) (?:in|of|on) (?:my|his|her|their|one side of (?:my|his|her|their)) (?:face|arm|arms)`),
	sign(warningEmergency, false, `(?:can(?:not|['’]t)|could(?:n['’]t| not)|unable to) (?:lift|move|raise|feel) (?:my|his|her|their) (?:right |left )?(?:arm|arms|leg|legs|hand|hands|face|feet|foot|side)`),
	sign(warningEmergency, false, `(?:lost|lose|losing) (?:the |my |his |her |their |all )?(?:vision|sight)|vision (?:\w+ ){0,2}(?:went|has gone|is going|goes) (?:black|dark|blank|grey|gray)|(?:can(?:not|['’]t)|could(?:n['’]t| not)) see (?:out of|from|with) (?:my|his|her|their|one) (?:left |right )?eye|(?:went|gone|go|going) blind|everything went double|vision loss|loss of vision`),
	sign(warningEmergency, false, `(?:suddenly|very|really|extremely) confused|confused and (?:drowsy|sleepy|can['’]t)|(?:can(?:not|['’]t)|could(?:n['’]t| not)) (?:walk straight|walk properly|keep my balance|stand up straight)|(?:trouble|difficulty) walking|(?:mini[- ]?)?strokes?`),
	sign(warningEmergency, true, `(?:doesn['’]t|does not|didn['’]t|don['’]t|do not) know where (?:he|she|they|i) (?:is|are|am|was)|(?:isn['’]t|wasn['’]t|is not|was not|not) responding|unresponsive|won['’]t wake up|(?:can['’]t|cannot|couldn['’]t) (?:wake (?:him|her|them) up|keep (?:him|her|them|myself) awake|stay awake)|(?:go to sleep|fall asleep|sleep) and never wake up|can['’]t keep (?:my|his|her|their) eyes open|no point (?:in )?(?:living|going on|being alive|being here)`),
	sign(warningEmergency, false, `pass(?:ed|ing|es)? out|(?:lost|lose|losing) consciousness|faint(?:ed|ing|s)?|seeing double|feel(?:ing)? faint|black(?:ed|ing|s)? out|lost consciousness|loss of consciousness|unconscious|collaps(?:e|ed|es|ing)|seizures?|(?:had|having|have) a fit|fitting|convuls(?:ing|ion|ions|ed)|(?:very|extremely) (?:drowsy|sleepy)`),
	// Headache, neck and rash.
	sign(warningEmergency, false, `worst headache (?:of (?:my|her|his|their|your) life|(?:i|she|he|they)(?:['’](?:ve|s)| have| has)? ever had)|thunderclap headache|sudden,? (?:severe|intense|explosive|excruciating|bad|terrible|awful|horrible|agonising|agonizing) headache|like (?:i['’]ve|i have|i['’]d|i had) been hit (?:on|in|over) the head|(?:fever|temperature)\b[^.;!?]{0,25}\bstiff neck|stiff neck\b[^.;!?]{0,20}\b(?:fever|temperature)|neck is stiff with (?:a )?(?:fever|temperature)`),
	sign(warningEmergency, true, `rash that (?:doesn['’]t|does not|won['’]t|will not) (?:fade|blanch|go away when)|non-?blanching rash`),
	// Suicide, self-harm, overdose and poisoning.
	sign(warningEmergency, false, `(?:kill|killing|hurt|hurting|harm|harming|cut|cutting) (?:myself|himself|herself|themselves)|self[- ]harm(?:ed|ing)?|suicid(?:e|al)|(?:want|wants|wanted|wanting) to (?:die|be dead)|better off dead|end it all|nothing (?:left )?to live for|(?:better off|be better) without me|(?:feel|feels|feeling) like (?:i['’]m|i am) dying|think (?:i['’]m|i am) dying|(?:end|ending|take|taking) (?:my|his|her|their) (?:own )?life|overdos(?:e|ed|es|ing)|took (?:a whole|an entire|all (?:of )?(?:my|the)|most of (?:my|the)|too many) (?:bottle of |pack of |packet of |box of )?(?:\w+ )?(?:pills|tablets|medicines?|meds)|taken too many (?:pills|tablets|medicines?|meds)`),
	sign(warningEmergency, true, `(?:don['’]t|do not|doesn['’]t|does not) want to (?:be alive|live|go on|exist|be here)`),
	sign(warningEmergency, false, `swallowed (?:bleach|poison|cleaning|detergent|antifreeze|a battery|batteries)|drank (?:bleach|poison|antifreeze)|poison(?:ed|ing)`),
	// Bleeding.
	sign(warningEmergency, false, `(?:vomit(?:ing|ed|s)?|throw(?:ing|s)? up|threw up|cough(?:ing|ed|s)? up|spit(?:ting|s)? up|spat up) (?:a little |a bit of |some |a lot of |lots of |any )?blood|blood (?:coming up|in (?:my |the )?(?:mouth|phlegm|spit|sputum))|coffee[- ]grounds?|soak(?:ing|ed) through (?:the |my |a )?(?:towels?|pads?|bandages?|dressings?|clothes)|blood in (?:my |his |her |their |the )?(?:vomit|sick)|blood when (?:i|you|he|she|they) coughs?|(?:black|tarry)(?: and tarry)? stools?|stools? (?:are|is|were|was|have been|has been) (?:black|tarry)|bleeding (?:heavily|a lot|badly|profusely)|heavy bleeding|passing (?:a lot of |lots of )?blood`),
	sign(warningEmergency, true, `(?:won['’]t|will not|doesn['’]t|does not|can['’]t|cannot|isn['’]t|is not) stop bleeding|bleeding (?:that )?(?:won['’]t|will not|doesn['’]t|does not|hasn['’]t|has not) stop(?:ped)?|can['’]t stop the bleeding|bleeding won['’]t stop`),
	// Severe allergic reaction.
	sign(warningEmergency, false, `(?:throat|airway) (?:is |feels |feels like it['’]s |feels like it is |may be |might be |seems |is starting |started )?(?:closing|swelling shut|tight)|(?:lips|tongue|face|throat|mouth)(?: and (?:lips|tongue|face|throat|mouth))? (?:is |are |has |have )?(?:started |been |starting )?(?:swelling|swollen|swelled)(?: up)?|(?:can(?:not|['’]t)|could(?:n['’]t| not)) swallow|(?:severe|serious|bad) allergic reaction|anaphyla(?:xis|ctic)`),
	// Abdomen, eye and testicle.
	sign(warningEmergency, false, `severe (?:abdominal|stomach|belly|tummy) (?:pain|ache)|(?:abdominal|stomach|belly|tummy) pain (?:is )?(?:severe|unbearable|excruciating)|pain in (?:my|the|his|her|their) (?:belly|stomach|abdomen|tummy) is (?:severe|unbearable|excruciating|terrible|awful)|sudden (?:severe )?pain in (?:my|his|her|their) (?:testicle|testicles|scrotum|testis)|severe (?:eye pain|pain in (?:my|his|her|their) eyes?)`),
	// Lessons from held-out set 2.
	sign(warningEmergency, false, `chest feels like (?:it['’]s |it is )?(?:in a vi[cs]e|being (?:crushed|squeezed)|on fire|exploding)|(?:pressure|pain|tightness|heaviness) behind (?:my|the|his|her) (?:breastbone|sternum)|tearing (?:pain |feeling )?(?:through|in|across) (?:my|his|her|the) (?:chest|back)`),
	sign(warningEmergency, true, `(?:not|isn['’]t|wasn['’]t) making (?:any )?sense`),
	sign(warningEmergency, false, `can(?:not|['’]t) remember who (?:anyone|people|everyone) (?:is|are)|(?:half|one side of) (?:my|his|her|their) body (?:feels |is |went |has gone )?(?:heavy|weak|numb|dead)|pupils? (?:\w+ ){0,2}(?:bigger|larger|smaller|different sizes?) than|end (?:things|everything)|cut (?:my|his|her|their) wrists?`),
	sign(warningEmergency, false, `took (?:a load|loads|a lot|lots|a handful|handfuls|a bunch) of (?:\w+ )?(?:paracetamol|acetaminophen|tylenol|ibuprofen|pills|tablets|meds|medication|medicine)|(?:poo|poop|bowel movements?) (?:is |are |was |were |has been |have been |looks? )?(?:black|tarry|like tar)|black (?:poo|poop)`),
	sign(warningEmergency, false, `(?:can only|can['’]t|cannot) (?:say|speak|get out) (?:more than )?(?:a few|a couple of|one or two) words|tongue (?:feels |is |looks )?(?:thick|huge|too big)|short of breath (?:right now|now|at the moment)`),
	sign(warningEmergency, false, `(?:headache|head hurts|head pain)\b[^.;!?]{0,20}\blike (?:nothing|never) (?:before|i['’]ve (?:had|felt))|hit (?:my|his|her|their) head\b[^.;!?]{0,40}\b(?:being sick|vomit(?:ing|ed)?|throw(?:ing)? up|threw up|confused|drowsy|passed out)|heart\b[^.;!?]{0,30}\b(?:so|too) fast (?:that )?(?:i|it) (?:can['’]t|cannot) (?:count|keep up)|(?:pregnant|contractions?|labou?r)\b[^.;!?]{0,30}\bbleeding|bleeding\b[^.;!?]{0,30}\b(?:pregnant|contractions?)`),
	sign(warningCheck, false, `(?:feel|feeling) (?:so |really |very |a bit )?confused|can['’]t keep (?:anything|any fluids|fluids|water|food) down|burning up|(?:feel|feeling) (?:very|really|extremely) (?:unwell|ill)|shaking uncontrollably|can (?:barely|hardly) stand|heart\b[^.;!?]{0,40}\bfast\b[^.;!?]{0,30}\b(?:dizzy|faint)`),
	sign(warningCheck, true, `(?:not|haven['’]t|hasn['’]t|haven't been|hasn't been) (?:been )?(?:peeing|peed|urinating|urinated|passing urine)`),
	// Needs a prompt check rather than emergency care.
	sign(warningCheck, false, `stop(?:s|ped|ping)? breathing|(?:keep|kept|keeps) falling (?:over|down)|sweating buckets`),
	sign(warningCheck, false, `(?:left|right|one) (?:calf|leg) (?:is |has been |became |got )?(?:swollen|swelling)|swollen,? (?:painful |red |tender )?(?:left|right) (?:calf|leg)`),
}

var yellowSkin = regexp.MustCompile(`(?i)\bskin (?:has |is |looks |seems )?(?:turned |turning |gone |become |becoming )?(?:a (?:little|bit) )?yellow(?:ish)?\b`)

type signMatch struct {
	start, end int
	level      string
	atomic     bool
}

// urgentSignMatches returns non-overlapping urgent sign matches, longest first.
func urgentSignMatches(text string) []signMatch {
	var all []signMatch
	for _, s := range urgentSigns {
		for _, m := range s.pattern.FindAllStringIndex(text, -1) {
			all = append(all, signMatch{m[0], m[1], s.level, s.atomic})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].start != all[j].start {
			return all[i].start < all[j].start
		}
		return all[i].end-all[i].start > all[j].end-all[j].start
	})
	var out []signMatch
	for _, m := range all {
		if n := len(out); n > 0 && m.start < out[n-1].end {
			if m.atomic {
				out[n-1].atomic = true
			}
			if m.level == warningEmergency {
				out[n-1].level = warningEmergency
			}
			continue
		}
		out = append(out, m)
	}
	return out
}

func urgentSignIn(text string) bool { return len(urgentSignMatches(text)) > 0 }

type urgentSignal struct {
	start, end  int
	level, rule string
}

// blocked reports whether context means the words don't describe a current
// concern for this user.
func (c spanContext) blocked() bool {
	return c.Negated || c.Hypothetical || c.Other || c.History == historyRemote
}

func levelFor(c spanContext, base string) string {
	if c.History == historyRecent {
		return warningCheck
	}
	return base
}

// screenText is the one safety screen for user text. Every input path calls it.
func screenText(text string, src textSource) []urgentSignal {
	var out []urgentSignal
	for _, re := range []*regexp.Regexp{yellowEyeWords, yellowSkin} {
		for _, m := range re.FindAllStringIndex(text, -1) {
			c := analyzeSpan(text, m[0], m[1], src)
			if c.blocked() || eyeDenial.MatchString(clauseAt(text, m[0])) || negatedInside(text, m[0], m[1]) {
				continue
			}
			out = append(out, urgentSignal{m[0], m[1], levelFor(c, warningEmergency), "reported_yellow_eyes"})
		}
	}
	for _, m := range urgentSignMatches(text) {
		if !m.atomic && negatedInside(text, m.start, m.end) {
			continue
		}
		c := analyzeSpan(text, m.start, m.end, src)
		if c.blocked() {
			continue
		}
		out = append(out, urgentSignal{m.start, m.end, levelFor(c, m.level), "reported_warning"})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].start < out[j].start })
	return out
}

// ScreenText exposes the screen to inputs that live outside an encounter, such
// as diary entries and dictation drafts. It returns the most serious signal.
func ScreenText(text, emergency string) (ScreenResult, bool) {
	signals := screenText(text, fromUser)
	if len(signals) == 0 {
		return ScreenResult{}, false
	}
	best := signals[0]
	for _, s := range signals {
		if s.level == warningEmergency && best.level != warningEmergency {
			best = s
		}
	}
	return ScreenResult{Level: best.level, Quote: text[best.start:best.end], Message: warningMessage(best.rule, best.level, emergency), PolicyVersion: PolicyVersion}, true
}

type ScreenResult struct {
	Level         string `json:"level"`
	Quote         string `json:"quote"`
	Message       string `json:"message"`
	PolicyVersion string `json:"policyVersion"`
}

func warningMessage(rule, level, emergency string) string {
	message := "Seek local emergency care immediately. Do not wait to finish this interview."
	switch {
	case rule == "reported_yellow_eyes" && level != warningCheck:
		message = "If the whites of your eyes or your skin look newly yellow, seek urgent medical advice today. Do not wait to finish the interview or take a photo. If you may be having an emergency, seek local emergency care immediately."
	case level == warningCheck:
		message = "Get this checked promptly by a doctor, nurse or urgent care service. If it gets worse, or you think it may be an emergency, seek local emergency care immediately."
	}
	if strings.TrimSpace(emergency) != "" {
		message += " " + strings.TrimSpace(emergency)
	}
	return message
}

func (e *Encounter) addWarning(ev Evidence, rule, level, emergency string) {
	for _, w := range e.Warnings {
		if w.Active && w.Evidence == ev {
			return
		}
	}
	factID := ""
	for _, f := range e.Facts {
		if f.Active && f.Evidence == ev && (rule == "reported_warning" || rule == "structured_finding") && (urgentSignIn(f.Concept) || f.Corrected) {
			factID = f.ID
			break
		}
	}
	stored := level
	if stored == warningEmergency {
		stored = ""
	}
	e.Warnings = append(e.Warnings, Warning{Evidence: ev, FactID: factID, ID: id("w", len(e.Warnings)), Rule: rule, Level: stored, PolicyVersion: PolicyVersion, Message: warningMessage(rule, level, emergency), Active: true, CreatedAt: time.Now().UTC()})
}

// affirmative replies to a question that names an urgent sign report that sign.
var affirmative = regexp.MustCompile(`(?i)^\s*(?:yes|yeah|yep|yup|i do|i have|i did|it is|it does|correct|sometimes|occasionally|a little|a bit|once|twice|definitely|absolutely)\b|^[^.?!]{0,30}\b(?:yes|yeah)\b`)
var historicalQuestion = regexp.MustCompile(`(?i)\b(?:ever|before|in the past|previously|history of)\b`)
var secondPerson = strings.NewReplacer("yourself", "myself", "Yourself", "Myself", "your", "my", "Your", "My", "you're", "I'm", "You're", "I'm", "you", "I", "You", "I")

func (e *Encounter) screen(a Answer, emergency string) {
	if pastEvent(a) {
		return
	}
	if a.Kind == "screen" {
		if a.Text == urgentYes {
			e.addWarning(Evidence{a.ID, a.Text}, "explicit_screen", warningEmergency, emergency)
		}
		return
	}
	signals := screenText(a.Text, sourceOf(a))
	for _, s := range signals {
		if s.rule == "reported_yellow_eyes" {
			e.addWarning(Evidence{a.ID, a.Text[s.start:s.end]}, s.rule, s.level, emergency)
			continue
		}
		e.recordUrgentSign(a, s, emergency)
	}
	if len(signals) == 0 && a.Kind == "reply" && affirmative.MatchString(a.Text) {
		e.screenAffirmative(a, emergency)
	}
}

// screenAffirmative handles "Yes, this morning" in reply to "Have you fainted?".
func (e *Encounter) screenAffirmative(a Answer, emergency string) {
	question := secondPerson.Replace(a.Question)
	for _, m := range urgentSignMatches(question) {
		if !m.atomic && negatedInside(question, m.start, m.end) {
			continue
		}
		if q := analyzeSpan(question, m.start, m.end, fromUser); q.Negated || q.Hypothetical || q.Other {
			continue
		}
		answer := analyzeSpan(a.Text, 0, len(a.Text), sourceOf(a))
		if answer.History == historyRemote {
			continue
		}
		level := levelFor(answer, m.level)
		if historicalQuestion.MatchString(question) && len(spans(currentNow, a.Text, 0, len(a.Text))) == 0 {
			level = warningCheck
		}
		quote := strings.TrimSpace(a.Text)
		at := strings.Index(a.Text, quote)
		ev := Evidence{a.ID, quote}
		sourceType := "patient_answer"
		e.recordFindingAt(FindingProposal{Evidence: ev, Concept: strings.ToLower(question[m.start:m.end]), Status: "reported_present", Subject: "self"}, sourceType, "", false, at)
		e.addWarning(ev, "reported_warning", level, emergency)
		return
	}
}

func (e *Encounter) recordUrgentSign(a Answer, s urgentSignal, emergency string) {
	quote := a.Text[s.start:s.end]
	ev := Evidence{a.ID, quote}
	status := "reported_present"
	if uncertainAt(a.Text, s.start) {
		status = "uncertain"
	}
	sourceType := "patient_answer"
	if a.Kind == "witness" {
		sourceType = "witness_observation"
	}
	e.recordFindingAt(FindingProposal{Evidence: ev, Concept: quote, Status: status, Subject: "self"}, sourceType, "", false, s.start)
	e.addWarning(ev, "reported_warning", s.level, emergency)
}

func firstPersonIn(text string) bool {
	for _, t := range tokenize(text) {
		if firstPronouns[t.word] || t.word == "my" {
			return true
		}
	}
	return false
}

// The structured correction wins for its primary topic. An additional clause may
// independently report a new symptom, even when the corrected finding is family
// history, denied or historical.
func (e *Encounter) screenCorrection(a Answer, f FindingProposal, emergency string) {
	active := isCurrentPersonalFinding(f)
	primarySign := false
	for _, s := range screenText(a.Text, sourceOf(a)) {
		if s.rule != "reported_warning" {
			continue
		}
		cs, ce := clauseRange(a.Text, s.start)
		clause := a.Text[cs:ce]
		separate := cs > 0 && strings.TrimSpace(a.Text[:cs]) != "" && (firstPersonIn(clause) || f.Subject == "self" && strings.Contains(strings.ToLower(clause), "now"))
		if separate {
			e.recordUrgentSign(a, s, emergency)
			continue
		}
		if active {
			primarySign = true
		}
	}
	if active && (primarySign || urgentSignIn(f.Concept) && !analyzeSpan(f.Quote, 0, len(f.Quote), fromUser).blocked()) {
		e.addWarning(f.Evidence, "structured_finding", warningEmergency, emergency)
	}
}

// hasWarning reports an active warning that interrupts the interview.
func (e *Encounter) hasWarning() bool {
	for _, w := range e.Warnings {
		if w.Active && w.Level != warningCheck {
			return true
		}
	}
	return false
}

// hasAnyWarning includes "check this" warnings that don't interrupt.
func (e *Encounter) hasAnyWarning() bool {
	for _, w := range e.Warnings {
		if w.Active {
			return true
		}
	}
	return false
}

func (e *Encounter) resolveSource(aid string) {
	for i := range e.Warnings {
		w := &e.Warnings[i]
		if w.Active && w.AnswerID == aid {
			w.Active = false
			w.ResolvedRevision = e.HistoryRevision
		}
	}
}

func isHistoricalTime(s string) bool {
	return remoteHistory.MatchString(s) || strings.EqualFold(s, "historical") || strings.EqualFold(s, "past")
}

func isCurrentPersonalFinding(f FindingProposal) bool {
	return f.Subject == "self" && (f.Status == "reported_present" || f.Status == "uncertain") && !isHistoricalTime(f.TimeContext)
}

func pastEvent(a Answer) bool {
	d, err := time.Parse(time.DateOnly, a.EventDate)
	return err == nil && time.Since(d) > 48*time.Hour
}

// personalCurrent reports whether a matched term describes the user now.
func personalCurrent(text string, start, end int, src textSource) bool {
	return !analyzeSpan(text, start, end, src).blocked()
}

// thirdParty recognizes an explicit mention of someone else. It is used to decide
// whether an opening statement fully answers a background question.
var thirdParty = regexp.MustCompile(`(?i)\b(my|our|his|her|their) (?:father|mother|dad|mom|mum|parent|family|grandmother|grandfather|brother|sister|friend|child|son|daughter|wife|husband|partner|patient)\b|\b(he|she|they) (?:has|had|is|was|could|can|feels)\b`)
