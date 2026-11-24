package clinical

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

func (e *Encounter) document(id string) *SourceDocument {
	for i := range e.Documents {
		if e.Documents[i].ID == id {
			return &e.Documents[i]
		}
	}
	return nil
}
func (d *SourceDocument) row(id string) *ReportRow {
	for i := range d.Rows {
		if d.Rows[i].ID == id {
			return &d.Rows[i]
		}
	}
	return nil
}
func (e *Encounter) currentReview(doc, row string) *ReportReview {
	for i := len(e.ReportReviews) - 1; i >= 0; i-- {
		r := &e.ReportReviews[i]
		if r.DocumentID == doc && r.RowID == row && r.Active {
			return r
		}
	}
	return nil
}
func validOwner(s string) bool { return s == "self" || s == "other" || s == "unknown" }
func newerID(items int) string { return fmt.Sprintf("rr%d", items+1) }
func isoDay(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse(time.DateOnly, s)
	return err == nil
}
func rowValid(r ReportRow) bool {
	if !utf8.ValidString(r.Original) || len(r.Original) > 8192 || len(r.Excerpt) > 4000 || len(r.Locator) > 300 || len(r.Name) > 180 || len(r.Value) > 500 || len(r.Units) > 80 || len(r.ReferenceRange) > 200 || len(r.Flag) > 80 || len(r.Note) > 1200 {
		return false
	}
	return len(r.ID) <= 128 && len(r.IssueDate) <= 100 && len(r.CollectionDate) <= 100 && (r.Subject == "" || validSubject(r.Subject))
}

func (e *Encounter) reportState() {
	e.Pathways = e.activePathways()
	e.Pending = ""
	e.ReviewExtraction = false
	e.Failure = ""
	e.FailedTask = ""
	e.Question = ""
	e.Choices = []string{}
	e.QuestionSource = nil
	e.QuestionReviewIDs = nil
	if len(e.OutstandingReviewIDs) > 0 {
		e.Outstanding = []string{}
		e.OutstandingReviewIDs = nil
	}
	switch {
	case e.ScopeReason != "":
		e.State = "unsupported_scope"
	case e.hasWarning():
		e.State = "interrupted_for_safety"
	default:
		e.State = "history_review"
	}
}

// AttachReport records local candidates and screens relevant printed directions
// before transcription review. This operation never initiates a provider call.
func (e *Encounter) AttachReport(doc SourceDocument, emergency string) error {
	if !validText(doc.ID, 128) || e.document(doc.ID) != nil || len(e.Documents) >= 30 || !validText(doc.Title, 240) || !validOwner(doc.BelongsTo) || len(doc.Rows) > 1500 || len(doc.Kind) > 100 || len(doc.Purpose) > 1000 || len(doc.Priority) > 1000 || len(doc.IssueDate) > 100 {
		return errors.New("Invalid report identity, size or metadata")
	}
	seen := map[string]bool{}
	rows := slices.Clone(doc.Rows)
	for i := range rows {
		r := &rows[i]
		if r.ID == "" {
			r.ID = id("r", i)
		}
		if seen[r.ID] || !rowValid(*r) || !validText(r.Original, 8192) || !validText(r.Locator, 300) {
			return errors.New("Each report row needs a distinct identifier, original text and source location")
		}
		seen[r.ID] = true
	}
	doc.Rows = rows
	doc.CreatedAt = time.Now().UTC()
	e.Documents = append(e.Documents, doc)
	e.changed()
	e.screenDocument(doc, emergency)
	e.reportState()
	e.touch()
	return nil
}

var printedCritical = regexp.MustCompile(`(?i)^(critical(?: high| low)?|panic(?: high| low)?|hh|ll)$`)
var explicitReportUrgency = regexp.MustCompile(`(?i)\b(critical (?:result|value|finding)|panic value|seek (?:immediate |urgent )?(?:emergency (?:care|help)|medical attention immediately)|go to (?:the |an )?emergency (?:room|department) (?:now|immediately)|call (?:emergency services|your local emergency number) (?:now|immediately))\b`)

// PrintedCriticalFlag identifies an explicit source flag in the development policy.
// It does not derive a clinical threshold from a result value.
func PrintedCriticalFlag(flag string) bool {
	return printedCritical.MatchString(strings.TrimSpace(flag))
}

func reportAlert(r ReportRow) bool {
	if PrintedCriticalFlag(r.Flag) {
		return true
	}
	for _, m := range explicitReportUrgency.FindAllStringIndex(r.Original, -1) {
		if !conditionalPrinted.MatchString(clauseAt(r.Original, m[0])) {
			return true
		}
	}
	return false
}

