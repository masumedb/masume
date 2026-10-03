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

func TestADraggedTabIsDrawnLifted(t *testing.T) {
	model := buildOfflineModel(t, 120, 40)
	connection := model.Active()
	connection.OpenQueryTab("select 1")
	connection.OpenQueryTab("select 2")
	model.render()

	held := model.layout.tabs[1]
	row := model.layout.tabRow
	model.Update(tea.MouseClickMsg{X: held.from + 1, Y: row, Button: tea.MouseLeft})
	model.Update(tea.MouseMotionMsg{X: held.from + 2, Y: row, Button: tea.MouseLeft})
	frame := strings.Split(model.render(), "\n")

	cell := mapCells(frame[row])[held.from+1]
	if _, ground := readCellColors(cell.sgr); ground == nil ||
		WriteHex(ground) != WriteHex(model.styles.Theme.AccentAlt) {
		t.Errorf("the dragged tab is drawn on %v", ground)
	}
	if bar := stripEscapes(frame[model.layout.hintRow]); !strings.Contains(bar, "release to drop the tab") {
		t.Errorf("the bar during the drag reads %q", bar)
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
