package app

import (
	"regexp"
	"slices"
	"time"

	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/hist"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/query"
	"github.com/masumedb/masume/internal/writeplan"
)

// Connection chat state and stored conversations. The UI handles model requests and passes response events to this state.

// ChatStatus is the response state.
type ChatStatus string

// The three states the chat can be in.
const (
	ChatIdle      ChatStatus = "idle"
	ChatStreaming ChatStatus = "streaming"
	ChatFailed    ChatStatus = "failed"
)

// ChatMessage is one turn of the conversation.
type ChatMessage struct {
	Role    string
	Content string
	// Parts is the reply in the order it arrived: blocks of text and the calls between
	// them. Content is the text of those blocks, which is what a later question sends.
	Parts []ChatPart
	// Context is the editor snapshot sent with the question and omitted from display.
	Context string
}

// ChatPart is one piece of a reply: a block of text, or one call the model made.
type ChatPart struct {
	// Step is the call this part is. A part with none is a block of text.
	Step string
	Text string
	// Done is true for a call that finished.
	Done bool
}

// IsStep is true for a part that is a call rather than text.
func (part ChatPart) IsStep() bool { return part.Step != "" }

// ChatUsage is the connection token usage for the current client session.
type ChatUsage struct {
	InputTokens  int
	OutputTokens int
	// CachedInputTokens is the cached portion of InputTokens.
	CachedInputTokens int
}

// Add sums what a run spent into the total of the connection.
func (usage ChatUsage) Add(spent ChatUsage) ChatUsage {
	return ChatUsage{
		InputTokens:       usage.InputTokens + spent.InputTokens,
		OutputTokens:      usage.OutputTokens + spent.OutputTokens,
		CachedInputTokens: usage.CachedInputTokens + spent.CachedInputTokens,
	}
}

// PendingRun is a statement the chat wants to run, waiting for the user to allow it.
type PendingRun struct {
	// Summary is the statement risk and environment.
	Summary string
	SQL     string
	// The measured write plan as text, or empty when unavailable.
	Plan []string
}

// ChatEventKind is the response event category.
type ChatEventKind string

// The things a run reports.
const (
	// ChatTextStarted opens a block of text that follows an earlier one.
	ChatTextStarted  ChatEventKind = "text-started"
	ChatTextArrived  ChatEventKind = "text"
	ChatStepStarted  ChatEventKind = "step-started"
	ChatStepFinished ChatEventKind = "step-finished"
	// ChatRunAsked asks the user whether a statement may run.
	ChatRunAsked ChatEventKind = "run-asked"
	// ChatTableRead marks a relation the chat described as read in the tree as well.
	ChatTableRead ChatEventKind = "table-read"
	// ChatUndoKept hands over the undo of a write that ran.
	ChatUndoKept ChatEventKind = "undo-kept"
	// ChatEnded is the final event with usage and error details.
	ChatEnded ChatEventKind = "ended"
)

// ChatEvent is one thing a run reported.
type ChatEvent struct {
	// Run is the response sequence for rejecting stale events.
	Run  int
	Kind ChatEventKind
	Text string
	Ask  PendingRun
	// Allowed carries the answer of the user back to the statement that waits for it.
	Allowed chan bool
	Usage   ChatUsage
	Problem string
	// The relation the chat described, and what the server said about it.
	Table  db.TableRef
	Detail db.TableDetail
	Undo   writeplan.Undo
}

// Chat is the chat of one connection.
type Chat struct {
	Messages []ChatMessage
	Status   ChatStatus
	Problem  string
	// Activity is the call that runs now. The calls that finished are in the reply.
	Activity string
	Usage    ChatUsage
	// Pending is the statement awaiting confirmation, or nil.
	Pending *PendingRun
	// allowed is where the answer to the waiting statement goes.
	allowed chan bool

	// Conversations is the profile history, newest first. OpenID is zero until the conversation has a stored turn.
	Conversations []hist.ChatConversation
	OpenID        int64

	// Run is the current response sequence.
	Run int
	// stop ends the run that writes now.
	stop func()

	// TurnAt is the selected turn. HasTurn is false before the first turn selection.
	TurnAt  int
	HasTurn bool
	// Offset is the scroll position. Follow is true while the newest row remains visible.
	Offset int
	Follow bool
	// StartedAt is the response start time.
	StartedAt time.Time
	// True while the reply of this run becomes a notebook, and what it is to cover.
	BuildsNotebook  bool
	NotebookSubject string
	// Notice is the line under the field, in place of what the chat spent.
	Notice string
}

