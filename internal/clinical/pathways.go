package clinical

import (
	"math/big"
	"regexp"
	"slices"
	"strings"
)

const PathwayVersion = "adult-development-v3"

type pathwayDefinition struct {
	id      string
	concern *regexp.Regexp
	context []string
}

var pathways = []pathwayDefinition{
	{"nail_change", regexp.MustCompile(`(?i)\b(fingernails?|toenails?|nails?|koilonychia)\b`), []string{"Invite the patient's description of the change before naming a shape. Ask which nails, when the change began, and how it has progressed. Use the distribution to choose the next question: local injury or exposure for one nail, shared timing or other changes for several. Never infer anemia or a nail sign from a photo."}},
	{"eye_appearance", regexp.MustCompile(`(?i)\b(eye whites?|whites? of (?:my |the )?eyes?|yellow eyes?|jaundice)\b`), []string{"Clarify which part of the eye looks different, when it changed and associated symptoms if these affect urgency. A patient description or a photo does not confirm or exclude jaundice. Never delay urgent care to collect an image."}},
	{"bowel_change", regexp.MustCompile(`(?i)\b(loose stools?|stools?|diarrh(?:ea|oea)|bowel movements?)\b`), []string{"Start with the patient's usual bowel pattern, then establish what changed in consistency, frequency, timing, and daily function. If travel preceded the change, establish the order and ask about relevant food, water, sick contacts, or medicines one at a time. Clarify reported blood, pain or difficulty keeping fluids down when they could change urgency."}},
	{"respiratory", regexp.MustCompile(`(?i)\b(cough|coughing|breathless|breathing|wheez(?:e|ing)|sore throat|runny nose|nose is runny|sneez(?:e|ing))\b`), []string{"Clarify onset, progression, breathlessness and relevant exposure when these could change the estimate.", "Consider respiratory limitations and urgent breathing concerns separately from likely explanations."}},
	{"headache", regexp.MustCompile(`(?i)\b(headach(?:e|es)|head pain|migraine)\b`), []string{"Establish whether onset was sudden, how this differs from prior episodes and any associated changes.", "Ask about recent medication changes or injury if relevant. Sudden severe onset may need an immediate interruption."}},
	{"abdominal", regexp.MustCompile(`(?i)\b(abdomen|abdominal|belly|stomach|flank|pelvic)\b`), []string{"Clarify location, onset, progression and associated digestive or urinary changes.", "Review recent medication changes, exposures and relevant prior episodes without guessing pregnancy status."}},
	{"dizziness", regexp.MustCompile(`(?i)\b(dizz(?:y|iness)|vertigo|lightheaded(?:ness)?|light-headed|off.balance)\b`), []string{"Clarify what dizziness feels like, its timing, triggers, fainting or any new neurological change.", "Ask about recent medication changes if they could affect urgency or possible causes."}},
}

