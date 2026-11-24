package server

import "testing"

func TestStartupConfig(t *testing.T) {
	for _, ok := range [][2]string{{"", ""}, {"Call 911 for a medical emergency.", "Your local urgent care clinic"}, {"In the UK call 999 or 112.", ""}} {
		if err := ValidateRegion(ok[0], ok[1]); err != nil {
			t.Fatalf("%q rejected: %v", ok[0], err)
		}
	}
	for _, bad := range [][2]string{{"Call your doctor", ""}, {"Call 911\nnow", ""}, {"<b>Call 911</b>", ""}, {"Call 911", "javascript:alert(1)"}} {
		if err := ValidateRegion(bad[0], bad[1]); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
	s := &Server{Locale: "en-IN", Units: "metric", Emergency: "Call 112.", CareDestination: "Nearest emergency department"}
	if got := s.installationSettings(); got["locale"] != "en-IN" || got["emergencyContact"] != "Call 112." || got["units"] != "metric" {
		t.Fatalf("exports must show the active settings: %v", got)
	}
}
