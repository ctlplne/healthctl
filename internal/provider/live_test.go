package provider

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ctlplne/healthctl/internal/clinical"
)

// Run only on explicit operator opt-in, with integrated clinical validation and
// synthetic inputs. Normal test runs do not contact any model provider.
func TestLiveRemoteSynthetic(t *testing.T) {
	if os.Getenv("MODEL_LIVE") != "1" {
		t.Skip("set MODEL_LIVE=1 and select a remote provider/model to enable")
	}
	cfg := Config{Kind: os.Getenv("MODEL_LIVE_KIND"), Model: os.Getenv("MODEL_LIVE_MODEL"), Endpoint: os.Getenv("MODEL_LIVE_ENDPOINT"), Structured: os.Getenv("MODEL_LIVE_STRUCTURED")}
	if cfg.Kind == "demo" || (cfg.Kind == "compatible" && !strings.HasPrefix(cfg.Endpoint, "https://")) {
		t.Fatal("live check requires a remote HTTPS API")
	}
	c := NewRemoteOnly()
	if err := c.Test(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	intake := clinical.Intake{Age: 34, Concern: "For two days my nose has been runny and I have been sneezing.", Background: "This started after a day outdoors. My eyes feel itchy.", NoticeVersion: clinical.NoticeVersion}
	enc, err := clinical.New("synthetic-live", intake, clinical.ProviderRef{Kind: cfg.Kind, Model: cfg.Model, Endpoint: cfg.Endpoint, Structured: cfg.Structured}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range []string{"interview", "assessment"} {
		if task == "assessment" {
			for _, action := range []string{"review", "confirm", "assess"} {
				if err := enc.Apply(clinical.Command{Action: action}, ""); err != nil {
					t.Fatal(err)
				}
			}
		}
		input, err := enc.ModelInput(task)
		if err != nil {
			t.Fatal(err)
		}
		var p clinical.Proposal
		_, meta, err := c.Generate(t.Context(), cfg, task, input, func(raw []byte) error {
			p = clinical.Proposal{}
			if err := json.Unmarshal(raw, &p); err != nil {
				return err
			}
			return clinical.Validate(input, p)
		})
		if err != nil {
			t.Fatalf("%s: %v", task, err)
		}
		m := clinical.ModelMeta{Provider: meta.Provider, Endpoint: meta.Endpoint, RequestedModel: meta.RequestedModel, ReturnedModel: meta.ReturnedModel, AppVersion: meta.AppVersion, PromptVersion: meta.PromptVersion, SchemaVersion: meta.SchemaVersion, Settings: meta.Settings}
		if err := enc.Accept(task, p, m, ""); err != nil {
			t.Fatal(err)
		}
		t.Logf("validated %s action %q via %s, requested %q, returned %q", task, p.Action, cfg.Kind, meta.RequestedModel, meta.ReturnedModel)
	}
	if len(enc.Assessments) != 1 || enc.Assessments[0].ReviewStatus != clinical.ReviewStatus {
		t.Fatal("synthetic assessment was not committed")
	}
}
