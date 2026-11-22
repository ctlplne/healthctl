// Package ehr reads the supported subset of FHIR R4 laboratory search results.
// Parsing and selection operate only on supplied bytes and never resolve references.
package ehr

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ctlplne/healthctl/internal/clinical"
)

const (
	maxObservationBytes   = 4 << 20
	maxObservationEntries = 160
	maxObservationRows    = 160
	categorySystem        = "http://terminology.hl7.org/CodeSystem/observation-category"
)

type Preview struct {
	Rows            []Row          `json:"rows"`
	Skipped         int            `json:"skipped"`
	SkippedByReason map[string]int `json:"skippedByReason"`
	More            bool           `json:"more"`
}

type CodingSource struct {
	Path    string `json:"path"`
	System  string `json:"system"`
	Code    string `json:"code"`
	Version string `json:"version,omitempty"`
}
type Row struct {
	Critical       bool           `json:"critical,omitempty"`
	PanelCritical  bool           `json:"panelCritical,omitempty"`
	Codings        []CodingSource `json:"codings,omitempty"`
	Key            string         `json:"key"`
	Label          string         `json:"label"`
	Value          string         `json:"value"`
	Unit           string         `json:"unit"`
	ReferenceRange string         `json:"referenceRange"`
	Flag           string         `json:"flag"`
	Date           string         `json:"date"`
	Issued         string         `json:"issued"`
	Source         string         `json:"source"`
	Status         string         `json:"status"`
	Version        string         `json:"version"`
	Panel          string         `json:"panel,omitempty"`
	Component      string         `json:"component,omitempty"`
	Context        []string       `json:"context,omitempty"`
	Absent         bool           `json:"absent,omitempty"`
}

type object = map[string]json.RawMessage

var fhirID = regexp.MustCompile(`^[A-Za-z0-9.-]{1,64}$`)
var decimal = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)
var markup = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://|(?:^|\s)//[a-z]|data:[a-z]|(?:^|\s)(?:Patient|Practitioner|Observation)/[a-z0-9.-]+|(?:^|\s)www\.|mailto:|javascript:|\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[a-z]{2,}|<(?:/?[[:alpha:]][^>]*|![^>]*)>|\]\s*\(|\*\*|__|\x60|\b(?:patient\s*(?:name|id)|medical record(?: number)?|mrn|date of birth|dob|address|telephone)\s*[:#=]`)
var dateLex = regexp.MustCompile(`^[0-9]{4}(?:-[0-9]{2}(?:-[0-9]{2})?)?$`)

func field(o object, key string) string { var s string; _ = json.Unmarshal(o[key], &s); return s }
func record(raw json.RawMessage) object { var x object; _ = json.Unmarshal(raw, &x); return x }
func array(raw json.RawMessage) ([]json.RawMessage, bool) {
	if raw == nil {
		return nil, true
	}
	var out []json.RawMessage
	err := json.Unmarshal(raw, &out)
	return out, err == nil && out != nil
}
func has(o object, key string) bool   { _, ok := o[key]; return ok }
func (p *Preview) skip(reason string) { p.Skipped++; p.SkippedByReason[reason]++ }

func canonicalBase(s string) (string, bool) {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.ForceQuery || u.RawQuery != "" || strings.Contains(s, "#") || u.RawPath != "" {
		return "", false
	}
	for _, p := range strings.Split(u.Path, "/") {
		if p == "." || p == ".." {
			return "", false
		}
	}
	return strings.TrimRight(u.String(), "/"), true
}

// Duplicate object members make identity and result semantics ambiguous. Reject
// the bundle rather than silently applying JSON's last-member-wins behavior.
func uniqueJSON(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 48 {
			return errors.New("FHIR JSON exceeds the nesting limit")
		}
		t, err := dec.Token()
		if err != nil {
			return errors.New("Malformed FHIR JSON")
		}
		d, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		if d != '{' && d != '[' {
			return errors.New("Malformed FHIR JSON")
		}
		var keys map[string]bool
		if d == '{' {
			keys = map[string]bool{}
		}
		for dec.More() {
			if d == '{' {
				k, err := dec.Token()
				s, ok := k.(string)
				if err != nil || !ok || keys[s] {
					return errors.New("FHIR JSON contains ambiguous object members")
				}
				keys[s] = true
			}
			if err := walk(depth + 1); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("FHIR response contains additional JSON")
	}
	return nil
}

