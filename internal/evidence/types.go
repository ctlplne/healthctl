// Package evidence provides local retrieval of versioned, attributable educational passages.
package evidence

type Feature struct {
	Quote    string   `json:"quote"`
	Concepts []string `json:"concepts"`
}
type ClaimLink struct {
	PassageID string   `json:"passageId"`
	Quote     string   `json:"quote"`
	FactIDs   []string `json:"factIds"`
}
type Applicability struct {
	Scope  string `json:"scope"`
	MinAge int    `json:"minAge"`
	MaxAge int    `json:"maxAge"`
}
type Passage struct {
	Applicability *Applicability `json:"applicability,omitempty"`
	Conditions    []string       `json:"conditions,omitempty"`
	Features      []Feature      `json:"features,omitempty"`
	ID            string         `json:"id"`
	Collection    string         `json:"collection,omitempty"`
	Version       string         `json:"version,omitempty"`
	Title         string         `json:"title"`
	Section       string         `json:"section"`
	Text          string         `json:"text"`
	Publisher     string         `json:"publisher"`
	URL           string         `json:"url"`
	Published     string         `json:"published"`
	Verified      string         `json:"verified"`
	Population    string         `json:"population"`
	Limitations   string         `json:"limitations"`
	Rights        string         `json:"rights"`
	RightsURL     string         `json:"rightsUrl"`
	ReviewStatus  string         `json:"reviewStatus"`
	ReviewRecord  string         `json:"reviewRecord,omitempty"`
	Status        string         `json:"status,omitempty"`
	ConflictsWith []string       `json:"conflictsWith"`
	Topics        []string       `json:"topics"`
}
type Pack struct {
	ID       string    `json:"id"`
	Version  string    `json:"version"`
	Title    string    `json:"title"`
	Passages []Passage `json:"passages"`
}
type Collection struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Title       string `json:"title"`
	Origin      string `json:"origin"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
	InstalledAt string `json:"installedAt"`
	Passages    int    `json:"passages"`
}
type Result struct {
	CorpusVersion string    `json:"corpusVersion"`
	Outcome       string    `json:"outcome"`
	Reason        string    `json:"reason"`
	Passages      []Passage `json:"passages"`
	Conflicts     []string  `json:"conflicts"`
}
type Support struct {
	Relation   string      `json:"relation"`
	ClaimLinks []ClaimLink `json:"claimLinks"`
	ItemIndex  int         `json:"itemIndex"`
	Result
}
