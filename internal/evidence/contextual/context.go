// Package contextual links a condition's educational sources to the patient's confirmed findings.
package contextual

import (
	"strings"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/evidence"
)

func Explain(cat evidence.Catalog, e *clinical.Encounter, index int, item clinical.Item) evidence.Support {
	facts := map[string]string{}
	for _, id := range item.Supporting {
		for _, f := range e.Facts {
			if f.ID != id || !f.Active || f.SourceOutdated || !f.Confirmed || f.Status != "reported_present" || f.Subject != "self" || f.SourceType != "patient_answer" {
				continue
			}
			// Unspecified timing uses the encounter's own positive report. Explicit
			// past, resolved, or unsupported time descriptions cannot ground the link.
			switch strings.ToLower(strings.TrimSpace(f.TimeContext)) {
			case "", "current", "currently", "ongoing", "today", "right now", "now", "present":
				facts[id] = f.Concept
			}
		}
	}
	return cat.Explain(index, item.Name, item.ClinicalTerm, e.Age, facts)
}
