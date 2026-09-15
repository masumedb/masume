package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// The keys of a query builder tab. The diagram takes no typed text, so every key runs an
// action and the cards take what has to be typed.

// runBuilderAction runs one action of the builder pane.
func (model *Model) runBuilderAction(
	connection *app.Connection, tab *app.Tab, match Match,
) (tea.Model, tea.Cmd) {
	if !tab.BuildsQuery() {
		return model, nil
	}
	builder := tab.Builder

	switch match.Action {
	case ActionCursorUp:
		model.stepBuilderCursor(builder, -1)
	case ActionCursorDown:
		model.stepBuilderCursor(builder, 1)
	case ActionPreviousTable:
		model.stepBuilderTable(builder, -1)
	case ActionNextTable:
		model.stepBuilderTable(builder, 1)
	case ActionPickColumn:
		model.pickBuilderColumn(connection, builder)
	case ActionEditBuilderRow:
		return model.openBuilderRow(connection, builder)
	case ActionAddBuilderTable:
		return model.askBuilderTable(connection, builder)
	case ActionAddBuilderFilter:
		return model.askBuilderFilter(connection, builder)
	case ActionDropBuilderRow:
		model.dropBuilderRow(builder)
	case ActionSendToEditor:
		return model.sendBuilderToEditor(connection, tab)
	}
	return model, nil
}

// stepBuilderCursor moves the cursor down the columns of a table, and into the filters
// under the last one.
func (model *Model) stepBuilderCursor(builder *app.Builder, step int) {
	if builder.Section == app.BuilderFilters {
		builder.Row += step
		if builder.Row < 0 {
			builder.Section, builder.Row = app.BuilderTables, 0
			return
		}
		builder.Row = min(builder.Row, len(builder.Filters)-1)
		return
	}

	table, found := builder.ActiveTable()
	if !found {
		return
	}
	held := builder.Column + step
	if held >= len(table.Columns) && len(builder.Filters) > 0 {
		builder.Section, builder.Row, builder.Rolled = app.BuilderFilters, 0, false
		return
	}
	builder.MoveCursor(builder.Table, clampBuilderIndex(held, len(table.Columns)))
}

// stepBuilderTable moves the cursor to the table before or after this one.
func (model *Model) stepBuilderTable(builder *app.Builder, step int) {
	if len(builder.Tables) == 0 {
		return
	}
	builder.Section = app.BuilderTables
	builder.MoveCursor(wrap(builder.Table+step, len(builder.Tables)), builder.Column)
	table, found := builder.ActiveTable()
	if !found {
		return
	}
	builder.MoveCursor(builder.Table, clampBuilderIndex(builder.Column, len(table.Columns)))
}

// clampBuilderIndex keeps an index inside a list.
func clampBuilderIndex(index, count int) int {
	if count == 0 {
		return 0
	}
	return clamp(index, count)
}

// pickBuilderColumn takes the column under the cursor into the select list, or out of it.
func (model *Model) pickBuilderColumn(connection *app.Connection, builder *app.Builder) {
	if builder.Section != app.BuilderTables {
		return
	}
	column, found := builder.ActiveColumn()
	if !found {
		return
	}
	column.Picked = !column.Picked
	if !column.Picked {
		column.Aggregate, column.As, column.Sort = statement.AggregateNone, "", ""
	}
}

// dropBuilderRow removes the table or the filter the cursor stands on.
func (model *Model) dropBuilderRow(builder *app.Builder) {
	if builder.Section == app.BuilderFilters {
		builder.DropFilter(builder.Row)
		return
	}
	builder.DropTable(builder.Table)
}

// askBuilderTable opens the picker of the tables of the server. The first table of a
// builder stands alone, and every table after it is joined to one already there.
func (model *Model) askBuilderTable(
	connection *app.Connection, builder *app.Builder,
) (tea.Model, tea.Cmd) {
	tables := connection.Catalog.Tables
	if len(tables) == 0 {
		connection.ShowError("the object tree holds no table yet")
		return model, nil
	}
	title := " add a table "
	if builder != nil && !builder.IsEmpty() {
		title = " join a table "
	}
	rows := make([]app.PaletteAction, 0, len(tables))
	for _, table := range tables {
		rows = append(rows, app.PaletteAction{
			ID: present.BuildTableID(table), Label: table.Schema + "." + table.Name,
		})
	}
	connection.Open(app.Overlay{
		Kind: app.OverlayBuilderTables, Title: title, Palette: rows,
		Draft: app.NewEditorBuffer("", 0),
	})
	return model, nil
}

