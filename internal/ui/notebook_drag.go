package ui

import (
	tea "charm.land/bubbletea/v2"
)

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
