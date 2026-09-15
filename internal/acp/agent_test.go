package acp

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/turanmahmudov/masume/internal/ai"
	"github.com/turanmahmudov/masume/internal/cfg"
)

// The tests drive the real client against a fake agent, which is this test binary run again
// with MASUME_FAKE_AGENT set. The script of the fake agent is the value of that variable.

// The scripts the fake agent can run.
const (
	scriptAnswers      = "answers"
	scriptCallsATool   = "calls-a-tool"
	scriptAsks         = "asks"
	scriptRefusesLogin = "refuses-login"
	scriptOffersLogin  = "offers-login"
	scriptNewerVersion = "newer-version"
	scriptDies         = "dies"
	scriptWaits        = "waits"
)

// TestMain runs the fake agent when the environment names a script, and the tests otherwise.
func TestMain(main *testing.M) {
	if script := os.Getenv("MASUME_FAKE_AGENT"); script != "" {
		runFakeAgent(script)
		return
	}
	os.Exit(main.Run())
}

// buildFakeAgent returns the settings that run this test binary as an agent.
func buildFakeAgent(t *testing.T, script string) cfg.AiAgentSettings {
	t.Helper()
	program, err := os.Executable()
	if err != nil {
		t.Fatalf("the test binary has no path: %v", err)
	}
	return cfg.AiAgentSettings{
		Name: "fake", Command: program, Env: []string{"MASUME_FAKE_AGENT=" + script},
	}
}

// collectedReply is everything one reply reported.
type collectedReply struct {
	guard sync.Mutex
	text  strings.Builder
	steps []string
	ends  int
	asked []string
}

// buildHooks returns hooks that collect what a reply reports, and answer permission with
// this answer.
func buildHooks(held *collectedReply, allow bool) ai.RunHooks {
	return ai.RunHooks{
		StartTextBlock: func() {
			held.guard.Lock()
			defer held.guard.Unlock()
			if held.text.Len() > 0 {
				held.text.WriteString("\n\n")
			}
		},
		AppendText: func(delta string) {
			held.guard.Lock()
			defer held.guard.Unlock()
			held.text.WriteString(delta)
		},
		StartToolStep: func(label string) {
			held.guard.Lock()
			defer held.guard.Unlock()
			held.steps = append(held.steps, label)
		},
		FinishToolStep: func() {
			held.guard.Lock()
			defer held.guard.Unlock()
			held.ends++
		},
		CallTool: func(context.Context, string, map[string]any) string { return "{}" },
		AskPermission: func(_ context.Context, title, detail string) bool {
			held.guard.Lock()
			held.asked = append(held.asked, title+" | "+detail)
			held.guard.Unlock()
			return allow
		},
		LogEvent: func(string) {},
	}
}

// buildTestRequest returns one question, in the form the chat sends.
func buildTestRequest() ai.Request {
	return ai.Request{
		System: "You are a SQL assistant.",
		Messages: []ai.Message{
			{Role: ai.RoleUser, Text: "which tables are there"},
			{Role: ai.RoleAssistant, Text: "Two tables."},
			{Role: ai.RoleUser, Text: "how many orders are unpaid"},
		},
	}
}

// openFakeAgent returns an agent that runs the script, in a directory of its own.
func openFakeAgent(t *testing.T, script string) *Agent {
	t.Helper()
	return Open(buildFakeAgent(t, script), []McpServer{{
		Name: "masume", Command: "masume", Args: []string{"--mcp", "--profile=shop"},
		Env: []McpEnvEntry{},
	}}, t.TempDir(), []string{"list_tables", "run_query"})
}

func TestAnAgentAnswersAQuestion(t *testing.T) {
	held := &collectedReply{}
	result, err := openFakeAgent(t, scriptAnswers).Reply(
		context.Background(), buildTestRequest(), buildHooks(held, true))
	if err != nil {
		t.Fatalf("the agent failed: %v", err)
	}
	if held.text.String() != "Four orders are unpaid." {
		t.Errorf("the reply reads %q", held.text.String())
	}
	if result.FinishReason != ai.FinishStop {
		t.Errorf("the agent stopped for %q", result.FinishReason)
	}
	if result.ReceivedChars != len("Four orders are unpaid.") {
		t.Errorf("the reply counts %d characters", result.ReceivedChars)
	}
	// The thinking of the agent is output the question paid for.
	wanted := ai.Usage{InputTokens: 1200, OutputTokens: 48, CachedInputTokens: 900}
	if result.Usage != wanted {
		t.Errorf("the question spent %+v, wanted %+v", result.Usage, wanted)
	}
}