// addBuilderTable puts the table the picker chose into the builder and reads its columns.
// Every table after the first is joined to one already there.
func (model *Model) addBuilderTable(
	connection *app.Connection, tab *app.Tab, table db.TableRef,
) (tea.Model, tea.Cmd) {
	builder := tab.Builder
	at := builder.AddTable(table)
	connection.CloseEveryOverlay()
	return model, readBuilderTable(model.ActiveID(), tab.ID, at, connection.Session, table,
		len(builder.Tables) > 1)
}

// openBuilderRow opens the card of the row the cursor stands on: the field card of a picked
// column, the join card of a table, or the filter of a where row.
func (model *Model) openBuilderRow(
	connection *app.Connection, builder *app.Builder,
) (tea.Model, tea.Cmd) {
	if builder.Section == app.BuilderFilters {
		return model.askBuilderFilterAt(connection, builder, builder.Row)
	}
	column, found := builder.ActiveColumn()
	if !found {
		return model, nil
	}
	if !column.Picked {
		column.Picked = true
	}
	connection.Open(app.Overlay{
		Kind: app.OverlayBuilderField, Title: " " + model.describeFieldTitle(builder) + " ",
		Field: 0, Draft: app.NewEditorBuffer(column.As, len(column.As)),
	})
	return model, nil
}

// describeFieldTitle returns the name of the column the field card edits.
func (model *Model) describeFieldTitle(builder *app.Builder) string {
	table, found := builder.ActiveTable()
	if !found {
		return "field"
	}
	column, holds := builder.ActiveColumn()
	if !holds {
		return table.Alias
	}
	return table.Alias + "." + column.Name
}

// askBuilderFilter opens the prompt of a new filter. The field opens empty, as the filter
// of the grid does.
func (model *Model) askBuilderFilter(
	connection *app.Connection, builder *app.Builder,
) (tea.Model, tea.Cmd) {
	connection.Open(app.Overlay{
		Kind: app.OverlayPrompt, Prompt: app.PromptBuilderFilter, Title: "where",
		Hint: "one condition of the where clause", Field: -1,
		Draft: app.NewEditorBuffer("", 0),
	})
	return model, nil
}

// askBuilderFilterAt opens the prompt of the filter of that row.
func (model *Model) askBuilderFilterAt(
	connection *app.Connection, builder *app.Builder, row int,
) (tea.Model, tea.Cmd) {
	if row < 0 || row >= len(builder.Filters) {
		return model, nil
	}
	written := builder.Filters[row]
	connection.Open(app.Overlay{
		Kind: app.OverlayPrompt, Prompt: app.PromptBuilderFilter, Title: "where",
		Hint: "one condition of the where clause", Field: row,
		Draft: app.NewEditorBuffer(written, len(written)),
	})
	return model, nil
}

// writeBuilderFilter takes the answer of the filter prompt.
func (model *Model) writeBuilderFilter(
	connection *app.Connection, tab *app.Tab, row int, written string,
) {
	builder := tab.Builder
	written = strings.TrimSpace(written)
	if written == "" {
		if row >= 0 {
			builder.DropFilter(row)
		}
		return
	}
	if row >= 0 && row < len(builder.Filters) {
		builder.Filters[row] = written
		return
	}
	builder.Filters = append(builder.Filters, written)
	builder.Section, builder.Row = app.BuilderFilters, len(builder.Filters)-1
}

// sendBuilderToEditor opens a query tab on the statement the builder wrote.
func (model *Model) sendBuilderToEditor(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	written := tab.Builder.BuildSQL(connection.Session.Dialect())
	if strings.TrimSpace(written) == "" {
		connection.ShowError("the builder holds no table")
		return model, nil
	}
	opened := connection.OpenQueryTab(written)
	opened.Focus = app.PaneEditor
	connection.Show("the query is in a tab of its own")
	return model, model.saveWorkspace(connection)
}

