package clinical

import (
	"regexp"
	"strings"
)

// Only a short, impersonal condition-level explanation can supplement the rationale
// generated from actual reviewed records. Arbitrary patient-specific model prose is
// intentionally absent from the persisted assessment and every rendering/export.
var symptomList = regexp.MustCompile(`^[\pL][\pL\pM ,'-]{0,170}\.?$`)
var embeddedClaim = regexp.MustCompile(`(?i)\b(you|your|their|they|he|she|his|her|patient|i|my|me|who|whose|which|because|given|since|when|reported|observed|confirmed|definite|known|history|actually|demonstrates?|shows?|suggests?|has|have|is|are|was|were|must|should|needs?|avoid|start|stop|take|seek|means?|proves?|rules?|ruled|therefore|so)\b`)

func generalEducation(item Item) string {
	text := strings.TrimSpace(item.Explanation)
	for _, name := range []string{"This condition", item.Name, item.ClinicalTerm} {
		if name == "" {
			continue
		}
		for _, verb := range []string{" can cause ", " may cause ", " can involve ", " may involve ", " can present with ", " may present with "} {
			prefix := name + verb
			if len(text) <= len(prefix) || !strings.EqualFold(text[:len(prefix)], prefix) {
				continue
			}
			list := text[len(prefix):]
			if symptomList.MatchString(list) && !embeddedClaim.MatchString(list) {
				return strings.TrimSuffix(text, ".") + "."
			}
		}
	}
	return ""
}

func reviewedRationale(item Item, facts map[string]Fact) string {
	sources := []string{}
	for _, fid := range item.Supporting {
		f := facts[fid]
		var label string
		switch f.Subject {
		case "self":
			label = "your report of "
		case "family":
			label = "the reported family history of "
		default:
			label = "the account about another person involving "
		}
		if f.Status == "explicitly_denied" {
			switch f.Subject {
			case "self":
				label = "your explicit denial of "
			case "family":
				label = "the explicit denial in the family history of "
			default:
				label = "the explicit denial about another person of "
			}
		}
		if f.SourceType == "report_entry" {
			label = "the reviewed report entry for "
			if f.Subject == "family" {
				label = "the reviewed report entry about your family for "
			}
		} else if f.Origin != nil {
			label = "the earlier finding you reconciled for "
			if f.Subject == "family" {
				label = "the reconciled family-history entry for "
			}
			if f.Status == "explicitly_denied" {
				label = "the reconciled earlier explicit denial of "
			}
			if f.Origin.ReconciliationStatus == "no_longer_applies" {
				label = "the earlier record you marked as no longer applying for "
			}
			if f.Origin.ReconciliationStatus == "changed" {
				label = "the earlier record you marked as changed for "
			}
		}
		sources = append(sources, label+"“"+f.Concept+"”")
		if len(sources) == 3 {
			break
		}
	}
	var citation string
	switch len(sources) {
	case 1:
		citation = sources[0]
	case 2:
		citation = strings.Join(sources, " and ")
	default:
		citation = strings.Join(sources[:len(sources)-1], ", ") + ", and " + sources[len(sources)-1]
	}
	explanation := "The model considered this possibility because of " + citation + "."
	if general := generalEducation(item); general != "" {
		explanation += " " + general
	}
	return explanation
}

func assessmentItems(in Input, p Proposal) []Item {
	facts := map[string]Fact{}
	for _, f := range in.Facts {
		facts[f.ID] = f
	}
	items := append([]Item{}, p.Items...)
	for i, item := range items {
		items[i].Explanation = reviewedRationale(item, facts)
	}
	return items
}
