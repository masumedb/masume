package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestALongRunPostsANotificationWhileTheTerminalIsNotFocused(t *testing.T) {
	model, connection, tab := buildEditingModel(t, "select 1", 0)
	t.Setenv("TMUX", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("TERM", "foot")
	model.settings.NotifyAfter = 10 * time.Second
	tab.Results.Start([]string{"select 1"}, 100)
	tab.Results.Results()[0].StartedAt = time.Now().Add(-time.Minute)

	if command := model.notifyRunEnd(connection, tab, runSucceeded); command != nil {
		t.Error("a run that ended while the terminal was focused posted a notification")
	}
	model.Update(tea.BlurMsg{})
	command := model.notifyRunEnd(connection, tab, runSucceeded)
	if command == nil {
		t.Fatal("a long run that ended while the terminal was not focused posted nothing")
	}
	if sequence := command().(tea.RawMsg).Msg.(string); !strings.HasPrefix(sequence, "\x1b]777;notify;") {
		t.Errorf("the notification reads %q", sequence)
	}

	tab.Results.Results()[0].StartedAt = time.Now()
	if command := model.notifyRunEnd(connection, tab, runSucceeded); command != nil {
		t.Error("a short run posted a notification")
	}
}

func TestKittyGetsTheNotificationOfItsOwnProtocol(t *testing.T) {
	t.Setenv("TMUX", "")
	t.Setenv("KITTY_WINDOW_ID", "1")
	if sequence := writeNotification("masume · shop", "query 1 finished in 12 s"); !strings.HasPrefix(sequence, "\x1b]99;") {
		t.Errorf("the notification in kitty reads %q", sequence)
	}
}