// runBuilderQuery runs the statement the builder wrote, in the tab of the builder.
func (model *Model) runBuilderQuery(
	connection *app.Connection, tab *app.Tab,
) (tea.Model, tea.Cmd) {
	written := tab.Builder.BuildSQL(connection.Session.Dialect())
	if strings.TrimSpace(written) == "" {
		connection.ShowError("the builder holds no table")
		return model, nil
	}
	return model.execute(connection, tab, []string{written})
}

// stepBuilderField steps one field of the field card through its values.
func stepBuilderField(builder *app.Builder, field, step int) {
	column, found := builder.ActiveColumn()
	if !found {
		return
	}
	switch field {
	case builderFieldAggregate:
		column.Aggregate = statement.Aggregates[wrap(
			indexOfAggregate(column.Aggregate)+step, len(statement.Aggregates))]
	case builderFieldSort:
		column.Sort = builderSorts[wrap(
			indexOfSort(column.Sort)+step, len(builderSorts))]
	}
}

// The rows of the field card.
const (
	builderFieldAggregate = 0
	builderFieldAlias     = 1
	builderFieldSort      = 2
	builderFieldRows      = 3
)

// builderSorts are the directions a field steps through.
var builderSorts = []core.SortDirection{"", core.SortAscending, core.SortDescending}

// indexOfAggregate returns the position of an aggregate in the list the card steps through.
func indexOfAggregate(held statement.Aggregate) int {
	for at, aggregate := range statement.Aggregates {
		if aggregate == held {
			return at
		}
	}
	return 0
}

// indexOfSort returns the position of a direction in the list the card steps through.
func indexOfSort(held core.SortDirection) int {
	for at, direction := range builderSorts {
		if direction == held {
			return at
		}
	}
	return 0
}

// stepJoinKind steps the join of a table through the kinds.
func stepJoinKind(builder *app.Builder, table, step int) {
	join, found := builder.FindJoin(table)
	if !found {
		return
	}
	join.Kind = statement.JoinKinds[wrap(
		indexOfJoinKind(join.Kind)+step, len(statement.JoinKinds))]
}

// indexOfJoinKind returns the position of a kind in the list the card steps through.
func indexOfJoinKind(held statement.JoinKind) int {
	for at, kind := range statement.JoinKinds {
		if kind == held {
			return at
		}
	}
	return 0
}

// openNewBuilder opens an empty builder tab and asks for its first table.
func (model *Model) openNewBuilder(connection *app.Connection) (tea.Model, tea.Cmd) {
	tab := connection.OpenBuilder()
	tab.Focus = app.PaneEditor
	return model.askBuilderTable(connection, tab.Builder)
}

// readBuilderTableAnswer writes the columns of a table into the builder that asked for
// them, and joins the table where the picker was opened to join.
func (model *Model) readBuilderTableAnswer(answered builderTableMsg) (tea.Model, tea.Cmd) {
	connection, _, found := model.findConnection(answered.ConnectionID)
	if !found {
		return model, nil
	}
	tab, holds := findTab(connection, answered.TabID)
	if !holds || !tab.BuildsQuery() {
		return model, nil
	}
	builder := tab.Builder

	if answered.Problem != "" {
		builder.WriteProblem(answered.Table, answered.Problem)
		return model, nil
	}
	builder.WriteColumns(answered.Table, answered.Detail)
	if !answered.Joins {
		return model, nil
	}

	join, proposed := builder.FindForeignKeyJoin(answered.Table)
	if !proposed {
		join = app.BuilderJoin{
			Kind: statement.JoinInner, Table: answered.Table, Base: 0,
		}
	}
	builder.AddJoin(join)
	return model.openJoinCard(connection, builder, answered.Table, proposed)
}

