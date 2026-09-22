package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/hist"
)

// openChatPanel opens the chat of the connection on show.
func openChatPanel(model *Model) *app.Chat {
	connection := model.Active()
	connection.Overlay = app.Overlay{
		Kind: app.OverlayAiChat, Draft: app.NewEditorBuffer("", 0),
	}
	return connection.Chat
}

// pressAskAgain returns the press that asks the last question again.
func pressAskAgain() tea.Key { return tea.Key{Code: 'r', Mod: uv.ModCtrl} }

// A question that failed is asked again with one key, so a provider that faltered needs no
// retyping.
func TestTheChatAsksTheLastQuestionAgain(t *testing.T) {
	model := buildOfflineModel(t, 100, 40)
	chat := openChatPanel(model)
	chat.StartTurn("how many orders are unpaid", "")
	chat.Fail("the provider said no")

	model.readKey(pressAskAgain())
	// The client has no key, so the ask fails again. The panel reporting the provider is
	// what says the question was asked.
	if chat.Status != app.ChatFailed || !strings.Contains(chat.Problem, "API key") {
		t.Errorf("the chat reads %q and %q", chat.Status, chat.Problem)
	}
	if held, _ := chat.FindLastQuestion(); held != "how many orders are unpaid" {
		t.Errorf("the question reads %q", held)
	}
}

// The key is offered while a question can be asked again, and not while one is being
// answered or while a statement waits for a yes.
func TestTheAskAgainKeyFollowsTheChat(t *testing.T) {
	model := buildOfflineModel(t, 100, 40)
	chat := openChatPanel(model)
	scene := keyScene{chat: chat}

	if asksAgain(scene) {
		t.Error("an empty chat offers to ask again")
	}
	chat.StartTurn("how many orders are unpaid", "")
	if !asksAgain(scene) {
		t.Error("a chat with a question does not offer to ask it again")
	}

	chat.Status = app.ChatStreaming
	if asksAgain(scene) {
		t.Error("a chat writing a reply offers to ask again")
	}
	chat.Status = app.ChatIdle
	chat.Ask(app.PendingRun{Summary: "writes to the database"}, make(chan bool, 1))
	if asksAgain(scene) {
		t.Error("a chat waiting for a yes offers to ask again")
	}
}

// A chat with no question asks nothing again.
func TestAnEmptyChatAsksNothingAgain(t *testing.T) {
	model := buildOfflineModel(t, 100, 40)
	chat := openChatPanel(model)

	model.readKey(pressAskAgain())
	if len(chat.Messages) != 0 {
		t.Errorf("the chat holds %+v", chat.Messages)
	}
}

// The key that fills the editor takes the most recent statement of the conversation. A
// question answered in prose does not hide the query of the answer before it.
func TestTheChatTakesTheMostRecentQuery(t *testing.T) {
	model := buildOfflineModel(t, 100, 40)
	chat := openChatPanel(model)
	chat.Messages = []app.ChatMessage{
		{Role: hist.ChatRoleUser, Content: "the unpaid orders"},
		{Role: hist.ChatRoleAssistant,
			Content: "Here it is.\n\n```sql\nselect * from orders where paid = 0;\n```"},
		{Role: hist.ChatRoleUser, Content: "what does paid mean"},
		{Role: hist.ChatRoleAssistant, Content: "It is 1 once the invoice is settled."},
	}

	if !holdsChatQuery(keyScene{chat: chat}) {
		t.Error("the chat offers no query to the editor")
	}
	model.readKey(tea.Key{Code: 'j', Mod: uv.ModCtrl})

	tab := model.Active().Active()
	if !strings.Contains(tab.Editor.Text, "where paid = 0") {
		t.Errorf("the editor reads %q", tab.Editor.Text)
	}
	if chat.Notice != "" {
		t.Errorf("the panel reports %q", chat.Notice)
	}
}

// A conversation that wrote no statement says so, and offers no key for one.
func TestAChatWithNoQuerySaysSo(t *testing.T) {
	model := buildOfflineModel(t, 100, 40)
	chat := openChatPanel(model)
	chat.Messages = []app.ChatMessage{
		{Role: hist.ChatRoleUser, Content: "what is in this database"},
		{Role: hist.ChatRoleAssistant, Content: "Orders and customers."},
	}

	if holdsChatQuery(keyScene{chat: chat}) {
		t.Error("a chat with no statement offers one to the editor")
	}
	model.readKey(tea.Key{Code: 'j', Mod: uv.ModCtrl})
	if !strings.Contains(chat.Notice, "no query yet") {
		t.Errorf("the panel reports %q", chat.Notice)
	}
}

// The reply goes on the clipboard, which is how the prose of an answer leaves the panel.
func TestTheChatCopiesTheLastReply(t *testing.T) {
	model := buildOfflineModel(t, 100, 40)
	chat := openChatPanel(model)

	if holdsChatText(keyScene{chat: chat}) {
		t.Error("an empty chat offers a reply to copy")
	}
	model.readKey(tea.Key{Code: 'a', Mod: uv.ModCtrl})
	if !strings.Contains(chat.Notice, "no reply yet") {
		t.Errorf("the panel reports %q", chat.Notice)
	}

	chat.Messages = []app.ChatMessage{
		{Role: hist.ChatRoleUser, Content: "which table holds the orders"},
		{Role: hist.ChatRoleAssistant, Content: "public.orders holds one row per order."},
	}
	if !holdsChatText(keyScene{chat: chat}) {
		t.Error("a chat with a reply offers none to copy")
	}
	model.readKey(tea.Key{Code: 'a', Mod: uv.ModCtrl})
	if !strings.Contains(chat.Notice, "on the clipboard") {
		t.Errorf("the panel reports %q", chat.Notice)
	}
}
