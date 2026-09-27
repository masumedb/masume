package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/query"
)

// openOrdersRowCard opens the row card on the first row of orders, with a foreign key on
// customer_id.
func openOrdersRowCard(t *testing.T, truncated bool) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model, connection, _ := buildPlannedModel(t)
	connection.Catalog.Tables = []db.TableRef{
		{Schema: "public", Name: "orders", Kind: db.RelationTable},
		{Schema: "public", Name: "customers", Kind: db.RelationTable},
	}
	tab := connection.Active()
	tab.Results.Start([]string{"select * from orders"}, 200)
	tab.Results.Succeed(0,
		db.ComposedRead{Text: "select * from orders", Display: "select * from orders"},
		db.QueryResult{
			Columns: []db.ResultColumn{
				{Name: "id", DataType: "integer"}, {Name: "customer_id", DataType: "integer"},
				{Name: "status", DataType: "text"},
			},
			Rows: [][]any{{int64(1), int64(7), "open"}}, Truncated: truncated,
		})
	tab.Target = app.EditTarget{
		Table:    db.TableRef{Schema: "public", Name: "orders"},
		Editable: true, KeyColumns: []string{"id"},
		ForeignKeys: []query.ForeignKey{{
			Columns: []string{"customer_id"}, TargetSchema: "public",
			TargetTable: "customers", TargetColumns: []string{"id"},
		}},
	}
	tab.Focus = app.PaneResult
	model.runGridAction(connection, tab, Match{Action: ActionOpenRow})
	if connection.Overlay.Kind != app.OverlayRowDetail {
		t.Fatalf("the grid opened %q", connection.Overlay.Kind)
	}
	return model, connection, tab
}

func TestTheRowCardSaysTheRowsAreLoaded(t *testing.T) {
	model, _, _ := openOrdersRowCard(t, true)
	if screen := stripEscapes(model.render()); !strings.Contains(screen, "row 1 of 1 loaded") {
		t.Errorf("the title does not say the rows are loaded:\n%s", screen)
	}
	model, _, _ = openOrdersRowCard(t, false)
	if screen := stripEscapes(model.render()); strings.Contains(screen, "loaded") {
		t.Errorf("a whole result says loaded:\n%s", screen)
	}
}

func TestTheRowCardFollowsTheForeignKeyUnderTheCursor(t *testing.T) {
	model, connection, _ := openOrdersRowCard(t, false)
	model.render()
	model.readKey(tea.Key{Code: tea.KeyDown})
	if connection.Overlay.List.Cursor != 1 {
		t.Fatalf("the cursor is on field %d", connection.Overlay.List.Cursor)
	}
	if screen := stripEscapes(model.render()); !strings.Contains(screen, "follow key") {
		t.Errorf("the card offers no key to follow:\n%s", screen)
	}

	model.readKey(tea.Key{Code: 'g', Text: "g"})
	if connection.Overlay.IsOpen() {
		t.Fatalf("the card %q is still open", connection.Overlay.Kind)
	}
	opened := connection.Active()
	if opened.Table.Name != "customers" || len(opened.Filter) != 1 {
		t.Errorf("the active tab is %q with %+v", opened.Table.Name, opened.Filter)
	}
}

func TestTheRowCardEditsTheFieldUnderTheCursor(t *testing.T) {
	model, connection, _ := openOrdersRowCard(t, false)
	model.render()
	model.readKey(tea.Key{Code: tea.KeyDown})
	model.readKey(tea.Key{Code: tea.KeyDown})
	model.readKey(tea.Key{Code: 'e', Text: "e"})
	if connection.Overlay.Kind != app.OverlayCellEdit ||
		connection.Overlay.Cell.Column.Name != "status" {
		t.Errorf("the card opened %q for %q",
			connection.Overlay.Kind, connection.Overlay.Cell.Column.Name)
	}
}

func TestTheRowCardKeepsTheScrollOfTheWheel(t *testing.T) {
	model, connection, _ := openOrdersRowCard(t, false)
	connection.Overlay.List.Offset, connection.Overlay.List.Rolled = 1, true
	model.render()
	if connection.Overlay.List.Offset != 1 {
		t.Errorf("drawing moved the offset to %d", connection.Overlay.List.Offset)
	}
}
