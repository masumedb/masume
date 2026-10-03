package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestTheMoveKeysMoveTheActiveTab(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	first := connection.OpenQueryTab("select 1")
	connection.OpenQueryTab("select 2")
	connection.OpenQueryTab("select 3")
	connection.ActivateTab(0)

	model.Update(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModAlt | tea.ModShift})
	if connection.Tabs[1] != first || connection.Active() != first {
		t.Errorf("alt+shift+down left the tab at %d", connection.ActiveIndex)
	}
	model.Update(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModAlt | tea.ModShift})
	model.Update(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModAlt | tea.ModShift})
	if connection.Tabs[0] != first || connection.Active() != first {
		t.Errorf("alt+shift+up past the first place left the tab at %d", connection.ActiveIndex)
	}
}

func TestADragMovesATabAlongTheRow(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	first := connection.OpenQueryTab("select 1")
	connection.OpenQueryTab("select 2")
	connection.OpenQueryTab("select 3")
	model.render()

	tabs := model.layout.tabs
	row := model.layout.tabRow
	model.Update(tea.MouseClickMsg{X: tabs[0].from + 1, Y: row, Button: tea.MouseLeft})
	model.render()
	last := model.layout.tabs[2]
	model.Update(tea.MouseMotionMsg{X: last.from + 1, Y: row, Button: tea.MouseLeft})
	model.Update(tea.MouseReleaseMsg{X: last.from + 1, Y: row, Button: tea.MouseLeft})
	if connection.Tabs[2] != first || connection.Active() != first {
		t.Errorf("the drag left the first tab at %d", connection.ActiveIndex)
	}
}

func TestADraggedTabShowsAGhostAndADimmedSlot(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	connection.OpenQueryTab("select 1")
	second := connection.OpenQueryTab("select 2")
	connection.ActivateTab(0)
	model.View()
	before := strings.Split(model.frame.shown, "\n")

	held := model.layout.tabs[1]
	row := model.layout.tabRow
	model.Update(tea.MouseClickMsg{X: held.from + 1, Y: row, Button: tea.MouseLeft})
	model.Update(tea.MouseMotionMsg{X: held.from + 3, Y: row, Button: tea.MouseLeft})
	model.View()
	frame := strings.Split(model.frame.shown, "\n")

	// The slot of the tab is drawn darker than it was.
	_, was := readCellColors(mapCells(before[row])[held.from+1].sgr)
	_, now := readCellColors(mapCells(frame[row])[held.from+1].sgr)
	if was == nil || now == nil || CalculateRelativeLuminance(now) >= CalculateRelativeLuminance(was) {
		t.Errorf("the slot of the dragged tab is drawn on %v, and was on %v", now, was)
	}
	// The ghost carries the label of the tab beside the pointer, on the row under it.
	ghost := stripEscapes(frame[row+1])
	if !strings.Contains(ghost, second.Label()) {
		t.Errorf("the row under the pointer reads %q, wanted the ghost of %q", ghost, second.Label())
	}
	if bar := stripEscapes(frame[model.layout.hintRow]); !strings.Contains(bar, "release to drop the tab") {
		t.Errorf("the bar during the drag reads %q", bar)
	}
	if connection.Active() == second {
		t.Error("the press activated the tab before the release")
	}
}

func TestAClickOnATabActivatesItOnTheRelease(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	connection.OpenQueryTab("select 1")
	second := connection.OpenQueryTab("select 2")
	connection.ActivateTab(0)
	model.render()

	held := model.layout.tabs[1]
	clickMouse(model, held.from+1, model.layout.tabRow)
	if connection.Active() != second {
		t.Error("a click on the tab left it inactive")
	}
}

func TestAPressOnATabLeavesTheSplitAsItIs(t *testing.T) {
	model, connection := buildSplitModel(t)
	left, right := findSideTab(t, model, 0), findSideTab(t, model, 1)
	third := connection.OpenQueryTab("select 3")
	connection.ActivateTab(connection.IndexOfTab(right.ID))
	model.render()

	held := model.layout.tabs[connection.IndexOfTab(third.ID)]
	model.Update(tea.MouseClickMsg{X: held.from + 1, Y: model.layout.tabRow, Button: tea.MouseLeft})
	model.render()
	if findSideTab(t, model, 0) != left || findSideTab(t, model, 1) != right {
		t.Error("the press on a tab changed the sides of the split view")
	}
}

func TestADroppedTabFlashesAndFades(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	connection.OpenQueryTab("select 1")
	connection.OpenQueryTab("select 2")
	model.render()

	tabs := model.layout.tabs
	row := model.layout.tabRow
	model.Update(tea.MouseClickMsg{X: tabs[0].from + 1, Y: row, Button: tea.MouseLeft})
	model.render()
	last := model.layout.tabs[1]
	model.Update(tea.MouseMotionMsg{X: last.from + 1, Y: row, Button: tea.MouseLeft})
	model.render()
	_, command := model.Update(tea.MouseReleaseMsg{X: last.from + 1, Y: row, Button: tea.MouseLeft})
	if command == nil || model.animation.landing == nil {
		t.Fatal("the drop started no flash")
	}
	if _, next := model.Update(wakeMsg{}); next == nil {
		t.Error("a frame of the flash asked for no next frame")
	}
	model.animation.landing.at = model.animation.landing.at.Add(-2 * landingFlash)
	model.animation.scheduled = false
	if _, next := model.Update(wakeMsg{}); next != nil || model.animation.landing != nil {
		t.Error("the flash went on after its time")
	}
}
