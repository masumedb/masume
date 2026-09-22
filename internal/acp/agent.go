package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/turanmahmudov/masume/internal/ai"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/proc"
)

// One coding agent, reached over ACP. masume starts the agent as a child process, hands it
// its own MCP server, and asks one question.

// ClientVersion is the masume version the handshake reports. An agent requires it, so the
// command sets it at start.
var ClientVersion = "dev"

// Agent is one coding agent of `[ai.agents]`.
type Agent struct {
	settings cfg.AiAgentSettings
	servers  []McpServer
	// cwd is the working directory of the session, which ACP requires.
	cwd string
	// served is the tools masume serves this agent. masume answers a permission request
	// about one of them itself, under the settings of the profile, and puts every other
	// request to the reader.
	served []string
}

// Open returns an agent that runs this command and reaches the database through these MCP
// servers. served is the tools masume serves.
func Open(
	settings cfg.AiAgentSettings, servers []McpServer, cwd string, served []string,
) *Agent {
	return &Agent{settings: settings, servers: servers, cwd: cwd, served: served}
}

// Describe returns the agent the chat sends to.
func (held *Agent) Describe() string { return "agent/" + held.settings.Name }

// session is one question in progress: the connection, the hooks, and what arrived.
type session struct {
	connection *connection
	hooks      ai.RunHooks
	id         string
	// guard covers every field below it, which the reader writes and the caller reads.
	guard sync.Mutex
	// chars is the reply character count, and started is true after the first block.
	chars   int
	started bool
	// steps is the tool calls that are open, so an end reports only a call that started.
	steps map[string]bool
	// ctx ends the question, and cancels the permission requests that wait.
	ctx context.Context
	// models is the model list of the session, in whichever shape the agent reported it.
	models modelChoice
	// served is the tools masume serves, which it answers permission for itself.
	served []string
	// servers is the names the agent knows the MCP servers of masume by.
	servers []string
}

// modelChoice is the model list of one session. An agent reports it as a config option or
// in the session answer itself, and the two are set by different methods.
type modelChoice struct {
	// optionID is the config option that chooses the model, for an agent with one.
	optionID string
	// inSession is true for an agent that reports its models in the session answer.
	inSession bool
	values    []ModelOption
}

// offersModels is true where the session reported a model list.
func (choice modelChoice) offersModels() bool {
	return choice.optionID != "" || choice.inSession
}

// start runs the agent as a child process and opens the connection to it.
func (held *Agent) start(hooks ai.RunHooks) (*exec.Cmd, *session, error) {
	child := exec.Command(held.settings.Command, held.settings.Args...)
	child.Dir = held.cwd
	child.Env = append(os.Environ(), held.settings.Env...)
	// The agent starts programs of its own, so it leads a process group. Ending the group
	// ends the whole agent; ending the child alone leaves its programs running.
	proc.LeadGroup(child)
	input, err := child.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	output, err := child.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	faults, err := child.StderrPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := child.Start(); err != nil {
		return nil, nil, describeStartFailure(held.settings, err)
	}

	open := &session{
		hooks: hooks, steps: map[string]bool{}, served: held.served,
		servers: held.readServerNames(), ctx: context.Background(),
	}
	open.connection = openConnection(input, handlers{
		callMe:   open.answerRequest,
		notifyMe: open.readNotification,
		logLine:  hooks.LogEvent,
	})
	go open.connection.readUntilEOF(output)
	go readFaultLines(faults, hooks.LogEvent)
	return child, open, nil
}

// closeChild ends the agent and every program it started, whether it answered or the user
// stopped it.
func closeChild(child *exec.Cmd) {
	if child.Process == nil {
		_ = child.Wait()
		return
	}
	if err := proc.KillGroup(child); err != nil {
		_ = child.Process.Kill()
	}
	_ = child.Wait()
}

