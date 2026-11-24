package clinical

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// PolicyRecord states whether a safety policy version has had clinical review.
// A version can be called reviewed only with a named reviewer, a date and a
// scope. Until then every export says the rules are development content.
type PolicyRecord struct {
	Version  string `json:"version"`
	Status   string `json:"status"`
	Reviewer string `json:"reviewer,omitempty"`
	Date     string `json:"date,omitempty"`
	Scope    string `json:"scope,omitempty"`
	Note     string `json:"note"`
}

const notReviewed = "Development content. These rules have not been clinically reviewed."

var policyRecords = map[string]PolicyRecord{
	"development-1": {Version: "development-1", Status: "development", Note: notReviewed},
	"development-2": {Version: "development-2", Status: "development", Note: notReviewed},
	"development-3": {Version: "development-3", Status: "development", Note: notReviewed},
	"development-4": {Version: "development-4", Status: "development", Note: notReviewed},
}

// PolicyReview returns the review record for a policy version.
func PolicyReview(version string) PolicyRecord {
	if r, ok := policyRecords[version]; ok {
		return r
	}
	return PolicyRecord{Version: version, Status: "unknown", Note: "No review record exists for this policy version."}
}

// InputHash identifies the exact model input, so a past result can be matched
// to the evidence it was validated against.
func InputHash(in Input) string {
	b, _ := json.Marshal(in)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:12])
}
