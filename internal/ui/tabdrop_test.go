package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// dragTabTo presses the tab at that place of the tab row, moves the pointer to the cell and
// back over it, and returns the frame drawn while the pointer stands there.
func dragTabTo(model *Model, index, x, y int) []string {
	model.render()
	held := model.layout.tabs[index]
	row := model.layout.tabRow
	model.Update(tea.MouseClickMsg{X: held.from + 1, Y: row, Button: tea.MouseLeft})
	model.Update(tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft})
	model.View()
	return strings.Split(model.frame.shown, "\n")
}

// releaseAt releases the left button on that cell.
func releaseAt(model *Model, x, y int) {
	model.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	model.render()
}

// centreOf returns the cell in the middle of a side.
func centreOf(rect sideRect) (int, int) {
	return rect.left + rect.width/2, rect.top + rect.height/2
}

func TestDroppingATabOnTheRightHalfOpensTheSplit(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 40, 3)
	connection := model.Active()
	first := connection.Active()
	second := connection.OpenQueryTab("select 2")
	connection.ActivateTab(0)
	model.render()

	zones, _, room := model.planDropZones()
	if !room {
		t.Fatal("the panes have no room for two sides")
	}
	x, y := centreOf(zones[1])
	frame := dragTabTo(model, 1, x, y)
	if bar := stripEscapes(frame[model.layout.hintRow]); !strings.Contains(bar, "release to split, with the tab on the right") {
		t.Errorf("the bar during the drag reads %q", bar)
	}
	model.drag.dropSince = model.drag.dropSince.Add(-dropFadeIn)
	model.View()
	frame = strings.Split(model.frame.shown, "\n")
	other, _ := centreOf(zones[0])
	_, inside := readCellColors(mapCells(frame[y])[x].sgr)
	_, outside := readCellColors(mapCells(frame[y])[other].sgr)
	if inside == nil || outside == nil || WriteHex(inside) == WriteHex(outside) {
		t.Errorf("the drop zone is drawn on %v, and the other half on %v", inside, outside)
	}
	releaseAt(model, x, y)

	if !model.split.open || findSideTab(t, model, 0) != first || findSideTab(t, model, 1) != second {
		t.Fatalf("the drop left the split %v", model.split)
	}
	if model.split.focused != 1 || connection.Active() != second {
		t.Error("the dropped tab does not have the focus")
	}
}

func TestDroppingATabOnASideShowsItThere(t *testing.T) {
	model, connection := buildSplitModel(t)
	right := findSideTab(t, model, 1)
	third := connection.OpenQueryTab("select 3")
	connection.ActivateTab(connection.IndexOfTab(right.ID))
	model.render()

	x, y := centreOf(model.layout.sideRects[0])
	dragTabTo(model, connection.IndexOfTab(third.ID), x, y)
	releaseAt(model, x, y)
	if findSideTab(t, model, 0) != third || findSideTab(t, model, 1) != right {
		t.Errorf("the drop on the left shows %q and %q", findSideTab(t, model, 0).Label(),
			findSideTab(t, model, 1).Label())
	}
	if model.split.focused != 0 {
		t.Error("the side the tab landed on does not have the focus")
	}
}

func TestDroppingTheTabOfOneSideOnTheOtherSwapsThem(t *testing.T) {
	model, _ := buildSplitModel(t)
	left, right := findSideTab(t, model, 0), findSideTab(t, model, 1)

	x, y := centreOf(model.layout.sideRects[1])
	dragTabTo(model, model.Active().IndexOfTab(left.ID), x, y)
	releaseAt(model, x, y)
	if findSideTab(t, model, 0) != right || findSideTab(t, model, 1) != left {
		t.Error("the drop of the left tab on the right did not swap the sides")
	}
}

func TestTheOtherSideKeyMovesTheTabThere(t *testing.T) {
	model, _ := buildSplitModel(t)
	left, right := findSideTab(t, model, 0), findSideTab(t, model, 1)

	pressKey(t, model, focusPaneKey)
	pressKey(t, model, tea.KeyPressMsg{Code: 'm', Text: "m"})
	model.render()
	if findSideTab(t, model, 0) != right || findSideTab(t, model, 1) != left ||
		model.split.focused != 0 {
		t.Errorf("alt+p m left the split %v", model.split)
	}
}

func TestADragOverThePanesLeavesTheTabOrder(t *testing.T) {
	model := buildLoadedModel(t, 1, 3, 40, 3)
	connection := model.Active()
	connection.OpenQueryTab("select 2")
	connection.OpenQueryTab("select 3")
	model.render()
	order := []int{connection.Tabs[0].ID, connection.Tabs[1].ID, connection.Tabs[2].ID}

	last := model.layout.tabs[2]
	dragTabTo(model, 0, last.from+1, model.layout.tabRow+5)
	for at, tab := range connection.Tabs {
		if tab.ID != order[at] {
			t.Fatalf("a drag over the panes moved the tabs to %v", connection.Tabs)
		}
	}
}
