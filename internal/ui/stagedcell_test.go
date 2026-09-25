package ui

import (
	"strings"
	"testing"

	"github.com/masumedb/masume/internal/app"
	"github.com/masumedb/masume/internal/core"
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
