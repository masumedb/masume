package ui

import (
	"context"
	"testing"
	"time"

	"github.com/masumedb/masume/internal/ai"
	"github.com/masumedb/masume/internal/app"
)

// A chat still asking the model holds the session and writes into a channel nobody reads
// once the connection is gone. Its goroutine fills that channel and then waits for ever, so
// closing the connection has to stop the run.
func TestClosingAConnectionStopsItsChat(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()

	stopped := false
	run, events := connection.Chat.Begin(func() { stopped = true })
	if run == 0 || events == nil {
		t.Fatal("the chat did not open a run")
	}
	if connection.Chat.Status != app.ChatStreaming {
		t.Fatalf("the chat reads %q, wanted it streaming", connection.Chat.Status)
	}

	model.closeActiveConnection()

	if !stopped {
		t.Error("the connection closed and its chat was left asking")
	}
	if connection.Chat.Status != app.ChatIdle {
		t.Errorf("the chat reads %q after the close", connection.Chat.Status)
	}
}

// The same holds when the program ends: every chat is stopped before its session closes.
func TestShuttingDownStopsEveryChat(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()

	stopped := false
	connection.Chat.Begin(func() { stopped = true })

	model.shutDown()

	if !stopped {
		t.Error("the program ended and a chat was left asking")
	}
}

// streamingResponder writes text until its run is cancelled.
type streamingResponder struct{}

func (streamingResponder) Describe() string { return "stream" }

func (streamingResponder) Reply(
	ctx context.Context, _ ai.Request, hooks ai.RunHooks,
) (ai.RunResult, error) {
	for ctx.Err() == nil {
		hooks.AppendText("x")
	}
	return ai.RunResult{}, ctx.Err()
}

func TestCancelledChatEndsWithAFullChannel(t *testing.T) {
	events := make(chan app.ChatEvent, 1)
	ctx, stop := context.WithCancel(context.Background())
	ended := make(chan struct{})
	go func() {
		runChatReply(ctx, chatRun{
			responder: streamingResponder{}, events: events, close: func() {},
		})
		close(ended)
	}()
	<-events
	stop()

	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("the cancelled chat still waits to send an event")
	}
}
