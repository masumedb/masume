package ui

import (
	"strings"
	"testing"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
)

// The server numbers an identity column itself, so the form of a new row leaves it out.
func TestTheFormOfANewRowLeavesOutAnIdentityColumn(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()
	tab.Focus = app.PaneResult
	tab.Results.Start([]string{"select * from tickets"}, 200)
	tab.Results.Succeed(0,
		db.ComposedRead{Text: "select * from tickets", Display: "select * from tickets"},
		db.QueryResult{
			Columns: []db.ResultColumn{
				{Name: "id", DataType: "integer"}, {Name: "note", DataType: "text"},
			},
			Rows: [][]any{{int64(1), "first"}},
		})
	tab.Target = app.EditTarget{
		Table:    db.TableRef{Schema: "public", Name: "tickets"},
		Editable: true, KeyColumns: []string{"id"},
		Columns: []db.ColumnDetail{
			{Name: "id", DataType: "integer", IsPrimaryKey: true, IsIdentityAlways: true},
			{Name: "note", DataType: "text"},
		},
	}
	model.render()

	model.insertRow(connection, tab, model.buildGridShape(connection, tab))
	overlay := connection.Overlay
	if overlay.Kind != app.OverlayCellEdit {
		t.Fatalf("the form did not open: %q", overlay.Kind)
	}
	written := overlay.Draft.Text
	if strings.Contains(written, `"id"`) {
		t.Errorf("the form of a new row holds the identity column:\n%s", written)
	}
	if !strings.Contains(written, `"note"`) {
		t.Errorf("the form holds no writable column:\n%s", written)
	}
}

// A copy of a row is a new row, so the server numbers it itself and the staged insert names
// no identity column.
func TestACopiedRowLeavesOutTheIdentityColumn(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()
	tab.Focus = app.PaneResult
	tab.Results.Start([]string{"select * from tickets"}, 200)
	tab.Results.Succeed(0,
		db.ComposedRead{Text: "select * from tickets", Display: "select * from tickets"},
		db.QueryResult{
			Columns: []db.ResultColumn{
				{Name: "code", DataType: "integer"}, {Name: "note", DataType: "text"},
			},
			Rows: [][]any{{int64(7), "first"}},
		})
	tab.Target = app.EditTarget{
		Table:    db.TableRef{Schema: "public", Name: "tickets"},
		Editable: true, KeyColumns: []string{"note"},
		Columns: []db.ColumnDetail{
			{Name: "code", DataType: "integer", IsIdentityAlways: true},
			{Name: "note", DataType: "text", IsPrimaryKey: true},
		},
	}
	model.render()

	model.duplicateRow(connection, tab, model.buildGridShape(connection, tab))
	if len(tab.Pending.Inserts) != 1 {
		t.Fatalf("the copy staged %d rows", len(tab.Pending.Inserts))
	}
	if _, held := tab.Pending.Inserts[0]["code"]; held {
		t.Errorf("the copy holds the identity column: %v", tab.Pending.Inserts[0])
	}
}
