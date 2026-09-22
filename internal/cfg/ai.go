package cfg

import (
	"strings"
	"time"

	"github.com/masumedb/masume/internal/core"
)

// AiProviderID is the name of one provider the AI chat can use. Each one has its own SDK.
type AiProviderID string

// The providers the AI chat can use.
const (
	ProviderAnthropic AiProviderID = "anthropic"
	ProviderOpenai    AiProviderID = "openai"
	ProviderGrok      AiProviderID = "grok"
	// ProviderOpenaiCompatible is any server with the OpenAI chat completions endpoint,
	// such as Ollama, LM Studio, llama.cpp or vLLM.
	ProviderOpenaiCompatible AiProviderID = "openai_compatible"
)

// AiProviderIDs lists the providers a config file can use.
var AiProviderIDs = []AiProviderID{
	ProviderAnthropic, ProviderOpenai, ProviderGrok, ProviderOpenaiCompatible,
}

// describeAiProviderIDs returns the supported provider names.
func describeAiProviderIDs() string {
	written := make([]string, 0, len(AiProviderIDs))
	for _, id := range AiProviderIDs {
		written = append(written, string(id))
	}
	last := len(written) - 1
	return "The providers are " + strings.Join(written[:last], ", ") + " and " +
		written[last] + "."
}

// AiProviderSettings is the model, API key, and provider address configuration.
type AiProviderSettings struct {
	Model      string
	APIKey     string
	APIKeyEnv  string
	BaseURL    string
	BaseURLEnv string
	// MaxToolSteps is the maximum number of model responses per question.
	MaxToolSteps int
}

// AiAgentSettings is one coding agent masume reaches over ACP. The agent runs as a child
// process and reads the protocol on its standard input and output.
type AiAgentSettings struct {
	// Name is the name of the agent under `[ai.agents]`.
	Name    string
	Command string
	Args    []string
	// Model is the model of the agent, chosen from the list the agent offers. An empty
	// name keeps the model the agent is configured with.
	Model string
	// Env is the extra environment of the child process, as NAME=VALUE entries.
	Env []string
}

// AiConfig is the configuration under `[ai]`.
type AiConfig struct {
	// Enabled is the switch for the AI chat, its actions, and its interface elements.
	Enabled         bool
	DefaultProvider AiProviderID
	Providers       map[AiProviderID]AiProviderSettings
	// DefaultAgent is the agent the chat sends to. An empty name sends to DefaultProvider.
	DefaultAgent string
	Agents       map[string]AiAgentSettings
	// The time one AI chat statement can run before it is cancelled.
	StatementTimeout time.Duration
	// Unsupported provider names under `[ai]`.
	Problems []string
}

// DefaultAiStatementTimeout is the default time limit for AI chat statements.
const DefaultAiStatementTimeout = 30 * time.Second

// DefaultMaxToolSteps is the default number of model responses per question.
const DefaultMaxToolSteps = 25

// DefaultAiConfig returns the default AI chat settings.
func DefaultAiConfig() AiConfig {
	return AiConfig{
		Enabled:          true,
		DefaultProvider:  ProviderAnthropic,
		StatementTimeout: DefaultAiStatementTimeout,
		// The agents and the models of the providers are tables of the config file.
		Agents:    map[string]AiAgentSettings{},
		Providers: map[AiProviderID]AiProviderSettings{},
	}
}

// parseProviderSettings reads the table of one provider and applies it over the default.
func parseProviderSettings(table Table, fallback AiProviderSettings) AiProviderSettings {
	if table == nil {
		return fallback
	}
	settings := fallback
	if written, present := FindString(table, "model"); present {
		settings.Model = written
	}
	if written, present := FindString(table, "api_key"); present {
		settings.APIKey = written
	}
	if written, present := FindString(table, "api_key_env"); present {
		settings.APIKeyEnv = written
	}
	if written, present := FindString(table, "base_url"); present {
		settings.BaseURL = written
	}
	if written, present := FindString(table, "base_url_env"); present {
		settings.BaseURLEnv = written
	}
	if steps, present := FindPositiveInteger(table, "max_tool_steps"); present {
		settings.MaxToolSteps = steps
	}
	return settings
}

// ParseAiConfig reads `[ai]`. An invalid setting uses the default.
func ParseAiConfig(document Table) AiConfig {
	config := DefaultAiConfig()
	ai, present := FindSection(document, "ai")
	if !present {
		return config
	}

	if enabled, named := FindBool(ai, "enabled"); named {
		config.Enabled = enabled
	}

	providerTables, _ := FindTable(ai["providers"])
	for _, id := range AiProviderIDs {
		table, held := FindTable(providerTables[string(id)])
		if !held {
			continue
		}
		// A provider the file names starts from the step limit of the client, and the
		// table sets the rest.
		config.Providers[id] = parseProviderSettings(
			table, AiProviderSettings{MaxToolSteps: DefaultMaxToolSteps})
	}
	for _, name := range sortedKeys(providerTables) {
		if _, known := core.FindAllowed(AiProviderIDs, name); !known {
			config.Problems = append(config.Problems,
				"ai.providers: unsupported provider \""+name+"\". Skipping this provider. "+
					describeAiProviderIDs())
		}
	}

	if written, named := FindString(ai, "default_provider"); named {
		id, known := core.FindAllowed(AiProviderIDs, written)
		if known {
			config.DefaultProvider = id
		} else {
			config.Problems = append(config.Problems,
				"ai.default_provider: unsupported provider \""+written+"\". Using "+
					string(config.DefaultProvider)+". "+describeAiProviderIDs())
		}
	}
	if milliseconds, named := FindPositiveInteger(ai, "statement_timeout_ms"); named {
		config.StatementTimeout = time.Duration(milliseconds) * time.Millisecond
	}

	config.Agents, config.Problems = parseAiAgents(ai, config.Problems)
	if written, named := FindString(ai, "default_agent"); named && written != "" {
		if _, held := config.Agents[written]; held {
			config.DefaultAgent = written
		} else {
			config.Problems = append(config.Problems,
				"ai.default_agent: no agent named \""+written+"\" under [ai.agents]. "+
					"Sending to the provider instead.")
		}
	}
	return config
}

// parseAiAgents reads the agents of `[ai.agents]`. Every agent the client can send to is a
// table of the config file, and one with no command is reported and left out.
func parseAiAgents(ai Table, problems []string) (map[string]AiAgentSettings, []string) {
	agents := map[string]AiAgentSettings{}
	tables, _ := FindTable(ai["agents"])
	for _, name := range sortedKeys(tables) {
		table, isTable := FindTable(tables[name])
		if !isTable {
			problems = append(problems,
				"ai.agents."+name+": not a table. Skipping this agent.")
			continue
		}

		settings := AiAgentSettings{Name: name}
		settings.Command, _ = FindString(table, "command")
		if strings.TrimSpace(settings.Command) == "" {
			problems = append(problems,
				"ai.agents."+name+": no command. Skipping this agent. Set command to the "+
					"program that serves ACP on its standard input and output.")
			continue
		}
		settings.Args, _ = FindStringList(table, "args")
		settings.Env, _ = FindStringList(table, "env")
		settings.Model, _ = FindString(table, "model")
		agents[name] = settings
	}
	return agents, problems
}
