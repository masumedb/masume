package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// The builder stands where a query tab draws its editor: the diagram of the tables at the
// top, then the joins, the filters and the SQL it writes.

// builderTypeWidth is the room the type of a column keeps in a box.
const builderTypeWidth = 8

// builderPress is what a press on one row of the pane does.
type builderPress string

// The rows of the pane a press reaches.
const (
	pressesNothing builderPress = ""
	pressesJoin    builderPress = "join"
	pressesField   builderPress = "field"
	pressesFilter  builderPress = "filter"
)

// builderRow is one drawn row of the pane and what a press on it reaches.
type builderRow struct {
	text string
	// press is what a press on this row opens, and row is the join, the field or the
	// filter it opens.
	press builderPress
	row   int
	// cells are the columns of the diagram drawn on this row, in the cells they cover.
	cells []present.BuilderCell
	// titles are the boxes whose name is drawn on this row.
	titles []present.BuilderCell
}

// renderBuilder draws the diagram, the joins, the filters and the SQL of a builder tab.
func (model *Model) renderBuilder(
	connection *app.Connection, tab *app.Tab, width, height int,
) []string {
	theme := model.styles.Theme
	builder := tab.Builder
	prompt, asking := findPromptBar(connection, app.PromptBuilderFilter)
	focused := tab.Focus == app.PaneEditor && (asking || !connection.Overlay.IsOpen())
	inner := width - 2
	body := max(height-2, 1)

	promptRows := []string{}
	if asking {
		promptRows = model.renderPromptBar(prompt, inner)
		body = max(body-len(promptRows), 1)
	}

	rows := model.buildBuilderRows(connection, tab, inner)
	// The pane follows the cursor, so a table taller than the pane still shows the column
	// the keys move.
	offset := clampOffset(builder.Offset, body, len(rows))
	if at, found := findCursorRow(builder, rows); found && !builder.Rolled {
		offset = scrollTo(at, offset, body, len(rows))
	}
	builder.Offset = offset

	written := make([]string, 0, body)
	for at := offset; at < len(rows) && len(written) < body; at++ {
		written = append(written, rows[at].text)
	}
	model.recordBuilderRows(rows, offset, len(written), width)
	for len(written) < body {
		written = append(written, "")
	}
	written = model.drawScrollTrack(written, scrollView{
		offset: offset, rows: body, total: len(rows),
		moveTo: func(held int) tea.Cmd {
			builder.Offset = held
			return nil
		},
	}, firstPaneRow+1, model.editorLeft+1, inner, theme.Panel)
	written = append(written, promptRows...)

	return model.styles.RenderBoxRows(BoxOptions{
		Width: width, Height: height, Focused: focused,
		Title:       " builder ",
		Note:        model.describeBuilderNote(builder, inner),
		BottomTitle: model.describeBuilderBorder(builder),
		Lines:       written, Ground: theme.Panel,
	})
}

// describeBuilderNote returns the count of the tables and the picked columns.
func (model *Model) describeBuilderNote(builder *app.Builder, width int) string {
	if builder.IsEmpty() {
		return ""
	}
	text := " " + present.FormatCountOf(int64(len(builder.Tables)), "table", "tables") +
		" · " + present.FormatCountOf(int64(builder.CountPicked()), "column", "columns") + " "
	return model.styles.Muted().Background(model.styles.Theme.Panel).
		Render(present.TruncateText(text, width))
}

// describeBuilderBorder returns the keys the bottom border of the pane names.
func (model *Model) describeBuilderBorder(builder *app.Builder) string {
	if builder.IsEmpty() {
		return ""
	}
	keys := model.buildKeyLineOf(builderBorderKeySpecs, keyScene{})
	return keys.buildText()
}

// recordBuilderRows keeps what every drawn row stands for, so a press reaches it.
func (model *Model) recordBuilderRows(rows []builderRow, offset, drawn, width int) {
	model.builderRows = rows
	model.layout.builderRows = rowsHit{
		top: firstPaneRow + 1, count: drawn, offset: offset,
		from: model.editorLeft + 1, to: model.editorLeft + width - 2,
	}
}

// buildBuilderRows returns every row of the pane, in the order it draws them.
func (model *Model) buildBuilderRows(
	connection *app.Connection, tab *app.Tab, width int,
) []builderRow {
	builder := tab.Builder
	if builder.IsEmpty() {
		return model.buildEmptyBuilderRows(width)
	}

	rows := model.buildDiagramRows(builder, width)
	rows = append(rows, model.buildJoinRows(builder, width)...)
	rows = append(rows, model.buildFieldRows(builder, width)...)
	rows = append(rows, model.buildFilterRows(builder, width)...)
	return append(rows, model.buildSQLRows(connection, tab, width)...)
}

// buildEmptyBuilderRows returns the rows of a builder that holds no table.
func (model *Model) buildEmptyBuilderRows(width int) []builderRow {
	keys := model.buildKeyLineOf(builderEmptyKeySpecs, keyScene{})
	rows := []builderRow{
		{text: ""},
		{text: " " + model.styles.Muted().Render("no table yet")},
		{text: ""},
	}
	for _, line := range present.WrapWords(keys.buildText(), width-2) {
		rows = append(rows, builderRow{text: " " + line})
	}
	return rows
}

