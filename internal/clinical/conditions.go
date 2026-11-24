package clinical

import (
	"strings"
)

// The app's list of condition names. An assessment item whose name is on the
// list is shown with the app's name, clinical term and description. Any other
// name is kept, but labeled as the model's own suggestion. Descriptions are
// development content: none of them has been clinically reviewed yet, so each
// one says so. "Don't miss" lines per pathway are development content too.
const ConditionListVersion = "development-1"

type condition struct {
	Name        string
	Term        string
	Synonyms    []string
	Description string
}

var conditionList = []condition{
	{"Common cold", "Viral upper respiratory infection", []string{"cold", "upper respiratory infection", "viral uri", "urti", "viral infection"}, "A common viral infection of the nose and throat."},
	{"Nasal allergies", "Allergic rhinitis", []string{"hay fever", "seasonal allergies"}, "Sneezing, a runny or blocked nose and itchy eyes caused by an allergy."},
	{"Influenza", "Influenza", []string{"flu"}, "A viral infection that often comes on quickly with fever, aches and tiredness."},
	{"COVID-19", "COVID-19", []string{"covid", "coronavirus infection", "sars-cov-2 infection"}, "A viral infection that can affect the nose, throat and lungs."},
	{"Acute bronchitis", "Acute bronchitis", []string{"chest cold"}, "Inflammation of the airways, usually after a cold, with a cough that can last weeks."},
	{"Pneumonia", "Pneumonia", []string{"chest infection", "lung infection"}, "An infection of the lungs that can cause cough, fever and breathlessness."},
	{"Asthma", "Asthma", []string{"asthma flare", "asthma attack"}, "A long-term condition in which the airways narrow, causing wheeze, cough or breathlessness."},
	{"Sinusitis", "Sinusitis", []string{"sinus infection"}, "Inflammation of the sinuses that can cause facial pressure and a blocked nose."},
	{"Sore throat from a virus", "Viral pharyngitis", []string{"pharyngitis", "viral sore throat"}, "A sore throat caused by a viral infection."},
	{"Strep throat", "Streptococcal pharyngitis", []string{"streptococcal throat infection"}, "A bacterial throat infection."},
	{"Tension-type headache", "Tension-type headache", []string{"tension headache"}, "A common headache often felt as pressure or tightness around the head."},
	{"Migraine", "Migraine", []string{"migraine headache"}, "Recurring headaches that can come with nausea or sensitivity to light."},
	{"Medication-overuse headache", "Medication-overuse headache", []string{"rebound headache"}, "Headaches that come back because pain relief is taken very often."},
	{"Subarachnoid hemorrhage", "Subarachnoid hemorrhage", []string{"brain bleed", "subarachnoid haemorrhage"}, "Bleeding around the brain, which needs emergency care."},
	{"Meningitis", "Meningitis", []string{"meningitis infection"}, "An infection of the lining around the brain, which needs emergency care."},
	{"Stroke", "Stroke", []string{"transient ischemic attack", "tia", "mini-stroke"}, "Interrupted blood supply to part of the brain, which needs emergency care."},
	{"Inner ear problem", "Vestibular disorder", []string{"benign paroxysmal positional vertigo", "bppv", "labyrinthitis", "vestibular neuritis"}, "A problem with the balance organ in the inner ear that can cause spinning dizziness."},
	{"Low blood pressure on standing", "Orthostatic hypotension", []string{"postural hypotension"}, "A drop in blood pressure when standing up that can cause lightheadedness."},
	{"Dehydration", "Dehydration", []string{"fluid loss"}, "Not having enough fluid in the body."},
	{"Stomach bug", "Viral gastroenteritis", []string{"gastroenteritis", "stomach flu", "gastro"}, "An infection of the gut that causes diarrhea, vomiting or both."},
	{"Food poisoning", "Foodborne illness", []string{"food-borne illness"}, "Illness from contaminated food or drink."},
	{"Irritable bowel syndrome", "Irritable bowel syndrome", []string{"ibs"}, "A long-term condition that causes stomach pain and changes in bowel habits."},
	{"Acid reflux", "Gastroesophageal reflux disease", []string{"gerd", "reflux", "heartburn"}, "Stomach acid coming back up into the food pipe."},
	{"Appendicitis", "Appendicitis", []string{"inflamed appendix"}, "Inflammation of the appendix, which may need urgent surgery."},
	{"Gallstones", "Cholelithiasis", []string{"gallbladder attack", "biliary colic", "cholecystitis"}, "Stones in the gallbladder that can cause pain in the upper right abdomen."},
	{"Bowel obstruction", "Intestinal obstruction", []string{"blocked bowel"}, "A blockage of the bowel, which needs urgent care."},
	{"Bleeding in the digestive tract", "Gastrointestinal bleeding", []string{"gi bleed", "gastrointestinal hemorrhage"}, "Bleeding anywhere in the gut, which needs urgent assessment."},
	{"Bladder infection", "Cystitis", []string{"urinary tract infection", "uti"}, "An infection of the bladder that can cause burning and frequent urination."},
	{"Urinary irritation", "", []string{"bladder irritation"}, "Irritation of the bladder or urethra without a confirmed infection."},
	{"Kidney infection", "Pyelonephritis", []string{"kidney infection"}, "An infection of the kidney, which needs prompt treatment."},
	{"Liver or bile duct problem", "Hepatobiliary disease", []string{"hepatitis", "liver disease", "bile duct blockage", "jaundice"}, "A problem with the liver or bile ducts that can cause yellow skin or eyes."},
	{"Iron deficiency", "Iron deficiency anemia", []string{"iron deficiency anaemia", "anemia", "anaemia"}, "Too little iron in the body, which can cause tiredness and nail changes."},
	{"Nail injury", "Nail trauma", []string{"trauma to the nail", "injured nail"}, "A change in a nail after an injury."},
	{"Fungal nail infection", "Onychomycosis", []string{"nail fungus"}, "A fungal infection that can thicken or discolor nails."},
	{"Blood clot in the lung", "Pulmonary embolism", []string{"pe", "pulmonary embolus"}, "A blood clot in the lungs, which needs emergency care."},
	{"Heart problem", "Cardiac cause", []string{"heart attack", "angina", "acute coronary syndrome", "arrhythmia"}, "A problem with the heart, which can need emergency care."},
}

