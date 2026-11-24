package provider

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// apiEffort is the reasoning effort the app sends by task when an OpenAI or
// Anthropic configuration has reasoning effort turned on. An interview turn
// asks one short question; an assessment gets the most thought.
var apiEffort = map[string]string{"interview": "low", "assessment": "high", "probe": "low"}

// outputBudget is the output ceiling a request asks for.
func outputBudget(kind string) int {
	if kind == "openai" || kind == "anthropic" {
		return reasoningOutputTokens
	}
	return maxOutputTokens
}

// streamKind names the event format of a streamed call, or "" for a single
// JSON response. The ChatGPT plan has its own request path.
func streamKind(cfg Config) string {
	if cfg.Kind != "chatgpt" && cfg.streams() {
		return cfg.Kind
	}
	return ""
}

// errNoAnswerYet is a time-out before the provider sent any response, which
// is not a connection failure.
var errNoAnswerYet = errors.New("the provider did not start answering within the time limit; your history is saved. Try again, or choose a lower reasoning effort or a faster model")

// readMessagesStream reads an Anthropic Messages event stream. It keeps only
// the text of a single text block; thinking is never kept. A refusal, a
// truncated answer or an error event fails the call.
func readMessagesStream(r io.Reader, usage *Usage) ([]byte, string, error) {
	reader := &io.LimitedReader{R: r, N: maxPlanStreamBytes + 1}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maxEnvelopeBytes+1)
	var data, text strings.Builder
	var eventType, model, stop string
	var input, output, cacheRead, cacheWrite int64
	started := false
	textBlocks := 0
	blocks := map[int]string{}
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
				if data.Len() > 0 {
					data.WriteByte('\n')
				}
				data.WriteString(value)
				if data.Len() > maxEnvelopeBytes {
					return nil, "", errIncomplete
				}
			}
			continue
		}
		if data.Len() == 0 {
			eventType = ""
			continue
		}
		var ev struct {
			Type    string `json:"type"`
			Index   int    `json:"index"`
			Message struct {
				Model string `json:"model"`
				Usage struct {
					Input      int64 `json:"input_tokens"`
					CacheRead  int64 `json:"cache_read_input_tokens"`
					CacheWrite int64 `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
			ContentBlock struct {
				Type string `json:"type"`
			} `json:"content_block"`
			Delta struct {
				Type       string `json:"type"`
				Text       string `json:"text"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				Output int64 `json:"output_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(data.String()), &ev) != nil || ev.Type == "" || eventType != "" && eventType != ev.Type {
			return nil, "", errIncomplete
		}
		data.Reset()
		eventType = ""
		switch ev.Type {
		case "message_start":
			started = true
			model = ev.Message.Model
			input, cacheRead, cacheWrite = ev.Message.Usage.Input, ev.Message.Usage.CacheRead, ev.Message.Usage.CacheWrite
		case "content_block_start":
			blocks[ev.Index] = ev.ContentBlock.Type
			switch ev.ContentBlock.Type {
			case "text":
				textBlocks++
			case "thinking", "redacted_thinking":
			default:
				return nil, "", errIncomplete
			}
		case "content_block_delta":
			if blocks[ev.Index] == "text" && ev.Delta.Type == "text_delta" {
				if text.Len()+len(ev.Delta.Text) > maxProposalBytes {
					return nil, "", errIncomplete
				}
				text.WriteString(ev.Delta.Text)
			}
		case "message_delta":
			if ev.Delta.StopReason != "" {
				stop = ev.Delta.StopReason
			}
			output = ev.Usage.Output
		case "message_stop":
			if started {
				usage.add("anthropic", fmt.Appendf(nil, `{"usage":{"input_tokens":%d,"output_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d}}`, input, output, cacheRead, cacheWrite))
			}
			if !started || stop != "end_turn" || textBlocks != 1 || model != "" && !validModel(model) {
				return nil, "", errIncomplete
			}
			return []byte(text.String()), model, nil
		case "error":
			return nil, "", errIncomplete
		}
	}
	return nil, "", errIncomplete
}
