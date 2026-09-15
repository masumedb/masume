package acp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// The fake agent, which this test binary becomes when MASUME_FAKE_AGENT names a script. It
// speaks the protocol on its standard input and output, as a real agent does.

// fakeAgent holds what the fake agent writes and the session it opened.
type fakeAgent struct {
	script   string
	session  string
	promptID json.RawMessage
	write    func(message)
}

// runFakeAgent reads requests until the stream ends, and answers them as the script says.
func runFakeAgent(script string) {
	output := bufio.NewWriter(os.Stdout)
	held := &fakeAgent{script: script, write: func(written message) {
		written.JSONRPC = "2.0"
		encoded, _ := json.Marshal(written)
		_, _ = output.Write(append(encoded, '\n'))
		_ = output.Flush()
	}}

	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 0, 64*1024), maxMessageBytes)
	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		if line == "" {
			continue
		}
		read := message{}
		if err := json.Unmarshal([]byte(line), &read); err != nil {
			continue
		}
		held.answer(read)
	}
	os.Exit(0)
}

// answer answers one message of masume.
func (held *fakeAgent) answer(read message) {
	switch read.Method {
	case methodInitialize:
		held.answerInitialize(read)
	case methodNewSession:
		held.answerNewSession(read)
	case methodPrompt:
		held.answerPrompt(read)
	case methodCancel:
		// A cancelled question still answers the request it was asked.
		held.write(message{ID: held.promptID, Result: buildResult(
			promptAnswer{StopReason: stopCancelled})})
	}
}

func (held *fakeAgent) answerInitialize(read message) {
	answered := initializeAnswer{
		ProtocolVersion: ProtocolVersion,
		AgentInfo:       implementation{Name: "fake", Version: "1.0.0"},
	}
	if held.script == scriptNewerVersion {
		answered.ProtocolVersion = ProtocolVersion + 1
	}
	if held.script == scriptRefusesLogin || held.script == scriptOffersLogin {
		answered.AuthMethods = []authMethod{{
			ID: "oauth", Name: "Log in", Description: "Run fake auth login to log in",
		}}
	}
	held.write(message{ID: read.ID, Result: buildResult(answered)})
	if held.script == scriptDies {
		os.Exit(1)
	}
}

func (held *fakeAgent) answerNewSession(read message) {
	// The record says what masume asked for, so the test can read it back.
	_ = os.WriteFile(fakeRecordPath(), []byte(read.Params), 0o600)
	if held.script == scriptRefusesLogin {
		held.write(message{ID: read.ID, Error: &rpcError{
			Code: -32000, Message: "Authentication required",
		}})
		return
	}
	held.session = "session-1"
	held.write(message{ID: read.ID, Result: buildResult(
		newSessionAnswer{SessionID: held.session})})
}

// answerPrompt runs the script of this fake agent.
func (held *fakeAgent) answerPrompt(read message) {
	held.promptID = read.ID
	switch held.script {
	case scriptAnswers, scriptOffersLogin:
		held.sendText("Four orders are unpaid.")
	case scriptCallsATool:
		held.sendText("Let me look.")
		held.sendUpdate(sessionUpdate{
			SessionUpdate: updateToolCall, ToolCallID: "call-1",
			Title: "Reading the tables", Kind: "read", Status: statusPending,
		})
		held.sendUpdate(sessionUpdate{
			SessionUpdate: updateToolCallEnd, ToolCallID: "call-1",
			Status: statusCompleted,
		})
		held.sendText("Four orders.")
	case scriptAsks:
		held.sendText(held.askToAct())
	case scriptWaits:
		held.sendText("Working on it.")
		// The question ends when masume cancels it.
		return
	}
	held.write(message{ID: read.ID, Result: buildResult(promptAnswer{
		StopReason: stopEndTurn,
		Usage: &usage{
			InputTokens: 1200, OutputTokens: 40, ThoughtTokens: 8,
			CachedReadTokens: 900,
		},
	})})
}

// askToAct asks masume for permission and returns the option it chose.
func (held *fakeAgent) askToAct() string {
	encoded, _ := json.Marshal(permissionRequest{
		SessionID: held.session,
		ToolCall: permissionToolCall{
			ToolCallID: "call-1", Title: "Delete the orders", Kind: "execute",
			RawInput: json.RawMessage(`{"sql":"delete from orders"}`),
		},
		Options: []permissionOption{
			{OptionID: "allow-once", Name: "Allow once", Kind: optionAllowOnce},
			{OptionID: "reject-once", Name: "Reject", Kind: optionRejectOnce},
		},
	})
	held.write(message{ID: json.RawMessage("9001"), Method: methodRequestPermission,
		Params: encoded})

	// The answer arrives on the same stream, so the fake agent reads one more line.
	reader := bufio.NewScanner(os.Stdin)
	reader.Buffer(make([]byte, 0, 64*1024), maxMessageBytes)
	for reader.Scan() {
		read := message{}
		if err := json.Unmarshal([]byte(strings.TrimSpace(reader.Text())), &read); err != nil {
			continue
		}
		if string(read.ID) != "9001" {
			continue
		}
		answered := permissionAnswer{}
		_ = json.Unmarshal(read.Result, &answered)
		if answered.Outcome.Outcome != outcomeSelected {
			return "the user " + answered.Outcome.Outcome + " it"
		}
		return "the user chose " + answered.Outcome.OptionID
	}
	return "no answer"
}

// sendText reports one block of the reply.
func (held *fakeAgent) sendText(text string) {
	encoded, _ := json.Marshal(contentBlock{Type: "text", Text: text})
	held.sendUpdate(sessionUpdate{
		SessionUpdate: updateAgentMessage, Content: encoded,
	})
}

// sendUpdate reports one thing the agent did.
func (held *fakeAgent) sendUpdate(update sessionUpdate) {
	encoded, _ := json.Marshal(sessionNotification{
		SessionID: held.session, Update: update,
	})
	held.write(message{Method: methodSessionUpdate, Params: encoded})
}

// buildResult returns a value as the result of an answer.
func buildResult(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(fmt.Sprintf("%q", err.Error()))
	}
	return encoded
}
