// Package clinical owns the encounter lifecycle. Storage and network I/O live outside it.
package clinical

import (
	_ "embed"
	"strings"
	"time"

	"github.com/ctlplne/healthctl/internal/evidence"
)

//go:embed model-guidance.txt
var modelGuidanceText string

// ModelGuidance is current operator advice. Historical patient consent is versioned separately.
func ModelGuidance() string { return strings.TrimSpace(modelGuidanceText) }

const (
	NoticeVersion = "2026-11-v1"
	Notice        = "Experimental AI assessment — not reviewed by a healthcare professional. This application collects your history and estimates possible explanations for your symptoms. It can be wrong or miss serious illness. This is not a confirmed diagnosis. Discuss the report with a qualified healthcare professional, and do not use it as the sole reason to delay care or start, stop, or change treatment. If you believe you have an emergency, seek local emergency care immediately. Use at your own discretion."
	ReviewStatus  = "not_clinician_reviewed"
	PolicyVersion = "development-2"
	DemoConcern   = "My nose is runny and I keep sneezing."
)

type ProviderRef struct {
	Kind       string `json:"kind"`
	Model      string `json:"model"`
	Endpoint   string `json:"endpoint"`
	Structured string `json:"structured"`
}

type Intake struct {
	GuidedHistory    bool   `json:"guidedHistory,omitempty"`
	ReportFirst      bool   `json:"reportFirst,omitempty"`
	Concern          string `json:"concern"`
	Age              int    `json:"age"`
	Background       string `json:"background"`
	PregnancyRelated bool   `json:"pregnancyRelated"`
	UrgentNow        bool   `json:"urgentNow"`
	NoticeVersion    string `json:"noticeVersion"`
}

type Command struct {
	EpisodeTitle         string      `json:"episodeTitle,omitempty"`
	Category             string      `json:"category,omitempty"`
	ReconciliationStatus string      `json:"reconciliationStatus,omitempty"`
	DocumentID           string      `json:"documentId,omitempty"`
	RowID                string      `json:"rowId,omitempty"`
	RowIDs               []string    `json:"rowIds,omitempty"`
	Row                  ReportRow   `json:"row,omitempty"`
	BelongsTo            string      `json:"belongsTo,omitempty"`
	Date                 string      `json:"date,omitempty"`
	Source               string      `json:"source,omitempty"`
	Locator              string      `json:"locator,omitempty"`
	Note                 string      `json:"note,omitempty"`
	SourceEncounterID    string      `json:"sourceEncounterId,omitempty"`
	Age                  int         `json:"age"`
	PregnancyRelated     *bool       `json:"pregnancyRelated"`
	Action               string      `json:"action"`
	Text                 string      `json:"text"`
	AnswerID             string      `json:"answerId"`
	FactID               string      `json:"factId"`
	Concept              string      `json:"concept"`
	Status               string      `json:"status"`
	Subject              string      `json:"subject"`
	TimeContext          string      `json:"timeContext"`
	UrgentNow            *bool       `json:"urgentNow"`
	Provider             ProviderRef `json:"provider"`
	ConfirmedDestination bool        `json:"confirmedDestination"`
}

