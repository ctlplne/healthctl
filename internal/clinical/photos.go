package clinical

import (
	"errors"
	"regexp"
	"slices"
	"time"
)

// Photos are patient-kept visual records. Neither image data nor attachment
// metadata is part of Input, the provider-facing history.
type ObservationPhoto struct {
	ID        string    `json:"id"`
	Area      string    `json:"area"`
	CreatedAt time.Time `json:"createdAt"`
}

func ValidPhotoArea(area string) bool {
	switch area {
	case "Eye", "Fingernails or hands", "Toenails or feet", "Other visible change":
		return true
	}
	return false
}

func (e *Encounter) AttachPhoto(id, area string) error {
	if !ValidPhotoArea(area) || id == "" || len(e.Photos) >= 12 || e.Pending != "" || e.ScopeReason != "" {
		return errors.New("Save an optional photo in an in-scope encounter once the current request has finished")
	}
	e.Photos = append(e.Photos, ObservationPhoto{ID: id, Area: area, CreatedAt: time.Now().UTC()})
	e.touch()
	return nil
}

var yellowEyeWords = regexp.MustCompile(`(?i)\b(?:(?:(?:whites? of (?:my |the )?eyes?)|(?:my )?eyes?|eye whites?) (?:have (?:turned|become)|(?:are|look|looks|appear|appears|seem|seems|turned|turning|became|becoming|went|get|getting))(?: (?:a (?:little|bit)|slightly|very|more|bright))? yellow(?:ish)?|yellow(?:ish)? (?:eyes?\b|eye whites?|whites? of (?:my |the )?eyes?)|jaundiced)\b`)
var eyeDenial = regexp.MustCompile(`(?i)\b(?:not|aren['’]t|weren['’]t|wasn['’]t|isn['’]t|never) (?:at all )?(?:look(?:ing)? )?yellow\b`)

// For a short, isolated nail complaint, the distribution is often the first
// useful distinction. Let the provider handle mixed concerns and free text.
var isolatedNailChange = regexp.MustCompile(`(?i)^(?:my |the )?(?:finger|toe)?nails? (?:have changed shape|look different|are changing shape|are spoon[- ]shaped|curve inward|are dented)[.!]?$`)

func (e *Encounter) askNailDistribution() bool {
	if e.Demo || !e.GuidedHistory || e.Focused || e.ReportFirst || e.Background != "" || !isolatedNailChange.MatchString(e.Concern) {
		return false
	}
	key, question, reason := "nail_distribution", "Which nails have changed?", "A change in one nail can have a different history from changes across several."
	choices := []string{"One nail", "Several nails"}
	for _, a := range e.Answers {
		if !a.Active || a.Kind == "screen" || a.Kind == "concern" {
			continue
		}
		if a.Kind != "reply" || a.QuestionSource == nil || a.QuestionSource.Key != "nail_distribution" {
			return false
		}
		switch a.Text {
		case "One nail":
			key, question, reason = "nail_local_event", "What happened to that finger or toe around the time the change began?", "An injury or repeated pressure may help explain why just one nail changed."
		case "Several nails":
			key, question, reason = "nail_sequence", "Did the nails start changing together, or did one change first?", "The sequence helps place the changes on a timeline."
		default:
			return false
		}
		choices = nil
	}
	e.State, e.Pending, e.Question, e.Choices = "interviewing", "", question, choices
	e.QuestionSource = &QuestionSource{Key: key, Pathway: "nail_change", Reason: reason}
	return true
}

func (e *Encounter) RemovePhoto(id string) error {
	for i, photo := range e.Photos {
		if photo.ID == id {
			e.Photos = slices.Delete(e.Photos, i, i+1)
			e.touch()
			return nil
		}
	}
	return errors.New("Choose a saved observation photo")
}
