package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/hist"
)

// buildChatModel answers a model with a conversation of a few turns open in the chat panel.
func buildChatModel(t *testing.T) (*Model, *app.Chat) {
	t.Helper()
	model := buildOfflineModelFor(t, 160, 48)
	connection := model.Active()
	connection.Chat.Messages = []app.ChatMessage{
		{Role: hist.ChatRoleUser, Content: "which table holds the orders?"},
		{Role: hist.ChatRoleAssistant, Content: "public.orders holds one row per order."},
		{Role: hist.ChatRoleUser, Content: "and the customers?"},
		{Role: hist.ChatRoleAssistant, Content: "public.customers, one row per customer."},
	}
	connection.Overlay = app.Overlay{Kind: app.OverlayAiChat, Draft: app.NewEditorBuffer("", 0)}
	return model, connection.Chat
}

// readChatRows answers the rows the panel draws now, through the same path the frame takes.
func readChatRows(model *Model, connection *app.Connection) []string {
	rows, _ := model.resolveChatRows(connection, model.resolveChatContent())
	return rows
}

// markedChatRow is laid over the rows that were kept. A draw of the conversation writes the
// turns, so the mark is gone wherever the conversation was drawn again.
const markedChatRow = "marked"

// markKeptChatRows lays the mark over the rows the model holds.
func markKeptChatRows(model *Model) {
	model.chatRows.rows = []string{markedChatRow}
}

// holdsMarkedChatRows is true while the rows the model holds are still the marked ones.
func holdsMarkedChatRows(model *Model) bool {
	return model.chatRows.drawn && len(model.chatRows.rows) == 1 &&
		model.chatRows.rows[0] == markedChatRow
}

// The scroll bounds, a jump between turns and the draw all read the rows, so one press drew
// the whole conversation three times. It is drawn once and kept.
func TestChatRowsAreKeptWhileNothingChanges(t *testing.T) {
	model, _ := buildChatModel(t)
	first := readChatRows(model, model.Active())

	markKeptChatRows(model)
	for range 20 {
		readChatRows(model, model.Active())
	}
	if !holdsMarkedChatRows(model) {
		t.Error("the conversation was drawn again although nothing changed")
	}

	model.chatRows.rows = first
	if len(readChatRows(model, model.Active())) != len(first) {
		t.Error("the conversation that was kept holds a different number of rows")
	}
}

// One test per input the rows are drawn from. A cache that misses one of these draws a
// conversation that is not the one on screen.
func TestChatRowsAreDrawnAgainForEveryChange(t *testing.T) {
	cases := []struct {
		name string
		// streaming asks for a reply to be written before the rows are first read,
		// because the calls and the wheel belong to one.
		streaming bool
		change    func(model *Model, chat *app.Chat)
	}{
		{name: "the panel is a different width", change: func(model *Model, _ *app.Chat) {
			model.width = 120
		}},
		{name: "the theme was changed", change: func(model *Model, _ *app.Chat) {
			model.styles.keepTheme(model.styles.Theme)
		}},
		{name: "a turn was added", change: func(_ *Model, chat *app.Chat) {
			chat.Messages = append(chat.Messages,
				app.ChatMessage{Role: hist.ChatRoleUser, Content: "and the roles?"})
		}},
		{name: "the text of a turn changed", change: func(_ *Model, chat *app.Chat) {
			chat.Messages[1].Content = "public.orders holds one row for each order placed."
		}},
		{
			name: "the text of a turn changed and kept its length",
			change: func(_ *Model, chat *app.Chat) {
				held := chat.Messages[1].Content
				chat.Messages[1].Content = strings.Repeat("x", len(held))
			},
		},
		{name: "the role of a turn changed", change: func(_ *Model, chat *app.Chat) {
			chat.Messages[1].Role = hist.ChatRoleUser
		}},
		{name: "a turn was marked", change: func(_ *Model, chat *app.Chat) {
			chat.HasTurn = true
		}},
		{name: "the mark moved to another turn", change: func(_ *Model, chat *app.Chat) {
			chat.HasTurn, chat.TurnAt = true, 2
		}},
		{name: "a reply began to stream", change: func(_ *Model, chat *app.Chat) {
			chat.Status = app.ChatStreaming
		}},
		{
			name: "a call of the reply finished", streaming: true,
			change: func(_ *Model, chat *app.Chat) {
				chat.StartStep("read the catalog")
				chat.FinishStep()
			},
		},
		{
			name: "the reply began another call", streaming: true,
			change: func(_ *Model, chat *app.Chat) {
				chat.Activity = "reading public.orders"
			},
		},
		{
			name: "the reply began at another moment", streaming: true,
			change: func(_ *Model, chat *app.Chat) {
				chat.StartedAt = time.Unix(1_700_000_000, 0)
			},
		},
		{
			name: "the wheel of the reply turned", streaming: true,
			change: func(model *Model, _ *app.Chat) { model.spinnerAt++ },
		},
	}

	for _, held := range cases {
		t.Run(held.name, func(t *testing.T) {
			model, chat := buildChatModel(t)
			if held.streaming {
				chat.Status = app.ChatStreaming
			}
			readChatRows(model, model.Active())

			markKeptChatRows(model)
			held.change(model, chat)
			readChatRows(model, model.Active())
			if holdsMarkedChatRows(model) {
				t.Error("the conversation was kept although " + held.name)
			}
		})
	}
}

