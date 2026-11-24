package provider

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReasoningRequestShapes(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	decode := func(cfg Config, task string) map[string]any {
		t.Helper()
		p := call{task: task, input: "{}", schema: schema, maxTokens: outputBudget(cfg.Kind), stream: cfg.streams()}
		if cfg.Reasoning == "effort" {
			p.reasoningEffort = apiEffort[task]
		}
		b, err := p.body(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		json.Unmarshal(b, &m)
		return m
	}
	openai := decode(Config{Kind: "openai", Model: "reasoner", Structured: "native", Reasoning: "effort"}, "assessment")
	if openai["reasoning"].(map[string]any)["effort"] != "high" || openai["stream"] != true || openai["max_output_tokens"] != float64(reasoningOutputTokens) {
		t.Fatalf("openai reasoning request: %v", openai)
	}
	if decode(Config{Kind: "openai", Model: "reasoner", Reasoning: "effort"}, "interview")["reasoning"].(map[string]any)["effort"] != "low" {
		t.Fatal("an interview turn must ask for low effort")
	}
	claude := decode(Config{Kind: "anthropic", Model: "claude-test", Structured: "native", Reasoning: "effort"}, "assessment")
	output := claude["output_config"].(map[string]any)
	if output["effort"] != "high" || output["format"] == nil || claude["stream"] != true || claude["max_tokens"] != float64(reasoningOutputTokens) {
		t.Fatalf("anthropic effort must sit beside the output format: %v", claude)
	}
	plain := decode(Config{Kind: "anthropic", Model: "claude-test"}, "interview")
	if oc, _ := plain["output_config"].(map[string]any); plain["stream"] != nil || oc["effort"] != nil || plain["max_tokens"] != float64(reasoningOutputTokens) {
		t.Fatalf("without effort the call is a single response with room for thinking: %v", plain)
	}
	if compat := decode(Config{Kind: "compatible", Model: "local"}, "interview"); compat["max_tokens"] != float64(maxOutputTokens) || compat["stream"] != nil {
		t.Fatalf("compatible endpoints keep the plain budget: %v", compat)
	}
	c := New()
	if err := c.Validate(Config{Kind: "compatible", Model: "local", Endpoint: "https://example.invalid/v1", Reasoning: "effort"}); err == nil {
		t.Fatal("reasoning effort was accepted for an endpoint that may not support it")
	}
	if err := c.Validate(Config{Kind: "openai", Model: "reasoner", Reasoning: "maximum"}); err == nil {
		t.Fatal("an unknown reasoning setting was accepted")
	}
}

func sse(events ...string) string {
	var b strings.Builder
	for _, e := range events {
		var probe struct{ Type string }
		json.Unmarshal([]byte(e), &probe)
		b.WriteString("event: " + probe.Type + "\ndata: " + e + "\n\n")
	}
	return b.String()
}

func TestAnthropicStreamAssembly(t *testing.T) {
	stream := func(stop string) string {
		return sse(
			`{"type":"message_start","message":{"model":"claude-test","usage":{"input_tokens":12,"cache_read_input_tokens":3}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"private reasoning"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"{\"a\":"}}`,
			`{"type":"ping"}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"1}"}}`,
			`{"type":"content_block_stop","index":1}`,
			`{"type":"message_delta","delta":{"stop_reason":"`+stop+`"},"usage":{"output_tokens":30}}`,
			`{"type":"message_stop"}`,
		)
	}
	var u Usage
	out, model, err := readMessagesStream(strings.NewReader(stream("end_turn")), &u)
	if err != nil || string(out) != `{"a":1}` || model != "claude-test" || u.InputTokens != 15 || u.OutputTokens != 30 || u.Reported != 1 {
		t.Fatalf("stream not assembled: %q %q %v %+v", out, model, err, u)
	}
	for _, stop := range []string{"refusal", "max_tokens", ""} {
		if _, _, err := readMessagesStream(strings.NewReader(stream(stop)), &Usage{}); !errors.Is(err, errIncomplete) {
			t.Fatalf("stop reason %q must fail the call, got %v", stop, err)
		}
	}
	cut := stream("end_turn")
	if _, _, err := readMessagesStream(strings.NewReader(cut[:len(cut)/2]), &Usage{}); !errors.Is(err, errIncomplete) {
		t.Fatal("a dropped stream must fail the call")
	}
	tool := sse(`{"type":"message_start","message":{"model":"claude-test","usage":{"input_tokens":1}}}`, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use"}}`)
	if _, _, err := readMessagesStream(strings.NewReader(tool), &Usage{}); !errors.Is(err, errIncomplete) {
		t.Fatal("a tool call is not an answer")
	}
}

func TestOpenAIResponsesStream(t *testing.T) {
	completed := `{"type":"response.completed","response":{"model":"reasoner-test","status":"completed","output":[{"type":"reasoning"},{"type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"{\"a\":1}"}]}],"usage":{"input_tokens":10,"output_tokens":50,"output_tokens_details":{"reasoning_tokens":40}}}}`
	var u Usage
	out, model, err := readResponsesStream(strings.NewReader(sse(`{"type":"response.output_text.delta","delta":"{\"a\":1}"}`, completed)), "", maxOutputTokens, &u, "openai")
	if err != nil || string(out) != `{"a":1}` || model != "reasoner-test" || u.OutputTokens != 50 {
		t.Fatalf("responses stream: %q %q %v %+v", out, model, err, u)
	}
	if _, _, err := readResponsesStream(strings.NewReader(sse(`{"type":"response.failed","response":{"error":{"code":"server_error"}}}`)), "", maxOutputTokens, &Usage{}, "openai"); !errors.Is(err, errIncomplete) {
		t.Fatalf("a failed stream must fail the call without a plan error, got %v", err)
	}
	if _, _, err := readResponsesStream(strings.NewReader("<html>proxy</html>"), "", maxOutputTokens, &Usage{}, "openai"); !errors.Is(err, errEnvelope) {
		t.Fatal("an HTML page is an unreadable response")
	}
}

func TestNoAnswerYetIsATimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()
	c := &Client{httpClients: map[string]*http.Client{"http": {Transport: &http.Transport{ResponseHeaderTimeout: 50 * time.Millisecond}}}}
	retry := false
	_, _, err := c.request(t.Context(), Config{Kind: "compatible", Model: "local", Endpoint: srv.URL + "/v1"}, "", []byte("{}"), &retry, &Usage{})
	if !errors.Is(err, errNoAnswerYet) || failureReason(err) != "timeout" || strings.Contains(err.Error(), "connection failed") {
		t.Fatalf("a slow provider must be reported as a time-out, got %v", err)
	}
}
