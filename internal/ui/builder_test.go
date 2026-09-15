package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/cfg"
	"github.com/turanmahmudov/masume/internal/core"
	"github.com/turanmahmudov/masume/internal/db"
	"github.com/turanmahmudov/masume/internal/present"
	"github.com/turanmahmudov/masume/internal/query"
	"github.com/turanmahmudov/masume/internal/query/statement"
)

// openBuilderTab opens a builder tab with the shop tables in the catalog of the connection.
func openBuilderTab(t *testing.T) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 118, 34)
	connection := model.connections.active()
	connection.Catalog.Tables = []db.TableRef{
		{Schema: "shop", Name: "customers"},
		{Schema: "shop", Name: "orders"},
	}
	connection.Catalog.Loading = false

	tab := connection.OpenBuilder()
	tab.Focus = app.PaneEditor
	return model, connection, tab
}

// writeBuilderTable answers the read of the columns of one table.
func writeBuilderTable(
	t *testing.T, model *Model, tab *app.Tab, at int, name string,
	columns []string, keys ...query.ForeignKey,
) {
	t.Helper()
	detail := db.TableDetail{ForeignKeys: keys}
	for _, column := range columns {
		detail.Columns = append(detail.Columns,
			db.ColumnDetail{Name: column, DataType: "text"})
	}
	held, _ := model.Update(builderTableMsg{
		ConnectionID: model.ActiveID(), TabID: tab.ID, Table: at,
		Detail: detail, Joins: at > 0,
	})
	if held != model {
		t.Fatal("an answer replaced the model")
	}
}

// The picker of the tables adds the row it chose, and the read of its columns follows.
func TestBuilderAddsTheTableThePickerChose(t *testing.T) {
	model, connection, tab := openBuilderTab(t)

	model.runBuilderAction(connection, tab, Match{Action: ActionAddBuilderTable})
	if connection.Overlay.Kind != app.OverlayBuilderTables {
		t.Fatalf("the card is %q", connection.Overlay.Kind)
	}
	if len(connection.Overlay.Palette) != 2 {
		t.Fatalf("the picker lists %d tables", len(connection.Overlay.Palette))
	}

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(tab.Builder.Tables) != 1 || tab.Builder.Tables[0].Ref.Name != "customers" {
		t.Fatalf("the builder holds %+v", tab.Builder.Tables)
	}
	if !tab.Builder.Tables[0].Reading {
		t.Error("the builder is not reading the columns of the table it added")
	}
}

// A second table is joined on the foreign key, and the card of the join opens on it.
func TestBuilderJoinsTheSecondTableOnItsForeignKey(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "customers"})
	writeBuilderTable(t, model, tab, 0, "customers", []string{"id", "name"})

	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 1, "orders", []string{"id", "customer_id"},
		query.ForeignKey{
			Columns: []string{"customer_id"}, TargetSchema: "shop",
			TargetTable: "customers", TargetColumns: []string{"id"},
		})

	if len(tab.Builder.Joins) != 1 {
		t.Fatalf("the builder holds %d joins", len(tab.Builder.Joins))
	}
	join := tab.Builder.Joins[0]
	if join.Column != "customer_id" || join.BaseColumn != "id" {
		t.Errorf("the join reads %+v", join)
	}
	if connection.Overlay.Kind != app.OverlayBuilderJoin {
		t.Errorf("the card is %q", connection.Overlay.Kind)
	}
	if !strings.Contains(connection.Overlay.Body, "foreign key") {
		t.Errorf("the card says %q", connection.Overlay.Body)
	}
}

// Space picks the column under the cursor, and the SQL follows it.
func TestBuilderPicksTheColumnUnderTheCursor(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 0, "orders", []string{"id", "total"})

	model.runBuilderAction(connection, tab, Match{Action: ActionCursorDown})
	model.runBuilderAction(connection, tab, Match{Action: ActionPickColumn})

	written := tab.Builder.BuildSQL(connection.Session.Dialect())
	if !strings.Contains(written, "select o.total") {
		t.Errorf("the builder wrote\n%s", written)
	}
	model.runBuilderAction(connection, tab, Match{Action: ActionPickColumn})
	if again := tab.Builder.BuildSQL(connection.Session.Dialect()); !strings.Contains(again, "select *") {
		t.Errorf("the second press wrote\n%s", again)
	}
}

