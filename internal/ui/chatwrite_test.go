package ui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/ai"
	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/mcp"
)

// scriptedCall is one tool call of the scripted model.
type scriptedCall struct {
	tool  string
	input map[string]any
}

// scriptedResponder is a model that calls the tools in order and keeps every tool result.
type scriptedResponder struct {
	calls   []scriptedCall
	answers []string
}

func (responder *scriptedResponder) Describe() string { return "scripted" }

func (responder *scriptedResponder) Reply(
	ctx context.Context, _ ai.Request, hooks ai.RunHooks,
) (ai.RunResult, error) {
	for _, call := range responder.calls {
		hooks.StartToolStep(call.tool)
		responder.answers = append(responder.answers,
			hooks.CallTool(ctx, call.tool, call.input))
		hooks.FinishToolStep()
		if err := ctx.Err(); err != nil {
			return ai.RunResult{}, err
		}
	}
	hooks.AppendText("done")
	return ai.RunResult{ReceivedChars: len("done"), FinishReason: ai.FinishStop}, nil
}

// callRunQuery returns a run_query call of this statement.
func callRunQuery(sql string) scriptedCall {
	return scriptedCall{tool: "run_query", input: map[string]any{"sql": sql}}
}

// decodeToolAnswer reads one tool result as the model reads it.
func decodeToolAnswer(t *testing.T, text string) map[string]any {
	t.Helper()
	decoded := map[string]any{}
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		t.Fatalf("the tool result is not JSON: %v: %s", err, text)
	}
	return decoded
}

// runScriptedChat sends one question to the scripted model, feeds the events of the run
// into the model and calls answer each time the chat waits for a yes. It returns once
// the run closes.
func (held *liveWorkspace) runScriptedChat(
	t *testing.T, responder *scriptedResponder, answer func(stop context.CancelFunc),
) {
	t.Helper()
	connection := held.connection
	openChatPanel(held.model)
	history := connection.Chat.StartTurn("hold order 3", "")

	ctx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	run, events := connection.Chat.Begin(stop)
	deps := held.model.buildChatToolDeps(ctx, connection, run, events)
	tools := mcp.BuildTools(mcp.BindConnection(deps))
	go runChatReply(ctx, chatRun{
		responder: responder,
		request:   held.model.buildChatRequest(connection, history, tools),
		tools:     tools, run: run, events: events,
		provider: responder.Describe(), profile: connection.Profile(),
		close: func() {},
	})

	id := held.model.ActiveID()
	for {
		message := waitForChatEvents(id, run, events)()
		held.model.Update(message)
		if _, closed := message.(chatClosedMsg); closed {
			return
		}
		if connection.Chat.Pending != nil {
			answer(stop)
		}
	}
}

// pressChatKey presses one key on the open chat.
func (held *liveWorkspace) pressChatKey(t *testing.T, key tea.KeyPressMsg) {
	t.Helper()
	held.model.Update(key)
}

var (
	pressYes       = tea.KeyPressMsg{Code: 'y', Text: "y"}
	pressNo        = tea.KeyPressMsg{Code: 'n', Text: "n"}
	pressStopReply = tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl}
)

// The chat asks before a read as well, and the rows of the read go back to the model.
func TestTheChatReturnsTheRowsOfAnAllowedRead(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	responder := &scriptedResponder{calls: []scriptedCall{
		callRunQuery("select count(*) as paid from orders where status = 'paid'"),
	}}

	asked := 0
	held.runScriptedChat(t, responder, func(context.CancelFunc) {
		asked++
		pending := held.connection.Chat.Pending
		if !strings.Contains(pending.Summary, "is classified as read-only") {
			t.Errorf("the question reads %q", pending.Summary)
		}
		if len(pending.Plan) != 0 {
			t.Errorf("a read carries the plan %q", pending.Plan)
		}
		held.pressChatKey(t, pressYes)
	})

	if asked != 1 {
		t.Errorf("the chat asked %d times", asked)
	}
	answer := decodeToolAnswer(t, responder.answers[0])
	if answer["ran"] != true || answer["rowCount"] != float64(1) {
		t.Fatalf("the model reads %v", answer)
	}
	if rows := answer["rows"].([]any); rows[0].([]any)[0] != float64(2) {
		t.Errorf("the model reads the rows %v", rows)
	}
	if chat := held.connection.Chat; chat.Status != app.ChatIdle || chat.Problem != "" {
		t.Errorf("the chat ends %q with %q", chat.Status, chat.Problem)
	}
}

