//go:build !windows

package proc

import (
	"os/exec"
	"syscall"
)

// LeadGroup makes the child the leader of a new process group. It runs before Start.
func LeadGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// StopGroup sends SIGTERM to the process group of the child.
func StopGroup(command *exec.Cmd) error {
	return syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
}

// KillGroup sends SIGKILL to the process group of the child.
func KillGroup(command *exec.Cmd) error {
	return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
}
