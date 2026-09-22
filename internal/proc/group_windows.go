package proc

import (
	"os/exec"
	"strconv"
	"syscall"
)

// newProcessGroup is CREATE_NEW_PROCESS_GROUP. The child and its programs leave the console
// group of masume, so a Ctrl-C in the terminal reaches masume alone.
const newProcessGroup = 0x00000200

// LeadGroup makes the child the leader of a new process group. It runs before Start.
func LeadGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: newProcessGroup}
}

// StopGroup asks the child and its programs to close.
func StopGroup(command *exec.Cmd) error {
	return runTaskkill(command, false)
}

// KillGroup ends the child and its programs.
func KillGroup(command *exec.Cmd) error {
	return runTaskkill(command, true)
}

// runTaskkill ends the process tree of the child. Windows has no signal for a process group,
// and taskkill walks the tree by parent id.
func runTaskkill(command *exec.Cmd, force bool) error {
	arguments := []string{"/T", "/PID", strconv.Itoa(command.Process.Pid)}
	if force {
		arguments = append([]string{"/F"}, arguments...)
	}
	kill := exec.Command("taskkill", arguments...)
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return kill.Run()
}