func (e *Encounter) screenDocument(d SourceDocument, emergency string) {
	if d.BelongsTo == "other" {
		return
	}
	for _, r := range d.Rows {
		if !reportAlert(r) && !documentUrgentSign(d, r) {
			continue
		}
		e.raiseReportWarning(d, r, emergency)
	}
}

func (e *Encounter) raiseReportWarning(d SourceDocument, r ReportRow, emergency string) {
	rule := "report_printed_urgency"
	msg := "This report appears to contain a critical flag or urgent instruction. Check the original report now and promptly contact the service that issued it. If you believe you have an emergency, seek local emergency care immediately."
	if !reportAlert(r) && documentUrgentSign(d, r) {
		rule = "report_possible_current_symptom"
		msg = "A passage in this report may describe an urgent symptom. Check the date and original wording. If this is happening now, seek local emergency care immediately."
	}
	for _, w := range e.Warnings {
		if w.Active && w.DocumentID == d.ID && w.RowID == r.ID && w.Rule == rule {
			return
		}
	}
	if d.BelongsTo == "unknown" {
		msg = "The report's owner has not been confirmed. " + msg
	}
	if emergency != "" {
		msg += " " + strings.TrimSpace(emergency)
	}
	e.Warnings = append(e.Warnings, Warning{
		ID: id("w", len(e.Warnings)), DocumentID: d.ID, RowID: r.ID, Evidence: Evidence{Quote: r.Original},
		Rule: rule, PolicyVersion: PolicyVersion, Active: true, Message: msg, CreatedAt: time.Now().UTC(),
	})
}

func (e *Encounter) resolveDocWarning(doc, row string) {
	for i := range e.Warnings {
		w := &e.Warnings[i]
		if w.Active && w.DocumentID == doc && (row == "" || w.RowID == row) {
			w.Active = false
			w.ResolvedRevision = e.HistoryRevision
		}
	}
}