// Reply asks the agent one question and reports what it does until it stops.
func (held *Agent) Reply(
	ctx context.Context, request ai.Request, hooks ai.RunHooks,
) (ai.RunResult, error) {
	result := ai.RunResult{FinishReason: ai.FinishUnknown}

	child, open, err := held.start(hooks)
	if err != nil {
		return result, err
	}
	defer closeChild(child)
	open.ctx = ctx

	if err := held.startSession(ctx, open); err != nil {
		return result, err
	}
	return held.askQuestion(ctx, open, request)
}

// startSession runs the handshake and opens one session with the MCP servers of masume.
func (held *Agent) startSession(ctx context.Context, open *session) error {
	started := initializeAnswer{}
	if err := open.callAndRead(ctx, methodInitialize, initializeRequest{
		ProtocolVersion: ProtocolVersion,
		ClientCapabilities: clientCapabilities{
			Fs: fileCapabilities{ReadTextFile: false, WriteTextFile: false},
		},
		ClientInfo: implementation{
			Name: "masume", Title: "masume", Version: ClientVersion,
		},
	}, &started); err != nil {
		return fmt.Errorf("the agent refused the handshake: %w", err)
	}
	if started.ProtocolVersion > ProtocolVersion {
		return fmt.Errorf("the agent speaks protocol version %d and masume speaks %d",
			started.ProtocolVersion, ProtocolVersion)
	}
	if held.needsHTTPMcp() && !started.AgentCapabilities.McpCapabilities.HTTP {
		return fmt.Errorf("%s reads no MCP server over a URL, and masume serves the "+
			"tools of the chat over one", held.settings.Name)
	}
	// An agent lists the ways it can be logged in whether or not it already is, so only a
	// session it refuses to open says that it is not.
	// An agent reads the list as an array, so a client with no server sends an empty one
	// rather than a null.
	servers := held.servers
	if servers == nil {
		servers = []McpServer{}
	}
	opened := newSessionAnswer{}
	if err := open.callAndRead(ctx, methodNewSession, newSessionRequest{
		Cwd: held.cwd, McpServers: servers,
	}, &opened); err != nil {
		if written := describeLogin(err, started.AuthMethods); written != "" {
			return fmt.Errorf("the agent opened no session: %w. %s", err, written)
		}
		return fmt.Errorf("the agent opened no session: %w", err)
	}
	if opened.SessionID == "" {
		return errors.New("the agent opened a session with no id")
	}
	open.id = opened.SessionID
	open.models = readModelChoice(opened)
	return held.chooseModel(ctx, open)
}

// needsHTTPMcp is true where masume hands this agent a server it reaches over a URL.
func (held *Agent) needsHTTPMcp() bool {
	for _, server := range held.servers {
		if server.Type == "http" {
			return true
		}
	}
	return false
}

// chooseModel sets the model of the session to the one the config file names. An agent that
// offers no model list is sent nothing.
func (held *Agent) chooseModel(ctx context.Context, open *session) error {
	wanted := strings.TrimSpace(held.settings.Model)
	if wanted == "" || !open.models.offersModels() {
		return nil
	}

	answered := json.RawMessage{}
	var err error
	if open.models.inSession {
		err = open.callAndRead(ctx, methodSetModel, setModelRequest{
			SessionID: open.id, ModelID: wanted,
		}, &answered)
	} else {
		err = open.callAndRead(ctx, methodSetConfig, setConfigOptionRequest{
			SessionID: open.id, ConfigID: open.models.optionID,
			Type: idOptionType, Value: wanted,
		}, &answered)
	}
	if err != nil {
		return fmt.Errorf("the agent refused the model %q: %w", wanted, err)
	}
	return nil
}

// readModelChoice returns the model list of the session, from whichever shape the agent
// reported it in.
func readModelChoice(opened newSessionAnswer) modelChoice {
	for _, option := range opened.ConfigOptions {
		if option.Category != modelOptionKind && option.ID != modelOptionID {
			continue
		}
		found := modelChoice{optionID: option.ID}
		for _, value := range option.Options {
			found.values = append(found.values, ModelOption(value))
		}
		return found
	}
	if opened.Models == nil {
		return modelChoice{}
	}
	found := modelChoice{inSession: true}
	for _, model := range opened.Models.AvailableModels {
		found.values = append(found.values,
			ModelOption{Value: model.ModelID, Name: model.Name})
	}
	return found
}

