package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const fixtureBase = "http://127.0.0.1:18031/v1"
const proposal = `{"action":"review","question":"","choices":[],"reason":"The history can be checked.","findings":[],"urgentEvidence":[],"outstanding":[],"items":[]}`

func newFixtureClient(t *testing.T) (*Client, Config) {
	t.Helper()
	for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "BYOM_API_KEY"} {
		t.Setenv(key, "fixture")
		t.Setenv(key+"_FILE", "")
	}
	t.Setenv("ANTHROPIC_WORKSPACE_ID", "fixture-workspace")
	t.Setenv("CLINICAL_ALLOWED_ENDPOINTS", fixtureBase)
	t.Setenv("BYOM_KEY_ENDPOINT", fixtureBase)
	return New(), Config{Kind: "compatible", Model: "operator-model", Endpoint: fixtureBase}
}

func (c *Client) fixtureTransport(f transportFunc) {
	for _, client := range c.httpClients {
		client.Transport = f
	}
}

func response(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

func envelope(kind, reply string) string {
	textBlock := map[string]string{"type": "text", "text": reply}
	obj := map[string]any{"model": "exact-returned-model"}
	switch kind {
	case "openai":
		textBlock["type"] = "output_text"
		obj["status"] = "completed"
		obj["output"] = []any{map[string]any{"type": "reasoning", "summary": []any{}}, map[string]any{"type": "message", "role": "assistant", "status": "completed", "content": []any{textBlock}}}
	case "anthropic":
		obj["stop_reason"] = "end_turn"
		obj["content"] = []any{textBlock}
	case "compatible":
		obj["choices"] = []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": reply}}}
	}
	raw, _ := json.Marshal(obj)
	return string(raw)
}

func TestConfiguredProtocolsAndProvenance(t *testing.T) {
	tests := []struct {
		kind, structured, route, field string
		native                         bool
	}{
		{"compatible", "auto", "/v1/chat/completions", "response_format", false},
		{"compatible", "native", "/v1/chat/completions", "response_format", true},
		{"openai", "auto", "/v1/responses", "text", true},
		{"openai", "prompt", "/v1/responses", "text", false},
		{"anthropic", "auto", "/v1/messages", "output_config", true},
		{"anthropic", "prompt", "/v1/messages", "output_config", false},
	}
	for _, tc := range tests {
		t.Run(tc.kind+"_"+tc.structured, func(t *testing.T) {
			c, cfg := newFixtureClient(t)
			cfg.Kind = tc.kind
			cfg.Structured = tc.structured
			if cfg.Kind != "compatible" {
				cfg.Endpoint = ""
			}
			calls := 0
			c.fixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "POST" || r.URL.Path != tc.route || r.Header.Get("Authorization") != "Bearer fixture" {
					t.Fatal("unexpected method, route or authorization")
				}
				if tc.kind == "anthropic" && (r.Header.Get("anthropic-version") != anthropicVersion || r.Header.Get("anthropic-workspace-id") != "fixture-workspace") {
					t.Fatal("missing pinned API version/workspace")
				}
				var b map[string]any
				if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
					t.Fatal(err)
				}
				if _, present := b[tc.field]; present != tc.native {
					t.Fatal("incorrect structured output capability")
				}
				if b["model"] != cfg.Model || b["tools"] != nil || b["stream"] != nil {
					t.Fatal("unexpected model or tool/stream settings")
				}
				if tc.kind == "openai" && b["store"] != false {
					t.Fatal("response storage was not disabled")
				}
				if b["max_tokens"] != float64(maxOutputTokens) && b["max_output_tokens"] != float64(maxOutputTokens) {
					t.Fatal("missing output budget")
				}
				wire, _ := json.Marshal(b)
				if !strings.Contains(string(wire), "synthetic-data-only") {
					t.Fatal("missing supplied input")
				}
				return response(200, envelope(tc.kind, proposal)), nil
			})
			validated := false
			raw, meta, err := c.Generate(t.Context(), cfg, "interview", map[string]string{"concern": "synthetic-data-only"}, func(b []byte) error { validated = string(b) == proposal; return nil })
			if err != nil || !validated || calls != 1 || string(raw) != proposal {
				t.Fatalf("call failed: %v", err)
			}
			if meta.RequestedModel != cfg.Model || meta.ReturnedModel != "exact-returned-model" || meta.Endpoint != cfg.Destination() || meta.Provider != tc.kind || meta.SchemaVersion == "" || meta.PromptVersion == "" || meta.AppVersion == "" || meta.Settings["requests"] != 1 {
				t.Fatalf("incomplete provenance: %#v", meta)
			}
		})
	}
}