func (e *Encounter) applyReport(c Command, emergency string) error {
	d := e.document(c.DocumentID)
	if d == nil {
		return errors.New("Choose an existing report")
	}
	switch c.Action {
	case "report_identity":
		if !validOwner(c.BelongsTo) {
			return errors.New("Choose whose report this is")
		}
		if d.BelongsTo == c.BelongsTo {
			return nil
		}
		e.changed()
		e.resolveDocWarning(d.ID, "")
		e.ReportIdentityChanges = append(e.ReportIdentityChanges, ReportIdentityChange{d.ID, d.BelongsTo, c.BelongsTo, e.HistoryRevision, time.Now().UTC()})
		d.BelongsTo = c.BelongsTo
		for i := range e.ReportReviews {
			r := &e.ReportReviews[i]
			if r.Active && r.DocumentID == d.ID {
				e.retireReview(r, true)
			}
		}
		e.screenDocument(*d, emergency)
	case "add_report_passage":
		if !validText(c.Text, 4000) || !validText(c.Locator, 300) || len(d.Rows) >= 1500 {
			return errors.New("Enter a transcription and its page, row or region")
		}
		rowID := id("manual", len(d.Rows))
		for d.row(rowID) != nil {
			rowID += "x"
		}
		r := ReportRow{ID: rowID, Original: c.Text, Locator: c.Locator, Manual: true, Uncertain: true}
		d.Rows = append(d.Rows, r)
		e.changed()
		if d.BelongsTo != "other" && (reportAlert(r) || documentUrgentSign(*d, r)) {
			e.raiseReportWarning(*d, r, emergency)
		}
	case "review_report_row":
		if d.BelongsTo != "self" {
			return errors.New("Confirm this is your report before reviewing entries")
		}
		source := d.row(c.RowID)
		if source == nil {
			return errors.New("Choose a report row to review")
		}
		row := c.Row
		if (row.Original != "" && row.Original != source.Original) || (row.Locator != "" && row.Locator != source.Locator) {
			return errors.New("Keep the original passage and location unchanged; record corrections in the transcription")
		}
		row.ID = source.ID
		row.Original = source.Original
		row.Locator = source.Locator
		row.Manual = source.Manual
		if row.IssueDate == "" {
			row.IssueDate = d.IssueDate
		}
		if row.Excerpt == "" {
			row.Excerpt = source.Original
		}
		if !validText(row.Name, 180) || !rowValid(row) || !validSubject(row.Subject) || !strings.Contains(source.Original, row.Excerpt) || strings.TrimSpace(row.Excerpt) == "" || len(e.ReportReviews) >= 3000 {
			return errors.New("Check the entry name, subject and exact selected passage")
		}
		e.changed()
		prior := ""
		previousFact := ""
		if old := e.currentReview(d.ID, source.ID); old != nil {
			prior = old.ID
			previousFact = old.FactID
			e.retireReview(old, false)
		}
		if prior == "" {
			for i := len(e.ReportReviews) - 1; i >= 0; i-- {
				old := e.ReportReviews[i]
				if old.DocumentID == d.ID && old.RowID == source.ID {
					prior = old.ID
					previousFact = old.FactID
					break
				}
			}
		}
		review := ReportReview{ID: newerID(len(e.ReportReviews)), DocumentID: d.ID, RowID: source.ID, Row: row, Active: true, HistoryRevision: e.HistoryRevision, FactID: id("f", len(e.Facts)), Supersedes: prior, CreatedAt: time.Now().UTC()}
		offset := strings.Index(source.Original, row.Excerpt)
		status := "documented"
		if row.Uncertain {
			status = "uncertain"
		}
		e.Facts = append(e.Facts, Fact{
			ID: review.FactID, SourceType: "report_entry", DocumentID: d.ID, RowID: source.ID, ReviewID: review.ID,
			FindingProposal: FindingProposal{Evidence: Evidence{Quote: row.Excerpt}, Concept: row.Name, Status: status,
				Subject: row.Subject, TimeContext: row.CollectionDate, Value: row.Value, Units: row.Units},
			StartByte: offset, EndByte: offset + len(row.Excerpt), Active: true, HistoryRevision: e.HistoryRevision,
			Corrected: prior != "", Supersedes: previousFact,
		})
		e.ReportReviews = append(e.ReportReviews, review)
		if row.Subject == "self" && (PrintedCriticalFlag(row.Flag) || reportAlert(*source)) {
			e.raiseReportWarning(*d, *source, emergency)
		}
		if row.Subject != "self" || (PrintedCriticalFlag(source.Flag) && !PrintedCriticalFlag(row.Flag) && !explicitReportUrgency.MatchString(source.Original)) {
			e.resolveDocWarning(d.ID, row.ID)
		}
		e.screenReviewComment(*d, row, emergency)
	case "withdraw_report_row":
		r := e.currentReview(d.ID, c.RowID)
		if r == nil {
			return errors.New("Choose an active reviewed row")
		}
		e.changed()
		e.retireReview(r, true)
		// Withdrawing permission for a source cannot remove an unresolved printed warning.
	case "authorize_report":
		preview, err := e.ReportPreview(d.ID, c.RowIDs)
		if err != nil {
			return err
		}
		selectionChanged := false
		for i := range e.ReportReviews {
			r := &e.ReportReviews[i]
			if !r.Active || r.DocumentID != d.ID {
				continue
			}
			selected := slices.Contains(c.RowIDs, r.RowID)
			previous := r.ApprovedFor != nil && *r.ApprovedFor == e.Provider
			if previous != selected {
				selectionChanged = true
			}
			r.ApprovedFor = nil
			if selected {
				dest := e.Provider
				r.ApprovedFor = &dest
			}
		}
		if selectionChanged {
			e.changed()
			var ids []string
			for _, row := range preview {
				ids = append(ids, row.ReviewID)
			}
			e.ReportConsents = append(e.ReportConsents, ReportConsent{d.ID, ids, e.Provider, e.HistoryRevision, time.Now().UTC()})
		}
	default:
		return errors.New("Unknown report action")
	}
	e.rebuildContradictions()
	e.reportState()
	return nil
}

func (e *Encounter) retireReview(r *ReportReview, withdraw bool) {
	r.Active = false
	r.Withdrawn = withdraw
	r.RetiredRevision = e.HistoryRevision
	r.ApprovedFor = nil
	if f := e.fact(r.FactID); f != nil {
		f.Active = false
	}
	// The patient truly gave these answers. Retain them and their question
	// context, but retire interpretations that depended on the older report.
	dependent := map[string]bool{}
	for i := range e.Answers {
		a := &e.Answers[i]
		if slices.Contains(a.ReviewIDs, r.ID) {
			a.SourceOutdated = true
			dependent[a.ID] = true
		}
	}
	for i := range e.Facts {
		f := &e.Facts[i]
		if dependent[f.AnswerID] {
			f.SourceOutdated = true
			f.Active = false
		}
	}
}

func extract(r ReportReview) ReportExtract {
	v := r.Row
	return ReportExtract{FactID: r.FactID, DocumentID: r.DocumentID, RowID: r.RowID, ReviewID: r.ID, Quote: v.Excerpt,
		Name: v.Name, Value: v.Value, Units: v.Units, ReferenceRange: v.ReferenceRange, Flag: v.Flag,
		CollectionDate: v.CollectionDate, IssueDate: v.IssueDate, Subject: v.Subject, Note: v.Note, Locator: v.Locator, Uncertain: v.Uncertain}
}