func validDate(s string, instant bool) bool {
	if s == "" {
		return false
	}
	if !instant && dateLex.MatchString(s) {
		var format string
		switch len(s) {
		case 4:
			format = "2006"
		case 7:
			format = "2006-01"
		case 10:
			format = time.DateOnly
		}
		_, err := time.Parse(format, s)
		return err == nil && s[:4] != "0000"
	}
	_, err := time.Parse(time.RFC3339Nano, s)
	return err == nil && !strings.HasPrefix(s, "0000")
}

type reader struct {
	patient, base string
	private       []string
}

// Accept only bounded plain clinical text. Resource ids, narratives, identifiers,
// performers, raw links and identities are never projected into report rows.
func (r reader) text(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 800 || !utf8.ValidString(s) || markup.MatchString(s) || strings.ContainsAny(s, "\n\r\t") {
		return "", false
	}
	for _, ch := range s {
		if unicode.IsControl(ch) || unicode.In(ch, unicode.Cf) {
			return "", false
		}
	}
	lower := strings.ToLower(s)
	for _, token := range r.private {
		if len(token) >= 4 && strings.Contains(lower, strings.ToLower(token)) {
			return "", false
		}
	}
	if strings.Contains(lower, "patient/"+strings.ToLower(r.patient)) {
		return "", false
	}
	return s, true
}

func unsafeExtension(raw json.RawMessage) bool {
	var v any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if dec.Decode(&v) != nil {
		return true
	}
	var unsafe func(any) bool
	unsafe = func(v any) bool {
		switch x := v.(type) {
		case map[string]any:
			for k, val := range x {
				if k == "modifierExtension" || k == "implicitRules" || k == "extension" || strings.HasPrefix(k, "_") {
					return true
				}
				if unsafe(val) {
					return true
				}
			}
		case []any:
			for _, val := range x {
				if unsafe(val) {
					return true
				}
			}
		}
		return false
	}
	return unsafe(v)
}

func labCategory(o object) bool {
	cs, ok := array(o["category"])
	if !ok {
		return false
	}
	for _, c := range cs {
		codes, _ := array(record(c)["coding"])
		for _, v := range codes {
			c := record(v)
			if field(c, "system") == categorySystem && field(c, "code") == "laboratory" {
				return true
			}
		}
	}
	return false
}

func (r reader) concept(raw json.RawMessage) (string, string, bool) {
	o := record(raw)
	if o == nil {
		return "", "", false
	}
	label := field(o, "text")
	if has(o, "text") {
		var ok bool
		label, ok = r.text(label)
		if !ok {
			return "", "", false
		}
	}
	codings, ok := array(o["coding"])
	if !ok || len(codings) > 12 {
		return "", "", false
	}
	var alternatives []string
	for _, v := range codings {
		c := record(v)
		display, code := field(c, "display"), field(c, "code")
		if c == nil || (display == "" && code == "") {
			return "", "", false
		}
		if display != "" {
			var ok bool
			display, ok = r.text(display)
			if !ok {
				return "", "", false
			}
		}
		if code != "" {
			var ok bool
			code, ok = r.text(code)
			if !ok {
				return "", "", false
			}
		}
		if label == "" {
			label = display
			if label == "" {
				label = code
			}
		}
		switch field(c, "system") {
		case "http://loinc.org":
			code = "LOINC " + code
		case "http://snomed.info/sct":
			code = "SNOMED CT " + code
		case "http://unitsofmeasure.org":
			code = "UCUM " + code
		}
		detail := strings.TrimSpace(strings.Join([]string{display, code}, " "))
		if detail != "" && detail != label && !slices.Contains(alternatives, detail) {
			alternatives = append(alternatives, detail)
		}
	}
	return label, strings.Join(alternatives, "; "), label != ""
}

