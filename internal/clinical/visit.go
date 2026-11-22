package clinical

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

// VisitDetail belongs to the verbatim answer it describes. ReviewIDs on that
// answer control disclosure and invalidation when a source document changes.
type VisitDetail struct {
	FeedbackFrom   string    `json:"feedbackFrom,omitempty"`
	FeedbackText   string    `json:"feedbackText,omitempty"`
	DiaryID        string    `json:"diaryId,omitempty"`
	Speaker        string    `json:"speaker,omitempty"`
	Onset          string    `json:"onset,omitempty"`
	End            string    `json:"end,omitempty"`
	Approximate    bool      `json:"approximate,omitempty"`
	FirstRecorded  time.Time `json:"firstRecorded"`
	PriorEncounter string    `json:"priorEncounter,omitempty"`
	PriorAnswer    string    `json:"priorAnswer,omitempty"`
	Printed        string    `json:"printed,omitempty"`
	Prescribed     string    `json:"prescribed,omitempty"`
}
type ClinicianFeedback struct {
	Speaker  string    `json:"speaker"`
	Category string    `json:"category"`
	Text     string    `json:"text"`
	Revision int       `json:"revision"`
	At       time.Time `json:"at"`
}
type ReturnFact struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Note   string `json:"note"`
}
type ReturnClarification struct {
	Index int    `json:"index"`
	Words string `json:"words"`
}
type ReturnVisit struct {
	Clarifications []ReturnClarification `json:"clarifications,omitempty"`
	ReviewIDs      []string              `json:"reviewIds,omitempty"`
	DiaryIDs       []string              `json:"diaryIds,omitempty"`
	Source         string                `json:"source"`
	Revision       int                   `json:"revision"`
	Facts          []ReturnFact          `json:"facts"`
	Entries        []string              `json:"entries"`
	Consent        bool                  `json:"consent"`
}

func (e *Encounter) StartReturn(prior *Encounter, v ReturnVisit, emergency string) error {
	if prior == nil || prior.ID != v.Source || prior.Revision != v.Revision || !v.Consent || prior.ConfirmedRevision != prior.HistoryRevision || e.Demo != prior.Demo || len(v.Facts)+len(v.Entries)+len(v.ReviewIDs)+len(v.Clarifications) > 48 {
		return errors.New("Review the current confirmed prior visit and its selected destination")
	}
	e.Pending = ""
	if err := e.JoinEpisode(prior); err != nil {
		return err
	}
	if err := e.copyReturnReviews(prior, v.ReviewIDs, emergency); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, item := range v.Facts {
		if seen[item.ID] {
			return errors.New("Duplicate earlier finding")
		}
		seen[item.ID] = true
		f := prior.fact(item.ID)
		if f == nil {
			return errors.New("Earlier finding unavailable")
		}
		if f.Origin != nil && f.Origin.ReconciliationStatus == "no_longer_applies" && item.Status == "still_applies" {
			return errors.New("Describe how this resolved finding changed")
		}
		if !slices.Contains([]string{"still_applies", "changed", "no_longer_applies", "uncertain"}, item.Status) {
			return errors.New("Select the current status")
		}
		note := strings.TrimSpace(item.Note)
		if note == "" && item.Status != "changed" {
			note = strings.ReplaceAll(item.Status, "_", " ")
		}
		if err := e.ReconcileFact(prior, Command{FactID: item.ID, Date: time.Now().UTC().Format(time.DateOnly), Text: note, ReconciliationStatus: item.Status, Category: "event"}, emergency); err != nil {
			return err
		}
	}
	for _, aid := range v.Entries {
		a := prior.answer(aid)
		if seen[aid] || a == nil || a.SourceOutdated || len(a.ReviewIDs) > 0 || a.Detail == nil || a.Kind != "episode_log" && a.Kind != "witness" {
			return errors.New("Select a current episode or witness entry")
		}
		seen[aid] = true
		e.changed()
		eventDate := a.EventDate
		if a.Kind == "witness" && eventDate == "" {
			eventDate = a.Detail.FirstRecorded.UTC().Format(time.DateOnly)
		}
		newAnswer := e.appendDatedAnswer(a.Kind, a.Question, a.Text, "", eventDate, emergency)
		d := *a.Detail
		d.PriorEncounter = prior.ID
		d.PriorAnswer = aid
		e.answer(newAnswer.ID).Detail = &d
	}
	used := map[int]bool{}
	for _, c := range v.Clarifications {
		if c.Index < 0 || c.Index >= len(prior.Feedback) || used[c.Index] || !validText(c.Words, 1000) {
			return errors.New("Review and explain each selected clarification")
		}
		feedback := prior.Feedback[c.Index]
		if feedback.Revision != prior.HistoryRevision {
			return errors.New("This feedback refers to an earlier history revision")
		}
		used[c.Index] = true
		e.changed()
		a := e.appendAnswer("patient_clarification", "What would you clarify after feedback on the earlier brief?", strings.TrimSpace(c.Words), "", emergency)
		e.answer(a.ID).Detail = &VisitDetail{FirstRecorded: a.CreatedAt, PriorEncounter: prior.ID, FeedbackText: feedback.Text, FeedbackFrom: feedback.Speaker}
	}
	e.ReturnSource = prior.ID
	e.routeInterview()
	if e.ScopeReason == "" && !e.hasWarning() {
		e.QuestionReviewIDs = nil
		e.Choices = nil
		e.Question = "What has changed since your last visit that matters most today?"
		e.QuestionSource = &QuestionSource{Key: "return_change", Reason: "Your earlier confirmed findings are available for comparison."}
		e.Pending = ""
	}
	return nil
}