func TestRepairLimitedAndSameDestination(t *testing.T) {
	for _, sometimesValid := range []bool{true, false} {
		c, cfg := newFixtureClient(t)
		calls := 0
		c.fixtureTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.String() != fixtureBase+"/chat/completions" {
				t.Fatal("destination changed")
			}
			data, _ := io.ReadAll(r.Body)
			if calls == 2 && (!strings.Contains(string(data), "prior proposal") || strings.Contains(string(data), "PAYLOAD-IGNORED")) {
				t.Fatal("repair must regenerate from original source")
			}
			reply := proposal
			if calls == 1 {
				reply = strings.Replace(proposal, "The history can be checked.", "PAYLOAD-IGNORED", -1)
			}
			return response(200, envelope(cfg.Kind, reply)), nil
		})
		out, _, err := c.Generate(t.Context(), cfg, "interview", struct{}{}, func(p []byte) error {
			if sometimesValid && string(p) == proposal {
				return nil
			}
			return errors.New("semantic disagreement")
		})
		if calls != 2 || sometimesValid != (err == nil) || (err != nil && out != nil) {
			t.Fatalf("calls=%d, error=%v", calls, err)
		}
	}
}

func TestMalformedContractsRejected(t *testing.T) {
	var schema schemaNode
	if err := json.Unmarshal(proposalSchema, &schema); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		strings.Replace(proposal, `"question":""`, `"question":"","question":""`, 1),
		strings.Replace(proposal, `"choices":[]`, `"choices":null`, 1),
		strings.Replace(proposal, `"urgentEvidence":[],`, "", 1),
		strings.Replace(proposal, `"reason"`, `"Reason"`, 1),
		strings.Replace(proposal, "\"review\"", "\"override_notice\"", 1),
		strings.Replace(proposal, "\"items\":[]", "\"items\":[],\"nextEndpoint\":\"synthetic\"", 1),
		proposal + "{}", "~~~json\n" + proposal + "\n~~~", strings.Repeat("x", maxProposalBytes+1),
	} {
		if validateJSON([]byte(data), &schema) == nil {
			t.Fatal("malformed contract was accepted")
		}
	}
}

func TestFailureIsBoundedAndPrivate(t *testing.T) {
	cases := []struct {
		label string
		code  int
		data  string
		calls int
	}{
		{"denied", 401, "synthetic-sensitive-detail", 1},
		{"incompatible-capability", 400, "synthetic-sensitive-detail", 1},
		{"redirect", 307, "", 1},
		{"temporary", 503, "", 2},
		{"large", 200, strings.Repeat("a", maxEnvelopeBytes+1), 1},
		{"truncated", 200, `{"choices":[{"finish_reason":"length","message":{"content":"synthetic-sensitive-detail"}}]}`, 1},
		{"unreadable", 200, "synthetic-sensitive-detail", 2},
		{"invalid-schema", 200, envelope("compatible", `{}`), 2},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			c, cfg := newFixtureClient(t)
			calls := 0
			c.fixtureTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host != "127.0.0.1:18031" {
					t.Fatal("destination changed")
				}
				calls++
				resp := response(tc.code, tc.data)
				resp.Header.Set("Retry-After", "0")
				if tc.code == 307 {
					resp.Header.Set("Location", "https://another.example")
				}
				return resp, nil
			})
			raw, _, err := c.Generate(t.Context(), cfg, "assessment", struct{}{}, func([]byte) error { return nil })
			if raw != nil || calls != tc.calls || err == nil || strings.Contains(err.Error(), "synthetic-sensitive-detail") {
				t.Fatalf("calls=%d, err=%v", calls, err)
			}
		})
	}
}

