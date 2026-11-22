package evaluation

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/evidence"
	"math"
	"slices"
)

//go:embed release-cases.json
var releaseCases []byte

//go:embed fixture-results.json
var originalBaseline []byte

func ReleaseCases() ([]Case, error) {
	all, err := Cases()
	if err != nil {
		return nil, err
	}
	var extra []Case
	if err := json.Unmarshal(releaseCases, &extra); err != nil {
		return nil, err
	}
	all = append(all, extra...)
	seen := map[string]bool{}
	for _, c := range all {
		if !evidence.ValidID(c.ID) || seen[c.ID] {
			return nil, fmt.Errorf("missing or duplicate release case identifier %q", c.ID)
		}
		seen[c.ID] = true
	}
	return all, nil
}

// DatasetFingerprint covers the authored cases, fixture scan and the comparison implementation.
func DatasetFingerprint() string {
	return evidence.Hash([]byte(evidence.Hash(caseBytes) + evidence.Hash(releaseCases) + evidence.Hash(blurredPanel) + evidence.Hash(reportProtocol)))
}

//go:embed report_comparison.go
var reportProtocol []byte

func starterIndex() evidence.Catalog {
	pack := evidence.Starter()
	raw, _ := json.Marshal(pack)
	return evidence.Build([]evidence.ActivePack{{Pack: pack, Hash: evidence.Hash(raw)}})
}

var bundledIndex = starterIndex()

func StarterCorpus() string { return bundledIndex.Version }

type ArmScore struct {
	Runs                  int      `json:"runs"`
	Completed             int      `json:"completed"`
	Failed                int      `json:"failed"`
	Abstained             int      `json:"abstained"`
	MissedUrgency         int      `json:"missedUrgencyCheckpoints"`
	ExcessiveUrgency      int      `json:"excessiveUrgencyCheckpoints"`
	KeyHistory            Metric   `json:"keyHistory"`
	Questions             int      `json:"questions"`
	RepeatQuestions       int      `json:"repeatQuestions"`
	P50RunUS              *int64   `json:"p50AutomatedRunMicroseconds"`
	P95RunUS              *int64   `json:"p95AutomatedRunMicroseconds"`
	TimedRuns             int      `json:"timedRuns"`
	ModelCalls            int      `json:"modelOperations"`
	Unmetered             int      `json:"operationsWithoutUsage"`
	CostPerCompleted      *float64 `json:"modelCostPerCompletedEncounterUSD"`
	CostPerAcceptable     *float64 `json:"costPerClinicianAcceptableBriefUSD"`
	Requests              int      `json:"httpRequests"`
	Retries               int      `json:"retries"`
	InputTokens           int64    `json:"inputTokens"`
	OutputTokens          int64    `json:"outputTokens"`
	UnknownCostOperations int      `json:"unknownCostOperations"`
	PricedSubtotal        *float64 `json:"pricedSubtotalUSD"`
}
type Scores struct {
	Arms         map[string]ArmScore `json:"arms"`
	Previous     map[string]ArmScore `json:"original19CaseFixtureBaseline"`
	Policy       string              `json:"policyVersion"`
	HumanStudies string              `json:"humanStudies"`
	CostLimit    string              `json:"costScope"`
}

func summarize(results []Result) map[string]ArmScore {
	out := map[string]ArmScore{}
	times := map[string][]int64{}
	for _, r := range results {
		a := out[r.Arm]
		a.Runs++
		a.ModelCalls += r.ModelCalls
		a.Unmetered += r.ModelCalls - len(r.ModelUsage)
		a.MissedUrgency += r.MissedAlerts
		a.ExcessiveUrgency += r.ExcessiveAlerts
		a.KeyHistory.Total += r.KeyHistory.Total
		a.KeyHistory.Matched += r.KeyHistory.Matched
		a.Questions += r.InterviewQuestions
		a.RepeatQuestions += r.RepeatedQuestions
		if r.Failed() {
			a.Failed++
		} else {
			a.Completed++
		}
		if r.AssessmentOutcome == "insufficient" {
			a.Abstained++
		}
		for _, u := range r.ModelUsage {
			a.Requests += u.Requests
			a.Retries += u.Retries
			a.InputTokens += u.InputTokens
			a.OutputTokens += u.OutputTokens
			if u.EstimatedUSD == nil {
				a.UnknownCostOperations++
			} else {
				if a.PricedSubtotal == nil {
					a.PricedSubtotal = new(float64)
				}
				*a.PricedSubtotal += *u.EstimatedUSD
			}
		}
		if r.ElapsedUS != nil {
			a.TimedRuns++
			times[r.Arm] = append(times[r.Arm], *r.ElapsedUS)
		}
		out[r.Arm] = a
	}
	for k, vals := range times {
		slices.Sort(vals)
		percentile := func(p float64) int64 { return vals[int(math.Ceil(float64(len(vals))*p))-1] }
		a := out[k]
		p50, p95 := percentile(.5), percentile(.95)
		a.P50RunUS, a.P95RunUS = &p50, &p95
		if a.Completed > 0 && a.Unmetered == 0 && a.UnknownCostOperations == 0 && a.PricedSubtotal != nil {
			unit := *a.PricedSubtotal / float64(a.Completed)
			a.CostPerCompleted = &unit
		}
		out[k] = a
	}
	return out
}
func Score(results []Result) Scores {
	var baseline struct {
		Results []Result `json:"results"`
	}
	json.Unmarshal(originalBaseline, &baseline)
	return Scores{Arms: summarize(results), Previous: summarize(baseline.Results), Policy: clinical.PolicyVersion, HumanStudies: "No user abandonment, independent clinician adjudication, comprehension or timed handoff study has been performed. Automated run latency measures software only.", CostLimit: "Operator-supplied flat token rates; local infrastructure, cached-token discounts, content maintenance, and cost per clinician-acceptable brief are unmeasured."}
}