func (r reader) quantity(raw json.RawMessage) (string, string, string, bool) {
	q := record(raw)
	if q == nil {
		return "", "", "", false
	}
	value := string(q["value"])
	if len(value) > 60 || !decimal.MatchString(value) {
		return "", "", "", false
	}
	comp := field(q, "comparator")
	if has(q, "comparator") && !slices.Contains([]string{"<", "<=", ">", ">="}, comp) {
		return "", "", "", false
	}
	unit, code := field(q, "unit"), field(q, "code")
	var ok bool
	if has(q, "unit") {
		unit, ok = r.text(unit)
		if !ok {
			return "", "", "", false
		}
	}
	if has(q, "code") {
		code, ok = r.text(code)
		if !ok {
			return "", "", "", false
		}
	}
	if unit == "" {
		unit = code
	}
	qual := ""
	if code != "" && (unit != code || field(q, "system") == "http://unitsofmeasure.org") {
		qual = "Coded unit: " + code
		if field(q, "system") == "http://unitsofmeasure.org" {
			qual = "UCUM unit: " + code
		}
	}
	return comp + value, unit, qual, true
}

func (r reader) period(raw json.RawMessage) (string, bool) {
	o := record(raw)
	if o == nil {
		return "", false
	}
	start, end := field(o, "start"), field(o, "end")
	if start == "" && end == "" {
		return "", false
	}
	if (has(o, "start") && !validDate(start, false)) || (has(o, "end") && !validDate(end, false)) {
		return "", false
	}
	if start != "" && end != "" {
		if dateLex.MatchString(start) && dateLex.MatchString(end) && len(start) == len(end) && start > end {
			return "", false
		}
		a, ae := time.Parse(time.RFC3339Nano, start)
		b, be := time.Parse(time.RFC3339Nano, end)
		if ae == nil && be == nil && a.After(b) {
			return "", false
		}
	}
	if start == "" {
		start = "start unknown"
	}
	if end == "" {
		end = "end unknown"
	}
	return start + " to " + end, true
}

func (r reader) effective(o object) (string, bool) {
	var key string
	for k := range o {
		if strings.HasPrefix(k, "effective") {
			if key != "" {
				return "", false
			}
			key = k
		}
	}
	switch key {
	case "":
		return "", true
	case "effectiveDateTime", "effectiveInstant":
		s := field(o, key)
		return s, validDate(s, key == "effectiveInstant")
	case "effectivePeriod":
		return r.period(o[key])
	default:
		return "", false
	}
}

func (r reader) codeList(raw json.RawMessage) (string, bool) {
	items, ok := array(raw)
	if !ok || len(items) > 12 {
		return "", false
	}
	var vals []string
	for _, item := range items {
		label, codes, ok := r.concept(item)
		if !ok {
			return "", false
		}
		if codes != "" {
			label += " (" + codes + ")"
		}
		vals = append(vals, label)
	}
	return strings.Join(vals, "; "), true
}