// The field card takes an aggregate and a name, and the SQL groups by the rest.
func TestBuilderFieldCardWritesTheAggregate(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 0, "orders", []string{"id", "customer_id"})
	tab.Builder.Tables[0].Columns[1].Picked = true

	model.runBuilderAction(connection, tab, Match{Action: ActionEditBuilderRow})
	if connection.Overlay.Kind != app.OverlayBuilderField {
		t.Fatalf("the card is %q", connection.Overlay.Kind)
	}
	stepBuilderField(tab.Builder, builderFieldAggregate, 1)
	connection.Overlay.Draft.SetText("orders")
	model.applyBuilderField(connection, tab, connection.Overlay.Draft.Text)

	written := tab.Builder.BuildSQL(connection.Session.Dialect())
	if !strings.Contains(written, "count(o.id) as orders") {
		t.Errorf("the builder wrote\n%s", written)
	}
	if column := tab.Builder.Tables[0].Columns[0]; column.Aggregate != statement.AggregateCount {
		t.Errorf("the column holds %q", column.Aggregate)
	}
}

// The where prompt adds one filter, and the row it opens on edits that filter.
func TestBuilderPromptWritesAndEditsAFilter(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 0, "orders", []string{"id", "total"})

	model.runBuilderAction(connection, tab, Match{Action: ActionAddBuilderFilter})
	if connection.Overlay.Prompt != app.PromptBuilderFilter {
		t.Fatalf("the prompt is %q", connection.Overlay.Prompt)
	}
	if written := connection.Overlay.Draft.Text; written != "" {
		t.Errorf("the prompt opened on %q", written)
	}
	model.writeBuilderFilter(connection, tab, -1, "o.total > 100")

	if len(tab.Builder.Filters) != 1 {
		t.Fatalf("the builder holds %d filters", len(tab.Builder.Filters))
	}
	model.writeBuilderFilter(connection, tab, 0, "o.total > 500")
	if tab.Builder.Filters[0] != "o.total > 500" {
		t.Errorf("the filter reads %q", tab.Builder.Filters[0])
	}
	if !strings.Contains(tab.Builder.BuildSQL(connection.Session.Dialect()), "where o.total > 500") {
		t.Error("the where clause does not hold the filter")
	}
}

// The builder sends its statement to a query tab, and the builder tab keeps its state.
func TestBuilderSendsTheStatementToAQueryTab(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 0, "orders", []string{"id"})
	tabs := len(connection.Tabs)

	model.runBuilderAction(connection, tab, Match{Action: ActionSendToEditor})

	if len(connection.Tabs) != tabs+1 {
		t.Fatalf("the connection holds %d tabs", len(connection.Tabs))
	}
	opened := connection.Active()
	if opened.Kind != app.TabQuery || !strings.Contains(opened.Editor.Text, "from") {
		t.Errorf("the tab holds %q", opened.Editor.Text)
	}
	if len(tab.Builder.Tables) != 1 {
		t.Error("the builder lost its table")
	}
}

// The pane draws the tables, the joins, the filters and the SQL.
func TestBuilderPaneDrawsEverySection(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "customers"})
	writeBuilderTable(t, model, tab, 0, "customers", []string{"id", "name"})
	tab.Builder.Tables[0].Columns[1].Picked = true
	tab.Builder.Filters = []string{"c.name is not null"}

	drawn := stripEscapes(strings.Join(
		model.renderBuilder(connection, tab, 118, 24), "\n"))
	for _, wanted := range []string{
		"shop.customers c", "[x] name", "where", "c.name is not null",
		"sql", "select c.name",
	} {
		if !strings.Contains(drawn, wanted) {
			t.Errorf("the pane holds no %q:\n%s", wanted, drawn)
		}
	}
}

// Dropping the table under the cursor empties the builder again.
func TestBuilderDropsTheTableUnderTheCursor(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 0, "orders", []string{"id"})

	model.runBuilderAction(connection, tab, Match{Action: ActionDropBuilderRow})

	if !tab.Builder.IsEmpty() {
		t.Errorf("the builder holds %d tables", len(tab.Builder.Tables))
	}
}

