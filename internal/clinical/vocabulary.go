package clinical

import (
	"regexp"
	"strings"
)

// The concept vocabulary maps a finding to a plain term and, where one is
// well established, an ICD-10-CM code. Contradiction checks compare terms, so
// "breathlessness" and "shortness of breath" are the same finding. This is
// development content that has not been clinically reviewed.
const VocabularyVersion = "development-2"

type conceptTerm struct {
	Term     string
	Code     string
	Synonyms []string
}

var vocabulary = []conceptTerm{
	{"Cough", "R05", []string{"coughing"}},
	{"Fever", "R50.9", []string{"high temperature", "a temperature", "feverish", "pyrexia"}},
	{"Shortness of breath", "R06.0", []string{"breathlessness", "breathless", "short of breath", "difficulty breathing", "trouble breathing", "breathing difficulty", "dyspnea", "dyspnoea"}},
	{"Wheezing", "R06.2", []string{"wheeze", "wheezy"}},
	{"Sneezing", "R06.7", []string{"sneeze", "sneezes"}},
	{"Chest pain", "R07.4", []string{"chest tightness", "chest pressure", "pain in my chest"}},
	{"Sore throat", "R07.0", []string{"throat pain", "painful throat", "throat feels sore"}},
	{"Runny nose", "", []string{"nose is runny", "rhinorrhea", "rhinorrhoea"}},
	{"Nasal congestion", "R09.81", []string{"blocked nose", "stuffy nose", "congested nose"}},
	{"Headache", "R51", []string{"head pain", "headaches"}},
	{"Dizziness", "R42", []string{"dizzy", "lightheaded", "light-headed", "lightheadedness", "vertigo"}},
	{"Fainting", "R55", []string{"fainted", "faint", "passed out", "syncope", "blacked out"}},
	{"Nausea", "R11.0", []string{"nauseous", "queasy", "feel sick"}},
	{"Vomiting", "R11.10", []string{"vomited", "throwing up", "threw up", "being sick"}},
	{"Diarrhea", "R19.7", []string{"diarrhoea", "loose stools", "loose stool", "watery stools"}},
	{"Constipation", "K59.00", []string{"constipated"}},
	{"Change in bowel habit", "R19.4", []string{"bowel change", "change in bowel movements"}},
	{"Abdominal pain", "R10.9", []string{"stomach pain", "belly pain", "tummy pain", "stomach ache", "stomachache", "pain in my abdomen", "abdominal cramps"}},
	{"Blood in stool", "K92.1", []string{"black stools", "tarry stools", "black or tarry stools", "melena", "melaena", "rectal bleeding"}},
	{"Vomiting blood", "K92.0", []string{"vomited blood", "hematemesis", "haematemesis"}},
	{"Coughing up blood", "R04.2", []string{"coughed up blood", "hemoptysis", "haemoptysis"}},
	{"Blood in urine", "R31.9", []string{"hematuria", "haematuria"}},
	{"Painful urination", "R30.0", []string{"pain when peeing", "burning when peeing", "dysuria", "stings when I pee"}},
	{"Frequent urination", "R35.0", []string{"peeing more often", "urinating more often"}},
	{"Fatigue", "R53.83", []string{"tiredness", "tired", "exhausted", "exhaustion", "worn out"}},
	{"Weakness", "R53.1", []string{"feel weak", "feeling weak"}},
	{"Rash", "R21", []string{"skin rash", "spots on my skin"}},
	{"Itching", "L29.9", []string{"itchy", "itch"}},
	{"Swelling", "R60.0", []string{"swollen", "puffy"}},
	{"Palpitations", "R00.2", []string{"heart racing", "racing heart", "pounding heart", "fluttering heart"}},
	{"Numbness", "R20.0", []string{"numb"}},
	{"Tingling", "R20.2", []string{"pins and needles", "tingly"}},
	{"Confusion", "R41.0", []string{"confused", "disoriented"}},
	{"Seizure", "R56.9", []string{"seizures", "convulsion", "convulsions"}},
	{"Slurred speech", "R47.81", []string{"slurring my words", "speech is slurred", "speech was slurred", "speech slurred", "slurred my words", "slurring words", "words were slurred", "words slurred"}},
	{"Facial droop", "R29.810", []string{"face drooped", "face drooping", "face droopy", "drooping face", "droopy face", "facial drooping", "mouth drooped", "mouth drooping", "drooping mouth", "smile drooped"}},
	{"Vision change", "H53.9", []string{"blurred vision", "blurry vision", "double vision", "loss of vision"}},
	{"Tremor", "R25.1", []string{"shaking hands", "shaky hands"}},
	{"Joint pain", "M25.50", []string{"sore joints", "painful joints"}},
	{"Difficulty swallowing", "R13.10", []string{"trouble swallowing", "hard to swallow", "dysphagia"}},
	{"Loss of smell", "R43.0", []string{"can't smell", "cannot smell", "anosmia"}},
	{"Weight loss", "R63.4", []string{"losing weight", "lost weight"}},
	{"Loss of appetite", "R63.0", []string{"not hungry", "no appetite", "off my food"}},
	{"Night sweats", "R61", []string{"sweating at night"}},
	{"Chills", "R68.83", []string{"shivering", "shivers"}},
	{"Yellow skin or eyes", "R17", []string{"jaundice", "yellow eyes", "yellow skin", "eye whites look yellow"}},
	{"Insomnia", "G47.00", []string{"can't sleep", "cannot sleep", "trouble sleeping"}},
	{"Lump", "R22.9", []string{"a lump", "a swelling"}},
	{"Suicidal thoughts", "R45.851", []string{"want to die", "kill myself", "end my life", "thoughts of suicide"}},
}

