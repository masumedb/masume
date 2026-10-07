package ui

import (
	"image/color"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/present"
	"github.com/masumedb/masume/internal/resultdiff"
)

// pinnedResult is the result the grid compares other results with: the loaded rows only.
type pinnedResult struct {
	ID        int
	Label     string
	Side      resultdiff.Side
	Truncated bool
}

// rowDiffCache is the last compare and the results it was made from.
type rowDiffCache struct {
	resultID, revision, rows, pinnedID int
	diff                               resultdiff.Diff
}

// readResultSide returns the loaded rows of the active result, and its key where the tab
// reads one table.
func readResultSide(tab *app.Tab) (resultdiff.Side, bool, bool) {
	active := tab.Results.Active()
	if active == nil || active.State.Kind != app.QuerySucceeded {
		return resultdiff.Side{}, false, false
	}
	result := active.State.Result
	side := resultdiff.Side{Rows: result.Rows, Key: tab.Target.KeyColumns}
	for _, column := range result.Columns {
		side.Columns = append(side.Columns, column.Name)
	}
	return side, result.Truncated, true
}

// describeResultLabel returns the label of the active result, such as the table it reads,
// or the tab label where the result has none.
func describeResultLabel(tab *app.Tab) string {
	if active := tab.Results.Active(); active != nil && active.Label != "" {
		return active.Label
	}
	return tab.Label()
}

func (model *Model) pinResult(connection *app.Connection, tab *app.Tab) (tea.Model, tea.Cmd) {
	side, truncated, found := readResultSide(tab)
	if !found {
		return model, nil
	}
	id := 1
	if model.pinned != nil {
		id = model.pinned.ID + 1
	}
	label := describeResultLabel(tab)
	model.pinned = &pinnedResult{ID: id, Label: label, Side: side, Truncated: truncated}
	connection.Show("result pinned: " + present.FormatCountOf(int64(len(side.Rows)), "row", "rows") +
		" of " + label)
	return model, nil
}

func (model *Model) compareWithPinned(connection *app.Connection, tab *app.Tab) (tea.Model, tea.Cmd) {
	if model.pinned == nil {
		connection.Show("pin a result first")
		return model, nil
	}
	if _, _, found := readResultSide(tab); !found {
		return model, nil
	}
	tab.ComparesPinned = true
	tab.View = app.ViewCompare
	tab.DetailOffset = 0
	return model, nil
}

// resolveRowDiff compares the active result with the pinned one.
func (model *Model) resolveRowDiff(tab *app.Tab) app.PaneContent {
	side, _, found := readResultSide(tab)
	if model.pinned == nil || !found {
		return app.PaneContent{Kind: app.DataIdle, Reason: "pin a result first"}
	}
	active := tab.Results.Active()
	cache := &model.rowDiff
	if cache.resultID != active.ID || cache.revision != active.Revision ||
		cache.rows != len(side.Rows) || cache.pinnedID != model.pinned.ID {
		*cache = rowDiffCache{
			resultID: active.ID, revision: active.Revision, rows: len(side.Rows),
			pinnedID: model.pinned.ID, diff: resultdiff.Compare(model.pinned.Side, side),
		}
	}
	if len(cache.diff.Columns) == 0 {
		return app.PaneContent{
			Kind:   app.DataIdle,
			Reason: "the pinned result " + model.pinned.Label + " and this result have no column in common",
		}
	}
	return app.PaneContent{Kind: app.DataRowDiff, RowDiff: cache.diff}
}

// summaryLine is one line above the rows of a compare, and its ink.
type summaryLine struct {
	text string
	ink  color.Color
}

