package app

import (
	"testing"

	"github.com/masumedb/masume/internal/hist"
)

func TestStartTurnSendsTheEditorOnce(t *testing.T) {
	chat := NewChat()

	history := chat.StartTurn("first", "the editor holds this")
	if len(history) != 1 || history[0].Context != "the editor holds this" {
		t.Fatalf("the first question reads %+v", history)
	}
	if len(chat.Messages) != 2 || chat.Messages[1].Role != hist.ChatRoleAssistant {
		t.Fatalf("the turns read %+v", chat.Messages)
	}

	// The same contents are already in the conversation, so they do not ride again.
	chat.AppendDelta("an answer")
	history = chat.StartTurn("second", "the editor holds this")
	if history[len(history)-1].Context != "" {
		t.Errorf("the editor was sent twice: %+v", history[len(history)-1])
	}

	// Contents that changed ride with the question.
	chat.AppendDelta("another answer")
	history = chat.StartTurn("third", "the editor holds something else")
	if history[len(history)-1].Context != "the editor holds something else" {
		t.Errorf("contents that changed were not sent: %+v", history[len(history)-1])
	}
}

func TestAReplyIsWrittenAsItArrives(t *testing.T) {
	chat := NewChat()
	chat.StartTurn("a question", "")

	// The first block opens the reply, so nothing is put before it.
	chat.StartTextBlock()
	chat.AppendDelta("Let me look.")
	// A block that follows text is parted from it.
	chat.StartTextBlock()
	chat.AppendDelta("Two tables.\n")
	// A block that follows a blank is not parted again.
	chat.StartTextBlock()
	chat.AppendDelta("And two rows.")

	reply, found := chat.FindLastReply()
	if !found || reply != "Let me look.\n\nTwo tables.\nAnd two rows." {
		t.Errorf("the reply reads %q", reply)
	}
}

func TestAFailedRunLeavesNoEmptyTurn(t *testing.T) {
	chat := NewChat()
	chat.StartTurn("a question", "")
	chat.Fail("the provider said no")

	if len(chat.Messages) != 1 || chat.Messages[0].Role != hist.ChatRoleUser {
		t.Errorf("the turns read %+v", chat.Messages)
	}
	if chat.Status != ChatFailed || chat.Problem != "the provider said no" {
		t.Errorf("the chat reads %q and %q", chat.Status, chat.Problem)
	}
}

// A reply holds its calls where they happened, between the blocks of text around them, so
// the turn reads in the order the model worked.
func TestAReplyHoldsItsCallsWhereTheyHappened(t *testing.T) {
	chat := NewChat()
	chat.StartTurn("how many orders are unpaid", "")

	chat.AppendDelta("Let me look.")
	chat.StartStep("listing the tables")
	chat.FinishStep()
	chat.StartTextBlock()
	chat.AppendDelta("Two tables.")
	chat.StartStep("reading the columns of orders")

	reply := chat.Messages[len(chat.Messages)-1]
	if len(reply.Parts) != 4 {
		t.Fatalf("the reply holds %+v", reply.Parts)
	}
	// A block that follows a call needs no blank line, because the call separates them.
	for at, wanted := range []ChatPart{
		{Text: "Let me look."},
		{Step: "listing the tables", Done: true},
		{Text: "Two tables."},
		{Step: "reading the columns of orders"},
	} {
		if reply.Parts[at] != wanted {
			t.Errorf("part %d reads %+v, wanted %+v", at, reply.Parts[at], wanted)
		}
	}
	// The text of the reply is what a later question sends, and holds no call.
	if reply.Content != "Let me look.\n\nTwo tables." {
		t.Errorf("the text reads %q", reply.Content)
	}
	if chat.Activity != "reading the columns of orders" {
		t.Errorf("the call that runs reads %q", chat.Activity)
	}
}

