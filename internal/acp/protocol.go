package acp

import "encoding/json"

// The wire shapes of the Agent Client Protocol, version 1. masume is the client and the
// coding agent is the agent, which runs as a child process.

// ProtocolVersion is the version masume speaks.
const ProtocolVersion = 1

// The methods masume calls on the agent.
const (
	methodInitialize = "initialize"
	methodNewSession = "session/new"
	methodPrompt     = "session/prompt"
	methodSetConfig  = "session/set_config_option"
	methodSetModel   = "session/set_model"
	methodCancel     = "session/cancel"
)

// The methods the agent calls on masume.
const (
	methodSessionUpdate     = "session/update"
	methodRequestPermission = "session/request_permission"
	methodReadTextFile      = "fs/read_text_file"
	methodWriteTextFile     = "fs/write_text_file"
)

// The kinds of update the agent sends while it answers.
const (
	updateAgentMessage = "agent_message_chunk"
	updateAgentThought = "agent_thought_chunk"
	updateToolCall     = "tool_call"
	updateToolCallEnd  = "tool_call_update"
	updatePlan         = "plan"
)

// The statuses one tool call of the agent reports.
const (
	statusPending    = "pending"
	statusInProgress = "in_progress"
	statusCompleted  = "completed"
	statusFailed     = "failed"
)

// The reasons the agent stops.
const (
	stopEndTurn         = "end_turn"
	stopCancelled       = "cancelled"
	stopMaxTokens       = "max_tokens"
	stopMaxTurnRequests = "max_turn_requests"
	stopRefusal         = "refusal"
)

// implementation names one side of the connection.
type implementation struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// fileCapabilities are the file methods the client serves. masume reads no files for an
// agent, so both are false and the agent never calls them.
type fileCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

type clientCapabilities struct {
	Fs fileCapabilities `json:"fs"`
	// Terminal is false, so the agent runs its commands itself.
	Terminal bool `json:"terminal"`
}

type initializeRequest struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities clientCapabilities `json:"clientCapabilities"`
	ClientInfo         implementation     `json:"clientInfo"`
}

type authMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type initializeAnswer struct {
	ProtocolVersion   int               `json:"protocolVersion"`
	AgentInfo         implementation    `json:"agentInfo"`
	AuthMethods       []authMethod      `json:"authMethods"`
	AgentCapabilities agentCapabilities `json:"agentCapabilities"`
}

// agentCapabilities is what the agent reported it can do.
type agentCapabilities struct {
	McpCapabilities struct {
		HTTP bool `json:"http"`
	} `json:"mcpCapabilities"`
}

// McpServer is one MCP server the agent connects to. masume passes its own server here, so
// the agent reaches the database through the tools of the chat.
type McpServer struct {
	// Type is "http" for a server the agent reaches over a URL. A server with a command
	// leaves it empty, which is the stdio transport every agent supports.
	Type string `json:"type,omitempty"`
	Name string `json:"name"`
	// The address and headers of an HTTP server.
	URL     string      `json:"url,omitempty"`
	Headers []McpHeader `json:"headers,omitempty"`
	// The program and environment of a stdio server.
	Command string        `json:"command,omitempty"`
	Args    []string      `json:"args,omitempty"`
	Env     []McpEnvEntry `json:"env,omitempty"`
}

// McpHeader is one header the agent sends to an HTTP server.
type McpHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// McpEnvEntry is one environment variable of a stdio server.
type McpEnvEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// BuildHTTPMcpServer returns an MCP server the agent reaches over a URL, with the token that
// admits it.
func BuildHTTPMcpServer(name, url, token string) McpServer {
	return McpServer{
		Type: "http", Name: name, URL: url,
		Headers: []McpHeader{{Name: "Authorization", Value: "Bearer " + token}},
	}
}

type newSessionRequest struct {
	Cwd        string      `json:"cwd"`
	McpServers []McpServer `json:"mcpServers"`
}

type newSessionAnswer struct {
	SessionID     string         `json:"sessionId"`
	ConfigOptions []configOption `json:"configOptions"`
	// Models is the model list of an agent that reports one in the session answer rather
	// than as a config option.
	Models *sessionModelState `json:"models"`
}

