package clinical

import (
	"slices"
	"strings"
)

var userReferences = strings.NewReplacer(
	"the patient's", "the user's", "the patient’s", "the user’s",
	"The patient's", "The user's", "The patient’s", "The user’s",
	"the patient ", "the user ", "The patient ", "The user ",
	"this patient ", "this user ", "This patient ", "This user ",
	"Patient reports", "User reports", "Patient-reported", "Self-reported",
)

func userReference(text string) string { return userReferences.Replace(text) }

// ForDisplay updates software-authored labels on older visits without changing
// the stored revision, original answers, quoted evidence, or manual edits.
func (e Encounter) ForDisplay() Encounter {
	e.Facts = slices.Clone(e.Facts)
	for i, f := range e.Facts {
		if f.SourceType == "patient_answer" && !f.Corrected {
			e.Facts[i].Concept = userReference(f.Concept)
		}
	}
	return e
}
