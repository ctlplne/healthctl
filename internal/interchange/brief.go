// Package interchange produces bounded handoffs from the current encounter revision.
package interchange

import (
	"time"

	"github.com/ctlplne/healthctl/internal/clinical"
)

type Brief struct {
	Agenda            string                       `json:"agenda,omitempty"`
	Changes           []clinical.Answer            `json:"changes,omitempty"`
	Feedback          []clinical.ClinicianFeedback `json:"feedback,omitempty"`
	Format            string                       `json:"format"`
	ID                string                       `json:"id"`
	HistoryRevision   int                          `json:"historyRevision"`
	AssessmentVersion int                          `json:"assessmentVersion"`
	Confirmed         bool                         `json:"confirmed"`
	Concern           string                       `json:"concern"`
	Age               int                          `json:"age"`
	UpdatedAt         time.Time                    `json:"updatedAt"`
	Notice            string                       `json:"notice"`
	ModelGuidance     string                       `json:"modelGuidance"`
	ReviewStatus      string                       `json:"reviewStatus"`
	Demo              bool                         `json:"demo"`
	Warnings          []clinical.Warning           `json:"warnings"`
	Facts             []clinical.Fact              `json:"facts"`
	ReportFindings    []clinical.ReportExtract     `json:"reportFindings"`
	Outstanding       []string                     `json:"outstanding"`
	Contradictions    []clinical.Contradiction     `json:"contradictions"`
	Assessment        *clinical.Assessment         `json:"assessment,omitempty"`
	Outcomes          []clinical.VisitOutcome      `json:"outcomes"`
	Priorities        []string                     `json:"priorities"`
	ScopeReason       string                       `json:"scopeReason"`
}

func FromEncounter(e *clinical.Encounter) Brief {
	b := Brief{
		Agenda: e.Agenda, Feedback: e.Feedback, Format: "clinical-history/brief-1", ID: e.ID, HistoryRevision: e.HistoryRevision,
		Confirmed: e.ConfirmedRevision == e.HistoryRevision, Concern: e.Concern, Age: e.Age, UpdatedAt: e.UpdatedAt,
		Notice: e.Notice, ModelGuidance: clinical.ModelGuidance(), ReviewStatus: e.ReviewStatus, Demo: e.Demo, Warnings: e.HandoffWarnings(),
		Facts: []clinical.Fact{}, ReportFindings: []clinical.ReportExtract{},
		Outstanding: e.Outstanding, Contradictions: e.Contradictions, ScopeReason: e.ScopeReason,
		Outcomes: append([]clinical.VisitOutcome{}, e.Outcomes...), Priorities: []string{},
	}
	for _, a := range e.Answers {
		if !a.Active || a.SourceOutdated {
			continue
		}
		if a.Detail != nil || a.Kind == "reconciliation" || a.QuestionSource != nil && a.QuestionSource.Key == "return_change" {
			b.Changes = append(b.Changes, a)
		}
	}
	for _, f := range e.Facts {
		if f.Active {
			b.Facts = append(b.Facts, f)
		}
	}
	for _, d := range e.Documents {
		if d.BelongsTo == "self" && d.Priority != "" {
			b.Priorities = append(b.Priorities, d.Priority)
		}
	}
	for _, review := range e.ReportReviews {
		if !review.Active || review.Withdrawn {
			continue
		}
		row := review.Row
		quote := row.Excerpt
		if quote == "" && !row.Manual {
			quote = row.Original
		}
		b.ReportFindings = append(b.ReportFindings, clinical.ReportExtract{FactID: review.FactID, DocumentID: review.DocumentID, RowID: review.RowID, ReviewID: review.ID, Quote: quote, Name: row.Name, Value: row.Value, Units: row.Units, ReferenceRange: row.ReferenceRange, Flag: row.Flag, CollectionDate: row.CollectionDate, IssueDate: row.IssueDate, Subject: row.Subject, Note: row.Note, Locator: row.Locator, Uncertain: row.Uncertain})
	}
	for i := len(e.Assessments) - 1; i >= 0; i-- {
		if e.Assessments[i].HistoryRevision == e.HistoryRevision {
			b.Assessment = &e.Assessments[i]
			b.AssessmentVersion = b.Assessment.Version
			break
		}
	}
	return b
}