// buildDiagramRows draws the tables and paints the column the cursor stands on. A diagram
// wider than the pane is scrolled sideways, so the table the cursor stands in is drawn.
func (model *Model) buildDiagramRows(builder *app.Builder, width int) []builderRow {
	drawn := present.RenderBuilderDiagram(
		buildDiagramBoxes(builder), buildDiagramLinks(builder))
	// The diagram follows the cursor, unless the wheel moved it away.
	box := builder.Table
	if builder.Rolled {
		box = -1
	}
	builder.ColumnOffset = present.FindBuilderColumnOffset(
		builder.ColumnOffset, box, len(builder.Tables), width-2)
	drawn = present.ScrollBuilderDiagram(drawn, builder.ColumnOffset, width-2)

	rows := make([]builderRow, 0, len(drawn.Lines)+1)
	for at, line := range drawn.Lines {
		held := builderRow{
			text: " " + model.paintDiagramLine(builder, drawn, at, line, width-2),
		}
		for _, cell := range drawn.Cells {
			if cell.Y == at {
				held.cells = append(held.cells, cell)
			}
		}
		for _, title := range drawn.Titles {
			if title.Y == at {
				held.titles = append(held.titles, title)
			}
		}
		rows = append(rows, held)
	}
	return append(rows, builderRow{text: ""})
}

// findCursorRow returns the row of the pane the cursor stands on: the column of the diagram,
// or the filter it marks.
func findCursorRow(builder *app.Builder, rows []builderRow) (int, bool) {
	for at, held := range rows {
		if builder.Section == app.BuilderFilters {
			if held.press == pressesFilter && held.row == builder.Row {
				return at, true
			}
			continue
		}
		for _, cell := range held.cells {
			if cell.Box == builder.Table && cell.Column == builder.Column {
				return at, true
			}
		}
	}
	return 0, false
}

// paintDiagramLine paints the title of every box and the column the cursor stands on.
func (model *Model) paintDiagramLine(
	builder *app.Builder, drawn present.BuilderDiagram, row int, line string, width int,
) string {
	theme := model.styles.Theme
	held := []rune(present.TruncateText(line, width))

	paint := func(cell present.BuilderCell, style func(...string) string) {
		if cell.Y != row || cell.X < 0 || cell.X >= len(held) {
			return
		}
		end := min(cell.X+cell.Width, len(held))
		text := style(string(held[cell.X:end]))
		line = string(held[:cell.X]) + text + string(held[end:])
	}

	for _, title := range drawn.Titles {
		if title.Box == builder.Table {
			paint(title, model.styles.Accent().Render)
			continue
		}
		paint(title, model.styles.Ink().Render)
	}
	for _, cell := range drawn.Cells {
		if cell.Box != builder.Table || cell.Column != builder.Column ||
			builder.Section != app.BuilderTables {
			continue
		}
		paint(cell, func(text ...string) string {
			return paintText(theme.OnAccent, theme.Accent, strings.Join(text, ""))
		})
	}
	return line
}

// buildDiagramBoxes returns the tables of the builder as the diagram draws them.
func buildDiagramBoxes(builder *app.Builder) []present.BuilderBox {
	boxes := make([]present.BuilderBox, 0, len(builder.Tables))
	for _, table := range builder.Tables {
		box := present.BuilderBox{
			Title:  table.Ref.Schema + "." + table.Ref.Name + " " + table.Alias,
			Reason: describeTableReason(table),
		}
		for _, column := range table.Columns {
			box.Columns = append(box.Columns, present.BuilderColumnBox{
				Name: column.Name, Picked: column.Picked,
				Kind: present.TruncateText(query.ReadBaseType(column.DataType), builderTypeWidth),
				Note: describeColumnNote(column),
			})
		}
		boxes = append(boxes, box)
	}
	return boxes
}

// describeTableReason returns what stands in place of the columns of a table.
func describeTableReason(table app.BuilderTable) string {
	switch {
	case table.Problem != "":
		return table.Problem
	case table.Reading:
		return "reading the columns…"
	case len(table.Columns) == 0:
		return "no column"
	}
	return ""
}

// describeColumnNote returns the aggregate and the sort drawn after a column name.
func describeColumnNote(column app.BuilderColumn) string {
	note := ""
	if column.Aggregate != statement.AggregateNone {
		note = string(column.Aggregate)
	}
	if column.Sort != "" {
		note = strings.TrimSpace(note + " " + string(column.Sort))
	}
	return note
}

// buildDiagramLinks returns the joins of the builder as the diagram draws them.
func buildDiagramLinks(builder *app.Builder) []present.BuilderLink {
	links := make([]present.BuilderLink, 0, len(builder.Joins))
	for _, join := range builder.Joins {
		for at := 0; at < min(len(join.Columns), len(join.BaseColumns)); at++ {
			links = append(links, present.BuilderLink{
				From: join.Table, FromColumn: join.Columns[at],
				To: join.Base, ToColumn: join.BaseColumns[at],
			})
		}
	}
	return links
}

