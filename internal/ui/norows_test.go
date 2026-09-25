package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/db"
)

func TestAResultWithNoRowsSaysSoUnderItsHeader(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()
	tab.Results.Start([]string{"select id from users where 1 = 0"}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select id from users where 1 = 0"},
		db.QueryResult{Columns: []db.ResultColumn{{Name: "id", DataType: "integer"}}})
	tab.Focus = app.PaneResult

	lines := model.renderGrid(connection, tab, model.buildGridShape(connection, tab), 80, 12)
	if !strings.Contains(stripEscapes(lines[0]), "id") {
		t.Errorf("the header reads %q", stripEscapes(lines[0]))
	}
	if drawn := stripEscapes(strings.Join(lines[1:], "\n")); !strings.Contains(drawn, "no rows") {
		t.Errorf("the grid under the header reads\n%s", drawn)
	}
}
