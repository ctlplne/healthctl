package clinical

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// commandFields lists the fields each action uses. A command that sets any
// other field is refused, so it can't carry data its action would ignore.
// "action" and "forRevision" belong to every command.
var commandFields = map[string][]string{
	"answer":              {"text", "urgentNow", "date"},
	"correct_answer":      {"text", "answerId", "urgentNow"},
	"correct_fact":        {"factId", "text", "concept", "status", "subject", "timeContext"},
	"correct_intake":      {"age", "pregnancyRelated"},
	"report_emergency":    {"urgentNow"},
	"visual_observation":  {"category", "text"},
	"safety_check":        {"category", "status"},
	"red_flag":            {"category", "status"},
	"set_episode":         {"episodeTitle", "sourceEncounterId"},
	"episode_log":         {"text", "onset", "end", "approximate", "reviewIds"},
	"witness":             {"text", "speaker", "confirmedDestination", "reviewIds"},
	"medication_use":      {"text", "reviewIds", "prescribed", "date", "confirmedDestination"},
	"resolve_sources":     {"text", "reviewIds", "date", "confirmedDestination"},
	"clinician_feedback":  {"text", "speaker", "category", "confirmedDestination"},
	"review_report_row":   {"documentId", "rowId", "row"},
	"withdraw_report_row": {"documentId", "rowId"},
	"authorize_report":    {"documentId", "rowIds"},
	"report_identity":     {"documentId", "belongsTo"},
	"add_report_passage":  {"documentId", "text", "locator"},
	"add_outcome":         {"text", "date", "source", "category", "documentId", "rowId", "note"},
	"reuse_fact":          {"sourceEncounterId", "factId", "date", "text", "category", "reconciliationStatus"},
	"review":              {},
	"confirm":             {},
	"assess":              {"stabilityCheck"},
	"retry":               {},
	"focus_interview":     {},
	"continue":            {},
	"change_provider":     {"provider", "confirmedDestination"},
}

var envelopeFields = []string{"action", "forRevision"}

// setFields lists the JSON names of the command fields that are not empty.
func (c Command) setFields() []string {
	var out []string
	v := reflect.ValueOf(c)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		name := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if name == "" || name == "-" || v.Field(i).IsZero() {
			continue
		}
		out = append(out, name)
	}
	return out
}

// CheckCommand refuses an unknown action or a field the action doesn't use.
func CheckCommand(c Command) error {
	allowed, ok := commandFields[c.Action]
	if !ok {
		return fmt.Errorf("Unknown encounter action")
	}
	for _, f := range c.setFields() {
		if !slices.Contains(allowed, f) && !slices.Contains(envelopeFields, f) {
			return fmt.Errorf("The %s action doesn't use %s", strings.ReplaceAll(c.Action, "_", " "), f)
		}
	}
	return nil
}
