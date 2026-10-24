package ui

import (
	"context"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
)

// runActivityCardAction runs an action of the server activity card. It reports whether the
// action belonged to the card.
func (model *Model) runActivityCardAction(
	connection *app.Connection, overlay *app.Overlay, match Match,
) (bool, tea.Model, tea.Cmd) {
	switch match.Action {
	case ActionFoldRow, ActionUnfoldRow:
		// Every panel folds together, because the cursor stands in the session list
		// and not on a panel, so there is no one panel the key could mean.
		for _, panel := range app.DashboardPanels {
			overlay.View.FoldPanel(panel, match.Action == ActionFoldRow)
		}
		return true, model, nil
	}
	switch match.Action {
	case ActionSortSessions:
		changeSessionView(overlay, func(view *app.DashboardView) { view.StepSessionSort() })
		return true, model, nil
	case ActionReverseSort:
		changeSessionView(overlay, func(view *app.DashboardView) {
			if view.SortBy != app.ActivityByServer {
				view.Descending = !view.Descending
			}
		})
		return true, model, nil
	case ActionToggleIdle:
		changeSessionView(overlay, func(view *app.DashboardView) {
			view.HidesIdle = !view.HidesIdle
		})
		return true, model, nil
	}
	sessions := app.ListShownSessions(overlay.Sessions, overlay.View)
	if overlay.List.Cursor >= len(sessions) {
		return false, model, nil
	}
	session := sessions[overlay.List.Cursor]
	switch match.Action {
	case ActionStopSession:
		held, command := model.askStopBackend(connection, session, stopStatement)
		return true, held, command
	case ActionListSecondary:
		held, command := model.askStopBackend(connection, session, endSession)
		return true, held, command
	}
	return false, model, nil
}

// changeSessionView changes how the server activity card lists its sessions, and keeps the
// cursor on the session it stood on where that session is still listed.
func changeSessionView(overlay *app.Overlay, change func(*app.DashboardView)) {
	before := app.ListShownSessions(overlay.Sessions, overlay.View)
	change(&overlay.View)
	overlay.List.Cursor = followSessionCursor(before, overlay.List.Cursor,
		app.ListShownSessions(overlay.Sessions, overlay.View))
	overlay.List.Rolled = false
}

// followSessionCursor returns the place in the sessions after a change of the session the
// cursor stood on before it, or the same place where that session is gone.
func followSessionCursor(before []db.Activity, cursor int, after []db.Activity) int {
	if cursor >= 0 && cursor < len(before) {
		pid := before[cursor].PID
		for at, session := range after {
			if session.PID == pid {
				return at
			}
		}
	}
	return clamp(cursor, len(after))
}

// The two ways the server stops a session. Cancelling ends the statement and leaves the
// connection open; ending the session closes the connection with it.
const (
	stopStatement = false
	endSession    = true
)

type stoppedBackendMsg struct {
	ConnectionID int
	PID          int64
	Ended        bool
	Stopped      bool
	Problem      string
}

func stopBackend(id int, session db.ServerAdmin, pid int64, ends bool) tea.Cmd {
	return func() tea.Msg {
		ctx, stop := context.WithTimeout(context.Background(), readTimeout)
		defer stop()

		stopped, err := session.CancelBackend(ctx, pid, ends)
		return stoppedBackendMsg{
			ConnectionID: id, PID: pid, Ended: ends, Stopped: stopped,
			Problem: db.DescribeError(err),
		}
	}
}

func (model *Model) readStopBackendAnswer(answered stoppedBackendMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	named := strconv.FormatInt(answered.PID, 10)
	done := "the statement in session " + named
	if answered.Ended {
		done = "session " + named
	}
	switch {
	case answered.Problem != "":
		connection.ShowError(done + " was not stopped: " + answered.Problem)
	case answered.Stopped:
		connection.Show(done + " was stopped")
	default:
		connection.Show("session " + named + " is no longer on the server")
	}
	return model, nil
}

// askStopBackend asks before another session of the server is stopped. Ending the session
// closes its connection as well, so each way names what it does.
func (model *Model) askStopBackend(
	connection *app.Connection, session db.Activity, ends bool,
) (tea.Model, tea.Cmd) {
	pid := session.PID
	held := connection.Session
	id := model.ActiveID()

	named := strconv.FormatInt(pid, 10)
	title, question, text, yes := " stop the statement ",
		"Stop the statement in session "+named+"?",
		"asked the server to stop the statement in session "+named, "stop statement"
	if ends {
		title, question, text, yes = " end the session ",
			"End session "+named+"? Its statement stops and its connection closes.",
			"asked the server to end session "+named, "end session"
	}

	connection.OpenOver(app.Overlay{
		Kind: app.OverlayConfirm, Title: title, Yes: yes, Destructive: true,
		Body: question + "\n\n" + present.TruncateText(session.Query, 60),
		Answers: app.OverlayAnswers{Answer: func(confirmed bool) app.AnswerCommand {
			if !confirmed {
				return nil
			}
			connection.Show(text)
			return carryAnswer(stopBackend(id, held, pid, ends))
		}},
	})
	return model, nil
}