func (r reader) rangeText(raw json.RawMessage) (string, bool) {
	items, ok := array(raw)
	if !ok || len(items) > 8 {
		return "", false
	}
	var summaries []string
	for _, item := range items {
		x := record(item)
		if x == nil {
			return "", false
		}
		var parts []string
		for _, k := range []string{"low", "high"} {
			if !has(x, k) {
				continue
			}
			if has(record(x[k]), "comparator") {
				return "", false
			}
			v, u, q, ok := r.quantity(x[k])
			if !ok {
				return "", false
			}
			s := k + " " + strings.TrimSpace(v+" "+u)
			if q != "" && q != "UCUM unit: "+u {
				s += " (" + q + ")"
			}
			parts = append(parts, s)
		}
		if has(x, "text") {
			s, ok := r.text(field(x, "text"))
			if !ok {
				return "", false
			}
			parts = append(parts, s)
		}
		if has(x, "type") {
			s, c, ok := r.concept(x["type"])
			if !ok {
				return "", false
			}
			if c != "" {
				s += " (" + c + ")"
			}
			parts = append(parts, "type "+s)
		}
		if has(x, "appliesTo") {
			s, ok := r.codeList(x["appliesTo"])
			if !ok {
				return "", false
			}
			parts = append(parts, "applies to "+s)
		}
		if has(x, "age") {
			age := record(x["age"])
			if age == nil {
				return "", false
			}
			var ages []string
			for _, k := range []string{"low", "high"} {
				if has(age, k) {
					v, u, _, ok := r.quantity(age[k])
					if !ok {
						return "", false
					}
					ages = append(ages, k+" "+strings.TrimSpace(v+" "+u))
				}
			}
			if len(ages) == 0 {
				return "", false
			}
			parts = append(parts, "age "+strings.Join(ages, ", "))
		}
		if len(parts) == 0 || (!has(x, "low") && !has(x, "high") && !has(x, "text")) {
			return "", false
		}
		summaries = append(summaries, strings.Join(parts, ", "))
	}
	return strings.Join(summaries, " / "), true
}

func (r reader) value(o object) (value, unit, extra string, absent, ok bool) {
	key := ""
	for k := range o {
		if strings.HasPrefix(k, "value") {
			if key != "" {
				return
			}
			key = k
		}
	}
	if has(o, "dataAbsentReason") {
		if key != "" {
			return
		}
		s, c, valid := r.concept(o["dataAbsentReason"])
		if !valid {
			return
		}
		if c != "" {
			s += " (" + c + ")"
		}
		return "Data absent: " + s, "", "", true, true
	}
	switch key {
	case "valueQuantity":
		v, u, q, valid := r.quantity(o[key])
		return v, u, q, false, valid
	case "valueCodeableConcept":
		s, c, valid := r.concept(o[key])
		if c != "" {
			c = "Result coding: " + c
		}
		return s, "", c, false, valid
	case "valueString":
		s, valid := r.text(field(o, key))
		return s, "", "", false, valid
	case "valueBoolean":
		s := string(o[key])
		return s, "", "", false, s == "true" || s == "false"
	case "valueInteger":
		s := string(o[key])
		_, err := strconv.ParseInt(s, 10, 32)
		return s, "", "", false, err == nil
	default:
		return
	}
}

func (r reader) interpret(raw json.RawMessage) (flag, detail string, ok bool) {
	full, ok := r.codeList(raw)
	if !ok {
		return "", "", false
	}
	if full == "" {
		return "", "", true
	}
	items, _ := array(raw)
	// Keep the source code used by the local warning policy. Retain any
	// additional display/flags explicitly, including on multi-flag results.
	var first string
	for _, v := range items {
		codes, _ := array(record(v)["coding"])
		for _, v := range codes {
			c := record(v)
			if field(c, "system") != "http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation" {
				continue
			}
			code := field(c, "code")
			if first == "" {
				first = code
			}
			switch code {
			case "HH", "LL":
				return code, full, true
			case "AA":
				return "Critical", full, true
			}
		}
	}
	if len(items) == 1 && first != "" {
		return first, full, true
	}
	return full, "", true
}

func (r reader) sharedContext(o object) ([]string, bool) {
	var out []string
	for _, k := range []string{"bodySite", "method"} {
		if !has(o, k) {
			continue
		}
		s, c, ok := r.concept(o[k])
		if !ok {
			return nil, false
		}
		if c != "" {
			s += " (" + c + ")"
		}
		out = append(out, k+": "+s)
	}
	notes, ok := array(o["note"])
	if !ok || len(notes) > 8 {
		return nil, false
	}
	for _, note := range notes {
		x, ok := r.text(field(record(note), "text"))
		if !ok {
			return nil, false
		}
		out = append(out, "Source note: "+x)
	}
	for _, k := range []string{"specimen", "device"} {
		if !has(o, k) {
			continue
		}
		ref := record(o[k])
		if ref == nil {
			return nil, false
		}
		s := k + " linked in source; linked record not retrieved"
		if has(ref, "display") {
			display, ok := r.text(field(ref, "display"))
			if !ok {
				return nil, false
			}
			s += " (" + display + ")"
		}
		out = append(out, s)
	}
	for _, k := range []string{"hasMember", "derivedFrom"} {
		if !has(o, k) {
			continue
		}
		list, ok := array(o[k])
		if !ok {
			return nil, false
		}
		out = append(out, fmt.Sprintf("%s: %d linked records, not expanded", k, len(list)))
	}
	return out, true
}