// Qualifiers that keep a concept's meaning. Location words are not here: an
// ankle swelling and a leg swelling are different findings.
var conceptQualifiers = map[string]bool{
	"dry": true, "wet": true, "productive": true, "mild": true, "moderate": true, "severe": true, "sudden": true, "intermittent": true,
	"persistent": true, "constant": true, "new": true, "recent": true, "current": true, "ongoing": true, "occasional": true, "slight": true,
	"bad": true, "worsening": true, "chronic": true, "acute": true, "frequent": true, "self-reported": true, "reported": true, "possible": true,
}

func (t conceptTerm) names() []string { return append([]string{t.Term}, t.Synonyms...) }

// genericWords don't identify a symptom on their own.
var genericWords = map[string]bool{"pain": true, "ache": true, "my": true, "in": true, "the": true, "a": true, "of": true, "or": true, "is": true, "when": true, "feel": true, "feeling": true, "up": true, "out": true, "to": true, "at": true, "on": true, "and": true, "loss": true, "change": true, "more": true, "often": true, "can't": true, "cannot": true, "trouble": true, "difficulty": true, "hard": true, "being": true}

// mentionedIn reports whether the user's words name this symptom, exactly or
// by every distinctive word of one of its names ("my stomach hurts" names
// stomach pain).
func (t conceptTerm) mentionedIn(lower string) bool {
	for _, n := range t.names() {
		n = strings.ToLower(n)
		if wholeWord(lower, n) {
			return true
		}
		distinctive := 0
		found := true
		for _, w := range strings.Fields(n) {
			if genericWords[w] {
				continue
			}
			distinctive++
			if !wholeWord(lower, w) {
				found = false
				break
			}
		}
		if found && distinctive > 0 {
			return true
		}
	}
	return false
}

var nonWord = regexp.MustCompile(`[^a-z0-9'’-]+`)

func wholeWord(text, phrase string) bool {
	text, phrase = strings.ToLower(text), strings.ToLower(phrase)
	for i := 0; ; {
		j := strings.Index(text[i:], phrase)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(phrase)
		if (start == 0 || !isWordByte(text[start-1])) && (end == len(text) || !isWordByte(text[end])) {
			return true
		}
		i = start + 1
	}
}

func isWordByte(b byte) bool { return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '\'' }

// lookupConcept maps a concept to a vocabulary term when it names one, alone
// or with qualifiers that keep its meaning.
func lookupConcept(concept string) (conceptTerm, bool) {
	words := strings.Fields(nonWord.ReplaceAllString(strings.ToLower(concept), " "))
	if len(words) == 0 {
		return conceptTerm{}, false
	}
	for len(words) > 0 && conceptQualifiers[words[0]] {
		words = words[1:]
	}
	core := strings.Join(words, " ")
	for _, t := range vocabulary {
		for _, n := range t.names() {
			if core == strings.Join(strings.Fields(nonWord.ReplaceAllString(strings.ToLower(n), " ")), " ") {
				return t, true
			}
		}
	}
	return conceptTerm{}, false
}

// sameConcept compares two findings by vocabulary term when both have one.
func sameConcept(a, b string) bool {
	if strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) {
		return true
	}
	ta, okA := lookupConcept(a)
	tb, okB := lookupConcept(b)
	return okA && okB && ta.Term == tb.Term
}

var medicineConcept = regexp.MustCompile(`(?i)\b(medicines?|medications?|tablets?|pills?|capsules?|supplements?|inhalers?|doses?|prescri\w*|drugs?|antibiotics?)\b`)
var allergyConcept = regexp.MustCompile(`(?i)\ballerg(?:y|ies|ic)\b`)

// factCategory places a finding in the brief's medicines and allergies
// section from where it was recorded: the question it answered, a medicine
// entry, or a concept that names a medicine or an allergy.
func (e *Encounter) factCategory(f FindingProposal) string {
	if a := e.answer(f.AnswerID); a != nil {
		switch {
		case a.Kind == "medication_use" || a.QuestionSource != nil && a.QuestionSource.Key == "medication_start":
			return "medication"
		case a.Kind == "reply" && backgroundTopics[0].matches(a.Question):
			return "medication"
		case a.Kind == "reply" && backgroundTopics[1].matches(a.Question):
			return "allergy"
		}
	}
	switch {
	case allergyConcept.MatchString(f.Concept):
		return "allergy"
	case medicineConcept.MatchString(f.Concept):
		return "medication"
	}
	return ""
}
