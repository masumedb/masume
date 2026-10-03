package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// liftTint is the weight a dragged cell mixes its ground toward the second accent with.
const liftTint = 0.25

// dragCell moves the focused cell of a notebook to the place of the cell under the pointer.
// The cell moves once the pointer passes the middle of the cell beside it, so a tall cell
// dragged over a short one stays where it moved to. A drag past either end of the list
// scrolls it.
func (model *Model) dragCell(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	connection := model.Active()
	if connection == nil || connection.Active().Notebook == nil {
		return model, nil
	}
	book := connection.Active().Notebook
	model.drag.lifted = true
	block := model.layout.cellRows
	switch {
	case mouse.Y <= block.top:
		book.Offset, book.Rolled = max(block.offset-1, 0), true
	case mouse.Y >= block.top+block.count-1:
		book.Offset, book.Rolled = block.offset+1, true
	}
	if block.count < 1 {
		return model, nil
	}
	item := block.offset + min(max(mouse.Y-block.top, 0), block.count-1)
	if item >= len(model.cellsOfRows) {
		return model, nil
	}
	target := model.cellsOfRows[item]
	if target == book.Focused {
		return model, nil
	}
	first, last := findCellRows(model.cellsOfRows, target)
	middle := first + (last-first)/2
	if (target > book.Focused && item >= middle) || (target < book.Focused && item <= middle) {
		book.MoveCellTo(target, !model.drag.moved)
		model.drag.moved = true
	}
	return model, nil
}

// findCellRows returns the first and the last row of the list a cell takes.
func findCellRows(cellsOfRows []int, cell int) (int, int) {
	first, last := -1, -1
	for row, held := range cellsOfRows {
		if held != cell {
			continue
		}
		if first < 0 {
			first = row
		}
		last = row
	}
	return first, last
}

// paintDraggedCell tints the rows of the cell a drag moves.
func (model *Model) paintDraggedCell(frame string) string {
	connection := model.Active()
	if !model.drag.holds(dragCell) || !model.drag.lifted || connection == nil ||
		connection.Active().Notebook == nil {
		return frame
	}
	focused := connection.Active().Notebook.Focused
	block := model.layout.cellRows
	rows := strings.Split(frame, "\n")
	for at := range block.count {
		item := block.offset + at
		row := block.top + at
		if item >= len(model.cellsOfRows) || model.cellsOfRows[item] != focused ||
			row >= len(rows) {
			continue
		}
		cells := mapCells(rows[row])
		model.tintRowCells(cells, block.from, block.to, model.styles.Theme.AccentAlt, liftTint)
		rows[row] = writeCells(cells)
	}
	return strings.Join(rows, "\n")
}
