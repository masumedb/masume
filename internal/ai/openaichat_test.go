package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/cfg"
)

// buildChatEvent returns one event of this stream, which carries no event name.
func buildChatEvent(data string) string {
	return "data: " + data + "\n\n"
}

// The prepared answers of a server with the chat completions endpoint. The first requests a
// call, with its arguments in fragments, and the second replies.
var chatCallsATool = buildChatEvent(
	`{"choices":[{"index":0,"delta":{"role":"assistant","content":"Let me look."}}]}`) +
	buildChatEvent(
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1",`+
			`"type":"function","function":{"name":"list_tables","arguments":"{\"lim"}}]}}]}`) +
	buildChatEvent(
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,`+
			`"function":{"arguments":"it\":5}"}}]}}]}`) +
	buildChatEvent(
		`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],`+
			`"usage":{"prompt_tokens":100,"completion_tokens":20}}`) +
	buildChatEvent(chatDone)

var chatAnswers = buildChatEvent(
	`{"choices":[{"index":0,"delta":{"role":"assistant","content":"Two "}}]}`) +
	buildChatEvent(`{"choices":[{"index":0,"delta":{"content":"tables."}}]}`) +
	buildChatEvent(
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],`+
			`"usage":{"prompt_tokens":170,"completion_tokens":55,`+
			`"prompt_tokens_details":{"cached_tokens":128}}}`) +
	buildChatEvent(chatDone)

func TestChatCompletionsAsksAndRunsWhatItIsAskedFor(t *testing.T) {
	server, sent := serveCannedStreams(t, chatCallsATool, chatAnswers)
	model := openChatModel("qwen3-coder", "", server.URL+"/v1")

	result, steps, written := collectRun(t, model, buildTestRequest())
	if written != "Let me look.\n\nTwo tables." {
		t.Errorf("the reply reads %q", written)
	}
	if len(steps) != 1 || steps[0] != "listing the tables" {
		t.Errorf("the steps are %v", steps)
	}
	if result.FinishReason != FinishStop {
		t.Errorf("the run stopped for %q", result.FinishReason)
	}
	wanted := Usage{InputTokens: 270, OutputTokens: 75, CachedInputTokens: 128}
	if result.Usage != wanted {
		t.Errorf("the run spent %+v, wanted %+v", result.Usage, wanted)
	}

	if len(*sent) != 2 {
		t.Fatalf("the model was asked %d times, wanted twice", len(*sent))
	}
	first := (*sent)[0]
	if first.path != "/v1/chat/completions" {
		t.Errorf("the request went to %s", first.path)
	}
	// A local server needs no key, so a request without one carries no header.
	if held := first.headers.Get("authorization"); held != "" {
		t.Errorf("the request carries %q", held)
	}
	if first.body["tool_choice"] != "auto" {
		t.Errorf("the request names the tool choice %v", first.body["tool_choice"])
	}
	options, _ := first.body["stream_options"].(map[string]any)
	if options["include_usage"] != true {
		t.Errorf("the request asks for usage as %v", first.body["stream_options"])
	}
	// The system prompt is the first message of this protocol.
	messages, _ := first.body["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("the first request holds %d messages, wanted two", len(messages))
	}
	if system, _ := messages[0].(map[string]any); system["role"] != "system" {
		t.Errorf("the first message reads %v", messages[0])
	}
}

// The second request repeats the call and answers it in a message of its own.
func TestChatCompletionsSendsTheCallAndItsResult(t *testing.T) {
	server, sent := serveCannedStreams(t, chatCallsATool, chatAnswers)
	model := openChatModel("qwen3-coder", "probe-key", server.URL+"/v1")
	collectRun(t, model, buildTestRequest())

	if held := (*sent)[0].headers.Get("authorization"); held != "Bearer probe-key" {
		t.Errorf("the request carries %q", held)
	}
	messages, _ := (*sent)[1].body["messages"].([]any)
	if len(messages) != 4 {
		t.Fatalf("the second request holds %d messages, wanted four", len(messages))
	}

	assistant, _ := messages[2].(map[string]any)
	calls, _ := assistant["tool_calls"].([]any)
	if len(calls) != 1 {
		t.Fatalf("the assistant turn holds %v", assistant["tool_calls"])
	}
	call, _ := calls[0].(map[string]any)
	function, _ := call["function"].(map[string]any)
	if call["id"] != "call_1" || function["name"] != "list_tables" {
		t.Errorf("the call reads %v", call)
	}
	// The fragments of the arguments are joined in the order they arrived.
	if function["arguments"] != `{"limit":5}` {
		t.Errorf("the arguments read %v", function["arguments"])
	}

	answered, _ := messages[3].(map[string]any)
	if answered["role"] != "tool" || answered["tool_call_id"] != "call_1" {
		t.Errorf("the result reads %v", answered)
	}
	if !strings.Contains(answered["content"].(string), `"called":"list_tables"`) {
		t.Errorf("the result carries %v", answered["content"])
	}
}

// A server that reports an error inside the stream answers with 200, so the error of the
// event is the error of the run.
func TestChatCompletionsReportsAnErrorOfTheStream(t *testing.T) {
	server, _ := serveCannedStreams(t,
		buildChatEvent(`{"error":{"message":"model \"qwen3\" not found"}}`))
	model := openChatModel("qwen3", "", server.URL+"/v1")

	_, err := RunChat(context.Background(), model, buildTestRequest(),
		cfg.DefaultMaxToolSteps, RunHooks{
			StartTextBlock: func() {}, AppendText: func(string) {},
			StartToolStep: func(string) {}, FinishToolStep: func() {},
			CallTool: func(context.Context, string, map[string]any) string { return "{}" },
			LogEvent: func(string) {},
		})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("the run failed with %v", err)
	}
}

func TestReadChatStopReason(t *testing.T) {
	cases := []struct {
		written string
		asked   bool
		wanted  string
	}{
		{"stop", false, FinishStop},
		{"tool_calls", true, FinishToolCalls},
		// Some servers request calls and still report the reason as a plain stop.
		{"stop", true, FinishToolCalls},
		{"length", false, FinishLength},
		{"content_filter", false, FinishContentFilter},
		{"", false, FinishUnknown},
		{"", true, FinishToolCalls},
		{"abandoned", false, FinishOther},
	}
	for _, held := range cases {
		if answered := readChatStopReason(held.written, held.asked); answered != held.wanted {
			t.Errorf("%q with calls=%v reads %q, wanted %q",
				held.written, held.asked, answered, held.wanted)
		}
	}
}
