//go:build !windows

package acp

import (
	"os"
	"syscall"
	"testing"
)

// An agent starts programs of its own, and ending the agent alone would leave them running.
// The agent therefore leads a process group, and the group is what ends.
func TestAnAgentLeadsItsOwnProcessGroup(t *testing.T) {
	held := openFakeAgent(t, scriptWaits)
	child, open, err := held.start(RunLogHooks())
	if err != nil {
		t.Fatalf("the agent did not start: %v", err)
	}
	defer closeChild(child)
	_ = open

	group, err := syscall.Getpgid(child.Process.Pid)
	if err != nil {
		t.Fatalf("the agent has no group: %v", err)
	}
	if group != child.Process.Pid {
		t.Errorf("the agent is in group %d and leads none", group)
	}
	// The group of masume is another one, so ending the agent never ends the client.
	mine, err := syscall.Getpgid(os.Getpid())
	if err != nil {
		t.Fatalf("masume has no group: %v", err)
	}
	if group == mine {
		t.Error("the agent shares the group of masume")
	}
}
