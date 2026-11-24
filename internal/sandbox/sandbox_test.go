package sandbox

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// TestReaderEnvironment checks that a reader never sees the app's secrets.
func TestReaderEnvironment(t *testing.T) {
	for _, secret := range []string{"CLINICAL_DATA_KEY", "PGPASSWORD", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "BYOM_API_KEY", "DATABASE_URL"} {
		t.Setenv(secret, "synthetic-secret-value")
	}
	if _, err := exec.LookPath("env"); err != nil {
		t.Skip("no env command")
	}
	out, err := Command(context.Background(), "env").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "synthetic-secret-value") {
		t.Fatalf("a reader saw a secret:\n%s", out)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		allowed := false
		for _, v := range Environment {
			allowed = allowed || line == v
		}
		if !allowed {
			t.Fatalf("unexpected reader environment entry %q", line)
		}
	}
	status := Status()
	if status["networkIsolated"] != NetworkIsolated() || status["network"] == "" {
		t.Fatalf("the owner must be told the real isolation: %v", status)
	}
}
