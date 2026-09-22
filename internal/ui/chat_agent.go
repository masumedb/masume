package ui

import (
	"context"
	"os"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/acp"
	"github.com/masumedb/masume/internal/ai"
	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/mcp"
)

// The chat sends to a provider model or to a coding agent. An agent reaches the database
// through the MCP server of masume, so the profile it asks about must be one that server
// serves.

// mcpServerName is the name the agent knows the MCP server of masume by.
const mcpServerName = "masume"

// findChatAgent returns the agent the chat sends to, and false when it sends to a provider.
func (model *Model) findChatAgent() (cfg.AiAgentSettings, bool) {
	if model.aiAgent == "" {
		return cfg.AiAgentSettings{}, false
	}
	settings, held := model.ai.Agents[model.aiAgent]
	return settings, held
}

// describeChatSource returns the agent or the provider and model the chat sends to.
func (model *Model) describeChatSource() string {
	if settings, held := model.findChatAgent(); held {
		return "agent/" + settings.Name
	}
	return ai.DescribeActiveModel(model.ai, model.aiProvider)
}

// describeChatProblem returns what to configure before the chat can ask, and an empty string
// where it can ask now.
func (model *Model) describeChatProblem(profile cfg.Profile) string {
	settings, held := model.findChatAgent()
	if !held {
		return ai.DescribeMissingSetting(model.ai, model.aiProvider)
	}
	return describeAgentProblem(settings)
}

// describeAgentProblem returns what stops this agent from answering.
func describeAgentProblem(settings cfg.AiAgentSettings) string {
	if settings.Command == "" {
		return "no command: set command under [ai.agents." + settings.Name +
			"] in the config file to the program that serves ACP on its standard input " +
			"and output."
	}
	if !cfg.FindsAgentCommand(settings) {
		return settings.Name + " is not installed: " + settings.Command +
			" is not on the PATH. Change the command of " + settings.Name +
			" in the settings, or install the program it runs."
	}
	return ""
}

// openChatResponder returns what answers the question, and what to close when the answer
// ends: a coding agent with the tools of the chat served to it, or a provider model with the
// tool loop of masume around it.
func (model *Model) openChatResponder(
	connection *app.Connection, cacheKey string, tools []mcp.Tool,
) (ai.Responder, func(), error) {
	settings, held := model.findChatAgent()
	if !held {
		opened, err := ai.OpenModel(model.ai, model.aiProvider, cacheKey)
		if err != nil {
			return nil, nil, err
		}
		return ai.OpenResponder(
			opened, ai.ResolveMaxToolSteps(model.ai, model.aiProvider)), func() {}, nil
	}

	// The agent runs in a process of its own, so it reaches the tools of the chat over the
	// loopback address rather than through a server of its own.
	served, err := mcp.StartLocalServer(tools, acp.ClientVersion, ai.LogEvent)
	if err != nil {
		return nil, nil, err
	}
	server := acp.BuildHTTPMcpServer(
		mcpServerName, served.Address(), served.Token())
	return acp.Open(settings, []acp.McpServer{server}, resolveAgentDirectory(),
			listToolNames(tools)),
		func() { _ = served.Close() }, nil
}

// resolveAgentDirectory returns the working directory of the session, which ACP requires as
// an absolute path.
func resolveAgentDirectory() string {
	held, err := os.Getwd()
	if err != nil {
		return os.TempDir()
	}
	return held
}

// resolveChatSteps returns the step limit of the tool loop of masume. An agent runs a loop
// of its own, so it has none here.
func (model *Model) resolveChatSteps() int {
	if _, held := model.findChatAgent(); held {
		return 0
	}
	return ai.ResolveMaxToolSteps(model.ai, model.aiProvider)
}

// askAgentToAct asks the user whether one action of an agent may run, and waits for the
// answer. The panel asks it, as it asks about a write.
func askAgentToAct(ctx context.Context, held chatRun, title, detail string) bool {
	ai.LogEvent("> asking to act: " + title)
	allowed := make(chan bool, 1)
	held.events <- app.ChatEvent{
		Run: held.run, Kind: app.ChatRunAsked, Allowed: allowed,
		Ask: app.PendingRun{
			Summary: title + " on " + string(held.profile.Environment), SQL: detail,
		},
	}

	select {
	case confirmed := <-allowed:
		if !confirmed {
			ai.LogEvent("< act refused")
			return false
		}
		ai.LogEvent("< act allowed")
		return true
	case <-ctx.Done():
		return false
	}
}

// sortedAgentNames returns the agent names in the order a list draws them.
func sortedAgentNames(agents map[string]cfg.AiAgentSettings) []string {
	names := make([]string, 0, len(agents))
	for name := range agents {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// describeAgentCommand returns the command of an agent as one line.
func describeAgentCommand(settings cfg.AiAgentSettings) string {
	return strings.TrimSpace(settings.Command + " " + strings.Join(settings.Args, " "))
}

// switchAiAgent changes the agent the chat would send to.
func (model *Model) switchAiAgent(
	connection *app.Connection, name string,
) (tea.Model, tea.Cmd) {
	if _, held := model.ai.Agents[name]; !held {
		return model, nil
	}
	model.aiAgent = name
	connection.Show("ai agent set to " + name)
	return model, nil
}

// listToolNames returns the names of the tools masume serves.
func listToolNames(tools []mcp.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

// buildToolSchemas returns the tool list in the form a provider reads.
func buildToolSchemas(tools []mcp.Tool) []ai.ToolSchema {
	schemas := make([]ai.ToolSchema, 0, len(tools))
	for _, tool := range tools {
		schemas = append(schemas, ai.ToolSchema{
			Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema,
		})
	}
	return schemas
}

// callChatTool runs one tool of the chat for a provider, and returns its result as the JSON
// text a model reads.
func callChatTool(
	ctx context.Context, tools []mcp.Tool, name string, input map[string]any,
) string {
	for _, tool := range tools {
		if tool.Name != name {
			continue
		}
		answered, err := tool.Call(ctx, input)
		if err != nil {
			return ai.EncodeToolOutput(map[string]any{"error": err.Error()})
		}
		return ai.EncodeToolOutput(answered)
	}
	return ai.EncodeToolOutput(map[string]any{"error": "no tool named " + name})
}