// sessionModelState is the model list an agent reports in the session answer.
type sessionModelState struct {
	AvailableModels []sessionModel `json:"availableModels"`
	CurrentModelID  string         `json:"currentModelId"`
}

// sessionModel is one model of that list.
type sessionModel struct {
	ModelID string `json:"modelId"`
	Name    string `json:"name"`
}

// setModelRequest chooses the model of an agent that reports its list in the session answer.
type setModelRequest struct {
	SessionID string `json:"sessionId"`
	ModelID   string `json:"modelId"`
}

// configOption is one setting of the session the agent offers, such as its model or its
// mode.
type configOption struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Category     string              `json:"category"`
	Type         string              `json:"type"`
	CurrentValue string              `json:"currentValue"`
	Options      []configOptionValue `json:"options"`
}

// configOptionValue is one value a config option offers.
type configOptionValue struct {
	Value string `json:"value"`
	Name  string `json:"name"`
}

// The config option masume sets, and the kind of value it sends.
const (
	modelOptionID   = "model"
	modelOptionKind = "model"
	idOptionType    = "id"
)

// setConfigOptionRequest chooses one setting of the session.
type setConfigOptionRequest struct {
	SessionID string `json:"sessionId"`
	ConfigID  string `json:"configId"`
	Type      string `json:"type"`
	Value     string `json:"value"`
}

// contentBlock is one piece of a prompt or of an answer.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type promptRequest struct {
	SessionID string         `json:"sessionId"`
	Prompt    []contentBlock `json:"prompt"`
}

type promptAnswer struct {
	StopReason string `json:"stopReason"`
	Usage      *usage `json:"usage"`
}

// usage is the token count of one question, which an agent reports when its answer ends.
type usage struct {
	InputTokens      int `json:"inputTokens"`
	OutputTokens     int `json:"outputTokens"`
	ThoughtTokens    int `json:"thoughtTokens"`
	CachedReadTokens int `json:"cachedReadTokens"`
}

type cancelNotification struct {
	SessionID string `json:"sessionId"`
}

// sessionUpdate is one thing the agent reports while it answers. A message chunk carries one
// content block and a tool call carries a list of them, so the field is read only where the
// kind of update says what it holds.
type sessionUpdate struct {
	SessionUpdate string          `json:"sessionUpdate"`
	Content       json.RawMessage `json:"content"`
	ToolCallID    string          `json:"toolCallId"`
	Title         string          `json:"title"`
	Kind          string          `json:"kind"`
	Status        string          `json:"status"`
}

// readChunkText returns the text of a message chunk, and an empty string for anything else.
func (update sessionUpdate) readChunkText() string {
	if len(update.Content) == 0 {
		return ""
	}
	block := contentBlock{}
	if err := json.Unmarshal(update.Content, &block); err != nil {
		return ""
	}
	if block.Type != "text" {
		return ""
	}
	return block.Text
}

type sessionNotification struct {
	SessionID string        `json:"sessionId"`
	Update    sessionUpdate `json:"update"`
}

// permissionOption is one answer the agent offers to a permission request.
type permissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	// Kind is allow_once, allow_always, reject_once or reject_always.
	Kind string `json:"kind"`
}

// The kinds of permission option, which say which answers allow and which refuse.
const (
	optionAllowOnce   = "allow_once"
	optionAllowAlways = "allow_always"
	optionRejectOnce  = "reject_once"
)

type permissionToolCall struct {
	ToolCallID string          `json:"toolCallId"`
	Title      string          `json:"title"`
	Kind       string          `json:"kind"`
	RawInput   json.RawMessage `json:"rawInput"`
}

type permissionRequest struct {
	SessionID string             `json:"sessionId"`
	ToolCall  permissionToolCall `json:"toolCall"`
	Options   []permissionOption `json:"options"`
}

// permissionOutcome is the answer of the user, either a chosen option or a cancellation.
type permissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

type permissionAnswer struct {
	Outcome permissionOutcome `json:"outcome"`
}

// The outcomes a permission request can have.
const (
	outcomeSelected  = "selected"
	outcomeCancelled = "cancelled"
)
