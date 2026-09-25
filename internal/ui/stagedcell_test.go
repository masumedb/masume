package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/core"
	"github.com/masumedb/masume/internal/db"
	"github.com/masumedb/masume/internal/present"
)

func TestAStagedNullAndAStagedEmptyTextLookLikeTheOnesTheServerRead(t *testing.T) {
	model, connection, tab := buildGridModel(t)
	tab.Pending.Edits = map[string]core.CellEdit{
		core.BuildEditKey(0, 1): {RowIndex: 0, ColumnIndex: 1, Value: core.CellValue{Kind: core.CellNull}},
		core.BuildEditKey(1, 1): {RowIndex: 1, ColumnIndex: 1, Value: core.CellValue{Kind: core.CellEmpty}},
	}
	shape := model.buildGridShape(connection, tab)

	for row, wanted := range map[int]string{0: present.NullDisplay, 1: present.EmptyTextDisplay} {
		drawn := stripEscapes(model.renderGridRow(tab, shape, []int{0, 1}, row, 3, 60))
		if !strings.Contains(drawn, wanted) {
			t.Errorf("the staged row %d reads %q, wanted %q", row, drawn, wanted)
		}
	}
	if drawn := stripEscapes(model.renderGridRow(tab, shape, []int{0, 1}, 0, 3, 60)); strings.Contains(drawn, core.NullText) {
		t.Errorf("the staged null reads %q", drawn)
	}
}

func TestTheCursorRowStaysMarkedWhenTheGridLosesTheFocus(t *testing.T) {
	model, connection, tab := buildGridModel(t)
	tab.Focus = app.PaneEditor
	shape := model.buildGridShape(connection, tab)

	tab.GridRow = 1
	marked := model.renderGridRow(tab, shape, []int{0, 1}, 1, 3, 60)
	tab.GridRow = 0
	plain := model.renderGridRow(tab, shape, []int{0, 1}, 1, 3, 60)
	if marked == plain {
		t.Error("the cursor row is drawn like any other row while the editor has the focus")
	}
}

func TestTheGridGutterWritesRowNumbersAsTheFooterDoes(t *testing.T) {
	model, connection, tab := buildGridModel(t)
	shape := model.buildGridShape(connection, tab)
	shape.RowIndexes = []int{12344, 12345, 12346}
	if drawn := stripEscapes(model.renderGridRow(tab, shape, []int{0}, 0, 8, 60)); !strings.Contains(drawn, "12,345") {
		t.Errorf("the gutter reads %q", drawn)
	}
}

func TestTheDecimalPointsOfANumberColumnLineUp(t *testing.T) {
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()
	tab.Results.Start([]string{"select revenue from t"}, 200)
	tab.Results.Succeed(0, db.ComposedRead{Text: "select revenue from t"}, db.QueryResult{
		Columns: []db.ResultColumn{{Name: "revenue", DataType: "numeric"}},
		Rows:    [][]any{{"51395.99"}, {"51140.3"}, {"496"}},
	})
	shape := model.buildGridShape(connection, tab)

	points := map[int]bool{}
	for at := range 3 {
		drawn := stripEscapes(model.renderGridRow(tab, shape, []int{0}, at, 3, 40))
		if at == 2 {
			if !strings.Contains(drawn, "496   ") {
				t.Errorf("the whole number reads %q", drawn)
			}
			continue
		}
		points[strings.Index(drawn, ".")] = true
	}
	if len(points) != 1 {
		t.Errorf("the points stand in %d different columns", len(points))
	}
}
