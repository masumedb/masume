package ui

import (
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/cfg"
	"github.com/masumedb/masume/internal/core"
)

// placeTreeCursor puts the cursor of the tree on the row of that id.
func placeTreeCursor(t *testing.T, model *Model, connection *app.Connection, id string) {
	t.Helper()
	for at, row := range model.treeRows(connection) {
		if row.ID == id {
			connection.Tree.Cursor = at
			return
		}
	}
	t.Fatalf("the tree has no row %q", id)
}

// assertTreeCursorOn fails unless the cursor of the tree is on the row of that id.
func assertTreeCursorOn(t *testing.T, model *Model, connection *app.Connection, id string) {
	t.Helper()
	row, found := model.treeRowAt(connection, model.treeRows(connection))
	if !found || row.ID != id {
		t.Errorf("the cursor is on %q, wanted %q", row.ID, id)
	}
}

// Rows inserted above the cursor leave the cursor on its row.
func TestTheTreeCursorStaysOnItsRow(t *testing.T) {
	cases := []struct {
		name   string
		action ActionID
	}{
		{"unfold a schema", ActionUnfoldRow},
		{"open a schema", ActionOpenNode},
		{"mark a favourite", ActionToggleFavourite},
	}
	for _, each := range cases {
		t.Run(each.name, func(t *testing.T) {
			model := buildLoadedModel(t, 3, 2, 1, 1)
			connection := model.Active()
			schema := core.BuildSchemaID("schema_01")
			placeTreeCursor(t, model, connection, schema)

			model.runTreeAction(connection, Match{Action: each.action, Scope: cfg.ScopeTree})

			assertTreeCursorOn(t, model, connection, schema)
		})
	}
}

// Marks read after the tree was drawn leave the cursor on its row.
func TestTheTreeCursorStaysOnItsRowWhenMarksArrive(t *testing.T) {
	model := buildLoadedModel(t, 3, 2, 1, 1)
	connection := model.Active()
	schema := core.BuildSchemaID("schema_01")
	placeTreeCursor(t, model, connection, schema)

	model.readMarksAnswer(marksReadMsg{
		ConnectionID: model.ActiveID(),
		Recent:       []core.RecentSchema{{Schema: "schema_02"}},
	})

	assertTreeCursorOn(t, model, connection, schema)
}