// A press on a column of the diagram picks that column.
func TestBuilderPressPicksTheColumnUnderThePointer(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 0, "orders", []string{"id", "total"})
	model.renderBuilder(connection, tab, 118, 24)

	// The second column of the first box, in the cells the renderer reported.
	var cell present.BuilderCell
	row := -1
	for at, held := range model.builderRows {
		for _, found := range held.cells {
			if found.Box == 0 && found.Column == 1 {
				cell, row = found, at
			}
		}
	}
	if row < 0 {
		t.Fatal("the pane reported no cell of the second column")
	}

	block := model.layout.builderRows
	model.pressBuilderRow(connection, tab, tea.Mouse{
		X: block.from + 1 + cell.X, Y: block.top + row - block.offset,
	}, row)

	if !tab.Builder.Tables[0].Columns[1].Picked {
		t.Error("the press picked no column")
	}
	if tab.Builder.Column != 1 {
		t.Errorf("the cursor stands on column %d", tab.Builder.Column)
	}
}

// The keys of the builder reach it through the registry, not through the action alone: a
// press of the bound chord has to move the diagram, open the cards and answer them.
func TestBuilderTakesItsKeysFromTheKeyboard(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 0, "orders", []string{"id", "total"})

	pressKey(t, model, tea.KeyPressMsg{Code: 't', Text: "t"})
	if connection.Overlay.Kind != app.OverlayBuilderTables {
		t.Errorf("t opened %q", connection.Overlay.Kind)
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})

	pressKey(t, model, tea.KeyPressMsg{Code: 'w', Text: "w"})
	if connection.Overlay.Prompt != app.PromptBuilderFilter {
		t.Errorf("w opened %q", connection.Overlay.Prompt)
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	pressKey(t, model, tea.KeyPressMsg{Code: ' ', Text: " "})
	if !tab.Builder.Tables[0].Columns[1].Picked {
		t.Error("space picked no column")
	}
	pressKey(t, model, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !tab.Builder.IsEmpty() {
		t.Error("x dropped no table")
	}
}

// The join card steps its kind with the arrows and takes the condition with Enter.
func TestBuilderJoinCardStepsTheKindAndCloses(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "customers"})
	writeBuilderTable(t, model, tab, 0, "customers", []string{"id", "name"})
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 1, "orders", []string{"id", "customer_id"},
		query.ForeignKey{
			Columns: []string{"customer_id"}, TargetSchema: "shop",
			TargetTable: "customers", TargetColumns: []string{"id"},
		})
	if connection.Overlay.Kind != app.OverlayBuilderJoin {
		t.Fatalf("the second table opened %q", connection.Overlay.Kind)
	}

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	if kind := tab.Builder.Joins[0].Kind; kind != statement.JoinLeft {
		t.Errorf("the right arrow left the kind on %q", kind)
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyLeft})
	if kind := tab.Builder.Joins[0].Kind; kind != statement.JoinInner {
		t.Errorf("the left arrow left the kind on %q", kind)
	}

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if connection.Overlay.IsOpen() {
		t.Errorf("enter left %q open", connection.Overlay.Kind)
	}
	if !strings.Contains(tab.Builder.BuildSQL(connection.Session.Dialect()),
		"inner join \"shop\".\"orders\" o on o.customer_id = c.id") {
		t.Errorf("the builder wrote\n%s", tab.Builder.BuildSQL(connection.Session.Dialect()))
	}
}

// The join card takes a condition the user typed over the one of the foreign key.
func TestBuilderJoinCardTakesATypedCondition(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "customers"})
	writeBuilderTable(t, model, tab, 0, "customers", []string{"id"})
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 1, "orders", []string{"id", "customer_id"})

	if connection.Overlay.Kind != app.OverlayBuilderJoin {
		t.Fatalf("the second table opened %q", connection.Overlay.Kind)
	}
	connection.Overlay.Draft.SetText("o.customer_id = c.id and o.id > 0")
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	if !strings.Contains(tab.Builder.BuildSQL(connection.Session.Dialect()),
		"on o.customer_id = c.id and o.id > 0") {
		t.Errorf("the builder wrote\n%s", tab.Builder.BuildSQL(connection.Session.Dialect()))
	}
}

