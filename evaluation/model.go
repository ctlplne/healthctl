package evaluation

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/provider"
)

// Both arms use this interface for extraction and assessment. Its only input
// contains currently disclosed encounter state.
type Model interface {
	Reference() clinical.ProviderRef
	Propose(context.Context, clinical.Input) (clinical.Proposal, clinical.ModelMeta, error)
}

type Remote struct {
	Client *provider.Client
	Config provider.Config
}

func (m Remote) Reference() clinical.ProviderRef {
	return clinical.ProviderRef{Kind: m.Config.Kind, Model: m.Config.Model, Endpoint: m.Config.Endpoint, Structured: m.Config.Structured}
}
func (m Remote) Propose(ctx context.Context, in clinical.Input) (clinical.Proposal, clinical.ModelMeta, error) {
	var p clinical.Proposal
	_, meta, err := m.Client.Generate(ctx, m.Config, in.Task, in, func(raw []byte) error {
		p = clinical.Proposal{}
		if err := json.Unmarshal(raw, &p); err != nil {
			return err
		}
		return clinical.Validate(in, p)
	})
	return p, clinical.ModelMeta{Provider: meta.Provider, Endpoint: meta.Endpoint, RequestedModel: meta.RequestedModel, ReturnedModel: meta.ReturnedModel, PromptVersion: meta.PromptVersion, SchemaVersion: meta.SchemaVersion, AppVersion: meta.AppVersion, Settings: meta.Settings}, err
}

// Fixture is an intentionally limited regression double used only by the evaluation
// executable. Its fixed vocabulary and questions have no access to case labels.
type Fixture struct{}

func (Fixture) Reference() clinical.ProviderRef {
	return clinical.ProviderRef{Kind: "evaluation-fixture", Model: "scripted-v1"}
}

type phrase struct {
	concept string
	pattern *regexp.Regexp
}

var vocabulary = []phrase{
	{"Sore throat", regexp.MustCompile(`(?i)throat feels sore`)}, {"Cough", regexp.MustCompile(`(?i)\bcough\b`)},
	{"Ankle injury", regexp.MustCompile(`(?i)twisted my ankle`)}, {"Ankle swelling", regexp.MustCompile(`(?i)ankle is swollen`)},
	{"Headache", regexp.MustCompile(`(?i)\bheadache\b`)}, {"Nausea", regexp.MustCompile(`(?i)\bnausea\b`)},
	{"Tiredness", regexp.MustCompile(`(?i)feel tired`)}, {"Medication history", regexp.MustCompile(`(?i)\b(medicines|pill)\b`)},
	{"Penicillin allergy", regexp.MustCompile(`(?i)penicillin allergy`)}, {"Chest pain", regexp.MustCompile(`(?i)chest pain`)},
	{"Breathing difficulty", regexp.MustCompile(`(?i)cannot breathe`)},
}

func (Fixture) Propose(_ context.Context, in clinical.Input) (clinical.Proposal, clinical.ModelMeta, error) {
	meta := clinical.ModelMeta{Provider: "evaluation-fixture", RequestedModel: "scripted-v1", ReturnedModel: "scripted-v1", PromptVersion: "evaluation-fixture-v1", SchemaVersion: "proposal-v1", AppVersion: "0.2.0", Settings: map[string]any{"network": false}}
	if in.Concern == clinical.DemoConcern || in.Concern == clinical.DemoReportConcern {
		p := clinical.Demo(in)
		if in.Task == "interview" && in.ReviewOnly {
			p.Action = "review"
			p.Question = ""
			p.Choices = nil
		}
		return p, meta, clinical.Validate(in, p)
	}
	p := clinical.Proposal{Action: "review"}
	if in.Task == "assessment" {
		// This double has only the application's authored nasal differential.
		p.Action = "insufficient"
		p.Outstanding = []string{"This regression fixture has no authored differential for this history."}
		return p, meta, clinical.Validate(in, p)
	}
	for _, a := range in.Answers {
		if a.Kind == "screen" || !a.Active {
			continue
		}
		for _, phrase := range strings.FieldsFunc(a.Text, func(r rune) bool { return r == '.' || r == '!' || r == '?' }) {
			phrase = strings.TrimSpace(phrase)
			text := strings.ToLower(phrase)
			for _, v := range vocabulary {
				if !v.pattern.MatchString(text) {
					continue
				}
				f := clinical.FindingProposal{Evidence: clinical.Evidence{AnswerID: a.ID, Quote: phrase}, Concept: v.concept, Subject: "self", Status: "reported_present"}
				switch {
				case strings.Contains(text, "rather not"):
					f.Status = "declined"
				case strings.Contains(text, "unsure") || strings.Contains(text, "might"):
					f.Status = "uncertain"
				case strings.Contains(" "+text, " no "):
					f.Status = "explicitly_denied"
				}
				switch {
				case strings.Contains(text, "my father"):
					f.Subject = "family"
				case strings.Contains(text, "my friend"):
					f.Subject = "other"
				}
				for _, timeText := range []string{"years ago", "last year", "right now"} {
					if strings.Contains(text, timeText) {
						f.TimeContext = timeText
					}
				}
				if v.concept == "Headache" && strings.Contains(text, "sudden severe headache") {
					f.Concept = "Sudden severe headache"
				}
				p.Findings = append(p.Findings, f)
			}
		}
	}
	if !in.ReviewOnly && len(in.ActiveWarnings) == 0 {
		questions := []string{"When did you first notice the symptoms?", "What other symptoms have you noticed?"}
		if strings.Contains(strings.ToLower(in.Concern), "tired") {
			questions = append(questions, "What medicines do you take?")
		}
		asked := map[string]bool{}
		for _, a := range in.Answers {
			if a.Kind == "reply" {
				asked[a.Question] = true
			}
		}
		for _, q := range questions {
			if !asked[q] {
				p.Action = "ask"
				p.Question = q
				p.Outstanding = []string{q}
				p.Reason = "The answer could clarify the timing or relevant clinical context."
				break
			}
		}
	}
	return p, meta, clinical.Validate(in, p)
}