func TestDeadlineAndCancellation(t *testing.T) {
	c, cfg := newFixtureClient(t)
	c.fixtureTransport(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Millisecond)
	defer cancel()
	if err := c.Test(ctx, cfg); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
	cancelled, stop := context.WithCancel(t.Context())
	stop()
	if err := c.Test(cancelled, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

func TestProbeNonclinicalAndBudget(t *testing.T) {
	c, cfg := newFixtureClient(t)
	calls := 0
	c.fixtureTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(raw), "Check the configured model connection.") || strings.Contains(string(raw), "patient interview") {
			t.Fatal("probe payload includes clinical template")
		}
		return response(200, envelope(cfg.Kind, `{"status":"ready"}`)), nil
	})
	if err := c.Test(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	if raw, _, err := c.Generate(t.Context(), cfg, "interview", strings.Repeat("x", maxInputBytes), func([]byte) error { return nil }); err == nil || raw != nil {
		t.Fatal("over-budget input accepted")
	}
	if calls != 1 {
		t.Fatal("over-budget input reached provider")
	}
}

func TestConfigurationAndDestinationIsolation(t *testing.T) {
	c, cfg := newFixtureClient(t)
	if err := c.Validate(cfg); err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{"file:///v1", "https://user:pass@example.org/v1", "http://169.254.169.254/v1", "https://example.org/v1?token=foo", "https://metadata.google.internal/v1", "https://example.org/v1/../v2", "https://elsewhere.example/v1", "http://0.0.0.0/v1", "http://8.8.8.8/v1"} {
		cfg.Endpoint = base
		if err := c.Validate(cfg); err == nil {
			t.Fatalf("accepted disallowed destination: %s", base)
		}
	}
	for _, sample := range []struct {
		ip                   string
		private, local, want bool
	}{
		{"fd00:ec2::254", true, false, false}, {"169.254.1.1", true, true, false}, {"fd01::1234", true, true, true},
		{"127.0.0.1", false, false, false}, {"8.8.8.8", true, true, false}, {"::ffff:169.254.1.1", true, false, false},
		{"192.168.1.9", true, true, true}, {"0.0.0.0", true, false, false},
	} {
		if permittedIP(netip.MustParseAddr(sample.ip), sample.private, sample.local) != sample.want {
			t.Fatalf("unexpected address policy for %s", sample.ip)
		}
	}
	t.Setenv("CLINICAL_ALLOWED_ENDPOINTS", fixtureBase+",https://other.example/v1")
	c = New()
	cfg.Endpoint = "https://other.example/v1"
	if err := c.Validate(cfg); err != nil {
		t.Fatal(err)
	}
	if key, err := c.credential(cfg); key != "" || err != nil {
		t.Fatal("key was shared across endpoints")
	}
	cfg.Kind = "openai"
	if err := c.Validate(cfg); err == nil {
		t.Fatal("official credential allowed custom endpoint")
	}
}

