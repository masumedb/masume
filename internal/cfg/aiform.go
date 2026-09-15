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

// ServesMcpProfile is true where the MCP server opens this connection to an outside agent.
func ServesMcpProfile(config McpConfig, name string) bool {
	for _, held := range config.Profiles {
		if held == name {
			return true
		}
	}
	return false
}

// resolveConfiguredSteps returns the step limit of a provider, with the default for one that
// has no limit of its own.
func resolveConfiguredSteps(settings AiProviderSettings) int {
	if settings.MaxToolSteps > 0 {
		return settings.MaxToolSteps
	}
	return DefaultMaxToolSteps
}

// copyAiConfig returns settings with maps of their own, so a change that is refused leaves
// the settings it was made against as they were.
func copyAiConfig(config AiConfig) AiConfig {
	built := config
	built.Providers = map[AiProviderID]AiProviderSettings{}
	for id, settings := range config.Providers {
		built.Providers[id] = settings
	}
	built.Agents = map[string]AiAgentSettings{}
	for name, settings := range config.Agents {
		built.Agents[name] = settings
	}
	return built
}