// The field card steps its rows and its values with the arrows, and Enter takes the name.
func TestBuilderFieldCardStepsItsRowsFromTheKeyboard(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 0, "orders", []string{"id", "total"})

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if connection.Overlay.Kind != app.OverlayBuilderField {
		t.Fatalf("enter opened %q", connection.Overlay.Kind)
	}

	// The first row is the aggregate, and the third is the sort.
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyRight})

	column := tab.Builder.Tables[0].Columns[0]
	if column.Aggregate != statement.AggregateCount {
		t.Errorf("the aggregate reads %q", column.Aggregate)
	}
	if column.Sort != core.SortAscending {
		t.Errorf("the sort reads %q", column.Sort)
	}

	connection.Overlay.Draft.SetText("orders")
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if connection.Overlay.IsOpen() {
		t.Errorf("enter left %q open", connection.Overlay.Kind)
	}
	if !strings.Contains(tab.Builder.BuildSQL(connection.Session.Dialect()),
		"count(o.id) as orders") {
		t.Errorf("the builder wrote\n%s", tab.Builder.BuildSQL(connection.Session.Dialect()))
	}
}

// The table picker filters as the user types, and Enter adds the row it stands on.
func TestBuilderTablePickerFiltersAndAdds(t *testing.T) {
	model, connection, tab := openBuilderTab(t)

	pressKey(t, model, tea.KeyPressMsg{Code: 't', Text: "t"})
	for _, key := range []tea.KeyPressMsg{
		{Code: 'o', Text: "o"}, {Code: 'r', Text: "r"}, {Code: 'd', Text: "d"},
	} {
		pressKey(t, model, key)
	}
	if kept := model.filterBuilderTables(connection.Overlay); len(kept) != 1 {
		t.Fatalf("the picker kept %d tables", len(kept))
	}

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(tab.Builder.Tables) != 1 || tab.Builder.Tables[0].Ref.Name != "orders" {
		t.Errorf("the builder holds %+v", tab.Builder.Tables)
	}
}

// The arrows move the cursor of the table picker, which needs the row count of the card.
func TestBuilderTablePickerMovesItsCursor(t *testing.T) {
	model, connection, tab := openBuilderTab(t)

	pressKey(t, model, tea.KeyPressMsg{Code: 't', Text: "t"})
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	if connection.Overlay.List.Cursor != 1 {
		t.Fatalf("down left the cursor on %d", connection.Overlay.List.Cursor)
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
	if connection.Overlay.List.Cursor != 0 {
		t.Errorf("up left the cursor on %d", connection.Overlay.List.Cursor)
	}

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(tab.Builder.Tables) != 1 || tab.Builder.Tables[0].Ref.Name != "orders" {
		t.Errorf("the picker added %+v", tab.Builder.Tables)
	}
}

// The bar at the foot of the frame names the keys of the builder, not the ones of an editor
// the tab does not hold.
func TestBuilderHintsNameTheKeysOfTheDiagram(t *testing.T) {
	model, _, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 0, "orders", []string{"id"})

	written := []string{}
	for _, hint := range model.BuildHints(HintContext{
		Pane: app.PaneEditor, TabKind: app.TabBuilder,
	}) {
		written = append(written, hint.Label)
	}
	held := strings.Join(written, " · ")
	for _, wanted := range []string{"table or join", "pick", "where", "run"} {
		if !strings.Contains(held, wanted) {
			t.Errorf("the bar reads %q, wanted %q in it", held, wanted)
		}
	}
	if strings.Contains(held, "select all") {
		t.Errorf("the bar names the keys of the editor: %q", held)
	}
}

// A run keeps the keyboard in the diagram, so the next change is one key away.
func TestBuilderKeepsTheKeyboardThroughARun(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 0, "orders", []string{"id"})

	model.runStatementAtCursor(connection, tab)

	if tab.Focus != app.PaneEditor {
		t.Errorf("the run moved the keyboard to %q", tab.Focus)
	}
}

// The field card marks the row the cursor stands on, so the arrows have something to move.
// The marker is what the keys change, and a card without one reads as a card that ignores
// them.
func TestBuilderFieldCardMarksTheRowItStandsOn(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 0, "orders", []string{"id", "total"})
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	marker := model.icons.Icon(cfg.IconField)
	rows := func() []string {
		return strings.Split(stripEscapes(
			model.renderBuilderField(tab, connection.Overlay, 72)), "\n")
	}

	// The card opens on the aggregate, and each press of Down moves the marker one row.
	for _, held := range []struct {
		label string
		press bool
	}{{"aggregate", false}, {"name", true}, {"sort", true}} {
		if held.press {
			pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
		}
		found := false
		for _, line := range rows() {
			if strings.Contains(line, marker) && strings.Contains(line, held.label) {
				found = true
			}
		}
		if !found {
			t.Errorf("the card marks no %s row:\n%s", held.label, strings.Join(rows(), "\n"))
		}
	}

	// The arrows step the row the marker stands on, and nothing else.
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	column := tab.Builder.Tables[0].Columns[0]
	if column.Sort != core.SortAscending || column.Aggregate != statement.AggregateNone {
		t.Errorf("the column holds aggregate %q and sort %q", column.Aggregate, column.Sort)
	}
}