// A cache that draws again and answers what it held before passes a test that only counts the
// draws, so the rows themselves are read.
func TestChatRowsDrawTheTextOfATurnThatChanged(t *testing.T) {
	model, chat := buildChatModel(t)
	readChatRows(model, model.Active())

	chat.Messages[1].Content = "the orders live in the shop schema"
	if !strings.Contains(strings.Join(readChatRows(model, model.Active()), "\n"), "shop schema") {
		t.Error("the rows do not hold the text the turn was changed to")
	}
	if strings.Contains(strings.Join(readChatRows(model, model.Active()), "\n"), "one row per order") {
		t.Error("the rows still hold the text the turn was changed from")
	}
}

// The wheel of a reply turns ten times a second and nothing else about it changes, so the rows
// of the turns before it are drawn again with it. A conversation with no reply being written
// keeps them whatever the wheel does.
func TestChatRowsAreKeptWhileNoReplyIsWritten(t *testing.T) {
	model, chat := buildChatModel(t)
	readChatRows(model, model.Active())

	markKeptChatRows(model)
	model.spinnerAt += 5
	chat.Activity = "something that ran before"
	readChatRows(model, model.Active())
	if !holdsMarkedChatRows(model) {
		t.Error("the conversation was drawn again although no reply is being written")
	}
}

// A jump between turns reads the row a turn begins on. It has to be the row the draw puts it
// on, or a jump lands somewhere else than the turn it named.
func TestFindChatTurnRowNamesTheRowTheTurnIsDrawnOn(t *testing.T) {
	model, chat := buildChatModel(t)
	connection := model.Active()
	rows, starts := model.resolveChatRows(connection, model.resolveChatContent())

	if len(starts) != len(chat.Messages) {
		t.Fatalf("the conversation answered %d turn rows for %d turns",
			len(starts), len(chat.Messages))
	}
	for turn := range chat.Messages {
		held := model.findChatTurnRow(connection, turn)
		if held != starts[turn] {
			t.Fatalf("turn %d was found on row %d, but it is drawn on row %d",
				turn, held, starts[turn])
		}
		if held >= len(rows) {
			t.Fatalf("turn %d was found on row %d, past the %d rows drawn",
				turn, held, len(rows))
		}
	}
	if model.findChatTurnRow(connection, 0) != 0 {
		t.Error("the first turn is not on the first row")
	}
	if model.findChatTurnRow(connection, len(chat.Messages)+4) !=
		starts[len(starts)-1] {
		t.Error("a turn past the last one is not held to the last turn")
	}
}

func TestPageUpScrollsTheChatBackToTheQuestion(t *testing.T) {
	model, chat := buildChatModel(t)
	chat.Messages = chat.Messages[:2]
	chat.Messages[1].Content = strings.Repeat("public.orders holds one row per order.\n\n", 30)
	chat.Follow = true
	model.render()
	if chat.Offset == 0 {
		t.Fatal("the conversation fits the panel")
	}

	for range 4 {
		model.readKey(tea.Key{Code: tea.KeyPgUp})
	}
	if !strings.Contains(stripEscapes(model.render()), "which table holds the orders?") {
		t.Errorf("the question is not drawn at offset %d", chat.Offset)
	}
}

