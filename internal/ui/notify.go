package ui

import (
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/present"
)

// The two ends of a run a notification reports.
const (
	runSucceeded = false
	runFailed    = true
)

// notifyRunEnd returns the command that posts a desktop notification for a run that ended,
// and nothing while the terminal is focused or the run was shorter than notify_after.
func (model *Model) notifyRunEnd(connection *app.Connection, tab *app.Tab, failed bool) tea.Cmd {
	after := model.settings.NotifyAfter
	results := tab.Results.Results()
	if after <= 0 || !model.terminalBlurred || len(results) == 0 {
		return nil
	}
	elapsed := time.Since(results[0].StartedAt)
	if elapsed < after {
		return nil
	}
	body := tab.Label() + " finished in " + present.FormatDuration(elapsed)
	if failed {
		body = tab.Label() + " failed after " + present.FormatDuration(elapsed)
	}
	return tea.Raw(writeNotification("masume · "+connection.Profile().Name, body))
}

// writeNotification returns the escape sequence that posts a desktop notification: OSC 99 in
// kitty, OSC 777 in other terminals. Inside tmux the sequence is wrapped for passthrough.
func writeNotification(title, body string) string {
	title, body = cleanNotificationText(title), cleanNotificationText(body)
	sequence := "\x1b]777;notify;" + title + ";" + body + "\x1b\\"
	if os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("TERM") == "xterm-kitty" {
		sequence = "\x1b]99;i=masume:d=0;" + title + "\x1b\\" +
			"\x1b]99;i=masume:p=body;" + body + "\x1b\\"
	}
	if os.Getenv("TMUX") != "" {
		return ansi.TmuxPassthrough(sequence)
	}
	return sequence
}

// cleanNotificationText drops the control characters and the field separator of OSC 777.
func cleanNotificationText(text string) string {
	return strings.Map(func(character rune) rune {
		if character < 0x20 || character == 0x7f || character == ';' {
			return ' '
		}
		return character
	}, text)
}