type Answer struct {
	Disposition    string          `json:"disposition,omitempty"`
	SourceOutdated bool            `json:"sourceOutdated,omitempty"`
	EventDate      string          `json:"eventDate,omitempty"`
	QuestionSource *QuestionSource `json:"questionSource,omitempty"`
	ReviewIDs      []string        `json:"reviewIds,omitempty"`
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	Question       string          `json:"question"`
	Text           string          `json:"text"`
	Active         bool            `json:"active"`
	Supersedes     string          `json:"supersedes,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
}

type Evidence struct {
	AnswerID string `json:"answerId"`
	Quote    string `json:"quote"`
}

// FindingProposal is a normalized claim about an exact substring of a patient answer.
type FindingProposal struct {
	Evidence
	Concept     string `json:"concept"`
	Status      string `json:"status"`
	Subject     string `json:"subject"`
	TimeContext string `json:"timeContext"`
	Value       string `json:"value"`
	Units       string `json:"units"`
}

type Fact struct {
	SourceOutdated bool        `json:"sourceOutdated,omitempty"`
	DocumentID     string      `json:"documentId,omitempty"`
	RowID          string      `json:"rowId,omitempty"`
	ReviewID       string      `json:"reviewId,omitempty"`
	Origin         *FactOrigin `json:"origin,omitempty"`
	FindingProposal
	ID              string `json:"id"`
	SourceType      string `json:"sourceType"`
	StartByte       int    `json:"startByte"`
	EndByte         int    `json:"endByte"`
	HistoryRevision int    `json:"historyRevision"`
	Active          bool   `json:"active"`
	Confirmed       bool   `json:"confirmed"`
	Corrected       bool   `json:"corrected"`
	Supersedes      string `json:"supersedes,omitempty"`
}

type Warning struct {
	DocumentID string `json:"documentId,omitempty"`
	RowID      string `json:"rowId,omitempty"`
	FactID     string `json:"factId,omitempty"`
	Evidence
	ID               string    `json:"id"`
	Rule             string    `json:"rule"`
	PolicyVersion    string    `json:"policyVersion"`
	Message          string    `json:"message"`
	Active           bool      `json:"active"`
	CreatedAt        time.Time `json:"createdAt"`
	ResolvedRevision int       `json:"resolvedRevision,omitempty"`
}

type Contradiction struct {
	Concept string   `json:"concept"`
	FactIDs []string `json:"factIds"`
}

type Item struct {
	Name         string   `json:"name"`
	ClinicalTerm string   `json:"clinicalTerm"`
	Explanation  string   `json:"explanation"`
	Supporting   []string `json:"supporting"`
	Conflicting  []string `json:"conflicting"`
	Limitations  []string `json:"limitations"`
}

type ModelMeta struct {
	Endpoint       string         `json:"endpoint,omitempty"`
	Provider       string         `json:"provider"`
	RequestedModel string         `json:"requestedModel"`
	ReturnedModel  string         `json:"returnedModel"`
	PromptVersion  string         `json:"promptVersion"`
	SchemaVersion  string         `json:"schemaVersion"`
	AppVersion     string         `json:"appVersion"`
	Settings       map[string]any `json:"settings"`
}

type Assessment struct {
	Pathways        []Pathway          `json:"pathways,omitempty"`
	Literature      []evidence.Support `json:"literature,omitempty"`
	CorpusVersion   string             `json:"corpusVersion,omitempty"`
	ReviewIDs       []string           `json:"reviewIds,omitempty"`
	FactIDs         []string           `json:"factIds,omitempty"`
	PolicyVersion   string             `json:"policyVersion,omitempty"`
	Version         int                `json:"version"`
	HistoryRevision int                `json:"historyRevision"`
	CreatedAt       time.Time          `json:"createdAt"`
	ReviewStatus    string             `json:"reviewStatus"`
	Outcome         string             `json:"outcome"`
	Provider        ModelMeta          `json:"provider"`
	Destination     ProviderRef        `json:"destination"`
	Items           []Item             `json:"items"`
	Outstanding     []string           `json:"outstanding"`
}

type Input struct {
	BackgroundQuestions []string        `json:"backgroundQuestions,omitempty"`
	ReportEntries       []ReportExtract `json:"reportEntries,omitempty"`
	Pathways            []Pathway       `json:"pathways,omitempty"`
	ReviewOnly          bool            `json:"reviewOnly,omitempty"`
	CorrectedEvidence   []Evidence      `json:"correctedEvidence,omitempty"`
	Task                string          `json:"task"`
	Age                 int             `json:"age"`
	Concern             string          `json:"concern"`
	Background          string          `json:"background"`
	Answers             []Answer        `json:"answers"`
	Facts               []Fact          `json:"facts"`
	Contradictions      []Contradiction `json:"contradictions"`
	Outstanding         []string        `json:"outstanding"`
	ActiveWarnings      []Warning       `json:"activeWarnings"`
	HistoryRevision     int             `json:"historyRevision"`
}

type Proposal struct {
	Action         string            `json:"action"`
	Question       string            `json:"question"`
	Choices        []string          `json:"choices"`
	Reason         string            `json:"reason"`
	Findings       []FindingProposal `json:"findings"`
	UrgentEvidence []Evidence        `json:"urgentEvidence"`
	Outstanding    []string          `json:"outstanding"`
	Items          []Item            `json:"items"`
}

type Summary struct {
	Outstanding      []string        `json:"outstanding"`
	Contradictions   []Contradiction `json:"contradictions"`
	Age              int             `json:"age"`
	PregnancyRelated bool            `json:"pregnancyRelated"`
	HistoryRevision  int             `json:"historyRevision"`
	ConfirmedAt      time.Time       `json:"confirmedAt"`
	FactIDs          []string        `json:"factIds"`
	AnswerIDs        []string        `json:"answerIds"`
}

type Turn struct {
	QuestionSource  *QuestionSource `json:"questionSource,omitempty"`
	Task            string          `json:"task"`
	Question        string          `json:"question,omitempty"`
	Reason          string          `json:"reason"`
	HistoryRevision int             `json:"historyRevision"`
	Meta            ModelMeta       `json:"meta"`
	CreatedAt       time.Time       `json:"createdAt"`
}

type ProviderChange struct {
	Previous ProviderRef `json:"previous"`
	Current  ProviderRef `json:"current"`
	At       time.Time   `json:"at"`
}

type IntakeRevision struct {
	Age              int  `json:"age"`
	PregnancyRelated bool `json:"pregnancyRelated"`
	HistoryRevision  int  `json:"historyRevision"`
}

type Encounter struct {
	GuidedHistory         bool                   `json:"guidedHistory,omitempty"`
	ReportConsents        []ReportConsent        `json:"reportConsents,omitempty"`
	ReportIdentityChanges []ReportIdentityChange `json:"reportIdentityChanges,omitempty"`
	EpisodeID             string                 `json:"episodeId,omitempty"`
	EpisodeTitle          string                 `json:"episodeTitle,omitempty"`
	ReportFirst           bool                   `json:"reportFirst,omitempty"`
	Documents             []SourceDocument       `json:"documents,omitempty"`
	ReportReviews         []ReportReview         `json:"reportReviews,omitempty"`
	Outcomes              []VisitOutcome         `json:"outcomes,omitempty"`
	QuestionSource        *QuestionSource        `json:"questionSource,omitempty"`
	QuestionReviewIDs     []string               `json:"questionReviewIds,omitempty"`
	OutstandingReviewIDs  []string               `json:"outstandingReviewIds,omitempty"`
	Pathways              []Pathway              `json:"pathways,omitempty"`
	IntakeHistory         []IntakeRevision       `json:"intakeHistory"`
	ReviewExtraction      bool                   `json:"reviewExtraction,omitempty"`
	ExtractedRevision     int                    `json:"extractedRevision,omitempty"`
	ID                    string                 `json:"id"`
	Concern               string                 `json:"concern"`
	Age                   int                    `json:"age"`
	Background            string                 `json:"background"`
	PregnancyRelated      bool                   `json:"pregnancyRelated"`
	Revision              int                    `json:"revision"`
	HistoryRevision       int                    `json:"historyRevision"`
	ConfirmedRevision     int                    `json:"confirmedRevision"`
	State                 string                 `json:"state"`
	Provider              ProviderRef            `json:"provider"`
	Demo                  bool                   `json:"demo"`
	CreatedAt             time.Time              `json:"createdAt"`
	UpdatedAt             time.Time              `json:"updatedAt"`
	NoticeVersion         string                 `json:"noticeVersion"`
	Notice                string                 `json:"notice"`
	ReviewStatus          string                 `json:"reviewStatus"`
	Answers               []Answer               `json:"answers"`
	Facts                 []Fact                 `json:"facts"`
	Warnings              []Warning              `json:"warnings"`
	Contradictions        []Contradiction        `json:"contradictions"`
	Outstanding           []string               `json:"outstanding"`
	Question              string                 `json:"question"`
	Choices               []string               `json:"choices"`
	Assessments           []Assessment           `json:"assessments"`
	Failure               string                 `json:"failure"`
	ScopeReason           string                 `json:"scopeReason"`
	Summaries             []Summary              `json:"summaries"`
	Turns                 []Turn                 `json:"turns"`
	ProviderChanges       []ProviderChange       `json:"providerChanges"`
	Pending               string                 `json:"pending"`
	FailedTask            string                 `json:"failedTask,omitempty"`
}

type Report struct {
	OutdatedAssessmentVersions []int `json:"outdatedAssessmentVersions"`
	CurrentAssessmentVersion   int   `json:"currentAssessmentVersion,omitempty"`
	Encounter
	ModelGuidance string    `json:"modelGuidance"`
	ExportVersion string    `json:"exportVersion"`
	ExportedAt    time.Time `json:"exportedAt"`
}

func (e *Encounter) Report() Report {
	r := Report{Encounter: *e, ModelGuidance: ModelGuidance(), ExportVersion: "clinical-history/1", ExportedAt: time.Now().UTC(), OutdatedAssessmentVersions: []int{}}
	for _, a := range e.Assessments {
		if a.HistoryRevision != e.HistoryRevision {
			r.OutdatedAssessmentVersions = append(r.OutdatedAssessmentVersions, a.Version)
		} else {
			r.CurrentAssessmentVersion = a.Version
		}
	}
	return r
}