// A call the reply never finished is not a call it made, so a stopped run drops it.
func TestAStoppedRunDropsTheCallThatNeverFinished(t *testing.T) {
	chat := NewChat()
	chat.StartTurn("how many orders are unpaid", "")
	chat.Begin(func() {})
	chat.AppendDelta("Let me look.")
	chat.StartStep("listing the tables")
	chat.FinishStep()
	chat.StartStep("reading the columns of orders")

	chat.Stopped()
	reply := chat.Messages[len(chat.Messages)-1]
	if len(reply.Parts) != 2 {
		t.Fatalf("the reply holds %+v", reply.Parts)
	}
	if !reply.Parts[1].Done {
		t.Errorf("the call that finished reads %+v", reply.Parts[1])
	}
	if chat.Activity != "" {
		t.Errorf("a call still runs: %q", chat.Activity)
	}
}

// A reply that made a call and wrote nothing is kept, because the call is what it did.
func TestAReplyWithACallIsKept(t *testing.T) {
	chat := NewChat()
	chat.StartTurn("how many orders are unpaid", "")
	chat.StartStep("listing the tables")
	chat.FinishStep()

	chat.DropEmptyReply()
	if len(chat.Messages) != 2 {
		t.Errorf("the turns read %+v", chat.Messages)
	}
}

func TestAnsweringTheWaitingStatement(t *testing.T) {
	chat := NewChat()
	allowed := make(chan bool, 1)
	chat.Ask(PendingRun{Summary: "writes to the database on prod", SQL: "delete from t"}, allowed)
	if chat.Pending == nil || chat.Pending.SQL != "delete from t" {
		t.Fatalf("the waiting statement reads %+v", chat.Pending)
	}

	chat.AnswerPending(true)
	if !<-allowed {
		t.Error("the answer did not reach the statement")
	}
	if chat.Pending != nil {
		t.Error("the statement is still waiting")
	}
	// A second answer reaches nobody, and must not block.
	chat.AnswerPending(false)
}

func TestStoppingARunKeepsWhatArrived(t *testing.T) {
	chat := NewChat()
	stopped := false
	run, events := chat.Begin(func() { stopped = true })
	if run == 0 || events == nil || chat.Status != ChatStreaming {
		t.Fatalf("the run opened as %d in state %q", run, chat.Status)
	}
	chat.StartTurn("a question", "")
	chat.AppendDelta("half an answer")

	chat.Stopped()
	if !stopped {
		t.Error("the run was not told to stop")
	}
	if chat.Status != ChatIdle || chat.Run == run {
		t.Errorf("the chat reads %q and run %d", chat.Status, chat.Run)
	}
	if reply, _ := chat.FindLastReply(); reply != "half an answer" {
		t.Errorf("what arrived was dropped: %q", reply)
	}
	// An event of the run that was stopped closes nothing.
	chat.End(run)
	if chat.Status != ChatIdle {
		t.Errorf("the old run changed the chat to %q", chat.Status)
	}
}

func TestDescribeUsage(t *testing.T) {
	chat := NewChat()
	if said := chat.DescribeUsage(); said != "" {
		t.Errorf("a chat that spent nothing says %q", said)
	}
	chat.Usage = ChatUsage{InputTokens: 1234567, OutputTokens: 55}
	if said := chat.DescribeUsage(); said != "1,234,567 in / 55 out this session" {
		t.Errorf("the line reads %q", said)
	}
	chat.Usage = chat.Usage.Add(ChatUsage{InputTokens: 100, CachedInputTokens: 1400})
	if said := chat.DescribeUsage(); said !=
		"1,234,667 in (1,400 cached) / 55 out this session" {
		t.Errorf("the line reads %q", said)
	}
}

func TestConversationsGoToTheFileAndBack(t *testing.T) {
	chat := NewChat()
	chat.StartTurn("a question", "the editor")
	chat.AppendDelta("an answer")

	turns := chat.WriteTurns()
	if len(turns) != 2 || turns[0].Context != "the editor" ||
		turns[1].Content != "an answer" {
		t.Fatalf("the turns read %+v", turns)
	}

	chat.OpenConversation(7, turns)
	if chat.OpenID != 7 || len(chat.Messages) != 2 {
		t.Errorf("the conversation on screen reads %d with %d turns",
			chat.OpenID, len(chat.Messages))
	}
	if chat.HasTurn || chat.Offset != 0 {
		t.Error("the jump of the last conversation was kept")
	}
}
