package clinical

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPayloadMigration(t *testing.T) {
	e := intake(t, "I have a cough.", false)
	if e.PayloadVersion != PayloadVersion {
		t.Fatalf("a new encounter must carry the current payload version, got %d", e.PayloadVersion)
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	// The sub-records are promoted, so the stored JSON keeps its flat shape.
	var flat map[string]json.RawMessage
	if err := json.Unmarshal(b, &flat); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"answers", "warnings", "state", "assessments", "documents", "payloadVersion"} {
		if _, ok := flat[key]; !ok && key != "documents" {
			t.Fatalf("stored payload lost top-level %q", key)
		}
	}
	for key := range flat {
		if strings.HasSuffix(key, "Record") || strings.HasSuffix(key, "State") || key == "EncounterIdentity" {
			t.Fatalf("a sub-record leaked into the payload as %q", key)
		}
	}
	back, err := DecodeEncounter(b)
	if err != nil || !reflect.DeepEqual(*back, *e) {
		t.Fatalf("a current payload must round-trip unchanged: %v", err)
	}
	// An original payload (no version) is migrated to the current version.
	delete(flat, "payloadVersion")
	old, _ := json.Marshal(flat)
	migrated, err := DecodeEncounter(old)
	if err != nil || migrated.PayloadVersion != PayloadVersion {
		t.Fatalf("an original payload was not migrated: %v %d", err, migrated.PayloadVersion)
	}
	flat["payloadVersion"] = json.RawMessage("99")
	future, _ := json.Marshal(flat)
	if _, err := DecodeEncounter(future); err == nil {
		t.Fatal("a payload from a newer version must be refused, not misread")
	}
}

func TestStoredWordingMigrated(t *testing.T) {
	old := Encounter{InterviewState: InterviewState{Question: "Did the patient notice it at night?", Choices: []string{}, Outstanding: []string{"The patient's sleep"}},
		HistoryRecord: HistoryRecord{Answers: []Answer{{ID: "a1", Question: "What does the patient report?", Text: "The patient next door was loud.", Active: true}}}}
	m := migrateV1(t, old)
	if m.Question != "Did the user notice it at night?" || m.Outstanding[0] != "The user's sleep" || m.Answers[0].Question != "What does the user report?" || m.Answers[0].Text != "The patient next door was loud." {
		t.Fatalf("stored wording was not migrated, or evidence changed: %+v", m)
	}
	// Display no longer rewrites: a current payload is shown exactly as stored.
	current := Encounter{PayloadVersion: PayloadVersion, InterviewState: InterviewState{Question: "Is the patient portal working?"}}
	if current.ForDisplay().Question != "Is the patient portal working?" || current.Report().Question != "Is the patient portal working?" {
		t.Fatal("display rewrote stored text")
	}
	// Migration runs once: reading a migrated payload changes nothing.
	b, _ := json.Marshal(m)
	again, err := DecodeEncounter(b)
	if err != nil || !reflect.DeepEqual(*again, m) {
		t.Fatal("reading a migrated payload changed it")
	}
}
