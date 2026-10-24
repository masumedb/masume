package ui

import (
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
)

// narrowestActivityRows is the least rows the server activity keeps for its sessions.
const narrowestActivityRows = 8

// The columns of the dashboard: the meter of the connections, and the clock at the right of
// a row of the blocking tree.
const (
	dashboardMeterWidth = 10
	dashboardGap        = "   "
	// dashboardBusyShare is the share of the connection limit that reads as a warning.
	dashboardBusyShare = 0.8
)

// renderActivity draws what the server is doing: the load it is under, the sessions waiting
// for a lock, and every session it holds.
func (model *Model) renderActivity(
	connection *app.Connection, overlay app.Overlay, width int,
) string {
	sessions := app.ListShownSessions(overlay.Sessions, overlay.View)
	labels := model.buildActivityHeadings(overlay.View)
	columns := measureActivityColumns(sessions, labels)
	rows := make([]string, 0, len(sessions))
	for at, session := range sessions {
		rows = append(rows, model.renderListRow(ListRowSpec{
			Label: present.FitTextRight(strconv.FormatInt(session.PID, 10), columns.pid) + "  " +
				present.FitText(session.State, columns.state),
			LabelWidth: columns.pid + 2 + columns.state + 2,
			Detail: present.FitTextRight(core.FormatClock(session.Duration), columns.time) + "  " +
				present.FitText(describeActivityUser(session), columns.user) + "  " +
				core.CollapseWhitespace(session.Query),
			Selected: at == overlay.List.Cursor, Destructive: session.State == "active",
			Width: width,
		}))
	}

	profile := connection.Profile()
	keys := model.buildCardKeys(app.OverlayActivity, keyScene{overlay: overlay})
	header := model.buildDashboardHeader(overlay, width)
	model.layout.activityHeadings = nil
	if len(overlay.Sessions) > 0 {
		header = append(header, model.renderActivityHeading(
			overlay.View, labels, columns, width, cardListRow+1+len(header)))
	}

	return model.renderListCard(ListCard{
		Kind:   app.OverlayActivity,
		Title:  " " + buildDashboardTitle(profile) + " ",
		Note:   model.renderServerUptime(overlay.Server),
		Header: header,
		Rows:   rows,
		Cursor: overlay.List.Cursor, Offset: overlay.List.Offset,
		Rolled: overlay.List.Rolled, Width: width,
		EmptyReport: describeEmptySessions(overlay),
		Keys:        keys,
		// A floor under the rows, so the card does not resize on every refresh while
		// the sessions of a quiet server come and go.
		ContentRows: max(len(rows), narrowestActivityRows) + len(header),
	})
}

// activityColumns are the widths of the columns of the session list.
type activityColumns struct {
	pid, state, time, user int
}

// buildActivityHeadings returns the heading of each column of the session list, the sorted
// one with the mark of its direction.
func (model *Model) buildActivityHeadings(view app.DashboardView) map[app.ActivityColumn]string {
	labels := map[app.ActivityColumn]string{
		app.ActivityByPID: "pid", app.ActivityByState: "state", app.ActivityByTime: "time",
		app.ActivityByUser: "user@app", app.ActivityByStatement: "statement",
	}
	if view.SortBy != app.ActivityByServer {
		mark := model.icons.Icon(cfg.IconSortUp)
		if view.Descending {
			mark = model.icons.Icon(cfg.IconSortDown)
		}
		labels[view.SortBy] += " " + mark
	}
	return labels
}

// renderActivityHeading draws the row of column names over the session list, and records
// where each name is drawn, so a press on one sorts by it. The row is counted from the card.
func (model *Model) renderActivityHeading(
	view app.DashboardView, labels map[app.ActivityColumn]string, columns activityColumns,
	width, row int,
) string {
	theme := model.styles.Theme
	parts := []struct {
		column app.ActivityColumn
		text   string
	}{
		{app.ActivityByPID, present.FitTextRight(labels[app.ActivityByPID], columns.pid)},
		{app.ActivityByState, present.FitText(labels[app.ActivityByState], columns.state)},
		{app.ActivityByTime, present.FitTextRight(labels[app.ActivityByTime], columns.time)},
		{app.ActivityByUser, present.FitText(labels[app.ActivityByUser], columns.user)},
		{app.ActivityByStatement, labels[app.ActivityByStatement]},
	}
	pad := paintOn(theme.Panel, " ")
	written := pad + paintOn(theme.Panel, strings.Repeat(" ", rowPaddingLeft))
	at := cardBodyColumn + rowPaddingLeft
	for index, part := range parts {
		if index > 0 {
			written += paintOn(theme.Panel, "  ")
			at += 2
		}
		ink := theme.Muted
		if part.column == view.SortBy {
			ink = theme.Accent
		}
		written += paintText(ink, theme.Panel, part.text)
		size := present.MeasureText(part.text)
		model.layout.activityHeadings = append(model.layout.activityHeadings, chipHit{
			index: slices.Index(app.ActivityColumns, part.column), row: row,
			from: at, to: at + size - 1,
		})
		at += size
	}
	return padStyledOn(written, width-2, theme.Panel) + pad
}