// A write shows its plan in the question. A no goes back to the model and writes nothing.
func TestTheChatWritesNothingTheUserRefuses(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	write := "update orders set status = 'held' where id = 3"
	responder := &scriptedResponder{calls: []scriptedCall{
		{tool: "plan_write", input: map[string]any{"sql": write}},
		callRunQuery(write),
	}}

	held.runScriptedChat(t, responder, func(context.CancelFunc) {
		pending := held.connection.Chat.Pending
		if !strings.Contains(pending.Summary, "writes to the database") ||
			pending.SQL != write {
			t.Errorf("the question reads %q for %q", pending.Summary, pending.SQL)
		}
		plan := strings.Join(pending.Plan, "\n")
		for _, wanted := range []string{
			"1 of 3 in orders", "status", "1 row to capture before the write",
		} {
			if !strings.Contains(plan, wanted) {
				t.Errorf("the plan does not hold %q:\n%s", wanted, plan)
			}
		}
		screen := strings.Join(held.readFrame(t), "\n")
		if !strings.Contains(screen, "run this on shop?") {
			t.Errorf("the panel does not ask:\n%s", screen)
		}
		held.pressChatKey(t, pressNo)
	})

	measured := decodeToolAnswer(t, responder.answers[0])
	if measured["measured"] != true || measured["rows"] != float64(1) ||
		measured["table"] != "main.orders" {
		t.Errorf("the plan reads %v", measured)
	}
	refused := decodeToolAnswer(t, responder.answers[1])
	if refused["ran"] != false || refused["reason"] != refusedByUser {
		t.Errorf("the model reads %v", refused)
	}
	if status := held.readStatusOfOrder(t, 3); status != "cancelled" {
		t.Errorf("order 3 is %q after a no", status)
	}
	if held.connection.Undo != nil {
		t.Errorf("a refused write kept the undo %+v", held.connection.Undo)
	}
}

// A yes runs the write, and its undo goes back to the model and to the connection.
func TestTheChatRunsAnAllowedWriteAndKeepsItsUndo(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	write := "update orders set status = 'held' where id = 3"
	responder := &scriptedResponder{calls: []scriptedCall{callRunQuery(write)}}

	held.runScriptedChat(t, responder, func(context.CancelFunc) {
		held.pressChatKey(t, pressYes)
	})

	answer := decodeToolAnswer(t, responder.answers[0])
	if answer["ran"] != true || answer["affected"] != float64(1) {
		t.Fatalf("the model reads %v", answer)
	}
	undo, _ := answer["undo"].([]any)
	if len(undo) != 1 || !strings.Contains(undo[0].(string), "'cancelled'") {
		t.Errorf("the model reads the undo %v", answer["undo"])
	}
	if status := held.readStatusOfOrder(t, 3); status != "held" {
		t.Errorf("order 3 is %q after a yes", status)
	}
	kept := held.connection.Undo
	if kept == nil || !kept.Undo.IsHeld() || kept.SQL != write {
		t.Fatalf("the connection keeps the undo %+v", kept)
	}
	if held.connection.Chat.Notice == "" {
		t.Error("the panel reports nothing about the write")
	}
}

