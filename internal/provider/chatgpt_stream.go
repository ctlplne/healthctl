package provider

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

const maxPlanStreamBytes = 2 << 20

// Consume a complete Responses SSE event. No tokens, fragments, reasoning, or
// remote error messages become patient-visible output before validation.
func readPlanStream(r io.Reader, requestID string, budget int, usage *Usage) ([]byte, string, error) {
	return readResponsesStream(r, requestID, budget, usage, "chatgpt")
}

// readResponsesStream reads a Responses API event stream for the ChatGPT plan
// or the OpenAI API. Only the ChatGPT plan has its own error type.
func readResponsesStream(r io.Reader, requestID string, budget int, usage *Usage, kind string) ([]byte, string, error) {
	reader := &io.LimitedReader{R: r, N: maxPlanStreamBytes + 1}
	buffered := bufio.NewReader(reader)
	if prefix, _ := buffered.Peek(3); bytes.Equal(prefix, []byte{0xef, 0xbb, 0xbf}) {
		buffered.Discard(3)
	}
	// Some gateways return an HTML page or JSON error under HTTP 200. Identify
	// these before parsing events and never expose the provider's response text.
	for {
		prefix, err := buffered.Peek(1)
		if err != nil || reader.N == 0 {
			return nil, "", errIncomplete
		}
		switch prefix[0] {
		case ' ', '\t', '\r', '\n':
			buffered.Discard(1)
			continue
		case '<':
			if kind != "chatgpt" {
				return nil, "", errEnvelope
			}
			return nil, "", &ChatGPTError{HTTPStatus: 200, BodyShape: "html", message: "The ChatGPT endpoint returned an HTML page. Check the network connection and retry the test."}
		case '{', '[':
			if kind != "chatgpt" {
				return nil, "", errEnvelope
			}
			var result struct{ Error *planWireError }
			if json.NewDecoder(io.LimitReader(buffered, 16<<10)).Decode(&result) == nil && result.Error != nil {
				return nil, "", planError(200, "error", result.Error.Code, result.Error.Param, requestID)
			}
			return nil, "", &ChatGPTError{HTTPStatus: 200, BodyShape: "json", message: "The ChatGPT endpoint returned JSON without a completed response stream."}
		}
		break
	}
	scanner := bufio.NewScanner(buffered)
	scanner.Buffer(make([]byte, 4096), maxEnvelopeBytes+1)
	var data, delta strings.Builder
	var eventType string
	events := 0
	for scanner.Scan() {
		if reader.N == 0 {
			return nil, "", errIncomplete
		}
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.HasPrefix(line, ":") {
			continue
		}
		if line != "" {
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "event":
				eventType = value
			case "data":
				if len(value)+data.Len()+1 > maxEnvelopeBytes {
					return nil, "", errIncomplete
				}
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(value)
			}
			continue
		}
		if data.Len() == 0 {
			eventType = ""
			continue
		}
		events++
		if events > 4096 {
			return nil, "", errIncomplete
		}
		var event struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
			Item  struct {
				Type string `json:"type"`
			} `json:"item"`
			Response json.RawMessage `json:"response"`
			Error    *planWireError  `json:"error"`
			Code     string          `json:"code"`
			Param    string          `json:"param"`
		}
		if json.Unmarshal([]byte(data.String()), &event) != nil || event.Type == "" || eventType != "" && eventType != event.Type {
			return nil, "", errIncomplete
		}
		data.Reset()
		eventType = ""
		switch event.Type {
		case "response.output_text.delta":
			if len(event.Delta) > maxProposalBytes-delta.Len() {
				return nil, "", errIncomplete
			}
			delta.WriteString(event.Delta)
		case "response.output_item.added", "response.output_item.done":
			if event.Item.Type != "message" && event.Item.Type != "reasoning" {
				return nil, "", errIncomplete
			}
		case "response.refusal.delta", "response.refusal.done", "response.incomplete":
			usage.add(kind, event.Response)
			return nil, "", errIncomplete
		case "error", "response.failed":
			issue := planWireError{Code: event.Code, Param: event.Param}
			if event.Error != nil {
				issue = *event.Error
			}
			var failure struct {
				Error *planWireError `json:"error"`
			}
			if json.Unmarshal(event.Response, &failure) == nil && failure.Error != nil {
				issue = *failure.Error
			}
			usage.add(kind, event.Response)
			if kind != "chatgpt" {
				return nil, "", errIncomplete
			}
			return nil, "", planError(200, event.Type, issue.Code, issue.Param, requestID)
		case "response.completed":
			usage.add(kind, event.Response)
			if !withinPlanAnswerBudget(event.Response, budget) {
				return nil, "", errIncomplete
			}
			out, model, err := unpack("openai", event.Response)
			badModel := kind == "chatgpt" && !validSubscriptionSlug(model) || kind != "chatgpt" && model != "" && !validModel(model)
			if err != nil || badModel || delta.Len() > 0 && delta.String() != string(out) {
				return nil, "", errIncomplete
			}
			return out, model, nil
		}
	}
	// A dropped connection, malformed/unterminated event, [DONE] without a
	// completed response, and a bare delta cannot produce a clinical proposal.
	return nil, "", errIncomplete
}

// Responses counts reasoning in output_tokens. Apply the answer budget to
// the remaining tokens while Usage records the provider's full reported total.
func withinPlanAnswerBudget(raw []byte, budget int) bool {
	var w struct {
		Usage struct {
			Output  int64 `json:"output_tokens"`
			Details struct {
				Reasoning int64 `json:"reasoning_tokens"`
			} `json:"output_tokens_details"`
		} `json:"usage"`
	}
	if json.Unmarshal(raw, &w) != nil {
		return false
	}
	output, reasoning := w.Usage.Output, w.Usage.Details.Reasoning
	return output >= 0 && output <= 1e9 && reasoning >= 0 && reasoning <= output && output-reasoning <= int64(budget)
}
