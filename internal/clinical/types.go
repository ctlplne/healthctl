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

// ModelGuidance is current operator advice. Historical user consent is versioned separately.
func ModelGuidance() string { return strings.TrimSpace(modelGuidanceText) }

//go:embed preview.txt
var previewText string

func PreviewNotice() string { return strings.TrimSpace(previewText) }

const (
	NoticeVersion = "2026-11-v2"
	Notice        = "Free public preview (beta). Use healthctl to organize your history and questions for a visit. AI suggestions are experimental and not reviewed by a healthcare professional. They can be wrong or miss serious illness. Ask a qualified healthcare professional to verify the findings before making care decisions. Do not use healthctl to diagnose yourself, change treatment, or decide it is safe to wait. For a possible emergency, seek local emergency care immediately."
	ReviewStatus  = "not_clinician_reviewed"
	PolicyVersion = "development-5"
	DemoConcern   = "My nose is runny and I keep sneezing."
)

type ProviderRef struct {
	Kind       string `json:"kind"`
	Model      string `json:"model"`
	Endpoint   string `json:"endpoint"`
	Structured string `json:"structured"`
	// Reasoning is "effort" when the app should send a reasoning effort by
	// task, stream the call and allow a longer time limit.
	Reasoning string `json:"reasoning,omitempty"`
}

type Intake struct {
	DemoPractice     bool   `json:"demoPractice,omitempty"`
	NarrativeFirst   bool   `json:"narrativeFirst,omitempty"`
	Focused          bool   `json:"focused,omitempty"`
	Agenda           string `json:"agenda,omitempty"`
	GuidedHistory    bool   `json:"guidedHistory,omitempty"`
	ReportFirst      bool   `json:"reportFirst,omitempty"`
	Concern          string `json:"concern"`
	Age              int    `json:"age"`
	Background       string `json:"background"`
	PregnancyRelated bool   `json:"pregnancyRelated"`
	UrgentNow        bool   `json:"urgentNow"`
	NoticeVersion    string `json:"noticeVersion"`
}

// AssessmentNotice is separate from the notice accepted at intake. Change its
// version whenever the disclosure or acknowledgment changes.
var AssessmentNotice = struct {
	Version   string `json:"version"`
	Risks     string `json:"risks"`
	Liability string `json:"liability"`
	Choice    string `json:"choice"`
}{
	Version:   "assessment-2026-11-22-v1",
	Risks:     "These experimental AI suggestions can be wrong or miss an emergency. No healthcare professional has reviewed them. A missing warning does not mean it is safe to wait. Ask a qualified healthcare professional to verify the findings before making care decisions.",
	Liability: "The publisher and maintainers provide this beta as is and limit their liability as described in the beta use notice, to the extent the law allows. You keep any rights the law protects.",
	Choice:    "I understand these limits and choose to request this experimental assessment at my own discretion.",
}

type AssessmentAcknowledgment struct {
	NoticeVersion   string      `json:"noticeVersion"`
	At              time.Time   `json:"at"`
	HistoryRevision int         `json:"historyRevision"`
	Provider        ProviderRef `json:"provider"`
}

type Command struct {
	Onset                string      `json:"onset,omitempty"`
	End                  string      `json:"end,omitempty"`
	Approximate          bool        `json:"approximate,omitempty"`
	Speaker              string      `json:"speaker,omitempty"`
	Prescribed           string      `json:"prescribed,omitempty"`
	ReviewIDs            []string    `json:"reviewIds,omitempty"`
	ForRevision          int         `json:"forRevision,omitempty"`
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
	StabilityCheck       bool        `json:"stabilityCheck,omitempty"`
}

type Answer struct {
	Detail         *VisitDetail    `json:"detail,omitempty"`
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

// FindingProposal is a normalized claim about an exact substring of a user answer.
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
	// Term and Code map the concept to the app vocabulary when it names one.
	Term string `json:"term,omitempty"`
	Code string `json:"code,omitempty"`
}

