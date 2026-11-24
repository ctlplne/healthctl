package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/ctlplne/healthctl/internal/clinical"
)

// Answerer replies to interview questions for one synthetic case. The
// interviewer never sees it; the app's own checks score the journey.
type Answerer interface {
	Answer(ctx context.Context, question string) (string, error)
}

// scripted answers with the case's fixed replies. The scorecard uses it.
type scripted struct{ p *patient }

func (s scripted) Answer(_ context.Context, q string) (string, error) { return s.p.answer(q), nil }

// Natural rephrases the case's disclosures and non-answers the way people
// type: fillers, contractions, hedges and lower-case starts. The case's
// alert phrase is never changed, so the expected warning stays defined.
type Natural struct {
	p     *patient
	r     *rand.Rand
	alert string
	// Changed counts answers that differ from the scripted wording.
	Changed int
}

func NewNatural(c Case, seed int64) *Natural {
	return &Natural{p: &patient{replies: c.Replies, sent: map[int]bool{}}, r: rand.New(rand.NewSource(seed)), alert: c.AlertWhen}
}

var nonAnswers = []string{"hmm, not really sure", "I don't know", "not sure tbh", "can't really remember", "no idea"}
var fillers = []string{"", "um, ", "well, ", "so, ", "honestly, "}
var tails = []string{"", " I think", "...", " if that helps"}
var contractions = strings.NewReplacer("I am ", "I'm ", "do not", "don't", "It is ", "It's ", "cannot", "can't", "I have been", "I've been")

func (n *Natural) Answer(_ context.Context, q string) (string, error) {
	base := n.p.answer(q)
	out := base
	switch {
	case base == "I am not sure.":
		out = nonAnswers[n.r.Intn(len(nonAnswers))]
	case n.alert != "" && strings.Contains(base, n.alert):
		out = fillers[n.r.Intn(len(fillers))] + base
	default:
		out = contractions.Replace(strings.TrimSuffix(base, "."))
		if n.r.Intn(2) == 0 && len(out) > 1 && !strings.HasPrefix(out, "I ") && !strings.HasPrefix(out, "I'") {
			out = strings.ToLower(out[:1]) + out[1:]
		}
		out = fillers[n.r.Intn(len(fillers))] + out + tails[n.r.Intn(len(tails))]
	}
	if out != base {
		n.Changed++
	}
	return out, nil
}

// ModelSimulator asks a separate model to answer as the synthetic person,
// using only the case's disclosures. It needs an OpenAI-compatible endpoint
// and key, so it runs only when the maintainer asks for a live run.
type ModelSimulator struct {
	Endpoint, Model, Key string
	Case                 Case
	Client               *http.Client
}

func (m ModelSimulator) Answer(ctx context.Context, q string) (string, error) {
	if !strings.HasPrefix(m.Endpoint, "https://") || m.Model == "" || m.Key == "" {
		return "", errors.New("the simulator needs an HTTPS endpoint, a model and a key")
	}
	var facts []string
	for _, r := range m.Case.Replies {
		facts = append(facts, r.Answer)
	}
	system := "You are role-playing a fictional adult in a synthetic test of a history-taking app. Answer the interviewer's question in one or two casual sentences, in your own words, using only these facts: " + strings.Join(facts, " | ") + " If the facts don't answer the question, say you're not sure. Never add symptoms."
	body, _ := json.Marshal(map[string]any{"model": m.Model, "messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": q}}, "max_tokens": 120})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(m.Endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+m.Key)
	req.Header.Set("Content-Type", "application/json")
	client := m.Client
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&out) != nil || len(out.Choices) == 0 {
		return "", fmt.Errorf("the simulator model returned status %d", resp.StatusCode)
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// RunSimulated runs an adaptive journey with a simulated user and scores it
// with the same invariants as the scorecard.
func RunSimulated(ctx context.Context, c Case, model Model, sim Answerer, maxTurns int) (r Result) {
	r = Result{Case: c.ID, Arm: "simulated", Model: model.Reference().Model, CheckpointAlerts: map[string]bool{}, Errors: []string{}, MissedTargets: []string{}, Unadjudicated: []string{}, FabricatedFindings: []string{}, StaleAssessments: []string{}, LostWarnings: []string{}, AlertErrors: []string{}, ContradictionErrors: []string{}}
	e, err := clinical.New("synthetic-encounter", clinical.Intake{GuidedHistory: true, Age: c.Age, Concern: c.Opening, UrgentNow: c.UrgentScreen, PregnancyRelated: c.PregnancyRelated, NoticeVersion: clinical.NoticeVersion}, model.Reference(), "")
	if err != nil {
		r.Errors = append(r.Errors, err.Error())
		return r
	}
	r.Pathways = e.Pathways
	if e.ScopeReason != "" {
		r.AssessmentOutcome = "history_only"
		return r
	}
	if err := runJourney(ctx, &r, c, e, model, maxTurns, true, sim); err != nil {
		r.Errors = append(r.Errors, err.Error())
	}
	return r
}