// Retain exact coding namespaces and versions in the local original. Their URIs
// are not copied into the subsequent model-facing transcription.
func (r reader) codingSources(o object) ([]CodingSource, bool) {
	var found []CodingSource
	var visit func(json.RawMessage, string) bool
	visit = func(raw json.RawMessage, path string) bool {
		if len(raw) == 0 {
			return true
		}
		switch raw[0] {
		case '[':
			items, ok := array(raw)
			if !ok {
				return false
			}
			for i, item := range items {
				if !visit(item, fmt.Sprintf("%s[%d]", path, i)) {
					return false
				}
			}
		case '{':
			v := record(raw)
			if has(v, "system") && has(v, "code") {
				system, code, version := field(v, "system"), field(v, "code"), field(v, "version")
				if len(system) == 0 || len(system) > 300 || strings.ContainsAny(system, "<>\r\n\t ") || strings.Contains(system, "?") {
					return false
				}
				if u, err := url.Parse(system); err != nil || !u.IsAbs() || u.User != nil {
					return false
				}
				if _, ok := r.text(code); !ok {
					return false
				}
				if version != "" {
					if _, ok := r.text(version); !ok {
						return false
					}
				}
				found = append(found, CodingSource{path, system, code, version})
			}
			for _, k := range []string{"coding", "low", "high", "type", "age", "appliesTo"} {
				if has(v, k) && !visit(v[k], path+"."+k) {
					return false
				}
			}
		}
		return len(found) <= 60
	}
	for _, k := range []string{"code", "valueQuantity", "valueCodeableConcept", "dataAbsentReason", "interpretation", "referenceRange", "bodySite", "method"} {
		if !visit(o[k], k) {
			return nil, false
		}
	}
	return found, true
}

func (r reader) result(node object, base Row) (Row, bool) {
	row := base
	row.Context = slices.Clone(base.Context)
	row.Codings = slices.Clone(base.Codings)
	codings, ok := r.codingSources(node)
	if !ok {
		return Row{}, false
	}
	row.Codings = append(row.Codings, codings...)
	label, codes, ok := r.concept(node["code"])
	if !ok {
		return Row{}, false
	}
	row.Label = label
	if codes != "" {
		row.Context = append(row.Context, "Result code: "+codes)
	}
	v, u, extra, absent, ok := r.value(node)
	if !ok {
		return Row{}, false
	}
	row.Value = v
	row.Unit = u
	row.Absent = absent
	if extra != "" {
		row.Context = append(row.Context, extra)
	}
	flag, detail, ok := r.interpret(node["interpretation"])
	if !ok {
		return Row{}, false
	}
	row.Flag = flag
	row.Critical = clinical.PrintedCriticalFlag(flag) || row.PanelCritical
	if detail != "" && detail != flag {
		row.Context = append(row.Context, "Source interpretation: "+detail)
	}
	row.ReferenceRange, ok = r.rangeText(node["referenceRange"])
	if !ok {
		return Row{}, false
	}
	return row, true
}

func hasResult(o object) bool {
	if has(o, "dataAbsentReason") {
		return true
	}
	for k := range o {
		if strings.HasPrefix(k, "value") {
			return true
		}
	}
	return false
}