// describeEmptySessions returns what the session list says while it shows no session.
func describeEmptySessions(overlay app.Overlay) string {
	if overlay.View.HidesIdle && len(overlay.Sessions) > 0 {
		return "every other session is idle"
	}
	return "no other sessions"
}

// measureActivityColumns returns the widths that hold the widest value and heading of each
// column of the session list.
func measureActivityColumns(
	sessions []db.Activity, labels map[app.ActivityColumn]string,
) activityColumns {
	columns := activityColumns{
		pid:   present.MeasureText(labels[app.ActivityByPID]),
		state: present.MeasureText(labels[app.ActivityByState]),
		time:  present.MeasureText(labels[app.ActivityByTime]),
		user:  present.MeasureText(labels[app.ActivityByUser]),
	}
	for _, session := range sessions {
		columns.pid = max(columns.pid, len(strconv.FormatInt(session.PID, 10)))
		columns.state = max(columns.state, present.MeasureText(session.State))
		columns.time = max(columns.time, present.MeasureText(core.FormatClock(session.Duration)))
		columns.user = min(max(columns.user, present.MeasureText(describeActivityUser(session))),
			activityUserWidth)
	}
	return columns
}

// activityUserWidth is the widest user@app column the session list draws.
const activityUserWidth = 28

// describeActivityUser returns the user and the application of a session.
func describeActivityUser(session db.Activity) string {
	application := session.ApplicationName
	if application == "" {
		application = "?"
	}
	return session.User + "@" + application
}

// buildDashboardTitle names the connection the card is watching, its environment, and how
// often it refreshes.
func buildDashboardTitle(profile cfg.Profile) string {
	text := "server activity · " + present.SafeText(profile.Name)
	if profile.Environment != "" {
		text += " · " + string(profile.Environment)
	}
	return text + " · refresh every " + core.FormatLargestUnit(dashboardRefreshWait)
}

// renderServerUptime returns how long the server has been up, for the right of the title.
func (model *Model) renderServerUptime(reading app.ServerReading) string {
	if !reading.HasLoad || reading.Load.StartedAt.IsZero() {
		return ""
	}
	return model.styles.Muted().Render(
		" uptime " + core.FormatLargestUnit(time.Since(reading.Load.StartedAt)) + " ")
}

// buildDashboardHeader returns the lines over the list: what the server is carrying, and
// the sessions waiting for a lock. A panel the server did not answer for is left out.
func (model *Model) buildDashboardHeader(overlay app.Overlay, width int) []string {
	lines := []string{}
	if summary := model.buildDashboardSummary(overlay, width); summary != "" {
		lines = append(lines, summary)
	}
	lines = append(lines, model.buildBlockingPanel(overlay, width)...)
	lines = append(lines, model.buildSlowPanel(overlay, width)...)
	if len(lines) == 0 {
		return lines
	}
	theme := model.styles.Theme
	return append(lines, model.renderPanelLine(
		paintText(theme.Faint, theme.Panel,
			strings.Repeat("─", max(width-present.CardChrome, 0))), width))
}

// buildDashboardSummary returns the one line of what the server is carrying now.
func (model *Model) buildDashboardSummary(overlay app.Overlay, width int) string {
	theme := model.styles.Theme
	text := paintOn(theme.Panel, strings.Repeat(" ", rowPaddingLeft))

	reading := overlay.Server
	if reading.HasLoad {
		load := reading.Load
		text += paintText(theme.Muted, theme.Panel, "connections ")
		text += paintText(model.resolveLoadInk(load), theme.Panel,
			strconv.FormatInt(load.Connections, 10)+"/"+
				strconv.FormatInt(load.MaxConnections, 10))
		text += paintOn(theme.Panel, " ")
		text += paintText(model.resolveLoadInk(load), theme.Panel, present.BuildMeter(
			float64(load.Connections), float64(load.MaxConnections), dashboardMeterWidth))
		text += paintOn(theme.Panel, dashboardGap)
	}

	text += paintText(theme.Muted, theme.Panel, "other sessions ")
	text += paintText(theme.Text, theme.Panel, strconv.Itoa(len(overlay.Sessions)))
	if idle := app.CountIdleSessions(overlay.Sessions); overlay.View.HidesIdle && idle > 0 {
		text += paintText(theme.Muted, theme.Panel, " · "+strconv.Itoa(idle)+" idle hidden")
	}

	if reading.HasLocks {
		waiting := app.CountBlockedSessions(reading.Locks)
		ink := theme.Text
		if waiting > 0 {
			ink = theme.Error
		}
		text += paintOn(theme.Panel, dashboardGap)
		text += paintText(theme.Muted, theme.Panel, "locks ")
		text += paintText(ink, theme.Panel, strconv.Itoa(waiting)+" waiting")
	}
	for _, measure := range model.buildDashboardMeasures(overlay) {
		text += paintOn(theme.Panel, dashboardGap)
		text += paintText(theme.Muted, theme.Panel, measure.label+" ")
		text += paintText(measure.ink, theme.Panel, measure.value)
	}
	return model.renderPanelLine(text, width)
}

