// Package sandbox runs the local document and speech readers. A reader gets a
// fixed, minimal environment, so no installation key, database password, API
// key or URL can reach it, and on Linux it runs in its own network namespace
// when the system allows that, so it has no network at all.
package sandbox

import (
	"context"
	"os/exec"
	"sync"
)

// Environment is the complete environment of every reader.
var Environment = []string{"PATH=/usr/bin:/bin:/usr/local/bin", "LC_ALL=C.UTF-8", "OMP_THREAD_LIMIT=1", "HOME=/nonexistent"}

var (
	probe    sync.Once
	isolated bool
)

// Command prepares a reader. It never inherits the caller's environment.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append([]string(nil), Environment...)
	if NetworkIsolated() {
		isolate(cmd)
	}
	return cmd
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