// The join card marks the row the cursor stands on, and the arrows move the marker as they
// do on every other form of the client.
func TestBuilderJoinCardMarksTheRowItStandsOn(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "customers"})
	writeBuilderTable(t, model, tab, 0, "customers", []string{"id"})
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 1, "orders", []string{"id", "customer_id"})
	if connection.Overlay.Kind != app.OverlayBuilderJoin {
		t.Fatalf("the second table opened %q", connection.Overlay.Kind)
	}

	marker := model.icons.Icon(cfg.IconField)
	markedRow := func() string {
		for _, line := range strings.Split(stripEscapes(
			model.renderBuilderJoin(tab, connection.Overlay, 72)), "\n") {
			if strings.Contains(line, marker) {
				return strings.TrimSpace(strings.ReplaceAll(line, "│", ""))
			}
		}
		return ""
	}

	// The card opens on the kind, where the arrows step it.
	if held := markedRow(); !strings.HasPrefix(held, marker+" join") {
		t.Fatalf("the card opened on %q", held)
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	if kind := tab.Builder.Joins[0].Kind; kind != statement.JoinLeft {
		t.Errorf("the arrow left the kind on %q", kind)
	}

	// Down moves the marker onto the condition, where the arrows move the caret instead.
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	if held := markedRow(); !strings.HasPrefix(held, marker+" on") {
		t.Fatalf("down left the marker on %q", held)
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyRight})
	if kind := tab.Builder.Joins[0].Kind; kind != statement.JoinLeft {
		t.Errorf("the arrow on the condition row stepped the kind to %q", kind)
	}

	// Up brings it back, and the kind steps again.
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyUp})
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyLeft})
	if kind := tab.Builder.Joins[0].Kind; kind != statement.JoinInner {
		t.Errorf("the arrow left the kind on %q", kind)
	}
}

// A typed character lands in the condition, whatever row the marker stands on, and the
// marker moves onto the row that holds it.
func TestBuilderJoinCardTypesIntoTheCondition(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "customers"})
	writeBuilderTable(t, model, tab, 0, "customers", []string{"id"})
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 1, "orders", []string{"id", "customer_id"})

	connection.Overlay.Draft.SetText("")
	for _, key := range []tea.KeyPressMsg{
		{Code: 'a', Text: "a"}, {Code: '=', Text: "="}, {Code: 'b', Text: "b"},
	} {
		pressKey(t, model, key)
	}

	if written := connection.Overlay.Draft.Text; written != "a=b" {
		t.Errorf("the condition reads %q", written)
	}
	if connection.Overlay.List.Cursor != builderJoinOnRow {
		t.Errorf("the marker stands on row %d", connection.Overlay.List.Cursor)
	}

	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(tab.Builder.BuildSQL(connection.Session.Dialect()), " on a=b") {
		t.Errorf("the builder wrote\n%s", tab.Builder.BuildSQL(connection.Session.Dialect()))
	}
}