// dashboardMeasure is one number of the summary: what it is called, what it reads, and how
// strongly it is drawn.
type dashboardMeasure struct {
	label string
	value string
	ink   color.Color
}

const (
	poorCacheHitRate = 0.90
	fairCacheHitRate = 0.99
)

const (
	fairReplicationLag = 5 * time.Second
	poorReplicationLag = time.Minute
)

// buildDashboardMeasures returns the numbers of the summary the server answered for. A rate
// is left out until there are two readings to measure it between.
func (model *Model) buildDashboardMeasures(overlay app.Overlay) []dashboardMeasure {
	theme := model.styles.Theme
	reading := overlay.Server
	if !reading.HasLoad {
		return nil
	}
	load := reading.Load
	measures := []dashboardMeasure{}

	span := reading.ReadAt.Sub(overlay.View.PreviousAt)
	rates := overlay.View.HasPrevious && load.HasCounters && overlay.View.Previous.HasCounters
	if rates {
		if rate, held := app.ResolveCounterRate(
			overlay.View.Previous.Transactions, load.Transactions, span); held {
			measures = append(measures, dashboardMeasure{
				label: "txn/s", value: core.FormatRate(rate), ink: theme.Text,
			})
		}
		if rate, held := app.ResolveCounterRate(
			overlay.View.Previous.WalBytes, load.WalBytes, span); held && load.HasWalBytes {
			measures = append(measures, dashboardMeasure{
				label: "wal", value: core.FormatByteRate(rate), ink: theme.Text,
			})
		}
	}
	if load.HasCacheHitRate {
		measures = append(measures, dashboardMeasure{
			label: "cache", value: core.FormatShare(load.CacheHitRate),
			ink: model.resolveCacheInk(load.CacheHitRate),
		})
	}
	if load.HasReplicationLag {
		measures = append(measures, dashboardMeasure{
			label: "lag", value: core.FormatLargestUnit(load.ReplicationLag),
			ink: model.resolveLagInk(load.ReplicationLag),
		})
	}
	if load.HasCounters {
		ink := theme.Text
		if load.TempFiles > 0 {
			ink = theme.Warning
		}
		measures = append(measures, dashboardMeasure{
			label: "temp files", value: present.FormatCount(load.TempFiles), ink: ink,
		})
	}
	return measures
}

// resolveCacheInk returns the colour of the cache hit rate.
func (model *Model) resolveCacheInk(share float64) color.Color {
	theme := model.styles.Theme
	switch {
	case share < poorCacheHitRate:
		return theme.Error
	case share < fairCacheHitRate:
		return theme.Warning
	}
	return theme.Text
}

// resolveLagInk returns the colour of the replication lag.
func (model *Model) resolveLagInk(lag time.Duration) color.Color {
	theme := model.styles.Theme
	switch {
	case lag >= poorReplicationLag:
		return theme.Error
	case lag >= fairReplicationLag:
		return theme.Warning
	}
	return theme.Text
}

// resolveLoadInk returns the colour of the connection count, which reads as a warning near
// the limit and as a fault at it.
func (model *Model) resolveLoadInk(load db.ServerLoad) color.Color {
	theme := model.styles.Theme
	if load.MaxConnections <= 0 {
		return theme.Text
	}
	switch share := float64(load.Connections) / float64(load.MaxConnections); {
	case share >= 1:
		return theme.Error
	case share >= dashboardBusyShare:
		return theme.Warning
	}
	return theme.Text
}

// buildBlockingPanel returns the panel of sessions waiting for a lock, and none where
// no session waits.
func (model *Model) buildBlockingPanel(overlay app.Overlay, width int) []string {
	reading := overlay.Server
	if !reading.HasLocks || len(reading.Locks) == 0 {
		return nil
	}
	theme := model.styles.Theme
	folded := overlay.View.IsPanelFolded(app.PanelBlocking)

	heading := paintOn(theme.Panel, strings.Repeat(" ", rowPaddingLeft))
	heading += paintText(theme.Error, theme.Panel, model.readFoldMark(folded)+" blocking tree")
	if folded {
		heading += paintText(theme.Muted, theme.Panel, " · "+
			strconv.Itoa(app.CountBlockedSessions(reading.Locks))+" waiting")
		return []string{model.renderPanelLine(heading, width)}
	}

	nodes := app.BuildBlockingTree(reading.Locks)
	depths := make([]int, 0, len(nodes))
	for _, node := range nodes {
		depths = append(depths, node.Depth)
	}
	guides := present.BuildGuidesForDepths(depths)

	lines := []string{model.renderPanelLine(heading, width)}
	for at, node := range nodes {
		lines = append(lines, model.renderBlockingRow(node, guides[at], width))
	}
	return lines
}