// An agent that reports no count leaves the chat with none, rather than a made-up number.
func TestAnAgentThatReportsNoUsageLeavesItEmpty(t *testing.T) {
	if held := readUsage(nil); held != (ai.Usage{}) {
		t.Errorf("a missing count reads %+v", held)
	}
}

// The agent is handed the MCP server of masume, so it reaches the database through the tools
// masume already publishes.
func TestAnAgentIsHandedTheMcpServerOfMasume(t *testing.T) {
	held := &collectedReply{}
	if _, err := openFakeAgent(t, scriptAnswers).Reply(
		context.Background(), buildTestRequest(), buildHooks(held, true)); err != nil {
		t.Fatalf("the agent failed: %v", err)
	}

	// The fake agent writes what it received beside its own binary.
	written := readFakeRecord(t)
	servers, _ := written["mcpServers"].([]any)
	if len(servers) != 1 {
		t.Fatalf("the session opened with %v", written["mcpServers"])
	}
	server, _ := servers[0].(map[string]any)
	if server["name"] != "masume" {
		t.Errorf("the server reads %v", server)
	}
	args, _ := server["args"].([]any)
	if len(args) != 2 || args[0] != "--mcp" || args[1] != "--profile=shop" {
		t.Errorf("the server runs %v", args)
	}
}

// The prompt carries the instructions, the earlier turns and the question, because the agent
// keeps no history of its own between questions.
func TestTheAgentPromptCarriesTheConversation(t *testing.T) {
	written := BuildPromptText(buildTestRequest())
	for _, wanted := range []string{
		"You are a SQL assistant.",
		historyHeading,
		"User: which tables are there",
		"Assistant: Two tables.",
		questionHeading,
		"how many orders are unpaid",
	} {
		if !strings.Contains(written, wanted) {
			t.Errorf("the prompt does not hold %q:\n%s", wanted, written)
		}
	}
	// The last question stands under its own heading, not in the history.
	if strings.Contains(written, "User: how many orders are unpaid") {
		t.Errorf("the question is in the history as well:\n%s", written)
	}
}

// A tool call of the agent is drawn as a step of the reply, and text after it opens a block
// of its own.
func TestAnAgentReportsItsToolCallsAsSteps(t *testing.T) {
	held := &collectedReply{}
	if _, err := openFakeAgent(t, scriptCallsATool).Reply(
		context.Background(), buildTestRequest(), buildHooks(held, true)); err != nil {
		t.Fatalf("the agent failed: %v", err)
	}

	if len(held.steps) != 1 || held.steps[0] != "Reading the tables" {
		t.Errorf("the steps are %v", held.steps)
	}
	if held.ends != 1 {
		t.Errorf("%d steps ended, wanted one", held.ends)
	}
	if held.text.String() != "Let me look.\n\nFour orders." {
		t.Errorf("the reply reads %q", held.text.String())
	}
}

// The user answers a permission request of the agent, and the answer reaches the agent.
func TestAnAgentAsksBeforeItActs(t *testing.T) {
	for _, held := range []struct {
		name   string
		allow  bool
		wanted string
	}{
		{"allowed", true, "allow-once"},
		{"refused", false, "reject-once"},
	} {
		t.Run(held.name, func(t *testing.T) {
			collected := &collectedReply{}
			if _, err := openFakeAgent(t, scriptAsks).Reply(
				context.Background(), buildTestRequest(),
				buildHooks(collected, held.allow)); err != nil {
				t.Fatalf("the agent failed: %v", err)
			}

			if len(collected.asked) != 1 {
				t.Fatalf("the user was asked %v", collected.asked)
			}
			// The question names the action and the input of the call.
			if !strings.Contains(collected.asked[0], "Delete the orders") ||
				!strings.Contains(collected.asked[0], "delete from orders") {
				t.Errorf("the question reads %q", collected.asked[0])
			}
			if !strings.Contains(collected.text.String(), held.wanted) {
				t.Errorf("the agent read the answer as %q", collected.text.String())
			}
		})
	}
}