// buildJoinRows returns one row per join, with the condition it writes.
func (model *Model) buildJoinRows(builder *app.Builder, width int) []builderRow {
	if len(builder.Joins) == 0 {
		return nil
	}
	rows := []builderRow{{text: model.buildSectionRule(" joins ", width)}}
	for at, join := range builder.Joins {
		condition := model.describeJoin(builder, join)
		kind := string(join.Kind)
		if condition == "" {
			kind = string(statement.JoinCross)
		}
		text := kind + "  " + condition
		rows = append(rows, builderRow{
			text:  "  " + model.styles.Muted().Render(present.TruncateText(text, width-3)),
			press: pressesJoin, row: at,
		})
	}
	return rows
}

// describeJoin returns the condition of one join as the pane shows it.
func (model *Model) describeJoin(builder *app.Builder, join app.BuilderJoin) string {
	if written := strings.TrimSpace(join.On); written != "" {
		return written
	}
	if join.Table >= len(builder.Tables) || join.Base >= len(builder.Tables) {
		return ""
	}
	pairs := make([]string, 0, len(join.Columns))
	for at := 0; at < min(len(join.Columns), len(join.BaseColumns)); at++ {
		pairs = append(pairs, builder.Tables[join.Table].Alias+"."+join.Columns[at]+" = "+
			builder.Tables[join.Base].Alias+"."+join.BaseColumns[at])
	}
	return strings.Join(pairs, " and ")
}

// builderFieldStride packs the table and the column of a field row into one number, so a
// press on the row reaches the column it draws.
const builderFieldStride = 1024

// The columns of one row of the fields section.
const (
	builderFieldNameWidth  = 34
	builderAggregateWidth  = 10
	builderFieldAliasWidth = 18
)

// buildFieldRows returns one row per picked column: the aggregate over it, the name of the
// result column, and the sort. A column without an aggregate is grouped by where another
// one has one.
func (model *Model) buildFieldRows(builder *app.Builder, width int) []builderRow {
	if builder.CountPicked() == 0 {
		return nil
	}
	grouped := builder.BuildPlan(nil).GroupsRows()

	rows := []builderRow{{text: model.buildSectionRule(" fields ", width)}}
	for at, table := range builder.Tables {
		for column := range table.Columns {
			if !table.Columns[column].Picked {
				continue
			}
			rows = append(rows, builderRow{
				text: "  " + model.buildFieldRow(
					table.Alias, table.Columns[column], grouped, width-3),
				press: pressesField, row: at*builderFieldStride + column,
			})
		}
	}
	return rows
}

// buildFieldRow writes one row of the fields section.
func (model *Model) buildFieldRow(
	alias string, column app.BuilderColumn, grouped bool, width int,
) string {
	aggregate := string(column.Aggregate)
	trail := string(column.Sort)
	if column.Aggregate == statement.AggregateNone && grouped {
		trail = strings.TrimSpace("group by " + trail)
	}
	written := model.styles.Ink().Render(present.FitText(
		alias+"."+column.Name, builderFieldNameWidth)) +
		model.styles.Accent().Render(present.FitText(aggregate, builderAggregateWidth)) +
		model.styles.Ink().Render(present.FitText(column.As, builderFieldAliasWidth)) +
		model.styles.Muted().Render(trail)
	return truncateStyled(written, width)
}

// buildFilterRows returns one row per filter of the where clause.
func (model *Model) buildFilterRows(builder *app.Builder, width int) []builderRow {
	if len(builder.Filters) == 0 {
		return nil
	}
	rows := []builderRow{{text: model.buildSectionRule(" where ", width)}}
	for at, filter := range builder.Filters {
		text := present.TruncateText(filter, width-3)
		if builder.Section == app.BuilderFilters && builder.Row == at {
			text = model.styles.Accent().Render(text)
		} else {
			text = model.styles.Ink().Render(text)
		}
		rows = append(rows, builderRow{
			text: "  " + text, press: pressesFilter, row: at,
		})
	}
	return rows
}

// buildSQLRows returns the statement the builder writes, coloured as SQL.
func (model *Model) buildSQLRows(
	connection *app.Connection, tab *app.Tab, width int,
) []builderRow {
	written := tab.Builder.BuildSQL(connection.Session.Dialect())
	if strings.TrimSpace(written) == "" {
		return nil
	}
	rows := []builderRow{{text: model.buildSectionRule(" sql ", width)}}
	spans := collectLineHighlights(
		written, connection.Session.Language().Tokenize(written), 0, len(written)+1)
	for at, line := range strings.Split(written, "\n") {
		held := codeLine{text: line, width: width - 3}
		if at < len(spans) {
			held.spans = spans[at]
		}
		rows = append(rows, builderRow{text: "  " + model.renderCodeLine(held)})
	}
	return rows
}

// buildSectionRule returns the rule that opens one section of the pane.
func (model *Model) buildSectionRule(title string, width int) string {
	text := title + strings.Repeat("─", max(width-present.MeasureText(title)-1, 0))
	return " " + model.styles.Faint().Render(text)
}