// Only the selected reviewed passages are approved for use in this encounter.
func (e *Encounter) copyReturnReviews(prior *Encounter, ids []string, emergency string) error {
	selected := map[string]bool{}
	for _, id := range ids {
		if selected[id] {
			return errors.New("Duplicate return visit passage")
		}
		selected[id] = true
	}
	for _, doc := range prior.Documents {
		if doc.BelongsTo != "self" {
			continue
		}
		dest := doc
		dest.Rows = nil
		dest.PriorEncounter = prior.ID
		dest.PriorRevision = prior.HistoryRevision
		var rows []ReportRow
		var rowIDs []string
		for _, r := range prior.ReportReviews {
			if !r.Active || r.Withdrawn || r.DocumentID != doc.ID || r.Row.Subject != "self" || !selected[r.ID] {
				continue
			}
			src := doc.row(r.RowID)
			if src == nil {
				return errors.New("Original row unavailable")
			}
			dest.Rows = append(dest.Rows, *src)
			rows = append(rows, r.Row)
			rowIDs = append(rowIDs, r.RowID)
			delete(selected, r.ID)
		}
		if len(rows) == 0 {
			continue
		}
		if err := e.AttachReport(dest, emergency); err != nil {
			return err
		}
		for _, r := range rows {
			if err := e.applyReport(Command{Action: "review_report_row", DocumentID: doc.ID, RowID: r.ID, Row: r}, emergency); err != nil {
				return err
			}
		}
		if err := e.applyReport(Command{Action: "authorize_report", DocumentID: doc.ID, RowIDs: rowIDs}, emergency); err != nil {
			return err
		}
	}
	if len(selected) != 0 {
		return errors.New("Select current user-reviewed passages")
	}
	return nil
}
func (e *Encounter) askEventTiming() bool {
	for i := len(e.Answers) - 1; i >= 0; i-- {
		a := e.Answers[i]
		if !a.Active || a.SourceOutdated || (a.Kind != "resolve_sources" && a.Kind != "medication_use" && !(a.QuestionSource != nil && a.QuestionSource.Key == "medication_start")) || a.EventDate == "" || !allApproved(a.ReviewIDs, e.disclosed()) {
			continue
		}
		if slices.ContainsFunc(e.Answers, func(reply Answer) bool {
			return reply.Active && !reply.SourceOutdated && reply.QuestionSource != nil && reply.QuestionSource.Key == "event_timing" && reply.QuestionSource.VisitAnswerID == a.ID
		}) {
			continue
		}
		e.Question = fmt.Sprintf("You dated this event to %s. When did your current concern start in relation to it?", a.EventDate)
		e.QuestionSource = &QuestionSource{Key: "event_timing", VisitAnswerID: a.ID, Reason: "The order of these events could change how your clinician interprets them."}
		e.QuestionReviewIDs = slices.Clone(a.ReviewIDs)
		e.Choices = []string{"Before the event", "After the event", "Around the same time"}
		e.State = "interviewing"
		e.Pending = ""
		return true
	}
	return false
}

