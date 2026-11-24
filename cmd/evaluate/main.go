// Command evaluate runs a blinded, authored synthetic-case comparison.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/ctlplne/healthctl/evaluation"
	"github.com/ctlplne/healthctl/internal/evidence"
	"github.com/ctlplne/healthctl/internal/provider"
)

func main() { os.Exit(run()) }

func run() int {
	caseID := flag.String("case", "", "Run only this case ID")
	live := flag.Bool("live", false, "Send disclosed synthetic data to the configured remote API")
	output := flag.String("out", "", "Write JSON results to this local file (stdout by default)")
	maxTurns := flag.Int("max-turns", 5, "Maximum number of answered follow-up questions per adaptive interview")
	release := flag.Bool("release", false, "Run the expanded release benchmark with static forms and report-guided journeys")
	repetitions := flag.Int("repeat", 1, "Repeat each model configuration 1–10 times")
	simulate := flag.String("simulate", "", "Answer with a simulated user instead of the scripted replies: natural (offline rephrasing) or model (a separate model; needs MODEL_SIMULATOR_ENDPOINT, MODEL_SIMULATOR_MODEL and MODEL_SIMULATOR_KEY)")
	flag.Parse()
	if *simulate != "" && *simulate != "natural" && *simulate != "model" {
		fmt.Fprintln(os.Stderr, "simulate must be natural or model")
		return 2
	}
	if *repetitions < 1 || *repetitions > 10 {
		fmt.Fprintln(os.Stderr, "repeat must be 1–10")
		return 2
	}
	if *maxTurns < 1 || *maxTurns > 12 {
		fmt.Fprintln(os.Stderr, "max-turns must be between 1 and 12")
		return 2
	}
	var model evaluation.Model = evaluation.Fixture{}
	mode := "deterministic_fixture"
	if *live {
		cfg := provider.Config{Kind: os.Getenv("MODEL_LIVE_KIND"), Model: os.Getenv("MODEL_LIVE_MODEL"), Endpoint: os.Getenv("MODEL_LIVE_ENDPOINT"), Structured: os.Getenv("MODEL_LIVE_STRUCTURED")}
		if cfg.Kind == "demo" || (cfg.Kind == "compatible" && !strings.HasPrefix(cfg.Endpoint, "https://")) {
			fmt.Fprintln(os.Stderr, "select a remote HTTPS provider")
			return 2
		}
		client := provider.NewRemoteOnly()
		if err := client.Validate(cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		model = evaluation.Remote{Client: client, Config: cfg}
		mode = "remote_api"
	}
	cases, err := evaluation.Cases()
	if *release {
		cases, err = evaluation.ReleaseCases()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	report := struct {
		Version       string                  `json:"evaluationVersion"`
		Provider      string                  `json:"provider"`
		Configuration string                  `json:"configurationFingerprint"`
		Dataset       string                  `json:"datasetFingerprint"`
		Corpus        string                  `json:"starterCorpusVersion"`
		Mode          string                  `json:"mode"`
		Model         string                  `json:"model"`
		Runs          int                     `json:"runs"`
		FailedRuns    int                     `json:"failedRuns"`
		Scores        *evaluation.Scores      `json:"scorecard,omitempty"`
		ReportCases   []evaluation.ReportCase `json:"reportComparison,omitempty"`
		Results       []evaluation.Result     `json:"results"`
	}{Version: "synthetic-1", Mode: mode, Provider: model.Reference().Kind, Model: model.Reference().Model, Dataset: evaluation.DatasetFingerprint(), Corpus: evaluation.StarterCorpus()}
	cfgJSON, _ := json.Marshal(model.Reference())
	report.Configuration = evidence.Hash(cfgJSON)[:20]
	arms := []string{"free_text_only", "adaptive"}
	if *release {
		report.Version = "synthetic-release-2"
		arms = append(arms, "static_form")
	}
	for trial := range *repetitions {
		for _, c := range cases {
			if *caseID != "" && c.ID != *caseID {
				continue
			}
			if *simulate != "" {
				// A simulated user replaces the scripted replies; the app's own
				// invariants score the journey.
				var sim evaluation.Answerer = evaluation.NewNatural(c, int64(trial+1))
				if *simulate == "model" {
					sim = evaluation.ModelSimulator{Endpoint: os.Getenv("MODEL_SIMULATOR_ENDPOINT"), Model: os.Getenv("MODEL_SIMULATOR_MODEL"), Key: os.Getenv("MODEL_SIMULATOR_KEY"), Case: c}
				}
				result := evaluation.RunSimulated(ctx, c, model, sim, *maxTurns)
				result.Arm = "simulated_" + *simulate
				result.Repetition = trial + 1
				result.RunID = fmt.Sprintf("%s/%s/%s/%02d", report.Configuration, c.ID, result.Arm, trial+1)
				report.Results = append(report.Results, result)
				report.Runs++
				if result.Failed() {
					report.FailedRuns++
				}
				continue
			}
			for _, arm := range arms {
				if ctx.Err() != nil {
					break
				}
				result := evaluation.RunArm(ctx, c, arm, model, *maxTurns)
				result.Repetition = trial + 1
				result.RunID = fmt.Sprintf("%s/%s/%s/%02d", report.Configuration, c.ID, arm, trial+1)
				report.Results = append(report.Results, result)
				report.Runs++
				if result.Failed() {
					report.FailedRuns++
				}
			}
		}
	}
	if *release {
		scores := evaluation.Score(report.Results)
		report.Scores = &scores
		if *caseID == "" && ctx.Err() == nil {
			for trial := range *repetitions {
				for _, p := range evaluation.ReportComparisons(ctx, model) {
					p.RunID = fmt.Sprintf("%s/report/%s/%02d", report.Configuration, p.Variant, trial+1)
					p.Repetition = trial + 1
					report.ReportCases = append(report.ReportCases, p)
					if len(p.Failures) > 0 {
						report.FailedRuns++
					}
				}
				if ctx.Err() != nil {
					break
				}
			}
		}
	}
	if report.Runs == 0 {
		fmt.Fprintln(os.Stderr, "no cases ran; check the case ID")
		return 2
	}
	var dest io.Writer = os.Stdout
	if *output != "" {
		f, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		defer f.Close()
		dest = f
	}
	encoder := json.NewEncoder(dest)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "%s: %d runs, %d with errors or invariant failures\n", mode, report.Runs, report.FailedRuns)
	if ctx.Err() != nil {
		return 130
	}
	if report.FailedRuns != 0 {
		return 1
	}
	return 0
}
