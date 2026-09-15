package cfg

import (
	"os/exec"
	"strings"
)

// The agents of `[ai.agents]`, as the palette and the chat read them.

// FindsAgentCommand is true where the command of this agent is on the PATH.
func FindsAgentCommand(settings AiAgentSettings) bool {
	if strings.TrimSpace(settings.Command) == "" {
		return false
	}
	_, err := exec.LookPath(settings.Command)
	return err == nil
}
