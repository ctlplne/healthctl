package clinical

import (
	"errors"
	"strings"
	"time"
)

// ReuseFact copies a specific reviewed finding with its original source and
// timing. No medication, illness or allergy silently carries into another visit.
func (e *Encounter) ReuseFact(source *Encounter, factID, date, note, emergency string) error {
	return e.ReconcileFact(source, Command{FactID: factID, Date: date, Text: note, Category: "other", ReconciliationStatus: "historical"}, emergency)
}

func (e *Encounter) JoinEpisode(source *Encounter) error {
	if e.Pending != "" {
		return errors.New("Finish or cancel the current request before changing the episode")
	}
	if source == nil || e.ID == source.ID || e.Demo != source.Demo {
		return errors.New("Choose a different encounter in the same workspace")
	}
	e.EpisodeID, e.EpisodeTitle = source.EpisodeID, source.EpisodeTitle
	if e.EpisodeID == "" {
		e.EpisodeID = source.ID
		e.EpisodeTitle = source.Concern
	}
	e.touch()
	return nil
}

func (e *Encounter) ReconcileFact(source *Encounter, c Command, emergency string) error {
	factID, date, note := c.FactID, c.Date, c.Text
	if note == "" {
		note = c.Note
	}
	if c.Category == "" {
		c.Category = "other"
	}
	if c.ReconciliationStatus == "" {
		c.ReconciliationStatus = "historical"
	}
	switch c.Category {
	case "medication", "allergy", "family_history", "event", "other":
	default:
		return errors.New("Choose a supported history category")
	}
	switch c.ReconciliationStatus {
	case "still_applies", "changed", "no_longer_applies", "uncertain", "historical":
	default:
		return errors.New("Choose how this history applies on the review date")
	}
	if source == nil || e.ID == source.ID || e.Demo != source.Demo || !isoDay(date) || !validText(note, 1000) {
		return errors.New("Choose a different encounter, a review date and how the earlier finding applies")
	}
	old := source.fact(factID)
	if old == nil || !old.Confirmed {
		return errors.New("Choose an active patient-confirmed source finding")
	}
	if a := source.answer(old.AnswerID); a != nil && len(a.ReviewIDs) > 0 {
		return errors.New("Review the report-dependent passage in this encounter before using it")
	}
	if old.SourceType == "report_entry" {
		return errors.New("Review and explicitly select the original passage in this encounter before using report evidence")
	}
	var previous *Fact
	for i := range e.Facts {
		f := &e.Facts[i]
		if !f.Active || f.Origin == nil || f.Origin.EncounterID != source.ID || f.Origin.FactID != factID {
			continue
		}
		if f.Origin.Date == date && f.Origin.Note == note && f.Origin.Category == c.Category && f.Origin.ReconciliationStatus == c.ReconciliationStatus {
			return errors.New("This reconciliation is already recorded")
		}
		previous = f
		break
	}
	e.changed()
	origin := FactOrigin{Category: c.Category, ReconciliationStatus: c.ReconciliationStatus, OriginalStatus: old.Status, OriginalTimeContext: old.TimeContext, EncounterID: source.ID, FactID: old.ID, HistoryRevision: old.HistoryRevision, Date: date, Note: note, SourceType: old.SourceType, DocumentID: old.DocumentID, RowID: old.RowID}
	f := *old
	f.ID = id("f", len(e.Facts))
	f.SourceType = "reconciled_history"
	f.Origin = &origin
	f.Supersedes = ""
	f.Corrected = false
	f.Confirmed = false
	if c.ReconciliationStatus == "uncertain" {
		f.Status = "uncertain"
	}
	f.AnswerID = ""
	f.HistoryRevision = e.HistoryRevision
	f.TimeContext = "Reported in the encounter dated " + source.CreatedAt.UTC().Format(time.DateOnly)
	if origin.OriginalTimeContext != "" {
		f.TimeContext += " (" + origin.OriginalTimeContext + " at that visit)"
	}
	if previous != nil {
		if date >= previous.Origin.Date {
			previous.Active = false
			f.Supersedes = previous.ID
		} else {
			f.Active = false
		}
	}
	e.Facts = append(e.Facts, f)
	e.appendDatedAnswer("reconciliation", "How does the earlier "+c.Category+" finding apply on "+date+"?", note, "", date, emergency)
	e.rebuildContradictions()
	e.reportState()
	e.touch()
	return nil
}

func (e *Encounter) addOutcome(c Command, emergency string) error {
	if !isoDay(c.Date) || !validText(c.Text, 3000) || len(e.Outcomes) >= 150 {
		return errors.New("Enter the follow-up date and a brief account of the outcome")
	}
	if c.Source != "symptom_change" && e.Pending != "" {
		return errors.New("Finish or cancel the current request before adding a visit outcome")
	}
	var rowID, locator string
	switch c.Source {
	case "documented":
		d := e.document(c.DocumentID)
		if d == nil || d.BelongsTo != "self" {
			return errors.New("Choose the source document for this documented outcome")
		}
		found := false
		for _, r := range d.Rows {
			if (c.RowID == "" || c.RowID == r.ID) && strings.Contains(r.Original, c.Text) {
				if found {
					return errors.New("Use a longer exact passage or select its row to disambiguate the source")
				}
				found = true
				rowID, locator = r.ID, r.Locator
			}
		}
		if !found {
			return errors.New("Use an exact passage from the source, or label this a patient-reported outcome")
		}
	case "patient_reported", "symptom_change":
		if c.DocumentID != "" {
			return errors.New("Select the documented source type to link a report")
		}
	default:
		return errors.New("Choose how the outcome was established")
	}
	o := VisitOutcome{RowID: rowID, Locator: locator, ID: id("out", len(e.Outcomes)), Date: c.Date, Source: c.Source, Text: c.Text, DocumentID: c.DocumentID, AssessmentVersion: len(e.Assessments), HistoryRevision: e.HistoryRevision, CreatedAt: time.Now().UTC()}
	e.Outcomes = append(e.Outcomes, o)
	if c.Source == "symptom_change" {
		e.changed()
		e.appendDatedAnswer("followup", "What changed after the visit on "+c.Date+"?", c.Text, "", c.Date, emergency)
		e.reportState()
	}
	return nil
}