// ModelOption is one model an agent offers.
type ModelOption struct {
	Value string
	Name  string
}

// ListModels returns the models this agent offers. It opens a session and closes it, so it
// asks nothing and spends nothing.
func ListModels(ctx context.Context, settings cfg.AiAgentSettings) ([]ModelOption, error) {
	held := Open(settings, nil, resolveListDirectory(), nil)
	// The list is read from the session, so the agent is started without a model set.
	held.settings.Model = ""

	child, open, err := held.start(RunLogHooks())
	if err != nil {
		return nil, err
	}
	defer closeChild(child)

	if err := held.startSession(ctx, open); err != nil {
		return nil, err
	}
	if !open.models.offersModels() {
		return nil, fmt.Errorf("%s offers no model list", settings.Name)
	}
	return open.models.values, nil
}

// RunLogHooks returns hooks that only write to the log, for a run with nothing to draw.
func RunLogHooks() ai.RunHooks {
	return ai.RunHooks{
		StartTextBlock: func() {}, AppendText: func(string) {},
		StartToolStep: func(string) {}, FinishToolStep: func() {},
		LogEvent: ai.LogEvent,
	}
}

// resolveListDirectory returns a directory for a session that only reads the model list.
func resolveListDirectory() string {
	if held, err := os.Getwd(); err == nil {
		return held
	}
	return os.TempDir()
}

// describeLogin returns how to log this agent in, and an empty string where the failure is
// not one a login answers.
func describeLogin(reason error, methods []authMethod) string {
	if !refusesForAuth(reason) {
		return ""
	}
	written := make([]string, 0, len(methods))
	for _, method := range methods {
		held := method.Description
		if held == "" {
			held = method.Name
		}
		if held != "" {
			written = append(written, held)
		}
	}
	if len(written) == 0 {
		return ""
	}
	return "The agent may not be logged in: " + strings.Join(written, "; ")
}

// refusesForAuth is true for a failure the agent itself raised. JSON-RPC keeps -32000 to
// -32099 for the errors of a server, and an agent refuses a session for a missing login
// there. Every other reserved code is a protocol fault, which no login answers.
func refusesForAuth(reason error) bool {
	failure := &rpcError{}
	if !errors.As(reason, &failure) {
		return false
	}
	return failure.Code <= -32000 && failure.Code >= -32099
}

// askQuestion sends the question and waits for the agent to stop, or for the user to.
func (held *Agent) askQuestion(
	ctx context.Context, open *session, request ai.Request,
) (ai.RunResult, error) {
	result := ai.RunResult{FinishReason: ai.FinishUnknown}
	room, err := open.connection.call(methodPrompt, promptRequest{
		SessionID: open.id,
		Prompt:    []contentBlock{{Type: "text", Text: BuildPromptText(request)}},
	})
	if err != nil {
		return result, err
	}

	select {
	case held := <-room:
		result.ReceivedChars = open.readChars()
		if held.err != nil {
			return result, held.err
		}
		answered := promptAnswer{}
		if err := json.Unmarshal(held.result, &answered); err != nil {
			return result, err
		}
		result.FinishReason = readStopReason(answered.StopReason)
		result.Usage = readUsage(answered.Usage)
		return result, nil
	case <-ctx.Done():
		// The agent stops its work and answers the question it was asked.
		_ = open.connection.notify(methodCancel, cancelNotification{SessionID: open.id})
		<-room
		result.ReceivedChars = open.readChars()
		result.FinishReason = ai.FinishStop
		return result, nil
	}
}

