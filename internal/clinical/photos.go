package clinical

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Photos are user-kept visual records. Neither image data nor attachment
// metadata is part of Input, the provider-facing history.
type ObservationPhoto struct {
	ID        string    `json:"id"`
	Area      string    `json:"area"`
	CreatedAt time.Time `json:"createdAt"`
}

func ValidPhotoArea(area string) bool {
	return validText(area, 160) && strings.TrimSpace(area) == area &&
		strings.IndexFunc(area, func(r rune) bool { return unicode.IsControl(r) || !unicode.IsPrint(r) }) == -1
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