// buildRowDiffSummary returns the lines above the rows: the two results and the match, the
// columns where they differ, and the counts.
func (model *Model) buildRowDiffSummary(tab *app.Tab, diff resultdiff.Diff) []summaryLine {
	theme := model.styles.Theme
	side, truncated, _ := readResultSide(tab)
	match := "matched by whole rows"
	if len(diff.Key) > 0 {
		match = "matched by " + strings.Join(diff.Key, ", ")
	}
	lines := []summaryLine{{text: "pinned: " + model.pinned.Label + ", " +
		present.FormatCountOf(int64(len(model.pinned.Side.Rows)), "row", "rows") +
		" · here: " + describeResultLabel(tab) + ", " +
		present.FormatCountOf(int64(len(side.Rows)), "row", "rows") + " · " + match, ink: theme.Text}}

	if len(diff.AddedColumns) > 0 || len(diff.RemovedColumns) > 0 {
		parts := []string{"different columns"}
		if len(diff.RemovedColumns) > 0 {
			parts = append(parts, "only pinned: "+strings.Join(diff.RemovedColumns, ", "))
		}
		if len(diff.AddedColumns) > 0 {
			parts = append(parts, "only here: "+strings.Join(diff.AddedColumns, ", "))
		}
		parts = append(parts, "compared: "+strings.Join(diff.Columns, ", "))
		lines = append(lines, summaryLine{text: strings.Join(parts, " · "), ink: theme.Warning})
	}

	counts := "+" + strconv.Itoa(diff.Count(resultdiff.ChangeAdded)) + " added · -" +
		strconv.Itoa(diff.Count(resultdiff.ChangeRemoved)) + " removed · ~" +
		strconv.Itoa(diff.Count(resultdiff.ChangeAltered)) + " changed · " +
		strconv.Itoa(diff.Same) + " same"
	if truncated || model.pinned.Truncated {
		counts += " · loaded rows only"
	}
	return append(lines, summaryLine{text: counts, ink: theme.Text})
}

// renderRowDiff draws the summary, then one line per row that differs. A changed cell shows
// the pinned value and the new value.
func (model *Model) renderRowDiff(
	tab *app.Tab, diff resultdiff.Diff, width, height int,
) []string {
	theme := model.styles.Theme
	lines := []string{}
	for _, line := range model.buildRowDiffSummary(tab, diff) {
		lines = append(lines, paintText(line.ink, theme.Panel, present.FitText(" "+line.text, width)))
	}
	if len(diff.Rows) == 0 {
		empty := "no differences"
		if len(diff.AddedColumns) > 0 || len(diff.RemovedColumns) > 0 {
			empty = "no differences in the compared columns"
		}
		return append(lines, model.renderEmptyState(width, height-len(lines), empty, nil)...)
	}

	headers := append([]string{" "}, diff.Columns...)
	cells := make([][]string, len(diff.Rows))
	for at, row := range diff.Rows {
		cells[at] = append([]string{changeMark(row.Change)}, buildRowCells(row)...)
	}
	widths := present.PlanDetailColumns(headers, cells, width-2, detailGap)
	gap := strings.Repeat(" ", detailGap)

	var header strings.Builder
	header.WriteString(paintOn(theme.Header, " "))
	for at, name := range headers {
		header.WriteString(paintText(theme.Accent, theme.Header, present.FitText(name, widths[at])+gap))
	}
	lines = append(lines, padStyledOn(header.String(), width, theme.Header))

	rows := max(height-len(lines), 1)
	tab.DetailOffset = clampOffset(tab.DetailOffset, rows, len(cells))
	for at := tab.DetailOffset; at < len(cells) && len(lines) < height; at++ {
		row := diff.Rows[at]
		ink := theme.Text
		switch row.Change {
		case resultdiff.ChangeAdded:
			ink = theme.Success
		case resultdiff.ChangeRemoved:
			ink = theme.Error
		}
		var written strings.Builder
		written.WriteString(paintOn(theme.Panel, " "))
		for column, cell := range cells[at] {
			if column >= len(widths) {
				break
			}
			cellInk := ink
			if row.Change == resultdiff.ChangeAltered && (column == 0 || row.Changed[column-1]) {
				cellInk = theme.Warning
			}
			written.WriteString(paintText(cellInk, theme.Panel, present.FitText(cell, widths[column])+gap))
		}
		lines = append(lines, padStyledOn(written.String(), width, theme.Panel))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

func changeMark(change resultdiff.Change) string {
	switch change {
	case resultdiff.ChangeAdded:
		return "+"
	case resultdiff.ChangeRemoved:
		return "-"
	}
	return "~"
}

// buildRowCells returns the cells of a row: the one side it is on, or `old → new` where a
// changed cell differs.
func buildRowCells(row resultdiff.Row) []string {
	switch row.Change {
	case resultdiff.ChangeAdded:
		return row.After
	case resultdiff.ChangeRemoved:
		return row.Before
	}
	cells := make([]string, len(row.After))
	for at, value := range row.After {
		cells[at] = value
		if row.Changed[at] {
			cells[at] = row.Before[at] + " → " + value
		}
	}
	return cells
}