type Warning struct {
	DocumentID string `json:"documentId,omitempty"`
	RowID      string `json:"rowId,omitempty"`
	FactID     string `json:"factId,omitempty"`
	Evidence
	ID               string    `json:"id"`
	Rule             string    `json:"rule"`
	Level            string    `json:"level,omitempty"`
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
	// Set by the app: whether the name is on the app's condition list, the
	// app's description, and whether the item changed between two runs.
	Vetted            bool   `json:"vetted,omitempty"`
	Description       string `json:"description,omitempty"`
	DescriptionStatus string `json:"descriptionStatus,omitempty"`
	Unstable          bool   `json:"unstable,omitempty"`
}

type ModelMeta struct {
	ValidatorVersion string         `json:"validatorVersion,omitempty"`
	InputHash        string         `json:"inputHash,omitempty"`
	Endpoint         string         `json:"endpoint,omitempty"`
	Provider         string         `json:"provider"`
	RequestedModel   string         `json:"requestedModel"`
	ReturnedModel    string         `json:"returnedModel"`
	PromptVersion    string         `json:"promptVersion"`
	SchemaVersion    string         `json:"schemaVersion"`
	AppVersion       string         `json:"appVersion"`
	Settings         map[string]any `json:"settings"`
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
	// DontMiss holds the app's fixed lines for dangerous causes the assessment
	// doesn't include. StabilityChecked records a second run for comparison.
	DontMiss         []string `json:"dontMiss,omitempty"`
	StabilityChecked bool     `json:"stabilityChecked,omitempty"`
}

type Input struct {
	// OmittedAnswerIDs lists older answers the app left out to fit the input budget.
	OmittedAnswerIDs    []string        `json:"omittedAnswerIds,omitempty"`
	ReturnVisit         bool            `json:"returnVisit,omitempty"`
	Focused             bool            `json:"focused,omitempty"`
	Agenda              string          `json:"agenda,omitempty"`
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
	// Gaps and UnansweredRedFlags record what the app still expected when the
	// history was confirmed.
	Gaps               []Gap             `json:"gaps,omitempty"`
	UnansweredRedFlags []RedFlagQuestion `json:"unansweredRedFlags,omitempty"`
}

