package evaluation

import (
	"slices"
	"strings"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/evidence"
	"github.com/ctlplne/healthctl/internal/evidence/contextual"
)

type CitationAudit struct {
	Item        int      `json:"itemIndex"`
	Relation    string   `json:"relation"`
	Corpus      string   `json:"corpusVersion"`
	Passages    []string `json:"passageIds"`
	LinkedFacts []string `json:"linkedFactIds"`
}

// Check immutable passage references, source quotation boundaries, declared
// population, and supporting patient IDs independently of the retrieval score.
func auditAssessment(e *clinical.Encounter, a clinical.Assessment, failures *[]string) []CitationAudit {
	var audits []CitationAudit
	for idx, item := range a.Items {
		explanation := contextual.Explain(bundledIndex, e, idx, item)
		row := CitationAudit{Item: idx, Relation: explanation.Relation, Corpus: explanation.CorpusVersion, Passages: []string{}, LinkedFacts: []string{}}
		for _, p := range explanation.Passages {
			row.Passages = append(row.Passages, p.Collection+"/"+p.Version+"/"+p.ID)
		}
		if (explanation.Relation == "supported_context") != (len(explanation.ClaimLinks) > 0) {
			*failures = append(*failures, "citation relation inconsistent with exact claim links")
		}
		for _, link := range explanation.ClaimLinks {
			source := slices.ContainsFunc(explanation.Passages, func(passage evidence.Passage) bool {
				population := passage.Applicability
				return passage.Collection+"/"+passage.Version+"/"+passage.ID == link.PassageID && passage.Status == "active" && strings.Contains(passage.Text, link.Quote) && population != nil && population.Scope == "general_education" && population.MinAge <= e.Age && population.MaxAge >= e.Age
			})
			if !source {
				*failures = append(*failures, "quote lacks an applicable active immutable source")
			}
			for _, id := range link.FactIDs {
				valid := slices.ContainsFunc(e.Facts, func(f clinical.Fact) bool {
					return f.ID == id && f.Active && f.Confirmed && f.Subject == "self" && f.Status == "reported_present" && f.SourceType == "patient_answer" && !f.SourceOutdated
				})
				if !valid || !slices.Contains(item.Supporting, id) {
					*failures = append(*failures, "passage linked to unsupported patient evidence")
				}
				row.LinkedFacts = append(row.LinkedFacts, id)
			}
		}
		audits = append(audits, row)
	}
	return audits
}
