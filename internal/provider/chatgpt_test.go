package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
)

const accountOne = "0123456789abcdef0123456789abcdef"
const accountTwo = "abcdef0123456789abcdef0123456789"

func planCfg() Config { return Config{Kind: "chatgpt", Endpoint: accountOne, Model: "plan-fixture"} }
func fixturePlanCatalog() string {
	return `{"models":[{"slug":"plan-second","display_name":"Second model","visibility":"list"},{"slug":"hidden-slug","display_name":"Hidden","visibility":"hide"},{"slug":"plan-fixture","display_name":"Test model","visibility":"list","supported_reasoning_levels":[{"effort":"medium"},{"effort":"high"}],"base_instructions":"NEVER_USE_CATALOG_INSTRUCTIONS"}]}`
}
func planEvent(kind string, payload map[string]any) string {
	payload["type"] = kind
	b, _ := json.Marshal(payload)
	return fmt.Sprintf("event: %s\r\ndata: %s\r\n\r\n", kind, b)
}
func planCompletion(text string, count int) string {
	var w map[string]any
	json.Unmarshal([]byte(envelope("openai", text)), &w)
	w["usage"] = map[string]int{"input_tokens": 21, "output_tokens": count}
	return planEvent("response.completed", map[string]any{"response": w})
}
func planResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestChatGPTPlanSocketModelCatalogAndInference(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if errors.Is(err, syscall.EPERM) {
		t.Skip("loopback networking unavailable in sandbox")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "SENTINEL_PLATFORM_API_KEY")
	t.Setenv("OPENAI_API_KEY_FILE", "")
	t.Setenv("CLINICAL_MODEL_PRICES", `[{"provider":"chatgpt","model":"plan-fixture","inputUSDPerMillion":500,"outputUSDPerMillion":1000}]`)
	var inference, catalog atomic.Int32
	server := &httptest.Server{Listener: ln, Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-one" || r.Header.Get("Cookie") != "" {
			t.Error("wrong account token or ambient credentials")
		}
		switch r.URL.Path {
		case "/v1/models":
			catalog.Add(1)
			if r.Method != "GET" || r.URL.RawQuery != "" {
				t.Error("unexpected model catalog request")
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, fixturePlanCatalog())
		case "/v1/responses":
			inference.Add(1)
			if r.Method != "POST" || r.Header.Get("Accept") != "text/event-stream" {
				t.Error("wrong inference method or headers")
			}
			var req map[string]json.RawMessage
			json.NewDecoder(r.Body).Decode(&req)
			for _, key := range []string{"temperature", "max_output_tokens", "background", "conversation", "max_tool_calls", "metadata", "moderation", "multi_agent", "prompt", "prompt_cache_retention", "safety_identifier", "top_logprobs", "top_p", "truncation", "user", "previous_response_id", "tools", "additional_tools", "store_id"} {
				if _, ok := req[key]; ok {
					t.Errorf("unsupported field %s", key)
				}
			}
			if len(req) != 6 || string(req["reasoning"]) != `{"effort":"high"}` || string(req["store"]) != "false" || string(req["stream"]) != "true" || string(req["model"]) != `"plan-fixture"` || strings.Contains(string(req["instructions"]), "NEVER_USE_CATALOG_INSTRUCTIONS") {
				t.Error("incorrect preview route request body")
			}
			var input []struct{ Role, Content string }
			if json.Unmarshal(req["input"], &input) != nil || len(input) != 1 || input[0].Role != "user" {
				t.Error("invalid responses input")
				return
			}
			output := proposal
			if strings.Contains(input[0].Content, "connect") {
				output = `{"status":"ready"}`
			} else if !strings.Contains(input[0].Content, "synthetic disclosed history") {
				t.Error("wrong clinical input")
			}
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			io.WriteString(w, ": heartbeat\n\n")
			io.WriteString(w, planEvent("response.output_text.delta", map[string]any{"delta": output}))
			io.WriteString(w, planCompletion(output, 7))
		default:
			t.Error("unknown inference endpoint")
			w.WriteHeader(404)
		}
	})}}
	server.Start()
	t.Cleanup(server.Close)
	fixtureURL, _ := url.Parse(server.URL)
	c := NewWithChatGPT(func(ctx context.Context, key string) (string, error) {
		if key != accountOne {
			return "", errors.New("SENTINEL_LOGIN_ERROR")
		}
		return "synthetic-one", nil
	})
	fixtureHTTP := server.Client()
	wire := fixtureHTTP.Transport
	// The application constructs the production URL. This test-only transport
	// directs the vetted request to the local fixture's actual TCP listener.
	c.httpClients["official"] = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" || r.URL.Host != "api.openai.com" {
			t.Fatal("unapproved plan destination")
		}
		local := r.Clone(r.Context())
		local.URL.Scheme, local.URL.Host = fixtureURL.Scheme, fixtureURL.Host
		return wire.RoundTrip(local)
	})}
	c.httpClients["plan"] = c.httpClients["official"]
	cfg := planCfg()
	models, err := c.ChatGPTModels(t.Context(), accountOne)
	if err != nil || len(models) != 2 || models[0].Slug != "plan-second" || len(models[0].ReasoningLevels) != 0 || models[1].DisplayName != "Test model" || strings.Join(models[1].ReasoningLevels, " ") != "medium high" {
		t.Fatalf("wrong account model catalog %v %v", models, err)
	}
	if err := c.Test(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	result, meta, err := c.Generate(t.Context(), cfg, "interview", map[string]string{"concern": "synthetic disclosed history"}, func([]byte) error { return nil })
	if err != nil || string(result) != proposal || meta.Provider != "chatgpt" || meta.Endpoint != accountOne || meta.ReturnedModel != "exact-returned-model" || meta.Usage.OutputTokens != 7 || meta.Usage.EstimatedUSD != nil || meta.Settings["billing"] != "chatgpt_plan" || meta.Settings["reasoningEffort"] != "high" || meta.Settings["maxOutputTokens"] != nil {
		t.Fatalf("invalid validated result or provenance %v", err)
	}
	cfg.Endpoint = accountTwo
	if err := c.Test(t.Context(), cfg); err == nil || strings.Contains(err.Error(), "SENTINEL") {
		t.Fatal("disconnected account switched accounts or exposed resolver error")
	}
	cfg.Endpoint = accountOne
	cfg.Model = "hidden-slug"
	if err := c.Test(t.Context(), cfg); err == nil {
		t.Fatal("accepted nonselectable model")
	}
	if inference.Load() != 2 || catalog.Load() != 4 {
		t.Fatal("unexpected network operation count")
	}
}