func normalName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.Trim(s, " .,"))), " ")
}

func findCondition(names ...string) (condition, bool) {
	for _, n := range names {
		n = normalName(n)
		if n == "" {
			continue
		}
		for _, c := range conditionList {
			if n == normalName(c.Name) || c.Term != "" && n == normalName(c.Term) {
				return c, true
			}
			for _, s := range c.Synonyms {
				if n == normalName(s) {
					return c, true
				}
			}
		}
	}
	return condition{}, false
}

// renderItems applies the app's names and descriptions. The rationale is
// already built from the cited facts by reviewedRationale.
func renderItems(items []Item) []Item {
	for i := range items {
		c, ok := findCondition(items[i].Name, items[i].ClinicalTerm)
		if !ok {
			items[i].Vetted = false
			items[i].Description = ""
			items[i].DescriptionStatus = ""
			continue
		}
		items[i].Vetted = true
		items[i].Name, items[i].ClinicalTerm = c.Name, c.Term
		items[i].Description = c.Description
		items[i].DescriptionStatus = "development, not clinically reviewed"
	}
	return items
}

// Don't-miss conditions for each pathway. If an assessment doesn't include
// one, the app adds a fixed line asking the user to raise it with a
// clinician. It is never presented as a diagnosis.
var dontMiss = map[string][]struct{ condition, line string }{
	"respiratory": {
		{"Pneumonia", "Ask your clinician whether a chest infection such as pneumonia needs to be checked for, especially if you feel very unwell or breathless."},
		{"Blood clot in the lung", "Ask your clinician about a blood clot in the lung if you have sudden breathlessness or chest pain."},
	},
	"headache": {
		{"Subarachnoid hemorrhage", "A sudden, severe headache can be a sign of bleeding around the brain. Ask your clinician about it, and seek emergency care if it happens."},
		{"Meningitis", "Ask your clinician about meningitis if the headache comes with fever, a stiff neck or a rash."},
	},
	"abdominal": {
		{"Appendicitis", "Ask your clinician whether appendicitis needs to be checked for, especially if the pain moves to the lower right side or gets worse."},
		{"Bowel obstruction", "Ask your clinician about a blocked bowel if you are vomiting and can't pass stool or wind."},
	},
	"dizziness": {
		{"Stroke", "Dizziness with weakness, numbness, slurred speech or double vision can be a stroke. Ask your clinician about it, and seek emergency care if it happens."},
		{"Heart problem", "Ask your clinician whether a heart rhythm problem could cause the dizziness, especially if you fainted or felt your heart racing."},
	},
	"bowel_change": {
		{"Bleeding in the digestive tract", "Ask your clinician about bleeding in the gut if you see blood or black stools."},
	},
	"eye_appearance": {
		{"Liver or bile duct problem", "Yellow eyes or skin can come from the liver or bile ducts. Ask your clinician about it promptly."},
	},
}

// dontMissLines lists the app's fixed lines for dangerous causes the
// assessment doesn't include.
func dontMissLines(pathways []Pathway, items []Item) []string {
	var out []string
	for _, p := range pathways {
		for _, d := range dontMiss[p.ID] {
			present := false
			for _, it := range items {
				if c, ok := findCondition(it.Name, it.ClinicalTerm); ok && c.Name == d.condition {
					present = true
				}
			}
			if !present && !contains(out, d.line) {
				out = append(out, d.line)
			}
		}
	}
	return out
}

// MarkUnstable compares two runs of the same assessment. A possibility that
// appears in only one run is marked unstable and kept.
func MarkUnstable(first, second []Item) []Item {
	key := func(it Item) string {
		if c, ok := findCondition(it.Name, it.ClinicalTerm); ok {
			return c.Name
		}
		return normalName(it.Name)
	}
	other := map[string]bool{}
	for _, it := range second {
		other[key(it)] = true
	}
	out := append([]Item{}, first...)
	for i := range out {
		out[i].Unstable = !other[key(out[i])]
	}
	return out
}
