package ehr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ctlplne/healthctl/internal/clinical"
)

type obsMap = map[string]any

const testBase = "https://hospital.example.invalid/r4"
const testPatient = "SYNTHETIC-SUBJECT-41"

func cod(system, code, display string) obsMap {
	return obsMap{"coding": []obsMap{{"system": system, "code": code, "display": display}}}
}
func sampleObs(id string) obsMap {
	return obsMap{
		"resourceType": "Observation", "id": id, "meta": obsMap{"versionId": "v3"}, "status": "corrected",
		"category":          []obsMap{cod(categorySystem, "laboratory", "Laboratory")},
		"code":              cod("http://loinc.org", "718-7", "Hemoglobin"),
		"subject":           obsMap{"reference": "Patient/" + testPatient, "display": "Synthetic Private Patient", "identifier": obsMap{"value": "MRN-SECRET-991"}},
		"effectiveDateTime": "2026-11-19T06:17:20+05:30", "issued": "2026-11-21T11:45:22Z",
		"valueQuantity":  obsMap{"value": json.Number("2.010"), "comparator": "<", "unit": "g/dL", "system": "http://unitsofmeasure.org", "code": "g/dL"},
		"interpretation": []obsMap{cod("http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation", "LL", "Critically low")},
	}
}
func searchRaw(t *testing.T, obs ...obsMap) []byte {
	t.Helper()
	entries := []obsMap{}
	for _, r := range obs {
		entries = append(entries, obsMap{"resource": r})
	}
	b, err := json.Marshal(obsMap{"resourceType": "Bundle", "type": "searchset", "entry": entries})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func parseObs(t *testing.T, obs ...obsMap) Preview {
	t.Helper()
	p, err := ParseObservations(searchRaw(t, obs...), testPatient, testBase)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadLabPreservesQuantitiesDatesStatusAndRangeQualifiers(t *testing.T) {
	o := sampleObs("lab-1")
	o["referenceRange"] = []obsMap{
		{"low": obsMap{"value": json.Number("4.00"), "unit": "g/dL"}, "high": obsMap{"value": json.Number("9.0"), "unit": "g/dL"}, "type": obsMap{"text": "Treatment monitoring"}, "appliesTo": []obsMap{{"text": "Adults"}}, "age": obsMap{"low": obsMap{"value": 18, "unit": "a"}}},
		{"text": "Consult the specimen-specific interval"},
	}
	o["method"] = cod("http://loinc.org", "method", "Measured assay")
	o["note"] = []obsMap{{"authorString": "PRIVATE-AUTHOR", "text": "Interference from hemolysis is possible."}}
	p := parseObs(t, o)
	if len(p.Rows) != 1 || p.Skipped != 0 {
		t.Fatalf("lost observation: %+v", p)
	}
	r := p.Rows[0]
	if r.Label != "Hemoglobin" || r.Value != "<2.010" || r.Unit != "g/dL" || r.Flag != "LL" || !r.Critical || r.Status != "corrected" || r.Version != "v3" || r.Date != "2026-11-19T06:17:20+05:30" || r.Issued != "2026-11-21T11:45:22Z" || r.Source != "Observation/lab-1" {
		t.Fatalf("changed clinical data: %+v", r)
	}
	for _, s := range []string{"low 4.00 g/dL", "high 9.0 g/dL", "Treatment monitoring", "Adults", "age low 18 a", "specimen-specific"} {
		if !strings.Contains(r.ReferenceRange, s) {
			t.Errorf("missing %q in %q", s, r.ReferenceRange)
		}
	}
	desc := strings.Join(r.Context, " ")
	for _, s := range []string{"Measured assay", "hemolysis", "UCUM unit: g/dL", "LOINC 718-7", "Critically low"} {
		if !strings.Contains(desc, s) {
			t.Errorf("lost qualifier: %s", s)
		}
	}
	if len(r.Codings) == 0 || len(r.Key) != 36 {
		t.Fatal("missing source coding or opaque row key")
	}
}

func TestPinnedIdentityStatusOriginAndLaboratoryBoundary(t *testing.T) {
	var resources []obsMap
	wants := map[string]int{}
	add := func(reason string, modify func(obsMap)) {
		o := sampleObs(fmt.Sprintf("r%d", len(resources)))
		modify(o)
		resources = append(resources, o)
		if reason != "" {
			wants[reason]++
		}
	}
	add("", func(o obsMap) { o["subject"] = obsMap{"reference": testBase + "/Patient/" + testPatient} })
	for _, ref := range []string{"Patient/other", "https://foreign.invalid/r4/Patient/" + testPatient, "/Patient/" + testPatient, "Patient/" + testPatient + "/_history/1", "Patient/" + testPatient + "?x", "urn:uuid:" + testPatient} {
		add("patient_context", func(o obsMap) { o["subject"] = obsMap{"reference": ref} })
	}
	add("patient_context", func(o obsMap) { o["subject"] = obsMap{"identifier": obsMap{"value": testPatient}} })
	add("patient_context", func(o obsMap) { o["subject"] = obsMap{"reference": "Patient/" + testPatient, "type": "Device"} })
	for _, status := range []string{"registered", "preliminary", "cancelled", "entered-in-error", "unknown", "pending", ""} {
		add("ineligible_status", func(o obsMap) { o["status"] = status })
	}
	for _, s := range []string{"amended", "final"} {
		add("", func(o obsMap) { o["status"] = s })
	}
	add("non_laboratory", func(o obsMap) { o["category"] = []obsMap{cod("http://else.invalid", "laboratory", "lab")} })
	add("unsupported_context", func(o obsMap) { o["focus"] = []obsMap{{"reference": "Patient/child"}} })
	add("unsupported_context", func(o obsMap) { o["modifierExtension"] = []obsMap{{"valueString": "unknown meaning"}} })
	add("unsupported_context", func(o obsMap) { o["valueQuantity"].(obsMap)["extension"] = []obsMap{{"url": "unknown"}} })
	add("unsupported_access_or_subset", func(o obsMap) { o["meta"] = obsMap{"tag": []obsMap{{"code": "SUBSETTED"}}} })
	p := parseObs(t, resources...)
	if len(p.Rows) != 3 || p.Skipped != len(resources)-3 {
		t.Fatalf("unexpected acceptance: %d rows, %+v", len(p.Rows), p.SkippedByReason)
	}
	for reason, n := range wants {
		if p.SkippedByReason[reason] != n {
			t.Errorf("reason %s count %d, want %d", reason, p.SkippedByReason[reason], n)
		}
	}
	o := sampleObs("foo")
	b, _ := json.Marshal(obsMap{"resourceType": "Bundle", "type": "searchset", "entry": []obsMap{{"resource": o, "fullUrl": "https://foreign.invalid/Observation/foo"}}})
	p, err := ParseObservations(b, testPatient, testBase)
	if err != nil || p.SkippedByReason["unmatched_resource_origin"] != 1 {
		t.Fatal("accepted resource from another base")
	}
}

func TestParserScalarAbsentTypesAndExplicitSkippedCases(t *testing.T) {
	vals := []struct {
		k    string
		v    any
		want string
	}{
		{"valueString", "Trace", "Trace"}, {"valueBoolean", false, "false"}, {"valueBoolean", true, "true"}, {"valueInteger", 0, "0"},
		{"valueCodeableConcept", cod("http://snomed.info/sct", "260385009", "Negative"), "Negative"},
		{"dataAbsentReason", obsMap{"text": "Specimen inadequate"}, "Data absent: Specimen inadequate"},
		{"valueRatio", obsMap{"numerator": 1}, ""}, {"valueQuantity", obsMap{"value": 12, "comparator": "approximately"}, ""}, {"valueString", "See https://outside.invalid/report", ""},
		{"valueString", "<img src=x>", ""}, {"valueString", "MRN-SECRET-991", ""}, {"valueString", "Synthetic Private Patient", ""},
	}
	for i, tc := range vals {
		o := sampleObs(fmt.Sprintf("v%d", i))
		delete(o, "valueQuantity")
		o[tc.k] = tc.v
		p := parseObs(t, o)
		if tc.want == "" {
			if p.Skipped != 1 || len(p.Rows) != 0 {
				t.Fatalf("unsafe/unsupported %s accepted", tc.k)
			}
			continue
		}
		if len(p.Rows) != 1 || p.Rows[0].Value != tc.want || p.Rows[0].Absent != (tc.k == "dataAbsentReason") {
			t.Fatalf("incorrect value for %s: %+v", tc.k, p)
		}
	}
	for _, mut := range []func(obsMap){func(o obsMap) { o["valueString"] = "also present" }, func(o obsMap) { o["dataAbsentReason"] = obsMap{"text": "Unknown"} }, func(o obsMap) { o["note"] = []obsMap{{"text": "Visit https://outside.invalid/sensitive."}} }, func(o obsMap) { o["effectivePeriod"] = obsMap{"start": "2026-11-19"} }} {
		o := sampleObs("ambiguous")
		mut(o)
		p := parseObs(t, o)
		if len(p.Rows) > 0 || p.Skipped == 0 {
			t.Fatal("ambiguous/unsafe qualifier was silently discarded")
		}
	}
}

func TestPanelsRetainContextWithoutApplyingParentRangeOrFlagToComponent(t *testing.T) {
	o := sampleObs("panel")
	o["code"] = obsMap{"text": "Chemistry panel"}
	delete(o, "valueQuantity")
	o["referenceRange"] = []obsMap{{"text": "Parent reference"}}
	o["component"] = []obsMap{
		{"code": obsMap{"text": "Analyte A"}, "valueQuantity": obsMap{"value": json.Number("0.040"), "comparator": ">=", "unit": "mmol/L"}, "referenceRange": []obsMap{{"text": "Child range"}}, "interpretation": []obsMap{cod("http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation", "H", "High")}},
		{"code": obsMap{"text": "Analyte B"}, "dataAbsentReason": obsMap{"text": "Not performed"}},
		{"code": obsMap{"text": "Unselected private marker"}, "valueSampledData": obsMap{"data": "1 2 3"}},
	}
	p := parseObs(t, o)
	if len(p.Rows) != 2 || p.SkippedByReason["unsupported_component"] != 1 {
		t.Fatalf("unexpected panel rows %+v", p)
	}
	a, b := p.Rows[0], p.Rows[1]
	if a.Component != "1 of 3" || a.Panel != "Chemistry panel" || a.Value != ">=0.040" || a.Flag != "H" || !a.Critical || !a.PanelCritical || a.ReferenceRange != "Child range" || !b.Absent || b.Flag != "" || b.ReferenceRange != "" {
		t.Fatalf("parent flag/range transposed: %+v %+v", a, b)
	}
	context := strings.Join(a.Context, " ")
	if !strings.Contains(context, "Panel critical value flag") || !strings.Contains(context, "Parent reference") {
		t.Fatal("panel qualifiers omitted")
	}
	d, raw, err := SelectedDocument(p, []string{a.Key}, "Fictional clinic")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Rows) != 1 || strings.Contains(string(raw), "Analyte B") || strings.Contains(string(raw), "Unselected private") || !strings.Contains(string(raw), "partial laboratory search") {
		t.Fatal("selection imported a sibling or concealed partial coverage")
	}
}

func TestSelectedLabGoesThroughNormalLocalReviewAndDisclosure(t *testing.T) {
	o := sampleObs("lab-private-resource")
	o["identifier"] = []obsMap{{"value": "PRIVATE-SOURCE-ACCESSION"}}
	o["text"] = obsMap{"status": "generated", "div": "<div>PRIVATE-NARRATIVE</div>"}
	other := sampleObs("other")
	other["code"] = obsMap{"text": "UNSELECTED-ANALYTE"}
	p := parseObs(t, o, other)
	d, original, err := SelectedDocument(p, []string{p.Rows[0].Key}, "Example EHR")
	if err != nil {
		t.Fatal(err)
	}
	if d.BelongsTo != "unknown" || d.Rows[0].CollectionDate != "" || !strings.Contains(d.Rows[0].Original, "Effective: 2026") || !strings.Contains(string(original), "Observation/lab-private-resource version v3") || !strings.Contains(string(original), "http://loinc.org") {
		t.Fatal("wrong identity/date semantics or missing local provenance")
	}
	for _, x := range []string{testPatient, "MRN-SECRET", "Synthetic Private Patient", "UNSELECTED-ANALYTE", "PRIVATE-NARRATIVE", "PRIVATE-SOURCE-ACCESSION"} {
		if bytes.Contains(original, []byte(x)) {
			t.Fatalf("unselected/identifying field %s in original", x)
		}
	}
	e, err := clinical.New("enc", clinical.Intake{Age: 30, Concern: "Review a result", NoticeVersion: clinical.NoticeVersion, ReportFirst: true}, clinical.ProviderRef{Kind: "compatible", Model: "synthetic"}, "")
	if err != nil {
		t.Fatal(err)
	}
	d.ID = "d1"
	d.BelongsTo = "self"
	if err = e.AttachReport(d, ""); err != nil {
		t.Fatal(err)
	}
	if len(e.Facts) != 0 || e.PendingTask() != "" || len(e.Warnings) == 0 || !e.Warnings[0].Active {
		t.Fatal("import did not enter ordinary unreviewed local warning workflow")
	}
	r := d.Rows[0]
	for _, c := range []clinical.Command{{Action: "review_report_row", DocumentID: d.ID, RowID: r.ID, Row: r}, {Action: "authorize_report", DocumentID: d.ID, RowIDs: []string{r.ID}}, {Action: "review"}} {
		if err = e.Apply(c, ""); err != nil {
			t.Fatal(err)
		}
	}
	in, err := e.ModelInput("interview")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(in)
	for _, x := range []string{testPatient, "MRN-SECRET", "Synthetic Private Patient", "UNSELECTED-ANALYTE", "PRIVATE-NARRATIVE", "PRIVATE-SOURCE-ACCESSION", "lab-private-resource", "http://", "https://", "<div>"} {
		if bytes.Contains(b, []byte(x)) {
			t.Fatalf("unexpected %q in disclosed extract", x)
		}
	}
	if len(in.ReportEntries) != 1 || in.ReportEntries[0].Value != "<2.010" || in.ReportEntries[0].IssueDate != "2026-11-21T11:45:22Z" {
		t.Fatal("reviewed result changed")
	}
}

func TestBundleBoundsPaginationAndStableSelectionIdentity(t *testing.T) {
	o := sampleObs("lab-1")
	other := sampleObs("lab-2")
	p := parseObs(t, o, other)
	q := parseObs(t, other, o)
	if p.Rows[0].Key != q.Rows[1].Key || p.Rows[0].Key == p.Rows[1].Key {
		t.Fatal("selection keys depend on entry ordering")
	}
	for _, keys := range [][]string{nil, {"not-in-preview"}, {p.Rows[0].Key, p.Rows[0].Key}} {
		if _, _, err := SelectedDocument(p, keys, "Clinic"); err == nil {
			t.Fatal("accepted invalid selection")
		}
	}
	o["status"] = "entered-in-error"
	q = parseObs(t, sampleObs("lab-1"), o)
	if len(q.Rows) != 0 || q.SkippedByReason["ambiguous_resource_identity"] != 2 {
		t.Fatal("older result survived a conflicting version")
	}
	for _, fields := range []obsMap{{"link": []obsMap{{"relation": "next", "url": "https://outside.invalid/unsafe"}}}, {"meta": obsMap{"tag": []obsMap{{"system": "urn:clinical-history:ehr-import", "code": "partial"}}}}, {"total": 10}} {
		fields["resourceType"] = "Bundle"
		fields["type"] = "searchset"
		b, _ := json.Marshal(fields)
		q, err := ParseObservations(b, testPatient, testBase)
		if err != nil || !q.More {
			t.Fatalf("partial collection unreported: %v", err)
		}
	}
	for _, b := range [][]byte{[]byte(`{"resourceType":"Bundle","type":"collection"}`), []byte(`{"resourceType":"Bundle","type":"searchset","type":"collection"}`), []byte(`{"resourceType":"Bundle","type":"searchset"}{}`), bytes.Repeat([]byte("x"), maxObservationBytes+1)} {
		if _, err := ParseObservations(b, testPatient, testBase); err == nil {
			t.Fatal("accepted invalid/bounded response")
		}
	}
	q = parseObs(t, sampleObs("x"))
	if _, err := ParseObservations(searchRaw(t, sampleObs("x")), "../patient", testBase); err == nil {
		t.Fatal("accepted invalid patient scope")
	}
	if len(q.Rows) != 1 {
		t.Fatal("valid scalar dropped")
	}
}

func TestEffectivePeriodNotRelabelledAsSpecimenCollection(t *testing.T) {
	o := sampleObs("period")
	delete(o, "effectiveDateTime")
	o["effectivePeriod"] = obsMap{"end": "2026-10"}
	p := parseObs(t, o)
	if len(p.Rows) != 1 || p.Rows[0].Date != "start unknown to 2026-10" {
		t.Fatal("lost open/bounded period or source precision")
	}
	d, _, err := SelectedDocument(p, []string{p.Rows[0].Key}, "Local")
	if err != nil || d.Rows[0].CollectionDate != "" || !strings.Contains(d.Rows[0].Original, "Effective: start unknown to 2026-10") {
		t.Fatal("effective period became a collection date")
	}
	o["effectivePeriod"] = obsMap{"start": "2026-02-30"}
	if p := parseObs(t, o); len(p.Rows) != 0 || p.SkippedByReason["unsupported_time"] != 1 {
		t.Fatal("accepted invalid effective date")
	}
}

func TestLimitsAndSourceNoteSurviveNormalClinicalValidation(t *testing.T) {
	o := sampleObs("with-notes")
	o["note"] = []obsMap{{"text": "Specimen collected after eating; compare with nonfasting source interval."}}
	p := parseObs(t, o)
	d, _, err := SelectedDocument(p, []string{p.Rows[0].Key}, "Fictional EHR")
	if err != nil || !strings.Contains(d.Rows[0].Note, "Source result status: corrected") || !strings.Contains(d.Rows[0].Note, "Specimen collected after eating") {
		t.Fatal("reviewable transcription lost a clinically material qualifier")
	}
	if _, _, err := SelectedDocument(p, []string{p.Rows[0].Key}, strings.Repeat("x", 121)); err == nil {
		t.Fatal("overlong source name accepted")
	}
	o["note"] = []obsMap{{"text": strings.Repeat("n", 650)}, {"text": strings.Repeat("a", 650)}}
	if got := parseObs(t, o); len(got.Rows) != 0 || got.Skipped != 1 {
		t.Fatal("source notes silently truncated")
	}
	o = sampleObs("period")
	delete(o, "effectiveDateTime")
	o["effectivePeriod"] = obsMap{"start": "2026-11-22", "end": "2026-11-20"}
	if got := parseObs(t, o); got.SkippedByReason["unsupported_time"] != 1 {
		t.Fatal("impossible source period accepted")
	}
}