var medicineChange = regexp.MustCompile(`(?i)\b(medicine|medication|tablet|dose|prescription|supplement)\b`)
var reportedChange = regexp.MustCompile(`(?i)\b(started|stopped|switched|changed|increased|decreased|new|first dose)\b`)
var eventOrder = regexp.MustCompile(`(?i)\b(before|after|since taking|since starting)\b`)

func (e *Encounter) askFocusedQuestion() bool {
	if !e.Focused {
		return false
	}
	text := e.Concern
	answered := map[string]bool{}
	for _, a := range e.Answers {
		if !a.Active || a.SourceOutdated || a.Kind != "reply" {
			continue
		}
		text += " " + a.Text
		answered[a.Question] = true
	}
	e.askPlanOpening()
	order := "How does the timing of your symptoms compare with when you took the medicine?"
	course := "How has this changed since you first noticed it?"
	impact := "How is this affecting what you can usually do?"
	if medicineChange.MatchString(text) && reportedChange.MatchString(text) && !eventOrder.MatchString(text) && !answered[order] {
		e.Question = order
		e.Choices = nil
		e.QuestionSource.Reason = "The order of events can change how your clinician interprets the symptoms."
	} else if answered[e.Question] {
		switch {
		case !answered[course] && !openingCourse.MatchString(text):
			e.Question = course
			e.Choices = nil
			e.QuestionSource.Reason = "The course helps identify what needs attention first."
		case !answered[impact] && !openingImpact.MatchString(text):
			e.Question = impact
			e.Choices = nil
			e.QuestionSource.Reason = "Changes in what you can do help describe the impact."
		default:
			e.Question = ""
			e.QuestionSource = nil
			e.Choices = nil
			e.State = "history_review"
			e.Pending = "interview"
			e.ReviewExtraction = true
		}
	}
	return true
}
func (e *Encounter) focusedLimitReached() bool {
	if !e.Focused {
		return false
	}
	count := 0
	for _, a := range e.Answers[min(e.FocusedFrom, len(e.Answers)):] {
		if a.Active && a.Kind == "reply" {
			count++
		}
	}
	return count >= 2
}
func laterEpisode(a, b Answer) bool {
	x, _ := time.Parse(time.RFC3339, a.Detail.Onset)
	y, _ := time.Parse(time.RFC3339, b.Detail.Onset)
	return x.After(y) || x.Equal(y) && a.Detail.FirstRecorded.After(b.Detail.FirstRecorded)
}
func (e *Encounter) askEpisodeChange() bool {
	logs := 0
	var latest Answer
	var compared string
	for _, a := range e.Answers {
		if !a.Active || a.SourceOutdated {
			continue
		}
		if a.Kind == "episode_log" {
			logs++
			if a.Detail != nil && (latest.Detail == nil || laterEpisode(a, latest)) {
				latest = a
			}
		}
		if a.QuestionSource != nil && a.QuestionSource.Key == "episode_change" {
			compared = a.QuestionSource.VisitAnswerID
		}
	}
	if logs < 2 || latest.ID == compared {
		return false
	}
	e.Question = fmt.Sprintf("You recorded %d episodes. What was different about the most recent one?", logs)
	e.Choices = []string{"It felt similar", "I noticed a change"}
	e.QuestionSource = &QuestionSource{Key: "episode_change", VisitAnswerID: latest.ID, Reason: "Comparing these episodes can clarify a change in their pattern."}
	e.State = "interviewing"
	e.Pending = ""
	return true
}
func (e *Encounter) visitEntry(c Command, emergency string) error {
	if len(e.Answers) >= 256 || !validText(c.Text, 3000) {
		return errors.New("Enter a short account of up to 3000 bytes")
	}
	if c.Action == "clinician_feedback" {
		if !validText(c.Speaker, 80) || !c.ConfirmedDestination || c.ForRevision != e.HistoryRevision || e.ConfirmedRevision != e.HistoryRevision || len(e.Feedback) >= 64 || !slices.Contains([]string{"retake", "incorrect", "changed_question"}, c.Category) {
			return errors.New("Review feedback against the current confirmed history")
		}
		e.Feedback = append(e.Feedback, ClinicianFeedback{strings.TrimSpace(c.Speaker), c.Category, strings.TrimSpace(c.Text), e.HistoryRevision, time.Now().UTC()})
		return nil
	}
	if e.Demo {
		return errors.New("Choose a clinical provider for a personal observation")
	}
	d := VisitDetail{FirstRecorded: time.Now().UTC()}
	var date, question string
	if len(c.ReviewIDs) > 12 {
		return errors.New("Select up to twelve source entries")
	}
	allowed := e.disclosed()
	for _, id := range c.ReviewIDs {
		row, ok := allowed[id]
		if !ok || row.Subject != "self" {
			return errors.New("Review and authorize each selected source for this destination")
		}
	}
	switch c.Action {
	case "episode_log":
		start, err := time.Parse(time.RFC3339, c.Onset)
		if err != nil || start.After(time.Now().Add(5*time.Minute)) {
			return errors.New("Choose an episode start in the past or present")
		}
		if c.End != "" {
			end, err := time.Parse(time.RFC3339, c.End)
			if err != nil || end.Before(start) || end.After(time.Now().Add(5*time.Minute)) {
				return errors.New("Check the episode end time")
			}
		}
		date = start.Format(time.DateOnly)
		d.Onset = c.Onset
		d.End = c.End
		d.Approximate = c.Approximate
		question = "What happened during this episode?"
	case "witness":
		if e.Age < 18 || !validText(c.Speaker, 80) || !c.ConfirmedDestination {
			return errors.New("Confirm sharing a separately attributed witness observation for this adult")
		}
		d.Speaker = strings.TrimSpace(c.Speaker)
		question = "Witness observation from " + d.Speaker
	case "medication_use":
		if len(c.ReviewIDs) != 1 || !c.ConfirmedDestination || len(c.Prescribed) > 500 {
			return errors.New("Choose one reviewed label and confirm the account of actual use")
		}
		row := allowed[c.ReviewIDs[0]]
		if doc := e.document(row.DocumentID); doc == nil || !doc.MedicationLabel {
			return errors.New("Select a reviewed medication label")
		}
		d.Printed = row.Quote
		d.Prescribed = strings.TrimSpace(c.Prescribed)
		if c.Date != "" && !isoDay(c.Date) {
			return errors.New("Check the date actual use changed")
		}
		date = c.Date
		question = "How do you actually take the medicine on this label?"
	case "resolve_sources":
		if len(c.ReviewIDs) < 2 || !c.ConfirmedDestination {
			return errors.New("Choose two source passages and confirm your recollection")
		}
		docs := map[string]bool{}
		for _, id := range c.ReviewIDs {
			docs[allowed[id].DocumentID] = true
		}
		if len(docs) < 2 {
			return errors.New("Select passages from two different documents")
		}
		if c.Date != "" && !isoDay(c.Date) {
			return errors.New("Check the event date")
		}
		date = c.Date
		question = "How do you remember the difference between these records?"
	default:
		return errors.New("Unknown visit entry")
	}
	e.changed()
	a := e.appendDatedAnswer(c.Action, question, strings.TrimSpace(c.Text), "", date, emergency)
	saved := e.answer(a.ID)
	saved.Detail = &d
	saved.ReviewIDs = slices.Clone(c.ReviewIDs)
	e.reportState()
	return nil
}

