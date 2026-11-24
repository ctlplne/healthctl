package provider

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"unicode/utf8"

	"github.com/ctlplne/healthctl/internal/clinical"
	"github.com/ctlplne/healthctl/internal/contract"
)

//go:embed contracts/proposal-v1.schema.json
var proposalSchema json.RawMessage

//go:embed contracts/clinical-v9.txt
var promptText string

// instructions states the contract limits from the same constants the validator uses.
var instructions = promptText + "\n" + contract.LimitsText() + "\n" + clinical.IntentText()

//go:embed contracts/repair-v1.txt
var repairInstructions string

var probeSchema = json.RawMessage(`{"type":"object","properties":{"status":{"type":"string","enum":["ready"]}},"required":["status"],"additionalProperties":false}`)

type schemaNode struct {
	Type       string                 `json:"type"`
	Properties map[string]*schemaNode `json:"properties"`
	Required   []string               `json:"required"`
	Items      *schemaNode            `json:"items"`
	Enum       []string               `json:"enum"`
}

// The embedded contract uses only objects, arrays, strings and enums. Parse via
// tokens so that duplicate, unknown, missing, case-mismatched and null fields
// are rejected before the typed domain validator sees a candidate.
func validateJSON(raw []byte, schema *schemaNode) error {
	if !utf8.Valid(raw) || len(raw) > maxProposalBytes {
		return errInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := schema.consume(d); err != nil {
		return errInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return errInvalid
	}
	return nil
}

var errInvalid = errors.New("provider output did not satisfy the requested contract; retry the model call or review the history")

func (s *schemaNode) consume(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return errInvalid
	}
	switch s.Type {
	case "object":
		if token != json.Delim('{') {
			return errInvalid
		}
		seen := make(map[string]bool, len(s.Required))
		for d.More() {
			key, err := d.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[name] || s.Properties[name] == nil {
				return errInvalid
			}
			seen[name] = true
			if err := s.Properties[name].consume(d); err != nil {
				return err
			}
		}
		for _, name := range s.Required {
			if !seen[name] {
				return errInvalid
			}
		}
		token, err = d.Token()
		if err != nil || token != json.Delim('}') {
			return errInvalid
		}
	case "array":
		if token != json.Delim('[') {
			return errInvalid
		}
		for d.More() {
			if err := s.Items.consume(d); err != nil {
				return err
			}
		}
		token, err = d.Token()
		if err != nil || token != json.Delim(']') {
			return errInvalid
		}
	case "string":
		value, ok := token.(string)
		if !ok || (len(s.Enum) > 0 && !slices.Contains(s.Enum, value)) {
			return errInvalid
		}
	default:
		return errInvalid
	}
	return nil
}

// taskSchema narrows the shared proposal schema to the actions a task allows,
// so an interview can't express a differential and an assessment can't ask.
// Unasked concepts belong in outstanding, so not_asked is not a finding status.
func taskSchema(mode string) json.RawMessage {
	var schema map[string]any
	if json.Unmarshal(proposalSchema, &schema) != nil {
		return proposalSchema
	}
	props := schema["properties"].(map[string]any)
	actions := []string{"ask", "review", "insufficient"}
	if mode == "assessment" {
		actions = []string{"differential", "insufficient"}
	}
	props["action"].(map[string]any)["enum"] = actions
	finding := props["findings"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	finding["status"].(map[string]any)["enum"] = []string{"reported_present", "explicitly_denied", "unknown", "uncertain", "declined"}
	out, err := json.Marshal(schema)
	if err != nil {
		return proposalSchema
	}
	return out
}
