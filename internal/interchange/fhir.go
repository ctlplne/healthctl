package interchange

import (
	"crypto/sha256"
	"fmt"
	"html"
	"strings"
	"time"
)

type resource = map[string]any

func uuid(s string) string {
	h := sha256.Sum256([]byte(s))
	h[6] = h[6]&15 | 0x80
	h[8] = h[8]&63 | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[:4], h[4:6], h[6:8], h[8:10], h[10:16])
}
func ref(id string) resource { return resource{"reference": "urn:uuid:" + id} }
func narrative(lines []string) resource {
	var b strings.Builder
	b.WriteString(`<div xmlns="http://www.w3.org/1999/xhtml">`)
	for _, line := range lines {
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(line))
		b.WriteString("</p>")
	}
	b.WriteString("</div>")
	return resource{"status": "additional", "div": b.String()}
}
func section(title string, lines []string) resource {
	return resource{"title": title, "text": narrative(lines)}
}
func entry(id string, r resource) resource {
	return resource{"fullUrl": "urn:uuid:" + id, "resource": r}
}

// FHIR maps a patient-authored history to an R4 document Bundle. Local labels,
// verbatim qualifiers and institutional units stay text unless coding is verified.
func FHIR(b Brief) resource {
	key := fmt.Sprintf("%s/%d/%d/%s/%s", b.ID, b.HistoryRevision, b.AssessmentVersion, b.UpdatedAt.UTC().Format("20060102T150405.000000000Z"), b.ModelGuidance)
	docID, patID, deviceID, compID := uuid(key+"/document"), uuid(b.ID+"/patient"), uuid("clinical-history/software"), uuid(key+"/composition")
	when := b.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")
	profiles := func(name string) resource {
		return resource{"profile": []string{"http://hl7.org/fhir/StructureDefinition/" + name}}
	}
	subject, device := ref(patID), ref(deviceID)
	sections := []resource{section("Patient concern and review status", []string{b.Concern, b.Notice, b.ModelGuidance, fmt.Sprintf("Age %d at intake. History revision %d, patient confirmed: %t. %s", b.Age, b.HistoryRevision, b.Confirmed, b.ScopeReason)})}
	if b.Demo {
		sections = append(sections, section("Synthetic demonstration", []string{"Fictional case from the synthetic demo."}))
	}
	var lines []string
	for _, w := range b.Warnings {
		state := "Previously shown"
		if w.Active {
			state = "ACTIVE"
		}
		lines = append(lines, state+" — "+w.Message)
	}
	if len(lines) > 0 {
		sections = append(sections, section("Urgent warnings shown", lines))
	}
	lines = nil
	for _, f := range b.Facts {
		lines = append(lines, fmt.Sprintf("%s · %s · %s · %s · %s — %s", f.SourceType, f.Subject, f.Concept, f.Status, f.TimeContext, f.Quote))
	}
	for _, f := range b.Facts {
		if f.Origin != nil {
			lines = append(lines, fmt.Sprintf("Reconciliation %s · %s · %s · %s — %s", f.Origin.Date, f.Origin.Category, f.Concept, f.Origin.ReconciliationStatus, f.Origin.Note))
		}
	}
	if len(lines) > 0 {
		sections = append(sections, section("Reviewed and reported history", lines))
	}
	var observations, references []resource
	for _, r := range b.ReportFindings {
		title := r.Name
		if title == "" {
			title = "Report passage"
		}
		if r.Subject != "self" {
			sections = append(sections, section("Related-person report entry", []string{r.Subject + " · " + title + " — " + r.Quote + " · " + r.Locator}))
			continue
		}
		oid := uuid(key + "/review/" + r.ReviewID)
		details := fmt.Sprintf("%s — %s. Collection date as printed: %s. Issue date as printed: %s. Flag: %s. %s", r.Locator, r.Quote, r.CollectionDate, r.IssueDate, r.Flag, r.Note)
		o := resource{"resourceType": "Observation", "id": oid, "meta": profiles("Observation"), "status": "unknown", "code": resource{"text": title}, "subject": subject, "note": []resource{{"text": details}}}
		o["text"] = narrative([]string{title + " · " + strings.TrimSpace(r.Value+" "+r.Units), details})
		// The patient-facing report schema does not identify the laboratory performer.
		absent := func(reason string) resource {
			return resource{"extension": []resource{{"url": "http://hl7.org/fhir/StructureDefinition/data-absent-reason", "valueCode": reason}}}
		}
		performer := absent("unsupported")
		performer["display"] = "Performing service unavailable in this transcription."
		o["performer"] = []resource{performer}
		if _, err := time.Parse(time.DateOnly, r.CollectionDate); err == nil {
			o["effectiveDateTime"] = r.CollectionDate
		} else if r.CollectionDate != "" {
			o["_effectiveDateTime"] = absent("as-text")
		} else {
			o["_effectiveDateTime"] = absent("unknown")
		}
		if r.Value != "" {
			o["valueString"] = strings.TrimSpace(r.Value + " " + r.Units)
		}
		if r.ReferenceRange != "" {
			o["referenceRange"] = []resource{{"text": r.ReferenceRange}}
		}
		observations = append(observations, entry(oid, o))
		references = append(references, ref(oid))
	}
	if len(observations) > 0 {
		sections = append(sections, resource{"title": "Documented findings reviewed by the patient", "entry": references})
	}
	if len(b.Priorities) > 0 {
		sections = append(sections, section("Patient priorities", b.Priorities))
	}
	lines = append([]string{}, b.Outstanding...)
	for _, c := range b.Contradictions {
		lines = append(lines, "Unresolved discrepancy: "+c.Concept)
	}
	if len(lines) > 0 {
		sections = append(sections, section("Open questions", lines))
	}
	if b.Assessment != nil {
		lines = []string{fmt.Sprintf("Experimental. Assessment version %d. Model %s / %s. %s", b.AssessmentVersion, b.Assessment.Provider.Provider, b.Assessment.Provider.RequestedModel, b.Assessment.Outcome)}
		for _, item := range b.Assessment.Items {
			lines = append(lines, item.Name+" — "+item.Explanation)
		}
		lines = append(lines, b.Assessment.Outstanding...)
		sections = append(sections, section("AI-generated provisional differential", lines))
	}
	lines = nil
	for _, o := range b.Outcomes {
		lines = append(lines, o.Date+" · "+o.Source+" · "+o.Text)
	}
	if len(lines) > 0 {
		sections = append(sections, section("Visit follow-up", lines))
	}
	lossID := uuid(key + "/loss")
	losses := []string{"Source files, full transcripts, corrections and older assessments are available in the owner workspace. This document carries the current handoff.", "Patient statements and experimental possibilities are narrative. No diagnostic codes or confirmed conditions are assigned. Reported values, dates and units retain their original spelling; cross-laboratory conversion is not provided.", "This subset targets base FHIR R4 document resources. IPS and US Core profiles are outside its mapped subset."}
	issues := []resource{}
	for _, s := range losses {
		issues = append(issues, resource{"severity": "information", "code": "informational", "details": resource{"text": s}})
	}
	sections = append(sections, resource{"title": "Mapping and omissions", "text": narrative(losses), "entry": []resource{ref(lossID)}})
	summary := []string{b.Concern, b.Notice}
	for _, w := range b.Warnings {
		if w.Active {
			summary = append(summary, w.Message)
		}
	}
	summary = append(summary, b.ModelGuidance)
	composition := resource{
		"resourceType": "Composition", "id": compID, "meta": profiles("Composition"),
		"status": "preliminary", "type": resource{"text": "Patient-prepared visit brief"},
		"subject": subject, "author": []resource{device}, "date": when,
		"title": "Patient-prepared clinical history", "section": sections,
		"text": narrative(summary),
	}
	patient := resource{
		"resourceType": "Patient", "id": patID, "meta": profiles("Patient"),
		"text": narrative([]string{fmt.Sprintf("Patient in the self-hosted encounter, age %d at intake.", b.Age)}),
	}
	software := resource{
		"resourceType": "Device", "id": deviceID, "meta": profiles("Device"),
		"text":       narrative([]string{"healthctl assembled the patient-prepared visit brief."}),
		"deviceName": []resource{{"name": "healthctl", "type": "user-friendly-name"}},
	}
	mapping := resource{
		"resourceType": "OperationOutcome", "id": lossID, "meta": profiles("OperationOutcome"),
		"text": narrative(losses), "issue": issues,
	}
	entries := []resource{entry(compID, composition), entry(patID, patient), entry(deviceID, software)}
	entries = append(entries, observations...)
	entries = append(entries, entry(lossID, mapping))
	return resource{
		"resourceType": "Bundle", "id": docID, "meta": profiles("Bundle"),
		"identifier": resource{"system": "urn:ietf:rfc:3986", "value": "urn:uuid:" + docID},
		"type":       "document", "timestamp": when, "entry": entries,
	}
}