// openJoinCard opens the card of the join of that table.
func (model *Model) openJoinCard(
	connection *app.Connection, builder *app.Builder, table int, proposed bool,
) (tea.Model, tea.Cmd) {
	join, found := builder.FindJoin(table)
	if !found {
		return model, nil
	}
	written := model.describeJoin(builder, *join)
	note := "no foreign key; write the condition"
	if proposed {
		note = "from the foreign key"
	}
	connection.Open(app.Overlay{
		Kind: app.OverlayBuilderJoin, Field: table, Body: note,
		Title: " join " + builder.Tables[table].Ref.Name + " ",
		Draft: app.NewEditorBuffer(written, len(written)),
	})
	return model, nil
}

// applyJoinCard takes the condition the join card holds.
func (model *Model) applyJoinCard(
	connection *app.Connection, tab *app.Tab, table int, written string,
) {
	builder := tab.Builder
	join, found := builder.FindJoin(table)
	if !found {
		return
	}
	written = strings.TrimSpace(written)
	if written == model.describeJoin(builder, *join) {
		written = ""
	}
	join.On = written
	connection.CloseEveryOverlay()
}

// applyBuilderField takes the name the field card holds and closes it.
func (model *Model) applyBuilderField(
	connection *app.Connection, tab *app.Tab, written string,
) {
	if column, found := tab.Builder.ActiveColumn(); found {
		column.As = strings.TrimSpace(written)
	}
	connection.CloseEveryOverlay()
}

// findBuilderTable returns the table of that row id.
func findBuilderTable(tables []db.TableRef, id string) (db.TableRef, bool) {
	for _, table := range tables {
		if present.BuildTableID(table) == id {
			return table, true
		}
	}
	return db.TableRef{}, false
}

// pressBuilderRow takes a press on the builder pane: a column of the diagram picks it, a
// title marks its table, and a join, a field or a filter row opens its card.
func (model *Model) pressBuilderRow(
	connection *app.Connection, tab *app.Tab, mouse tea.Mouse, row int,
) (tea.Model, tea.Cmd) {
	if !tab.BuildsQuery() || row < 0 || row >= len(model.builderRows) {
		return model, nil
	}
	held := model.builderRows[row]
	builder := tab.Builder

	// A row of the diagram holds one cell per table, so the column of the press decides
	// which table it landed in.
	left := model.layout.builderRows.from + 1
	for _, cell := range held.cells {
		if mouse.X < left+cell.X || mouse.X >= left+cell.X+cell.Width {
			continue
		}
		builder.Section = app.BuilderTables
		builder.MoveCursor(cell.Box, cell.Column)
		model.pickBuilderColumn(connection, builder)
		return model, nil
	}
	for _, title := range held.titles {
		if mouse.X < left+title.X || mouse.X >= left+title.X+title.Width {
			continue
		}
		builder.Section = app.BuilderTables
		builder.MoveCursor(title.Box, 0)
		return model, nil
	}

	switch held.press {
	case pressesJoin:
		join, found := builder.FindJoinAt(held.row)
		if !found {
			return model, nil
		}
		builder.Section, builder.Table = app.BuilderTables, join.Table
		return model.openJoinCard(connection, builder, join.Table, join.Column != "")
	case pressesField:
		builder.Section = app.BuilderTables
		builder.Table = held.row / builderFieldStride
		builder.Column = held.row % builderFieldStride
		return model.openBuilderRow(connection, builder)
	case pressesFilter:
		builder.Section, builder.Row = app.BuilderFilters, held.row
		return model.askBuilderFilterAt(connection, builder, held.row)
	}
	return model, nil
}

// readBuilderTables asks the server for the columns of every table of a restored builder.
func (model *Model) readBuilderTables(
	connection *app.Connection, tab *app.Tab,
) tea.Cmd {
	reads := []tea.Cmd{}
	for at, table := range tab.Builder.Tables {
		if !table.Reading {
			continue
		}
		reads = append(reads, readBuilderTable(
			model.ActiveID(), tab.ID, at, connection.Session, table.Ref, joinsNothing))
	}
	if len(reads) == 0 {
		return nil
	}
	return tea.Batch(reads...)
}

// joinsNothing reads the columns of a table that is already in the builder, so the answer
// opens no join card.
const joinsNothing = false
