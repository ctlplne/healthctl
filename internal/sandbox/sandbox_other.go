//go:build !linux

package sandbox

import "os/exec"

func isolate(*exec.Cmd) {}

func canIsolate() bool { return false }
