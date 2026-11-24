// Package sandbox runs the local document and speech readers. A reader gets a
// fixed, minimal environment, so no installation key, database password, API
// key or URL can reach it, and on Linux it runs in its own network namespace
// when the system allows that, so it has no network at all.
package sandbox

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// Limits bound one reader run: wall-clock time, CPU seconds and address
// space. The container's own memory limit still applies on top.
type Limits struct {
	Time       time.Duration
	CPUSeconds int
	MemoryMB   int
}

// Reader limits for document text recognition and speech transcription.
var (
	DocumentLimits = Limits{Time: 60 * time.Second, CPUSeconds: 60, MemoryMB: 2048}
	SpeechLimits   = Limits{Time: 180 * time.Second, CPUSeconds: 300, MemoryMB: 3072}
)

// Environment is the complete environment of every reader.
var Environment = []string{"PATH=/usr/bin:/bin:/usr/local/bin", "LC_ALL=C.UTF-8", "OMP_THREAD_LIMIT=1", "HOME=/nonexistent"}

var (
	probe    sync.Once
	isolated bool
)

// Command prepares a reader. It never inherits the caller's environment, it
// is stopped after the time limit, and a shell applies the CPU and memory
// limits before the reader starts. Call the returned function when done.
func Command(ctx context.Context, limits Limits, name string, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, limits.Time)
	cmd := exec.CommandContext(ctx, name, args...)
	if _, err := os.Stat("/bin/sh"); err == nil && (limits.CPUSeconds > 0 || limits.MemoryMB > 0) {
		script := ""
		if limits.CPUSeconds > 0 {
			script += "ulimit -t " + strconv.Itoa(limits.CPUSeconds) + " && "
		}
		if limits.MemoryMB > 0 {
			script += "{ ulimit -v " + strconv.Itoa(limits.MemoryMB*1024) + " 2>/dev/null || true; } && "
		}
		// The shell adds its own variables, so env -i hands the reader exactly Environment.
		launch := `exec "$0" "$@"`
		if _, err := os.Stat("/usr/bin/env"); err == nil {
			launch = "exec /usr/bin/env -i"
			for _, v := range Environment {
				launch += " '" + v + "'"
			}
			launch += ` "$0" "$@"`
		}
		cmd = exec.CommandContext(ctx, "/bin/sh", append([]string{"-c", script + launch, name}, args...)...)
	}
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = append([]string(nil), Environment...)
	if NetworkIsolated() {
		isolate(cmd)
	}
	return cmd, cancel
}

// NetworkIsolated reports whether readers run without network access. It is
// checked once by starting a trivial process in a new network namespace.
func NetworkIsolated() bool {
	probe.Do(func() { isolated = canIsolate() })
	return isolated
}

// Status describes reader isolation for the owner.
func Status() map[string]any {
	network := "not isolated: this system does not allow a separate network namespace, so readers share the app's network"
	if NetworkIsolated() {
		network = "isolated: readers run in their own network namespace with no network"
	}
	return map[string]any{"environment": "minimal: no keys, passwords or URLs", "network": network, "networkIsolated": NetworkIsolated()}
}
