package app

import (
	"cmp"
	"slices"
	"strings"

	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
)

// ActivityColumn is a column of the session list of the server activity card.
type ActivityColumn string

// The columns the session list sorts by. ActivityByServer keeps the order of the server.
const (
	ActivityByServer    ActivityColumn = ""
	ActivityByPID       ActivityColumn = "pid"
	ActivityByState     ActivityColumn = "state"
	ActivityByTime      ActivityColumn = "time"
	ActivityByUser      ActivityColumn = "user"
	ActivityByStatement ActivityColumn = "statement"
)

// ActivityColumns are the columns of the session list, in the order they are drawn.
var ActivityColumns = []ActivityColumn{
	ActivityByPID, ActivityByState, ActivityByTime, ActivityByUser, ActivityByStatement,
}

// idleStates are the states an engine reports for a session that waits for a statement:
// PostgreSQL, MySQL and SQL Server in that order. A session idle inside a transaction is not
// one of them.
var idleStates = []string{"idle", "sleep", "sleeping"}

// IsIdleSession is true for a session that waits for its next statement.
func IsIdleSession(session db.Activity) bool {
	return slices.Contains(idleStates, strings.ToLower(strings.TrimSpace(session.State)))
}

// CountIdleSessions returns how many sessions wait for their next statement.
func CountIdleSessions(sessions []db.Activity) int {
	count := 0
	for _, session := range sessions {
		if IsIdleSession(session) {
			count++
		}
	}
	return count
}

// ListShownSessions returns the sessions the card draws, in the order it draws them: without
// the idle ones while they are hidden, and sorted by the column of the view.
func ListShownSessions(sessions []db.Activity, view DashboardView) []db.Activity {
	shown := make([]db.Activity, 0, len(sessions))
	for _, session := range sessions {
		if view.HidesIdle && IsIdleSession(session) {
			continue
		}
		shown = append(shown, session)
	}
	if view.SortBy == ActivityByServer {
		return shown
	}
	slices.SortStableFunc(shown, func(one, other db.Activity) int {
		order := compareSessions(one, other, view.SortBy)
		if view.Descending {
			return -order
		}
		return order
	})
	return shown
}

// compareSessions orders two sessions by one column.
func compareSessions(one, other db.Activity, column ActivityColumn) int {
	switch column {
	case ActivityByPID:
		return cmp.Compare(one.PID, other.PID)
	case ActivityByState:
		return strings.Compare(strings.ToLower(one.State), strings.ToLower(other.State))
	case ActivityByTime:
		return cmp.Compare(one.Duration, other.Duration)
	case ActivityByUser:
		return strings.Compare(
			strings.ToLower(one.User+"@"+one.ApplicationName),
			strings.ToLower(other.User+"@"+other.ApplicationName))
	case ActivityByStatement:
		return strings.Compare(
			strings.ToLower(core.CollapseWhitespace(one.Query)),
			strings.ToLower(core.CollapseWhitespace(other.Query)))
	}
	return 0
}

// SortSessionsBy sorts the session list by a column. A second sort by the same column
// reverses it. The time sorts the longest first, and every other column sorts up.
func (view *DashboardView) SortSessionsBy(column ActivityColumn) {
	if view.SortBy == column && column != ActivityByServer {
		view.Descending = !view.Descending
		return
	}
	view.SortBy, view.Descending = column, column == ActivityByTime
}

// StepSessionSort sorts the session list by the next column, and returns to the order of the
// server after the last one.
func (view *DashboardView) StepSessionSort() {
	at := slices.Index(ActivityColumns, view.SortBy)
	if at+1 >= len(ActivityColumns) {
		view.SortBy, view.Descending = ActivityByServer, false
		return
	}
	next := ActivityColumns[at+1]
	view.SortBy, view.Descending = next, next == ActivityByTime
}
