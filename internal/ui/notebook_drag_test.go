package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

const threeCells = "```sql id=first\nselect 1\n```\n\n```sql id=second\nselect 2\n```\n\n" +
	"```sql id=third\nselect 3\n```\n"

// findCellRow returns the screen row of the last row a cell takes in the list.
func findCellRow(t *testing.T, model *Model, cell int) int {
	t.Helper()
	_, last := findCellRows(model.cellsOfRows, cell)
	if last < 0 {
		t.Fatalf("the list draws no row of cell %d", cell+1)
	}
	return model.layout.cellRows.top + last - model.layout.cellRows.offset
}

// readCellIDs returns the ids of the cells in their order.
func readCellIDs(model *Model) string {
	ids := []string{}
	for _, cell := range model.Active().Active().Notebook.Cells {
		ids = append(ids, cell.ID)
	}
	return strings.Join(ids, " ")
}

func TestADragMovesACellAndOneUndoPutsItBack(t *testing.T) {
	model, tab := buildNotebookModel(t, threeCells)
	model.render()
	x := model.layout.cellRows.from + 4
	press := model.layout.cellRows.top

	model.Update(tea.MouseClickMsg{X: x, Y: press, Button: tea.MouseLeft})
	model.render()
	model.Update(tea.MouseMotionMsg{X: x, Y: findCellRow(t, model, 1), Button: tea.MouseLeft})
	frame := strings.Split(model.render(), "\n")
	model.Update(tea.MouseMotionMsg{X: x, Y: findCellRow(t, model, 2), Button: tea.MouseLeft})
	model.render()
	model.Update(tea.MouseReleaseMsg{X: x, Y: findCellRow(t, model, 2), Button: tea.MouseLeft})

	if ids := readCellIDs(model); ids != "second third first" {
		t.Fatalf("the drag left the cells as %q", ids)
	}
	if tab.Notebook.Focused != 2 {
		t.Errorf("the focus stands on cell %d, wanted the moved cell", tab.Notebook.Focused+1)
	}
	if bar := stripEscapes(frame[model.layout.hintRow]); !strings.Contains(bar, "release to drop the cell") {
		t.Errorf("the bar during the drag reads %q", bar)
	}
	if !tab.Notebook.UndoCellChange() || readCellIDs(model) != "first second third" {
		t.Errorf("one undo left the cells as %q", readCellIDs(model))
	}
}

func TestAClickOnACellMovesNoCell(t *testing.T) {
	model, _ := buildNotebookModel(t, threeCells)
	model.render()
	x, y := model.layout.cellRows.from+4, findCellRow(t, model, 1)
	model.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	model.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	if ids := readCellIDs(model); ids != "first second third" {
		t.Errorf("a click left the cells as %q", ids)
	}
}

func TestADraggedCellShowsAGhostAndFlashesWhereItLands(t *testing.T) {
	model, tab := buildNotebookModel(t, threeCells)
	model.render()
	x := model.layout.cellRows.from + 4
	model.Update(tea.MouseClickMsg{X: x, Y: model.layout.cellRows.top, Button: tea.MouseLeft})
	model.render()
	y := findCellRow(t, model, 1)
	model.Update(tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft})
	model.View()
	title := tab.Notebook.GetFocusedCell().BuildTitle()
	if ghost := stripEscapes(strings.Split(model.frame.shown, "\n")[model.frame.pointerY+1]); !strings.Contains(ghost, title) {
		t.Errorf("the row under the pointer reads %q, wanted the ghost of %q", ghost, title)
	}
	model.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	if model.animation.landing == nil {
		t.Error("the dropped cell does not flash")
	}
}