func (r reader) read(entry object, p *Preview, dups map[string]int) {
	o := record(entry["resource"])
	if field(o, "resourceType") != "Observation" {
		p.skip("other_entry")
		return
	}
	id := field(o, "id")
	if !fhirID.MatchString(id) || id == "." || id == ".." || dups[id] != 1 {
		p.skip("ambiguous_resource_identity")
		return
	}
	if search := record(entry["search"]); has(search, "mode") && field(search, "mode") != "match" {
		p.skip("non_match")
		return
	}
	if full := field(entry, "fullUrl"); has(entry, "fullUrl") && full != r.base+"/Observation/"+id {
		p.skip("unmatched_resource_origin")
		return
	}
	subj := record(o["subject"])
	ref := field(subj, "reference")
	if ref != "Patient/"+r.patient && ref != r.base+"/Patient/"+r.patient {
		p.skip("patient_context")
		return
	}
	if typ := field(subj, "type"); has(subj, "type") && typ != "Patient" && typ != "http://hl7.org/fhir/StructureDefinition/Patient" {
		p.skip("patient_context")
		return
	}
	status := field(o, "status")
	if !slices.Contains([]string{"final", "amended", "corrected"}, status) {
		p.skip("ineligible_status")
		return
	}
	if !labCategory(o) {
		p.skip("non_laboratory")
		return
	}
	meta := record(o["meta"])
	tags, tagOK := array(meta["tag"])
	if !tagOK || has(meta, "security") {
		p.skip("unsupported_access_or_subset")
		return
	}
	for _, tag := range tags {
		if field(record(tag), "code") == "SUBSETTED" {
			p.skip("unsupported_access_or_subset")
			return
		}
	}
	if has(o, "focus") || has(o, "contained") || unsafeExtension(entry["resource"]) {
		p.skip("unsupported_context")
		return
	}
	r.private = append(slices.Clone(r.private), r.patient, field(subj, "display"), field(record(subj["identifier"]), "value"))
	version := field(record(o["meta"]), "versionId")
	if has(meta, "versionId") && !fhirID.MatchString(version) {
		p.skip("unsupported_version")
		return
	}
	label, panelCoding, ok := r.concept(o["code"])
	if !ok {
		p.skip("invalid_code")
		return
	}
	date, ok := r.effective(o)
	issued := field(o, "issued")
	if !ok || has(o, "issued") && !validDate(issued, true) {
		p.skip("unsupported_time")
		return
	}
	context, ok := r.sharedContext(o)
	if !ok {
		p.skip("unsupported_context")
		return
	}
	base := Row{Date: date, Issued: issued, Status: status, Source: "Observation/" + id, Version: version, Context: context}
	comps, ok := array(o["component"])
	if !ok || len(comps) > 64 {
		p.skip("unsupported_components")
		return
	}
	hasValue := hasResult(o)
	var rows []Row
	if hasValue {
		main, ok := r.result(o, base)
		if !ok {
			p.skip("unsupported_result")
			return
		}
		if len(comps) > 0 {
			main.Context = append(main.Context, fmt.Sprintf("Panel also contains %d components.", len(comps)))
		}
		rows = append(rows, main)
	}
	if len(comps) > 0 {
		base.Panel = label
		panelCodes, ok := r.codingSources(object{"code": o["code"], "interpretation": o["interpretation"], "referenceRange": o["referenceRange"], "method": o["method"], "bodySite": o["bodySite"]})
		if !ok {
			p.skip("unsupported_panel_context")
			return
		}
		for i := range panelCodes {
			panelCodes[i].Path = "panel." + panelCodes[i].Path
		}
		base.Codings = panelCodes
		base.Context = slices.Clone(context)
		if panelCoding != "" {
			base.Context = append(base.Context, "Panel code: "+panelCoding)
		}
		ip, ok := r.codeList(o["interpretation"])
		if !ok {
			p.skip("unsupported_panel_context")
			return
		}
		if ip != "" {
			message := "Panel interpretation: " + ip
			if flag, _, _ := r.interpret(o["interpretation"]); clinical.PrintedCriticalFlag(flag) {
				base.PanelCritical = true
				message = "Panel critical value flag: " + ip + " (applies to the panel)."
			}
			base.Context = append(base.Context, message)
		}
		rg, ok := r.rangeText(o["referenceRange"])
		if !ok {
			p.skip("unsupported_panel_context")
			return
		}
		if rg != "" {
			base.Context = append(base.Context, "Parent reference range (not a component range): "+rg)
		}
		if hasValue {
			base.Context = append(base.Context, "Parent observation has a separate result.")
		}
		for i, c := range comps {
			part, ok := r.result(record(c), base)
			if !ok {
				p.skip("unsupported_component")
				continue
			}
			part.Component = strconv.Itoa(i+1) + " of " + strconv.Itoa(len(comps))
			rows = append(rows, part)
		}
	}
	if !hasValue && len(comps) == 0 {
		p.skip("no_supported_result")
		return
	}
	for _, row := range rows {
		if len(row.Label) > 180 || len(row.Value) > 500 || len(row.Unit) > 80 || len(row.ReferenceRange) > 200 || len(row.Flag) > 80 || len(strings.Join(row.Context, "; ")) > 950 || len(row.Panel) > 180 || len(reviewNote(row)) > 1200 || len(rowText(row)) > 3800 {
			p.skip("review_size_limit")
			continue
		}
		if len(p.Rows) >= maxObservationRows {
			p.More = true
			p.skip("row_limit")
			continue
		}
		b, _ := json.Marshal(row)
		h := sha256.New()
		h.Write([]byte(r.base + "\x00" + r.patient + "\x00"))
		h.Write(b)
		row.Key = hex.EncodeToString(h.Sum(nil)[:18])
		p.Rows = append(p.Rows, row)
	}
}

