package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/masumedb/masume/internal/app"
)

// drawnColumns returns the result columns the grid drew, in order.
func drawnColumns(model *Model) []int {
	model.render()
	drawn := []int{}
	for _, column := range model.layout.gridColumns {
		drawn = append(drawn, column.index)
	}
	return drawn
}

func TestHideColumnLeavesTheColumnOffTheGrid(t *testing.T) {
	model := buildLoadedModel(t, 1, 2, 12, 4)
	tab := model.Active().Active()
	tab.Focus = app.PaneResult
	tab.GridColumn = 1

	model.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if drawn := drawnColumns(model); slices.Contains(drawn, 1) || tab.GridColumn != 2 {
		t.Errorf("h drew %v with the cursor on %d", drawn, tab.GridColumn)
	}

	model.Update(tea.KeyPressMsg{Code: 'H', Text: "H"})
	overlay := &model.Active().Overlay
	if overlay.Kind != app.OverlayColumns || overlay.Kept["1"] {
		t.Fatalf("H opened %q with %v kept", overlay.Kind, overlay.Kept)
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if drawn := drawnColumns(model); !slices.Contains(drawn, 1) {
		t.Errorf("the column checked again was not drawn: %v", drawn)
	}
}

func TestTheMoveKeysAndADragReorderTheColumns(t *testing.T) {
	model := buildLoadedModel(t, 1, 2, 12, 4)
	tab := model.Active().Active()
	tab.Focus = app.PaneResult
	tab.GridColumn = 0

	model.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt | tea.ModShift})
	if drawn := drawnColumns(model); drawn[0] != 1 || drawn[1] != 0 {
		t.Fatalf("alt+shift+right drew %v", drawn)
	}

	columns := model.layout.gridColumns
	row := model.layout.gridHeaderRow
	model.Update(tea.MouseClickMsg{X: columns[3].from + 1, Y: row, Button: tea.MouseLeft})
	model.Update(tea.MouseMotionMsg{X: columns[0].from + 1, Y: row, Button: tea.MouseLeft})
	model.Update(tea.MouseReleaseMsg{X: columns[0].from + 1, Y: row, Button: tea.MouseLeft})
	if drawn := drawnColumns(model); drawn[0] != 3 {
		t.Errorf("the drag of the fourth name drew %v", drawn)
	}
	if len(tab.Sort) != 0 {
		t.Error("a drag of a name sorted by it")
	}
}

func TestADraggedColumnShowsAGhostAndFlashesWhereItLands(t *testing.T) {
	model := buildLoadedModel(t, 1, 2, 12, 4)
	tab := model.Active().Active()
	tab.Focus = app.PaneResult
	model.render()

	columns := model.layout.gridColumns
	row := model.layout.gridHeaderRow
	name := model.buildGridShape(model.Active(), tab).Columns[columns[2].index].Name
	model.Update(tea.MouseClickMsg{X: columns[2].from + 1, Y: row, Button: tea.MouseLeft})
	model.Update(tea.MouseMotionMsg{X: columns[0].from + 1, Y: row, Button: tea.MouseLeft})
	model.View()
	if ghost := stripEscapes(strings.Split(model.frame.shown, "\n")[row+1]); !strings.Contains(ghost, name) {
		t.Errorf("the row under the pointer reads %q, wanted the ghost of %q", ghost, name)
	}
	model.Update(tea.MouseReleaseMsg{X: columns[0].from + 1, Y: row, Button: tea.MouseLeft})
	if model.animation.landing == nil {
		t.Error("the dropped column does not flash")
	}
}
