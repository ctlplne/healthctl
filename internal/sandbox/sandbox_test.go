package sandbox

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestReaderEnvironment checks that a reader never sees the app's secrets.
func TestReaderEnvironment(t *testing.T) {
	for _, secret := range []string{"CLINICAL_DATA_KEY", "PGPASSWORD", "OPENAI_API_KEY", "ANTHROPIC_API_KEY", "BYOM_API_KEY", "DATABASE_URL"} {
		t.Setenv(secret, "synthetic-secret-value")
	}
	if _, err := exec.LookPath("env"); err != nil {
		t.Skip("no env command")
	}
	cmd, cancel := Command(context.Background(), DocumentLimits, "env")
	defer cancel()
	out, err := cmd.Output()
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

func TestReaderLimits(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("no sleep command")
	}
	start := time.Now()
	cmd, cancel := Command(context.Background(), Limits{Time: 200 * time.Millisecond}, "sleep", "5")
	defer cancel()
	if err := cmd.Run(); err == nil || time.Since(start) > 4*time.Second {
		t.Fatalf("the time limit did not stop the reader (%v after %s)", err, time.Since(start))
	}
	busy, stop := Command(context.Background(), Limits{Time: 20 * time.Second, CPUSeconds: 1}, "/bin/sh", "-c", "while :; do :; done")
	defer stop()
	start = time.Now()
	if err := busy.Run(); err == nil || time.Since(start) > 15*time.Second {
		t.Fatalf("the CPU limit did not stop the reader (%v after %s)", err, time.Since(start))
	}
}