func ParseObservations(raw []byte, patientID, fhirBase string) (Preview, error) {
	p := Preview{Rows: []Row{}, SkippedByReason: map[string]int{}}
	base, ok := canonicalBase(fhirBase)
	if !ok || !fhirID.MatchString(patientID) || patientID == "." || patientID == ".." {
		return p, errors.New("The configured FHIR address and record identity are required")
	}
	if len(raw) == 0 || len(raw) > maxObservationBytes || !utf8.Valid(raw) {
		return p, errors.New("FHIR search response is unreadable or exceeds the local size limit")
	}
	if err := uniqueJSON(raw); err != nil {
		return p, err
	}
	root := record(raw)
	if field(root, "resourceType") != "Bundle" || field(root, "type") != "searchset" || has(root, "modifierExtension") || has(root, "implicitRules") {
		return p, errors.New("Expected a FHIR R4 searchset Bundle with ordinary interpretation rules")
	}
	links, ok := array(root["link"])
	if !ok {
		return p, errors.New("Invalid FHIR search pagination")
	}
	for _, l := range links {
		if field(record(l), "relation") == "next" {
			p.More = true
		}
	}
	tags, _ := array(record(root["meta"])["tag"])
	for _, t := range tags {
		c := record(t)
		if field(c, "system") == "urn:clinical-history:ehr-import" && field(c, "code") == "partial" {
			p.More = true
		}
	}
	entries, ok := array(root["entry"])
	if !ok || len(entries) > maxObservationEntries {
		return p, errors.New("FHIR search response exceeds the local entry limit or has invalid entries")
	}
	if has(root, "total") {
		n, err := strconv.ParseUint(string(root["total"]), 10, 32)
		if err != nil {
			return p, errors.New("Invalid FHIR search total")
		}
		if n > uint64(len(entries)) {
			p.More = true
		}
	}
	dups := map[string]int{}
	for _, v := range entries {
		o := record(record(v)["resource"])
		if field(o, "resourceType") == "Observation" {
			dups[field(o, "id")]++
		}
	}
	r := reader{patient: patientID, base: base}
	for _, v := range entries {
		r.read(record(v), &p, dups)
	}
	return p, nil
}