// NewChat builds the chat of one connection.
func NewChat() *Chat {
	return &Chat{Status: ChatIdle, Follow: true}
}

// IsStreaming is true while a reply is being written.
func (chat *Chat) IsStreaming() bool {
	return chat.Status == ChatStreaming
}

// StartTurn appends a question and an empty reply. The request includes editor context only when the context changes.
func (chat *Chat) StartTurn(prompt, context string) []ChatMessage {
	sent := ""
	for _, message := range chat.Messages {
		if message.Context != "" {
			sent = message.Context
		}
	}
	asked := ChatMessage{Role: hist.ChatRoleUser, Content: prompt}
	if context != "" && context != sent {
		asked.Context = context
	}

	// Cap request history before the empty reply.
	held := make([]ChatMessage, 0, len(chat.Messages)+2)
	held = append(append(held, chat.Messages...), asked)
	history := held[:len(held):len(held)]
	chat.Messages = append(held, ChatMessage{Role: hist.ChatRoleAssistant})
	return history
}

// AppendDelta writes what arrived into the block of text at the end of the reply.
func (chat *Chat) AppendDelta(delta string) {
	chat.writeReply(func(reply ChatMessage) (ChatMessage, bool) {
		reply.Content += delta
		at := len(reply.Parts) - 1
		if at < 0 || reply.Parts[at].IsStep() {
			reply.Parts = append(reply.Parts, ChatPart{Text: delta})
			return reply, true
		}
		reply.Parts[at].Text += delta
		return reply, true
	})
}

// StartTextBlock separates response text blocks when the previous text has no trailing whitespace.
func (chat *Chat) StartTextBlock() {
	chat.writeReply(func(reply ChatMessage) (ChatMessage, bool) {
		if reply.Content == "" || endsInBlank.MatchString(reply.Content) {
			return reply, false
		}
		reply.Content += "\n\n"
		at := len(reply.Parts) - 1
		if at >= 0 && !reply.Parts[at].IsStep() {
			reply.Parts[at].Text += "\n\n"
		}
		return reply, true
	})
}

// endsInBlank matches a reply that already ends in a blank.
var endsInBlank = regexp.MustCompile(`\s$`)

// DropEmptyReply removes a trailing empty assistant message.
func (chat *Chat) DropEmptyReply() {
	if len(chat.Messages) == 0 {
		return
	}
	last := chat.Messages[len(chat.Messages)-1]
	if last.Role == hist.ChatRoleAssistant && last.Content == "" && len(last.Parts) == 0 {
		chat.Messages = chat.Messages[:len(chat.Messages)-1]
	}
}

// writeReply updates the final assistant message when present.
func (chat *Chat) writeReply(rewrite func(reply ChatMessage) (ChatMessage, bool)) {
	if len(chat.Messages) == 0 {
		return
	}
	at := len(chat.Messages) - 1
	if chat.Messages[at].Role != hist.ChatRoleAssistant {
		return
	}
	written, changed := rewrite(chat.Messages[at])
	if changed {
		chat.Messages[at] = written
	}
}

// StartStep puts the call that starts into the reply, where it happened.
func (chat *Chat) StartStep(label string) {
	chat.Activity = label
	chat.writeReply(func(reply ChatMessage) (ChatMessage, bool) {
		reply.Parts = append(reply.Parts, ChatPart{Step: label})
		return reply, true
	})
}

// FinishStep marks the call at the end of the reply as one that finished.
func (chat *Chat) FinishStep() {
	chat.Activity = ""
	chat.writeReply(func(reply ChatMessage) (ChatMessage, bool) {
		at := len(reply.Parts) - 1
		if at < 0 || !reply.Parts[at].IsStep() || reply.Parts[at].Done {
			return reply, false
		}
		reply.Parts[at].Done = true
		return reply, true
	})
}

// ClearActivity drops the call that was running, which no longer is.
func (chat *Chat) ClearActivity() {
	chat.Activity = ""
}

// DropUnfinishedStep removes a call the reply never finished, which a stopped run leaves.
func (chat *Chat) DropUnfinishedStep() {
	chat.writeReply(func(reply ChatMessage) (ChatMessage, bool) {
		at := len(reply.Parts) - 1
		if at < 0 || !reply.Parts[at].IsStep() || reply.Parts[at].Done {
			return reply, false
		}
		reply.Parts = reply.Parts[:at]
		return reply, true
	})
}