// An agent lists the ways it can be logged in whether or not it already is, so only a
// session it refuses to open says that it is not. The failure then says how to log in.
func TestAnAgentThatRefusesASessionSaysHowToLogIn(t *testing.T) {
	held := &collectedReply{}
	_, err := openFakeAgent(t, scriptRefusesLogin).Reply(
		context.Background(), buildTestRequest(), buildHooks(held, true))
	if err == nil {
		t.Fatal("a refused session reported nothing")
	}
	for _, wanted := range []string{"opened no session", "may not be logged in", "log in"} {
		if !strings.Contains(err.Error(), wanted) {
			t.Errorf("the failure does not hold %q: %v", wanted, err)
		}
	}
}

// An agent that offers a way to log in and opens a session anyway is not reported, because
// the list alone does not mean it is logged out.
func TestAnAgentThatOffersALoginStillOpensASession(t *testing.T) {
	held := &collectedReply{}
	if _, err := openFakeAgent(t, scriptOffersLogin).Reply(
		context.Background(), buildTestRequest(), buildHooks(held, true)); err != nil {
		t.Fatalf("an agent that offers a login failed: %v", err)
	}
	if held.text.String() != "Four orders are unpaid." {
		t.Errorf("the reply reads %q", held.text.String())
	}
}

// An agent that speaks a later version of the protocol is refused, because masume cannot
// read what it sends.
func TestAnAgentOnALaterProtocolIsRefused(t *testing.T) {
	held := &collectedReply{}
	_, err := openFakeAgent(t, scriptNewerVersion).Reply(
		context.Background(), buildTestRequest(), buildHooks(held, true))
	if err == nil || !strings.Contains(err.Error(), "protocol version") {
		t.Errorf("the agent failed with %v", err)
	}
}

// An agent that ends without answering is reported, rather than leaving the chat waiting.
func TestAnAgentThatEndsIsReported(t *testing.T) {
	held := &collectedReply{}
	_, err := openFakeAgent(t, scriptDies).Reply(
		context.Background(), buildTestRequest(), buildHooks(held, true))
	if err == nil {
		t.Fatal("an agent that ended reported nothing")
	}
}

// A stopped question ends the reply and keeps what already arrived.
func TestAStoppedQuestionEndsTheReply(t *testing.T) {
	held := &collectedReply{}
	ctx, stop := context.WithCancel(context.Background())
	agent := openFakeAgent(t, scriptWaits)

	go func() {
		// The fake agent writes one block and then waits, so the stop lands mid-reply.
		for {
			held.guard.Lock()
			arrived := held.text.Len() > 0
			held.guard.Unlock()
			if arrived {
				stop()
				return
			}
		}
	}()

	result, err := agent.Reply(ctx, buildTestRequest(), buildHooks(held, true))
	if err != nil {
		t.Fatalf("a stopped question failed: %v", err)
	}
	if held.text.String() != "Working on it." {
		t.Errorf("the reply reads %q", held.text.String())
	}
	if result.FinishReason != ai.FinishStop {
		t.Errorf("the reply stopped for %q", result.FinishReason)
	}
}

// A command that is not on the PATH names the agent, the command, and where to change it.
func TestAnAgentThatIsNotInstalledSaysWhatToInstall(t *testing.T) {
	agent := Open(cfg.AiAgentSettings{
		Name: "fake", Command: "masume-no-such-agent",
	}, nil, t.TempDir(), nil)

	held := &collectedReply{}
	_, err := agent.Reply(context.Background(), buildTestRequest(), buildHooks(held, true))
	if err == nil {
		t.Fatal("an agent that is not installed reported nothing")
	}
	for _, wanted := range []string{
		"fake is not installed", "masume-no-such-agent is not on the PATH",
		"in the settings",
	} {
		if !strings.Contains(err.Error(), wanted) {
			t.Errorf("the failure does not hold %q: %v", wanted, err)
		}
	}
}