// rowText is the inspectable source transcription offered for subsequent owner
// review. Clinical dates are labelled as effective times, not collection times.
func rowText(r Row) string {
	fields := []string{r.Label + " | " + strings.TrimSpace(r.Value+" "+r.Unit), "Status: " + r.Status}
	if r.Panel != "" {
		fields = append(fields, "Panel: "+r.Panel+" (component "+r.Component+")")
	}
	for _, x := range []struct{ label, v string }{{"Reference range", r.ReferenceRange}, {"Flag", r.Flag}, {"Effective", r.Date}, {"Issued", r.Issued}} {
		if x.v != "" {
			fields = append(fields, x.label+": "+x.v)
		}
	}
	fields = append(fields, r.Context...)
	return strings.Join(fields, " | ")
}

func cleanSourceName(s string) (string, bool) {
	name, ok := (reader{}).text(s)
	return name, ok && len(name) <= 120
}

func reviewNote(r Row) string {
	note := "Source result status: " + r.Status + "."
	if r.Panel != "" {
		note += " Component " + r.Component + " of " + r.Panel + "."
	}
	if r.Date != "" {
		note += " Effective: " + r.Date + "."
	}
	if len(r.Context) > 0 {
		note += " " + strings.Join(r.Context, "; ")
	}
	return note
}

// SelectedDocument returns a local text original and unreviewed candidate rows.
// Only the local original carries resource/version identifiers. The caller binds
// the document ID and records the owner's patient-identity attestation.
func SelectedDocument(p Preview, keys []string, sourceName string) (clinical.SourceDocument, []byte, error) {
	d := clinical.SourceDocument{}
	name, ok := cleanSourceName(sourceName)
	if !ok {
		return d, nil, errors.New("Choose a short plain source name")
	}
	if len(keys) == 0 || len(keys) > 100 || len(p.Rows) > maxObservationRows {
		return d, nil, errors.New("Select between one and 100 current laboratory rows")
	}
	all := map[string]Row{}
	for _, r := range p.Rows {
		if _, exists := all[r.Key]; exists {
			return d, nil, errors.New("Ambiguous result selection")
		}
		all[r.Key] = r
	}
	used := map[string]bool{}
	var original strings.Builder
	original.WriteString("Selected FHIR R4 laboratory results\nSource: " + name + "\n")
	if p.More || p.Skipped > 0 {
		fmt.Fprintf(&original, "Partial search. Additional results possible: %t. Entries or components omitted: %d.\n", p.More, p.Skipped)
	}
	d = clinical.SourceDocument{Title: name + " laboratory results", Kind: "txt", BelongsTo: "unknown", Rows: []clinical.ReportRow{}}
	for i, key := range keys {
		r, ok := all[key]
		if !ok || used[key] {
			return clinical.SourceDocument{}, nil, errors.New("A selected laboratory result is unavailable or duplicated")
		}
		used[key] = true
		line := rowText(r)
		if p.More || p.Skipped > 0 {
			line += " | Selected from a partial laboratory search."
		}
		fmt.Fprintf(&original, "\nEntry %d\n%s", i+1, r.Source)
		if r.Version != "" {
			original.WriteString(" version " + r.Version)
		}
		original.WriteString("\n" + line + "\n")
		for _, c := range r.Codings {
			fmt.Fprintf(&original, "Source coding (%s): %s | %s", c.Path, c.System, c.Code)
			if c.Version != "" {
				original.WriteString(" | version " + c.Version)
			}
			original.WriteByte('\n')
		}
		note := reviewNote(r)
		d.Rows = append(d.Rows, clinical.ReportRow{ID: fmt.Sprintf("ehr%d", i+1), Original: line, Locator: fmt.Sprintf("Selected FHIR lab entry %d", i+1),
			Name: r.Label, Value: r.Value, Units: r.Unit, ReferenceRange: r.ReferenceRange, Flag: r.Flag, IssueDate: r.Issued, Note: note, Subject: "self", Uncertain: r.Absent})
	}
	return d, []byte(original.String()), nil
}