// Match a medication only when its reviewed name occurs verbatim in a second
// passage describing use. Never map a foreign brand or guess an equivalent.
func (e *Encounter) sourceQuestion(a, b ReportExtract) (string, string) {
	if a.CollectionDate != "" && a.CollectionDate == b.CollectionDate && a.Units == b.Units && a.Name != "" && strings.EqualFold(a.Name, b.Name) && a.Value != "" && b.Value != "" && !samePrintedValue(a.Value, b.Value) {
		return "source_difference", fmt.Sprintf("Two records list different values for %s on %s. What do you remember about the difference?", a.Name, a.CollectionDate)
	}
	for _, pair := range [][2]ReportExtract{{a, b}, {b, a}} {
		label, note := pair[0], pair[1]
		d := e.document(label.DocumentID)
		if d == nil || !d.MedicationLabel || len(strings.TrimSpace(label.Name)) < 3 || !reportedChange.MatchString(note.Quote) {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(label.Name))
		if slices.Contains([]string{"tablet", "tablets", "capsule", "capsules", "medicine", "medication", "dose", "drug"}, name) {
			continue
		}
		// Spaces at both ends prevent a partial brand match.
		words := func(s string) string {
			return " " + strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }), " ") + " "
		}
		if strings.Contains(words(note.Quote), words(name)) {
			return "medication_start", fmt.Sprintf("The two records mention %s. When, if ever, did you actually start taking it?", label.Name)
		}
	}
	return "", ""
}
func (e *Encounter) askSourceDifference() bool {
	allowed := e.disclosed()
	for i, left := range e.ReportReviews {
		a, ok := allowed[left.ID]
		if !ok || a.Subject != "self" {
			continue
		}
		for _, right := range e.ReportReviews[i+1:] {
			b, ok := allowed[right.ID]
			if !ok || b.Subject != "self" || a.DocumentID == b.DocumentID {
				continue
			}
			key, question := e.sourceQuestion(a, b)
			if key == "" {
				continue
			}
			addressed := slices.ContainsFunc(e.Answers, func(answer Answer) bool {
				return answer.Active && !answer.SourceOutdated && (answer.Kind == "resolve_sources" || answer.QuestionSource != nil && answer.QuestionSource.Key == key) && slices.Contains(answer.ReviewIDs, a.ReviewID) && slices.Contains(answer.ReviewIDs, b.ReviewID)
			})
			if addressed {
				continue
			}
			e.Question = question
			e.QuestionReviewIDs = []string{a.ReviewID, b.ReviewID}
			e.QuestionSource = &QuestionSource{Key: key, DocumentID: a.DocumentID, RowID: a.RowID, ReviewID: a.ReviewID, Reason: "Your recollection can clarify the timing and wording in these two records."}
			e.State = "interviewing"
			e.Pending = ""
			e.Choices = nil
			return true
		}
	}
	return false
}

type DiaryEntry struct {
	ID          string    `json:"id"`
	Text        string    `json:"text"`
	Onset       string    `json:"onset"`
	End         string    `json:"end,omitempty"`
	Approximate bool      `json:"approximate"`
	Recorded    time.Time `json:"recordedAt"`
}

func (d DiaryEntry) Valid() bool {
	start, err := time.Parse(time.RFC3339, d.Onset)
	if err != nil || start.After(time.Now().Add(5*time.Minute)) || !validText(d.Text, 3000) {
		return false
	}
	if d.End != "" {
		end, err := time.Parse(time.RFC3339, d.End)
		return err == nil && !end.Before(start) && !end.After(time.Now().Add(5*time.Minute))
	}
	return true
}
func (e *Encounter) IncludeDiary(d DiaryEntry, emergency string) error {
	if err := e.visitEntry(Command{Action: "episode_log", Text: d.Text, Onset: d.Onset, End: d.End, Approximate: d.Approximate}, emergency); err != nil {
		return err
	}
	a := &e.Answers[len(e.Answers)-1]
	a.Detail.FirstRecorded = d.Recorded
	a.Detail.DiaryID = d.ID
	if !e.ReportFirst {
		e.routeInterview()
	}
	return nil
}