type Turn struct {
	QuestionSource *QuestionSource `json:"questionSource,omitempty"`
	Task           string          `json:"task"`
	Question       string          `json:"question,omitempty"`
	Asked          bool            `json:"asked,omitempty"`
	// OmittedAnswerIDs lists older answers left out to fit the input budget.
	OmittedAnswerIDs []string  `json:"omittedAnswerIds,omitempty"`
	Reason           string    `json:"reason"`
	HistoryRevision  int       `json:"historyRevision"`
	Meta             ModelMeta `json:"meta"`
	CreatedAt        time.Time `json:"createdAt"`
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

// Encounter is stored as one encrypted JSON payload. It is split into
// sub-records by concern; their fields are promoted, so the JSON stays flat.
// PayloadVersion lets DecodeEncounter migrate older payloads once.
type Encounter struct {
	PayloadVersion int `json:"payloadVersion,omitempty"`
	EncounterIdentity
	InterviewState
	HistoryRecord
	SafetyRecord
	SourceRecords
	AssessmentRecord
}

// EncounterIdentity is who the encounter is for and how it was set up.
type EncounterIdentity struct {
	ID               string           `json:"id"`
	Concern          string           `json:"concern"`
	Age              int              `json:"age"`
	Background       string           `json:"background"`
	PregnancyRelated bool             `json:"pregnancyRelated"`
	Agenda           string           `json:"agenda,omitempty"`
	IntakeHistory    []IntakeRevision `json:"intakeHistory"`
	Provider         ProviderRef      `json:"provider"`
	ProviderChanges  []ProviderChange `json:"providerChanges"`
	Demo             bool             `json:"demo"`
	DemoPractice     bool             `json:"demoPractice,omitempty"`
	EpisodeID        string           `json:"episodeId,omitempty"`
	EpisodeTitle     string           `json:"episodeTitle,omitempty"`
	Revision         int              `json:"revision"`
	CreatedAt        time.Time        `json:"createdAt"`
	UpdatedAt        time.Time        `json:"updatedAt"`
	NoticeVersion    string           `json:"noticeVersion"`
	Notice           string           `json:"notice"`
	ReviewStatus     string           `json:"reviewStatus"`
}

// InterviewState is where the interview is and what it asks next.
type InterviewState struct {
	State                string          `json:"state"`
	Pending              string          `json:"pending"`
	Failure              string          `json:"failure"`
	FailedTask           string          `json:"failedTask,omitempty"`
	ScopeReason          string          `json:"scopeReason"`
	Question             string          `json:"question"`
	Choices              []string        `json:"choices"`
	QuestionSource       *QuestionSource `json:"questionSource,omitempty"`
	QuestionReviewIDs    []string        `json:"questionReviewIds,omitempty"`
	Outstanding          []string        `json:"outstanding"`
	OutstandingReviewIDs []string        `json:"outstandingReviewIds,omitempty"`
	Pathways             []Pathway       `json:"pathways,omitempty"`
	Turns                []Turn          `json:"turns"`
	GuidedHistory        bool            `json:"guidedHistory,omitempty"`
	NarrativeFirst       bool            `json:"narrativeFirst,omitempty"`
	Focused              bool            `json:"focused,omitempty"`
	FocusedFrom          int             `json:"focusedFrom,omitempty"`
	ReportFirst          bool            `json:"reportFirst,omitempty"`
	ReturnSource         string          `json:"returnSource,omitempty"`
	ReviewExtraction     bool            `json:"reviewExtraction,omitempty"`
	ExtractedRevision    int             `json:"extractedRevision,omitempty"`
}

// HistoryRecord is what the user reported and confirmed.
type HistoryRecord struct {
	Answers           []Answer        `json:"answers"`
	Facts             []Fact          `json:"facts"`
	Contradictions    []Contradiction `json:"contradictions"`
	HistoryRevision   int             `json:"historyRevision"`
	ConfirmedRevision int             `json:"confirmedRevision"`
	Summaries         []Summary       `json:"summaries"`
}

// SafetyRecord holds warnings and the app's own safety questions.
type SafetyRecord struct {
	Warnings     []Warning         `json:"warnings"`
	SafetyChecks []SafetyCheck     `json:"safetyChecks,omitempty"`
	RedFlagsDue  []RedFlagQuestion `json:"redFlagsDue,omitempty"`
	Gaps         []Gap             `json:"gaps,omitempty"`
}

// SourceRecords holds documents, photos and visit entries from outside the interview.
type SourceRecords struct {
	Documents             []SourceDocument       `json:"documents,omitempty"`
	ReportReviews         []ReportReview         `json:"reportReviews,omitempty"`
	ReportConsents        []ReportConsent        `json:"reportConsents,omitempty"`
	ReportIdentityChanges []ReportIdentityChange `json:"reportIdentityChanges,omitempty"`
	Photos                []ObservationPhoto     `json:"photos,omitempty"`
	Feedback              []ClinicianFeedback    `json:"feedback,omitempty"`
	Outcomes              []VisitOutcome         `json:"outcomes,omitempty"`
}

// AssessmentRecord holds optional assessments and their acknowledgments.
type AssessmentRecord struct {
	Assessments               []Assessment               `json:"assessments"`
	AssessmentAcknowledgments []AssessmentAcknowledgment `json:"assessmentAcknowledgments,omitempty"`
	// StabilityRequested asks for the assessment to be run twice and compared.
	StabilityRequested bool `json:"stabilityRequested,omitempty"`
}

type Report struct {
	PolicyReview               PolicyRecord `json:"policyReview"`
	OutdatedAssessmentVersions []int        `json:"outdatedAssessmentVersions"`
	CurrentAssessmentVersion   int          `json:"currentAssessmentVersion,omitempty"`
	Encounter
	PreviewNotice string    `json:"previewNotice"`
	ModelGuidance string    `json:"modelGuidance"`
	ExportVersion string    `json:"exportVersion"`
	ExportedAt    time.Time `json:"exportedAt"`
}

func (e *Encounter) Report() Report {
	r := Report{Encounter: e.ForDisplay(), PolicyReview: PolicyReview(PolicyVersion), PreviewNotice: PreviewNotice(), ModelGuidance: ModelGuidance(), ExportVersion: "clinical-history/1", ExportedAt: time.Now().UTC(), OutdatedAssessmentVersions: []int{}}
	for _, a := range e.Assessments {
		if a.HistoryRevision != e.HistoryRevision {
			r.OutdatedAssessmentVersions = append(r.OutdatedAssessmentVersions, a.Version)
		} else {
			r.CurrentAssessmentVersion = a.Version
		}
	}
	return r
}
