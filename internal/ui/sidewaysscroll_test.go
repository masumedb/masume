package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/turanmahmudov/masume/internal/app"
	"github.com/turanmahmudov/masume/internal/db"
)

// buildWideModel answers a model whose result holds more columns than the pane can draw.
func buildWideModel(t *testing.T, columns int) (*Model, *app.Connection, *app.Tab) {
	t.Helper()
	model := buildOfflineModel(t, 160, 48)
	connection := model.Active()
	tab := connection.Active()

	names := make([]db.ResultColumn, 0, columns)
	values := make([]any, 0, columns)
	for at := range columns {
		names = append(names, db.ResultColumn{
			Name: fmt.Sprintf("column_%d", at), DataType: "text",
		})
		values = append(values, fmt.Sprintf("value %d %s", at, strings.Repeat("-", 30)))
	}
	tab.Results.Start([]string{"select * from wide"}, 200)
	tab.Results.Succeed(0,
		db.ComposedRead{Text: "select * from wide", Display: "select * from wide"},
		db.QueryResult{Columns: names, Rows: [][]any{values}})
	model.View()
	return model, connection, tab
}

// The wheel of a trackpad moves the grid along its columns, and the grid goes back to
// following the cursor at the next move of it.
func TestWheelScrollsTheGridSideways(t *testing.T) {
	model, _, tab := buildWideModel(t, 20)

	roll := func(button tea.MouseButton) {
		t.Helper()
		model.Update(tea.MouseWheelMsg{
			X: model.editorLeft + 4, Y: model.layout.resultTop + 1, Button: button,
		})
	}

	roll(tea.MouseWheelRight)
	roll(tea.MouseWheelRight)
	if tab.GridColumnOffset != 2 {
		t.Fatalf("the wheel left the grid at column %d", tab.GridColumnOffset)
	}
	if !tab.GridColumnRolled {
		t.Error("the wheel did not mark the columns as rolled")
	}
	model.View()
	if tab.GridColumnOffset != 2 {
		t.Errorf("the frame pulled the grid back to column %d", tab.GridColumnOffset)
	}
	if first := model.layout.gridColumns[0].index; first != 2 {
		t.Errorf("the grid draws column %d first, wanted the third", first)
	}

	roll(tea.MouseWheelLeft)
	if tab.GridColumnOffset != 1 {
		t.Errorf("the other way left the grid at column %d", tab.GridColumnOffset)
	}

	// The wheel never runs before the first column.
	for range 5 {
		roll(tea.MouseWheelLeft)
	}
	if tab.GridColumnOffset != 0 {
		t.Errorf("the wheel ran to column %d", tab.GridColumnOffset)
	}

	// A key that moves the cursor takes the window back to it. The cursor stands on the
	// second column, and the window opens there.
	for range 6 {
		roll(tea.MouseWheelRight)
	}
	model.View()
	model.runGridAction(model.Active(), tab, Match{Action: ActionCursorRight})
	if tab.GridColumnRolled {
		t.Error("a key that moved the cursor left the columns rolled")
	}
	model.View()
	if tab.GridColumnOffset != 1 {
		t.Errorf("the window did not follow the cursor back, it stands at %d",
			tab.GridColumnOffset)
	}
}

// A terminal with no sideways wheel sends Shift with the one it has.
func TestShiftWheelScrollsTheGridSideways(t *testing.T) {
	model, _, tab := buildWideModel(t, 20)

	model.Update(tea.MouseWheelMsg{
		X: model.editorLeft + 4, Y: model.layout.resultTop + 1,
		Button: tea.MouseWheelDown, Mod: uv.ModShift,
	})
	if tab.GridColumnOffset != 1 {
		t.Errorf("shift and the wheel left the grid at column %d", tab.GridColumnOffset)
	}
	if tab.GridRowOffset != 0 {
		t.Errorf("shift and the wheel moved the rows to %d", tab.GridRowOffset)
	}
}

// The wheel moves the statement along the cells of its lines, and the pane goes back to
// following the caret at the next key.
func TestWheelScrollsTheStatementSideways(t *testing.T) {
	model, _, tab := buildEditingModel(t, strings.Repeat("a", 300), 0)
	model.View()

	roll := func(button tea.MouseButton) {
		t.Helper()
		model.Update(tea.MouseWheelMsg{
			X: model.editorLeft + 4, Y: model.layout.editorTop + 2, Button: button,
		})
	}

	roll(tea.MouseWheelRight)
	roll(tea.MouseWheelRight)
	if tab.EditorColumnOffset != 2*wheelColumns {
		t.Fatalf("the wheel left the statement at cell %d", tab.EditorColumnOffset)
	}
	model.View()
	if tab.EditorColumnOffset != 2*wheelColumns {
		t.Errorf("the frame pulled the statement back to cell %d", tab.EditorColumnOffset)
	}

	roll(tea.MouseWheelLeft)
	if tab.EditorColumnOffset != wheelColumns {
		t.Errorf("the other way left the statement at cell %d", tab.EditorColumnOffset)
	}

	// The wheel never runs before the first cell of a line.
	for range 5 {
		roll(tea.MouseWheelLeft)
	}
	if tab.EditorColumnOffset != 0 {
		t.Errorf("the wheel ran to cell %d", tab.EditorColumnOffset)
	}

	// A key takes the pane back to the caret, which stands on the first cell.
	roll(tea.MouseWheelRight)
	pressKey(t, model, tea.KeyPressMsg{Code: tea.KeyLeft})
	if tab.EditorRolled {
		t.Error("a key left the pane rolled")
	}
	model.View()
	if tab.EditorColumnOffset != 0 {
		t.Errorf("the pane did not follow the caret back, it stands at cell %d",
			tab.EditorColumnOffset)
	}
}
