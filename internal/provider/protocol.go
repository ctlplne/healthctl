package provider

import (
	"encoding/json"
	"errors"
)

const anthropicVersion = "2026-10-21"

type call struct {
	task            string // interview, assessment, or probe
	reasoningEffort string
	system          string
	input           string
	schema          json.RawMessage
	maxTokens       int
}

func (p call) body(cfg Config) ([]byte, error) {
	system := p.system
	if !cfg.native() {
		system += "\nUse this exact JSON schema:\n" + string(p.schema)
	}
	textContent := func(role, text string) map[string]string {
		return map[string]string{"role": role, "content": text}
	}
	body := map[string]any{"model": cfg.Model}
	switch cfg.Kind {
	case "openai", "chatgpt":
		body["instructions"] = system
		body["store"] = false
		if cfg.Kind == "chatgpt" {
			body["input"] = []map[string]string{textContent("user", p.input)}
			body["stream"] = true
			if p.reasoningEffort != "" {
				body["reasoning"] = map[string]string{"effort": p.reasoningEffort}
			}
		} else {
			body["input"] = p.input
			body["max_output_tokens"] = p.maxTokens
		}
		if cfg.native() {
			body["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": "clinical_history_result", "strict": true, "schema": p.schema}}
		}
	case "anthropic":
		body["system"] = system
		body["messages"] = []map[string]string{textContent("user", p.input)}
		body["max_tokens"] = p.maxTokens
		if cfg.native() {
			body["output_config"] = map[string]any{"format": map[string]any{"type": "json_schema", "schema": p.schema}}
		}
	case "compatible":
		body["messages"] = []map[string]string{textContent("system", system), textContent("user", p.input)}
		body["max_tokens"] = p.maxTokens
		if cfg.native() {
			body["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "clinical_history_result", "strict": true, "schema": p.schema}}
		}
	default:
		return nil, errors.New("a real provider is required")
	}
	return json.Marshal(body)
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
type wireResponse struct {
	Model  string          `json:"model"`
	Status string          `json:"status"`
	Error  json.RawMessage `json:"error"`
	Output []struct {
		Type    string         `json:"type"`
		Role    string         `json:"role"`
		Status  string         `json:"status"`
		Content []contentBlock `json:"content"`
	} `json:"output"`
	Content    []contentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Choices    []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role         string            `json:"role"`
			Content      string            `json:"content"`
			Refusal      string            `json:"refusal"`
			ToolCalls    []json.RawMessage `json:"tool_calls"`
			FunctionCall json.RawMessage   `json:"function_call"`
		} `json:"message"`
	} `json:"choices"`
}

var errIncomplete = errors.New("provider refused, truncated, or returned an incomplete response; review the history or model settings")
var errEnvelope = errors.New("provider returned an unreadable response")

func unpack(kind string, raw []byte) ([]byte, string, error) {
	var w wireResponse
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, "", errEnvelope
	}
	if len(w.Error) > 0 && string(w.Error) != "null" {
		return nil, "", errIncomplete
	}
	if w.Model != "" && !validModel(w.Model) {
		return nil, "", errEnvelope
	}
	var answer string
	switch kind {
	case "openai":
		if w.Status != "completed" {
			return nil, "", errIncomplete
		}
		var messages int
		for _, item := range w.Output {
			if item.Type == "reasoning" {
				continue
			}
			if item.Type != "message" || item.Role != "assistant" || (item.Status != "" && item.Status != "completed") {
				return nil, "", errIncomplete
			}
			messages++
			if messages > 1 || len(item.Content) != 1 || item.Content[0].Type != "output_text" {
				return nil, "", errIncomplete
			}
			answer = item.Content[0].Text
		}
	case "anthropic":
		if w.StopReason != "end_turn" {
			return nil, "", errIncomplete
		}
		count := 0
		for _, block := range w.Content {
			switch block.Type {
			case "thinking", "redacted_thinking":
				continue
			case "text":
				count++
				if count > 1 {
					return nil, "", errIncomplete
				}
				answer = block.Text
			default:
				return nil, "", errIncomplete
			}
		}
	case "compatible":
		if len(w.Choices) != 1 {
			return nil, "", errIncomplete
		}
		ch := w.Choices[0]
		if ch.FinishReason != "stop" || ch.Message.Role != "assistant" || ch.Message.Refusal != "" || len(ch.Message.ToolCalls) > 0 || (len(ch.Message.FunctionCall) > 0 && string(ch.Message.FunctionCall) != "null") {
			return nil, "", errIncomplete
		}
		answer = ch.Message.Content
	}
	if answer == "" {
		return nil, "", errEnvelope
	}
	if len(answer) > maxProposalBytes {
		return nil, "", errIncomplete
	}
	return []byte(answer), w.Model, nil
}
