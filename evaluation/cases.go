// Package evaluation compares blinded, synthetic interviews against authored expectations.
package evaluation

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type ExpectedFact struct {
	Label   string `json:"label"`
	Concept string `json:"concept"`
	Source  string `json:"source"`
	Status  string `json:"status"`
	Subject string `json:"subject"`
	Time    string `json:"time"`
}

type Reply struct {
	WhenAsked string `json:"whenAsked,omitempty"`
	Topic     string `json:"topic"`
	Answer    string `json:"answer"`
}
type Forbidden struct {
	Concept string `json:"concept"`
	Status  string `json:"status"`
	Subject string `json:"subject"`
}
type Case struct {
	PregnancyRelated     bool           `json:"pregnancyRelated,omitempty"`
	ID                   string         `json:"id"`
	Age                  int            `json:"age"`
	Opening              string         `json:"opening"`
	UrgentScreen         bool           `json:"urgentScreen"`
	Replies              []Reply        `json:"replies"`
	Facts                []ExpectedFact `json:"facts"`
	Forbidden            []Forbidden    `json:"forbidden"`
	Alert                bool           `json:"alert"`
	AlertWhen            string         `json:"alertWhen"`
	Correction           string         `json:"correction"`
	AfterCorrectionAlert bool           `json:"afterCorrectionAlert"`
	ConflictWhen         string         `json:"conflictWhen"`
	Reference            string         `json:"reference"`
	Insufficient         bool           `json:"insufficient"`
}

//go:embed cases.json
var caseBytes []byte

func Cases() ([]Case, error) {
	var cases []Case
	if err := json.Unmarshal(caseBytes, &cases); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if seen[c.ID] || c.ID == "" || c.Opening == "" {
			return nil, fmt.Errorf("invalid case %q", c.ID)
		}
		seen[c.ID] = true
		var patterns []string
		for _, f := range c.Facts {
			patterns = append(patterns, f.Concept)
		}
		for _, f := range c.Forbidden {
			patterns = append(patterns, f.Concept)
		}
		patterns = append(patterns, c.Reference)
		for _, p := range patterns {
			if _, err := regexp.Compile("(?i)" + p); err != nil {
				return nil, fmt.Errorf("case %s: %w", c.ID, err)
			}
		}
		for _, r := range c.Replies {
			if _, err := regexp.Compile("(?i)" + r.WhenAsked); err != nil {
				return nil, fmt.Errorf("invalid question pattern for case %s", c.ID)
			}
			if topics[r.Topic] == nil || strings.TrimSpace(r.Answer) == "" {
				return nil, fmt.Errorf("invalid reply topic in %s", c.ID)
			}
		}
	}
	return cases, nil
}

var topics = map[string]*regexp.Regexp{
	"timing":     regexp.MustCompile(`(?i)\b(when|long|onset|start|started|begin|began|duration)\b`),
	"trigger":    regexp.MustCompile(`(?i)\b(trigger|triggers|outdoors|environment|exposure|notice the symptoms most|worse around|bring it on)\b`),
	"associated": regexp.MustCompile(`(?i)\b(anything else|what else|other symptoms?|other changes|associated symptoms|along with|additional symptoms)\b`),
	"medication": regexp.MustCompile(`(?i)\b(medicine|medicines|medication|medications|prescriptions?|pills?|drugs?)\b`),
}

// The simulator keeps only unrevealed disclosures. The interviewer never receives
// this object, case IDs, expected findings, alerts or reference diagnoses.
type patient struct {
	replies []Reply
	sent    map[int]bool
}

func (p *patient) answer(question string) string {
	// Specific subject phrases outrank the generic word "when" in exposure questions.
	for i, r := range p.replies {
		if !p.sent[i] && r.WhenAsked != "" && regexp.MustCompile("(?i)"+r.WhenAsked).MatchString(question) {
			p.sent[i] = true
			return r.Answer
		}
	}
	for _, topic := range []string{"trigger", "medication", "associated", "timing"} {
		if !topics[topic].MatchString(question) {
			continue
		}
		for i, r := range p.replies {
			if !p.sent[i] && r.Topic == topic {
				p.sent[i] = true
				return r.Answer
			}
		}
		return "I am not sure."
	}
	return "I am not sure."
}
