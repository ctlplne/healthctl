//go:build linux

package sandbox

import (
	"os"
	"os/exec"
	"syscall"
)

// isolate starts the reader in new user and network namespaces. The new
// network namespace has only a down loopback interface.
func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:  syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: os.Getuid(), HostID: os.Getuid(), Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: os.Getgid(), HostID: os.Getgid(), Size: 1}},
	}
}

func canIsolate() bool {
	path, err := exec.LookPath("true")
	if err != nil {
		return false
	}
	cmd := exec.Command(path)
	cmd.Env = Environment
	isolate(cmd)
	return cmd.Run() == nil
}
