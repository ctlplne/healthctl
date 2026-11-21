package clinical

import (
	"regexp"
	"strings"
	"time"
)

// These conservative, intentionally narrow development rules complement an explicit
// patient screen and model flags. Policy scope and limitations are in POLICY.md.
var emergencySigns = regexp.MustCompile(`(?i)\b(can(?:not|'t|’t) breathe|struggl(?:e|ing) to breathe|severe (?:difficulty|trouble) breathing|severe (?:central )?chest (?:pain|pressure)|crushing (?:chest )?(?:pain|pressure)|chest pain|vomit(?:ing|ed)? blood|cough(?:ing|ed)? up blood|bleeding (?:that )?(?:will not|won't|won’t) stop|sudden(?:ly)? (?:one.sided weakness|facial droop|severe headache|trouble walking|trouble speaking)|severe abdominal pain|feel(?:ing)? suicidal|going to kill myself)\b`)
var negative = regexp.MustCompile(`(?i)\b(no|never|without|den(?:y|ies|ied)|negative for|not (?:having|experiencing|present|now|currently)|do(?:es)? not (?:have|feel)|did not have|don['’]t (?:have|feel)|isn['’]t|absent|none)\b`)
var uncertain = regexp.MustCompile(`(?i)\b(not sure|unsure|uncertain|might|maybe|possibly|can't remember|cannot remember|don't know|don’t know)\b`)
var historical = regexp.MustCompile(`(?i)\b(years? ago|last year|in childhood|used to|as a child|in the past|previously|no longer|resolved|gone now|not anymore|past history|back in (?:19|20)[0-9]{2})\b`)
var thirdParty = regexp.MustCompile(`(?i)\b(my|our|his|her|their) (?:father|mother|dad|mom|mum|parent|family|grandmother|grandfather|brother|sister|friend|child|son|daughter|wife|husband|partner|patient)\b|\b(he|she|they) (?:has|had|is|was|could|can|feels)\b`)
var speculating = regexp.MustCompile(`(?i)\b(if (?:i|someone)|what if|hypothetically|for example|an example|a quote)\b`)
var clauseBreak = regexp.MustCompile(`(?i)[.;!?\n]|\b(?:but|however|whereas)\b|,\s*(?:and\s+)?(?:now\s+)?(I |my |currently )|\band\s+(?:now\s+)?(I |my )`)

func clauseAt(text string, at int) string {
	start, end := 0, len(text)
	for _, rng := range clauseBreak.FindAllStringSubmatchIndex(text, -1) {
		if rng[1] <= at {
			start = rng[1]
			for group := 2; group < len(rng); group += 2 {
				if rng[group] >= 0 {
					start = rng[group]
					break
				}
			}
		} else if rng[0] > at {
			end = rng[0]
			break
		}
	}
	return text[start:end]
}

func contextualRisk(text string) bool {
	// Uncertainty about an emergency symptom is kept visible. Explicit denial,
	// remote history and statements about other people are handled separately.
	return (!negative.MatchString(text) || uncertain.MatchString(text)) && !historical.MatchString(text) && !thirdParty.MatchString(text) && !speculating.MatchString(text)
}

func (e *Encounter) addWarning(ev Evidence, rule, emergency string) {
	for _, w := range e.Warnings {
		if w.Active && w.Evidence == ev {
			return
		}
	}
	message := "Seek local emergency care immediately. Do not wait to finish this interview."
	if strings.TrimSpace(emergency) != "" {
		message += " " + strings.TrimSpace(emergency)
	}
	factID := ""
	for _, f := range e.Facts {
		if f.Active && f.Evidence == ev && (rule == "reported_warning" || rule == "structured_finding") && (emergencySigns.MatchString(f.Concept) || f.Corrected) {
			factID = f.ID
			break
		}
	}
	e.Warnings = append(e.Warnings, Warning{Evidence: ev, FactID: factID, ID: id("w", len(e.Warnings)), Rule: rule, PolicyVersion: PolicyVersion, Message: message, Active: true, CreatedAt: time.Now().UTC()})
}

func (e *Encounter) screen(a Answer, emergency string) {
	if pastEvent(a) {
		return
	}
	if a.Kind == "screen" {
		if a.Text == urgentYes {
			e.addWarning(Evidence{a.ID, a.Text}, "explicit_screen", emergency)
		}
		return
	}
	for _, span := range emergencySigns.FindAllStringIndex(a.Text, -1) {
		if contextualRisk(clauseAt(a.Text, span[0])) {
			e.recordUrgentSign(a, span, emergency)
		}
	}
}

func (e *Encounter) recordUrgentSign(a Answer, span []int, emergency string) {
	quote := a.Text[span[0]:span[1]]
	ev := Evidence{a.ID, quote}
	status := "reported_present"
	if uncertain.MatchString(clauseAt(a.Text, span[0])) {
		status = "uncertain"
	}
	e.recordFindingAt(FindingProposal{Evidence: ev, Concept: quote, Status: status, Subject: "self"}, "patient_answer", "", false, span[0])
	e.addWarning(ev, "reported_warning", emergency)
}

var firstPerson = regexp.MustCompile(`(?i)\b(i|my|me)\b`)

// The structured correction wins for its primary topic. An additional clause may
// independently report a new symptom, even when the corrected finding is family
// history, denied or historical.
func (e *Encounter) screenCorrection(a Answer, f FindingProposal, emergency string) {
	active := isCurrentPersonalFinding(f)
	primarySign := false
	for _, span := range emergencySigns.FindAllStringIndex(a.Text, -1) {
		clause := clauseAt(a.Text, span[0])
		if !contextualRisk(clause) {
			continue
		}
		separate := strings.TrimSpace(a.Text[:span[0]]) != "" && len(clause) < len(a.Text) &&
			strings.Index(a.Text, clause) > 0 && (firstPerson.MatchString(clause) ||
			(f.Subject == "self" && strings.Contains(strings.ToLower(clause), "now")))
		if separate {
			e.recordUrgentSign(a, span, emergency)
			continue
		}
		if active {
			primarySign = true
		}
	}
	if active && (primarySign || emergencySigns.MatchString(f.Concept) && contextualRisk(f.Quote)) {
		e.addWarning(f.Evidence, "structured_finding", emergency)
	}
}

func (e *Encounter) hasWarning() bool {
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
	return historical.MatchString(s) || strings.EqualFold(s, "historical") || strings.EqualFold(s, "past")
}

func isCurrentPersonalFinding(f FindingProposal) bool {
	return f.Subject == "self" && (f.Status == "reported_present" || f.Status == "uncertain") && !isHistoricalTime(f.TimeContext)
}

func pastEvent(a Answer) bool {
	d, err := time.Parse(time.DateOnly, a.EventDate)
	return err == nil && time.Since(d) > 48*time.Hour
}
