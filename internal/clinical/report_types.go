package clinical

import "time"

// A SourceDocument identifies a local original. The file and full extraction are
// outside the aggregate. A row is a candidate transcription, never a diagnosis.
type SourceDocument struct {
	PriorEncounter  string      `json:"priorEncounter,omitempty"`
	PriorRevision   int         `json:"priorRevision,omitempty"`
	MedicationLabel bool        `json:"medicationLabel,omitempty"`
	ID              string      `json:"id"`
	Title           string      `json:"title"`
	Kind            string      `json:"kind"`
	BelongsTo       string      `json:"belongsTo"`
	Purpose         string      `json:"purpose"`
	Priority        string      `json:"priority"`
	IssueDate       string      `json:"issueDate"`
	Rows            []ReportRow `json:"rows"`
	CreatedAt       time.Time   `json:"createdAt"`
}

type ReportRow struct {
	ID             string `json:"id"`
	Original       string `json:"original"`
	Locator        string `json:"locator"`
	Excerpt        string `json:"excerpt,omitempty"`
	Name           string `json:"name"`
	Value          string `json:"value"`
	Units          string `json:"units"`
	ReferenceRange string `json:"referenceRange"`
	Flag           string `json:"flag"`
	CollectionDate string `json:"collectionDate"`
	IssueDate      string `json:"issueDate"`
	Subject        string `json:"subject"`
	Note           string `json:"note"`
	Uncertain      bool   `json:"uncertain"`
	Manual         bool   `json:"manual,omitempty"`
}

type ReportReview struct {
	RetiredRevision int          `json:"retiredRevision,omitempty"`
	ID              string       `json:"id"`
	DocumentID      string       `json:"documentId"`
	RowID           string       `json:"rowId"`
	Row             ReportRow    `json:"row"`
	Active          bool         `json:"active"`
	Withdrawn       bool         `json:"withdrawn"`
	HistoryRevision int          `json:"historyRevision"`
	FactID          string       `json:"factId"`
	Supersedes      string       `json:"supersedes,omitempty"`
	ApprovedFor     *ProviderRef `json:"approvedFor,omitempty"`
	CreatedAt       time.Time    `json:"createdAt"`
}

type ReportExtract struct {
	FactID         string `json:"factId"`
	DocumentID     string `json:"documentId"`
	RowID          string `json:"rowId"`
	ReviewID       string `json:"reviewId"`
	Quote          string `json:"quote"`
	Name           string `json:"name"`
	Value          string `json:"value"`
	Units          string `json:"units"`
	ReferenceRange string `json:"referenceRange"`
	Flag           string `json:"flag"`
	CollectionDate string `json:"collectionDate"`
	IssueDate      string `json:"issueDate"`
	Subject        string `json:"subject"`
	Note           string `json:"note"`
	Locator        string `json:"locator"`
	Uncertain      bool   `json:"uncertain"`
}

type QuestionSource struct {
	VisitAnswerID string `json:"visitAnswerId,omitempty"`
	EntryLabel    string `json:"entryLabel,omitempty"`
	AnswerHint    string `json:"answerHint,omitempty"`
	DocumentID    string `json:"documentId,omitempty"`
	RowID         string `json:"rowId,omitempty"`
	ReviewID      string `json:"reviewId,omitempty"`
	Reason        string `json:"reason"`
	Pathway       string `json:"pathway,omitempty"`
	Key           string `json:"key,omitempty"`
	Topic         string `json:"topic,omitempty"`
	// ModelNote keeps the model's own note about a question it proposed. The
	// app writes Reason; the note is shown only as a labeled model suggestion.
	ModelNote string `json:"modelNote,omitempty"`
}

type FactOrigin struct {
	Category             string `json:"category,omitempty"`
	ReconciliationStatus string `json:"reconciliationStatus,omitempty"`
	OriginalStatus       string `json:"originalStatus,omitempty"`
	OriginalTimeContext  string `json:"originalTimeContext,omitempty"`
	EncounterID          string `json:"encounterId"`
	FactID               string `json:"factId"`
	HistoryRevision      int    `json:"historyRevision"`
	Date                 string `json:"date"`
	Note                 string `json:"note"`
	SourceType           string `json:"sourceType"`
	DocumentID           string `json:"documentId,omitempty"`
	RowID                string `json:"rowId,omitempty"`
}

type VisitOutcome struct {
	RowID             string    `json:"rowId,omitempty"`
	Locator           string    `json:"locator,omitempty"`
	ID                string    `json:"id"`
	Date              string    `json:"date"`
	Source            string    `json:"source"`
	Text              string    `json:"text"`
	DocumentID        string    `json:"documentId,omitempty"`
	AssessmentVersion int       `json:"assessmentVersion"`
	HistoryRevision   int       `json:"historyRevision"`
	CreatedAt         time.Time `json:"createdAt"`
}

type Pathway struct {
	ID           string   `json:"id"`
	Version      string   `json:"version"`
	ReviewStatus string   `json:"reviewStatus"`
	Context      []string `json:"context"`
}

type ReportConsent struct {
	DocumentID      string      `json:"documentId"`
	ReviewIDs       []string    `json:"reviewIds"`
	Provider        ProviderRef `json:"provider"`
	HistoryRevision int         `json:"historyRevision"`
	At              time.Time   `json:"at"`
}

type ReportIdentityChange struct {
	DocumentID      string    `json:"documentId"`
	Previous        string    `json:"previous"`
	Current         string    `json:"current"`
	HistoryRevision int       `json:"historyRevision"`
	At              time.Time `json:"at"`
}
