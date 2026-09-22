package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/mcp"
)

// buildAgentSettings returns one agent, as a config file with `[ai.agents]` has.
func buildAgentSettings() cfg.AiAgentSettings {
	return cfg.AiAgentSettings{
		Name: "claude", Command: "npx", Args: []string{"claude-code-acp"},
	}
}

// useChatAgent gives the client one agent and sends the chat to it.
func useChatAgent(model *Model) {
	model.ai.Agents = map[string]cfg.AiAgentSettings{"claude": buildAgentSettings()}
	model.aiAgent = "claude"
}

// The panel names the agent the chat sends to, not the provider it would otherwise use.
func TestTheChatNamesTheAgentItSendsTo(t *testing.T) {
	model := buildOfflineModel(t, 100, 40)
	if held := model.describeChatSource(); held != "anthropic/claude-opus-5" {
		t.Errorf("the chat names %q", held)
	}

	useChatAgent(model)
	if held := model.describeChatSource(); held != "agent/claude" {
		t.Errorf("the chat names %q", held)
	}
}

// The AI chat of the client is the user acting on a connection they opened, so the `[mcp]`
// allowlist does not reach it. That list governs the agents that reach masume from outside.
func TestTheChatDoesNotReadTheMcpAllowlist(t *testing.T) {
	model := buildOfflineModel(t, 100, 40)
	useChatAgent(model)
	// The client lists no profile for MCP at all, as a config file without `[mcp]` does.
	model.mcp = cfg.DefaultMcpConfig()

	if held := model.describeChatProblem(cfg.Profile{Name: "supabase"}); held != "" {
		t.Errorf("the chat reports %q", held)
	}
}

// An agent with no command cannot be started, and the panel says what to set.
func TestAnAgentWithNoCommandIsReported(t *testing.T) {
	held := describeAgentProblem(cfg.AiAgentSettings{Name: "bespoke"})
	if !strings.Contains(held, "no command") ||
		!strings.Contains(held, "[ai.agents.bespoke]") {
		t.Errorf("the problem reads %q", held)
	}
	if held := describeAgentProblem(buildAgentSettings()); held != "" {
		t.Errorf("an agent with a command reports %q", held)
	}
}

// The agent is handed the tools of the chat over the loopback address, so it reads the
// connection the chat is open on rather than opening one of its own.
func TestTheAgentIsHandedTheToolsOfTheChat(t *testing.T) {
	model := buildOfflineModel(t, 100, 40)
	useChatAgent(model)
	connection := model.Active()
	run, events := connection.Chat.Begin(func() {})
	deps := model.buildChatToolDeps(connection, run, events)

	tools := mcp.BuildTools(mcp.BindConnection(deps))
	held, closeSource, err := model.openChatResponder(connection, "masume/offline", tools)
	if err != nil {
		t.Fatalf("the chat opened no source: %v", err)
	}
	defer closeSource()
	if held.Describe() != "agent/claude" {
		t.Errorf("the chat sends to %q", held.Describe())
	}
}

// The tool loop of masume has a step limit; an agent runs a loop of its own and has none
// here.
func TestAnAgentCarriesNoStepLimitOfMasume(t *testing.T) {
	model := buildOfflineModel(t, 100, 40)
	if held := model.resolveChatSteps(); held != cfg.DefaultMaxToolSteps {
		t.Errorf("the provider allows %d steps", held)
	}

	useChatAgent(model)
	if held := model.resolveChatSteps(); held != 0 {
		t.Errorf("the agent allows %d steps of masume", held)
	}
}

// The palette offers one row per agent, and choosing one sends the chat to it.
func TestThePaletteOffersEveryAgent(t *testing.T) {
	model := buildOfflineModel(t, 100, 40)
	model.ai.Agents = map[string]cfg.AiAgentSettings{"claude": buildAgentSettings()}

	connection := model.Active()
	rows := model.buildPaletteActions(connection)
	found := false
	for _, row := range rows {
		if row.ID != aiAgentPrefix+"claude" {
			continue
		}
		found = true
		if row.Detail != "npx claude-code-acp" {
			t.Errorf("the row reads %q", row.Detail)
		}
	}
	if !found {
		t.Fatal("the palette offers no agent")
	}

	model.switchAiAgent(connection, "claude")
	if model.aiAgent != "claude" {
		t.Errorf("the chat sends to %q", model.aiAgent)
	}

	// A provider chosen after an agent sends to the provider again.
	model.switchAiProvider(connection, string(cfg.ProviderOpenai))
	if model.aiAgent != "" || model.aiProvider != cfg.ProviderOpenai {
		t.Errorf("the chat sends to agent %q and provider %q",
			model.aiAgent, model.aiProvider)
	}
}

// An agent whose command is not installed says so before the first question, rather than
// failing when one is asked.
func TestAnAgentThatIsNotInstalledIsReported(t *testing.T) {
	held := describeAgentProblem(cfg.AiAgentSettings{
		Name: "claude", Command: "masume-no-such-command",
	})
	for _, wanted := range []string{
		"claude is not installed", "masume-no-such-command is not on the PATH",
		"in the settings",
	} {
		if !strings.Contains(held, wanted) {
			t.Errorf("the problem does not hold %q: %q", wanted, held)
		}
	}

	// A command that is there reports nothing, so the chat asks its question.
	if held := describeAgentProblem(cfg.AiAgentSettings{
		Name: "shell", Command: "sh",
	}); held != "" {
		t.Errorf("an installed command reports %q", held)
	}
}