// The subscription endpoint may return event frames with a generic, missing
// or malformed media type. Only the terminal event can validate the probe.
func TestChatGPTProbeReadsBodyWithInconsistentContentType(t *testing.T) {
	ready := planCompletion(`{"status":"ready"}`, 7)
	for _, tc := range []struct {
		name, contentType, body string
		wantOK                  bool
		wantMessage             string
	}{
		{"no header", "", ready, true, ""},
		{"binary", "application/octet-stream", ready, true, ""},
		{"plain text", "text/plain; charset=utf-8", ready, true, ""},
		{"JSON label", "application/json", ready, true, ""},
		{"duplicate media type", "text/event-stream, text/event-stream", ready, true, ""},
		{"partial stream", "", planEvent("response.output_text.delta", map[string]any{"delta": `{"status":"ready"}`}), false, ""},
		{"wrong result", "application/octet-stream", planCompletion(`{"status":"offline"}`, 7), false, ""},
		{"HTML", "text/html", "<html>SENTINEL_PRIVATE</html>\n\n" + ready, false, "HTML"},
		{"quota JSON", "application/json", `{"error":{"code":"subscription_sharing_usage_limit_exceeded","message":"SENTINEL_PRIVATE"}}`, false, "usage limit"},
		{"bare JSON", "application/json", `{"status":"ready"}`, false, "JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewWithChatGPT(func(context.Context, string) (string, error) { return "plan-grant", nil })
			posts := 0
			c.fixtureTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method == "GET" {
					return planResponse(fixturePlanCatalog()), nil
				}
				posts++
				res := planResponse(tc.body)
				res.Header.Set("Content-Type", tc.contentType)
				return res, nil
			})
			err := c.Test(t.Context(), planCfg())
			if (err == nil) != tc.wantOK || posts != 1 {
				t.Fatalf("posts=%d, probe error=%v", posts, err)
			}
			if err != nil && (strings.Contains(err.Error(), "SENTINEL_PRIVATE") || !strings.Contains(err.Error(), tc.wantMessage)) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestChatGPTSubscriptionStreamsFailClosed(t *testing.T) {
	quota := planEvent("response.failed", map[string]any{"response": map[string]any{"error": map[string]string{"code": "subscription_sharing_usage_limit_exceeded", "message": "SENTINEL_REMOTE_DETAIL"}}})
	tool := planEvent("response.output_item.added", map[string]any{"item": map[string]string{"type": "function_call"}})
	delta := planEvent("response.output_text.delta", map[string]any{"delta": proposal})
	for _, tc := range []struct{ name, data string }{
		{"delta only", delta}, {"interrupted", delta + "data: {"}, {"invalid terminator", delta + "data: [DONE]\n\n"},
		{"quota after output", delta + quota}, {"truncated", delta + planEvent("response.incomplete", map[string]any{})},
		{"explicit error", delta + planEvent("error", map[string]any{"code": "subscription_sharing_usage_unavailable"})},
		{"unexpected tool", tool + planCompletion(proposal, 10)}, {"different completion", delta + planCompletion("other", 10)},
		{"oversized answer", planEvent("response.output_text.delta", map[string]any{"delta": strings.Repeat("x", maxProposalBytes+1)})},
		{"too many output tokens", planCompletion(proposal, maxOutputTokens+1)},
		{"oversized ignored data", ":" + strings.Repeat("a", maxEnvelopeBytes) + "\n\n" + planCompletion(proposal, 10)},
		{"oversized stream", strings.Repeat(":"+strings.Repeat("a", 1022)+"\n", maxPlanStreamBytes/1024) + planCompletion(proposal, 10)},
		{"oversized multiline event", "data: " + strings.Repeat(" ", maxEnvelopeBytes/2) + "\ndata: " + strings.Repeat(" ", maxEnvelopeBytes/2) + "\n\n"},
		{"event type mismatch", "event: response.completed\ndata: {\"type\":\"response.failed\"}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := Usage{}
			result, model, err := readPlanStream(strings.NewReader(tc.data), "req_123", maxOutputTokens, &u)
			if err == nil || result != nil || model != "" || strings.Contains(err.Error(), "SENTINEL") {
				t.Fatal("invalid stream produced output or leaked diagnostics")
			}
			if tc.name == "quota after output" {
				var e *ChatGPTError
				if !errors.As(err, &e) || e.RequestID != "req_123" || e.Code != "subscription_sharing_usage_limit_exceeded" || e.BodyShape != "response.failed" {
					t.Fatal("quota recovery metadata missing")
				}
			}
		})
	}
}