// Ask keeps the statement that waits for a yes.
func (chat *Chat) Ask(pending PendingRun, allowed chan bool) {
	held := pending
	chat.Pending, chat.allowed = &held, allowed
}

// AnswerPending sends the confirmation response and clears the pending request.
func (chat *Chat) AnswerPending(confirmed bool) {
	if chat.allowed == nil {
		return
	}
	chat.allowed <- confirmed
	chat.Pending, chat.allowed = nil, nil
}

// Fail reports why the reply stopped.
func (chat *Chat) Fail(problem string) {
	chat.DropEmptyReply()
	chat.Problem = problem
	chat.Status = ChatFailed
}

// Stopped ends the run that writes now, and keeps what it had written.
func (chat *Chat) Stopped() {
	// Refuse pending execution before cancelling the response.
	chat.AnswerPending(false)
	if chat.stop != nil {
		chat.stop()
		chat.stop = nil
	}
	chat.Run++
	// A call that never finished is not a call the reply made.
	chat.DropUnfinishedStep()
	chat.DropEmptyReply()
	chat.ClearActivity()
	chat.Status = ChatIdle
}

// Begin starts a response and returns its sequence and event channel.
func (chat *Chat) Begin(stop func()) (int, chan ChatEvent) {
	chat.Run++
	chat.stop = stop
	chat.StartedAt = time.Now()
	chat.Follow = true
	chat.Notice = ""
	chat.Problem = ""
	chat.ClearActivity()
	chat.Status = ChatStreaming
	// Each response has a separate event channel.
	return chat.Run, make(chan ChatEvent, ChatEventRoom)
}

// ChatEventRoom is the response event buffer capacity.
const ChatEventRoom = 256

// End closes the run, and does nothing for a run that is not the one writing.
func (chat *Chat) End(run int) {
	if run != chat.Run {
		return
	}
	chat.stop = nil
	chat.FinishStep()
}

// OpenConversation puts a conversation read from the file on screen.
func (chat *Chat) OpenConversation(id int64, turns []hist.ChatTurn) {
	chat.Messages = make([]ChatMessage, 0, len(turns))
	for _, turn := range turns {
		chat.Messages = append(chat.Messages, ChatMessage{
			Role: turn.Role, Content: turn.Content, Context: turn.Context,
		})
	}
	chat.OpenID = id
	chat.TurnAt, chat.HasTurn, chat.Offset = 0, false, 0
}

// WriteTurns returns the turns of the conversation as the file keeps them.
func (chat *Chat) WriteTurns() []hist.ChatTurn {
	turns := make([]hist.ChatTurn, 0, len(chat.Messages))
	for _, message := range chat.Messages {
		turns = append(turns, hist.ChatTurn{
			Role: message.Role, Content: message.Content, Context: message.Context,
		})
	}
	return turns
}

// FindLastQuestion returns the last question of the conversation, and whether it has one.
func (chat *Chat) FindLastQuestion() (string, bool) {
	for at := len(chat.Messages) - 1; at >= 0; at-- {
		if chat.Messages[at].Role == hist.ChatRoleUser {
			return chat.Messages[at].Content, true
		}
	}
	return "", false
}

// FindLastReply returns what the model last wrote, and whether it wrote anything.
func (chat *Chat) FindLastReply() (string, bool) {
	for _, v := range slices.Backward(chat.Messages) {
		if v.Role == hist.ChatRoleAssistant {
			return v.Content, true
		}
	}
	return "", false
}

// FindLastQuery returns the most recent statement of the conversation, and whether it has
// one. A reply that answered in prose does not hide the query of the reply before it.
func (chat *Chat) FindLastQuery() (string, bool) {
	for _, held := range slices.Backward(chat.Messages) {
		if held.Role != hist.ChatRoleAssistant {
			continue
		}
		if sql, wrote := query.FindSQLBlock(held.Content); wrote {
			return sql, true
		}
	}
	return "", false
}

// DescribeUsage summarizes session token usage, or returns an empty string before usage is recorded.
func (chat *Chat) DescribeUsage() string {
	if chat.Usage.InputTokens == 0 && chat.Usage.OutputTokens == 0 {
		return ""
	}
	written := present.FormatCount(int64(chat.Usage.InputTokens)) + " in"
	if chat.Usage.CachedInputTokens > 0 {
		written += " (" + present.FormatCount(int64(chat.Usage.CachedInputTokens)) + " cached)"
	}
	return written + " / " +
		present.FormatCount(int64(chat.Usage.OutputTokens)) + " out this session"
}