func TestTheChatReadsItsKeysOutsideTheKeyRow(t *testing.T) {
	held := FindDialogActions(string(app.OverlayAiChat))
	for _, action := range []ActionID{
		ActionScrollBack, ActionScrollForward, ActionPreviousTurn, ActionNextTurn,
		ActionChatToNotebook,
	} {
		if !slices.Contains(held, action) {
			t.Errorf("the chat does not read %q", action)
		}
	}
}

// The chat sends on Enter, because a question is asked far more often than it is written over
// several lines. A modifier writes the line.
func TestTheChatSendsOnEnterAndWritesALineOnAModifier(t *testing.T) {
	model, chat := buildChatModel(t)
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayAiChat, Draft: app.NewEditorBuffer("how many orders", 15),
	}

	for _, held := range []struct {
		name string
		mod  uv.KeyMod
	}{{"shift", uv.ModShift}, {"alt", uv.ModAlt}} {
		t.Run(held.name+" writes a line", func(t *testing.T) {
			connection.Overlay.Draft = app.NewEditorBuffer("how many orders", 15)
			model.readOverlayKey(connection, tea.Key{Code: tea.KeyEnter, Mod: held.mod})
			if written := connection.Overlay.Draft.Text; written != "how many orders\n" {
				t.Errorf("the field holds %q", written)
			}
			if chat.IsStreaming() {
				t.Error("the question was sent")
			}
		})
	}

	connection.Overlay.Draft = app.NewEditorBuffer("how many orders", 15)
	model.readOverlayKey(connection, tea.Key{Code: tea.KeyEnter})
	if written := connection.Overlay.Draft.Text; written == "how many orders\n" {
		t.Errorf("Enter wrote a line instead of sending, and the field holds %q", written)
	}
}

// A call of the model stands where it happened, between the blocks of text around it, and a
// turn that ended keeps its calls.
func TestTheChatDrawsACallWhereItHappened(t *testing.T) {
	model, chat := buildChatModel(t)
	chat.StartTurn("how many orders are unpaid", "")
	chat.AppendDelta("Let me look.")
	chat.StartStep("listing the tables")
	chat.FinishStep()
	chat.StartTextBlock()
	chat.AppendDelta("Two unpaid orders.")

	rows := stripEscapes(strings.Join(readChatRows(model, model.Active()), "\n"))
	look, call, answer := strings.Index(rows, "Let me look."),
		strings.Index(rows, "listing the tables"), strings.Index(rows, "Two unpaid orders.")
	if look < 0 || call < 0 || answer < 0 {
		t.Fatalf("the turn reads:\n%s", rows)
	}
	if !(look < call && call < answer) {
		t.Errorf("the call is not between the blocks:\n%s", rows)
	}
	// The turn ended, and the call it made is still there.
	if !strings.Contains(rows, "✓ listing the tables") {
		t.Errorf("the call that finished is not marked:\n%s", rows)
	}
}

// The call that runs carries the wheel, so the panel draws it once and not twice.
func TestTheRunningCallIsDrawnOnce(t *testing.T) {
	model, chat := buildChatModel(t)
	chat.StartTurn("how many orders are unpaid", "")
	chat.Begin(func() {})
	chat.AppendDelta("Let me look.")
	chat.StartStep("running the query")

	rows := stripEscapes(strings.Join(readChatRows(model, model.Active()), "\n"))
	if strings.Count(rows, "running the query") != 1 {
		t.Errorf("the call is drawn more than once:\n%s", rows)
	}
	if strings.Contains(rows, "Thinking") {
		t.Errorf("the panel says it is thinking while a call runs:\n%s", rows)
	}
}