func TestChatGPTReasoningStreamKeepsFullUsageAndBoundsAnswer(t *testing.T) {
	reasoning := planEvent("response.output_item.done", map[string]any{"item": map[string]string{"type": "reasoning", "encrypted_content": strings.Repeat("x", 96<<10)}})
	for _, tc := range []struct {
		reasoning int
		wantOK    bool
	}{{8192, true}, {0, false}, {-1, false}, {10000, false}} {
		var w map[string]any
		if err := json.Unmarshal([]byte(envelope("openai", proposal)), &w); err != nil {
			t.Fatal(err)
		}
		w["usage"] = map[string]any{"input_tokens": 21, "output_tokens": 8200, "output_tokens_details": map[string]int{"reasoning_tokens": tc.reasoning}}
		stream := strings.Repeat(reasoning, 3) + planEvent("response.completed", map[string]any{"response": w})
		var usage Usage
		result, _, err := readPlanStream(strings.NewReader(stream), "", maxOutputTokens, &usage)
		if (err == nil) != tc.wantOK || tc.wantOK && string(result) != proposal || usage.OutputTokens != 8200 {
			t.Fatalf("reasoning=%d result accepted=%t reported=%d", tc.reasoning, err == nil, usage.OutputTokens)
		}
	}
}

func TestChatGPTPlanDenialsNeverChangeBillingOrRetry(t *testing.T) {
	for _, stage := range []string{"denied catalog", "denied inference", "quota after delta"} {
		t.Run(stage, func(t *testing.T) {
			c := NewWithChatGPT(func(context.Context, string) (string, error) { return "plan-grant", nil })
			posts, gets := 0, 0
			c.fixtureTransport(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Authorization") != "Bearer plan-grant" {
					t.Fatal("switched credential")
				}
				if r.Method == "GET" {
					gets++
					if stage != "denied catalog" {
						return planResponse(fixturePlanCatalog()), nil
					}
				} else {
					posts++
				}
				if stage == "quota after delta" {
					return planResponse(planEvent("response.output_text.delta", map[string]any{"delta": "draft"}) + planEvent("response.failed", map[string]any{"response": map[string]any{"error": map[string]string{"code": "subscription_sharing_usage_limit_exceeded"}}})), nil
				}
				return &http.Response{StatusCode: 403, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"detail":"SENTINEL_PRIVATE_DETAIL"}`))}, nil
			})
			_, meta, err := c.Generate(t.Context(), planCfg(), "interview", struct{}{}, func([]byte) error { return nil })
			var safe *ChatGPTError
			if !errors.As(err, &safe) || strings.Contains(err.Error(), "SENTINEL") || gets != 1 || posts > 1 || posts != meta.Usage.Requests || stage == "denied catalog" && posts != 0 {
				t.Fatal("unexpected usage, retry or error exposure")
			}
		})
	}
}

func TestSubscriptionValidationDoesNotRequireConnectedAccount(t *testing.T) {
	c := New()
	cfg := planCfg()
	if c.Validate(cfg) != nil || c.Test(t.Context(), cfg) == nil {
		t.Fatal("saved configuration must remain readable and fail closed upon use")
	}
	for _, key := range []string{"", "https://api.openai.com/v1", accountOne + "zz", strings.ToUpper(accountTwo), "../" + accountOne} {
		cfg.Endpoint = key
		if c.Validate(cfg) == nil || cfg.Destination() != "" {
			t.Fatal("accepted alternate destination")
		}
	}
}

func TestChatGPTCompletedProposalRequiresDomainValidation(t *testing.T) {
	cfg := planCfg()
	cfg.Structured = "native"
	c := NewWithChatGPT(func(context.Context, string) (string, error) { return "plan-grant", nil })
	var instructionsSeen []string
	c.fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return planResponse(fixturePlanCatalog()), nil
		}
		var body struct {
			Instructions string
			Text         struct {
				Format struct {
					Type   string
					Strict bool
				}
			}
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Text.Format.Type != "json_schema" || !body.Text.Format.Strict {
			t.Fatal("explicit native schema was omitted")
		}
		instructionsSeen = append(instructionsSeen, body.Instructions)
		// Exercise clinical validation and repair on the headerless plan route too.
		reply := planResponse(planCompletion(proposal, 5))
		reply.Header.Del("Content-Type")
		return reply, nil
	})
	accepts := 0
	results, meta, err := c.Generate(t.Context(), cfg, "interview", struct{}{}, func(raw []byte) error {
		accepts++
		if accepts == 1 {
			return errors.New("SENTINEL_INVALID_CLINICAL_INTERPRETATION")
		}
		return nil
	})
	if err != nil || string(results) != proposal || accepts != 2 || len(instructionsSeen) != 2 || instructionsSeen[0] == instructionsSeen[1] || strings.Contains(instructionsSeen[1], "SENTINEL") || meta.Usage.Requests != 2 || meta.Usage.Retries != 1 || meta.Usage.OutputTokens != 10 {
		t.Fatal("invalid clinical proposal bypassed validation or repair bounds")
	}
}

func TestChatGPTUsesOnlyAdvertisedResponsesReasoningEffort(t *testing.T) {
	for _, tc := range []struct {
		name     string
		levels   []string
		want     string
		declined bool
	}{
		{"absent", nil, "", false},
		{"unknown only", []string{"custom"}, "", false},
		{"sole none", []string{"none"}, "none", false},
		{"minimal", []string{"none", "minimal"}, "minimal", false},
		{"unranked list", []string{"low", "custom", "xhigh", "high"}, "xhigh", false},
		{"max", []string{"medium", "max", "xhigh"}, "max", false},
		{"declined", []string{"high", "low"}, "high", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewWithChatGPT(func(context.Context, string) (string, error) { return "opaque-plan-token", nil })
			posts := 0
			c.fixtureTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method == "GET" {
					model := map[string]any{"slug": "plan-fixture", "display_name": "Fixture", "visibility": "list"}
					if tc.levels != nil {
						levels := make([]map[string]string, 0, len(tc.levels))
						for _, level := range tc.levels {
							levels = append(levels, map[string]string{"effort": level})
						}
						model["supported_reasoning_levels"] = levels
					}
					b, _ := json.Marshal(map[string]any{"models": []any{model}})
					return planResponse(string(b)), nil
				}
				posts++
				var request map[string]json.RawMessage
				if json.NewDecoder(r.Body).Decode(&request) != nil {
					t.Fatal("invalid request body")
				}
				reasoning, present := request["reasoning"]
				if present != (tc.want != "") {
					t.Fatal("unexpected presence of request reasoning")
				}
				if present {
					var cfg struct{ Effort string }
					if json.Unmarshal(reasoning, &cfg) != nil || cfg.Effort != tc.want {
						t.Fatalf("wrong reasoning request %s", reasoning)
					}
				}
				if tc.declined {
					return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"subscription_sharing_unsupported_capability","param":"reasoning.effort"}}`))}, nil
				}
				return planResponse(planCompletion(proposal, 5)), nil
			})
			_, meta, err := c.Generate(t.Context(), planCfg(), "interview", struct{}{}, func([]byte) error { return nil })
			if posts != 1 {
				t.Fatal("unexpected retry on effort rejection")
			}
			if tc.declined {
				var planErr *ChatGPTError
				if !errors.As(err, &planErr) || planErr.Param != "reasoning.effort" {
					t.Fatal("rejection was not surfaced")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, ok := meta.Settings["reasoningEffort"]
			if ok != (tc.want != "") || ok && got != tc.want {
				t.Fatal("successful provenance differs from actual reasoning request")
			}
		})
	}
}
