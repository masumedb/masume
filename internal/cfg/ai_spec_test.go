package cfg_test

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// readAiConfig reads the chat settings from a config file.
func readAiConfig(t *testing.T, body string) cfg.AiConfig {
	t.Helper()
	document, err := cfg.DecodeDocument(body)
	if err != nil {
		t.Fatalf("the config text does not read: %v", err)
	}
	return cfg.ParseAiConfig(document)
}

// An unknown provider is reported. Without the report the chat starts with the default
// provider and the user does not see the spelling error.
func TestParseAiConfigReportsAProviderItDoesNotHave(t *testing.T) {
	for _, held := range []struct {
		name string
		body string
		says string
	}{
		{
			"the provider that starts active",
			"[ai]\ndefault_provider = \"anthropik\"\n",
			"anthropik",
		},
		{
			"a table of settings",
			"[ai.providers.anthropik]\nmodel = \"a-model\"\n",
			"anthropik",
		},
	} {
		t.Run(held.name, func(t *testing.T) {
			config := readAiConfig(t, held.body)
			if len(config.Problems) != 1 {
				t.Fatalf("the read reports %v, wanted one problem", config.Problems)
			}
			if !strings.Contains(config.Problems[0], held.says) {
				t.Errorf("the problem reads %q, wanted the name it does not have",
					config.Problems[0])
			}
		})
	}
}

// A known provider is read and reports no problem.
func TestParseAiConfigReadsAProviderItHas(t *testing.T) {
	config := readAiConfig(t,
		"[ai]\ndefault_provider = \"openai\"\n[ai.providers.openai]\nmodel = \"a-model\"\n")
	if len(config.Problems) != 0 {
		t.Errorf("the read reports %v, wanted nothing", config.Problems)
	}
	if config.DefaultProvider != cfg.ProviderOpenai {
		t.Errorf("the provider reads %q, wanted openai", config.DefaultProvider)
	}
	if config.Providers[cfg.ProviderOpenai].Model != "a-model" {
		t.Errorf("the model reads %q", config.Providers[cfg.ProviderOpenai].Model)
	}
}

// The compatible provider reaches a server of the user, so its address and its step limit
// are read from the config file.
func TestParseAiConfigReadsTheCompatibleProvider(t *testing.T) {
	config := readAiConfig(t, "[ai]\ndefault_provider = \"openai_compatible\"\n"+
		"[ai.providers.openai_compatible]\nmodel = \"qwen3-coder\"\n"+
		"base_url = \"http://localhost:11434\"\nmax_tool_steps = 8\n")
	if len(config.Problems) != 0 {
		t.Errorf("the read reports %v, wanted nothing", config.Problems)
	}
	if config.DefaultProvider != cfg.ProviderOpenaiCompatible {
		t.Errorf("the provider reads %q", config.DefaultProvider)
	}
	settings := config.Providers[cfg.ProviderOpenaiCompatible]
	if settings.Model != "qwen3-coder" || settings.BaseURL != "http://localhost:11434" {
		t.Errorf("the settings read %+v", settings)
	}
	if settings.MaxToolSteps != 8 {
		t.Errorf("the step limit reads %d, wanted 8", settings.MaxToolSteps)
	}
}

// A provider without its own step limit uses the default.
func TestParseAiConfigKeepsTheDefaultStepLimit(t *testing.T) {
	config := readAiConfig(t, "[ai.providers.openai]\nmodel = \"a-model\"\n")
	if held := config.Providers[cfg.ProviderOpenai].MaxToolSteps; held != cfg.DefaultMaxToolSteps {
		t.Errorf("the step limit reads %d, wanted %d", held, cfg.DefaultMaxToolSteps)
	}
}