// slowStatementRows is how many statements the panel draws.
const slowStatementRows = 5

// buildSlowPanel returns the panel of statements the server spends its time in.
func (model *Model) buildSlowPanel(overlay app.Overlay, width int) []string {
	reading := overlay.Server
	if !reading.HasSlow || len(reading.Slow) == 0 {
		return nil
	}
	theme := model.styles.Theme
	folded := overlay.View.IsPanelFolded(app.PanelSlow)

	heading := paintOn(theme.Panel, strings.Repeat(" ", rowPaddingLeft))
	heading += paintText(theme.Text, theme.Panel,
		model.readFoldMark(folded)+" slowest statements")
	heading += paintText(theme.Muted, theme.Panel, " · by mean time")
	if folded {
		return []string{model.renderPanelLine(heading, width)}
	}

	shown := reading.Slow
	if len(shown) > slowStatementRows {
		shown = shown[:slowStatementRows]
	}
	meanWidth := 0
	for _, held := range shown {
		meanWidth = max(meanWidth, present.MeasureText(core.FormatDuration(held.MeanTime)))
	}

	lines := []string{model.renderPanelLine(heading, width)}
	for _, held := range shown {
		lines = append(lines, model.renderSlowRow(held, meanWidth, width))
	}
	return lines
}

// renderSlowRow draws one statement the server spends its time in: how long it takes on
// average, how often it ran, and the statement itself.
func (model *Model) renderSlowRow(
	held db.StatementStat, meanWidth, width int,
) string {
	theme := model.styles.Theme
	mean := core.FormatDuration(held.MeanTime)
	calls := "×" + present.FormatCount(held.Calls)

	text := paintOn(theme.Panel, strings.Repeat(" ", rowPaddingLeft))
	text += paintText(theme.Text, theme.Panel,
		strings.Repeat(" ", max(meanWidth-present.MeasureText(mean), 0))+mean)
	text += paintText(theme.Muted, theme.Panel, " "+calls+" ")

	room := max(width-present.CardChrome-rowPaddingLeft-meanWidth-
		present.MeasureText(calls)-2-rowScrollbarWidth, 0)
	text += paintText(theme.Muted, theme.Panel, present.FitText(
		present.TruncateText(present.SafeText(
			core.CollapseWhitespace(held.Query)), room), room))
	return model.renderPanelLine(text, width)
}

// renderBlockingRow draws one session of the blocking tree: which session it is, what it is
// running, and how long it has been waiting or holding.
func (model *Model) renderBlockingRow(
	node app.BlockingNode, guide string, width int,
) string {
	theme := model.styles.Theme
	text := paintOn(theme.Panel, strings.Repeat(" ", rowPaddingLeft))
	text += paintText(theme.Muted, theme.Panel, guide)
	text += paintText(theme.Text, theme.Panel, strconv.FormatInt(node.PID, 10)+" ")

	trail := core.FormatClock(node.Elapsed)
	if node.Waiting {
		trail = "waiting " + trail
	} else if mode := core.FormatLockMode(node.Mode); mode != "" {
		trail = mode + " " + trail
	}
	trail = present.SafeText(" " + trail + " ")

	room := max(width-present.CardChrome-rowPaddingLeft-present.MeasureText(guide)-
		present.MeasureText(strconv.FormatInt(node.PID, 10))-1-
		present.MeasureText(trail)-rowScrollbarWidth, 0)
	text += paintText(theme.Muted, theme.Panel, present.FitText(
		present.TruncateText(present.SafeText(
			core.CollapseWhitespace(node.Query)), room), room))

	ink := theme.Warning
	if node.Waiting {
		ink = theme.Error
	}
	text += paintText(ink, theme.Panel, trail)
	return model.renderPanelLine(text, width)
}

// readFoldMark returns the glyph of a panel that is folded away, or of one that is open.
func (model *Model) readFoldMark(folded bool) string {
	if folded {
		return model.icons.Icon(cfg.IconFoldClosed)
	}
	return model.icons.Icon(cfg.IconFoldOpen)
}

// renderPanelLine pads a line of a panel to the width the rows of the list are drawn in.
func (model *Model) renderPanelLine(written string, width int) string {
	pad := paintOn(model.styles.Theme.Panel, " ")
	return pad + padStyledOn(written, width-4, model.styles.Theme.Panel) + pad + pad
}