func TestReadStopReason(t *testing.T) {
	for _, held := range [][2]string{
		{stopEndTurn, ai.FinishStop},
		{stopCancelled, ai.FinishStop},
		{stopMaxTokens, ai.FinishLength},
		{stopMaxTurnRequests, ai.FinishToolCalls},
		{stopRefusal, ai.FinishContentFilter},
		{"whatever", ai.FinishUnknown},
	} {
		if read := readStopReason(held[0]); read != held[1] {
			t.Errorf("%q reads %q, wanted %q", held[0], read, held[1])
		}
	}
}

// masume serves no file methods, so an agent that asks for one is refused rather than
// answered with nothing.
func TestMasumeRefusesTheFileMethods(t *testing.T) {
	open := &session{hooks: buildHooks(&collectedReply{}, true), ctx: context.Background()}
	for _, method := range []string{methodReadTextFile, methodWriteTextFile, "terminal/create"} {
		result, failure := open.answerRequest(method, json.RawMessage(`{}`))
		if failure == nil {
			t.Errorf("%s answered %v", method, result)
			continue
		}
		if failure.Code != codeMethodNotFound {
			t.Errorf("%s failed with %v", method, failure)
		}
	}
}

// readFakeRecord returns what the fake agent wrote about the session it opened.
func readFakeRecord(t *testing.T) map[string]any {
	t.Helper()
	written, err := os.ReadFile(fakeRecordPath())
	if err != nil {
		t.Fatalf("the fake agent wrote no record: %v", err)
	}
	read := map[string]any{}
	if err := json.Unmarshal(written, &read); err != nil {
		t.Fatalf("the record does not read: %v", err)
	}
	return read
}

// fakeRecordPath is where the fake agent writes the session it was asked to open.
func fakeRecordPath() string {
	return filepath.Join(os.TempDir(), "masume-fake-agent-session.json")
}

// A tool call carries a list of content blocks and a message chunk carries one, so the field
// is read only where the kind of update says what it holds. Reading it blindly drops every
// tool call of the agent.
func TestAToolCallWithContentIsStillAStep(t *testing.T) {
	open := &session{steps: map[string]bool{}, ctx: context.Background()}
	steps := []string{}
	open.hooks = ai.RunHooks{
		StartTextBlock: func() {}, AppendText: func(string) {},
		StartToolStep:  func(label string) { steps = append(steps, label) },
		FinishToolStep: func() {},
		LogEvent:       func(string) {},
	}
	open.id = "session-1"

	open.readNotification(methodSessionUpdate, json.RawMessage(`{
		"sessionId":"session-1",
		"update":{"sessionUpdate":"tool_call","toolCallId":"call-1",
			"title":"Reading the tables","status":"pending",
			"content":[{"type":"content","content":{"type":"text","text":"rows"}}]}}`))

	if len(steps) != 1 || steps[0] != "Reading the tables" {
		t.Errorf("the steps are %v", steps)
	}
}

// Agents report their models in two shapes, and masume sets each with its own method.
func TestReadModelChoiceReadsBothShapes(t *testing.T) {
	// A config option, as opencode reports it.
	option := readModelChoice(newSessionAnswer{ConfigOptions: []configOption{
		{ID: "mode", Category: "mode"},
		{ID: "model", Category: "model", Options: []configOptionValue{
			{Value: "opencode-go/deepseek-v4-flash", Name: "DeepSeek V4 Flash"},
		}},
	}})
	if !option.offersModels() || option.inSession || option.optionID != "model" {
		t.Errorf("the config option reads %+v", option)
	}
	if len(option.values) != 1 || option.values[0].Value != "opencode-go/deepseek-v4-flash" {
		t.Errorf("the models read %v", option.values)
	}

	// The session answer itself, as the Claude Code adapter reports it.
	held := readModelChoice(newSessionAnswer{Models: &sessionModelState{
		CurrentModelID: "default",
		AvailableModels: []sessionModel{
			{ModelID: "default", Name: "Default"}, {ModelID: "haiku", Name: "Haiku"},
		},
	}})
	if !held.offersModels() || !held.inSession || held.optionID != "" {
		t.Errorf("the session models read %+v", held)
	}
	if len(held.values) != 2 || held.values[1].Value != "haiku" {
		t.Errorf("the models read %v", held.values)
	}

	// An agent that reports neither offers no list.
	if readModelChoice(newSessionAnswer{}).offersModels() {
		t.Error("an agent with no list offers one")
	}
}

