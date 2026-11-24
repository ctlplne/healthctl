package server

import (
	"errors"
	"strings"
	"unicode"
)

// ValidateRegion checks the emergency contact and care destination text an
// installation shows in every warning and export, so a typo can't put a
// broken or misleading instruction in front of the user.
func ValidateRegion(emergency, careDestination string) error {
	for name, value := range map[string]string{"CLINICAL_EMERGENCY_CONTACT": emergency, "CLINICAL_CARE_DESTINATION": careDestination} {
		if len(value) > 240 {
			return errors.New(name + " must be at most 240 characters")
		}
		for _, r := range value {
			if unicode.IsControl(r) {
				return errors.New(name + " must be one line of plain text")
			}
		}
		lower := strings.ToLower(value)
		if strings.Contains(lower, "<") || strings.Contains(lower, "javascript:") {
			return errors.New(name + " must be plain text, not markup")
		}
	}
	if strings.TrimSpace(emergency) != "" && !strings.ContainsAny(emergency, "0123456789") {
		return errors.New("CLINICAL_EMERGENCY_CONTACT should include the local emergency number, for example \"Call 911\"")
	}
	return nil
}

// installationSettings are the regional settings in effect, included in
// every export so a reader knows which emergency instructions applied.
func (s *Server) installationSettings() map[string]string {
	return map[string]string{"locale": s.Locale, "units": s.Units, "emergencyContact": s.Emergency, "careDestination": s.CareDestination}
}