func (e *Encounter) activePathways() []Pathway {
	if e.ScopeReason != "" {
		return nil
	}
	return matchPathways(e.Answers)
}
func matchPathways(answers []Answer) []Pathway {
	var out []Pathway
	for _, p := range pathways {
		found := false
		for _, a := range answers {
			if !a.Active || a.SourceOutdated || a.Kind == "screen" || pastEvent(a) {
				continue
			}
			for _, span := range p.concern.FindAllStringIndex(a.Text, -1) {
				if contextualRisk(clauseAt(a.Text, span[0])) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if found {
			out = append(out, Pathway{ID: p.id, Version: PathwayVersion, ReviewStatus: "unvalidated_development", Context: p.context})
		}
	}
	return out
}

var sampleTiming = regexp.MustCompile(`(?i)\b(before|after|during|since|prior to|same (?:time|day) as)\b.{0,55}\b(sample|collection|specimen|test|urine|blood draw|swab|report)\b|\b(sample|specimen|test|draw)\b.{0,55}\b(before|after)\b.{0,45}\b(symptom|started|began)\b`)
var datedOnset = regexp.MustCompile(`(?i)\b(started|began|onset|since)\b.{0,64}\b[0-9]{4}-[0-9]{2}-[0-9]{2}\b`)
var urinaryConcern = regexp.MustCompile(`(?i)\b(urinary|urination|urinate|urinating|urine|pee|peeing|bladder|dysuria|flank)\b`)
var urineRow = regexp.MustCompile(`(?i)\b(urine|urinalysis|nitrit(?:e|es)|leukocyte esterase|urinary|u/a)\b`)
var respiratoryRow = regexp.MustCompile(`(?i)\b(swab|respiratory|influenza|covid|sars.cov|rsv|sputum)\b`)
var disagreement = regexp.MustCompile(`(?i)\b(disagree|dispute|does not match|different from|conflict|not what I remember)\b`)
var noSymptoms = regexp.MustCompile(`(?i)\b(no symptoms|without symptoms|asymptomatic|routine screening|routine checkup)\b`)

func (e *Encounter) alreadyAnswered(key, review string) bool {
	for _, a := range e.Answers {
		q := a.QuestionSource
		if a.Active && !a.SourceOutdated && q != nil && q.Key == key && q.ReviewID == review {
			return true
		}
	}
	return false
}

func (e *Encounter) timingKnown(row ReportReview) bool {
	for _, a := range e.Answers {
		if !a.Active || a.SourceOutdated || a.Kind == "screen" || pastEvent(a) {
			continue
		}
		q := a.QuestionSource
		if q != nil && q.ReviewID != "" {
			if q.ReviewID != row.ID {
				sameSample := false
				for _, old := range e.ReportReviews {
					if old.ID == q.ReviewID && old.DocumentID == row.DocumentID && old.Row.CollectionDate == row.Row.CollectionDate {
						sameSample = true
						break
					}
				}
				if !sameSample {
					continue
				}
			}
			// A decline or "not sure" applies to the same sample too.
			if q.Key == "sample_timing" {
				return true
			}
		}
		if sampleTiming.MatchString(a.Text) || datedOnset.MatchString(a.Text) {
			return true
		}
	}
	return false
}

func (e *Encounter) groundedQuestion() bool {
	if e.Demo && !e.ReportFirst && len(e.ReportReviews) == 0 {
		return false
	}
	for i := len(e.ReportReviews) - 1; i >= 0; i-- {
		r := e.ReportReviews[i]
		if !r.Active || r.Row.Subject != "self" {
			continue
		}
		d := e.document(r.DocumentID)
		if d == nil || d.BelongsTo != "self" {
			continue
		}
		q := QuestionSource{DocumentID: d.ID, RowID: r.RowID, ReviewID: r.ID}
		var question string
		switch {
		case (disagreement.MatchString(r.Row.Note) || e.conflictingRecollection(r) != "") && !e.alreadyAnswered("discrepancy", r.ID):
			q.Key = "discrepancy"
			q.Reason = "The reviewed entry and your account may differ. Clarifying the difference helps preserve both accounts."
			question = "How does your recollection differ from this report entry?"
		case r.Row.Uncertain && !e.alreadyAnswered("transcription", r.ID):
			q.Key = "transcription"
			q.Reason = "You marked this transcription uncertain. The exact wording could change its interpretation."
			question = "Which part of this report entry is uncertain to you?"
		case r.Row.CollectionDate != "" && !e.alreadyAnswered("sample_timing", r.ID) && !e.timingKnown(r) && !noSymptoms.MatchString(e.Concern+" "+d.Purpose):
			relevant := urinaryConcern.MatchString(e.Concern+" "+d.Purpose) && urineRow.MatchString(d.Kind+" "+d.Title+" "+r.Row.Name+" "+r.Row.Excerpt)
			if !relevant && respiratoryRow.MatchString(d.Kind+" "+r.Row.Name+" "+r.Row.Excerpt) {
				relevant = slices.ContainsFunc(e.Pathways, func(p Pathway) bool { return p.ID == "respiratory" })
			}
			if !relevant {
				continue
			}
			q.Key = "sample_timing"
			q.Reason = "The report gives a collection date. The timing of your symptoms relative to this sample is still unclear."
			question = "Did your symptoms start before or after the sample for this report entry was collected?"
		default:
			continue
		}
		if len(e.Pathways) > 0 {
			q.Pathway = e.Pathways[0].ID
		}
		e.QuestionSource = &q
		e.QuestionReviewIDs = []string{r.ID}
		e.State = "interviewing"
		e.Question = question
		e.Pending = ""
		e.Choices = []string{"Not sure", "Prefer not to answer"}
		if q.Key == "sample_timing" {
			e.Choices = []string{"Before the sample", "After the sample", "I had no symptoms", "Not sure", "Prefer not to answer"}
		}
		return true
	}
	return false
}

func (e *Encounter) updateReportConflicts() {
	for _, r := range e.ReportReviews {
		if !r.Active || r.Row.Subject != "self" {
			continue
		}
		fid := e.conflictingRecollection(r)
		if fid != "" {
			e.Contradictions = append(e.Contradictions, Contradiction{Concept: r.Row.Name + " — the recollection and report differ", FactIDs: []string{fid, r.FactID}})
		} else if disagreement.MatchString(r.Row.Note) {
			e.Contradictions = append(e.Contradictions, Contradiction{Concept: r.Row.Name + " — the patient disputes the report", FactIDs: []string{r.FactID}})
		}
	}
}

// Compare explicit values for the same identified test and collection day. A
// similarly named measurement from another day is not a contradiction.
func (e *Encounter) conflictingRecollection(r ReportReview) string {
	if r.Row.Value == "" || r.Row.CollectionDate == "" || r.Row.Uncertain {
		return ""
	}
	for _, f := range e.Facts {
		if !f.Active || f.SourceType == "report_entry" || f.Subject != r.Row.Subject || f.Value == "" || f.Status != "reported_present" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(f.Concept), strings.TrimSpace(r.Row.Name)) ||
			!strings.EqualFold(strings.TrimSpace(f.Units), strings.TrimSpace(r.Row.Units)) ||
			f.TimeContext != r.Row.CollectionDate {
			continue
		}
		if !samePrintedValue(f.Value, r.Row.Value) {
			return f.ID
		}
	}
	return ""
}

func samePrintedValue(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if strings.EqualFold(a, b) {
		return true
	}
	for _, qualifier := range []string{"<=", ">=", "<", ">", "≤", "≥"} {
		if strings.HasPrefix(a, qualifier) != strings.HasPrefix(b, qualifier) {
			return false
		}
		if strings.HasPrefix(a, qualifier) {
			a, b = strings.TrimSpace(strings.TrimPrefix(a, qualifier)), strings.TrimSpace(strings.TrimPrefix(b, qualifier))
			break
		}
	}
	x, ok := new(big.Rat).SetString(a)
	if !ok {
		return false
	}
	y, ok := new(big.Rat).SetString(b)
	return ok && x.Cmp(y) == 0
}
