package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
)

// buildDescribedTable answers a model whose table tab was opened on the columns view.
func buildDescribedTable(t *testing.T) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	row := present.TreeRow{Node: present.TreeNode{
		Kind:  present.NodeTable,
		Table: db.TableRef{Schema: "public", Name: "orders", Kind: db.RelationTable},
	}}
	model.describeTable(connection, row)
	tab := connection.Active()
	if tab.Kind != app.TabTable {
		t.Fatalf("the active tab is a %v tab, wanted the table tab", tab.Kind)
	}
	return model, connection, tab
}

// The rows view of a described table reads the rows.
func TestTheRowsViewOfADescribedTableReadsTheRows(t *testing.T) {
	model, connection, tab := buildDescribedTable(t)

	_, command := model.selectViewAt(connection, tab, 0)

	if command == nil || tab.Results.State().Kind != app.QueryRunning {
		t.Errorf("the rows view left the read %v", tab.Results.State().Kind)
	}
}

// A described table draws the view strip before its rows are read.
func TestADescribedTableDrawsTheViewStrip(t *testing.T) {
	model, _, _ := buildDescribedTable(t)

	frame := stripEscapes(model.render())

	if !strings.Contains(frame, "indexes") {
		t.Errorf("the frame has no view strip:\n%s", frame)
	}
}
