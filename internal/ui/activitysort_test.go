package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
)

// shownPIDs returns the PIDs the activity card lists, in order.
func shownPIDs(overlay app.Overlay) []int64 {
	pids := []int64{}
	for _, session := range app.ListShownSessions(overlay.Sessions, overlay.View) {
		pids = append(pids, session.PID)
	}
	return pids
}

func TestTheSortKeyAndAColumnNameSortTheSessions(t *testing.T) {
	model, connection, _ := buildActivityModel(t)
	model.render()

	pressOnCard(t, model, tea.KeyPressMsg{Code: 's', Text: "s"})
	if connection.Overlay.View.SortBy != app.ActivityByPID {
		t.Fatalf("s sorted by %q", connection.Overlay.View.SortBy)
	}

	frame := strings.Split(model.render(), "\n")
	var timeHeading chipHit
	for _, heading := range model.layout.activityHeadings {
		if app.ActivityColumns[heading.index] == app.ActivityByTime {
			timeHeading = heading
		}
	}
	if text := stripEscapes(frame[timeHeading.row]); !strings.Contains(text, "time") {
		t.Fatalf("the heading row reads %q", text)
	}
	clickMouse(model, timeHeading.from, timeHeading.row)
	view := connection.Overlay.View
	if view.SortBy != app.ActivityByTime || !view.Descending {
		t.Errorf("a click on time left the sort %+v", view)
	}
}

func TestHidingIdleSessionsKeepsTheCursorOnItsSession(t *testing.T) {
	model, connection, session := buildActivityModel(t)
	connection.Overlay.Sessions = append(connection.Overlay.Sessions,
		db.Activity{PID: 4600, User: "batch", State: "active", Query: "vacuum"})
	connection.Overlay.List.Cursor = 2
	model.render()

	pressOnCard(t, model, tea.KeyPressMsg{Code: 'i', Text: "i"})
	if got := shownPIDs(connection.Overlay); len(got) != 2 || got[1] != 4600 {
		t.Fatalf("hiding the idle sessions listed %v", got)
	}
	if connection.Overlay.List.Cursor != 1 {
		t.Errorf("the cursor stands on row %d, wanted the session it stood on", connection.Overlay.List.Cursor)
	}
	if text := stripEscapes(model.render()); !strings.Contains(text, "1 idle hidden") {
		t.Error("the summary does not count the hidden idle session")
	}

	// The stop key acts on the session under the cursor in the list as drawn.
	pressOnCard(t, model, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if connection.Overlay.Answers.Answer == nil {
		t.Fatalf("the card is %q and holds no question", connection.Overlay.Kind)
	}
	if started := connection.Overlay.Answers.Answer(true); started != nil {
		started()
	}
	if session.pid != 4600 {
		t.Errorf("the server was asked to stop session %d, wanted 4600", session.pid)
	}
}

func TestARefreshKeepsTheCursorOnItsSession(t *testing.T) {
	model, connection, _ := buildActivityModel(t)
	connection.Overlay.View.SortBy = app.ActivityByTime
	connection.Overlay.View.Descending = true
	connection.Overlay.Sessions = []db.Activity{
		{PID: 1, State: "active", Duration: 3}, {PID: 2, State: "active", Duration: 2},
		{PID: 3, State: "active", Duration: 1},
	}
	connection.Overlay.List.Cursor = 2

	model.Update(activityReadMsg{
		ConnectionID: model.ActiveID(), Refresh: true,
		Sessions: []db.Activity{
			{PID: 1, State: "active", Duration: 3}, {PID: 2, State: "active", Duration: 2},
			{PID: 3, State: "active", Duration: 9},
		},
	})
	if got := shownPIDs(connection.Overlay); got[connection.Overlay.List.Cursor] != 3 {
		t.Errorf("after the refresh the cursor stands on session %d of %v",
			got[connection.Overlay.List.Cursor], got)
	}
}