// A press on the title of a box marks that table, and a press on a join, a field or a
// filter row opens the card of the row it looks like.
func TestBuilderPressOpensTheRowUnderThePointer(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "customers"})
	writeBuilderTable(t, model, tab, 0, "customers", []string{"id", "name"})
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 1, "orders", []string{"id", "customer_id"},
		query.ForeignKey{
			Columns: []string{"customer_id"}, TargetSchema: "shop",
			TargetTable: "customers", TargetColumns: []string{"id"},
		})
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})
	tab.Builder.Tables[0].Columns[1].Picked = true
	tab.Builder.Filters = []string{"c.name is not null"}

	press := func(want builderPress, row int) {
		t.Helper()
		model.renderBuilder(connection, tab, 118, 30)
		block := model.layout.builderRows
		for at, held := range model.builderRows {
			if held.press != want || held.row != row {
				continue
			}
			model.pressBuilderRow(connection, tab, tea.Mouse{
				X: block.from + 2, Y: block.top + at - block.offset,
			}, at)
			return
		}
		t.Fatalf("the pane draws no %s row %d", want, row)
	}

	press(pressesJoin, 0)
	if connection.Overlay.Kind != app.OverlayBuilderJoin {
		t.Errorf("the join row opened %q", connection.Overlay.Kind)
	}
	connection.CloseEveryOverlay()

	press(pressesField, builderFieldStride*0+1)
	if connection.Overlay.Kind != app.OverlayBuilderField {
		t.Errorf("the field row opened %q", connection.Overlay.Kind)
	}
	connection.CloseEveryOverlay()

	press(pressesFilter, 0)
	if connection.Overlay.Prompt != app.PromptBuilderFilter {
		t.Errorf("the filter row opened %q", connection.Overlay.Prompt)
	}
}

// A press on the name of a box marks that table, so the keys that follow act on it.
func TestBuilderPressMarksTheTableOfTheTitle(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "customers"})
	writeBuilderTable(t, model, tab, 0, "customers", []string{"id"})
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 1, "orders", []string{"id"})
	connection.CloseEveryOverlay()
	tab.Builder.Table = 0

	model.renderBuilder(connection, tab, 118, 30)
	block := model.layout.builderRows
	for at, held := range model.builderRows {
		for _, title := range held.titles {
			if title.Box != 1 {
				continue
			}
			model.pressBuilderRow(connection, tab, tea.Mouse{
				X: block.from + 1 + title.X, Y: block.top + at - block.offset,
			}, at)
		}
	}

	if tab.Builder.Table != 1 {
		t.Errorf("the press left the cursor on table %d", tab.Builder.Table)
	}
}

// The height of the pane belongs to the tab, so a drag of the split line in one tab leaves
// every other tab as it was.
func TestSplitDragKeepsTheHeightOfEveryOtherTab(t *testing.T) {
	model, connection, builder := openBuilderTab(t)
	query := connection.OpenQueryTab("select 1")
	model.View()

	// The line is dragged in the query tab.
	model.layout.editorTop, model.layout.editorRows, model.layout.resultRows = 3, 10, 10
	model.dragSplit(tea.Mouse{Y: 3 + 7 - 1})
	if query.PaneHeight != 7 {
		t.Fatalf("the query tab keeps %d rows", query.PaneHeight)
	}
	if builder.PaneHeight != 0 {
		t.Errorf("the drag changed the builder tab to %d rows", builder.PaneHeight)
	}

	// And again in the builder tab, which keeps its own height.
	connection.ActiveIndex = 0
	model.dragSplit(tea.Mouse{Y: 3 + 14 - 1})
	if builder.PaneHeight != 14 || query.PaneHeight != 7 {
		t.Errorf("the tabs keep %d and %d rows", builder.PaneHeight, query.PaneHeight)
	}
}

// The marks of a choice row are pressable: a press on one steps the value, as the arrows do.
// The marks are recorded while the card is drawn, so the test presses where they landed.
func TestBuilderCardMarksTakeAPress(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "customers"})
	writeBuilderTable(t, model, tab, 0, "customers", []string{"id"})
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 1, "orders", []string{"id", "customer_id"},
		query.ForeignKey{
			Columns: []string{"customer_id"}, TargetSchema: "shop",
			TargetTable: "customers", TargetColumns: []string{"id"},
		})
	if connection.Overlay.Kind != app.OverlayBuilderJoin {
		t.Fatalf("the second table opened %q", connection.Overlay.Kind)
	}

	press := func(x, y int) {
		t.Helper()
		model.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		model.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	}

	model.View()
	if len(model.layout.formChoices) != 1 {
		t.Fatalf("the card recorded %d marks", len(model.layout.formChoices))
	}
	mark := model.layout.formChoices[0]
	press(mark.on, mark.row)
	if kind := tab.Builder.Joins[0].Kind; kind != statement.JoinLeft {
		t.Errorf("the mark stepped the kind to %q", kind)
	}
	model.View()
	mark = model.layout.formChoices[0]
	press(mark.back, mark.row)
	if kind := tab.Builder.Joins[0].Kind; kind != statement.JoinInner {
		t.Errorf("the other mark stepped the kind to %q", kind)
	}

	// A press on a row of the card marks that row.
	model.View()
	block := model.layout.formRows
	press(block.from+2, block.top+builderJoinOnRow)
	if connection.Overlay.List.Cursor != builderJoinOnRow {
		t.Errorf("the press left the marker on row %d", connection.Overlay.List.Cursor)
	}
}