// A read-only connection refuses a write before the user is asked, and writes nothing.
func TestTheChatRefusesAWriteOnAReadOnlyConnection(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessReadOnly)
	responder := &scriptedResponder{calls: []scriptedCall{
		callRunQuery("update orders set status = 'held' where id = 3"),
	}}

	held.runScriptedChat(t, responder, func(context.CancelFunc) {
		t.Errorf("the chat asks %q on a read-only connection",
			held.connection.Chat.Pending.Summary)
		held.pressChatKey(t, pressYes)
	})

	answer := decodeToolAnswer(t, responder.answers[0])
	if answer["ran"] != false || !strings.Contains(answer["reason"].(string), "read-only") {
		t.Errorf("the model reads %v", answer)
	}
	if status := held.readStatusOfOrder(t, 3); status != "cancelled" {
		t.Errorf("order 3 is %q on a read-only connection", status)
	}
}

// An update with no WHERE clause is asked about as a write to every row.
func TestTheChatNamesAWriteToEveryRow(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	responder := &scriptedResponder{calls: []scriptedCall{
		callRunQuery("update orders set status = 'held'"),
	}}

	held.runScriptedChat(t, responder, func(context.CancelFunc) {
		pending := held.connection.Chat.Pending
		if !strings.Contains(pending.Summary, "has no WHERE clause and may affect every row") {
			t.Errorf("the question reads %q", pending.Summary)
		}
		if plan := strings.Join(pending.Plan, "\n"); !strings.Contains(plan, "3 of 3 in orders") ||
			!strings.Contains(plan, "every row") {
			t.Errorf("the plan reads:\n%s", plan)
		}
		held.pressChatKey(t, pressNo)
	})

	for id := 1; id <= 3; id++ {
		if status := held.readStatusOfOrder(t, id); status == "held" {
			t.Errorf("order %d is held after a no", id)
		}
	}
}

// A reply stopped while a write waits refuses the write, and the panel asks nothing more.
func TestStoppingTheChatRefusesTheWriteThatWaits(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	responder := &scriptedResponder{calls: []scriptedCall{
		callRunQuery("update orders set status = 'held' where id = 3"),
	}}

	held.runScriptedChat(t, responder, func(context.CancelFunc) {
		held.pressChatKey(t, pressStopReply)
	})

	answer := decodeToolAnswer(t, responder.answers[0])
	if answer["ran"] != false || answer["reason"] != refusedByUser {
		t.Errorf("the model reads %v", answer)
	}
	if status := held.readStatusOfOrder(t, 3); status != "cancelled" {
		t.Errorf("order 3 is %q after the stop", status)
	}
	chat := held.connection.Chat
	if chat.Pending != nil || chat.Status != app.ChatIdle {
		t.Errorf("the chat ends %q with the question %+v", chat.Status, chat.Pending)
	}
}

// A run cancelled while the question waits refuses the write without an answer.
func TestACancelledRunRefusesTheWriteThatWaits(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	responder := &scriptedResponder{calls: []scriptedCall{
		callRunQuery("update orders set status = 'held' where id = 3"),
	}}

	held.runScriptedChat(t, responder, func(stop context.CancelFunc) { stop() })

	answer := decodeToolAnswer(t, responder.answers[0])
	if answer["ran"] != false || answer["reason"] != refusedByUser {
		t.Errorf("the model reads %v", answer)
	}
	if status := held.readStatusOfOrder(t, 3); status != "cancelled" {
		t.Errorf("order 3 is %q after the cancel", status)
	}
}

// A tool the chat does not have is an error the model reads.
func TestTheChatReportsAnUnknownTool(t *testing.T) {
	held := buildLiveWorkspace(t, cfg.AccessWrite)
	responder := &scriptedResponder{calls: []scriptedCall{
		{tool: "drop_everything", input: map[string]any{}},
	}}

	held.runScriptedChat(t, responder, func(context.CancelFunc) {
		t.Error("an unknown tool asks the user")
		held.pressChatKey(t, pressNo)
	})

	answer := decodeToolAnswer(t, responder.answers[0])
	if answer["error"] != "no tool named drop_everything" {
		t.Errorf("the model reads %v", answer)
	}
}