var identifierHeader = regexp.MustCompile(`(?i)\b(patient (?:name|id|identifier)|name of patient|medical record(?: number)?|mrn|date of birth|d\.?o\.?b\.?|accession (?:id|number)|social security|home address|e-?mail|telephone)\s*[:#=]|\b[^\s@]+@[^\s@]+\.[a-z]{2,}\b`)

func (e *Encounter) ReportPreview(doc string, rows []string) ([]ReportExtract, error) {
	d := e.document(doc)
	if d == nil || d.BelongsTo != "self" {
		return nil, errors.New("Choose a report confirmed as your own")
	}
	if len(rows) > 100 {
		return nil, errors.New("Select up to 100 reviewed entries at a time")
	}
	out := []ReportExtract{}
	seen := map[string]bool{}
	for _, id := range rows {
		r := e.currentReview(doc, id)
		if r == nil || seen[id] {
			return nil, errors.New("Preview requires distinct active reviewed rows")
		}
		seen[id] = true
		x := extract(*r)
		if identifierHeader.MatchString(strings.Join([]string{x.Quote, x.Name, x.Value, x.Units, x.ReferenceRange, x.Flag, x.CollectionDate, x.IssueDate, x.Note, x.Locator}, " ")) {
			return nil, errors.New("Remove identifying headers from the selected passage and fields before model disclosure")
		}
		out = append(out, x)
	}
	return out, nil
}

// HandoffWarnings preserves every notice while keeping an unreviewed original
// passage inside the encounter. The owner-only provenance appendix retains it.
func (e *Encounter) HandoffWarnings() []Warning {
	warnings := append([]Warning{}, e.Warnings...)
	for i := range warnings {
		w := &warnings[i]
		if w.DocumentID == "" {
			continue
		}
		w.Quote = "Original report passage retained in the private encounter."
		d := e.document(w.DocumentID)
		if d == nil || d.BelongsTo != "self" {
			continue
		}
		r := e.currentReview(w.DocumentID, w.RowID)
		if r != nil && !r.Withdrawn {
			quote := r.Row.Excerpt
			if w.Rule == "report_review_comment" {
				quote = r.Row.Note
			}
			if quote != "" && !identifierHeader.MatchString(quote) {
				w.Quote = quote
			}
		}
	}
	return warnings
}

func documentUrgentSign(d SourceDocument, r ReportRow) bool {
	if r.Subject == "family" || r.Subject == "other" {
		return false
	}
	date := r.IssueDate
	if date == "" {
		date = d.IssueDate
	}
	if when, err := time.Parse(time.DateOnly, date); err == nil && time.Since(when) > 48*time.Hour {
		return false
	}
	for _, s := range screenText(r.Original, fromUser) {
		clause := clauseAt(r.Original, s.start)
		if s.rule == "reported_warning" && documentCurrent.MatchString(clause) && !conditionalPrinted.MatchString(clause) {
			return true
		}
	}
	return false
}

// An owner may add a new personal concern while correcting a transcription.
// Keep that warning distinct from a printed flag on the same row.
func (e *Encounter) screenReviewComment(d SourceDocument, r ReportRow, emergency string) {
	var quote string
	for _, s := range screenText(r.Note, fromUser) {
		clause := clauseAt(r.Note, s.start)
		if s.rule == "reported_warning" && firstPersonIn(clause) && !conditionalPrinted.MatchString(clause) {
			quote = strings.TrimSpace(clause)
			break
		}
	}
	for i := range e.Warnings {
		w := &e.Warnings[i]
		if w.Active && w.DocumentID == d.ID && w.RowID == r.ID && w.Rule == "report_review_comment" {
			if w.Quote == quote {
				return
			}
			w.Active = false
			w.ResolvedRevision = e.HistoryRevision
		}
	}
	if quote == "" {
		return
	}
	msg := "Your review comment may describe an urgent symptom. Seek local emergency care immediately. Do not wait to finish this interview."
	if emergency != "" {
		msg += " " + strings.TrimSpace(emergency)
	}
	e.Warnings = append(e.Warnings, Warning{ID: id("w", len(e.Warnings)), DocumentID: d.ID, RowID: r.ID, Evidence: Evidence{Quote: quote}, Rule: "report_review_comment", PolicyVersion: PolicyVersion, Message: msg, Active: true, CreatedAt: time.Now().UTC()})
}