// The rows of the field card take a press as well, and the marks of its two choice rows
// step the aggregate and the sort.
func TestBuilderFieldCardTakesAPress(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 0, "orders", []string{"id", "total"})
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEnter})

	press := func(x, y int) {
		t.Helper()
		model.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		model.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	}

	model.View()
	mark := model.layout.formChoices[0]
	press(mark.on, mark.row)
	if held := tab.Builder.Tables[0].Columns[0].Aggregate; held != statement.AggregateCount {
		t.Errorf("the mark stepped the aggregate to %q", held)
	}

	model.View()
	block := model.layout.formRows
	press(block.from+2, block.top+builderFieldSort)
	if connection.Overlay.Field != builderFieldSort {
		t.Errorf("the press left the marker on row %d", connection.Overlay.Field)
	}
	model.View()
	mark = model.layout.formChoices[0]
	press(mark.on, mark.row)
	if held := tab.Builder.Tables[0].Columns[0].Sort; held != core.SortAscending {
		t.Errorf("the mark stepped the sort to %q", held)
	}
}

// A join with no condition is a cross join, which every server reads. An inner join with no
// condition is a statement the server refuses.
func TestBuilderWritesACrossJoinWithoutACondition(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "customers"})
	writeBuilderTable(t, model, tab, 0, "customers", []string{"id"})
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "orders"})
	writeBuilderTable(t, model, tab, 1, "orders", []string{"id"})

	// The card of a join without a foreign key opens empty, and Esc leaves it that way.
	if connection.Overlay.Kind != app.OverlayBuilderJoin {
		t.Fatalf("the second table opened %q", connection.Overlay.Kind)
	}
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyEscape})

	written := tab.Builder.BuildSQL(connection.Session.Dialect())
	if !strings.Contains(written, `cross join "shop"."orders" o`) {
		t.Errorf("the builder wrote\n%s", written)
	}
	if strings.Contains(written, "inner join") {
		t.Errorf("the builder wrote a join with no condition:\n%s", written)
	}

	// The joins section says what the SQL writes.
	drawn := stripEscapes(strings.Join(model.renderBuilder(connection, tab, 118, 30), "\n"))
	if !strings.Contains(drawn, "cross") {
		t.Errorf("the joins section reads:\n%s", drawn)
	}
}

// The pane follows the cursor, so a table taller than the pane still shows the column the
// keys move to.
func TestBuilderPaneFollowsTheCursor(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "wide"})
	columns := []string{}
	for at := 'a'; at <= 'z'; at++ {
		columns = append(columns, string(at))
	}
	writeBuilderTable(t, model, tab, 0, "wide", columns)

	height := 12
	model.renderBuilder(connection, tab, 118, height)
	if tab.Builder.Offset != 0 {
		t.Fatalf("the pane opened at row %d", tab.Builder.Offset)
	}

	for range columns {
		pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	drawn := stripEscapes(strings.Join(
		model.renderBuilder(connection, tab, 118, height), "\n"))
	if !strings.Contains(drawn, "[ ] z") {
		t.Errorf("the last column is not drawn:\n%s", drawn)
	}
	if tab.Builder.Offset == 0 {
		t.Error("the pane did not follow the cursor")
	}
}

// A server that joins no tables in one statement has no builder.
func TestBuilderIsRefusedWhereTheEngineJoinsNothing(t *testing.T) {
	if AnswersFor(core.Capabilities{JoinsTables: true}, NeedsJoinsTables) != true {
		t.Error("a server that joins tables answers no builder")
	}
	if AnswersFor(core.Capabilities{}, NeedsJoinsTables) != false {
		t.Error("a server that joins nothing answers a builder")
	}
	for _, engine := range []core.Engine{
		core.EnginePostgres, core.EngineMysql, core.EngineSqlserver,
		core.EngineClickhouse, core.EngineSqlite,
	} {
		if !core.ResolveEngineInfo(engine).Capabilities.JoinsTables {
			t.Errorf("%s joins no tables", engine)
		}
	}
	if core.ResolveEngineInfo(core.EngineMongo).Capabilities.JoinsTables {
		t.Error("mongodb joins tables in one statement")
	}
}

