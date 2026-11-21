package provider

import (
	"encoding/json"
	"math"
	"os"
	"time"
)

type Usage struct {
	Requests     int      `json:"requests"`
	Reported     int      `json:"requestsWithTokenCounts"`
	Retries      int      `json:"retries"`
	InputTokens  int64    `json:"inputTokens"`
	OutputTokens int64    `json:"outputTokens"`
	DurationMS   int64    `json:"durationMs"`
	EstimatedUSD *float64 `json:"estimatedUSD"`
}
type Rate struct {
	Provider string   `json:"provider"`
	Model    string   `json:"model"`
	Endpoint string   `json:"endpoint"`
	Input    *float64 `json:"inputUSDPerMillion"`
	Output   *float64 `json:"outputUSDPerMillion"`
}

func ReadRate(cfg Config) Rate {
	if cfg.Kind == "chatgpt" {
		return Rate{}
	}
	var rates []Rate
	if err := json.Unmarshal([]byte(os.Getenv("CLINICAL_MODEL_PRICES")), &rates); err != nil {
		return Rate{}
	}
	for _, r := range rates {
		if r.Provider != cfg.Kind || r.Model != cfg.Model || cfg.Kind == "compatible" && r.Endpoint != cfg.Destination() || r.Input == nil || r.Output == nil {
			continue
		}
		if *r.Input >= 0 && *r.Output >= 0 && *r.Input < 1e6 && *r.Output < 1e6 {
			return r
		}
	}
	return Rate{}
}
func (u *Usage) finish(start time.Time, cfg Config) {
	u.DurationMS = time.Since(start).Milliseconds()
	u.Retries = max(0, u.Requests-1)
	r := ReadRate(cfg)
	if u.Requests == 0 || u.Reported != u.Requests || r.Input == nil {
		return
	}
	price := (float64(u.InputTokens)*(*r.Input) + float64(u.OutputTokens)*(*r.Output)) / 1e6
	if !math.IsNaN(price) && !math.IsInf(price, 0) {
		u.EstimatedUSD = &price
	}
}
func (u *Usage) add(kind string, raw []byte) {
	var w struct {
		Usage struct {
			Input      *int64 `json:"input_tokens"`
			Output     *int64 `json:"output_tokens"`
			Prompt     *int64 `json:"prompt_tokens"`
			Completion *int64 `json:"completion_tokens"`
			CacheRead  int64  `json:"cache_read_input_tokens"`
			CacheWrite int64  `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &w) != nil {
		return
	}
	in, out := w.Usage.Input, w.Usage.Output
	if kind == "compatible" {
		in, out = w.Usage.Prompt, w.Usage.Completion
	}
	if in == nil || out == nil || *in < 0 || *out < 0 || *in > 1e9 || *out > 1e9 {
		return
	}
	input := *in
	if kind == "anthropic" {
		if w.Usage.CacheRead < 0 || w.Usage.CacheRead > 1e9 || w.Usage.CacheWrite < 0 || w.Usage.CacheWrite > 1e9 {
			return
		}
		input += w.Usage.CacheRead + w.Usage.CacheWrite
	}
	u.InputTokens += input
	u.OutputTokens += *out
	u.Reported++
}
func Budgets() map[string]int {
	return map[string]int{"maxOutputTokens": maxOutputTokens, "maxInputBytes": maxInputBytes, "timeoutSeconds": int(requestDeadline.Seconds()), "chatgptClinicalTimeoutSeconds": int(PlanClinicalDeadline.Seconds()), "maxRequestsPerOperation": 3}
}