// The traffic log records what masume sends the agent, and the session carries the token of
// the tool server. A token in a file outlives the question it belongs to, so the log holds
// none.
func TestTheLogHoldsNoToken(t *testing.T) {
	written := redactTokens(`{"mcpServers":[{"type":"http","url":"http://127.0.0.1:1",` +
		`"headers":[{"name":"Authorization","value":"Bearer 195d667b80154f1e"}]}]}`)
	if strings.Contains(written, "195d667b") {
		t.Errorf("the line holds the token: %s", written)
	}
	if !strings.Contains(written, "Bearer <redacted>") {
		t.Errorf("the line reads %s", written)
	}
	// Every other part of the line is kept, so the log still follows the traffic.
	if !strings.Contains(written, "http://127.0.0.1:1") {
		t.Errorf("the line lost the address: %s", written)
	}
}

// buildPermissionParams returns one permission request about this action.
func buildPermissionParams(title string) json.RawMessage {
	encoded, _ := json.Marshal(permissionRequest{
		SessionID: "session-1",
		ToolCall:  permissionToolCall{ToolCallID: "call-1", Title: title},
		Options: []permissionOption{
			{OptionID: "allow-once", Name: "Allow once", Kind: optionAllowOnce},
			{OptionID: "reject-once", Name: "Reject", Kind: optionRejectOnce},
		},
	})
	return encoded
}

// masume governs its own tools, so an agent asking to run one is answered without a question
// to the reader. A read would otherwise ask on every call, and a write is asked about by the
// tool itself, which knows the profile.
func TestAnAgentNeedsNoPermissionForTheToolsOfMasume(t *testing.T) {
	held := &collectedReply{}
	open := &session{
		hooks: buildHooks(held, false), ctx: context.Background(), id: "session-1",
		served: []string{"list_tables", "run_query"},
	}

	// Agents name a tool of a server after that server, in one of two forms.
	for _, title := range []string{
		"list_tables", "masume_list_tables", "mcp__masume__run_query",
	} {
		answered, failure := open.answerRequest(
			methodRequestPermission, buildPermissionParams(title))
		if failure != nil {
			t.Fatalf("%s failed: %v", title, failure)
		}
		outcome := answered.(permissionAnswer).Outcome
		if outcome.Outcome != outcomeSelected || outcome.OptionID != "allow-once" {
			t.Errorf("%s answered %+v", title, outcome)
		}
	}
	if len(held.asked) != 0 {
		t.Errorf("the reader was asked %v", held.asked)
	}
}

// An agent brings tools of its own, and masume has no rules for those, so the reader answers.
func TestAnAgentAsksTheReaderForItsOwnTools(t *testing.T) {
	held := &collectedReply{}
	open := &session{
		hooks: buildHooks(held, false), ctx: context.Background(), id: "session-1",
		served: []string{"list_tables", "run_query"},
	}

	// A file named after the server is not a tool of the server.
	for _, title := range []string{"Bash", "Read /home/turan/masume/notes", "write_file"} {
		answered, failure := open.answerRequest(
			methodRequestPermission, buildPermissionParams(title))
		if failure != nil {
			t.Fatalf("%s failed: %v", title, failure)
		}
		outcome := answered.(permissionAnswer).Outcome
		if outcome.OptionID != "reject-once" {
			t.Errorf("%s answered %+v", title, outcome)
		}
	}
	if len(held.asked) != 3 {
		t.Errorf("the reader was asked %v", held.asked)
	}
}

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

// npx comes with Node, which the failure says for the agents masume starts with it.
func TestTheFailureNamesWhatCarriesTheCommand(t *testing.T) {
	held := describeStartFailure(
		cfg.AiAgentSettings{Name: "claude", Command: "npx"}, exec.ErrNotFound)
	if !strings.Contains(held.Error(), "npx comes with Node") {
		t.Errorf("the failure reads %v", held)
	}

	// A command masume does not know is named without a guess about what carries it.
	other := describeStartFailure(
		cfg.AiAgentSettings{Name: "goose", Command: "goose"}, exec.ErrNotFound)
	if strings.Contains(other.Error(), "Node") {
		t.Errorf("the failure guesses: %v", other)
	}
}