// The wheel moves the pane down its rows and the diagram along its boxes, and the pane goes
// back to following the cursor at the next key that moves it.
func TestBuilderWheelScrollsBothWays(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	for _, name := range []string{"customers", "orders", "order_items", "products"} {
		tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: name})
		writeBuilderTable(t, model, tab, len(tab.Builder.Tables)-1, name,
			[]string{"id", "a", "b", "c", "d", "e"})
		connection.CloseEveryOverlay()
	}
	tab.Builder.MoveCursor(0, 0)
	model.View()

	roll := func(button tea.MouseButton) {
		t.Helper()
		model.Update(tea.MouseWheelMsg{
			X: model.editorLeft + 4, Y: model.layout.editorTop + 2, Button: button,
		})
	}

	roll(tea.MouseWheelRight)
	roll(tea.MouseWheelRight)
	if tab.Builder.ColumnOffset != 2*wheelColumns {
		t.Fatalf("the wheel left the diagram at column %d", tab.Builder.ColumnOffset)
	}
	if !tab.Builder.Rolled {
		t.Error("the wheel did not mark the pane as rolled")
	}
	model.View()
	if tab.Builder.ColumnOffset != 2*wheelColumns {
		t.Errorf("the frame pulled the diagram back to column %d", tab.Builder.ColumnOffset)
	}

	roll(tea.MouseWheelLeft)
	if tab.Builder.ColumnOffset != wheelColumns {
		t.Errorf("the other way left the diagram at column %d", tab.Builder.ColumnOffset)
	}

	// The wheel never runs before the first box.
	for range 10 {
		roll(tea.MouseWheelLeft)
	}
	if tab.Builder.ColumnOffset != 0 {
		t.Errorf("the wheel ran to column %d", tab.Builder.ColumnOffset)
	}

	// A key that moves the cursor takes the pane back to it.
	roll(tea.MouseWheelRight)
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyDown})
	if tab.Builder.Rolled {
		t.Error("a key that moved the cursor left the pane rolled")
	}
	model.View()
	if tab.Builder.ColumnOffset != 0 {
		t.Errorf("the pane did not follow the cursor back to column %d",
			tab.Builder.ColumnOffset)
	}
}

// The wheel over the pane moves its rows, not the editor of another tab.
func TestBuilderWheelScrollsItsOwnRows(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: "wide"})
	columns := []string{}
	for at := 'a'; at <= 'z'; at++ {
		columns = append(columns, string(at))
	}
	writeBuilderTable(t, model, tab, 0, "wide", columns)
	connection.CloseEveryOverlay()
	model.View()

	model.Update(tea.MouseWheelMsg{
		X: model.editorLeft + 4, Y: model.layout.editorTop + 2,
		Button: tea.MouseWheelDown,
	})
	if tab.Builder.Offset != wheelRows {
		t.Errorf("the wheel left the pane at row %d", tab.Builder.Offset)
	}
	if tab.EditorRowOffset != 0 {
		t.Errorf("the wheel moved the editor of the tab to row %d", tab.EditorRowOffset)
	}
}

// A terminal with no wheel of its own for the other axis sends Shift with the one it has.
func TestBuilderShiftWheelScrollsSideways(t *testing.T) {
	model, connection, tab := openBuilderTab(t)
	for _, name := range []string{"customers", "orders", "order_items"} {
		tab.Builder.AddTable(db.TableRef{Schema: "shop", Name: name})
		writeBuilderTable(t, model, tab, len(tab.Builder.Tables)-1, name, []string{"id", "a"})
		connection.CloseEveryOverlay()
	}
	tab.Builder.MoveCursor(0, 0)
	model.View()

	model.Update(tea.MouseWheelMsg{
		X: model.editorLeft + 4, Y: model.layout.editorTop + 2,
		Button: tea.MouseWheelDown, Mod: uv.ModShift,
	})

	if tab.Builder.ColumnOffset != wheelColumns {
		t.Errorf("shift and the wheel left the diagram at column %d", tab.Builder.ColumnOffset)
	}
	if tab.Builder.Offset != 0 {
		t.Errorf("shift and the wheel moved the rows to %d", tab.Builder.Offset)
	}
}