// A reply that runs no call says it is thinking, on a row of its own.
func TestAReplyWithNoCallSaysItIsThinking(t *testing.T) {
	model, chat := buildChatModel(t)
	chat.StartTurn("how many orders are unpaid", "")
	chat.Begin(func() {})

	rows := stripEscapes(strings.Join(readChatRows(model, model.Active()), "\n"))
	if !strings.Contains(rows, "Thinking") {
		t.Errorf("the panel says nothing while the model thinks:\n%s", rows)
	}
}

// A Markdown table in a reply is drawn as columns: a rule under the header, and the numbers
// against the right edge of their column.
func TestTheChatDrawsAMarkdownTableAsColumns(t *testing.T) {
	model, chat := buildChatModel(t)
	chat.Messages = append(chat.Messages, app.ChatMessage{
		Role: hist.ChatRoleAssistant,
		Content: "Revenue by country:\n| Country | Revenue |\n|---------|---------|\n" +
			"| NL | 304,274.92 |\n| DE | 9,985.59 |",
	})

	rows := stripEscapes(strings.Join(readChatRows(model, model.Active()), "\n"))
	for _, wanted := range []string{
		"Country     Revenue", "───────  ──────────", "NL       304,274.92", "DE         9,985.59",
	} {
		if !strings.Contains(rows, wanted) {
			t.Errorf("the rows do not have %q:\n%s", wanted, rows)
		}
	}
	if strings.Contains(rows, "|") {
		t.Errorf("the table is drawn with its pipes:\n%s", rows)
	}
}

func TestTheChatGroupsTheDigitsOfALongNumberColumn(t *testing.T) {
	model, chat := buildChatModel(t)
	chat.Messages = append(chat.Messages, app.ChatMessage{
		Role: hist.ChatRoleAssistant,
		Content: "| Year | Revenue |\n|------|---------|\n" +
			"| 2024 | 304274.92 |\n| 2025 | 1234.5 |",
	})

	rows := stripEscapes(strings.Join(readChatRows(model, model.Active()), "\n"))
	for _, wanted := range []string{"2024  304,274.92", "2025     1,234.5"} {
		if !strings.Contains(rows, wanted) {
			t.Errorf("the rows do not have %q:\n%s", wanted, rows)
		}
	}
}

// The most recent query is the one the insert key reads, so only that block has the key.
func TestOnlyTheMostRecentQueryHasTheInsertKey(t *testing.T) {
	model, chat := buildChatModel(t)
	chat.Messages = append(chat.Messages,
		app.ChatMessage{Role: hist.ChatRoleAssistant, Content: "```sql\nselect 1\n```"},
		app.ChatMessage{Role: hist.ChatRoleAssistant, Content: "```sql\nselect 2\n```"},
		app.ChatMessage{Role: hist.ChatRoleAssistant, Content: "no query here"},
	)

	rows := stripEscapes(strings.Join(readChatRows(model, model.Active()), "\n"))
	if strings.Count(rows, "^J insert") != 1 {
		t.Fatalf("the insert key is drawn %d times:\n%s",
			strings.Count(rows, "^J insert"), rows)
	}
	if strings.Index(rows, "^J insert") < strings.Index(rows, "select 2") {
		t.Errorf("the insert key is not under the most recent query:\n%s", rows)
	}
}

// The bottom border has the total the chat spent, and the status bar has the breakdown. The
// row under the field is for reports.
func TestTheChatShowsTokensOnItsBorder(t *testing.T) {
	model, chat := buildChatModel(t)
	chat.Usage.InputTokens, chat.Usage.OutputTokens = 390, 132
	chat.Usage.CachedInputTokens = 200
	model.Active().Autocommit = true

	frame := readFrameRows(model.View().Content)
	bottom := -1
	for at, row := range frame {
		if strings.Contains(row, "╰") && strings.Contains(row, "522 tokens") {
			bottom = at
		}
	}
	if bottom < 0 {
		t.Errorf("no border row has the total:\n%s", strings.Join(frame, "\n"))
	}
	if !strings.Contains(frame[len(frame)-1], "390 in (200 cached) / 132 out this session") {
		t.Errorf("the status bar reads %q", frame[len(frame)-1])
	}
	if strings.Count(strings.Join(frame, "\n"), "132 out") != 1 {
		t.Errorf("the breakdown is drawn more than once:\n%s", strings.Join(frame, "\n"))
	}
}