func TestSecretFileAndMissingKey(t *testing.T) {
	c, cfg := newFixtureClient(t)
	cfg = Config{Kind: "openai", Model: "configured"}
	t.Setenv("OPENAI_API_KEY", "")
	c = New()
	c.fixtureTransport(func(r *http.Request) (*http.Response, error) {
		t.Fatal("missing credential reached transport")
		return nil, nil
	})
	if err := c.Test(t.Context(), cfg); err == nil || !strings.Contains(err.Error(), "OPENAI_API_KEY") {
		t.Fatal("missing key needs setup instructions")
	}
	f := filepath.Join(t.TempDir(), "openai_key")
	if err := os.WriteFile(f, []byte("fixture-file-key\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY_FILE", f)
	c = New()
	key, err := c.credential(cfg)
	if err != nil || key != "fixture-file-key" {
		t.Fatal("file credential unavailable")
	}
	t.Setenv("OPENAI_API_KEY", "another")
	if _, err := New().credential(cfg); err == nil {
		t.Fatal("ambiguous secret sources accepted")
	}
}

func TestSameDestinationTransientRecovery(t *testing.T) {
	c, cfg := newFixtureClient(t)
	n := 0
	c.fixtureTransport(func(r *http.Request) (*http.Response, error) {
		n++
		if r.URL.String() != fixtureBase+"/chat/completions" {
			t.Fatal("destination changed")
		}
		if n == 1 {
			resp := response(429, "")
			resp.Header.Set("Retry-After", "0")
			return resp, nil
		}
		return response(200, envelope(cfg.Kind, proposal)), nil
	})
	_, meta, err := c.Generate(t.Context(), cfg, "interview", struct{}{}, func([]byte) error { return nil })
	if err != nil || n != 2 || meta.Settings["requests"] != 2 {
		t.Fatalf("transient failure recovery: %v", err)
	}
}

func TestUsageAcrossFailedOutputAndRepair(t *testing.T) {
	c, cfg := newFixtureClient(t)
	t.Setenv("CLINICAL_MODEL_PRICES", `[{"provider":"compatible","model":"operator-model","endpoint":"http://127.0.0.1:18031/v1","inputUSDPerMillion":1,"outputUSDPerMillion":3}]`)
	iter := 0
	c.fixtureTransport(func(req *http.Request) (*http.Response, error) {
		iter++
		reply := proposal
		if iter == 1 {
			reply = "{}"
		}
		var w map[string]any
		json.Unmarshal([]byte(envelope("compatible", reply)), &w)
		w["usage"] = map[string]any{"prompt_tokens": 100, "completion_tokens": 50}
		b, _ := json.Marshal(w)
		return response(200, string(b)), nil
	})
	_, m, err := c.Generate(t.Context(), cfg, "interview", struct{}{}, func([]byte) error { return nil })
	if err != nil || m.Usage.Reported != 2 || m.Usage.Retries != 1 || m.Usage.InputTokens != 200 || m.Usage.OutputTokens != 100 || m.Usage.EstimatedUSD == nil || *m.Usage.EstimatedUSD != 0.0005 {
		t.Fatalf("incorrect measured usage %+v (%v)", m.Usage, err)
	}
	c.fixtureTransport(func(*http.Request) (*http.Response, error) { return response(401, ""), nil })
	_, m, err = c.Generate(t.Context(), cfg, "interview", struct{}{}, func([]byte) error { return nil })
	if err == nil || m.Usage.Requests != 1 || m.Usage.Reported != 0 || m.Usage.EstimatedUSD != nil {
		t.Fatal("missing usage on rejection reported as complete")
	}
}

func TestRepairCarriesReasonCodes(t *testing.T) {
	c, cfg := newFixtureClient(t)
	var second string
	calls := 0
	c.fixtureTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		data, _ := io.ReadAll(r.Body)
		if calls == 2 {
			second = string(data)
		}
		reply := proposal
		if calls == 1 {
			reply = strings.Replace(proposal, "The history can be checked.", "REJECTED-MODEL-TEXT", -1)
		}
		return response(200, envelope(cfg.Kind, reply)), nil
	})
	_, meta, err := c.Generate(t.Context(), cfg, "interview", struct{}{}, func(p []byte) error {
		if strings.Contains(string(p), "REJECTED-MODEL-TEXT") {
			return hintError{}
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("repair did not complete: calls=%d err=%v", calls, err)
	}
	if !strings.Contains(second, "APP-AUTHORED-HINT multiple_questions") || strings.Contains(second, "REJECTED-MODEL-TEXT") {
		t.Fatal("the repair request must carry the app's reason codes and never the rejected text")
	}
	if meta.FailureReason != "" {
		t.Fatal("a repaired call is not a failure")
	}
	_, meta, err = c.Generate(t.Context(), cfg, "interview", struct{}{}, func([]byte) error { return hintError{} })
	if err == nil || meta.FailureReason != "multiple_questions" {
		t.Fatalf("a failed call must record its reason code, got %q", meta.FailureReason)
	}
}

type hintError struct{}

func (hintError) Error() string      { return "invalid" }
func (hintError) RepairHint() string { return "APP-AUTHORED-HINT multiple_questions" }
func (hintError) ReasonCode() string { return "multiple_questions" }
