package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/agent"
	"github.com/turanmahmudov/masume/internal/mcp"
)

// openLocalServer returns a server of one tool, and closes it with the test.
func openLocalServer(t *testing.T) *mcp.LocalServer {
	t.Helper()
	served, err := mcp.StartLocalServer([]mcp.Tool{{
		Name: "probe", Description: "A probe tool.",
		InputSchema: agent.BuildEmptySchema(),
		Call: func(context.Context, map[string]any) (any, error) {
			return map[string]any{"ran": true}, nil
		},
	}}, "probe", func(string) {})
	if err != nil {
		t.Fatalf("the server did not start: %v", err)
	}
	t.Cleanup(func() { _ = served.Close() })
	return served
}

// askLocalServer sends one message and returns the answer and the status.
func askLocalServer(t *testing.T, served *mcp.LocalServer, token, body string) (
	map[string]any, int,
) {
	t.Helper()
	asked, err := http.NewRequest(
		http.MethodPost, served.Address(), strings.NewReader(body))
	if err != nil {
		t.Fatalf("the request was not built: %v", err)
	}
	asked.Header.Set("authorization", "Bearer "+token)
	answered, err := http.DefaultClient.Do(asked)
	if err != nil {
		t.Fatalf("the server answered nothing: %v", err)
	}
	defer func() { _ = answered.Body.Close() }()

	read := map[string]any{}
	written := bytes.Buffer{}
	_, _ = written.ReadFrom(answered.Body)
	if written.Len() > 0 {
		_ = json.Unmarshal(written.Bytes(), &read)
	}
	return read, answered.StatusCode
}

// The server answers the tools of the chat, so an agent reads what the reader reads.
func TestTheLocalServerAnswersTheToolsOfTheChat(t *testing.T) {
	served := openLocalServer(t)

	read, status := askLocalServer(t, served, served.Token(),
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if status != http.StatusOK {
		t.Fatalf("the server answered %d", status)
	}
	result, _ := read["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("the server offers %v", result["tools"])
	}
	if held, _ := tools[0].(map[string]any); held["name"] != "probe" {
		t.Errorf("the tool reads %v", tools[0])
	}

	read, _ = askLocalServer(t, served, served.Token(),
		`{"jsonrpc":"2.0","id":2,"method":"tools/call",`+
			`"params":{"name":"probe","arguments":{}}}`)
	if _, held := read["result"]; !held {
		t.Errorf("the call answered %v", read)
	}
}

// The server stands on the loopback address behind a token, so no other program on this
// machine reaches the connection of the chat.
func TestTheLocalServerRefusesARequestWithoutItsToken(t *testing.T) {
	served := openLocalServer(t)
	if !strings.HasPrefix(served.Address(), "http://127.0.0.1:") {
		t.Errorf("the server stands at %s", served.Address())
	}
	if len(served.Token()) < 32 {
		t.Errorf("the token reads %q", served.Token())
	}

	if _, status := askLocalServer(t, served, "",
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); status != http.StatusUnauthorized {
		t.Errorf("a request with no token answered %d", status)
	}
	if _, status := askLocalServer(t, served, "wrong-token",
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); status != http.StatusUnauthorized {
		t.Errorf("a request with a wrong token answered %d", status)
	}
}

// A notification has no answer, and a closed server answers nothing at all.
func TestTheLocalServerAnswersANotificationWithNoBody(t *testing.T) {
	served := openLocalServer(t)
	if _, status := askLocalServer(t, served, served.Token(),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`); status !=
		http.StatusAccepted {
		t.Errorf("a notification answered %d", status)
	}

	if err := served.Close(); err != nil {
		t.Fatalf("the server did not close: %v", err)
	}
	if _, err := http.Get(served.Address()); err == nil {
		t.Error("a closed server still answers")
	}
}

// A web page can reach a port on this machine, and it sends the origin of the page. No MCP
// client sends one, so a request that carries one is refused before the token is read.
func TestTheLocalServerRefusesARequestFromAPage(t *testing.T) {
	served := openLocalServer(t)
	asked, err := http.NewRequest(http.MethodPost, served.Address(),
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatalf("the request was not built: %v", err)
	}
	asked.Header.Set("authorization", "Bearer "+served.Token())
	asked.Header.Set("origin", "https://example.com")

	answered, err := http.DefaultClient.Do(asked)
	if err != nil {
		t.Fatalf("the server answered nothing: %v", err)
	}
	defer func() { _ = answered.Body.Close() }()
	if answered.StatusCode != http.StatusForbidden {
		t.Errorf("a request from a page answered %d", answered.StatusCode)
	}
}