// callAndRead sends one request and reads its answer into the value.
func (open *session) callAndRead(
	ctx context.Context, method string, params any, into any,
) error {
	room, err := open.connection.call(method, params)
	if err != nil {
		return err
	}
	select {
	case held := <-room:
		if held.err != nil {
			return held.err
		}
		return json.Unmarshal(held.result, into)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// readChars returns the reply character count so far.
func (open *session) readChars() int {
	open.guard.Lock()
	defer open.guard.Unlock()
	return open.chars
}

// readNotification reads one thing the agent reports while it answers.
func (open *session) readNotification(method string, params json.RawMessage) {
	if method != methodSessionUpdate {
		return
	}
	read := sessionNotification{}
	if err := json.Unmarshal(params, &read); err != nil {
		open.hooks.LogEvent("! cannot parse the update: " + err.Error())
		return
	}
	if read.SessionID != open.id {
		return
	}

	switch read.Update.SessionUpdate {
	case updateAgentMessage:
		open.appendText(read.Update.readChunkText())
	case updateToolCall:
		open.startStep(read.Update)
	case updateToolCallEnd:
		open.finishStep(read.Update)
	}
}

// appendText reports a block of the reply. A block that follows a tool call starts a block
// of its own.
func (open *session) appendText(text string) {
	if text == "" {
		return
	}
	open.guard.Lock()
	first := !open.started
	open.started = true
	open.chars += len([]rune(text))
	open.guard.Unlock()

	if first {
		open.hooks.StartTextBlock()
	}
	open.hooks.AppendText(text)
}

// startStep reports the tool call the agent started.
func (open *session) startStep(update sessionUpdate) {
	if update.Status == statusCompleted || update.Status == statusFailed {
		return
	}
	open.guard.Lock()
	if open.steps[update.ToolCallID] {
		open.guard.Unlock()
		return
	}
	open.steps[update.ToolCallID] = true
	// Text after a call belongs to a block of its own.
	open.started = false
	open.guard.Unlock()

	open.hooks.StartToolStep(describeStep(update))
}

// finishStep reports the end of a tool call that started.
func (open *session) finishStep(update sessionUpdate) {
	if update.Status != statusCompleted && update.Status != statusFailed {
		return
	}
	open.guard.Lock()
	started := open.steps[update.ToolCallID]
	delete(open.steps, update.ToolCallID)
	open.guard.Unlock()
	if started {
		open.hooks.FinishToolStep()
	}
}

// describeStep returns the label of one tool call, for the row the chat draws.
func describeStep(update sessionUpdate) string {
	if update.Title != "" {
		return update.Title
	}
	if update.Kind != "" {
		return update.Kind
	}
	return "working"
}

// answerRequest answers one request of the agent. masume serves the permission request and
// refuses every other method.
func (open *session) answerRequest(
	method string, params json.RawMessage,
) (any, *rpcError) {
	if method != methodRequestPermission {
		return nil, &rpcError{
			Code: codeMethodNotFound,
			Message: "masume serves no " + method +
				"; it is a database client, not an editor",
		}
	}

	read := permissionRequest{}
	if err := json.Unmarshal(params, &read); err != nil {
		return nil, &rpcError{Code: codeInternalError, Message: err.Error()}
	}
	if open.ctx.Err() != nil {
		return permissionAnswer{Outcome: permissionOutcome{Outcome: outcomeCancelled}}, nil
	}

	allowed, refused := findPermissionOptions(read.Options)
	// masume governs its own tools. A read needs no question, and a write is asked about
	// by the tool itself, which knows the profile and measures the rows.
	if allowed != "" && open.servesTool(describePermission(read)) {
		return permissionAnswer{
			Outcome: permissionOutcome{Outcome: outcomeSelected, OptionID: allowed},
		}, nil
	}
	if allowed == "" {
		return permissionAnswer{
			Outcome: permissionOutcome{Outcome: outcomeCancelled},
		}, nil
	}
	if open.hooks.AskPermission == nil ||
		!open.hooks.AskPermission(open.ctx, describePermission(read), describeRawInput(read)) {
		if refused == "" {
			return permissionAnswer{
				Outcome: permissionOutcome{Outcome: outcomeCancelled},
			}, nil
		}
		return permissionAnswer{
			Outcome: permissionOutcome{Outcome: outcomeSelected, OptionID: refused},
		}, nil
	}
	return permissionAnswer{
		Outcome: permissionOutcome{Outcome: outcomeSelected, OptionID: allowed},
	}, nil
}

// readServerNames returns the names the agent knows the MCP servers of masume by.
func (held *Agent) readServerNames() []string {
	names := make([]string, 0, len(held.servers))
	for _, server := range held.servers {
		names = append(names, server.Name)
	}
	return names
}

// servesTool is true where this action is one of the tools masume serves. An agent names a
// tool of a server after that server, as `mcp__masume__list_tables` or `masume_list_tables`.
// The agent writes the title, and only these three forms are taken.
func (open *session) servesTool(title string) bool {
	for _, name := range open.served {
		if title == name {
			return true
		}
		for _, server := range open.servers {
			if title == server+"_"+name || title == "mcp__"+server+"__"+name {
				return true
			}
		}
	}
	return false
}

// findPermissionOptions returns the option that allows this action once and the option that
// refuses it. An option that allows every later action is never chosen for the user.
func findPermissionOptions(options []permissionOption) (string, string) {
	allowed, refused := "", ""
	for _, option := range options {
		if option.Kind == optionAllowOnce && allowed == "" {
			allowed = option.OptionID
		}
		if option.Kind == optionRejectOnce && refused == "" {
			refused = option.OptionID
		}
	}
	// An agent that offers only a standing permission still needs an answer.
	if allowed == "" {
		for _, option := range options {
			if option.Kind == optionAllowAlways {
				allowed = option.OptionID
				break
			}
		}
	}
	return allowed, refused
}

// describePermission returns the line the question shows.
func describePermission(read permissionRequest) string {
	title := read.ToolCall.Title
	if title == "" {
		title = read.ToolCall.Kind
	}
	if title == "" {
		title = "an action"
	}
	return title
}

// maxShownInput is the number of characters of the tool input the question shows.
const maxShownInput = 2000

// describeRawInput returns the input of the call as text, cut to what a card can hold.
func describeRawInput(read permissionRequest) string {
	if len(read.ToolCall.RawInput) == 0 {
		return ""
	}
	written := strings.TrimSpace(string(read.ToolCall.RawInput))
	if len([]rune(written)) > maxShownInput {
		written = string([]rune(written)[:maxShownInput]) + "…"
	}
	return written
}

// readUsage returns the token count of one question. An agent that reports none leaves it at
// zero, and the chat then shows no count.
func readUsage(spent *usage) ai.Usage {
	if spent == nil {
		return ai.Usage{}
	}
	return ai.Usage{
		InputTokens: spent.InputTokens,
		// The thinking of the agent is output the question paid for.
		OutputTokens:      spent.OutputTokens + spent.ThoughtTokens,
		CachedInputTokens: spent.CachedReadTokens,
	}
}

// readStopReason maps the reason the agent stopped to a shared finish reason.
func readStopReason(written string) string {
	switch written {
	case stopEndTurn:
		return ai.FinishStop
	case stopCancelled:
		return ai.FinishStop
	case stopMaxTokens:
		return ai.FinishLength
	case stopMaxTurnRequests:
		return ai.FinishToolCalls
	case stopRefusal:
		return ai.FinishContentFilter
	}
	return ai.FinishUnknown
}

// describeNodeHint names what carries the command, for the ones masume knows.
func describeNodeHint(command string) string {
	if command == "npx" || command == "npm" {
		return "npx comes with Node. "
	}
	return ""
}

// readFaultLines reads what the agent writes on its standard error into the log.
func readFaultLines(faults io.Reader, logLine func(string)) {
	reader := bufio.NewScanner(faults)
	reader.Buffer(make([]byte, 0, 64*1024), maxMessageBytes)
	for reader.Scan() {
		if line := strings.TrimSpace(reader.Text()); line != "" {
			logLine("! " + line)
		}
	}
}

// describeStartFailure returns what to do about an agent that did not start.
func describeStartFailure(settings cfg.AiAgentSettings, err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%s is not installed: %s is not on the PATH. %sChange the "+
			"command of %s in the settings, or install the program it runs",
			settings.Name, settings.Command, describeNodeHint(settings.Command),
			settings.Name)
	}
	return fmt.Errorf("%s did not start: %w", settings.Command, err)
}