// An agent of `[ai.agents]` is the command masume runs and the arguments it passes.
func TestParseAiConfigReadsAnAgent(t *testing.T) {
	config := readAiConfig(t, "[ai]\ndefault_agent = \"claude\"\n"+
		"[ai.agents.claude]\ncommand = \"npx\"\n"+
		"args = [\"@zed-industries/claude-code-acp\"]\nenv = [\"NO_COLOR=1\"]\n")
	if len(config.Problems) != 0 {
		t.Errorf("the read reports %v, wanted nothing", config.Problems)
	}
	if config.DefaultAgent != "claude" {
		t.Errorf("the agent reads %q", config.DefaultAgent)
	}

	settings := config.Agents["claude"]
	if settings.Name != "claude" || settings.Command != "npx" {
		t.Errorf("the agent reads %+v", settings)
	}
	if len(settings.Args) != 1 || settings.Args[0] != "@zed-industries/claude-code-acp" {
		t.Errorf("the arguments read %v", settings.Args)
	}
	if len(settings.Env) != 1 || settings.Env[0] != "NO_COLOR=1" {
		t.Errorf("the environment reads %v", settings.Env)
	}
}

// An agent masume does not know and that names no command cannot be started, so it is
// reported and left out.
func TestParseAiConfigReportsAnAgentWithNoCommand(t *testing.T) {
	config := readAiConfig(t, "[ai.agents.bespoke]\nargs = [\"acp\"]\n")
	if len(config.Problems) != 1 || !strings.Contains(config.Problems[0], "ai.agents.bespoke") {
		t.Fatalf("the read reports %v", config.Problems)
	}
	if _, held := config.Agents["bespoke"]; held {
		t.Errorf("the agent was kept: %v", config.Agents)
	}
}

// A default agent that is neither in the file nor one masume knows is reported, and the chat
// sends to the provider.
func TestParseAiConfigReportsADefaultAgentItHasNot(t *testing.T) {
	config := readAiConfig(t, "[ai]\ndefault_agent = \"bespoke\"\n")
	if len(config.Problems) != 1 || !strings.Contains(config.Problems[0], "bespoke") {
		t.Fatalf("the read reports %v", config.Problems)
	}
	if config.DefaultAgent != "" {
		t.Errorf("the agent reads %q", config.DefaultAgent)
	}
}

// masume knows how to start the common agents, so a file that names none still offers them.
func TestParseAiConfigCarriesTheAgentsItKnows(t *testing.T) {
	config := readAiConfig(t, "[ai]\nenabled = true\n")
	for _, name := range []string{"claude", "codex", "opencode"} {
		settings, held := config.Agents[name]
		if !held {
			t.Errorf("masume does not know %s", name)
			continue
		}
		if settings.Command == "" || settings.Name != name {
			t.Errorf("%s reads %+v", name, settings)
		}
	}
	// One of them can be chosen without a table of its own.
	if held := readAiConfig(t, "[ai]\ndefault_agent = \"opencode\"\n"); held.DefaultAgent != "opencode" {
		t.Errorf("the agent reads %q, and the read reports %v", held.DefaultAgent, held.Problems)
	}
}

// A table of an agent masume knows changes that agent and keeps what it does not set.
func TestParseAiConfigChangesAnAgentItKnows(t *testing.T) {
	config := readAiConfig(t, "[ai.agents.opencode]\nmodel = \"opencode-go/deepseek-v4-flash\"\n")
	settings := config.Agents["opencode"]
	if settings.Model != "opencode-go/deepseek-v4-flash" {
		t.Errorf("the model reads %q", settings.Model)
	}
	if settings.Command != "opencode" || len(settings.Args) != 1 || settings.Args[0] != "acp" {
		t.Errorf("the command reads %q %v", settings.Command, settings.Args)
	}

	// A command of the file replaces the arguments masume knows, which belong to the old
	// command.
	held := readAiConfig(t, "[ai.agents.opencode]\ncommand = \"/opt/opencode\"\n")
	if written := held.Agents["opencode"]; written.Command != "/opt/opencode" ||
		len(written.Args) != 0 {
		t.Errorf("the command reads %q %v", written.Command, written.Args)
	}
}
