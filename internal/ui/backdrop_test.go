package ui

import (
	"strings"
	"testing"
)

// findBrightCell returns the first cell outside the rectangle that has text in another ink
// than the faint one.
func findBrightCell(model *Model, frame []string, rect blockRect) (string, int, bool) {
	faint := describeInk(model.styles.Theme.Faint)
	for row := model.layout.titleRow + 1; row < model.height-1; row++ {
		for column, cell := range mapCells(frame[row]) {
			if row >= rect.fromY && row <= rect.toY &&
				column >= rect.fromX && column <= rect.toX {
				continue
			}
			if strings.TrimSpace(cell.text) != "" && !strings.Contains(cell.sgr, faint) {
				return cell.text, row, true
			}
		}
	}
	return "", 0, false
}

func TestTheFrameUnderACardIsDimmed(t *testing.T) {
	model, _ := buildObjectMenuModel(t)
	frame := strings.Split(model.render(), "\n")

	inside := model.layout.selectionBlocks[0]
	card := blockRect{
		fromX: inside.fromX - 1, toX: inside.toX + 1,
		fromY: inside.fromY - 1, toY: inside.toY + 1,
	}
	if text, row, found := findBrightCell(model, frame, card); found {
		t.Errorf("the cell %q on row %d beside the card is not dimmed", text, row)
	}
	if !strings.Contains(stripEscapes(frame[model.layout.tabRow]), "1") {
		t.Errorf("the tab row under the card lost its text: %q",
			stripEscapes(frame[model.layout.tabRow]))
	}
}

func TestTheQuitQuestionKeepsTheWorkspaceDimmed(t *testing.T) {
	model, _, tab := buildTableTabModel(t)
	stageCellEdits(tab, 1)
	before := strings.Split(model.render(), "\n")
	tabRow := stripEscapes(before[model.layout.tabRow])

	model.readKey(pressCtrlC())
	frame := strings.Split(model.render(), "\n")

	if got := stripEscapes(frame[model.layout.tabRow]); got != tabRow {
		t.Errorf("the tab row under the question reads %q, wanted %q", got, tabRow)
	}
	faint := describeInk(model.styles.Theme.Faint)
	for _, cell := range mapCells(frame[model.layout.tabRow]) {
		if strings.TrimSpace(cell.text) != "" && !strings.Contains(cell.sgr, faint) {
			t.Errorf("the cell %q of the tab row under the question is not dimmed", cell.text)
			break
		}
	}
	bar := stripEscapes(frame[model.height-1])
	if !strings.Contains(bar, "cancel") {
		t.Errorf("the status bar under the question reads %q", bar)
	}
	if len(model.layout.buttons) > 0 {
		t.Errorf("the frame under the question has %d live buttons", len(model.layout.buttons))
	}
}
